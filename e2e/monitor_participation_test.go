package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/creack/pty"
)

// This oracle reads the node's own exact power fraction; it does not use the
// monitor's event tracker, consensus parser, or rounded percentage field.
func assertMonitorNodePrevotePower(t *testing.T, endpoint string, height int64, round int, power, total int64) {
	t.Helper()
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(endpoint + "/consensus_state")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("consensus oracle returned HTTP %d", response.StatusCode)
	}
	var body struct {
		Result struct {
			RoundState struct {
				HeightRoundStep string `json:"height/round/step"`
				Votes           []struct {
					Round int    `json:"round"`
					Power string `json:"prevotes_bit_array"`
				} `json:"height_vote_set"`
			} `json:"round_state"`
		} `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&body); err != nil {
		t.Fatal(err)
	}
	state := body.Result.RoundState
	if !strings.HasPrefix(state.HeightRoundStep, fmt.Sprintf("%d/%d/", height, round)) {
		t.Fatalf("independent consensus round = %s, want %d/%d", state.HeightRoundStep, height, round)
	}
	for _, votes := range state.Votes {
		if votes.Round == round && strings.Contains(votes.Power, fmt.Sprintf(" %d/%d =", power, total)) {
			return
		}
	}
	t.Fatalf("independent vote power = %+v, want %d/%d", state.Votes, power, total)
}

func assertMonitorTerminalParticipation(t *testing.T, endpoint string, height int64, prevote, precommit int) {
	t.Helper()
	home := t.TempDir()
	initHome(t, home)
	cmd := exec.Command(aktBinary(t), "--home", home, "monitor", "--node", endpoint)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	terminal, slave, err := pty.Open()
	if err != nil {
		t.Fatalf("open monitor terminal: %v", err)
	}
	t.Cleanup(func() {
		_ = terminal.Close()
		_ = slave.Close()
	})
	if err := pty.Setsize(terminal, &pty.Winsize{
		Rows: 40,
		Cols: 140,
	}); err != nil {
		t.Fatalf("size monitor terminal: %v", err)
	}
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	// Ctty names the child's stdin descriptor, not the parent's slave fd.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    0,
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start monitor terminal: %v", err)
	}
	_ = slave.Close()

	var mu sync.Mutex
	var output []byte
	readDone := make(chan struct{})
	exited := make(chan error, 1)
	go func() {
		exited <- cmd.Wait()
	}()
	go func() {
		defer close(readDone)
		buffer := make([]byte, 8192)
		for {
			n, err := terminal.Read(buffer)
			mu.Lock()
			output = append(output, buffer[:n]...)
			if len(output) > 1<<20 {
				output = output[len(output)-(1<<20):]
			}
			mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		_, _ = terminal.Write([]byte("q"))
		select {
		case <-exited:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-exited
		}
		_ = terminal.Close()
		<-readDone
	})
	want := regexp.MustCompile(fmt.Sprintf(`\*\s+%d\s+%d\.0%%\s+%d\.0%%`, height, prevote, precommit))
	deadline := time.Now().Add(8 * time.Second)
	var screen string
	for time.Now().Before(deadline) {
		mu.Lock()
		screen = ansi.Strip(string(output))
		mu.Unlock()
		if match := want.FindString(screen); match != "" {
			t.Logf("public monitor terminal: %s", match)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(screen) > 4000 {
		screen = screen[len(screen)-4000:]
	}
	t.Fatalf("terminal never displayed expected participation %s:\n%s", want, screen)
}
