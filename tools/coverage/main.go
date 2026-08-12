// Command coverage enforces akt's checked-in coverage contract.
//
// It uses Go's standard library plus the repository's pinned YAML parser so CI
// can run it without installing or trusting a mutable external binary.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	goscanner "go/scanner"
	"go/token"
	"io"
	"math"
	"math/bits"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	classActive          = "active"
	classExperimentalTUI = "experimental-tui"
	classTooling         = "tooling"
	classSupport         = "support"

	activeBaselinePath          = "coverage/baseline-active-union.tsv"
	experimentalTUIBaselinePath = "coverage/baseline-experimental-tui-union.tsv"
	toolingBaselinePath         = "coverage/baseline-tooling-unit.tsv"
	binaryIdentityFilename      = "binary-identity.tsv"
)

type packageEntry struct {
	Name     string
	Class    string
	Critical bool
}

type packageSet struct {
	Entries []packageEntry
	ByName  map[string]packageEntry
	Module  string
}

type coverBlock struct {
	File                   string
	StartLine, StartColumn int
	EndLine, EndColumn     int
	Statements             int64
	Count                  uint64
	Raw                    string
}

type coverProfile struct {
	Mode       string
	Blocks     []coverBlock
	EdgeBlocks []coverBlock
}

type sourceRegion struct {
	StartLine, StartColumn int
	EndLine, EndColumn     int
	SyntheticEdge          bool
}

type statementCount struct {
	Covered int64
	Total   int64
}

type packageLocation struct {
	ImportPath string
	Dir        string
}

type exception struct {
	Package  string
	File     string
	Line     int
	Deadline time.Time
}

type binaryIdentity struct {
	BinaryDigest         string
	SourceManifestDigest string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "coverage:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("expected one of: validate, source-manifest, binary-identity, verify-binary-identity, verify-shard, filter, report, baseline, ratchet, patch")
	}

	switch args[0] {
	case "validate":
		return runValidate(args[1:])
	case "source-manifest":
		return runSourceManifest(args[1:])
	case "binary-identity":
		return runBinaryIdentity(args[1:])
	case "verify-binary-identity":
		return runVerifyBinaryIdentity(args[1:])
	case "verify-shard":
		return runVerifyShard(args[1:])
	case "filter":
		return runFilter(args[1:])
	case "report":
		return runReport(args[1:])
	case "baseline":
		return runBaseline(args[1:])
	case "ratchet":
		return runRatchet(args[1:])
	case "patch":
		return runPatch(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runVerifyShard(args []string) error {
	fs := flag.NewFlagSet("verify-shard", flag.ContinueOnError)
	directory := fs.String("dir", "", "raw Go coverage data directory")
	name := fs.String("name", "coverage", "human-readable shard name")
	expectedManifest := fs.String("source-manifest", "", "source manifest generated from the reporting checkout")
	requireBinaryIdentity := fs.Bool("require-binary-identity", false, "require and validate an E2E binary identity")
	binary := fs.String("binary", "", "exact E2E binary to verify during collection")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *directory == "" || *expectedManifest == "" {
		return errors.New("verify-shard requires -dir and -source-manifest")
	}
	if err := verifySourceManifest(filepath.Join(*directory, "source-manifest.tsv"), *expectedManifest); err != nil {
		return fmt.Errorf("%s shard: %w", *name, err)
	}
	hasBinaryIdentity := *requireBinaryIdentity || *binary != ""
	if hasBinaryIdentity {
		if err := verifyBinaryIdentityFile(
			*binary,
			filepath.Join(*directory, "source-manifest.tsv"),
			filepath.Join(*directory, binaryIdentityFilename),
		); err != nil {
			return fmt.Errorf("%s shard: %w", *name, err)
		}
	}
	metadata, counters, err := verifyCovdataShard(*directory, hasBinaryIdentity)
	if err != nil {
		return fmt.Errorf("%s shard: %w", *name, err)
	}
	fmt.Printf("%s coverage shard: %d metadata files, %d counter files\n", *name, metadata, counters)
	return nil
}

func runBinaryIdentity(args []string) error {
	fs := flag.NewFlagSet("binary-identity", flag.ContinueOnError)
	binary := fs.String("binary", "", "coverage-instrumented executable")
	sourceManifest := fs.String("source-manifest", "", "build-consistent source manifest")
	out := fs.String("out", "", "output binary identity path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *binary == "" || *sourceManifest == "" || *out == "" {
		return errors.New("binary-identity requires -binary, -source-manifest, and -out")
	}
	contents, err := buildBinaryIdentity(*binary, *sourceManifest)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, contents, 0o600); err != nil {
		return fmt.Errorf("write binary identity %s: %w", *out, err)
	}
	return nil
}

func runVerifyBinaryIdentity(args []string) error {
	fs := flag.NewFlagSet("verify-binary-identity", flag.ContinueOnError)
	binary := fs.String("binary", "", "coverage-instrumented executable")
	sourceManifest := fs.String("source-manifest", "", "build-consistent source manifest")
	identity := fs.String("identity", "", "recorded binary identity")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *binary == "" || *sourceManifest == "" || *identity == "" {
		return errors.New("verify-binary-identity requires -binary, -source-manifest, and -identity")
	}
	return verifyBinaryIdentityFile(*binary, *sourceManifest, *identity)
}

func runSourceManifest(args []string) error {
	fs := flag.NewFlagSet("source-manifest", flag.ContinueOnError)
	root := fs.String("root", ".", "repository root")
	out := fs.String("out", "", "output manifest path")
	buildTags := fs.String("build-tags", "", "evaluated comma-separated release build tags")
	buildOptions := fs.String("build-options", "", "evaluated Make BUILD_OPTIONS")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("source-manifest requires -out")
	}
	manifest, err := buildSourceManifest(*root, *buildTags, *buildOptions)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, manifest, 0o600); err != nil {
		return fmt.Errorf("write source manifest %s: %w", *out, err)
	}
	return nil
}

func buildSourceManifest(root, buildTags, buildOptions string) ([]byte, error) {
	goEnvironment, err := effectiveGoBuildEnvironment(root)
	if err != nil {
		return nil, err
	}
	if goEnvironment["GOWORK"] != "off" {
		return nil, fmt.Errorf("source manifest requires GOWORK=off, got %q", goEnvironment["GOWORK"])
	}
	canonicalTags, err := canonicalBuildTags(buildTags)
	if err != nil {
		return nil, fmt.Errorf("source manifest build tags: %w", err)
	}
	embeddedInputs, err := discoverEmbeddedInputs(root, canonicalTags)
	if err != nil {
		return nil, err
	}
	defaultEmbeddedInputs, err := discoverEmbeddedInputs(root, "")
	if err != nil {
		return nil, err
	}
	for name := range defaultEmbeddedInputs {
		embeddedInputs[name] = true
	}
	entries := map[string]string{
		"@build-options": buildOptions,
		"@build-tags":    canonicalTags,
		"@go-version":    runtime.Version(),
		"@platform":      goEnvironment["GOOS"] + "/" + goEnvironment["GOARCH"],
	}
	for key, value := range goEnvironment {
		entries["@go-env-"+key] = value
	}
	err = filepath.WalkDir(root, func(filename string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if rel == ".git" || rel == ".cache" {
				return filepath.SkipDir
			}
			return nil
		}
		if !sourceManifestInput(rel) && !embeddedInputs[rel] {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("source manifest input %s is not a regular file", rel)
		}
		contents, err := os.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("read source manifest input %s: %w", rel, err)
		}
		entries[rel] = string(contents)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk source manifest root %s: %w", root, err)
	}

	paths := make([]string, 0, len(entries))
	for filename := range entries {
		paths = append(paths, filename)
	}
	sort.Strings(paths)
	var result strings.Builder
	writer := csv.NewWriter(&result)
	writer.Comma = '\t'
	if err := writer.Write([]string{"path", "sha256"}); err != nil {
		return nil, fmt.Errorf("write source manifest header: %w", err)
	}
	for _, filename := range paths {
		digest := sha256.Sum256([]byte(entries[filename]))
		if err := writer.Write([]string{filename, fmt.Sprintf("%x", digest)}); err != nil {
			return nil, fmt.Errorf("write source manifest entry %q: %w", filename, err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("write source manifest: %w", err)
	}
	return []byte(result.String()), nil
}

func discoverEmbeddedInputs(root, buildTags string) (map[string]bool, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve source manifest root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve source manifest root symlinks: %w", err)
	}

	arguments := []string{"list", "-json", "-test"}
	if strings.TrimSpace(buildTags) != "" {
		canonicalTags, err := canonicalBuildTags(buildTags)
		if err != nil {
			return nil, fmt.Errorf("source manifest build tags: %w", err)
		}
		arguments = append(arguments, "-tags="+canonicalTags)
	}
	arguments = append(arguments, "./...")
	command := exec.Command("go", arguments...)
	command.Dir = root
	command.Env = replaceEnv(os.Environ(), "GOWORK", "off")
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("discover go:embed inputs: %s", strings.TrimSpace(stderr.String()))
	}

	type listedPackage struct {
		Dir             string
		EmbedFiles      []string
		TestEmbedFiles  []string
		XTestEmbedFiles []string
	}
	inputs := make(map[string]bool)
	decoder := json.NewDecoder(bytes.NewReader(output))
	for {
		var listed listedPackage
		if err := decoder.Decode(&listed); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("decode go:embed package inventory: %w", err)
		}
		for _, name := range append(append(listed.EmbedFiles, listed.TestEmbedFiles...), listed.XTestEmbedFiles...) {
			filename := filepath.Join(listed.Dir, filepath.FromSlash(name))
			rel, err := filepath.Rel(root, filename)
			if err != nil {
				return nil, fmt.Errorf("resolve embedded input %s: %w", filename, err)
			}
			if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return nil, fmt.Errorf("embedded input %s escapes repository root", filename)
			}
			inputs[filepath.ToSlash(rel)] = true
		}
	}
	return inputs, nil
}

