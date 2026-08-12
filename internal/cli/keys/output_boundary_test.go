package keys

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

type keysFaultWriter struct {
	err error
}

func (w keysFaultWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type keysSequencedWriter struct {
	err    error
	failAt int
	writes int
}

type keysMatchingWriter struct {
	err    error
	match  string
	writes []string
}

func (w *keysMatchingWriter) Write(p []byte) (int, error) {
	w.writes = append(w.writes, string(p))
	if strings.Contains(string(p), w.match) {
		return 0, w.err
	}

	return len(p), nil
}

func (w *keysSequencedWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.failAt {
		return 0, w.err
	}

	return len(p), nil
}

func executeKeysBoundaryCommand(
	t *testing.T,
	kr sdkkeyring.Keyring,
	input string,
	stdout io.Writer,
	stderr io.Writer,
	args ...string,
) error {
	t.Helper()

	cmd := Commands(func() (sdkkeyring.Keyring, error) { return kr, nil }, nil)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetIn(strings.NewReader(input))
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)

	return cmd.Execute()
}

func mnemonicSource(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "mnemonic.txt")
	contents := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write mnemonic source: %v", err)
	}

	return path
}

func TestKeyAddPromptFailuresDoNotCreateKeys(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		args      func(*testing.T) []string
		stderr    func(error) io.Writer
		wantError string
	}{
		{
			name: "recovery destination",
			args: func(*testing.T) []string {
				return []string{"add", "recovered", "--recover"}
			},
			stderr: func(writeErr error) io.Writer { return keysFaultWriter{err: writeErr} },
		},
		{
			name: "recovery empty input",
			args: func(*testing.T) []string {
				return []string{"add", "recovered", "--recover"}
			},
			stderr:    func(error) io.Writer { return &bytes.Buffer{} },
			wantError: "read mnemonic",
		},
		{
			name: "BIP39 destination",
			args: func(t *testing.T) []string {
				return []string{"add", "recovered", "--recover", "--source", mnemonicSource(t), "--interactive"}
			},
			stderr: func(writeErr error) io.Writer { return keysFaultWriter{err: writeErr} },
		},
		{
			name: "BIP39 empty input",
			args: func(t *testing.T) []string {
				return []string{"add", "recovered", "--recover", "--source", mnemonicSource(t), "--interactive"}
			},
			stderr:    func(error) io.Writer { return &bytes.Buffer{} },
			wantError: "read BIP39 passphrase",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
			writeErr := errors.New("key prompt unavailable")
			err := executeKeysBoundaryCommand(
				t,
				kr,
				test.input,
				&bytes.Buffer{},
				test.stderr(writeErr),
				test.args(t)...,
			)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("add error = %v, want %q", err, test.wantError)
				}
			} else if !errors.Is(err, writeErr) {
				t.Fatalf("add error = %v, want destination error", err)
			}
			if _, err := kr.Key("recovered"); err == nil {
				t.Fatal("a failed key prompt created the key")
			}
		})
	}
}

func TestKeyDeletePromptControlsMutation(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		stderr     func(error) io.Writer
		wantError  string
		wantExists bool
	}{
		{
			name:       "confirmation destination",
			stderr:     func(writeErr error) io.Writer { return keysFaultWriter{err: writeErr} },
			wantExists: true,
		},
		{
			name:       "empty input",
			stderr:     func(error) io.Writer { return &bytes.Buffer{} },
			wantError:  "read delete confirmation",
			wantExists: true,
		},
		{
			name:  "cancellation destination",
			input: "n\n",
			stderr: func(writeErr error) io.Writer {
				return &keysSequencedWriter{err: writeErr, failAt: 2}
			},
			wantExists: true,
		},
		{
			name:  "confirmed",
			input: "yes\n",
			stderr: func(error) io.Writer {
				return &bytes.Buffer{}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kr := testKeyring(t)
			writeErr := errors.New("delete prompt unavailable")
			err := executeKeysBoundaryCommand(
				t,
				kr,
				test.input,
				&bytes.Buffer{},
				test.stderr(writeErr),
				"delete", "alice",
			)

			switch {
			case test.wantError != "":
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("delete error = %v, want %q", err, test.wantError)
				}
			case test.wantExists:
				if !errors.Is(err, writeErr) {
					t.Fatalf("delete error = %v, want destination error", err)
				}
			case err != nil:
				t.Fatalf("delete key: %v", err)
			}

			_, keyErr := kr.Key("alice")
			if test.wantExists && keyErr != nil {
				t.Fatalf("a failed confirmation removed the key: %v", keyErr)
			}
			if !test.wantExists && keyErr == nil {
				t.Fatal("a confirmed deletion left the key behind")
			}
		})
	}
}

