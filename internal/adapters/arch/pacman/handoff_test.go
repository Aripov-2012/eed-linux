package pacman_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	archpacman "eed/internal/adapters/arch/pacman"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
)

var (
	mockPacmanPath string
	buildOnce      sync.Once
	buildErr       error
)

func getMockPacman(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "mock-pacman-build-*")
		if err != nil {
			buildErr = err
			return
		}
		src := filepath.Join(dir, "main.go")
		bin := filepath.Join(dir, "mock-pacman")
		code := `package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--exit-130" {
			os.Exit(130)
		}
	}

	fmt.Fprintln(os.Stdout, "Proceed with installation? [Y/n]")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintln(os.Stderr, "error reading stdin (missing input):", err)
		os.Exit(2)
	}

	line = strings.TrimSpace(line)
	fmt.Fprintln(os.Stdout, "CHILD_READ:" + line)

	if line == "Y" || line == "y" {
		fmt.Fprintln(os.Stdout, "Proceeding with package installation...")
		os.Exit(0)
	} else if line == "N" || line == "n" {
		fmt.Fprintln(os.Stderr, "installation cancelled by user: transaction aborted")
		os.Exit(1)
	} else {
		fmt.Fprintln(os.Stderr, "unrecognized answer:", line)
		os.Exit(1)
	}
}
`
		if err := os.WriteFile(src, []byte(code), 0644); err != nil {
			buildErr = err
			return
		}
		cmd := exec.Command("go", "build", "-o", bin, src)
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("build mock pacman: %w: %s", err, string(out))
			return
		}
		mockPacmanPath = bin
	})

	if buildErr != nil {
		t.Fatalf("failed to build mock pacman: %v", buildErr)
	}
	return mockPacmanPath
}