func effectiveGoBuildEnvironment(root string) (map[string]string, error) {
	keys := []string{
		"AR", "CC", "CGO_CFLAGS", "CGO_CPPFLAGS", "CGO_CXXFLAGS", "CGO_ENABLED", "CGO_FFLAGS", "CGO_LDFLAGS", "CXX", "FC",
		"GO386", "GOAMD64", "GOARCH", "GOARM", "GOARM64", "GOEXPERIMENT", "GOFLAGS", "GOMIPS",
		"GOMIPS64", "GOOS", "GOPPC64", "GORISCV64", "GOFIPS140", "GO_EXTLINK_ENABLED", "GO_LDSO", "GOTOOLCHAIN", "GOWORK",
		"PKG_CONFIG",
	}
	args := append([]string{"env", "-json"}, keys...)
	command := exec.Command("go", args...)
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("inspect effective Go build environment: %w", err)
	}
	values := make(map[string]string, len(keys))
	if err := json.Unmarshal(output, &values); err != nil {
		return nil, fmt.Errorf("decode effective Go build environment: %w", err)
	}
	for _, key := range keys {
		if _, ok := values[key]; !ok {
			return nil, fmt.Errorf("effective Go build environment omitted %s", key)
		}
	}
	return values, nil
}

func sourceManifestInput(filename string) bool {
	if goBuildSourceInput(filename) {
		return true
	}
	if strings.Contains("/"+filepath.ToSlash(filename), "/testdata/") {
		return true
	}
	switch filename {
	case ".env", ".envrc", ".goreleaser.yaml", ".goreleaser.yml",
		"go.mod", "go.sum", "Makefile":
		return true
	}
	if strings.HasPrefix(filename, ".github/workflows/") &&
		(strings.HasSuffix(filename, ".yaml") || strings.HasSuffix(filename, ".yml")) {
		return true
	}
	if filename == "coverage/packages.tsv" {
		return true
	}
	return strings.HasPrefix(filename, "make/") && strings.HasSuffix(filename, ".mk")
}

func goBuildSourceInput(filename string) bool {
	switch filepath.Ext(filename) {
	case ".go", ".c", ".cc", ".cpp", ".cxx", ".h", ".hh", ".hpp", ".hxx", ".m", ".mm", ".f", ".F", ".f90", ".for", ".s", ".S", ".sx", ".swig", ".swigcxx", ".syso":
		return true
	default:
		return false
	}
}

func verifySourceManifest(shardFile, expectedFile string) error {
	shard, err := loadSourceManifest(shardFile)
	if err != nil {
		return fmt.Errorf("load collected source manifest: %w", err)
	}
	expected, err := loadSourceManifest(expectedFile)
	if err != nil {
		return fmt.Errorf("load current source manifest: %w", err)
	}
	if string(shard) == string(expected) {
		return nil
	}
	shardDigest := sha256.Sum256(shard)
	expectedDigest := sha256.Sum256(expected)
	return fmt.Errorf(
		"source manifest mismatch (collected %x, current %x); recollect this shard",
		shardDigest[:8], expectedDigest[:8],
	)
}

func loadSourceManifest(filename string) ([]byte, error) {
	contents, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	reader := csv.NewReader(strings.NewReader(string(contents)))
	reader.Comma = '\t'
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}
	if len(records) < 3 || len(records[0]) != 2 || records[0][0] != "path" || records[0][1] != "sha256" {
		return nil, fmt.Errorf("%s has an invalid header or no source entries", filename)
	}
	previous := ""
	digestRE := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for row, record := range records[1:] {
		if len(record) != 2 || record[0] == "" || !digestRE.MatchString(record[1]) {
			return nil, fmt.Errorf("%s row %d is invalid", filename, row+2)
		}
		if previous >= record[0] {
			return nil, fmt.Errorf("%s paths are not strictly sorted at row %d", filename, row+2)
		}
		previous = record[0]
	}
	return contents, nil
}

func buildBinaryIdentity(binaryFile, sourceManifestFile string) ([]byte, error) {
	binaryDigest, err := digestRegularFile(binaryFile)
	if err != nil {
		return nil, fmt.Errorf("digest coverage binary: %w", err)
	}
	manifest, err := loadSourceManifest(sourceManifestFile)
	if err != nil {
		return nil, fmt.Errorf("load binary source manifest: %w", err)
	}
	manifestDigest := sha256.Sum256(manifest)
	return []byte(fmt.Sprintf(
		"artifact\tsha256\nbinary\t%x\nsource-manifest\t%x\n",
		binaryDigest, manifestDigest,
	)), nil
}

func verifyBinaryIdentityFile(binaryFile, sourceManifestFile, identityFile string) error {
	identity, err := loadBinaryIdentity(identityFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("binary identity is required: %w", err)
		}
		return fmt.Errorf("load binary identity: %w", err)
	}
	manifest, err := loadSourceManifest(sourceManifestFile)
	if err != nil {
		return fmt.Errorf("load binary source manifest: %w", err)
	}
	manifestDigest := fmt.Sprintf("%x", sha256.Sum256(manifest))
	if identity.SourceManifestDigest != manifestDigest {
		return fmt.Errorf(
			"source manifest digest mismatch (recorded %.16s, current %.16s); rebuild and recollect this shard",
			identity.SourceManifestDigest, manifestDigest,
		)
	}
	if binaryFile == "" {
		return nil
	}
	binaryDigest, err := digestRegularFile(binaryFile)
	if err != nil {
		return fmt.Errorf("digest coverage binary: %w", err)
	}
	currentBinaryDigest := fmt.Sprintf("%x", binaryDigest)
	if identity.BinaryDigest != currentBinaryDigest {
		return fmt.Errorf(
			"binary digest mismatch (recorded %.16s, current %.16s); rebuild and recollect this shard",
			identity.BinaryDigest, currentBinaryDigest,
		)
	}
	return nil
}

func loadBinaryIdentity(filename string) (binaryIdentity, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		return binaryIdentity{}, fmt.Errorf("inspect %s: %w", filename, err)
	}
	if !info.Mode().IsRegular() {
		return binaryIdentity{}, fmt.Errorf("%s must be a regular file", filename)
	}
	contents, err := os.ReadFile(filename)
	if err != nil {
		return binaryIdentity{}, fmt.Errorf("read %s: %w", filename, err)
	}
	reader := csv.NewReader(strings.NewReader(string(contents)))
	reader.Comma = '\t'
	records, err := reader.ReadAll()
	if err != nil {
		return binaryIdentity{}, fmt.Errorf("parse %s: %w", filename, err)
	}
	if len(records) != 3 {
		return binaryIdentity{}, fmt.Errorf("%s must contain exactly a header and two digest rows", filename)
	}
	if len(records[0]) != 2 || records[0][0] != "artifact" || records[0][1] != "sha256" {
		return binaryIdentity{}, fmt.Errorf("%s has an invalid header", filename)
	}
	expectedNames := []string{"binary", "source-manifest"}
	digests := make([]string, len(expectedNames))
	digestRE := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for index, expectedName := range expectedNames {
		record := records[index+1]
		if len(record) != 2 || record[0] != expectedName || !digestRE.MatchString(record[1]) {
			return binaryIdentity{}, fmt.Errorf("%s row %d must be %s and a lowercase SHA-256 digest", filename, index+2, expectedName)
		}
		digests[index] = record[1]
	}
	return binaryIdentity{BinaryDigest: digests[0], SourceManifestDigest: digests[1]}, nil
}

func digestRegularFile(filename string) ([sha256.Size]byte, error) {
	var zero [sha256.Size]byte
	pathInfo, err := os.Lstat(filename)
	if err != nil {
		return zero, err
	}
	if !pathInfo.Mode().IsRegular() {
		return zero, fmt.Errorf("%s is not a regular file", filename)
	}
	file, err := os.Open(filename)
	if err != nil {
		return zero, err
	}
	defer func() { _ = file.Close() }()
	before, err := file.Stat()
	if err != nil {
		return zero, err
	}
	if before.Size() == 0 {
		return zero, fmt.Errorf("%s is empty", filename)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return zero, err
	}
	after, err := file.Stat()
	if err != nil {
		return zero, err
	}
	currentPathInfo, err := os.Lstat(filename)
	if err != nil {
		return zero, err
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) ||
		!currentPathInfo.Mode().IsRegular() || !os.SameFile(after, currentPathInfo) {
		return zero, fmt.Errorf("%s changed while its digest was calculated", filename)
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return digest, nil
}

func verifyCovdataShard(directory string, allowBinaryIdentity bool) (int, int, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, 0, fmt.Errorf("read %s: %w", directory, err)
	}

	metadataHashes := make(map[string]struct{})
	counterHashes := make(map[string]int)
	metadataName := regexp.MustCompile(`^covmeta\.([0-9a-f]{32})$`)
	counterName := regexp.MustCompile(`^covcounters\.([0-9a-f]{32})\.([0-9]+)\.([0-9]+)$`)
	for _, entry := range entries {
		kind := ""
		hash := ""
		switch matches := metadataName.FindStringSubmatch(entry.Name()); {
		case entry.Name() == "source-manifest.tsv":
			if !entry.Type().IsRegular() {
				return 0, 0, errors.New("source manifest is not a regular file")
			}
			continue
		case entry.Name() == binaryIdentityFilename && allowBinaryIdentity:
			if !entry.Type().IsRegular() {
				return 0, 0, errors.New("binary identity is not a regular file")
			}
			continue
		case matches != nil:
			kind = "metadata"
			hash = matches[1]
		case strings.HasPrefix(entry.Name(), "covmeta."):
			return 0, 0, fmt.Errorf("metadata file %s has an invalid Go covdata name", entry.Name())
		case counterName.MatchString(entry.Name()):
			kind = "counter"
			hash = counterName.FindStringSubmatch(entry.Name())[1]
		case strings.HasPrefix(entry.Name(), "covcounters."):
			return 0, 0, fmt.Errorf("counter file %s has an invalid Go covdata name", entry.Name())
		default:
			return 0, 0, fmt.Errorf("unexpected coverage shard entry %s", entry.Name())
		}
		if !entry.Type().IsRegular() {
			return 0, 0, fmt.Errorf("%s file %s is not a regular file", kind, entry.Name())
		}
		info, err := entry.Info()
		if err != nil {
			return 0, 0, fmt.Errorf("inspect %s file %s: %w", kind, entry.Name(), err)
		}
		if info.Size() == 0 {
			return 0, 0, fmt.Errorf("%s file %s is empty", kind, entry.Name())
		}
		if kind == "metadata" {
			metadataHashes[hash] = struct{}{}
		} else {
			counterHashes[hash]++
		}
	}
	metadata := len(metadataHashes)
	counters := 0
	for hash, count := range counterHashes {
		if _, ok := metadataHashes[hash]; !ok {
			return metadata, counters, fmt.Errorf("counter hash %s has no matching covmeta file", hash)
		}
		counters += count
	}
	if metadata == 0 || counters == 0 {
		return metadata, counters, fmt.Errorf("expected non-empty covmeta and covcounters files, found %d metadata and %d counter files", metadata, counters)
	}
	orphanMetadata := make([]string, 0)
	for hash := range metadataHashes {
		if counterHashes[hash] == 0 {
			orphanMetadata = append(orphanMetadata, hash)
		}
	}
	if len(orphanMetadata) > 0 {
		sort.Strings(orphanMetadata)
		return metadata, counters, fmt.Errorf("metadata hash %s has no matching covcounters file", orphanMetadata[0])
	}
	if err := decodeCovdataShard(directory); err != nil {
		return metadata, counters, err
	}
	return metadata, counters, nil
}