func TestKeyArmorPromptsRejectBrokenIOBeforeMutation(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		args      func(*testing.T) []string
		stderr    func(error) io.Writer
		wantError string
	}{
		{
			name: "export destination",
			args: func(*testing.T) []string { return []string{"export", "alice"} },
			stderr: func(writeErr error) io.Writer {
				return keysFaultWriter{err: writeErr}
			},
		},
		{
			name:      "export empty input",
			args:      func(*testing.T) []string { return []string{"export", "alice"} },
			stderr:    func(error) io.Writer { return &bytes.Buffer{} },
			wantError: "read export passphrase",
		},
		{
			name: "import destination",
			args: func(t *testing.T) []string {
				path := filepath.Join(t.TempDir(), "key.armor")
				if err := os.WriteFile(path, []byte("not reached"), 0o600); err != nil {
					t.Fatalf("write armor fixture: %v", err)
				}
				return []string{"import", "restored", path}
			},
			stderr: func(writeErr error) io.Writer {
				return keysFaultWriter{err: writeErr}
			},
		},
		{
			name:  "import empty input",
			input: "",
			args: func(t *testing.T) []string {
				path := filepath.Join(t.TempDir(), "key.armor")
				if err := os.WriteFile(path, []byte("not reached"), 0o600); err != nil {
					t.Fatalf("write armor fixture: %v", err)
				}
				return []string{"import", "restored", path}
			},
			stderr:    func(error) io.Writer { return &bytes.Buffer{} },
			wantError: "read import passphrase",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kr := testKeyring(t)
			writeErr := errors.New("passphrase prompt unavailable")
			err := executeKeysBoundaryCommand(
				t,
				kr,
				test.input,
				&bytes.Buffer{},
				test.stderr(writeErr),
				test.args(t)...,
			)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("armor command error = %v, want %q", err, test.wantError)
				}
			} else if !errors.Is(err, writeErr) {
				t.Fatalf("armor command error = %v, want destination error", err)
			}
			if _, err := kr.Key("restored"); err == nil {
				t.Fatal("failed import prompt created a key")
			}
		})
	}
}

func TestEmptyKeyListPrettyOutputHonorsCommandWriter(t *testing.T) {
	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)

	t.Run("helpful empty state", func(t *testing.T) {
		var stdout bytes.Buffer
		if err := executeKeysBoundaryCommand(t, kr, "", &stdout, &bytes.Buffer{}, "list"); err != nil {
			t.Fatalf("list keys: %v", err)
		}
		if !strings.Contains(stdout.String(), "akt context keys add") {
			t.Fatalf("empty key list = %q, want next-step hint", stdout.String())
		}
	})

	t.Run("destination failure", func(t *testing.T) {
		writeErr := errors.New("key-list output unavailable")
		err := executeKeysBoundaryCommand(t, kr, "", keysFaultWriter{err: writeErr}, &bytes.Buffer{}, "list")
		if !errors.Is(err, writeErr) {
			t.Fatalf("list error = %v, want destination error", err)
		}
	})
}

func TestKeyExportWritesUsableArmorAndPropagatesOutputFailure(t *testing.T) {
	kr := testKeyring(t)

	t.Run("usable armor", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if err := executeKeysBoundaryCommand(t, kr, "secret-passphrase\n", &stdout, &stderr, "export", "alice"); err != nil {
			t.Fatalf("export key: %v", err)
		}
		if !strings.Contains(stderr.String(), "passphrase") {
			t.Fatalf("export prompt = %q", stderr.String())
		}
		armor := strings.TrimSpace(stdout.String())
		if !strings.Contains(armor, "BEGIN TENDERMINT PRIVATE KEY") {
			t.Fatalf("export output is not encrypted armor: %q", armor)
		}

		restored := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
		if err := restored.ImportPrivKey("restored", armor, "secret-passphrase"); err != nil {
			t.Fatalf("import exported armor: %v", err)
		}
		originalRecord, err := kr.Key("alice")
		if err != nil {
			t.Fatalf("read original key: %v", err)
		}
		restoredRecord, err := restored.Key("restored")
		if err != nil {
			t.Fatalf("read restored key: %v", err)
		}
		originalAddress, _ := originalRecord.GetAddress()
		restoredAddress, _ := restoredRecord.GetAddress()
		if !originalAddress.Equals(restoredAddress) {
			t.Fatalf("restored address = %s, want %s", restoredAddress, originalAddress)
		}
	})

	t.Run("destination failure", func(t *testing.T) {
		writeErr := errors.New("armor output unavailable")
		err := executeKeysBoundaryCommand(
			t,
			kr,
			"secret-passphrase\n",
			keysFaultWriter{err: writeErr},
			&bytes.Buffer{},
			"export", "alice",
		)
		if !errors.Is(err, writeErr) {
			t.Fatalf("export error = %v, want destination error", err)
		}
	})
}

