# EED — Fix Interactive Pacman Input and Terminal Handoff

## Problem

During `eed install`, Pacman displays an interactive confirmation:

`Proceed with installation? [Y/n]`

However, the user cannot enter Y or N.

The prompt is visible, but keyboard input does not reach the child process.

This is likely caused by incorrect stdin forwarding or the parent TUI retaining terminal control.

## Objective

Implement reliable interactive subprocess execution for Pacman and other commands that require terminal input.

## 1. Audit subprocess execution

Inspect all uses of:
- exec.Command
- exec.CommandContext
- cmd.Run()
- cmd.Output()
- cmd.CombinedOutput()
- cmd.StdinPipe()

Identify how the Pacman transaction is launched and which component owns terminal input.

Determine whether the confirmation prompt originates from EED, sudo or Pacman.

## 2. Forward standard streams

For interactive subprocesses, ensure:

- cmd.Stdin receives the actual terminal input.
- cmd.Stdout forwards output to the terminal.
- cmd.Stderr forwards errors and prompts to the terminal.
- The child process inherits the appropriate terminal environment.

Do not use Output() or CombinedOutput() for interactive Pacman transactions.

Do not leave Stdin nil.

Do not create StdinPipe unless there is a deliberate input-forwarding implementation.

## 3. Bubble Tea terminal handoff

EED uses a terminal interface.

Inspect the current Bubble Tea implementation and its version in go.mod.

Before launching an interactive child process:

1. Pause the parent TUI.
2. Stop terminal raw-mode input handling.
3. Restore normal terminal input and output.
4. Temporarily release terminal control to the child process.
5. Launch Pacman with inherited stdin, stdout and stderr.
6. Wait for the child process to exit.
7. Restore the parent TUI.
8. Resume normal keyboard handling.

Use Bubble Tea's supported external-process execution mechanism, such as tea.ExecProcess when compatible with the existing architecture.

Do not implement an unsafe terminal handoff that leaves the terminal in raw mode.

## 4. Preserve interactive confirmation

Requirements:

- Do not add --noconfirm.
- Do not automatically answer Y.
- Do not redirect input from /dev/null.
- Do not swallow keyboard input.
- Allow the user to enter Y or N.
- Allow Ctrl+C cancellation.
- Preserve sudo password prompts.
- Propagate child exit codes and errors to EED.

If no interactive terminal is available, fail safely before starting the transaction and explain that installation requires an interactive terminal.

Never interpret missing input as approval.

## 5. User experience

During execution, show a clear message:

`EED is handing terminal control to Pacman.`

After Pacman exits, restore the EED interface and report:

- Transaction completed.
- Transaction cancelled.
- Transaction failed.
- Exit status.
- Verification status.

Do not report success merely because the child process started.

## 6. Regression tests

Create tests using a mock interactive executable.

Test that:

1. The child receives stdin.
2. The child can read Y.
3. The child can read N.
4. Stdout is forwarded.
5. Stderr is forwarded.
6. Ctrl+C cancellation is handled.
7. Parent TUI resumes after child exit.
8. Terminal state is restored.
9. A missing TTY causes a safe failure.
10. No default automatic confirmation is introduced.

Do not run real Pacman installation transactions during unit tests.

## Constraints

- Preserve the existing Go architecture.
- Preserve Arch package transaction safety.
- Do not modify Ubuntu or Arch system configuration during tests.
- Run gofmt, go test ./..., go vet ./... and go test -race ./....

## Deliverables

Provide:
- Root cause.
- Modified Go files.
- Terminal handoff implementation.
- Test results.
- Instructions for testing interactive Pacman in an Arch VM.