func decodeCovdataShard(directory string) error {
	command := exec.Command("go", "tool", "covdata", "percent", "-i="+directory)
	command.Env = replaceEnv(os.Environ(), "GOWORK", "off")
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go toolchain rejected coverage data: %s", boundedDiagnostic(output, 512))
	}
	return nil
}

func boundedDiagnostic(value []byte, limit int) string {
	value = bytes.TrimSpace(value)
	if len(value) == 0 {
		return "no diagnostic"
	}
	if len(value) > limit {
		return string(value[:limit]) + "..."
	}
	return string(value)
}

func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	packagesFile := fs.String("packages", "coverage/packages.tsv", "package taxonomy")
	exceptionsFile := fs.String("exceptions", "coverage/exceptions.tsv", "reviewed exceptions TSV")
	releaseTags := fs.String("release-tags", "", "comma-separated release build tags")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*releaseTags) == "" {
		return errors.New("validate requires -release-tags")
	}
	packages, err := loadPackages(*packagesFile)
	if err != nil {
		return err
	}
	exceptions, err := loadExceptions(*exceptionsFile, packages)
	if err != nil {
		return err
	}
	root, err := moduleRoot()
	if err != nil {
		return err
	}
	if err := validateWorkflowActionPins(root); err != nil {
		return err
	}
	actual, err := discoverSourcePackages(root, packages.Module)
	if err != nil {
		return err
	}
	releaseDependencies, err := listReleaseDependencies(*releaseTags)
	if err != nil {
		return err
	}
	repositoryImports, err := listRepositoryImports(*releaseTags)
	if err != nil {
		return err
	}
	releaseLocations, err := listReleaseDependencyLocations(*releaseTags)
	if err != nil {
		return err
	}
	monitorRuntime := packages.Module + "/internal/monitor/runtime"
	monitorDependencies, err := listPackageDependencies(*releaseTags, "./internal/monitor/runtime")
	if err != nil {
		return err
	}
	if err := validateGoreleaserReleaseTags(filepath.Join(root, ".goreleaser.yaml"), *releaseTags); err != nil {
		return err
	}
	if err := validateTaxonomy(packages, exceptions, actual, releaseDependencies); err != nil {
		return err
	}
	if err := validateLocalReleaseDependencies(root, packages, releaseLocations); err != nil {
		return err
	}
	if err := validateExperimentalImportBoundary(packages, repositoryImports); err != nil {
		return err
	}
	if err := validateActiveDependencyClosure(packages, monitorRuntime, monitorDependencies); err != nil {
		return err
	}

	fmt.Printf("coverage package taxonomy: %d packages validated\n", len(actual))
	return nil
}

func validateWorkflowActionPins(root string) error {
	directory := filepath.Join(root, ".github", "workflows")
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read workflow directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		extension := strings.ToLower(filepath.Ext(entry.Name()))
		if extension != ".yml" && extension != ".yaml" {
			continue
		}

		filename := filepath.Join(directory, entry.Name())
		contents, err := os.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("read workflow %s: %w", entry.Name(), err)
		}
		var document yaml.Node
		if err := yaml.Unmarshal(contents, &document); err != nil {
			return fmt.Errorf("parse workflow %s: %w", entry.Name(), err)
		}
		if err := validateWorkflowNodeActionPins(&document, entry.Name()); err != nil {
			return err
		}
	}

	return nil
}

func validateWorkflowNodeActionPins(node *yaml.Node, workflow string) error {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			key := node.Content[index]
			value := node.Content[index+1]
			if key.Value == "uses" && value.Kind == yaml.ScalarNode {
				if err := validateWorkflowActionReference(value.Value); err != nil {
					return fmt.Errorf("workflow %s line %d uses %q: %w", workflow, value.Line, value.Value, err)
				}
			}
		}
	}
	for _, child := range node.Content {
		if err := validateWorkflowNodeActionPins(child, workflow); err != nil {
			return err
		}
	}
	return nil
}

func validateWorkflowActionReference(reference string) error {
	reference = strings.TrimSpace(reference)
	if strings.HasPrefix(reference, "./") {
		return nil
	}

	separator := strings.LastIndexByte(reference, '@')
	if separator <= 0 || separator == len(reference)-1 {
		return errors.New("external action must end in @ followed by a full commit SHA")
	}
	commit := reference[separator+1:]
	if len(commit) != 40 {
		return errors.New("external action must use a 40-character commit SHA")
	}
	for _, char := range commit {
		if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
			return errors.New("external action commit must be hexadecimal")
		}
	}
	return nil
}

func discoverSourcePackages(root, module string) (map[string]bool, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve module root: %w", err)
	}
	actual := make(map[string]bool)
	err = filepath.WalkDir(root, func(filename string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if rel == "." {
				return nil
			}
			name := entry.Name()
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "vendor" {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(filename, "go.mod")); err == nil {
				return filepath.SkipDir
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("inspect nested module %s: %w", filename, err)
			}
			return nil
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			return nil
		}
		directory := filepath.ToSlash(filepath.Dir(rel))
		packageName := module
		if directory != "." {
			packageName += "/" + directory
		}
		actual[packageName] = true
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover non-test Go source: %w", err)
	}
	return actual, nil
}

func listReleaseDependencies(releaseTags string) (map[string]bool, error) {
	return listPackageDependencies(releaseTags, "./cmd/akt")
}

func listPackageDependencies(releaseTags, packagePattern string) (map[string]bool, error) {
	canonicalTags, err := canonicalBuildTags(releaseTags)
	if err != nil {
		return nil, fmt.Errorf("release tags: %w", err)
	}
	cmd := exec.Command("go", "list", "-deps", "-tags="+canonicalTags, packagePattern)
	cmd.Env = replaceEnv(os.Environ(), "GOWORK", "off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go list dependencies for %s: %s", packagePattern, strings.TrimSpace(string(out)))
	}
	dependencies := make(map[string]bool)
	for _, name := range strings.Fields(string(out)) {
		dependencies[name] = true
	}
	return dependencies, nil
}

func listRepositoryImports(releaseTags string) (map[string][]string, error) {
	canonicalTags, err := canonicalBuildTags(releaseTags)
	if err != nil {
		return nil, fmt.Errorf("release tags: %w", err)
	}
	command := exec.Command("go", "list", "-tags="+canonicalTags, "-f={{.ImportPath}}\t{{join .Imports \" \"}}", "./...")
	command.Env = replaceEnv(os.Environ(), "GOWORK", "off")
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go list repository imports: %s", strings.TrimSpace(string(output)))
	}
	result := make(map[string][]string)
	for _, line := range strings.Split(strings.TrimRight(string(output), "\r\n"), "\n") {
		name, imports, found := strings.Cut(line, "\t")
		if !found || name == "" {
			return nil, fmt.Errorf("malformed go list import row %q", line)
		}
		result[name] = strings.Fields(imports)
	}
	return result, nil
}

func listReleaseDependencyLocations(releaseTags string) ([]packageLocation, error) {
	canonicalTags, err := canonicalBuildTags(releaseTags)
	if err != nil {
		return nil, fmt.Errorf("release tags: %w", err)
	}
	command := exec.Command("go", "list", "-deps", "-json", "-tags="+canonicalTags, "./cmd/akt")
	command.Env = replaceEnv(os.Environ(), "GOWORK", "off")
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("go list release dependency locations: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	var result []packageLocation
	for {
		var listed packageLocation
		if err := decoder.Decode(&listed); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("decode release dependency locations: %w", err)
		}
		result = append(result, listed)
	}
	return result, nil
}

func validateLocalReleaseDependencies(root string, packages packageSet, locations []packageLocation) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve repository root symlinks: %w", err)
	}
	var problems []string
	for _, location := range locations {
		if location.Dir == "" {
			continue
		}
		directory, err := filepath.EvalSymlinks(location.Dir)
		if err != nil {
			return fmt.Errorf("resolve release dependency %s directory: %w", location.ImportPath, err)
		}
		relative, err := filepath.Rel(root, directory)
		if err != nil {
			return fmt.Errorf("compare release dependency %s directory: %w", location.ImportPath, err)
		}
		if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		if _, classified := packages.ByName[location.ImportPath]; !classified {
			problems = append(problems, fmt.Sprintf(
				"repository-local release dependency is unclassified: %s (%s)",
				location.ImportPath,
				filepath.ToSlash(relative),
			))
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n"))
	}
	return nil
}

