package bootstrap

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	aktctx "pkg.akt.dev/akt/internal/context"
)

const (
	// legacyDirName is the home directory of the legacy akash and
	// provider-services CLIs, relative to the user's home directory.
	legacyDirName = ".akash"

	// legacyKeyringFileDir and legacyKeyringTestDir are the legacy CLI's
	// file- and test-backend keyring directories inside that home.
	legacyKeyringFileDir = "keyring-file"
	legacyKeyringTestDir = "keyring-test"
)

// legacyHome describes what the first-run wizard found in the legacy akash
// CLI home (SPEC §1.12).
//
// Detection is read-only and bounded to os.Stat and a filename glob. akt
// never reads, moves, modifies, or deletes anything under the legacy home,
// and never opens a legacy keyring — the notice built from this struct says
// so, so the code must not contradict it.
type legacyHome struct {
	// Path is the directory that was probed, set whether or not it exists.
	Path string

	// Exists reports whether Path is a directory.
	Exists bool

	// Certs holds the base names of the *.pem files directly inside Path.
	// Only names are collected; no PEM file is ever opened.
	Certs []string

	// KeyringFile and KeyringTest report the presence of the legacy
	// file- and test-backend keyring directories.
	KeyringFile bool
	KeyringTest bool
}

// Found reports whether the legacy home holds anything worth telling the user
// about. A bare or unrelated ~/.akash does not qualify: the notice is entirely
// about keys and certificates, so printing it with nothing to carry over would
// be noise.
func (l legacyHome) Found() bool {
	return l.Exists && (len(l.Certs) > 0 || l.KeyringFile || l.KeyringTest)
}

// detectLegacyHome probes homeDir for a legacy akash CLI home. An empty
// homeDir, or one that cannot be probed, yields a zero result rather than an
// error: a first run must never fail because of what is or is not in an
// unrelated directory.
func detectLegacyHome(homeDir string) legacyHome {
	l := legacyHome{}

	if homeDir == "" {
		return l
	}

	l.Path = filepath.Join(homeDir, legacyDirName)

	if !isDir(l.Path) {
		return l
	}

	l.Exists = true

	// Filenames only. Reading a PEM would be unnecessary — the notice tells
	// the user where to copy it, not what is in it — and would break the
	// promise the notice itself makes.
	if matches, err := filepath.Glob(filepath.Join(l.Path, "*.pem")); err == nil {
		for _, m := range matches {
			l.Certs = append(l.Certs, filepath.Base(m))
		}
	}

	l.KeyringFile = isDir(filepath.Join(l.Path, legacyKeyringFileDir))
	l.KeyringTest = isDir(filepath.Join(l.Path, legacyKeyringTestDir))

	return l
}

// isDir reports whether path exists and is a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)

	return err == nil && info.IsDir()
}

// legacyNoticeLines renders the legacy-home notice (SPEC §1.12) as plain
// text. It returns nil when there is nothing to report.
//
// The notice is deliberately not an importer. It states what was found, why
// none of it is visible to akt, and the exact commands that reproduce the
// account and the certificate — including which of them costs a transaction.
func legacyNoticeLines(l legacyHome, cfgRoot, backend string) []string {
	if !l.Found() {
		return nil
	}

	kr := aktctx.Keyring{Name: defaultKeyringName}

	lines := []string{
		"An existing Akash CLI home was found at " + l.Path,
		"Nothing in it is carried over automatically, and there is no import",
		"command. akt never reads, modifies, or deletes anything inside it.",
		"",
		"Found:",
	}

	for _, name := range l.Certs {
		lines = append(lines, "  "+filepath.Join(l.Path, name)+"  (client certificate)")
	}

	if l.KeyringFile {
		lines = append(lines, "  "+filepath.Join(l.Path, legacyKeyringFileDir)+"  (file keyring)")
	}

	if l.KeyringTest {
		lines = append(lines, "  "+filepath.Join(l.Path, legacyKeyringTestDir)+"  (test keyring)")
	}

	lines = append(lines,
		"",
		"Why none of it is visible to akt:",
		"  os keyring    the legacy CLI stores entries under the system keyring",
		fmt.Sprintf("                service %q; akt uses %q",
			aktctx.LegacyKeyringServiceName, aktctx.KeyringServiceName),
		"  file keyring  akt keeps keyrings in "+aktctx.KeyringDir(cfgRoot, kr),
		"  certificates  akt looks for <address>.pem in "+cfgRoot,
		"",
		"Recovering your account (same address, same on-chain identity):",
		"  akt context keys add <name> --recover",
		"",
		"Your published certificate is still valid. It is on-chain state, not a",
		"local file. Confirm it with:",
		"  akt query cert list <address>",
		"",
		"Only the local half is in the wrong place. Once the key is in akt's",
		"keyring, copy it across:",
		"  cp "+filepath.Join(l.Path, "<address>.pem")+" "+filepath.Join(cfgRoot, "<address>.pem"),
		"",
		"The PEM password is derived from a keyring signature over the address, so",
		"the copy works as-is: no re-publish, no transaction. Regenerating instead",
		"broadcasts a transaction and pays a fee:",
		"  akt tx cert generate client",
		"  akt tx cert publish client",
	)

	if backend == "os" {
		lines = append(lines,
			"",
			"You chose the os keyring backend, so a recovered key is written to the",
			fmt.Sprintf("system keyring under service %q. The legacy %q entries are left",
				aktctx.KeyringServiceName, aktctx.LegacyKeyringServiceName),
			"untouched.")
	}

	return lines
}

// writeLegacyNotice prints the legacy-home notice to w. It writes nothing
// when there is nothing to report.
func writeLegacyNotice(w io.Writer, l legacyHome, cfgRoot, backend string) {
	lines := legacyNoticeLines(l, cfgRoot, backend)
	if len(lines) == 0 {
		return
	}

	fmt.Fprintln(w, ansiBold+"Existing Akash CLI configuration"+ansiReset)
	fmt.Fprintln(w)

	for _, line := range lines {
		fmt.Fprintln(w, line)
	}

	fmt.Fprintln(w)
}
