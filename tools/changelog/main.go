// Command changelog validates and assembles contributor changelog fragments.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type fragment struct {
	name, category, body, raw string
}

func main() {
	if err := run(".", os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "changelog:", err)
		os.Exit(1)
	}
}

func run(root string, args []string) error {
	if len(args) == 0 || len(args) > 2 ||
		(args[0] != "check" && args[0] != "assemble" && args[0] != "release-check") ||
		(len(args) == 2 && args[0] != "check") {
		return errors.New("usage: changelog check [base-commit] | assemble | release-check")
	}
	fragments, err := readFragments(root)
	if err != nil {
		return err
	}
	if args[0] == "release-check" {
		if len(fragments) != 0 {
			return errors.New("pending fragments: run make changelog-assemble and merge the result before tagging")
		}
		return nil
	}
	archivePath := filepath.Join(root, "AICHANGELOG.md")
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		return err
	}
	assembled, err := assemble(archive, fragments)
	if err != nil {
		return err
	}
	if args[0] == "check" {
		if len(args) == 2 {
			return checkPR(root, args[1], archive, fragments)
		}
		return nil
	}
	if !bytes.Equal(assembled, archive) {
		if err := writeArchive(archivePath, assembled); err != nil {
			return err
		}
	}
	for _, entry := range fragments {
		if err := os.Remove(filepath.Join(root, ".changelog", entry.name)); err != nil {
			return fmt.Errorf("archive written; rerun make changelog-assemble to finish cleanup: %w", err)
		}
	}
	return nil
}

func readFragments(root string) ([]fragment, error) {
	entries, err := os.ReadDir(filepath.Join(root, ".changelog"))
	if err != nil {
		return nil, err
	}
	var fragments []fragment
	// ReadDir sorts by filename, which also defines entry order within a category.
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			return nil, fmt.Errorf(".changelog/%s must be a regular file", entry.Name())
		}
		if entry.Name() == "README.md" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(root, ".changelog", entry.Name()))
		if err != nil {
			return nil, err
		}
		parsed, err := parseFragment(entry.Name(), content)
		if err != nil {
			return nil, err
		}
		fragments = append(fragments, parsed)
	}
	return fragments, nil
}

func parseFragment(name string, content []byte) (fragment, error) {
	pattern := regexp.MustCompile(`^[a-z0-9][a-z0-9-]*\.(added|changed|fixed|deprecated|removed|security)\.md$`)
	match := pattern.FindStringSubmatch(name)
	if match == nil {
		return fragment{}, fmt.Errorf(".changelog/%s: expected <unique-slug>.<added|changed|fixed|deprecated|removed|security>.md", name)
	}
	body := strings.TrimSpace(string(content))
	if !strings.HasPrefix(body, "- ") || strings.TrimSpace(strings.TrimPrefix(body, "- ")) == "" {
		return fragment{}, fmt.Errorf(".changelog/%s: write a non-empty Markdown bullet describing the change", name)
	}
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || strings.Contains(line, "<!-- changelog:") ||
			strings.Contains(line, "<!-- /changelog") || strings.HasPrefix(line, "<<<<<<<") ||
			strings.HasPrefix(line, "=======") || strings.HasPrefix(line, ">>>>>>>") {
			return fragment{}, fmt.Errorf(".changelog/%s: headings, assembly markers, and merge conflict markers are not allowed", name)
		}
	}
	return fragment{name: name, category: match[1], body: body, raw: string(content)}, nil
}

func assemble(archive []byte, fragments []fragment) ([]byte, error) {
	const heading = "## Unreleased\n"
	text := string(archive)
	if strings.Count(text, heading) != 1 {
		return nil, errors.New("AICHANGELOG.md must contain exactly one ## Unreleased heading")
	}
	var addition strings.Builder
	for _, category := range []string{"added", "changed", "deprecated", "removed", "fixed", "security"} {
		wroteHeading := false
		for _, entry := range fragments {
			if entry.category != category {
				continue
			}
			marker := "<!-- changelog: " + entry.name + " -->\n"
			block := marker + entry.body + "\n<!-- /changelog -->\n"
			if strings.Contains(text, marker) {
				if !strings.Contains(text, block) {
					return nil, fmt.Errorf("fragment %s was already assembled with different contents; use a new filename", entry.name)
				}
				continue
			}
			if !wroteHeading {
				addition.WriteString("### " + strings.ToUpper(category[:1]) + category[1:] + "\n\n")
				wroteHeading = true
			}
			addition.WriteString(block + "\n")
		}
	}
	if addition.Len() == 0 {
		return archive, nil
	}
	position := strings.Index(text, heading) + len(heading)
	// Keep the entire historical suffix, including its spacing and headings.
	if strings.HasPrefix(text[position:], "\n") {
		position++
	}
	prefix := text[:position]
	if !strings.HasSuffix(prefix, "\n\n") {
		prefix += "\n"
	}
	return []byte(prefix + addition.String() + text[position:]), nil
}

func writeArchive(path string, content []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".aichangelog-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func checkPR(root, base string, archive []byte, fragments []fragment) error {
	resolved, err := gitOutput(root, "rev-parse", "--verify", "--end-of-options", base+"^{commit}")
	if err != nil {
		return err
	}
	base = strings.TrimSpace(string(resolved))
	previous, err := gitOutput(root, "show", base+":AICHANGELOG.md")
	if err != nil {
		return err
	}
	paths, err := gitOutput(root, "ls-tree", "-r", "--name-only", "-z", base, "--", ".changelog/")
	if err != nil {
		return err
	}
	var before []fragment
	for name := range strings.SplitSeq(strings.TrimRight(string(paths), "\x00"), "\x00") {
		if name == "" || name == ".changelog/README.md" {
			continue
		}
		content, err := gitOutput(root, "show", base+":"+name)
		if err != nil {
			return err
		}
		entry, err := parseFragment(strings.TrimPrefix(name, ".changelog/"), content)
		if err != nil {
			return err
		}
		before = append(before, entry)
	}
	if bytes.Equal(previous, archive) {
		current := make(map[string]fragment, len(fragments))
		for _, entry := range fragments {
			if strings.Contains(string(archive), "<!-- changelog: "+entry.name+" -->") {
				return fmt.Errorf("fragment %s was already assembled; use a new filename for this PR", entry.name)
			}
			current[entry.name] = entry
		}
		for _, entry := range before {
			if current[entry.name] != entry {
				return fmt.Errorf("existing fragment %s must not be changed or removed in an ordinary PR", entry.name)
			}
		}
		if len(fragments) <= len(before) {
			return errors.New("add a new .changelog/<unique-slug>.<category>.md fragment for this PR")
		}
		return nil
	}
	expected, err := assemble(previous, before)
	if err != nil {
		return err
	}
	if len(before) == 0 || len(fragments) != 0 || !bytes.Equal(expected, archive) {
		return errors.New("do not edit AICHANGELOG.md in ordinary PRs; add a fragment instead, or run make changelog-assemble in a release-preparation PR")
	}
	changed, err := gitOutput(root, "diff", "--name-only", "--no-renames", "-z", base, "--")
	if err != nil {
		return err
	}
	allowed := map[string]bool{"AICHANGELOG.md": true}
	for _, entry := range before {
		allowed[".changelog/"+entry.name] = true
	}
	for name := range strings.SplitSeq(strings.TrimRight(string(changed), "\x00"), "\x00") {
		if !allowed[name] {
			return fmt.Errorf("release-preparation PR must only assemble the changelog; unrelated change: %s", name)
		}
	}
	return nil
}

func gitOutput(root string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return output, nil
}