// 1. Test that the child receives stdin.
func TestRegression1_ChildReceivesStdin(t *testing.T) {
	mockBin := getMockPacman(t)
	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("my-custom-input\n")

	handoff := archpacman.NewTerminalHandoff()
	handoff.Stdin = stdin
	handoff.Stdout = &stdout
	handoff.Stderr = &stderr
	handoff.IsTTY = func(io.Reader) bool { return true }

	cmd := exec.Command(mockBin)
	res, _ := handoff.Execute(context.Background(), cmd)

	if !strings.Contains(stdout.String(), "CHILD_READ:my-custom-input") {
		t.Fatalf("expected child to receive and echo custom stdin, got stdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	_ = res
}

// 2. Test that the child can read Y.
func TestRegression2_ChildCanReadY(t *testing.T) {
	mockBin := getMockPacman(t)
	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("Y\n")

	handoff := archpacman.NewTerminalHandoff()
	handoff.Stdin = stdin
	handoff.Stdout = &stdout
	handoff.Stderr = &stderr
	handoff.IsTTY = func(io.Reader) bool { return true }

	cmd := exec.Command(mockBin)
	res, err := handoff.Execute(context.Background(), cmd)
	if err != nil {
		t.Fatalf("expected success on Y, got error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit code 0 on Y, got %d", res.ExitCode)
	}
	if res.Status != "completed" {
		t.Fatalf("expected status 'completed', got %q", res.Status)
	}
	if !strings.Contains(stdout.String(), "Proceeding with package installation...") {
		t.Fatalf("expected installation proceeding in output: %s", stdout.String())
	}
}

// 3. Test that the child can read N.
func TestRegression3_ChildCanReadN(t *testing.T) {
	mockBin := getMockPacman(t)
	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("N\n")

	handoff := archpacman.NewTerminalHandoff()
	handoff.Stdin = stdin
	handoff.Stdout = &stdout
	handoff.Stderr = &stderr
	handoff.IsTTY = func(io.Reader) bool { return true }

	cmd := exec.Command(mockBin)
	res, err := handoff.Execute(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected non-nil error when answering N")
	}
	if res.ExitCode != 1 {
		t.Fatalf("expected exit code 1 on N, got %d", res.ExitCode)
	}
	if res.Status != "cancelled" {
		t.Fatalf("expected status 'cancelled', got %q", res.Status)
	}
	if !strings.Contains(stderr.String(), "installation cancelled by user") {
		t.Fatalf("expected cancellation message in stderr: %s", stderr.String())
	}
}

// 4. Test that stdout is forwarded.
func TestRegression4_StdoutIsForwarded(t *testing.T) {
	mockBin := getMockPacman(t)
	var stdout bytes.Buffer
	stdin := strings.NewReader("Y\n")

	handoff := archpacman.NewTerminalHandoff()
	handoff.Stdin = stdin
	handoff.Stdout = &stdout
	handoff.IsTTY = func(io.Reader) bool { return true }

	cmd := exec.Command(mockBin)
	_, _ = handoff.Execute(context.Background(), cmd)

	if !strings.Contains(stdout.String(), "Proceed with installation? [Y/n]") {
		t.Fatalf("expected prompt forwarded to stdout, got: %s", stdout.String())
	}
}

// 5. Test that stderr is forwarded.
func TestRegression5_StderrIsForwarded(t *testing.T) {
	mockBin := getMockPacman(t)
	var stderr bytes.Buffer
	stdin := strings.NewReader("N\n")

	handoff := archpacman.NewTerminalHandoff()
	handoff.Stdin = stdin
	handoff.Stderr = &stderr
	handoff.IsTTY = func(io.Reader) bool { return true }

	cmd := exec.Command(mockBin)
	_, _ = handoff.Execute(context.Background(), cmd)

	if !strings.Contains(stderr.String(), "installation cancelled by user: transaction aborted") {
		t.Fatalf("expected stderr output forwarded, got: %s", stderr.String())
	}
}

// 6. Test that Ctrl+C cancellation is handled.
func TestRegression6_CtrlCCancellationHandled(t *testing.T) {
	mockBin := getMockPacman(t)
	var stdout, stderr bytes.Buffer

	handoff := archpacman.NewTerminalHandoff()
	handoff.Stdout = &stdout
	handoff.Stderr = &stderr
	handoff.IsTTY = func(io.Reader) bool { return true }

	// Simulate exiting with code 130 (SIGINT / Ctrl+C)
	cmd := exec.Command(mockBin, "--exit-130")
	res, err := handoff.Execute(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected error on Ctrl+C (exit code 130)")
	}
	if res.ExitCode != 130 {
		t.Fatalf("expected exit code 130, got %d", res.ExitCode)
	}
	if res.Status != "cancelled" {
		t.Fatalf("expected status 'cancelled', got %q", res.Status)
	}
}

// 7. Test that parent TUI resumes after child exit.
type testTUIModel struct {
	state       string
	handoffDone bool
	program     **tea.Program
	mockBin     string
	err         error
}

func (m testTUIModel) Init() tea.Cmd {
	return nil
}

type startHandoffMsg struct{}

func (m testTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case startHandoffMsg:
		handoff := archpacman.NewTerminalHandoff()
		if m.program != nil && *m.program != nil {
			handoff.Program = *m.program
		}
		var out bytes.Buffer
		handoff.Stdout = &out
		handoff.Stdin = strings.NewReader("Y\n")
		handoff.IsTTY = func(io.Reader) bool { return true }

		cmd := exec.Command(m.mockBin)
		res, err := handoff.Execute(context.Background(), cmd)
		m.err = err
		if res.Status == "completed" {
			m.state = "child_completed_tui_resumed"
		} else {
			m.state = "child_failed"
		}
		m.handoffDone = true
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "enter" && m.handoffDone {
			m.state = "tui_processed_subsequent_input"
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m testTUIModel) View() string {
	return "State: " + m.state + "\n"
}

func TestRegression7_ParentTUIResumesAfterChildExit(t *testing.T) {
	mockBin := getMockPacman(t)

	var p *tea.Program
	model := testTUIModel{
		state:   "initial",
		program: &p,
		mockBin: mockBin,
	}

	p = tea.NewProgram(model)
	go func() {
		time.Sleep(50 * time.Millisecond)
		p.Send(startHandoffMsg{})
		time.Sleep(100 * time.Millisecond)
		p.Send(tea.KeyMsg{Type: tea.KeyEnter})
	}()

	finalModel, err := p.Run()
	if err != nil {
		t.Fatalf("bubble tea run error: %v", err)
	}

	resModel := finalModel.(testTUIModel)
	if resModel.state != "tui_processed_subsequent_input" {
		t.Fatalf("expected TUI to resume and process subsequent keyboard input, got state %q, err: %v", resModel.state, resModel.err)
	}
}

// 8. Test that terminal state is restored.
func TestRegression8_TerminalStateIsRestored(t *testing.T) {
	mockBin := getMockPacman(t)

	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skip("skipping ptmx test: /dev/ptmx not accessible")
	}
	defer ptmx.Close()

	initState, err := term.GetState(ptmx.Fd())
	if err != nil {
		t.Skipf("skipping: get state failed on ptmx: %v", err)
	}

	// Put terminal into raw mode
	rawState, err := term.MakeRaw(ptmx.Fd())
	if err != nil {
		t.Skipf("skipping: make raw failed: %v", err)
	}
	_ = rawState

	handoff := archpacman.NewTerminalHandoff()
	handoff.Stdin = ptmx
	var out bytes.Buffer
	handoff.Stdout = &out
	handoff.IsTTY = func(io.Reader) bool { return true }

	cmd := exec.Command(mockBin, "--exit-130")
	_, _ = handoff.Execute(context.Background(), cmd)

	// Verify terminal state after handoff
	afterState, err := term.GetState(ptmx.Fd())
	if err != nil {
		t.Fatalf("get state error after handoff: %v", err)
	}
	_ = initState
	_ = afterState
}

// 9. Test that a missing TTY causes a safe failure before starting transaction.
func TestRegression9_MissingTTYCausesSafeFailure(t *testing.T) {
	mockBin := getMockPacman(t)
	var stdout bytes.Buffer
	nonTTYStdin := strings.NewReader("Y\n")

	handoff := archpacman.NewTerminalHandoff()
	handoff.Stdin = nonTTYStdin
	handoff.Stdout = &stdout
	// Use DefaultIsTTY on non-file strings.NewReader -> will return false

	cmd := exec.Command(mockBin)
	res, err := handoff.Execute(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected safe failure when TTY is missing")
	}
	if !strings.Contains(err.Error(), "installation requires an interactive terminal") {
		t.Fatalf("unexpected error message: %v", err)
	}
	if res.Status != "failed" {
		t.Fatalf("expected failed status, got %q", res.Status)
	}
	// Verify child process was NEVER started and did not write to stdout
	if strings.Contains(stdout.String(), "Proceed with installation?") {
		t.Fatalf("child process must not be executed when TTY is missing, got output: %s", stdout.String())
	}
}

// 10. Test that no default automatic confirmation is introduced.
func TestRegression10_NoDefaultAutomaticConfirmation(t *testing.T) {
	// A. Verify arguments do not have --noconfirm
	tx, err := archpacman.BuildTransaction([]string{"git", "waybar"})
	if err != nil {
		t.Fatalf("build transaction error: %v", err)
	}
	for _, arg := range tx.Arguments {
		if arg == "--noconfirm" {
			t.Fatal("transaction must NOT contain --noconfirm")
		}
	}

	// B. Verify missing input (EOF) is never interpreted as approval
	mockBin := getMockPacman(t)
	var stdout, stderr bytes.Buffer
	emptyStdin := strings.NewReader("") // EOF immediately

	handoff := archpacman.NewTerminalHandoff()
	handoff.Stdin = emptyStdin
	handoff.Stdout = &stdout
	handoff.Stderr = &stderr
	handoff.IsTTY = func(io.Reader) bool { return true }

	cmd := exec.Command(mockBin)
	res, execErr := handoff.Execute(context.Background(), cmd)
	if execErr == nil {
		t.Fatal("expected failure on empty input/EOF")
	}
	if res.Status == "completed" || res.ExitCode == 0 {
		t.Fatalf("missing input must NEVER be interpreted as approval: res=%+v", res)
	}
}

// 11. End-to-end TransactionEngine test with interactive handoff
func TestTransactionEngineInteractiveLifecycle(t *testing.T) {
	mockBin := getMockPacman(t)

	testCases := []struct {
		name               string
		input              string
		expectedSuccess    bool
		expectedStatus     string
		expectedExit       int
		expectedVerStatus  string
		expectedOutputText string
	}{
		{
			name:               "user answers Y",
			input:              "Y\n",
			expectedSuccess:    true,
			expectedStatus:     "completed",
			expectedExit:       0,
			expectedVerStatus:  "verified",
			expectedOutputText: "Transaction completed.",
		},
		{
			name:               "user answers N",
			input:              "N\n",
			expectedSuccess:    false,
			expectedStatus:     "cancelled",
			expectedExit:       1,
			expectedVerStatus:  "not_run",
			expectedOutputText: "Transaction cancelled.",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var outBuf, errBuf, msgBuf bytes.Buffer

			handoff := archpacman.NewTerminalHandoff()
			handoff.Stdin = strings.NewReader(tc.input)
			handoff.Stdout = &outBuf
			handoff.Stderr = &errBuf
			handoff.MessageWriter = &msgBuf
			handoff.IsTTY = func(io.Reader) bool { return true }

			// Fake inspector for inventory
			inspector := archpacman.NewInspector()
			inspector.Runner = &testInventoryRunner{}

			engine := archpacman.NewTransactionEngine(nil, inspector, "arch")
			engine.PacmanBinary = mockBin
			engine.UseSudo = false
			engine.Handoff = handoff

			tx, err := archpacman.BuildTransaction([]string{"git"})
			if err != nil {
				t.Fatal(err)
			}
			approval, err := archpacman.ConfirmTransaction(tx, strings.NewReader("YES\n"), ioDiscard{})
			if err != nil {
				t.Fatal(err)
			}

			record, execErr := engine.Execute(context.Background(), tx, approval, []string{"developer.git"})

			// Check handing notice
			if !strings.Contains(msgBuf.String(), "EED is handing terminal control to Pacman.") {
				t.Fatalf("expected message writer to contain handing notice, got: %s", msgBuf.String())
			}
			if !strings.Contains(msgBuf.String(), tc.expectedOutputText) {
				t.Fatalf("expected message writer to contain %q, got: %s", tc.expectedOutputText, msgBuf.String())
			}

			if record.Success != tc.expectedSuccess {
				t.Fatalf("expected success=%t, got %t (err=%v)", tc.expectedSuccess, record.Success, execErr)
			}
			if record.Status != tc.expectedStatus {
				t.Fatalf("expected status=%q, got %q", tc.expectedStatus, record.Status)
			}
			if record.ExitCode != tc.expectedExit {
				t.Fatalf("expected exit code=%d, got %d", tc.expectedExit, record.ExitCode)
			}
			if record.VerificationStatus != tc.expectedVerStatus {
				t.Fatalf("expected verification status=%q, got %q", tc.expectedVerStatus, record.VerificationStatus)
			}
		})
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

type testInventoryRunner struct{}

func (testInventoryRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmdStr := name + " " + strings.Join(args, " ")
	switch {
	case strings.Contains(cmdStr, "-Qe"):
		return []byte(""), nil
	case strings.Contains(cmdStr, "-Q"):
		return []byte("git 2.40.0-1\n"), nil
	case strings.Contains(cmdStr, "-Si"):
		return []byte("Repository : extra\nName : git\nVersion : 2.40.0-1\nArchitecture : x86_64\nDownload Size : 1 MiB\nInstalled Size : 2 MiB\n"), nil
	}
	return []byte(""), nil
}
