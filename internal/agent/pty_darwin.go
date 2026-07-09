//go:build darwin
// +build darwin

package agent

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

type darwinTerminal struct {
	shellCmd *exec.Cmd
	pty      *os.File
	termType string
}

func NewTerminal(termType string) Terminal {
	return &darwinTerminal{termType: termType}
}

func (t *darwinTerminal) StartShell() error {
	currentShell := "/bin/bash"
	if _, err := os.Stat(currentShell); err != nil {
		currentShell = "/bin/sh"
	}

	var cmd *exec.Cmd
	monitorScript := os.Getenv("T2T_MONITOR_SCRIPT")
	if strings.HasSuffix(currentShell, "bash") {
		if monitorScript != "" {
			if _, err := os.Stat(monitorScript); err == nil {
				cmd = exec.Command(currentShell, "--rcfile", monitorScript, "-i")
			} else {
				cmd = exec.Command(currentShell, "-i")
			}
		} else {
			cmd = exec.Command(currentShell, "-i")
		}
	} else {
		cmd = exec.Command(currentShell, "-i")
	}

	cmd.Env = append(os.Environ(),
		fmt.Sprintf("TERM=%s", t.termType),
		"COLORTERM=truecolor",
	)
	u, err := user.Current()
	if err == nil && u.HomeDir != "" {
		cmd.Dir = u.HomeDir
	}
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("启动 PTY 失败: %v", err)
	}
	if err := pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
		return fmt.Errorf("设置 PTY 大小失败: %v", err)
	}
	if err := setupPtyAttr(ptmx); err != nil {
		return fmt.Errorf("设置 PTY 属性失败: %v", err)
	}
	t.shellCmd = cmd
	t.pty = ptmx
	return nil
}

func (t *darwinTerminal) Read(b []byte) (int, error)  { return t.pty.Read(b) }
func (t *darwinTerminal) Write(b []byte) (int, error) { return t.pty.Write(b) }
func (t *darwinTerminal) Resize(rows, cols int) error {
	return pty.Setsize(t.pty, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}
func (t *darwinTerminal) Close() error {
	if t.pty != nil {
		t.pty.Close()
	}
	if t.shellCmd != nil && t.shellCmd.Process != nil {
		t.shellCmd.Process.Kill()
	}
	return nil
}

func setupPtyAttr(ptmx *os.File) error {
	termios, err := unix.IoctlGetTermios(int(ptmx.Fd()), unix.TIOCGETA)
	if err != nil {
		return err
	}
	termios.Iflag |= unix.ICRNL
	termios.Oflag |= unix.ONLCR
	termios.Lflag |= unix.ICANON | unix.ECHO | unix.ECHOE | unix.ECHOK
	termios.Oflag |= unix.OPOST | unix.ONLRET
	termios.Cc[unix.VMIN] = 1
	termios.Cc[unix.VTIME] = 0
	return unix.IoctlSetTermios(int(ptmx.Fd()), unix.TIOCSETA, termios)
}