func validateActiveDependencyClosure(packages packageSet, root string, dependencies map[string]bool) error {
	entry, ok := packages.ByName[root]
	if !ok || entry.Class != classActive {
		return fmt.Errorf("active dependency root %s is not classified active", root)
	}
	var problems []string
	for name := range dependencies {
		dependency, classified := packages.ByName[name]
		if !classified || dependency.Class == classActive {
			continue
		}
		problems = append(problems, fmt.Sprintf(
			"active package %s depends on %s package %s",
			root,
			dependency.Class,
			name,
		))
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n"))
	}
	return nil
}

func validateExperimentalImportBoundary(packages packageSet, imports map[string][]string) error {
	allowedSource := packages.Module + "/internal/cli"
	allowedTarget := packages.Module + "/internal/tui"
	var problems []string
	for source, dependencies := range imports {
		sourceEntry, classified := packages.ByName[source]
		if !classified || sourceEntry.Class != classActive {
			continue
		}
		for _, dependency := range dependencies {
			targetEntry, classified := packages.ByName[dependency]
			if !classified || targetEntry.Class != classExperimentalTUI {
				continue
			}
			if source == allowedSource && dependency == allowedTarget {
				continue
			}
			problems = append(problems, fmt.Sprintf(
				"active package %s imports experimental package %s",
				source,
				dependency,
			))
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n"))
	}
	return nil
}

func validateTaxonomy(
	packages packageSet,
	exceptions map[string]exception,
	actual map[string]bool,
	releaseDependencies map[string]bool,
) error {
	var problems []string
	for name := range actual {
		if _, ok := packages.ByName[name]; !ok {
			problems = append(problems, "unclassified package: "+name)
		}
	}
	for name := range packages.ByName {
		if !actual[name] {
			problems = append(problems, "stale package entry: "+name)
		}
	}
	for _, entry := range packages.Entries {
		relative := strings.TrimPrefix(entry.Name, packages.Module+"/")
		switch {
		case entry.Class == classTooling && relative != "tools" && !strings.HasPrefix(relative, "tools/"):
			problems = append(problems, "tooling package is outside tools: "+entry.Name)
		case entry.Class == classExperimentalTUI && relative != "internal/tui" && !strings.HasPrefix(relative, "internal/tui/"):
			problems = append(problems, "experimental-tui package is outside internal/tui: "+entry.Name)
		case (relative == "tools" || strings.HasPrefix(relative, "tools/")) && entry.Class != classTooling:
			problems = append(problems, "package under tools must use tooling class: "+entry.Name)
		}
		if entry.Class == classTooling && releaseDependencies[entry.Name] {
			problems = append(problems, "tooling package is linked into the release binary: "+entry.Name)
		}
		if (entry.Class == classActive || entry.Class == classExperimentalTUI) && !releaseDependencies[entry.Name] {
			problems = append(problems, entry.Class+" package is not linked into the release binary: "+entry.Name)
		}
		if entry.Class != classSupport {
			continue
		}
		found := false
		for _, reviewed := range exceptions {
			if reviewed.Package == entry.Name && reviewed.Line == 0 {
				found = true
				break
			}
		}
		if !found {
			problems = append(problems, "support package lacks a reviewed package exception: "+entry.Name)
		}
		if releaseDependencies[entry.Name] {
			problems = append(problems, "support package is linked into the release binary: "+entry.Name)
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n"))
	}
	return nil
}

func validateGoreleaserReleaseTags(filename, releaseTags string) error {
	contents, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read goreleaser configuration: %w", err)
	}
	var configuration struct {
		Builds []struct {
			ID    string   `yaml:"id"`
			Main  string   `yaml:"main"`
			Flags []string `yaml:"flags"`
		} `yaml:"builds"`
	}
	if err := yaml.Unmarshal(contents, &configuration); err != nil {
		return fmt.Errorf("parse goreleaser configuration: %w", err)
	}
	if len(configuration.Builds) == 0 {
		return errors.New("goreleaser configuration has no builds")
	}
	want, err := canonicalBuildTags(releaseTags)
	if err != nil {
		return fmt.Errorf("release tags: %w", err)
	}
	for index, build := range configuration.Builds {
		label := build.ID
		if label == "" {
			label = strconv.Itoa(index + 1)
		}
		if path.Clean(filepath.ToSlash(strings.TrimSpace(build.Main))) != "cmd/akt" {
			return fmt.Errorf("goreleaser build %q main package must be ./cmd/akt, got %q", label, build.Main)
		}
		var tagValues []string
		for flagIndex := 0; flagIndex < len(build.Flags); flagIndex++ {
			value := strings.TrimSpace(build.Flags[flagIndex])
			switch {
			case value == "-tags":
				if flagIndex+1 >= len(build.Flags) {
					return fmt.Errorf("goreleaser build %q has -tags without a value", label)
				}
				flagIndex++
				tagValues = append(tagValues, strings.TrimSpace(build.Flags[flagIndex]))
			case strings.HasPrefix(value, "-tags="):
				tagValues = append(tagValues, strings.TrimPrefix(value, "-tags="))
			}
		}
		if len(tagValues) != 1 {
			return fmt.Errorf("goreleaser build %q must have exactly one -tags flag, found %d", label, len(tagValues))
		}
		got, err := canonicalBuildTags(tagValues[0])
		if err != nil {
			return fmt.Errorf("goreleaser build %q -tags value: %w", label, err)
		}
		if got != want {
			return fmt.Errorf("goreleaser build %q -tags %q do not match -release-tags %q", label, got, want)
		}
	}
	return nil
}

func canonicalBuildTags(value string) (string, error) {
	tags := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	if len(tags) == 0 {
		return "", errors.New("build tag list is empty")
	}
	seen := make(map[string]bool)
	for _, tag := range tags {
		if strings.HasPrefix(tag, "-") || strings.ContainsAny(tag, "'\"") {
			return "", fmt.Errorf("invalid build tag %q", tag)
		}
		seen[tag] = true
	}
	tags = tags[:0]
	for tag := range seen {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return strings.Join(tags, ","), nil
}

func runFilter(args []string) error {
	fs := flag.NewFlagSet("filter", flag.ContinueOnError)
	profileFile := fs.String("profile", "", "input Go coverage profile")
	packagesFile := fs.String("packages", "coverage/packages.tsv", "package taxonomy")
	class := fs.String("class", "repository", "repository, active, experimental-tui, or tooling")
	outFile := fs.String("out", "", "output Go coverage profile")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *profileFile == "" || *outFile == "" {
		return errors.New("filter requires -profile and -out")
	}
	packages, profile, err := loadInputs(*packagesFile, *profileFile)
	if err != nil {
		return err
	}
	if err := validateClass(*class); err != nil {
		return err
	}

	selected := coverProfile{Mode: profile.Mode}
	for _, block := range profile.Blocks {
		entry, ok := packageForProfileFile(packages, block.File)
		if !ok {
			return fmt.Errorf("profile file %q does not belong to a classified package", block.File)
		}
		if classIncludes(*class, entry.Class) {
			selected.Blocks = append(selected.Blocks, block)
		}
	}
	return writeProfile(*outFile, selected)
}

func runReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	profileFile := fs.String("profile", "", "input Go coverage profile")
	packagesFile := fs.String("packages", "coverage/packages.tsv", "package taxonomy")
	class := fs.String("class", "repository", "repository, active, experimental-tui, or tooling")
	outFile := fs.String("out", "", "TSV report path; stdout when empty")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *profileFile == "" {
		return errors.New("report requires -profile")
	}
	packages, profile, err := loadInputs(*packagesFile, *profileFile)
	if err != nil {
		return err
	}
	if err := validateClass(*class); err != nil {
		return err
	}
	counts, err := countStatements(packages, profile, *class)
	if err != nil {
		return err
	}
	return writeCounts(*outFile, packages, counts, *class, true)
}

func runBaseline(args []string) error {
	fs := flag.NewFlagSet("baseline", flag.ContinueOnError)
	profileFile := fs.String("profile", "", "input Go coverage profile")
	packagesFile := fs.String("packages", "coverage/packages.tsv", "package taxonomy")
	class := fs.String("class", "active", "active, experimental-tui, or tooling")
	outFile := fs.String("out", "", "baseline path; stdout when empty")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *profileFile == "" {
		return errors.New("baseline requires -profile")
	}
	packages, profile, err := loadInputs(*packagesFile, *profileFile)
	if err != nil {
		return err
	}
	if *class != classActive && *class != classExperimentalTUI && *class != classTooling {
		return errors.New("baseline class must be active, experimental-tui, or tooling")
	}
	counts, err := countStatements(packages, profile, *class)
	if err != nil {
		return err
	}
	return writeCounts(*outFile, packages, counts, *class, false)
}

func runRatchet(args []string) error {
	fs := flag.NewFlagSet("ratchet", flag.ContinueOnError)
	profileFile := fs.String("profile", "", "input Go coverage profile")
	packagesFile := fs.String("packages", "coverage/packages.tsv", "package taxonomy")
	class := fs.String("class", "active", "active, experimental-tui, or tooling")
	baselineFile := fs.String("baseline", "", "checked-in baseline TSV")
	base := fs.String("base", "", "base Git revision used to prevent baseline weakening")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *profileFile == "" || *baselineFile == "" {
		return errors.New("ratchet requires -profile and -baseline")
	}
	packages, profile, err := loadInputs(*packagesFile, *profileFile)
	if err != nil {
		return err
	}
	if *class != classActive && *class != classExperimentalTUI && *class != classTooling {
		return errors.New("ratchet class must be active, experimental-tui, or tooling")
	}
	canonicalBaseline, err := requireCanonicalBaselinePath(*class, *baselineFile)
	if err != nil {
		return err
	}
	current, err := countStatements(packages, profile, *class)
	if err != nil {
		return err
	}
	baseline, err := loadBaseline(canonicalBaseline)
	if err != nil {
		return err
	}
	if *base != "" {
		if strings.HasPrefix(*base, "-") {
			return errors.New("base Git revision may not start with '-'")
		}
		resolvedBase, err := resolveGitCommit(*base)
		if err != nil {
			return err
		}
		prior, exists, err := loadBaselineAtRevision(resolvedBase, canonicalBaseline)
		if err != nil {
			return err
		}
		if exists {
			migrationPrior, err := loadMigratedPackageBaselinesAtRevision(resolvedBase, canonicalBaseline)
			if err != nil {
				return err
			}
			if err := guardBaseline(packages, current, baseline, prior, migrationPrior, *class); err != nil {
				return err
			}
		} else {
			legacyPackages, err := sourcePackagesAtRevision(resolvedBase, packages.Module)
			if err != nil {
				return err
			}
			if err := guardInitialBaseline(packages, current, legacyPackages, *class); err != nil {
				return err
			}
			fmt.Printf("%s coverage ratchet: base has no baseline (one-time bootstrap)\n", *class)
		}
	}

	gotTotal := current["@total"]
	failures := compareRatchet(packages, current, baseline, *class)
	sort.Strings(failures)
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "\n"))
	}
	fmt.Printf("%s coverage ratchet: %s, no package regressed\n", *class, percent(gotTotal))
	return nil
}

