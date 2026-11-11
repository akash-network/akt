package e2e

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeysRecoverFromPipe(t *testing.T) {
	for _, backend := range []string{"test", "file"} {
		t.Run(backend, func(t *testing.T) {
			home := t.TempDir()
			initHome(t, home)
			input := testMnemonic + "\n"
			const password = "test-password"
			if backend == "file" {
				mustRunAkt(t, home, "context", "network", "create", "local",
					"--chain-id", "localnet-1", "--rpc", unreachableNode)
				mustRunAkt(t, home, "context", "create", "dev", "--network", "local")
				input += password + "\n" + password + "\n"
			}
			stdout, stderr, code := runAktStdin(t, home, input,
				"context", "keys", "add", "recovered", "--recover", "--keyring-backend", backend)
			require.Zero(t, code, "stdout: %s; stderr: %s", stdout, stderr)
			require.Empty(t, stderr, "piped recovery must not print a prompt")
			require.Contains(t, stdout, testMnemonicAddr)
			require.NotContains(t, stdout, testMnemonic)
			require.NotContains(t, stdout, password)

			stdout, stderr, code = runAktStdin(t, home, password+"\n",
				"context", "keys", "show", "recovered", "-a", "--keyring-backend", backend)
			require.Zero(t, code, "stdout: %s; stderr: %s", stdout, stderr)
			require.Equal(t, testMnemonicAddr, strings.TrimSpace(stdout))

			// An existing name remains protected, including when --yes is supplied.
			stdout, stderr, code = runAktStdin(t, home, password+"\n"+testMnemonic+"\n",
				"context", "keys", "add", "recovered", "--recover", "--yes", "--keyring-backend", backend)
			require.NotZero(t, code)
			require.Contains(t, stderr, "already exists")
			require.Empty(t, stdout)

			if backend == "file" {
				stdout, stderr, code = runAktStdin(t, home, testMnemonic+"\n"+strings.Repeat("wrong-password\n", 3),
					"context", "keys", "add", "blocked", "--recover", "--index", "1", "--keyring-backend", backend)
				require.NotZero(t, code)
				require.Contains(t, stderr, "incorrect passphrase")
				require.Empty(t, stdout)
				require.NotContains(t, stderr, testMnemonic)
				require.NotContains(t, stderr, "wrong-password")
				_, _, code = runAktStdin(t, home, password+"\n",
					"context", "keys", "show", "blocked", "--keyring-backend", backend)
				require.NotZero(t, code, "failed keyring authentication must not persist a key")
			}
		})
	}
}

func TestKeysRecoverFromPipeFailures(t *testing.T) {
	for _, input := range []string{"", " \n", "not a mnemonic\n", strings.Repeat("abandon ", 11) + "abandon\n"} {
		t.Run(input, func(t *testing.T) {
			home := t.TempDir()
			initHome(t, home)
			stdout, stderr, code := runAktStdin(t, home, input,
				"context", "keys", "add", "recovered", "--recover")
			require.NotZero(t, code)
			require.Empty(t, stdout)
			require.NotContains(t, stderr, "Enter your")
			_, _, code = runAkt(t, home, "context", "keys", "show", "recovered")
			require.NotZero(t, code, "failed recovery must not persist a key")
		})
	}
}
