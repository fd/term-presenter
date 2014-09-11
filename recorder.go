package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/dapplebeforedawn/pty"
	"github.com/vaughan0/go-ini"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"time"
)

type Recorder struct {
	Meta  Meta
	w     io.Writer
	buf   bytes.Buffer
	times []Timing
	start time.Time
	prev  time.Time
	size  int
}

type Timing struct {
	D time.Duration
	L int
}

type Meta struct {
	UserToken string  `json:"user_token,omitempty"`
	Username  string  `json:"username,omitempty"`
	Duration  float64 `json:"duration,omitempty"`
	Title     string  `json:"title,omitempty"`
	Shell     string  `json:"shell,omitempty"`
	Term      struct {
		Type    string `json:"type,omitempty"`
		Lines   int    `json:"lines,omitempty"`
		Columns int    `json:"columns,omitempty"`
	} `json:"term,omitempty"`
}

func (m *Meta) Populate() {
	m.Username = os.Getenv("USER")
	m.Shell = "/bin/bash"
	m.Term.Type = os.Getenv("TERM")

	r, c, err := pty.Getsize(os.Stdout)
	if err == nil {
		m.Term.Lines = r
		m.Term.Columns = c
	}

	cnf, err := ini.LoadFile(os.Getenv("HOME") + "/.asciinema/config")
	if err == nil {
		token, ok := cnf.Get("api", "token")
		if ok && token != "" {
			m.UserToken = token
		}
	}
}

func NewRecorder(w io.Writer) *Recorder {
	return &Recorder{
		w:     w,
		start: time.Now(),
		prev:  time.Now(),
	}
}

func (r *Recorder) Write(p []byte) (int, error) {
	n, err := r.w.Write(p)
	if err != nil {
		return n, err
	}

	n, err = r.buf.Write(p)
	if err != nil {
		return n, err
	}

	r.size += n

	if n > 1 && p[n-1] == '\r' {
		return n, err
	}

	now := time.Now()
	delta := now.Sub(r.prev)

	// delta is too small
	if delta < 10*time.Millisecond {
		return n, err
	}

	r.times = append(r.times, Timing{delta, r.size})
	r.prev = now
	r.size = 0
	return n, err
}

func (r *Recorder) Flush() {
	r.Meta.Duration = float64(time.Since(r.start)) / float64(time.Second)

	if r.size == 0 {
		return
	}

	now := time.Now()
	delta := now.Sub(r.prev)

	r.times = append(r.times, Timing{delta, r.size})
	r.prev = now
	r.size = 0
}

func (r *Recorder) Upload() error {
	var (
		body  bytes.Buffer
		mpart = multipart.NewWriter(&body)
	)

	{
		w, err := mpart.CreateFormFile("asciicast[stdout]", "stdout")
		if err != nil {
			return err
		}

		c, err := compress(r.buf.Bytes())
		if err != nil {
			return err
		}

		_, err = w.Write(c)
		if err != nil {
			return err
		}
	}

	{
		w, err := mpart.CreateFormFile("asciicast[stdout_timing]", "stdout.time")
		if err != nil {
			return err
		}

		var b bytes.Buffer
		for _, t := range r.times {
			d := float64(t.D) / float64(time.Second)
			fmt.Fprintf(&b, "%f %d\n", d, t.L)
		}

		c, err := compress(b.Bytes())
		if err != nil {
			return err
		}

		_, err = w.Write(c)
		if err != nil {
			return err
		}
	}

	{
		w, err := mpart.CreateFormFile("asciicast[meta]", "meta.json'")
		if err != nil {
			return err
		}

		d, err := json.Marshal(r.Meta)
		if err != nil {
			return err
		}

		_, err = w.Write(d)
		if err != nil {
			return err
		}
	}

	err := mpart.Close()
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", "https://asciinema.org/api/asciicasts", bytes.NewReader(body.Bytes()))
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", mpart.FormDataContentType())

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode/100 != 2 {
		return fmt.Errorf("unexpected status: %s", res.Status)
	}

	body.Reset()
	io.Copy(&body, res.Body)
	fmt.Printf("url: %s\n", body.String())

	return nil
}

func compress(p []byte) ([]byte, error) {
	var buf bytes.Buffer

	cmd := exec.Command("bzip2", "-zc4")
	cmd.Stdin = bytes.NewReader(p)
	cmd.Stdout = &buf
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