func guardInitialBaseline(packages packageSet, current map[string]statementCount, legacyPackages map[string]bool, class string) error {
	var failures []string
	for _, entry := range packages.Entries {
		if !classIncludes(class, entry.Class) {
			continue
		}
		if legacyPackages[entry.Name] {
			continue
		}
		minimum, enforced := packageEntryFloor(class, entry)
		if !enforced {
			continue
		}
		got := current[entry.Name]
		if belowPercentFloor(got, minimum) {
			failures = append(failures, fmt.Sprintf("initial package %s is %s; minimum is %d%%", entry.Name, percent(got), minimum))
		}
	}
	sort.Strings(failures)
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "\n"))
	}
	return nil
}

func runPatch(args []string) error {
	fs := flag.NewFlagSet("patch", flag.ContinueOnError)
	profileFile := fs.String("profile", "", "active union Go coverage profile")
	edgeProfileFile := fs.String("edge-profile", "", "raw union Go coverage profile containing synthetic edge evidence")
	packagesFile := fs.String("packages", "coverage/packages.tsv", "package taxonomy")
	exceptionsFile := fs.String("exceptions", "coverage/exceptions.tsv", "reviewed exceptions TSV")
	base := fs.String("base", "", "base Git revision")
	head := fs.String("head", "WORKTREE", "head Git revision or WORKTREE")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *profileFile == "" || *base == "" || *head == "" {
		return errors.New("patch requires -profile, -base, and -head")
	}
	if strings.HasPrefix(*base, "-") || strings.HasPrefix(*head, "-") {
		return errors.New("git revisions may not start with '-'")
	}
	packages, profile, err := loadInputs(*packagesFile, *profileFile)
	if err != nil {
		return err
	}
	if *edgeProfileFile != "" && *edgeProfileFile != *profileFile {
		edgeProfile, err := loadProfile(*edgeProfileFile)
		if err != nil {
			return fmt.Errorf("load edge profile: %w", err)
		}
		if err := validateProfileStatementRanges(packages, edgeProfile); err != nil {
			return fmt.Errorf("validate edge profile: %w", err)
		}
		profile.EdgeBlocks = edgeProfile.EdgeBlocks
	}
	exceptions, err := loadExceptions(*exceptionsFile, packages)
	if err != nil {
		return err
	}
	if err := validateActiveProfile(packages, profile); err != nil {
		return err
	}
	changed, err := changedGoLines(*base, *head)
	if err != nil {
		return err
	}

	uncovered, checked, exempted, err := uncoveredChangedActiveLines(packages, profile, exceptions, changed, ".")
	if err != nil {
		return err
	}
	if len(uncovered) > 0 {
		return fmt.Errorf("changed active executable lines are not 100%% covered:\n%s", strings.Join(uncovered, "\n"))
	}
	fmt.Printf("active patch coverage: 100%% (%d executable lines, %d reviewed exceptions)\n", checked, exempted)
	return nil
}

func validateActiveProfile(packages packageSet, profile coverProfile) error {
	activeCounts, err := countStatements(packages, profile, classActive)
	if err != nil {
		return err
	}
	if activeCounts["@total"].Total == 0 {
		return errors.New("active coverage profile contains no executable statements")
	}
	var missing []string
	for _, entry := range packages.Entries {
		if entry.Class == classActive && activeCounts[entry.Name].Total == 0 {
			missing = append(missing, entry.Name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("active coverage profile contains no statements for classified packages:\n%s", strings.Join(missing, "\n"))
	}
	return nil
}

func uncoveredChangedActiveLines(
	packages packageSet,
	profile coverProfile,
	exceptions map[string]exception,
	changed map[string]map[int]bool,
	sourceRoot string,
) ([]string, int, int, error) {
	blocksByFile := make(map[string][]coverBlock)
	for _, block := range profile.Blocks {
		entry, ok := packageForProfileFile(packages, block.File)
		if !ok || entry.Class != classActive {
			continue
		}
		rel, err := profileFileRelative(packages.Module, block.File)
		if err != nil {
			return nil, 0, 0, err
		}
		blocksByFile[rel] = append(blocksByFile[rel], block)
	}
	edgeBlocksByFile := make(map[string][]coverBlock)
	for _, block := range profile.EdgeBlocks {
		entry, ok := packageForProfileFile(packages, block.File)
		if !ok || entry.Class != classActive {
			continue
		}
		rel, err := profileFileRelative(packages.Module, block.File)
		if err != nil {
			return nil, 0, 0, err
		}
		edgeBlocksByFile[rel] = append(edgeBlocksByFile[rel], block)
	}

	checked := 0
	exempted := 0
	var uncovered []string
	for file, lines := range changed {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		pkgName := packages.Module
		if directory := path.Dir(file); directory != "." {
			pkgName += "/" + directory
		}
		entry, ok := packages.ByName[pkgName]
		if !ok || entry.Class != classActive {
			continue
		}
		blocks := blocksByFile[file]
		if len(blocks) == 0 {
			hasStatements, err := fileHasExecutableStatements(sourceRoot, file)
			if err != nil {
				return nil, 0, 0, err
			}
			if hasStatements {
				uncovered = append(uncovered, fmt.Sprintf(
					"%s (%s: active source contains executable statements but is absent from the coverage profile)",
					file, pkgName,
				))
			}
			continue
		}
		executableLines, executableRegions, err := fileExecutableSyntax(sourceRoot, file)
		if err != nil {
			return nil, 0, 0, err
		}
		for line := range lines {
			intersections := intersectingBlocks(blocks, line)
			edgeIntersections, syntheticEdge := exactSyntheticEdgeBlocks(
				executableRegions,
				edgeBlocksByFile[file],
				line,
			)
			if syntheticEdge {
				intersections = edgeIntersections
			} else if len(intersections) == 0 {
				if !executableLines[line] {
					continue
				}
				intersections = enclosingRegionBlocks(executableRegions, blocks, line)
			}
			if len(intersections) == 0 {
				checked++
				if _, ok := exceptions[exceptionKey(pkgName, file, line)]; ok {
					exempted++
					continue
				}
				uncovered = append(uncovered, fmt.Sprintf(
					"%s:%d (%s: no coverage block intersects executable syntax)",
					file, line, pkgName,
				))
				continue
			}
			checked++
			if _, ok := exceptions[exceptionKey(pkgName, file, line)]; ok {
				exempted++
				continue
			}
			for _, block := range intersections {
				if block.Count == 0 {
					uncovered = append(uncovered, fmt.Sprintf("%s:%d (%s)", file, line, pkgName))
					break
				}
			}
		}
	}
	sort.Strings(uncovered)
	return uncovered, checked, exempted, nil
}

func fileExecutableLines(root, filename string) (map[int]bool, error) {
	lines, _, err := fileExecutableSyntax(root, filename)
	return lines, err
}

func fileExecutableSyntax(root, filename string) (map[int]bool, []sourceRegion, error) {
	clean := path.Clean(filename)
	if clean != filename || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return nil, nil, fmt.Errorf("changed file has unsafe path %q", filename)
	}
	sourcePath := filepath.Join(root, filepath.FromSlash(clean))
	info, err := os.Lstat(sourcePath)
	if err != nil {
		return nil, nil, fmt.Errorf("inspect changed active source %s: %w", filename, err)
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("changed active source %s is not a regular file", filename)
	}
	contents, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, nil, fmt.Errorf("read changed active source %s: %w", filename, err)
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(
		fileSet,
		filepath.Join(root, filepath.FromSlash(clean)),
		contents,
		parser.SkipObjectResolution,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("parse changed active source %s: %w", filename, err)
	}
	lines := make(map[int]bool)
	regions := make([]sourceRegion, 0)
	mark := func(node ast.Node, syntheticEdge bool) {
		start := fileSet.Position(node.Pos())
		end := fileSet.Position(node.End())
		regions = append(regions, sourceRegion{
			StartLine: start.Line, StartColumn: start.Column,
			EndLine: end.Line, EndColumn: end.Column,
			SyntheticEdge: syntheticEdge,
		})
		for line := start.Line; line <= end.Line; line++ {
			lines[line] = true
		}
	}
	ast.Inspect(parsed, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.BlockStmt, *ast.EmptyStmt:
			return true
		case *ast.CaseClause:
			mark(typed, len(typed.Body) == 0)
		case *ast.CommClause:
			mark(typed, len(typed.Body) == 0)
		case ast.Stmt:
			mark(typed, false)
		}
		return true
	})
	for _, declaration := range parsed.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.VAR {
			continue
		}
		for _, spec := range general.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, value := range valueSpec.Values {
				mark(value, false)
			}
		}
	}
	tokenLines := sourceTokenLines(clean, contents)
	for line := range lines {
		if !lineHasExecutableToken(tokenLines[line]) {
			delete(lines, line)
		}
	}
	return lines, regions, nil
}

func exactSyntheticEdgeBlocks(regions []sourceRegion, blocks []coverBlock, line int) ([]coverBlock, bool) {
	var result []coverBlock
	syntheticEdge := false
	for _, region := range regions {
		if !region.SyntheticEdge || region.EndLine != line {
			continue
		}
		syntheticEdge = true
		for _, block := range blocks {
			if block.StartLine == region.EndLine && block.StartColumn == region.EndColumn &&
				block.EndLine == region.EndLine && block.EndColumn == region.EndColumn {
				result = append(result, block)
			}
		}
	}
	return result, syntheticEdge
}

