package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunDispatchRejectsMissingAndUnknownCommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing command", want: "expected one of"},
		{name: "unknown command", args: []string{"unknown"}, want: `unknown command "unknown"`},
		{name: "verify shard arguments", args: []string{"verify-shard"}, want: "requires -dir and -source-manifest"},
		{name: "verify shard flags", args: []string{"verify-shard", "-unknown"}, want: "flag provided but not defined"},
		{name: "source manifest arguments", args: []string{"source-manifest"}, want: "requires -out"},
		{name: "source manifest flags", args: []string{"source-manifest", "-unknown"}, want: "flag provided but not defined"},
		{name: "binary identity arguments", args: []string{"binary-identity"}, want: "requires -binary, -source-manifest, and -out"},
		{name: "binary identity flags", args: []string{"binary-identity", "-unknown"}, want: "flag provided but not defined"},
		{name: "verify binary identity arguments", args: []string{"verify-binary-identity"}, want: "requires -binary, -source-manifest, and -identity"},
		{name: "verify binary identity flags", args: []string{"verify-binary-identity", "-unknown"}, want: "flag provided but not defined"},
		{name: "validate arguments", args: []string{"validate"}, want: "requires -release-tags"},
		{name: "validate flags", args: []string{"validate", "-unknown"}, want: "flag provided but not defined"},
		{name: "filter arguments", args: []string{"filter"}, want: "requires -profile and -out"},
		{name: "filter flags", args: []string{"filter", "-unknown"}, want: "flag provided but not defined"},
		{name: "report arguments", args: []string{"report"}, want: "requires -profile"},
		{name: "report flags", args: []string{"report", "-unknown"}, want: "flag provided but not defined"},
		{name: "patch arguments", args: []string{"patch"}, want: "requires -profile, -base, and -head"},
		{name: "patch flags", args: []string{"patch", "-unknown"}, want: "flag provided but not defined"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := run(test.args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("run(%q) error = %v, want containing %q", test.args, err, test.want)
			}
		})
	}
}

