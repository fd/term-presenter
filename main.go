package main

import (
  "os"

  "github.com/docopt/docopt-go"
)

const usage = `Terminal presenter.

Usage:
  term-present <src>
  term-present -h | --help
  term-present --version

Options:
  -h --help     Show this screen.
  --version     Show version.
`

func main() {
  var (
    args, _ = docopt.Parse(usage, nil, true, "1.0", false)
    src, _  = args["<src>"].(string)
  )

  script, err := ParseFile(src)
  if err != nil {
    Exec(os.Stderr, &OpOops{err.Error()})
    os.Exit(1)
  }

  Exec(os.Stdout, script)
}
