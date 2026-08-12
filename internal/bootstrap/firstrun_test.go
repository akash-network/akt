package bootstrap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/glyphs"
)

func networks(names ...string) []aktctx.Network {
	out := make([]aktctx.Network, 0, len(names))
	for _, n := range names {
		out = append(out, aktctx.Network{Name: n, ChainID: n + "-1"})
	}

	return out
}

// TestPickInitialContextPrefersATestNetwork is the money test: the wizard's
// active-context cursor decides whether "press enter three times" produces a
// configuration whose next transaction spends real AKT. Mainnet must never be
// the resting position while any safer network was selected.
func TestPickInitialContextPrefersATestNetwork(t *testing.T) {
	cases := []struct {
		name string
		in   []aktctx.Network
		want string
	}{
		{"sandbox wins over everything", networks("mainnet", "testnet", "sandbox"), "sandbox"},
		{"sandbox wins regardless of order", networks("sandbox", "mainnet"), "sandbox"},
		{"testnet when no sandbox", networks("mainnet", "testnet"), "testnet"},
		{"suffixed names still match", networks("mainnet", "testnet-02", "sandbox-2"), "sandbox-2"},
		{"any non-mainnet beats mainnet", networks("mainnet", "edgenet"), "edgenet"},
		{"mainnet only", networks("mainnet"), "mainnet"},
		{"single test network", networks("sandbox"), "sandbox"},
		{"empty", nil, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pickInitialContext(c.in); got != c.want {
				t.Errorf("pickInitialContext(%v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestPickInitialContextNeverReturnsMainnetWhenAnythingElseExists states the
// property directly, so a future rewrite of the preference list cannot quietly
// reintroduce the old mainnet-first behavior.
func TestPickInitialContextNeverReturnsMainnetWhenAnythingElseExists(t *testing.T) {
	sets := [][]aktctx.Network{
		networks("mainnet", "sandbox"),
		networks("mainnet", "testnet"),
		networks("mainnet", "somethingelse"),
		networks("akashnet-mainnet", "sandbox"),
	}

	for _, set := range sets {
		if got := pickInitialContext(set); isMainnetName(got) {
			t.Errorf("pickInitialContext(%v) = %q, a mainnet, with a safer option available", set, got)
		}
	}
}

// TestSelectActiveContextFallsBackToTheSafeChoice covers the arm taken when
// raw mode is unavailable (a pipe, a CI runner): the prompt cannot render, so
// the value returned must still be the one the cursor would have started on.
func TestSelectActiveContextFallsBackToTheSafeChoice(t *testing.T) {
	restoreNonTTYStdin(t)

	if got := selectActiveContext(networks("mainnet", "testnet", "sandbox")); got != "sandbox" {
		t.Errorf("selectActiveContext fallback = %q, want sandbox", got)
	}

	if got := selectActiveContext(nil); got != "" {
		t.Errorf("selectActiveContext(nil) = %q, want empty", got)
	}
}

// restoreNonTTYStdin points os.Stdin at a pipe so term.MakeRaw fails
// deterministically, whatever the test runner attached.
func restoreNonTTYStdin(t *testing.T) {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	prev := os.Stdin
	os.Stdin = r

	t.Cleanup(func() {
		os.Stdin = prev
		_ = w.Close()
		_ = r.Close()
	})
}

// TestActiveContextPromptRestsOnTheSafeRow asserts the frame the user
// actually sees: the cursor glyph and the checked box sit on the test
// network, and the mainnet row says out loud what selecting it costs.
func TestActiveContextPromptRestsOnTheSafeRow(t *testing.T) {
	nets := []aktctx.Network{
		{Name: "mainnet", ChainID: "akashnet-2"},
		{Name: "testnet", ChainID: "testnet-02"},
		{Name: "sandbox", ChainID: "sandbox-2"},
	}

	options := make([]selectOption, 0, len(nets))
	initial := 0

	preferred := pickInitialContext(nets)
	for i, n := range nets {
		if n.Name == preferred {
			initial = i
		}

		options = append(options, selectOption{value: n.Name, label: n.Name, desc: networkRisk(n.Name)})
	}

	frame := renderSingleSelect("Select the active context", options, initial)

	rows := strings.Split(frame, "\r\n")

	var mainnetRow, sandboxRow string

	for _, row := range rows {
		switch {
		case strings.Contains(row, "mainnet"):
			mainnetRow = row
		case strings.Contains(row, "sandbox"):
			sandboxRow = row
		}
	}

	if !strings.Contains(sandboxRow, glyphs.G().Cursor) {
		t.Errorf("cursor is not on the sandbox row:\n%q", sandboxRow)
	}

	if strings.Contains(mainnetRow, glyphs.G().Cursor) {
		t.Errorf("cursor rests on the mainnet row:\n%q", mainnetRow)
	}

	if !strings.Contains(mainnetRow, "spend real AKT") {
		t.Errorf("mainnet row does not state its cost:\n%q", mainnetRow)
	}
}

func TestNetworkRisk(t *testing.T) {
	cases := map[string]string{
		"mainnet":    "live network - transactions spend real AKT",
		"sandbox-2":  "test network - tokens have no value",
		"testnet-02": "test network - tokens have no value",
		"unknown":    "",
	}

	for name, want := range cases {
		if got := networkRisk(name); got != want {
			t.Errorf("networkRisk(%q) = %q, want %q", name, got, want)
		}
	}
}

// TestDestinationLinesNameTheRootAndItsOverrides covers the announcement that
// runs before the first prompt. A user who Ctrl-Cs mid-wizard, or who has
// AKT_HOME set by another tool, learns the destination only from these lines.
func TestDestinationLinesNameTheRootAndItsOverrides(t *testing.T) {
	root := "/tmp/example/akt-home"
	body := strings.Join(destinationLines(root), "\n")

	for _, want := range []string{
		root,
		aktctx.ConfigPath(root),
		"--home",
		"AKT_HOME",
		"XDG_CONFIG_HOME",
		"Nothing is written",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("destination announcement omits %q:\n%s", want, body)
		}
	}
}

func TestDestinationAndSummaryWritersPreserveLineContracts(t *testing.T) {
	root := "/tmp/example/akt-home"

	var destination strings.Builder
	writeDestination(&destination, root)
	for _, line := range destinationLines(root) {
		if !strings.Contains(destination.String(), line+"\n") {
			t.Errorf("destination writer omitted line %q:\n%s", line, destination.String())
		}
	}

	var summary strings.Builder
	writeSummary(&summary, root, "sandbox", "sandbox-2", "file")
	if !strings.Contains(summary.String(), "Setup complete") {
		t.Errorf("summary writer omitted heading:\n%s", summary.String())
	}
	for _, line := range summaryLines(root, "sandbox", "sandbox-2", "file") {
		if !strings.Contains(summary.String(), line+"\n") {
			t.Errorf("summary writer omitted line %q:\n%s", line, summary.String())
		}
	}
}

// TestSummaryLinesNameEveryLocation covers the closing summary. Before this,
// the only path the wizard ever printed was config.yaml, so the store, action
// log, and keyring were left for the user to guess at.
func TestSummaryLinesNameEveryLocation(t *testing.T) {
	root := "/tmp/example/akt-home"
	body := strings.Join(summaryLines(root, "sandbox", "sandbox-2", "file"), "\n")

	for _, want := range []string{
		aktctx.ConfigPath(root),
		"sandbox (sandbox-2)",
		aktctx.ContextDir(root, "sandbox"),
		aktctx.StoreDir(root, "sandbox"),
		aktctx.ActionLogPath(root, "sandbox"),
		aktctx.KeyringDir(root, aktctx.Keyring{Name: defaultKeyringName}),
		// No context is given a default-account, so the summary has to say
		// what to do about it.
		"akt context keys add <name>",
		"--recover",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("summary omits %q:\n%s", want, body)
		}
	}

	// SaveConfig creates only the config root. Presenting the rest as
	// existing directories would send the user looking for them.
	for _, dir := range []string{
		aktctx.ContextDir(root, "sandbox"),
		aktctx.StoreDir(root, "sandbox"),
	} {
		if !strings.Contains(body, dir+pendingSuffix) {
			t.Errorf("summary presents %q as already created:\n%s", dir, body)
		}
	}
}

// TestKeyringLocationMatchesTheBackend covers the one summary line whose
// answer is not a path: the os backend stores nothing on disk, so naming a
// directory for it would be wrong.
func TestKeyringLocationMatchesTheBackend(t *testing.T) {
	root := "/tmp/example/akt-home"

	osLine := keyringLocation(root, "os")
	if !strings.Contains(osLine, aktctx.KeyringServiceName) {
		t.Errorf("os keyring location = %q, want the service name", osLine)
	}

	if strings.Contains(osLine, root) {
		t.Errorf("os keyring location = %q, must not name a directory", osLine)
	}

	for _, backend := range []string{"file", "test"} {
		line := keyringLocation(root, backend)
		if !strings.Contains(line, aktctx.KeyringDir(root, aktctx.Keyring{Name: defaultKeyringName})) {
			t.Errorf("%s keyring location = %q, want the keyring directory", backend, line)
		}
	}
}

func TestChainIDOf(t *testing.T) {
	nets := networks("mainnet", "sandbox")

	if got := chainIDOf(nets, "sandbox"); got != "sandbox-1" {
		t.Errorf("chainIDOf = %q, want sandbox-1", got)
	}

	if got := chainIDOf(nets, "absent"); got != "" {
		t.Errorf("chainIDOf(absent) = %q, want empty", got)
	}
}

func TestSingleSelectFallbackSkipsUnavailableOptions(t *testing.T) {
	restoreNonTTYStdin(t)

	tests := []struct {
		name    string
		options []selectOption
		initial int
		want    string
	}{
		{name: "empty"},
		{
			name: "invalid initial uses first",
			options: []selectOption{
				{value: "first", label: "first"},
				{value: "second", label: "second"},
			},
			initial: 99,
			want:    "first",
		},
		{
			name: "unavailable initial advances",
			options: []selectOption{
				{value: "missing", label: "missing", unavailable: true},
				{value: "usable", label: "usable"},
			},
			want: "usable",
		},
		{
			name: "all unavailable",
			options: []selectOption{
				{value: "one", label: "one", unavailable: true},
				{value: "two", label: "two", unavailable: true},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := singleSelect("Choose", tc.options, tc.initial); got != tc.want {
				t.Errorf("singleSelect = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRenderSingleSelectShowsUnavailableAndWideLabels(t *testing.T) {
	frame := renderSingleSelect("Choose", []selectOption{
		{value: "off", label: "not-installed", desc: "unavailable here", unavailable: true},
		{value: "on", label: "usable", desc: "ready"},
	}, 1)

	if !strings.Contains(frame, "not-installed") || !strings.Contains(frame, "unavailable here") {
		t.Errorf("unavailable option is not visible:\n%s", frame)
	}
	rows := strings.Split(frame, "\r\n")
	for _, row := range rows {
		if strings.Contains(row, "not-installed") && strings.Contains(row, glyphs.G().Cursor) {
			t.Errorf("unavailable row carried the cursor: %q", row)
		}
		if strings.Contains(row, "usable") && !strings.Contains(row, glyphs.G().Cursor) {
			t.Errorf("selected usable row lacked the cursor: %q", row)
		}
	}
}

// --- Legacy home detection ---

// writeLegacyFixture builds a fake home directory and returns it.
func writeLegacyFixture(t *testing.T, certs []string, keyringFile, keyringTest bool) string {
	t.Helper()

	home := t.TempDir()
	legacy := filepath.Join(home, legacyDirName)

	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatalf("mkdir legacy home: %v", err)
	}

	for _, name := range certs {
		if err := os.WriteFile(filepath.Join(legacy, name), []byte("-----BEGIN CERTIFICATE-----\n"), 0o600); err != nil {
			t.Fatalf("write cert fixture: %v", err)
		}
	}

	if keyringFile {
		if err := os.MkdirAll(filepath.Join(legacy, legacyKeyringFileDir), 0o700); err != nil {
			t.Fatalf("mkdir keyring-file: %v", err)
		}
	}

	if keyringTest {
		if err := os.MkdirAll(filepath.Join(legacy, legacyKeyringTestDir), 0o700); err != nil {
			t.Fatalf("mkdir keyring-test: %v", err)
		}
	}

	return home
}

func TestDetectLegacyHome(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		l := detectLegacyHome(t.TempDir())
		if l.Exists || l.Found() {
			t.Errorf("empty home reported a legacy install: %+v", l)
		}
	})

	t.Run("empty home directory is not a finding", func(t *testing.T) {
		// A bare ~/.akash with nothing recoverable in it must stay silent:
		// the notice is entirely about keys and certificates.
		l := detectLegacyHome(writeLegacyFixture(t, nil, false, false))
		if !l.Exists {
			t.Fatal("existing ~/.akash must be detected")
		}

		if l.Found() {
			t.Errorf("empty ~/.akash must not produce a notice: %+v", l)
		}
	})

	t.Run("certificates only", func(t *testing.T) {
		l := detectLegacyHome(writeLegacyFixture(t, []string{"akash1abc.pem", "akash1def.pem"}, false, false))
		if !l.Found() {
			t.Fatalf("certs must be a finding: %+v", l)
		}

		sort.Strings(l.Certs)
		if len(l.Certs) != 2 || l.Certs[0] != "akash1abc.pem" || l.Certs[1] != "akash1def.pem" {
			t.Errorf("certs = %v", l.Certs)
		}

		if l.KeyringFile || l.KeyringTest {
			t.Errorf("no keyring directories exist, got %+v", l)
		}
	})

	t.Run("keyrings only", func(t *testing.T) {
		l := detectLegacyHome(writeLegacyFixture(t, nil, true, true))
		if !l.Found() || !l.KeyringFile || !l.KeyringTest {
			t.Errorf("keyring directories must be a finding: %+v", l)
		}

		if len(l.Certs) != 0 {
			t.Errorf("certs = %v, want none", l.Certs)
		}
	})

	t.Run("no home directory", func(t *testing.T) {
		if l := detectLegacyHome(""); l.Exists || l.Found() {
			t.Errorf("empty home dir must yield nothing, got %+v", l)
		}
	})

	t.Run("a file named .akash is not a home", func(t *testing.T) {
		home := t.TempDir()
		if err := os.WriteFile(filepath.Join(home, legacyDirName), []byte("x"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}

		if l := detectLegacyHome(home); l.Exists {
			t.Errorf("a regular file must not count as a legacy home: %+v", l)
		}
	})
}

// TestDetectLegacyHomeNeverMutates is the promise the notice itself makes.
// Detection is os.Stat and a filename glob; anything more — reading a PEM,
// opening a keyring, "helpfully" moving a file — would make the printed
// assurance a lie.
func TestDetectLegacyHomeNeverMutates(t *testing.T) {
	home := writeLegacyFixture(t, []string{"akash1abc.pem"}, true, true)

	before := snapshotTree(t, home)

	l := detectLegacyHome(home)
	if !l.Found() {
		t.Fatal("fixture must be detected")
	}

	// Rendering the notice must be read-only too.
	if lines := legacyNoticeLines(l, "/tmp/akt-home", "os"); len(lines) == 0 {
		t.Fatal("a found legacy home must produce a notice")
	}

	after := snapshotTree(t, home)

	if before != after {
		t.Errorf("legacy home was modified.\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// snapshotTree renders a stable description of every entry under root: path,
// mode, size, modification time, and a content hash for regular files.
func snapshotTree(t *testing.T, root string) string {
	t.Helper()

	var b strings.Builder

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		sum := ""
		if info.Mode().IsRegular() {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}

			h := sha256.Sum256(data)
			sum = hex.EncodeToString(h[:])
		}

		fmt.Fprintf(&b, "%s mode=%s size=%d mtime=%d sha=%s\n",
			rel, info.Mode(), info.Size(), info.ModTime().UnixNano(), sum)

		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	return b.String()
}

// TestLegacyNoticeSaysWhatDoesNotCarryOverAndHow covers the notice body. Each
// assertion is a fact the user cannot get anywhere else: that akt does not read
// the legacy home, that the account is recoverable, that the published
// certificate survives, and which remedy costs a transaction.
func TestLegacyNoticeSaysWhatDoesNotCarryOverAndHow(t *testing.T) {
	home := writeLegacyFixture(t, []string{"akash1abc.pem"}, true, false)
	l := detectLegacyHome(home)

	cfgRoot := "/tmp/example/akt-home"
	body := strings.Join(legacyNoticeLines(l, cfgRoot, "os"), "\n")

	for _, want := range []string{
		l.Path,
		"Nothing in it is carried over automatically",
		"there is no import",
		"never reads, modifies, or deletes",
		filepath.Join(l.Path, "akash1abc.pem"),
		filepath.Join(l.Path, legacyKeyringFileDir),
		// The mismatch, named on both sides.
		`"akash"`,
		`"akt"`,
		aktctx.KeyringDir(cfgRoot, aktctx.Keyring{Name: defaultKeyringName}),
		cfgRoot,
		// Recovery, verbatim and runnable.
		"akt context keys add <name> --recover",
		"akt query cert list <address>",
		"akt tx cert generate client",
		"akt tx cert publish client",
		// The free path and the paid path, distinguished.
		"no re-publish, no transaction",
		"broadcasts a transaction and pays a fee",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("legacy notice omits %q:\n%s", want, body)
		}
	}

	// A keyring directory that does not exist must not be listed.
	if strings.Contains(body, filepath.Join(l.Path, legacyKeyringTestDir)) {
		t.Errorf("notice lists a keyring-test directory that does not exist:\n%s", body)
	}
}

func TestWriteLegacyNoticeRendersFinding(t *testing.T) {
	home := writeLegacyFixture(t, []string{"akash1abc.pem"}, true, true)
	finding := detectLegacyHome(home)

	var output strings.Builder
	writeLegacyNotice(&output, finding, "/tmp/akt-home", "file")

	if !strings.Contains(output.String(), "Existing Akash CLI configuration") {
		t.Fatalf("legacy notice heading missing:\n%s", output.String())
	}
	for _, line := range legacyNoticeLines(finding, "/tmp/akt-home", "file") {
		if !strings.Contains(output.String(), line+"\n") {
			t.Errorf("legacy notice writer omitted line %q", line)
		}
	}
}

// TestLegacyNoticeSilentWithoutAFinding keeps the wizard quiet for the common
// case: no legacy install, or a stray empty directory.
func TestLegacyNoticeSilentWithoutAFinding(t *testing.T) {
	cases := map[string]legacyHome{
		"absent":     detectLegacyHome(t.TempDir()),
		"empty home": detectLegacyHome(writeLegacyFixture(t, nil, false, false)),
	}

	for name, l := range cases {
		if lines := legacyNoticeLines(l, "/tmp/akt-home", "os"); len(lines) != 0 {
			t.Errorf("%s produced a notice:\n%s", name, strings.Join(lines, "\n"))
		}

		var b strings.Builder

		writeLegacyNotice(&b, l, "/tmp/akt-home", "os")

		if b.Len() != 0 {
			t.Errorf("%s wrote output: %q", name, b.String())
		}
	}
}
