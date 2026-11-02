package pacman

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
	"github.com/mattn/go-isatty"
)

// HandoffResult captures the outcome of an interactive subprocess execution.
type HandoffResult struct {
	ExitCode int
	Status   string // "completed", "cancelled", "failed"
	Error    error
}

// TerminalHandoff manages handing over terminal control to an interactive child process
// (such as pacman or sudo) and safely restoring parent TUI / terminal state afterward.
type TerminalHandoff struct {
	// Program is an optional Bubble Tea program. If set, ReleaseTerminal and RestoreTerminal
	// are called around child execution to pause/resume the TUI and restore cooked mode.
	Program *tea.Program

	// Stdin, Stdout, Stderr are the streams connected to the child process.
	// They default to os.Stdin, os.Stdout, os.Stderr.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// MessageWriter is where EED status messages (such as "EED is handing terminal control to Pacman.")
	// are written. Defaults to Stdout or os.Stdout.
	MessageWriter io.Writer

	// IsTTY determines whether the given input stream is an interactive terminal.
	// Defaults to checking isatty on the underlying file descriptor.
	IsTTY func(io.Reader) bool
}

// NewTerminalHandoff creates a new TerminalHandoff configured with standard operating system streams.
func NewTerminalHandoff() *TerminalHandoff {
	return &TerminalHandoff{
		Stdin:         os.Stdin,
		Stdout:        os.Stdout,
		Stderr:        os.Stderr,
		MessageWriter: os.Stdout,
		IsTTY:         DefaultIsTTY,
	}
}

// DefaultIsTTY checks whether the given reader is an interactive terminal.
func DefaultIsTTY(r io.Reader) bool {
	if r == nil {
		return false
	}
	if f, ok := r.(*os.File); ok {
		fd := f.Fd()
		return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
	}
	return false
}

// ExecProcess is a convenience wrapper for Bubble Tea's tea.ExecProcess.
func ExecProcess(cmd *exec.Cmd, fn func(err error) tea.Msg) tea.Cmd {
	return tea.ExecProcess(cmd, fn)
}

// Execute hands terminal control to cmd, waits for it to complete, and restores terminal state.
func (h *TerminalHandoff) Execute(ctx context.Context, cmd *exec.Cmd) (HandoffResult, error) {
	if cmd == nil {
		return HandoffResult{ExitCode: -1, Status: "failed"}, errors.New("cannot execute nil command")
	}

	stdin := h.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	stdout := h.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := h.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	msgOut := h.MessageWriter
	if msgOut == nil {
		msgOut = stdout
	}
	isTTYFn := h.IsTTY
	if isTTYFn == nil {
		isTTYFn = DefaultIsTTY
	}

	// 1. TTY Validation: fail safely before starting transaction if not an interactive terminal
	if !isTTYFn(stdin) {
		err := errors.New("installation requires an interactive terminal to confirm package transactions; standard input is not a terminal")
		return HandoffResult{
			ExitCode: -1,
			Status:   "failed",
			Error:    err,
		}, err
	}

	// 2. User experience notice
	fmt.Fprintln(msgOut, "EED is handing terminal control to Pacman.")

	// 3. Save terminal state if stdin is a terminal file
	var originalState *term.State
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(f.Fd()) {
		if s, err := term.GetState(f.Fd()); err == nil {
			originalState = s
		}
	}

	// 4. Pause parent TUI and restore cooked mode
	if h.Program != nil {
		if err := h.Program.ReleaseTerminal(); err != nil {
			return HandoffResult{ExitCode: -1, Status: "failed", Error: err}, fmt.Errorf("release terminal from bubbletea: %w", err)
		}
	} else if originalState != nil {
		// Restore normal mode if previously set to raw
		if f, ok := stdin.(*os.File); ok {
			_ = term.Restore(f.Fd(), originalState)
		}
	}

	// Ensure terminal state and parent TUI are restored when done
	defer func() {
		if h.Program != nil {
			_ = h.Program.RestoreTerminal()
		} else if originalState != nil {
			if f, ok := stdin.(*os.File); ok {
				_ = term.Restore(f.Fd(), originalState)
			}
		}
	}()

	// 5. Connect streams
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	var errCapture bytes.Buffer
	cmd.Stderr = io.MultiWriter(stderr, &errCapture)
	if cmd.Env == nil {
		cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	}

	// 6. Launch child and wait for exit
	runErr := cmd.Run()

	// 7. Inspect exit status
	res := HandoffResult{
		ExitCode: 0,
		Status:   "completed",
		Error:    runErr,
	}

	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
			errLower := strings.ToLower(errCapture.String())
			if res.ExitCode == 130 {
				res.Status = "cancelled"
			} else if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() && ws.Signal() == syscall.SIGINT {
				res.Status = "cancelled"
			} else if strings.Contains(errLower, "abort") || strings.Contains(errLower, "cancel") {
				res.Status = "cancelled"
			} else if res.ExitCode == 1 && strings.Contains(errLower, "user") {
				res.Status = "cancelled"
			} else {
				res.Status = "failed"
			}
		} else {
			res.ExitCode = -1
			res.Status = "failed"
		}
	}

	return res, runErr
}