func enclosingRegionBlocks(regions []sourceRegion, blocks []coverBlock, line int) []coverBlock {
	var selected *sourceRegion
	for index := range regions {
		region := &regions[index]
		if line < region.StartLine || line > region.EndLine {
			continue
		}
		if selected == nil || region.EndLine-region.StartLine < selected.EndLine-selected.StartLine ||
			(region.EndLine-region.StartLine == selected.EndLine-selected.StartLine && region.StartLine > selected.StartLine) {
			selected = region
		}
	}
	if selected == nil {
		return nil
	}
	for candidateLine := selected.StartLine; candidateLine <= selected.EndLine; candidateLine++ {
		if intersections := intersectingBlocks(blocks, candidateLine); len(intersections) > 0 {
			return intersections
		}
	}
	return nil
}

func sourceTokenLines(filename string, contents []byte) map[int][]token.Token {
	fileSet := token.NewFileSet()
	file := fileSet.AddFile(filename, fileSet.Base(), len(contents))
	var scanner goscanner.Scanner
	scanner.Init(file, contents, nil, 0)
	result := make(map[int][]token.Token)
	for {
		position, tok, _ := scanner.Scan()
		if tok == token.EOF {
			return result
		}
		line := fileSet.Position(position).Line
		result[line] = append(result[line], tok)
	}
}

func lineHasExecutableToken(tokens []token.Token) bool {
	for _, tok := range tokens {
		switch tok {
		case token.LBRACE, token.RBRACE, token.RPAREN, token.RBRACK, token.COMMA, token.SEMICOLON:
			continue
		default:
			return true
		}
	}
	return false
}

func fileHasExecutableStatements(root, filename string) (bool, error) {
	lines, err := fileExecutableLines(root, filename)
	if err != nil {
		return false, err
	}
	return len(lines) > 0, nil
}

func loadInputs(packagesFile, profileFile string) (packageSet, coverProfile, error) {
	packages, err := loadPackages(packagesFile)
	if err != nil {
		return packageSet{}, coverProfile{}, err
	}
	profile, err := loadProfile(profileFile)
	if err != nil {
		return packageSet{}, coverProfile{}, err
	}
	if err := validateProfileStatementRanges(packages, profile); err != nil {
		return packageSet{}, coverProfile{}, err
	}
	return packages, profile, nil
}

func validateProfileStatementRanges(packages packageSet, profile coverProfile) error {
	seen := make(map[string]string)
	for _, blocks := range [][]coverBlock{profile.Blocks, profile.EdgeBlocks} {
		for _, block := range blocks {
			relative, err := profileFileRelative(packages.Module, block.File)
			if err != nil {
				return err
			}
			key := fmt.Sprintf("%s:%d.%d,%d.%d", relative, block.StartLine, block.StartColumn, block.EndLine, block.EndColumn)
			if prior, duplicate := seen[key]; duplicate {
				return fmt.Errorf(
					"profile contains duplicate statement range after path normalization %s (from %q and %q)",
					key,
					prior,
					block.File,
				)
			}
			seen[key] = block.File
		}
	}
	return nil
}

func loadPackages(filename string) (packageSet, error) {
	file, err := os.Open(filename)
	if err != nil {
		return packageSet{}, fmt.Errorf("open package taxonomy: %w", err)
	}
	defer func() { _ = file.Close() }()
	records, err := readTSV(file)
	if err != nil {
		return packageSet{}, fmt.Errorf("read package taxonomy: %w", err)
	}
	if len(records) == 0 || !equalStrings(records[0], []string{"package", "class", "critical"}) {
		return packageSet{}, errors.New("package taxonomy header must be: package, class, critical")
	}

	result := packageSet{ByName: make(map[string]packageEntry)}
	last := ""
	for row, record := range records[1:] {
		if len(record) != 3 {
			return packageSet{}, fmt.Errorf("package taxonomy row %d: expected 3 fields", row+2)
		}
		if record[0] <= last {
			return packageSet{}, fmt.Errorf("package taxonomy must be sorted and unique near %q", record[0])
		}
		last = record[0]
		if record[1] != classActive && record[1] != classExperimentalTUI && record[1] != classTooling && record[1] != classSupport {
			return packageSet{}, fmt.Errorf("package taxonomy row %d: invalid class %q", row+2, record[1])
		}
		critical, err := strconv.ParseBool(record[2])
		if err != nil {
			return packageSet{}, fmt.Errorf("package taxonomy row %d: critical must be true or false", row+2)
		}
		entry := packageEntry{Name: record[0], Class: record[1], Critical: critical}
		result.Entries = append(result.Entries, entry)
		result.ByName[entry.Name] = entry
	}
	if len(result.Entries) == 0 {
		return packageSet{}, errors.New("package taxonomy is empty")
	}
	module, err := moduleName()
	if err != nil {
		return packageSet{}, err
	}
	result.Module = module
	for _, entry := range result.Entries {
		if entry.Name != module && !strings.HasPrefix(entry.Name, module+"/") {
			return packageSet{}, fmt.Errorf("package %q is outside module %q", entry.Name, module)
		}
	}
	return result, nil
}

func moduleName() (string, error) {
	cmd := exec.Command("go", "list", "-m")
	cmd.Env = replaceEnv(os.Environ(), "GOWORK", "off")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go list -m: %w", err)
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", errors.New("go list -m returned an empty module name")
	}
	return name, nil
}

func moduleRoot() (string, error) {
	cmd := exec.Command("go", "list", "-m", "-f={{.Dir}}")
	cmd.Env = replaceEnv(os.Environ(), "GOWORK", "off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go list module root: %s", strings.TrimSpace(string(out)))
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", errors.New("go list returned an empty module root")
	}
	return root, nil
}

func loadProfile(filename string) (coverProfile, error) {
	file, err := os.Open(filename)
	if err != nil {
		return coverProfile{}, fmt.Errorf("open profile: %w", err)
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	if !scanner.Scan() {
		return coverProfile{}, errors.New("coverage profile is empty")
	}
	header := strings.TrimSpace(scanner.Text())
	if !strings.HasPrefix(header, "mode: ") {
		return coverProfile{}, fmt.Errorf("invalid coverage profile header %q", header)
	}
	profile := coverProfile{Mode: strings.TrimSpace(strings.TrimPrefix(header, "mode: "))}
	if profile.Mode != "atomic" {
		return coverProfile{}, fmt.Errorf("coverage mode %q is not merge-safe atomic mode", profile.Mode)
	}
	seen := make(map[string]bool)
	coverLineRE := regexp.MustCompile(`^(.+):(\d+)\.(\d+),(\d+)\.(\d+) (\d+) (\d+)$`)
	line := 1
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		matches := coverLineRE.FindStringSubmatch(raw)
		if matches == nil {
			return coverProfile{}, fmt.Errorf("profile line %d is malformed", line)
		}
		values := make([]int64, 5)
		for index := 0; index < 5; index++ {
			value, err := strconv.ParseInt(matches[index+2], 10, 64)
			if err != nil {
				return coverProfile{}, fmt.Errorf("profile line %d: %w", line, err)
			}
			values[index] = value
		}
		count, err := strconv.ParseUint(matches[7], 10, 64)
		if err != nil {
			return coverProfile{}, fmt.Errorf("profile line %d: %w", line, err)
		}
		block := coverBlock{
			File: matches[1], StartLine: int(values[0]), StartColumn: int(values[1]),
			EndLine: int(values[2]), EndColumn: int(values[3]), Statements: values[4], Count: count, Raw: raw,
		}
		if block.StartLine < 1 || block.StartColumn < 1 || block.EndLine < 1 || block.EndColumn < 1 ||
			block.EndLine < block.StartLine ||
			(block.EndLine == block.StartLine && block.EndColumn < block.StartColumn) ||
			(block.Statements > 0 && block.EndLine == block.StartLine && block.EndColumn == block.StartColumn) {
			return coverProfile{}, fmt.Errorf("profile line %d has invalid statement range %d.%d,%d.%d", line, block.StartLine, block.StartColumn, block.EndLine, block.EndColumn)
		}
		key := fmt.Sprintf("%s:%d.%d,%d.%d", block.File, block.StartLine, block.StartColumn, block.EndLine, block.EndColumn)
		if seen[key] {
			return coverProfile{}, fmt.Errorf("profile contains duplicate statement range %s", key)
		}
		seen[key] = true
		if block.Statements == 0 {
			profile.EdgeBlocks = append(profile.EdgeBlocks, block)
			continue
		}
		profile.Blocks = append(profile.Blocks, block)
	}
	if err := scanner.Err(); err != nil {
		return coverProfile{}, fmt.Errorf("read profile: %w", err)
	}
	return profile, nil
}

func writeProfile(filename string, profile coverProfile) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("create profile: %w", err)
	}
	writer := bufio.NewWriter(file)
	if _, err := fmt.Fprintf(writer, "mode: %s\n", profile.Mode); err != nil {
		return err
	}
	for _, block := range profile.Blocks {
		if _, err := fmt.Fprintln(writer, block.Raw); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close profile: %w", err)
	}
	return nil
}

func countStatements(packages packageSet, profile coverProfile, class string) (map[string]statementCount, error) {
	counts := make(map[string]statementCount)
	for _, entry := range packages.Entries {
		if classIncludes(class, entry.Class) {
			counts[entry.Name] = statementCount{}
		}
	}
	total := statementCount{}
	for _, block := range profile.Blocks {
		entry, ok := packageForProfileFile(packages, block.File)
		if !ok {
			return nil, fmt.Errorf("profile file %q does not belong to a classified package", block.File)
		}
		if !classIncludes(class, entry.Class) {
			continue
		}
		count := counts[entry.Name]
		if block.Statements < 0 {
			return nil, fmt.Errorf("profile file %q has a negative statement count", block.File)
		}
		if block.Statements > math.MaxInt64-count.Total || block.Statements > math.MaxInt64-total.Total {
			return nil, fmt.Errorf("statement count overflow while accumulating profile file %q", block.File)
		}
		count.Total += block.Statements
		total.Total += block.Statements
		if block.Count > 0 {
			count.Covered += block.Statements
			total.Covered += block.Statements
		}
		counts[entry.Name] = count
	}
	counts["@total"] = total
	return counts, nil
}

