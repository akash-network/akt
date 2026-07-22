package bootstrap

import "testing"

func TestParseYesNo(t *testing.T) {
	cases := []struct {
		in   string
		def  bool
		want bool
	}{
		{"y\n", false, true},
		{"Y\n", false, true},
		{"yes\n", false, true},
		{"n\n", true, false},
		{"NO\n", true, false},
		{"\n", false, false},
		{"\n", true, true},
		{"maybe\n", false, false},
		{"maybe\n", true, true},
		{"  y  \n", false, true},
	}

	for _, c := range cases {
		if got := parseYesNo(c.in, c.def); got != c.want {
			t.Errorf("parseYesNo(%q, %v) = %v, want %v", c.in, c.def, got, c.want)
		}
	}
}

func TestConsoleOnboardingSkipsWithoutTTY(t *testing.T) {
	// Test processes have no TTY on stdin, so onboarding must be a no-op.
	key, route := consoleOnboarding("prod")
	if key != "" || route {
		t.Errorf("expected non-interactive skip, got key=%q route=%v", key, route)
	}
}
