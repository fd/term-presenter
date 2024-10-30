package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/creack/pty"
)

func Exec(w io.Writer, op Op) {
	cmd := exec.Command("bash", "--noprofile", "--norc")
	cmd.Env = append(os.Environ(), []string{
		"PS1=\x1B@$?.",
		"PS2=",
		"PS3=",
		"PS4=",
		"PROMPT_COMMAND=",
	}...)

	rows, cols, err := pty.Getsize(os.Stdin)
	if err != nil {
		op := OpOops{err.Error()}
		op.Exec(w, nil, nil)
		return
	}

	f, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
	if err != nil {
		op := OpOops{err.Error()}
		op.Exec(w, nil, nil)
		return
	}
	defer f.Write([]byte{4})

	ptyState, err := newPtyState(f)
	if err != nil {
		op := OpOops{err.Error()}
		op.Exec(w, nil, nil)
		return
	}
	defer ptyState.Restore()

	go io.Copy(f, os.Stdin)

	var cp = BashCopy{ptyState: ptyState, r: f, o: os.Stdin}
	err = cp.WriteTo(w)
	if err != nil {
		op := OpOops{err.Error()}
		op.Exec(w, f, ptyState)
	}

	err = op.Exec(w, f, ptyState)
	if err != nil {
		op := OpOops{err.Error()}
		op.Exec(w, f, ptyState)
	}
}

type Op interface {
	Exec(w io.Writer, pty *os.File, ptyState *PtyState) error
}

type Script []Op

func (s Script) Exec(w io.Writer, pty *os.File, ptyState *PtyState) error {
	for _, op := range s {
		time.Sleep(250 * time.Millisecond)
		err := op.Exec(w, pty, ptyState)
		if err != nil {
			return err
		}
	}
	return nil
}

type OpEcho struct {
	content string
}

func (e *OpEcho) Exec(w io.Writer, pty *os.File, ptyState *PtyState) error {
	_, err := w.Write([]byte("\x1B[35m"))
	if err != nil {
		return err
	}

	err = shellTyper(w, "# "+e.content, 0, true)
	if err != nil {
		return err
	}

	_, err = w.Write([]byte("\x1B[0m\r\n"))
	if err != nil {
		return err
	}

	return nil
}

type OpType struct {
	content string
}

func (e *OpType) Exec(w io.Writer, pty *os.File, ptyState *PtyState) error {
	err := shellTyper(pty, e.content, 0, false)
	if err != nil {
		return err
	}

	return nil
}

type OpOops struct {
	content string
}

func (e *OpOops) Exec(w io.Writer, pty *os.File, ptyState *PtyState) error {
	_, err := w.Write([]byte("\x1B[31m"))
	if err != nil {
		return err
	}

	err = shellTyper(w, "! "+e.content, 0, true)
	if err != nil {
		return err
	}

	_, err = w.Write([]byte("\x1B[0m\r\n"))
	if err != nil {
		return err
	}

	return nil
}

type OpExec struct {
	cmd string
	Ops Script
}

func (e *OpExec) Exec(w io.Writer, pty *os.File, ptyState *PtyState) error {
	_, err := w.Write([]byte("\x1B[0m\x1B[32m$ \x1B[0m"))
	if err != nil {
		return err
	}

	var cErr = make(chan error)
	go func() {
		err = shellTyper(pty, e.cmd, 0, false)
		if err != nil {
			cErr <- err
			return
		}

		time.Sleep(100 * time.Millisecond)

		_, err = pty.Write([]byte("\n"))
		if err != nil {
			cErr <- err
			return
		}

		if len(e.Ops) > 0 {
			time.Sleep(500 * time.Millisecond)

			err = e.Ops.Exec(w, pty, ptyState)
			if err != nil {
				cErr <- err
				return
			}
		}

		cErr <- nil
	}()

	var cp = BashCopy{ptyState: ptyState, r: pty, o: os.Stdin}
	err = cp.WriteTo(w)
	if err != nil {
		return err
	}

	err = <-cErr
	if err != nil {
		return err
	}

	if cp.code != 0 {
		return fmt.Errorf("the command exited with status %d.", cp.code)
	}

	return nil
}