func TestKeyImportCreatesEquivalentKeyAndPropagatesSuccessNoticeFailure(t *testing.T) {
	source := testKeyring(t)
	armor, err := source.ExportPrivKeyArmor("alice", "secret-passphrase")
	if err != nil {
		t.Fatalf("export fixture key: %v", err)
	}
	path := filepath.Join(t.TempDir(), "alice.armor")
	if err := os.WriteFile(path, []byte(armor), 0o600); err != nil {
		t.Fatalf("write armor fixture: %v", err)
	}
	originalRecord, err := source.Key("alice")
	if err != nil {
		t.Fatalf("read fixture key: %v", err)
	}
	originalAddress, _ := originalRecord.GetAddress()

	t.Run("equivalent key", func(t *testing.T) {
		restored := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
		var diagnostics bytes.Buffer
		if err := executeKeysBoundaryCommand(
			t,
			restored,
			"secret-passphrase\n",
			&bytes.Buffer{},
			&diagnostics,
			"import", "restored", path,
		); err != nil {
			t.Fatalf("import key: %v", err)
		}
		if !strings.Contains(diagnostics.String(), `Key "restored" imported successfully.`) {
			t.Fatalf("import diagnostics = %q", diagnostics.String())
		}
		restoredRecord, err := restored.Key("restored")
		if err != nil {
			t.Fatalf("read imported key: %v", err)
		}
		restoredAddress, _ := restoredRecord.GetAddress()
		if !originalAddress.Equals(restoredAddress) {
			t.Fatalf("imported address = %s, want %s", restoredAddress, originalAddress)
		}
	})

	t.Run("success notice destination failure", func(t *testing.T) {
		restored := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
		writeErr := errors.New("import success notice unavailable")
		diagnostics := &keysMatchingWriter{err: writeErr, match: "imported successfully"}
		err := executeKeysBoundaryCommand(
			t,
			restored,
			"secret-passphrase\n",
			&bytes.Buffer{},
			diagnostics,
			"import", "restored", path,
		)
		if !errors.Is(err, writeErr) {
			t.Fatalf("import error = %v, want destination error", err)
		}
		if _, err := restored.Key("restored"); err != nil {
			t.Fatalf("destination failure rolled back a completed import: %v", err)
		}
		written := strings.Join(diagnostics.writes, "")
		if !strings.Contains(written, "decrypt the key") || !strings.Contains(written, "imported successfully") {
			t.Fatalf("import diagnostics = %q, want prompt and success notice", written)
		}
	})
}

func TestMnemonicAndPrettyParseHonorCommandOutput(t *testing.T) {
	t.Run("mnemonic", func(t *testing.T) {
		var stdout bytes.Buffer
		if err := executeKeysBoundaryCommand(t, nil, "", &stdout, &bytes.Buffer{}, "mnemonic"); err != nil {
			t.Fatalf("generate mnemonic: %v", err)
		}
		if fields := strings.Fields(stdout.String()); len(fields) != 24 {
			t.Fatalf("mnemonic contains %d words, want 24: %q", len(fields), stdout.String())
		}

		writeErr := errors.New("mnemonic output unavailable")
		if err := executeKeysBoundaryCommand(t, nil, "", keysFaultWriter{err: writeErr}, &bytes.Buffer{}, "mnemonic"); !errors.Is(err, writeErr) {
			t.Fatalf("mnemonic error = %v, want destination error", err)
		}
	})

	t.Run("pretty address parse", func(t *testing.T) {
		var stdout bytes.Buffer
		if err := executeKeysBoundaryCommand(t, nil, "", &stdout, &bytes.Buffer{}, "parse", keysTestAddress); err != nil {
			t.Fatalf("parse address: %v", err)
		}
		for _, want := range []string{"Format:  bech32", "HRP:     akash", "Hex:", "cosmos:", "osmo:"} {
			if !strings.Contains(stdout.String(), want) {
				t.Errorf("parse output %q does not contain %q", stdout.String(), want)
			}
		}

		writeErr := errors.New("parse output unavailable")
		if err := executeKeysBoundaryCommand(t, nil, "", keysFaultWriter{err: writeErr}, &bytes.Buffer{}, "parse", keysTestAddress); !errors.Is(err, writeErr) {
			t.Fatalf("parse error = %v, want destination error", err)
		}
	})
}