func TestRunBinaryIdentityFailsClosedOnMismatch(t *testing.T) {
	root := t.TempDir()
	binaryContents := []byte("coverage-instrumented akt binary\n")
	manifestContents := []byte(strings.Join([]string{
		"path\tsha256",
		"@build-options\t0000000000000000000000000000000000000000000000000000000000000000",
		"cmd/akt/main.go\t1111111111111111111111111111111111111111111111111111111111111111",
		"",
	}, "\n"))
	binary := commandTestWriteFile(t, root, "akt", string(binaryContents))
	manifest := commandTestWriteFile(t, root, "source-manifest.tsv", string(manifestContents))
	identity := filepath.Join(root, "binary-identity.tsv")

	if err := run([]string{
		"binary-identity", "-binary", binary,
		"-source-manifest", manifest, "-out", identity,
	}); err != nil {
		t.Fatalf("run binary-identity: %v", err)
	}
	binaryDigest := sha256.Sum256(binaryContents)
	manifestDigest := sha256.Sum256(manifestContents)
	commandTestRequireFile(t, identity, fmt.Sprintf(
		"artifact\tsha256\nbinary\t%x\nsource-manifest\t%x\n",
		binaryDigest, manifestDigest,
	))
	info, err := os.Stat(identity)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("binary identity mode = %o, want 600", info.Mode().Perm())
	}

	verifyArgs := []string{
		"verify-binary-identity", "-binary", binary,
		"-source-manifest", manifest, "-identity", identity,
	}
	if err := run(verifyArgs); err != nil {
		t.Fatalf("verify matching binary identity: %v", err)
	}
	if err := os.WriteFile(binary, []byte("replacement binary\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(verifyArgs); err == nil || !strings.Contains(err.Error(), "binary digest mismatch") {
		t.Fatalf("replacement binary error = %v, want binary digest mismatch", err)
	}
	if err := os.Remove(binary); err != nil {
		t.Fatal(err)
	}
	if err := run(verifyArgs); err == nil || !strings.Contains(err.Error(), "digest coverage binary") {
		t.Fatalf("missing binary error = %v, want digest failure", err)
	}
	if err := os.WriteFile(binary, binaryContents, 0o600); err != nil {
		t.Fatal(err)
	}
	changedManifest := strings.Replace(string(manifestContents), strings.Repeat("1", 64), strings.Repeat("2", 64), 1)
	if err := os.WriteFile(manifest, []byte(changedManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(verifyArgs); err == nil || !strings.Contains(err.Error(), "source manifest digest mismatch") {
		t.Fatalf("replacement manifest error = %v, want source manifest digest mismatch", err)
	}
	if err := os.WriteFile(manifest, manifestContents, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(identity, []byte("artifact\tsha256\nbinary\t"+strings.Repeat("0", 64)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(verifyArgs); err == nil || !strings.Contains(err.Error(), "must contain exactly") {
		t.Fatalf("malformed identity error = %v, want strict record-count failure", err)
	}
	if err := os.Remove(identity); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(binary, identity); err != nil {
		t.Fatal(err)
	}
	if err := run(verifyArgs); err == nil || !strings.Contains(err.Error(), "must be a regular file") {
		t.Fatalf("identity symlink error = %v, want regular-file failure", err)
	}
}

func TestRunBinaryIdentityRejectsInvalidArtifacts(t *testing.T) {
	root := t.TempDir()
	binary := commandTestWriteFile(t, root, "akt", "instrumented binary\n")
	manifestContents := strings.Join([]string{
		"path\tsha256",
		"@build-options\t0000000000000000000000000000000000000000000000000000000000000000",
		"cmd/akt/main.go\t1111111111111111111111111111111111111111111111111111111111111111",
		"",
	}, "\n")
	manifest := commandTestWriteFile(t, root, "source-manifest.tsv", manifestContents)
	identity := filepath.Join(root, "binary-identity.tsv")

	for _, test := range []struct {
		name     string
		binary   string
		manifest string
		out      string
		want     string
	}{
		{name: "missing binary", binary: filepath.Join(root, "missing-akt"), manifest: manifest, out: identity, want: "digest coverage binary"},
		{name: "directory binary", binary: root, manifest: manifest, out: identity, want: "is not a regular file"},
		{name: "empty binary", binary: commandTestWriteFile(t, root, "empty-akt", ""), manifest: manifest, out: identity, want: "is empty"},
		{name: "invalid source manifest", binary: binary, manifest: commandTestWriteFile(t, root, "invalid-manifest.tsv", "invalid\n"), out: identity, want: "load binary source manifest"},
		{name: "unwritable output", binary: binary, manifest: manifest, out: root, want: "write binary identity"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := run([]string{
				"binary-identity", "-binary", test.binary,
				"-source-manifest", test.manifest, "-out", test.out,
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("binary-identity error = %v, want containing %q", err, test.want)
			}
		})
	}

	if err := run([]string{
		"binary-identity", "-binary", binary,
		"-source-manifest", manifest, "-out", identity,
	}); err != nil {
		t.Fatalf("create valid identity: %v", err)
	}
	verifyArgs := []string{
		"verify-binary-identity", "-binary", binary,
		"-source-manifest", manifest, "-identity", identity,
	}
	if err := os.WriteFile(manifest, []byte("not a manifest\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(verifyArgs); err == nil || !strings.Contains(err.Error(), "load binary source manifest") {
		t.Fatalf("verify invalid source manifest error = %v", err)
	}
	if err := os.WriteFile(manifest, []byte(manifestContents), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		contents string
		want     string
	}{
		{name: "malformed TSV", contents: "\"unterminated\n", want: "parse"},
		{name: "invalid header", contents: "kind\tdigest\nbinary\t" + strings.Repeat("0", 64) + "\nsource-manifest\t" + strings.Repeat("1", 64) + "\n", want: "invalid header"},
		{name: "invalid binary row", contents: "artifact\tsha256\nexecutable\t" + strings.Repeat("0", 64) + "\nsource-manifest\t" + strings.Repeat("1", 64) + "\n", want: "row 2 must be binary"},
		{name: "invalid manifest digest", contents: "artifact\tsha256\nbinary\t" + strings.Repeat("0", 64) + "\nsource-manifest\tNOT-A-DIGEST\n", want: "row 3 must be source-manifest"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(identity, []byte(test.contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := run(verifyArgs); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("verify-binary-identity error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestRunReportRejectsMissingProfileBeforePublication(t *testing.T) {
	root := commandTestModule(t, "example.test/missingprofile")
	t.Chdir(root)
	packagesFile := commandTestWriteFile(t, root, "coverage/packages.tsv", strings.Join([]string{
		"package\tclass\tcritical",
		"example.test/missingprofile/internal/active\tactive\tfalse",
		"",
	}, "\n"))

	err := run([]string{
		"report",
		"-packages", packagesFile,
		"-profile", filepath.Join(root, "coverage/missing.out"),
		"-out", filepath.Join(root, "coverage/report.tsv"),
	})
	if err == nil || !strings.Contains(err.Error(), "open profile") {
		t.Fatalf("report missing-profile error = %v, want open profile failure", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "coverage/report.tsv")); !os.IsNotExist(statErr) {
		t.Fatalf("report artifact exists after missing input: %v", statErr)
	}
}

func TestRunReportRequireCompleteRejectsMissingExecutablePackage(t *testing.T) {
	root := commandTestModule(t, "example.test/complete")
	t.Chdir(root)

	module := "example.test/complete"
	packagesFile := commandTestWriteFile(t, root, "coverage/packages.tsv", strings.Join([]string{
		"package\tclass\tcritical",
		module + "/internal/present\texperimental-tui\tfalse",
		module + "/internal/missing\texperimental-tui\tfalse",
		"",
	}, "\n"))
	commandTestWriteFile(t, root, "internal/present/present.go", "package present\n\nfunc Present() { println(\"present\") }\n")
	commandTestWriteFile(t, root, "internal/missing/missing.go", "package missing\n\nfunc Missing() { println(\"missing\") }\n")
	profileFile := commandTestWriteFile(t, root, "coverage/profile.out", strings.Join([]string{
		"mode: atomic",
		module + "/internal/present/present.go:3.16,3.36 1 1",
		"",
	}, "\n"))
	reportFile := filepath.Join(root, "coverage/report.tsv")

	err := run([]string{
		"report", "-packages", packagesFile, "-profile", profileFile,
		"-class", classExperimentalTUI, "-require-complete", "-out", reportFile,
	})
	if err == nil || !strings.Contains(err.Error(), module+"/internal/missing") {
		t.Fatalf("complete report error = %v, want missing executable package", err)
	}
	if _, statErr := os.Stat(reportFile); !os.IsNotExist(statErr) {
		t.Fatalf("report artifact exists after incomplete profile: %v", statErr)
	}
}

func TestRunReportRequireCompleteRejectsMissingExecutableFile(t *testing.T) {
	root := commandTestModule(t, "example.test/filecomplete")
	t.Chdir(root)

	module := "example.test/filecomplete"
	packagesFile := commandTestWriteFile(t, root, "coverage/packages.tsv", strings.Join([]string{
		"package\tclass\tcritical",
		module + "/internal/tui\texperimental-tui\tfalse",
		"",
	}, "\n"))
	commandTestWriteFile(t, root, "internal/tui/covered.go", "package tui\n\nfunc Covered() { println(\"covered\") }\n")
	commandTestWriteFile(t, root, "internal/tui/missing.go", "package tui\n\nfunc Missing() { println(\"missing\") }\n")
	commandTestWriteFile(t, root, "internal/tui/declarations.go", "package tui\n\ntype Ready struct{}\n")
	profileFile := commandTestWriteFile(t, root, "coverage/profile.out", strings.Join([]string{
		"mode: atomic",
		module + "/internal/tui/covered.go:3.16,3.36 1 1",
		"",
	}, "\n"))
	reportFile := filepath.Join(root, "coverage/report.tsv")

	err := run([]string{
		"report", "-packages", packagesFile, "-profile", profileFile,
		"-class", classExperimentalTUI, "-require-complete", "-out", reportFile,
	})
	if err == nil || !strings.Contains(err.Error(), "internal/tui/missing.go") {
		t.Fatalf("complete report error = %v, want missing executable file", err)
	}
	if _, statErr := os.Stat(reportFile); !os.IsNotExist(statErr) {
		t.Fatalf("report artifact exists after incomplete profile: %v", statErr)
	}
}

func TestRunReportRequireCompleteAllowsDeclarationsOnlyPackage(t *testing.T) {
	root := commandTestModule(t, "example.test/declarations")
	t.Chdir(root)

	module := "example.test/declarations"
	packagesFile := commandTestWriteFile(t, root, "coverage/packages.tsv", strings.Join([]string{
		"package\tclass\tcritical",
		module + "/internal/messages\texperimental-tui\tfalse",
		module + "/internal/tui\texperimental-tui\tfalse",
		"",
	}, "\n"))
	commandTestWriteFile(t, root, "internal/tui/tui.go", "package tui\n\nfunc Run() { println(\"run\") }\n")
	commandTestWriteFile(t, root, "internal/messages/messages.go", "package messages\n\ntype Ready struct{}\n\nvar Default = Ready{}\n")
	commandTestWriteFile(t, root, "internal/messages/_ignored.go", "package messages\n\nfunc Ignored() { println(\"ignored\") }\n")
	commandTestWriteFile(t, root, "internal/messages/messages_test.go", "package messages\n\nfunc testOnly() { println(\"test\") }\n")
	commandTestWriteFile(t, root, "internal/messages/notes.txt", "not Go source\n")
	profileFile := commandTestWriteFile(t, root, "coverage/profile.out", strings.Join([]string{
		"mode: atomic",
		module + "/internal/tui/tui.go:3.12,3.28 1 1",
		"",
	}, "\n"))
	reportFile := filepath.Join(root, "coverage/report.tsv")

	if err := run([]string{
		"report", "-packages", packagesFile, "-profile", profileFile,
		"-class", classExperimentalTUI, "-require-complete", "-out", reportFile,
	}); err != nil {
		t.Fatalf("complete report with declarations-only package: %v", err)
	}
	if report := commandTestReadFile(t, reportFile); !strings.Contains(
		report,
		module+"/internal/messages\t0\t0\t100.00%",
	) {
		t.Fatalf("declarations-only package missing from report:\n%s", report)
	}
}

func TestRunFilterAndReportWriteExactArtifacts(t *testing.T) {
	root := commandTestModule(t, "example.test/coveragecmd")
	t.Chdir(root)

	module := "example.test/coveragecmd"
	packagesFile := commandTestWriteFile(t, root, "coverage/packages.tsv", strings.Join([]string{
		"package\tclass\tcritical",
		module + "/internal/active\tactive\tfalse",
		module + "/internal/support\tsupport\tfalse",
		module + "/internal/tui\texperimental-tui\tfalse",
		module + "/tools/coverage\ttooling\tfalse",
		"",
	}, "\n"))
	profileFile := commandTestWriteFile(t, root, "coverage/all.out", strings.Join([]string{
		"mode: atomic",
		module + "/internal/active/a.go:1.1,2.2 2 1",
		module + "/internal/support/helper.go:1.1,2.2 1 1",
		module + "/internal/tui/view.go:1.1,2.2 3 0",
		"/tmp/build/" + module + "/tools/coverage/main.go:1.1,2.2 5 4",
		"",
	}, "\n"))

	filtered := filepath.Join(root, "coverage", "active.out")
	if err := run([]string{"filter", "-packages", packagesFile, "-profile", profileFile, "-class", classActive, "-out", filtered}); err != nil {
		t.Fatalf("run filter: %v", err)
	}
	commandTestRequireFile(t, filtered, "mode: atomic\n"+module+"/internal/active/a.go:1.1,2.2 2 1\n")

	report := filepath.Join(root, "coverage", "report.tsv")
	if err := run([]string{"report", "-packages", packagesFile, "-profile", profileFile, "-class", "repository", "-out", report}); err != nil {
		t.Fatalf("run report: %v", err)
	}
	commandTestRequireFile(t, report, strings.Join([]string{
		"package\tcovered\ttotal\tpercent",
		module + "/internal/active\t2\t2\t100.00%",
		module + "/internal/tui\t0\t3\t0.00%",
		module + "/tools/coverage\t5\t5\t100.00%",
		"@total\t7\t10\t70.00%",
		"",
	}, "\n"))

	for _, command := range []string{"filter", "report"} {
		args := []string{command, "-packages", packagesFile, "-profile", profileFile, "-class", "bogus"}
		if command == "filter" {
			args = append(args, "-out", filepath.Join(root, "coverage", "unused.out"))
		}
		if err := run(args); err == nil || !strings.Contains(err.Error(), `invalid coverage class "bogus"`) {
			t.Fatalf("%s invalid class error = %v", command, err)
		}
	}
	unclassified := commandTestWriteFile(t, root, "coverage/unclassified.out", "mode: atomic\n"+module+"/internal/missing/a.go:1.1,2.2 1 1\n")
	if err := run([]string{"filter", "-packages", packagesFile, "-profile", unclassified, "-out", filtered}); err == nil || !strings.Contains(err.Error(), "does not belong to a classified package") {
		t.Fatalf("filter unclassified profile error = %v", err)
	}
	if err := run([]string{"filter", "-packages", packagesFile, "-profile", profileFile, "-out", root}); err == nil || !strings.Contains(err.Error(), "create profile") {
		t.Fatalf("filter directory output error = %v", err)
	}
	if err := run([]string{"report", "-packages", packagesFile, "-profile", profileFile, "-out", root}); err == nil || !strings.Contains(err.Error(), "write report") {
		t.Fatalf("report directory output error = %v", err)
	}
}

func TestRunSourceManifestAndVerifyShardBoundaries(t *testing.T) {
	root := commandTestModule(t, "example.test/manifest")
	commandTestWriteFile(t, root, "sample.go", "package sample\n\nimport _ \"embed\"\n\n//go:embed data.txt\nvar data string\n")
	commandTestWriteFile(t, root, "data.txt", "embedded input\n")
	manifest := filepath.Join(root, "source-manifest.tsv")

	if err := run([]string{"source-manifest", "-root", root, "-out", manifest, "-build-tags", "netgo,netgo", "-build-options", "static"}); err != nil {
		t.Fatalf("run source-manifest: %v", err)
	}
	contents, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []string{"@build-options\t", "@build-tags\t", "data.txt\t", "go.mod\t", "sample.go\t"} {
		if !strings.Contains(string(contents), entry) {
			t.Errorf("source manifest omitted %q:\n%s", entry, contents)
		}
	}
	info, err := os.Stat(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("source manifest mode = %o, want 600", info.Mode().Perm())
	}
	if err := run([]string{"source-manifest", "-root", root, "-out", manifest, "-build-tags=-bad"}); err == nil || !strings.Contains(err.Error(), "invalid build tag") {
		t.Fatalf("source-manifest invalid tags error = %v", err)
	}
	if err := run([]string{"source-manifest", "-root", root, "-out", root, "-build-tags", "netgo"}); err == nil || !strings.Contains(err.Error(), "write source manifest") {
		t.Fatalf("source-manifest directory output error = %v", err)
	}

	shard := filepath.Join(root, "shard")
	if err := os.MkdirAll(shard, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shard, "source-manifest.tsv"), contents, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, covdata := range buildValidCovdataShard(t) {
		if name == "source-manifest.tsv" {
			continue
		}
		if err := os.WriteFile(filepath.Join(shard, name), []byte(covdata), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	stdout, err := commandTestCaptureStdout(t, func() error {
		return run([]string{"verify-shard", "-dir", shard, "-name", "unit", "-source-manifest", manifest})
	})
	if err != nil {
		t.Fatalf("run verify-shard: %v", err)
	}
	if stdout != "unit coverage shard: 1 metadata files, 1 counter files\n" {
		t.Fatalf("verify-shard stdout = %q", stdout)
	}

	stale := strings.Replace(string(contents), "@build-options", "@build-optionz", 1)
	if err := os.WriteFile(filepath.Join(shard, "source-manifest.tsv"), []byte(stale), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"verify-shard", "-dir", shard, "-name", "unit", "-source-manifest", manifest}); err == nil || !strings.Contains(err.Error(), "unit shard: source manifest mismatch") {
		t.Fatalf("verify-shard stale manifest error = %v", err)
	}
}

func TestRunVerifyShardRequiresMatchingBinaryIdentity(t *testing.T) {
	root := t.TempDir()
	manifestContents := strings.Join([]string{
		"path\tsha256",
		"@build-options\t0000000000000000000000000000000000000000000000000000000000000000",
		"cmd/akt/main.go\t1111111111111111111111111111111111111111111111111111111111111111",
		"",
	}, "\n")
	manifest := commandTestWriteFile(t, root, "current-source-manifest.tsv", manifestContents)
	binary := commandTestWriteFile(t, root, "akt", "coverage-instrumented binary\n")
	shard := filepath.Join(root, "e2e-shard")
	if err := os.MkdirAll(shard, 0o700); err != nil {
		t.Fatal(err)
	}
	commandTestWriteFile(t, shard, "source-manifest.tsv", manifestContents)
	for name, covdata := range buildValidCovdataShard(t) {
		if name == "source-manifest.tsv" {
			continue
		}
		commandTestWriteFile(t, shard, name, covdata)
	}

	verifyArgs := []string{
		"verify-shard", "-dir", shard, "-name", "e2e-offline",
		"-source-manifest", manifest, "-require-binary-identity",
	}
	if err := run(verifyArgs); err == nil || !strings.Contains(err.Error(), "binary identity is required") {
		t.Fatalf("missing E2E binary identity error = %v", err)
	}

	identity := filepath.Join(shard, "binary-identity.tsv")
	if err := run([]string{
		"binary-identity", "-binary", binary,
		"-source-manifest", manifest, "-out", identity,
	}); err != nil {
		t.Fatalf("create shard binary identity: %v", err)
	}
	if _, err := commandTestCaptureStdout(t, func() error { return run(verifyArgs) }); err != nil {
		t.Fatalf("report-time E2E identity verification: %v", err)
	}
	collectionArgs := append(append([]string{}, verifyArgs...), "-binary", binary)
	if _, err := commandTestCaptureStdout(t, func() error { return run(collectionArgs) }); err != nil {
		t.Fatalf("collection-time E2E identity verification: %v", err)
	}

	if err := os.WriteFile(binary, []byte("replaced after shard preparation\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(collectionArgs); err == nil || !strings.Contains(err.Error(), "binary digest mismatch") {
		t.Fatalf("post-prepare replacement error = %v, want binary digest mismatch", err)
	}
	if err := run([]string{
		"verify-shard", "-dir", shard, "-name", "unit",
		"-source-manifest", manifest,
	}); err == nil || !strings.Contains(err.Error(), "unexpected coverage shard entry binary-identity.tsv") {
		t.Fatalf("unit identity error = %v, want unexpected-entry failure", err)
	}
}

func TestRunValidateChecksRealTemporaryModule(t *testing.T) {
	root := commandTestModule(t, "example.test/validate")
	t.Chdir(root)
	module := "example.test/validate"
	commandTestWriteFile(t, root, "cmd/akt/main.go", "package main\n\nimport monitor \""+module+"/internal/monitor/runtime\"\n\nfunc main() { monitor.Run() }\n")
	commandTestWriteFile(t, root, "internal/monitor/runtime/runtime.go", "package runtime\n\nfunc Run() {}\n")
	packagesFile := commandTestWriteFile(t, root, "coverage/packages.tsv", strings.Join([]string{
		"package\tclass\tcritical",
		module + "/cmd/akt\tactive\tfalse",
		module + "/internal/monitor/runtime\tactive\tfalse",
		"",
	}, "\n"))
	exceptionsFile := commandTestWriteFile(t, root, "coverage/exceptions.tsv", "package\tfile\tline\treason\towner\tevidence\treview_deadline\n")
	commandTestWriteFile(t, root, ".goreleaser.yaml", "builds:\n  - id: sample\n    main: ./cmd/akt\n    flags:\n      - -tags=netgo\n")

	stdout, err := commandTestCaptureStdout(t, func() error {
		return run([]string{"validate", "-packages", packagesFile, "-exceptions", exceptionsFile, "-release-tags", "netgo"})
	})
	if err != nil {
		t.Fatalf("run validate: %v", err)
	}
	if stdout != "coverage package taxonomy: 2 packages validated\n" {
		t.Fatalf("validate stdout = %q", stdout)
	}
	if err := run([]string{"validate", "-packages", packagesFile, "-exceptions", exceptionsFile, "-release-tags=-bad"}); err == nil || !strings.Contains(err.Error(), "release tags: invalid build tag") {
		t.Fatalf("validate invalid release tags error = %v", err)
	}
	if err := run([]string{"validate", "-packages", packagesFile, "-exceptions", filepath.Join(root, "missing-exceptions.tsv"), "-release-tags", "netgo"}); err == nil || !strings.Contains(err.Error(), "open exceptions") {
		t.Fatalf("validate missing exceptions error = %v", err)
	}
	if err := run([]string{"validate", "-packages", filepath.Join(root, "missing-packages.tsv"), "-exceptions", exceptionsFile, "-release-tags", "netgo"}); err == nil || !strings.Contains(err.Error(), "open package taxonomy") {
		t.Fatalf("validate missing packages error = %v", err)
	}

	workflowDirectory := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(workflowDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	workflow := filepath.Join(workflowDirectory, "ci.yml")
	if err := os.WriteFile(workflow, []byte("jobs:\n  test:\n    uses: owner/repo/.github/workflows/test.yml@main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"validate", "-packages", packagesFile, "-exceptions", exceptionsFile, "-release-tags", "netgo"}); err == nil || !strings.Contains(err.Error(), "40-character commit SHA") {
		t.Fatalf("validate floating workflow action error = %v", err)
	}
	if err := os.Remove(workflow); err != nil {
		t.Fatal(err)
	}

	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("go.mod", filepath.Join(nested, "go.mod")); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"validate", "-packages", packagesFile, "-exceptions", exceptionsFile, "-release-tags", "netgo"}); err == nil || !strings.Contains(err.Error(), "inspect nested module") {
		t.Fatalf("validate source discovery error = %v", err)
	}
	if err := os.Remove(filepath.Join(nested, "go.mod")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(nested); err != nil {
		t.Fatal(err)
	}

	aktDirectory := filepath.Join(root, "cmd", "akt")
	hiddenDirectory := filepath.Join(root, "cmd", "hidden")
	if err := os.Rename(aktDirectory, hiddenDirectory); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"validate", "-packages", packagesFile, "-exceptions", exceptionsFile, "-release-tags", "netgo"}); err == nil || !strings.Contains(err.Error(), "go list dependencies") {
		t.Fatalf("validate release dependency inventory error = %v", err)
	}
	if err := os.Rename(hiddenDirectory, aktDirectory); err != nil {
		t.Fatal(err)
	}

	validPackages := commandTestReadFile(t, packagesFile)
	withStalePackage := strings.Replace(validPackages, module+"/internal/monitor/runtime\tactive\tfalse\n", module+"/internal/monitor/runtime\tactive\tfalse\n"+module+"/internal/stale\tactive\tfalse\n", 1)
	if err := os.WriteFile(packagesFile, []byte(withStalePackage), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"validate", "-packages", packagesFile, "-exceptions", exceptionsFile, "-release-tags", "netgo"}); err == nil || !strings.Contains(err.Error(), "stale package entry") {
		t.Fatalf("validate taxonomy drift error = %v", err)
	}
	if err := os.WriteFile(packagesFile, []byte(validPackages), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, ".goreleaser.yaml"), []byte("builds: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"validate", "-packages", packagesFile, "-exceptions", exceptionsFile, "-release-tags", "netgo"}); err == nil || !strings.Contains(err.Error(), "parse goreleaser configuration") {
		t.Fatalf("validate malformed goreleaser error = %v", err)
	}
}

func TestRunPatchUsesGitDiffAndUntrackedFiles(t *testing.T) {
	root := commandTestModule(t, "example.test/patch")
	t.Chdir(root)
	module := "example.test/patch"
	packagesFile := commandTestWriteFile(t, root, "coverage/packages.tsv", "package\tclass\tcritical\n"+module+"/internal/active\tactive\tfalse\n")
	exceptionsFile := commandTestWriteFile(t, root, "coverage/exceptions.tsv", "package\tfile\tline\treason\towner\tevidence\treview_deadline\n")
	profileFile := commandTestWriteFile(t, root, "coverage/profile.out", strings.Join([]string{
		"mode: atomic",
		module + "/internal/active/a.go:3.1,5.2 1 1",
		module + "/internal/active/extra.go:3.1,5.2 1 1",
		"",
	}, "\n"))
	commandTestWriteFile(t, root, "internal/active/a.go", "package active\n\nfunc Value() int {\n\treturn 1\n}\n")
	runTestGit(t, root, "init", "--quiet")
	runTestGit(t, root, "config", "user.email", "coverage-test@example.invalid")
	runTestGit(t, root, "config", "user.name", "coverage test")
	runTestGit(t, root, "add", ".")
	runTestGit(t, root, "commit", "--quiet", "-m", "initial source")

	commandTestWriteFile(t, root, "internal/active/a.go", "package active\n\nfunc Value() int {\n\treturn 2\n}\n")
	commandTestWriteFile(t, root, "internal/active/extra.go", "package active\n\nfunc Extra() int {\n\treturn 3\n}\n")
	stdout, err := commandTestCaptureStdout(t, func() error {
		return run([]string{"patch", "-packages", packagesFile, "-profile", profileFile, "-exceptions", exceptionsFile, "-base", "HEAD"})
	})
	if err != nil {
		t.Fatalf("worktree patch: %v", err)
	}
	if stdout != "active patch coverage: 100% (4 executable lines, 0 reviewed exceptions)\n" {
		t.Fatalf("worktree patch stdout = %q", stdout)
	}

	uncoveredProfile := strings.Replace(commandTestReadFile(t, profileFile), "extra.go:3.1,5.2 1 1", "extra.go:3.1,5.2 1 0", 1)
	if err := os.WriteFile(profileFile, []byte(uncoveredProfile), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"patch", "-packages", packagesFile, "-profile", profileFile, "-exceptions", exceptionsFile, "-base", "HEAD"}); err == nil || !strings.Contains(err.Error(), "internal/active/extra.go:4") {
		t.Fatalf("uncovered worktree patch error = %v", err)
	}
	if err := os.WriteFile(profileFile, []byte(strings.Replace(uncoveredProfile, "extra.go:3.1,5.2 1 0", "extra.go:3.1,5.2 1 1", 1)), 0o600); err != nil {
		t.Fatal(err)
	}

	runTestGit(t, root, "add", ".")
	runTestGit(t, root, "commit", "--quiet", "-m", "change source")
	stdout, err = commandTestCaptureStdout(t, func() error {
		return run([]string{"patch", "-packages", packagesFile, "-profile", profileFile, "-exceptions", exceptionsFile, "-base", "HEAD^", "-head", "HEAD"})
	})
	if err != nil {
		t.Fatalf("revision patch: %v", err)
	}
	if stdout != "active patch coverage: 100% (4 executable lines, 0 reviewed exceptions)\n" {
		t.Fatalf("revision patch stdout = %q", stdout)
	}
	if err := run([]string{"patch", "-profile", profileFile, "-base=-unsafe", "-head", "HEAD"}); err == nil || !strings.Contains(err.Error(), "git revisions may not start") {
		t.Fatalf("patch unsafe revision error = %v", err)
	}
	if err := run([]string{"patch", "-packages", packagesFile, "-profile", profileFile, "-exceptions", exceptionsFile, "-base", "missing-ref"}); err == nil || !strings.Contains(err.Error(), "git diff") {
		t.Fatalf("patch missing revision error = %v", err)
	}
	if err := run([]string{"patch", "-packages", packagesFile, "-profile", profileFile, "-exceptions", filepath.Join(root, "missing-exceptions.tsv"), "-base", "HEAD"}); err == nil || !strings.Contains(err.Error(), "open exceptions") {
		t.Fatalf("patch missing exceptions error = %v", err)
	}
	emptyProfile := commandTestWriteFile(t, root, "coverage/empty-profile.out", "mode: atomic\n"+module+"/internal/active/a.go:3.1,5.2 0 0\n")
	if err := run([]string{"patch", "-packages", packagesFile, "-profile", emptyProfile, "-exceptions", exceptionsFile, "-base", "HEAD"}); err == nil || !strings.Contains(err.Error(), "no executable statements") {
		t.Fatalf("patch empty active profile error = %v", err)
	}
}

func TestRunPatchLoadsSyntheticEdgesFromSeparateRawProfile(t *testing.T) {
	root := commandTestModule(t, "example.test/edgepatch")
	t.Chdir(root)
	const module = "example.test/edgepatch"
	packagesFile := commandTestWriteFile(
		t,
		root,
		"coverage/packages.tsv",
		"package\tclass\tcritical\n"+module+"/internal/active\tactive\tfalse\n",
	)
	exceptionsFile := commandTestWriteFile(
		t,
		root,
		"coverage/exceptions.tsv",
		"package\tfile\tline\treason\towner\tevidence\treview_deadline\n",
	)
	activeProfile := commandTestWriteFile(t, root, "coverage/active.out", strings.Join([]string{
		"mode: atomic",
		module + "/internal/active/active.go:3.1,4.15 1 1",
		module + "/internal/active/active.go:6.2,7.15 1 1",
		module + "/internal/active/active.go:9.2,9.13 1 1",
		"",
	}, "\n"))
	rawProfile := commandTestWriteFile(t, root, "coverage/raw.out", strings.Join([]string{
		"mode: atomic",
		module + "/internal/active/active.go:5.20,5.20 0 1",
		"",
	}, "\n"))

	const initial = `package active

func choose(value string) bool {
	switch value {
	case "old":
	default:
		return false
	}
	return true
}
`
	commandTestWriteFile(t, root, "internal/active/active.go", initial)
	runTestGit(t, root, "init", "--quiet")
	runTestGit(t, root, "config", "user.email", "coverage-test@example.invalid")
	runTestGit(t, root, "config", "user.name", "coverage test")
	runTestGit(t, root, "add", ".")
	runTestGit(t, root, "commit", "--quiet", "-m", "initial source")
	commandTestWriteFile(t, root, "internal/active/active.go", strings.Replace(initial, `case "old":`, `case "old", "new":`, 1))

	withoutEdges := run([]string{
		"patch", "-packages", packagesFile, "-profile", activeProfile,
		"-exceptions", exceptionsFile, "-base", "HEAD",
	})
	if withoutEdges == nil || !strings.Contains(withoutEdges.Error(), "no coverage block intersects executable syntax") {
		t.Fatalf("patch without edge profile error = %v, want fail-closed edge", withoutEdges)
	}

	stdout, err := commandTestCaptureStdout(t, func() error {
		return run([]string{
			"patch", "-packages", packagesFile, "-profile", activeProfile,
			"-edge-profile", rawProfile,
			"-exceptions", exceptionsFile, "-base", "HEAD",
		})
	})
	if err != nil {
		t.Fatalf("patch with separate edge profile: %v", err)
	}
	if stdout != "active patch coverage: 100% (1 executable lines, 0 reviewed exceptions)\n" {
		t.Fatalf("edge patch stdout = %q", stdout)
	}

	missingEdgeProfile := filepath.Join(root, "coverage", "missing-raw.out")
	err = run([]string{
		"patch", "-packages", packagesFile, "-profile", activeProfile,
		"-edge-profile", missingEdgeProfile,
		"-exceptions", exceptionsFile, "-base", "HEAD",
	})
	if err == nil || !strings.Contains(err.Error(), "load edge profile") {
		t.Fatalf("missing edge profile error = %v, want isolated load failure", err)
	}

	outsideModuleProfile := commandTestWriteFile(
		t,
		root,
		"coverage/outside-raw.out",
		"mode: atomic\nexample.invalid/active.go:5.20,5.20 0 1\n",
	)
	err = run([]string{
		"patch", "-packages", packagesFile, "-profile", activeProfile,
		"-edge-profile", outsideModuleProfile,
		"-exceptions", exceptionsFile, "-base", "HEAD",
	})
	if err == nil || !strings.Contains(err.Error(), "validate edge profile") {
		t.Fatalf("outside-module edge profile error = %v, want isolated validation failure", err)
	}
}

func TestLoadExceptionsAcceptsReviewedScopesAndRejectsMalformedRows(t *testing.T) {
	module := "example.test/exceptions"
	active := module + "/internal/active"
	support := module + "/internal/support"
	tooling := module + "/tools/helper"
	packages := packageSet{
		Module: module,
		Entries: []packageEntry{
			{Name: active, Class: classActive},
			{Name: support, Class: classSupport},
			{Name: tooling, Class: classTooling},
		},
		ByName: map[string]packageEntry{
			active:  {Name: active, Class: classActive},
			support: {Name: support, Class: classSupport},
			tooling: {Name: tooling, Class: classTooling},
		},
	}
	header := "package\tfile\tline\treason\towner\tevidence\treview_deadline\n"
	deadline := time.Now().UTC().AddDate(0, 0, 30).Format("2006-01-02")
	activeRow := fmt.Sprintf("%s\tinternal/active/a.go\t4\tgenerated branch\tteam\tissue-1\t%s\n", active, deadline)
	supportRow := fmt.Sprintf("%s\tinternal/support\tpackage\ttest helper\tteam\tissue-2\t%s\n", support, deadline)
	valid := commandTestWriteFile(t, t.TempDir(), "exceptions.tsv", header+activeRow+supportRow)
	got, err := loadExceptions(valid, packages)
	if err != nil {
		t.Fatalf("load valid exceptions: %v", err)
	}
	if len(got) != 2 || got[exceptionKey(active, "internal/active/a.go", 4)].Line != 4 || got[exceptionKey(support, "internal/support", 0)].Package != support {
		t.Fatalf("loaded exceptions = %#v", got)
	}

	expired := time.Now().UTC().AddDate(0, 0, -2).Format("2006-01-02")
	tooFar := time.Now().UTC().AddDate(0, 0, 181).Format("2006-01-02")
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "malformed TSV", body: "\"unterminated", want: "parse error"},
		{name: "invalid header", body: "package\tfile\n", want: "invalid header"},
		{name: "wrong field count", body: header + active + "\tfile.go\t1\n", want: "expected 7 fields"},
		{name: "unclassified package", body: header + fmt.Sprintf("missing\tfile.go\t1\treason\tteam\tissue\t%s\n", deadline), want: "package is not classified"},
		{name: "package scope on active", body: header + fmt.Sprintf("%s\tinternal/active\tpackage\treason\tteam\tissue\t%s\n", active, deadline), want: "package scope is only valid"},
		{name: "wrong support directory", body: header + fmt.Sprintf("%s\tinternal/other\tpackage\treason\tteam\tissue\t%s\n", support, deadline), want: "package-scope file"},
		{name: "zero line", body: header + fmt.Sprintf("%s\tinternal/active/a.go\t0\treason\tteam\tissue\t%s\n", active, deadline), want: "line must be positive"},
		{name: "line scope on tooling", body: header + fmt.Sprintf("%s\ttools/helper/a.go\t1\treason\tteam\tissue\t%s\n", tooling, deadline), want: "line exceptions are only valid"},
		{name: "file outside package", body: header + fmt.Sprintf("%s\tinternal/other/a.go\t1\treason\tteam\tissue\t%s\n", active, deadline), want: "file does not belong"},
		{name: "missing reason", body: header + fmt.Sprintf("%s\tinternal/active/a.go\t1\t \tteam\tissue\t%s\n", active, deadline), want: "reason is required"},
		{name: "missing owner", body: header + fmt.Sprintf("%s\tinternal/active/a.go\t1\treason\t \tissue\t%s\n", active, deadline), want: "owner is required"},
		{name: "missing evidence", body: header + fmt.Sprintf("%s\tinternal/active/a.go\t1\treason\tteam\t \t%s\n", active, deadline), want: "evidence is required"},
		{name: "malformed deadline", body: header + fmt.Sprintf("%s\tinternal/active/a.go\t1\treason\tteam\tissue\tnext-week\n", active), want: "deadline"},
		{name: "expired", body: header + fmt.Sprintf("%s\tinternal/active/a.go\t1\treason\tteam\tissue\t%s\n", active, expired), want: "expired"},
		{name: "too far", body: header + fmt.Sprintf("%s\tinternal/active/a.go\t1\treason\tteam\tissue\t%s\n", active, tooFar), want: "more than 180 days"},
		{name: "duplicate", body: header + activeRow + activeRow, want: "duplicate exception"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filename := commandTestWriteFile(t, t.TempDir(), "exceptions.tsv", test.body)
			_, err := loadExceptions(filename, packages)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("loadExceptions() error = %v, want containing %q", err, test.want)
			}
		})
	}
	if _, err := loadExceptions(filepath.Join(t.TempDir(), "missing.tsv"), packages); err == nil || !strings.Contains(err.Error(), "open exceptions") {
		t.Fatalf("missing exceptions error = %v", err)
	}
}

func TestTaxonomyAndProfileParsersRejectAmbiguousInput(t *testing.T) {
	module, err := moduleName()
	if err != nil {
		t.Fatal(err)
	}
	validEntry := module + "/tools/coverage\ttooling\tfalse\n"
	taxonomyTests := []struct {
		name string
		body string
		want string
	}{
		{name: "malformed TSV", body: "\"unterminated", want: "read package taxonomy"},
		{name: "bad header", body: "name\tclass\tcritical\n" + validEntry, want: "package taxonomy header"},
		{name: "wrong fields", body: "package\tclass\tcritical\n" + module + "/tools/coverage\ttooling\n", want: "expected 3 fields"},
		{name: "unsorted", body: "package\tclass\tcritical\n" + module + "/tools/z\ttooling\tfalse\n" + module + "/tools/a\ttooling\tfalse\n", want: "sorted and unique"},
		{name: "invalid class", body: "package\tclass\tcritical\n" + module + "/tools/coverage\tother\tfalse\n", want: "invalid class"},
		{name: "invalid critical", body: "package\tclass\tcritical\n" + module + "/tools/coverage\ttooling\tmaybe\n", want: "critical must be"},
		{name: "empty", body: "package\tclass\tcritical\n", want: "taxonomy is empty"},
		{name: "outside module", body: "package\tclass\tcritical\nexample.invalid/pkg\tactive\tfalse\n", want: "outside module"},
	}
	for _, test := range taxonomyTests {
		t.Run("taxonomy "+test.name, func(t *testing.T) {
			filename := commandTestWriteFile(t, t.TempDir(), "packages.tsv", test.body)
			_, err := loadPackages(filename)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("loadPackages() error = %v, want containing %q", err, test.want)
			}
		})
	}
	if _, err := loadPackages(filepath.Join(t.TempDir(), "missing.tsv")); err == nil || !strings.Contains(err.Error(), "open package taxonomy") {
		t.Fatalf("missing taxonomy error = %v", err)
	}

	profileTests := []struct {
		name string
		body string
		want string
	}{
		{name: "empty", want: "coverage profile is empty"},
		{name: "bad header", body: "atomic\n", want: "invalid coverage profile header"},
		{name: "non-atomic", body: "mode: count\n", want: "not merge-safe atomic"},
		{name: "malformed line", body: "mode: atomic\nnot-a-block\n", want: "profile line 2 is malformed"},
		{name: "zero coordinate", body: "mode: atomic\np/a.go:0.1,2.2 1 1\n", want: "invalid statement range"},
		{name: "reversed lines", body: "mode: atomic\np/a.go:2.1,1.2 1 1\n", want: "invalid statement range"},
		{name: "reversed columns", body: "mode: atomic\np/a.go:1.2,1.1 1 1\n", want: "invalid statement range"},
		{name: "zero-width executable block", body: "mode: atomic\np/a.go:1.2,1.2 1 1\n", want: "invalid statement range"},
		{name: "overflow coordinate", body: "mode: atomic\np/a.go:999999999999999999999999.1,2.2 1 1\n", want: "value out of range"},
		{name: "overflow count", body: "mode: atomic\np/a.go:1.1,2.2 1 999999999999999999999999\n", want: "value out of range"},
		{name: "oversized line", body: "mode: atomic\n" + strings.Repeat("x", 4*1024*1024+1) + "\n", want: "read profile"},
	}
	for _, test := range profileTests {
		t.Run("profile "+test.name, func(t *testing.T) {
			filename := commandTestWriteFile(t, t.TempDir(), "profile.out", test.body)
			_, err := loadProfile(filename)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("loadProfile() error = %v, want containing %q", err, test.want)
			}
		})
	}
	if _, err := loadProfile(filepath.Join(t.TempDir(), "missing.out")); err == nil || !strings.Contains(err.Error(), "open profile") {
		t.Fatalf("missing profile error = %v", err)
	}
	if _, _, err := loadInputs(filepath.Join(t.TempDir(), "missing-packages.tsv"), filepath.Join(t.TempDir(), "missing-profile.out")); err == nil || !strings.Contains(err.Error(), "open package taxonomy") {
		t.Fatalf("loadInputs() missing taxonomy error = %v", err)
	}
	validTaxonomy := commandTestWriteFile(t, t.TempDir(), "packages.tsv", "package\tclass\tcritical\n"+module+"/tools/coverage\ttooling\tfalse\n")
	if _, _, err := loadInputs(validTaxonomy, filepath.Join(t.TempDir(), "missing-profile.out")); err == nil || !strings.Contains(err.Error(), "open profile") {
		t.Fatalf("loadInputs() missing profile error = %v", err)
	}

	t.Run("profile canonical duplicate path aliases", func(t *testing.T) {
		root := commandTestModule(t, "example.test/canonical")
		t.Chdir(root)
		packagesFile := commandTestWriteFile(t, root, "coverage/packages.tsv", strings.Join([]string{
			"package\tclass\tcritical",
			"example.test/canonical/internal/example\tactive\tfalse",
			"",
		}, "\n"))
		profileFile := commandTestWriteFile(t, root, "coverage/profile.out", strings.Join([]string{
			"mode: atomic",
			"example.test/canonical/internal/example/example.go:1.1,2.2 1 1",
			"/tmp/build/example.test/canonical/internal/example/example.go:1.1,2.2 1 0",
			"",
		}, "\n"))

		_, _, err := loadInputs(packagesFile, profileFile)
		if err == nil || !strings.Contains(err.Error(), "duplicate statement range after path normalization") {
			t.Fatalf("loadInputs() error = %v, want canonical duplicate range rejection", err)
		}
	})

	t.Run("profile outside module", func(t *testing.T) {
		root := commandTestModule(t, "example.test/canonical")
		t.Chdir(root)
		packagesFile := commandTestWriteFile(t, root, "coverage/packages.tsv", "package\tclass\tcritical\nexample.test/canonical/internal/example\tactive\tfalse\n")
		profileFile := commandTestWriteFile(t, root, "coverage/profile.out", "mode: atomic\nexample.invalid/internal/example/example.go:1.1,2.2 1 1\n")
		_, _, err := loadInputs(packagesFile, profileFile)
		if err == nil || !strings.Contains(err.Error(), "outside module") {
			t.Fatalf("loadInputs() error = %v, want outside-module rejection", err)
		}
	})

}

func TestLoadProfileSeparatesSyntheticZeroStatementBlocks(t *testing.T) {
	filename := commandTestWriteFile(t, t.TempDir(), "profile.out", strings.Join([]string{
		"mode: atomic",
		"p/a.go:1.2,1.2 0 17",
		"p/a.go:2.1,2.2 0 0",
		"p/a.go:3.1,3.2 1 1",
		"",
	}, "\n"))

	profile, err := loadProfile(filename)
	if err != nil {
		t.Fatalf("loadProfile() error = %v", err)
	}
	if len(profile.Blocks) != 1 {
		t.Fatalf("loadProfile() blocks = %d, want only the executable block", len(profile.Blocks))
	}
	if profile.Blocks[0].Statements != 1 || profile.Blocks[0].Raw != "p/a.go:3.1,3.2 1 1" {
		t.Fatalf("loadProfile() retained block = %#v", profile.Blocks[0])
	}
	if len(profile.EdgeBlocks) != 2 {
		t.Fatalf("loadProfile() edge blocks = %d, want both synthetic blocks", len(profile.EdgeBlocks))
	}
	if profile.EdgeBlocks[0].Statements != 0 || profile.EdgeBlocks[0].Count != 17 ||
		profile.EdgeBlocks[1].Statements != 0 || profile.EdgeBlocks[1].Count != 0 {
		t.Fatalf("loadProfile() edge blocks = %#v, want isolated execution evidence", profile.EdgeBlocks)
	}

	published := filepath.Join(t.TempDir(), "published.out")
	if err := writeProfile(published, profile); err != nil {
		t.Fatalf("writeProfile() error = %v", err)
	}
	contents := commandTestReadFile(t, published)
	if strings.Contains(contents, " 0 17") || strings.Contains(contents, " 0 0") {
		t.Fatalf("writeProfile() published synthetic edge evidence:\n%s", contents)
	}
}

func TestOutputAndClassificationHelpersHandleBoundaryValues(t *testing.T) {
	module := "example.test/helpers"
	active := packageEntry{Name: module + "/active", Class: classActive}
	packages := packageSet{Module: module, Entries: []packageEntry{active}, ByName: map[string]packageEntry{active.Name: active}}
	counts := map[string]statementCount{active.Name: {}, "@total": {}}
	stdout, err := commandTestCaptureStdout(t, func() error {
		return writeCounts("", packages, counts, classActive)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, active.Name+"\t0\t0\t100.00%") || !strings.Contains(stdout, "@total\t0\t0\t100.00%") {
		t.Fatalf("zero-statement report = %q", stdout)
	}

	absolute := "/tmp/build/" + module + "/active/a.go"
	if entry, ok := packageForProfileFile(packages, absolute); !ok || entry != active {
		t.Fatalf("packageForProfileFile(%q) = (%+v, %t)", absolute, entry, ok)
	}
	if _, ok := packageForProfileFile(packages, module+"/missing/a.go"); ok {
		t.Fatal("packageForProfileFile accepted an unclassified package")
	}
	for _, class := range []string{"repository", classActive, classExperimentalTUI, classTooling} {
		if err := validateClass(class); err != nil {
			t.Fatalf("validateClass(%q): %v", class, err)
		}
	}
	if err := validateClass("support"); err == nil {
		t.Fatal("validateClass accepted support as a report class")
	}
	if equalStrings([]string{"same"}, []string{"different"}) || equalStrings([]string{"one"}, []string{"one", "two"}) {
		t.Fatal("equalStrings accepted unequal inputs")
	}
	if got := boundedDiagnostic([]byte("  \n\t"), 8); got != "no diagnostic" {
		t.Fatalf("empty bounded diagnostic = %q", got)
	}
	if got := boundedDiagnostic([]byte(" 0123456789 "), 5); got != "01234..." {
		t.Fatalf("truncated bounded diagnostic = %q", got)
	}
	if got, err := canonicalBuildTags(" \n\t"); err == nil || got != "" || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty canonical tags = %q, error %v", got, err)
	}
}

func commandTestModule(t *testing.T, module string) string {
	t.Helper()
	root := t.TempDir()
	commandTestWriteFile(t, root, "go.mod", "module "+module+"\n\ngo 1.26.1\n")
	return root
}

func commandTestWriteFile(t *testing.T, root, name, contents string) string {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return filename
}

func commandTestReadFile(t *testing.T, filename string) string {
	t.Helper()
	contents, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func commandTestRequireFile(t *testing.T, filename, want string) {
	t.Helper()
	got := commandTestReadFile(t, filename)
	if got != want {
		t.Fatalf("%s contents:\n%s\nwant:\n%s", filename, got, want)
	}
}

func commandTestCaptureStdout(t *testing.T, invoke func() error) (string, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = previous
		_ = reader.Close()
		_ = writer.Close()
	}()

	runErr := invoke()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = previous
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(output), runErr
}
