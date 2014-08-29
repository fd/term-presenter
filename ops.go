package main

import (
  "fmt"
  "io"
  "os"
  "os/exec"
  "syscall"
  "time"
  "unicode/utf8"
  "unsafe"

  "github.com/dapplebeforedawn/pty"
)

func Exec(w io.Writer, op Op) {
  cmd := exec.Command("bash")
  cmd.Env = append(os.Environ(), []string{
    "PS1=\x1B@$?.",
    "PS2=",
    "PS3=",
    "PS4=",
    // "TERM=ansi-term",
  }...)

  f, err := pty.Start(cmd)
  if err != nil {
    op := OpOops{err.Error()}
    op.Exec(w, nil, nil)
    return
  }
  defer f.Write([]byte{4})

  {
    r, c, err := pty.Getsize(os.Stdin)
    if err != nil {
      r, c = 80, 120
    }

    err = pty.Setsize(f, uint16(r), uint16(c))
    if err != nil {
      op := OpOops{err.Error()}
      op.Exec(w, nil, nil)
      return
    }
  }

  ptyState, err := newPtyState(f)
  if err != nil {
    op := OpOops{err.Error()}
    op.Exec(w, nil, nil)
    return
  }
  defer ptyState.Restore()

  err = ptyState.NoEcho()
  if err != nil {
    op := OpOops{err.Error()}
    op.Exec(w, nil, nil)
    return
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
  _, err := w.Write([]byte("\x1B[90m"))
  if err != nil {
    return err
  }

  err = shellTyper(w, "# "+e.content+"\n", 0)
  if err != nil {
    return err
  }

  _, err = w.Write([]byte("\x1B[0m"))
  if err != nil {
    return err
  }

  return nil
}

type OpType struct {
  content string
}

func (e *OpType) Exec(w io.Writer, pty *os.File, ptyState *PtyState) error {
  err := shellTyper(pty, e.content, 0)
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

  err = shellTyper(w, "! "+e.content+"\n", 0)
  if err != nil {
    return err
  }

  _, err = w.Write([]byte("\x1B[0m"))
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
  _, err := w.Write([]byte("\x1B[32m$\x1B[0m "))
  if err != nil {
    return err
  }

  err = shellTyper(w, e.cmd+"\n", 0)
  if err != nil {
    return err
  }

  go e.Ops.Exec(w, pty, ptyState)

  time.Sleep(100 * time.Millisecond)

  _, err = pty.Write([]byte(e.cmd + "\n"))
  if err != nil {
    return err
  }

  ptyState.Restore()
  defer func() {
    ptyState.Restore()
    ptyState.NoEcho()
  }()

  var r = BashReader{ptyState: ptyState, r: pty, o: os.Stdin}
  _, err = io.Copy(w, &r)
  if err != nil {
    return err
  }

  if r.code != 0 {
    return fmt.Errorf("the command exited with status %d.", r.code)
  }

  return nil
}

type OpBreath struct{ nl bool }

func (e *OpBreath) Exec(w io.Writer, pty *os.File, ptyState *PtyState) error {
  if e.nl {
    _, err := w.Write([]byte("\n"))
    if err != nil {
      return err
    }
  }

  time.Sleep(time.Second)
  return nil
}

func shellTyper(w io.Writer, s string, rate int) error {
  if rate == 0 {
    rate = 12
  }

  var (
    p     = []byte(s)
    delay = time.Second / time.Duration(rate)
  )

  for len(p) > 0 {
    time.Sleep(delay)
    _, n := utf8.DecodeRune(p)
    _, err := w.Write(p[:n])
    if err != nil {
      return err
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

func (p *PtyState) NoEcho() error {
  newState := p.oldState
  newState.Lflag &^= syscall.ECHO
  newState.Lflag |= syscall.ICANON | syscall.ISIG
  newState.Iflag |= syscall.ICRNL

  if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL,
    p.pty.Fd(),
    ioctlWriteTermios,
    uintptr(unsafe.Pointer(&newState)),
    0, 0, 0); err != 0 {
    return err
  }

  return nil
}

func (p *PtyState) CopyTo(f *os.File) error {
  var state syscall.Termios

  if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL,
    p.pty.Fd(),
    ioctlReadTermios,
    uintptr(unsafe.Pointer(&state)),
    0, 0, 0); err != 0 {
    return err
  }

  if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL,
    f.Fd(),
    ioctlWriteTermios,
    uintptr(unsafe.Pointer(&state)),
    0, 0, 0); err != 0 {
    return err
  }

  return nil
}

func (p *PtyState) Restore() error {
  if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL,
    p.pty.Fd(),
    ioctlWriteTermios,
    uintptr(unsafe.Pointer(&p.oldState)),
    0, 0, 0); err != 0 {
    return err
  }

  return nil
}

type BashReader struct {
  ptyState *PtyState
  r        *os.File
  o        *os.File
  code     uint8
  state    int
}

func (b *BashReader) Read(p []byte) (int, error) {
  var (
    buf  [1]byte
    code uint8
    n    int
  )

  b.ptyState.CopyTo(b.o)

  switch b.state {

  case 0:
    _, err := b.r.Read(buf[:])
    if err != nil {
      return n, err
    }

    if buf[0] == 0x1B {
      b.state = 1
    } else {
      p[0] = buf[0]
      p = p[1:]
      n += 1
    }

  case 1:
    _, err := b.r.Read(buf[:])
    if err != nil {
      return n, err
    }

    if buf[0] == '@' {
      b.state = 2
    } else {
      b.state = 0
      p[0] = 0x1B
      p[1] = buf[0]
      p = p[2:]
      n += 2
    }

  case 2:
    _, err := b.r.Read(buf[:])
    if err != nil {
      return n, err
    }

    if '0' <= buf[0] && buf[0] <= '9' {
      code = (code * 10) + (buf[0] - '0')
    } else if buf[0] == '.' {
      b.code = code
      b.state = 3
    } else {
      panic("error while readin exit status")
    }

  case 3:
    return n, io.EOF

  }

  return n, nil
}
