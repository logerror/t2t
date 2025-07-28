//go:build linux
// +build linux

package agent

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

type linuxTerminal struct {
	shellCmd *exec.Cmd
	pty      *os.File
	termType string
}

func NewTerminal(termType string) Terminal {
	return &linuxTerminal{termType: termType}
}

func (t *linuxTerminal) StartShell() error {
	currentShell := "/bin/bash"
	if _, err := os.Stat(currentShell); err != nil {
		currentShell = "/bin/sh"
	}
	cmd := exec.Command(currentShell)
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

func (t *linuxTerminal) Read(b []byte) (int, error)  { return t.pty.Read(b) }
func (t *linuxTerminal) Write(b []byte) (int, error) { return t.pty.Write(b) }
func (t *linuxTerminal) Resize(rows, cols int) error {
	return pty.Setsize(t.pty, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}
func (t *linuxTerminal) Close() error {
	if t.pty != nil {
		t.pty.Close()
	}
	if t.shellCmd != nil && t.shellCmd.Process != nil {
		t.shellCmd.Process.Kill()
	}
	return nil
}

func setupPtyAttr(ptmx *os.File) error {
	termios, err := unix.IoctlGetTermios(int(ptmx.Fd()), unix.TCGETS)
	if err != nil {
		return err
	}
	termios.Iflag |= unix.ICRNL
	termios.Oflag |= unix.ONLCR
	termios.Lflag |= unix.ICANON | unix.ECHO | unix.ECHOE | unix.ECHOK
	termios.Oflag |= unix.OPOST | unix.ONLRET
	termios.Cc[unix.VMIN] = 1
	termios.Cc[unix.VTIME] = 0
	return unix.IoctlSetTermios(int(ptmx.Fd()), unix.TCSETS, termios)
}