func writeCounts(filename string, packages packageSet, counts map[string]statementCount, class string, withPercent bool) error {
	var output strings.Builder
	if withPercent {
		fmt.Fprintln(&output, "package\tcovered\ttotal\tpercent")
	} else {
		fmt.Fprintln(&output, "package\tcovered\ttotal")
	}
	for _, entry := range packages.Entries {
		if !classIncludes(class, entry.Class) {
			continue
		}
		count := counts[entry.Name]
		writeCount(&output, entry.Name, count, withPercent)
	}
	writeCount(&output, "@total", counts["@total"], withPercent)
	if filename == "" {
		fmt.Print(output.String())
		return nil
	}
	if err := os.WriteFile(filename, []byte(output.String()), 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

func writeCount(writer *strings.Builder, name string, count statementCount, withPercent bool) {
	if withPercent {
		fmt.Fprintf(writer, "%s\t%d\t%d\t%s\n", name, count.Covered, count.Total, percent(count))
		return
	}
	fmt.Fprintf(writer, "%s\t%d\t%d\n", name, count.Covered, count.Total)
}

func loadBaseline(filename string) (map[string]statementCount, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open baseline: %w", err)
	}
	defer func() { _ = file.Close() }()
	return parseBaseline(file)
}

func requireCanonicalBaselinePath(class, filename string) (string, error) {
	var canonical string
	switch class {
	case classActive:
		canonical = activeBaselinePath
	case classExperimentalTUI:
		canonical = experimentalTUIBaselinePath
	case classTooling:
		canonical = toolingBaselinePath
	default:
		return "", fmt.Errorf("unknown baseline class %q", class)
	}

	clean := filepath.ToSlash(filepath.Clean(filename))
	if clean != canonical {
		return "", fmt.Errorf("%s ratchet baseline must use canonical path %q, got %q", class, canonical, filename)
	}
	return canonical, nil
}

func parseBaseline(reader io.Reader) (map[string]statementCount, error) {
	records, err := readTSV(reader)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 || !equalStrings(records[0], []string{"package", "covered", "total"}) {
		return nil, errors.New("baseline header must be: package, covered, total")
	}
	result := make(map[string]statementCount)
	for row, record := range records[1:] {
		if len(record) != 3 {
			return nil, fmt.Errorf("baseline row %d: expected 3 fields", row+2)
		}
		covered, err := strconv.ParseInt(record[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("baseline row %d covered: %w", row+2, err)
		}
		total, err := strconv.ParseInt(record[2], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("baseline row %d total: %w", row+2, err)
		}
		if covered < 0 || total < 0 || covered > total {
			return nil, fmt.Errorf("baseline row %d has impossible counts", row+2)
		}
		if _, exists := result[record[0]]; exists {
			return nil, fmt.Errorf("baseline has duplicate package %q", record[0])
		}
		result[record[0]] = statementCount{Covered: covered, Total: total}
	}
	return result, nil
}

func loadBaselineAtRevision(revision, filename string) (map[string]statementCount, bool, error) {
	resolvedRevision, err := resolveGitCommit(revision)
	if err != nil {
		return nil, false, err
	}
	cleanFilename := path.Clean(filename)
	listed := exec.Command("git", "ls-tree", "-z", "--full-tree", resolvedRevision, "--", cleanFilename)
	listing, err := listed.CombinedOutput()
	if err != nil {
		return nil, false, fmt.Errorf(
			"inspect baseline %s at revision %q: %s",
			cleanFilename,
			resolvedRevision,
			strings.TrimSpace(string(listing)),
		)
	}
	if len(listing) == 0 {
		return nil, false, nil
	}
	object := resolvedRevision + ":" + cleanFilename
	show := exec.Command("git", "show", object)
	output, err := show.CombinedOutput()
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %s", object, strings.TrimSpace(string(output)))
	}
	baseline, err := parseBaseline(strings.NewReader(string(output)))
	if err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", object, err)
	}
	return baseline, true, nil
}

func resolveGitCommit(revision string) (string, error) {
	verify := exec.Command("git", "rev-parse", "--verify", revision+"^{commit}")
	output, err := verify.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("verify base revision %q: %s", revision, strings.TrimSpace(string(output)))
	}
	resolved := strings.TrimSpace(string(output))
	valid, err := regexp.MatchString(`^[0-9a-f]{40}([0-9a-f]{24})?$`, resolved)
	if err != nil {
		return "", err
	}
	if !valid {
		return "", fmt.Errorf("verify base revision %q returned malformed object ID %q", revision, resolved)
	}
	return resolved, nil
}

func loadMigratedPackageBaselinesAtRevision(revision, currentBaseline string) (map[string]statementCount, error) {
	result := make(map[string]statementCount)
	for _, filename := range []string{activeBaselinePath, experimentalTUIBaselinePath, toolingBaselinePath} {
		if filename == currentBaseline {
			continue
		}
		baseline, exists, err := loadBaselineAtRevision(revision, filename)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		for name, count := range baseline {
			if name == "@total" {
				continue
			}
			if prior, duplicate := result[name]; duplicate && prior != count {
				return nil, fmt.Errorf("package %s has conflicting prior baselines across denominators", name)
			}
			result[name] = count
		}
	}
	return result, nil
}

func sourcePackagesAtRevision(revision, module string) (map[string]bool, error) {
	root, err := moduleRoot()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("git", "ls-tree", "-rz", "--name-only", revision, "--")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf(
			"inspect source packages at revision %q: %s",
			revision,
			strings.TrimSpace(string(output)),
		)
	}
	return sourcePackagesFromTree(module, output)
}

func sourcePackagesFromTree(module string, tree []byte) (map[string]bool, error) {
	if strings.TrimSpace(module) == "" || strings.ContainsAny(module, "\t\r\n ") {
		return nil, errors.New("module name is empty or malformed")
	}
	paths := make([]string, 0, bytes.Count(tree, []byte{0}))
	for _, rawName := range bytes.Split(tree, []byte{0}) {
		if len(rawName) == 0 {
			continue
		}
		filename := filepath.ToSlash(string(rawName))
		clean := path.Clean(filename)
		if clean != filename || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, fmt.Errorf("source package tree contains unsafe path %q", filename)
		}
		paths = append(paths, clean)
	}
	nestedModules := make(map[string]bool)
	for _, filename := range paths {
		if path.Base(filename) == "go.mod" && path.Dir(filename) != "." {
			nestedModules[path.Dir(filename)] = true
		}
	}
	result := make(map[string]bool)
	for _, filename := range paths {
		if path.Ext(filename) != ".go" || strings.HasSuffix(filename, "_test.go") || ignoredSourceTreePath(filename, nestedModules) {
			continue
		}
		directory := path.Dir(filename)
		packageName := module
		if directory != "." {
			packageName += "/" + directory
		}
		result[packageName] = true
	}
	return result, nil
}

func ignoredSourceTreePath(filename string, nestedModules map[string]bool) bool {
	directory := path.Dir(filename)
	for current := directory; current != "."; current = path.Dir(current) {
		if nestedModules[current] {
			return true
		}
		name := path.Base(current)
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "vendor" {
			return true
		}
	}
	name := path.Base(filename)
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

func packageEntryFloor(class string, entry packageEntry) (int64, bool) {
	if class == classTooling || entry.Class == classTooling {
		return 0, false
	}
	if entry.Critical {
		return 95, true
	}
	return 80, true
}

func guardBaseline(
	packages packageSet,
	current map[string]statementCount,
	head map[string]statementCount,
	prior map[string]statementCount,
	migrationPrior map[string]statementCount,
	class string,
) error {
	var failures []string
	for name, oldCount := range prior {
		newCount, exists := head[name]
		if !exists {
			if name != "@total" {
				entry, stillClassified := packages.ByName[name]
				if !stillClassified || !classIncludes(class, entry.Class) {
					continue
				}
			}
			failures = append(failures, "baseline removed protected entry: "+name)
			continue
		}
		if ratioLess(newCount, oldCount) {
			failures = append(failures, fmt.Sprintf("baseline for %s was lowered from %s to %s", name, percent(oldCount), percent(newCount)))
		}
	}
	for name := range head {
		if name == "@total" {
			continue
		}
		if _, existed := prior[name]; existed {
			continue
		}
		entry, ok := packages.ByName[name]
		if !ok || !classIncludes(class, entry.Class) {
			continue
		}
		minimum, enforced := packageEntryFloor(class, entry)
		got := current[name]
		if enforced && belowPercentFloor(got, minimum) {
			failures = append(failures, fmt.Sprintf("new package %s is %s; minimum is %d%%", name, percent(got), minimum))
		}
		if oldCount, migrated := migrationPrior[name]; migrated && ratioLess(head[name], oldCount) {
			failures = append(failures, fmt.Sprintf("baseline for migrated package %s was lowered from %s to %s", name, percent(oldCount), percent(head[name])))
		}
	}
	sort.Strings(failures)
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "\n"))
	}
	return nil
}

