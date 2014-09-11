package main

import (
	"fmt"
	"os"

	"github.com/docopt/docopt-go"
)

const usage = `Terminal presenter.

Usage:
  term-present [--upload] <src>
  term-present -h | --help
  term-present --version

Options:
  -h --help     Show this screen.
  --version     Show version.
  -u --upload   Upload this session to asciinema.org.
`

func main() {
	var (
		args, _   = docopt.Parse(usage, nil, true, "1.0", false)
		src, _    = args["<src>"].(string)
		upload, _ = args["--upload"].(bool)
	)

	script, err := ParseFile(src)
	if err != nil {
		Exec(os.Stderr, &OpOops{err.Error()})
		os.Exit(1)
	}

	rec := NewRecorder(os.Stdout)
	rec.Meta.Populate()

	Exec(rec, script)

	rec.Flush()
	if upload {
		err := rec.Upload()
		if err != nil {
			fmt.Printf("error: %s\n", err)
			os.Exit(1)
		}
	}
}