type OpBreath struct{ nl bool }

func (e *OpBreath) Exec(w io.Writer, pty *os.File, ptyState *PtyState) error {
	if e.nl {
		_, err := w.Write([]byte("\r\n"))
		if err != nil {
			return err
		}
	}

	time.Sleep(time.Second)
	return nil
}

func shellTyper(w io.Writer, s string, rate int, out bool) error {
	if rate == 0 {
		rate = 16
	}

	var (
		p     = []byte(s)
		delay = time.Second / time.Duration(rate)
	)

	for len(p) > 0 {
		time.Sleep(delay)
		r, n := utf8.DecodeRune(p)
		if r == '\n' && out {
			_, err := w.Write([]byte("\r\n"))
			if err != nil {
				return err
			}
		} else {
			_, err := w.Write(p[:n])
			if err != nil {
				return err
			}
		}
		p = p[n:]
	}

	return nil
}

type PtyState struct {
	pty      *os.File
	oldState syscall.Termios
}

func newPtyState(pty *os.File) (*PtyState, error) {
	var oldState syscall.Termios

	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL,
		pty.Fd(),
		ioctlReadTermios,
		uintptr(unsafe.Pointer(&oldState)),
		0, 0, 0); err != 0 {
		return nil, err
	}

	return &PtyState{pty, oldState}, nil
}

func (p *PtyState) CopyTo(f *os.File) error {
	var state = new(syscall.Termios)

	p.pty.Sync()
	f.Sync()

	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL,
		p.pty.Fd(),
		ioctlReadTermios,
		uintptr(unsafe.Pointer(state)),
		0, 0, 0); err != 0 {
		return err
	}

	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL,
		f.Fd(),
		ioctlWriteTermios,
		uintptr(unsafe.Pointer(state)),
		0, 0, 0); err != 0 {
		return err
	}

	p.pty.Sync()
	f.Sync()

	return nil
}

func (p *PtyState) Restore() error {
	p.pty.Sync()

	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL,
		p.pty.Fd(),
		ioctlWriteTermios,
		uintptr(unsafe.Pointer(&p.oldState)),
		0, 0, 0); err != 0 {
		return err
	}

	return p.pty.Sync()
}

var ptybuf bytes.Buffer

type BashCopy struct {
	ptyState *PtyState
	r        *os.File
	o        *os.File
	code     uint8
}

func (b *BashCopy) WriteTo(w io.Writer) error {
	var (
		buf   [1]byte
		code  uint8
		state int
	)

	for {
		err := b.ptyState.CopyTo(b.o)
		if err != nil {
			panic("oops")
		}

		switch state {

		case 0:
			_, err := b.r.Read(buf[:])
			if err != nil {
				return err
			}
			ptybuf.WriteByte(buf[0])

			if buf[0] == 0x1B {
				state = 1
			} else {
				w.Write(buf[:])
			}

		case 1:
			_, err := b.r.Read(buf[:])
			if err != nil {
				return err
			}
			ptybuf.WriteByte(buf[0])

			if buf[0] == '@' {
				state = 2
			} else {
				w.Write([]byte{0x1B})
				w.Write(buf[:])
				state = 0
			}

		case 2:
			_, err := b.r.Read(buf[:])
			if err != nil {
				return err
			}
			ptybuf.WriteByte(buf[0])

			if '0' <= buf[0] && buf[0] <= '9' {
				code = code*10 + (buf[0] - '0')
			} else if buf[0] == '.' {
				b.code = code
				state = 3
			} else {
				panic("error while reading exit status")
			}

		case 3:
			return nil

		}
	}
}