func compareRatchet(packages packageSet, current, baseline map[string]statementCount, class string) []string {
	remaining := make(map[string]statementCount, len(baseline))
	for name, count := range baseline {
		remaining[name] = count
	}
	var failures []string
	for _, entry := range packages.Entries {
		if !classIncludes(class, entry.Class) {
			continue
		}
		got := current[entry.Name]
		want, exists := remaining[entry.Name]
		if !exists {
			failures = append(failures, "checked-in baseline is missing package: "+entry.Name)
			minimum, enforced := packageEntryFloor(class, entry)
			if enforced && belowPercentFloor(got, minimum) {
				failures = append(failures, fmt.Sprintf("new package %s is %s; minimum is %d%%", entry.Name, percent(got), minimum))
			}
			continue
		}
		if ratioLess(got, want) {
			failures = append(failures, fmt.Sprintf("%s regressed from %s to %s", entry.Name, percent(want), percent(got)))
		} else if ratioLess(want, got) {
			failures = append(failures, fmt.Sprintf("%s baseline is stale at %s; raise it to the current %s", entry.Name, percent(want), percent(got)))
		} else if got != want {
			failures = append(failures, fmt.Sprintf("%s baseline counts are stale at %d/%d; record the current %d/%d", entry.Name, want.Covered, want.Total, got.Covered, got.Total))
		}
		delete(remaining, entry.Name)
	}
	gotTotal := current["@total"]
	if wantTotal, ok := remaining["@total"]; !ok {
		failures = append(failures, "baseline is missing @total")
	} else {
		if ratioLess(gotTotal, wantTotal) {
			failures = append(failures, fmt.Sprintf("aggregate regressed from %s to %s", percent(wantTotal), percent(gotTotal)))
		} else if ratioLess(wantTotal, gotTotal) {
			failures = append(failures, fmt.Sprintf("aggregate baseline is stale at %s; raise it to the current %s", percent(wantTotal), percent(gotTotal)))
		} else if gotTotal != wantTotal {
			failures = append(failures, fmt.Sprintf("aggregate baseline counts are stale at %d/%d; record the current %d/%d", wantTotal.Covered, wantTotal.Total, gotTotal.Covered, gotTotal.Total))
		}
		delete(remaining, "@total")
	}
	for stale := range remaining {
		failures = append(failures, "baseline has stale package: "+stale)
	}
	return failures
}

func loadExceptions(filename string, packages packageSet) (map[string]exception, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open exceptions: %w", err)
	}
	defer func() { _ = file.Close() }()
	records, err := readTSV(file)
	if err != nil {
		return nil, err
	}
	wantHeader := []string{"package", "file", "line", "reason", "owner", "evidence", "review_deadline"}
	if len(records) == 0 || !equalStrings(records[0], wantHeader) {
		return nil, errors.New("exceptions file has an invalid header")
	}
	now := time.Now().UTC()
	result := make(map[string]exception)
	for row, record := range records[1:] {
		if len(record) != len(wantHeader) {
			return nil, fmt.Errorf("exception row %d: expected %d fields", row+2, len(wantHeader))
		}
		entry, ok := packages.ByName[record[0]]
		if !ok {
			return nil, fmt.Errorf("exception row %d: package is not classified", row+2)
		}
		line := 0
		exceptionFile := path.Clean(record[1])
		if record[2] == "package" {
			if entry.Class != classSupport {
				return nil, fmt.Errorf("exception row %d: package scope is only valid for support exclusions", row+2)
			}
			if exceptionFile != strings.TrimPrefix(entry.Name, packages.Module+"/") {
				return nil, fmt.Errorf("exception row %d: package-scope file must be the package directory", row+2)
			}
		} else {
			line, err = strconv.Atoi(record[2])
			if err != nil || line < 1 {
				return nil, fmt.Errorf("exception row %d: line must be positive or package", row+2)
			}
			if entry.Class != classActive {
				return nil, fmt.Errorf("exception row %d: line exceptions are only valid for active packages", row+2)
			}
			if packages.Module+"/"+path.Dir(exceptionFile) != entry.Name {
				return nil, fmt.Errorf("exception row %d: file does not belong to package", row+2)
			}
		}
		for index := 3; index <= 5; index++ {
			if strings.TrimSpace(record[index]) == "" {
				return nil, fmt.Errorf("exception row %d: %s is required", row+2, wantHeader[index])
			}
		}
		deadline, err := time.Parse("2006-01-02", record[6])
		if err != nil {
			return nil, fmt.Errorf("exception row %d deadline: %w", row+2, err)
		}
		if deadline.Before(now.Truncate(24 * time.Hour)) {
			return nil, fmt.Errorf("exception row %d expired on %s", row+2, record[6])
		}
		if deadline.After(now.AddDate(0, 0, 180)) {
			return nil, fmt.Errorf("exception row %d deadline is more than 180 days away", row+2)
		}
		ex := exception{Package: record[0], File: exceptionFile, Line: line, Deadline: deadline}
		key := exceptionKey(ex.Package, ex.File, ex.Line)
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("duplicate exception at %s:%d", ex.File, ex.Line)
		}
		result[key] = ex
	}
	return result, nil
}

func changedGoLines(base, head string) (map[string]map[int]bool, error) {
	args := []string{"diff", "--unified=0", "--no-color", "--no-renames", base}
	if head != "WORKTREE" {
		args = append(args, head)
	}
	args = append(args, "--", "*.go")
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git diff %s %s: %s", base, head, strings.TrimSpace(string(out)))
	}
	result, err := parseChangedGoLines(out)
	if err != nil {
		return nil, err
	}
	if head != "WORKTREE" {
		return result, nil
	}

	untracked := exec.Command("git", "ls-files", "--others", "--exclude-standard", "-z", "--", "*.go")
	untrackedOut, err := untracked.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list untracked Go files: %s", strings.TrimSpace(string(untrackedOut)))
	}
	for _, raw := range strings.Split(string(untrackedOut), "\x00") {
		filename := filepath.ToSlash(raw)
		if filename == "" {
			continue
		}
		if err := addWholeFileLines(result, ".", filename); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func parseChangedGoLines(diff []byte) (map[string]map[int]bool, error) {
	result := make(map[string]map[int]bool)
	currentFile := ""
	hunkStarted := false
	hunkRE := regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)
	scanner := bufio.NewScanner(strings.NewReader(string(diff)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "diff --git ") {
			currentFile = ""
			hunkStarted = false
			continue
		}
		if !hunkStarted && strings.HasPrefix(line, "+++ b/") {
			currentFile = strings.TrimPrefix(line, "+++ b/")
			continue
		}
		matches := hunkRE.FindStringSubmatch(line)
		if matches == nil || currentFile == "" {
			continue
		}
		hunkStarted = true
		start, _ := strconv.Atoi(matches[1])
		count := 1
		if matches[2] != "" {
			count, _ = strconv.Atoi(matches[2])
		}
		if count == 0 {
			continue
		}
		if result[currentFile] == nil {
			result[currentFile] = make(map[int]bool)
		}
		for changedLine := start; changedLine < start+count; changedLine++ {
			result[currentFile][changedLine] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func addWholeFileLines(changed map[string]map[int]bool, root, filename string) error {
	clean := path.Clean(filename)
	if clean != filename || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("untracked Go file has unsafe path %q", filename)
	}
	sourcePath := filepath.Join(root, filepath.FromSlash(clean))
	info, err := os.Lstat(sourcePath)
	if err != nil {
		return fmt.Errorf("inspect untracked Go file %s: %w", filename, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("untracked Go file %s is not a regular file", filename)
	}
	file, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open untracked Go file %s: %w", filename, err)
	}
	defer func() { _ = file.Close() }()

	lines := make(map[int]bool)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for line := 1; scanner.Scan(); line++ {
		lines[line] = true
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read untracked Go file %s: %w", filename, err)
	}
	if len(lines) == 0 {
		lines[1] = true
	}
	changed[clean] = lines
	return nil
}

func packageForProfileFile(packages packageSet, filename string) (packageEntry, bool) {
	directory := path.Dir(path.Clean(filename))
	if entry, ok := packages.ByName[directory]; ok {
		return entry, true
	}
	marker := "/" + packages.Module + "/"
	if index := strings.LastIndex(directory, marker); index >= 0 {
		entry, ok := packages.ByName[directory[index+1:]]
		return entry, ok
	}
	return packageEntry{}, false
}

func profileFileRelative(module, filename string) (string, error) {
	clean := path.Clean(filename)
	prefix := module + "/"
	if strings.HasPrefix(clean, prefix) {
		return strings.TrimPrefix(clean, prefix), nil
	}
	marker := "/" + module + "/"
	if index := strings.LastIndex(clean, marker); index >= 0 {
		return clean[index+len(marker):], nil
	}
	return "", fmt.Errorf("profile file %q is outside module %q", filename, module)
}

func intersectingBlocks(blocks []coverBlock, line int) []coverBlock {
	var result []coverBlock
	for _, block := range blocks {
		if block.StartLine <= line && line <= block.EndLine {
			result = append(result, block)
		}
	}
	return result
}

func classIncludes(selected, actual string) bool {
	if selected == "repository" {
		return actual != classSupport
	}
	return selected == actual
}

func validateClass(class string) error {
	if class != "repository" && class != classActive && class != classExperimentalTUI && class != classTooling {
		return fmt.Errorf("invalid coverage class %q", class)
	}
	return nil
}

func ratioLess(got, want statementCount) bool {
	if want.Total == 0 {
		return got.Total > 0 && got.Covered != got.Total
	}
	if got.Total == 0 {
		return true
	}
	gotHigh, gotLow := bits.Mul64(uint64(got.Covered), uint64(want.Total))
	wantHigh, wantLow := bits.Mul64(uint64(want.Covered), uint64(got.Total))
	if gotHigh != wantHigh {
		return gotHigh < wantHigh
	}
	return gotLow < wantLow
}

func belowPercentFloor(got statementCount, minimum int64) bool {
	return ratioLess(got, statementCount{Covered: minimum, Total: 100})
}

func percent(count statementCount) string {
	if count.Total == 0 {
		return "100.00%"
	}
	return fmt.Sprintf("%.2f%%", float64(count.Covered)*100/float64(count.Total))
}

func readTSV(reader io.Reader) ([][]string, error) {
	csvReader := csv.NewReader(reader)
	csvReader.Comma = '\t'
	csvReader.FieldsPerRecord = -1
	csvReader.Comment = '#'
	return csvReader.ReadAll()
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func replaceEnv(environment []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(environment)+1)
	for _, item := range environment {
		if !strings.HasPrefix(item, prefix) {
			result = append(result, item)
		}
	}
	return append(result, prefix+value)
}

func exceptionKey(packageName, filename string, line int) string {
	return fmt.Sprintf("%s\x00%s\x00%d", packageName, path.Clean(filename), line)
}
