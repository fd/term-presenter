package main

import (
	"errors"
	"io/ioutil"
	"strings"
)

func ParseFile(name string) (Script, error) {
	data, err := ioutil.ReadFile(name)
	if err != nil {
		return nil, err
	}

	return Parse(string(data))
}

func Parse(source string) (Script, error) {
	var (
		script  Script
		lines   = strings.Split(source, "\n")
		lastRun *OpExec
	)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if ignoreLine(line) {
			continue
		}

		if strings.HasPrefix(line, "- ") {
			if lastRun == nil {
				return nil, errors.New("unable to interpret the script.")
			}

			line = strings.TrimSpace(line[2:])
			op, err := parseSubLine(line)
			if err != nil {
				return nil, err
			}

			lastRun.Ops = append(lastRun.Ops, op)

		} else {
			lastRun = nil
			op, err := parseLine(line)
			if err != nil {
				return nil, err
			}

			if x, ok := op.(*OpExec); ok {
				lastRun = x
			}

			script = append(script, op)
		}
	}

	return script, nil
}

func ignoreLine(line string) bool {
	return strings.HasPrefix(line, "#") || line == ""
}

func parseLine(line string) (Op, error) {
	switch {

	case strings.HasPrefix(line, "SAY ") && len(line) > 4:
		return &OpEcho{line[4:]}, nil

	case strings.HasPrefix(line, "RUN ") && len(line) > 4:
		return &OpExec{line[4:], nil}, nil

	case line == "BREATH":
		return &OpBreath{nl: true}, nil

	default:
		return nil, errors.New("unable to interpret the script.")

	}
}

func parseSubLine(line string) (Op, error) {
	switch {

	case strings.HasPrefix(line, "TYPE ") && len(line) > 5:
		line = replaceEscapeSequences(line[5:])
		return &OpType{line}, nil

	case line == "BREATH":
		return &OpBreath{}, nil

	default:
		return nil, errors.New("unable to interpret the script.")

	}
}

func replaceEscapeSequences(s string) string {
	r := strings.NewReplacer(
		"\\e", "\x1B",
		"\\n", "\n",
		"\\t", "\t",
		"\\v", "\v",
		"\\\\", "\\",
		"\\NUL", "\x00", // Null char
		"\\SOH", "\x01", // Start of Heading
		"\\STX", "\x02", // Start of Text
		"\\ETX", "\x03", // End of Text
		"\\EOT", "\x04", // End of Transmission
		"\\ENQ", "\x05", // Enquiry
		"\\ACK", "\x06", // Acknowledgment
		"\\BEL", "\x07", // Bell
		"\\BS", "\x08", // Back Space
		"\\HT", "\x09", // Horizontal Tab
		"\\LF", "\x0A", // Line Feed
		"\\VT", "\x0B", // Vertical Tab
		"\\FF", "\x0C", // Form Feed
		"\\CR", "\x0D", // Carriage Return
		"\\SO", "\x0E", // Shift Out / X-On
		"\\SI", "\x0F", // Shift In / X-Off
		"\\DLE", "\x10", // Data Line Escape
		"\\DC1", "\x11", // Device Control 1 (oft. XON)
		"\\DC2", "\x12", // Device Control 2
		"\\DC3", "\x13", // Device Control 3 (oft. XOFF)
		"\\DC4", "\x14", // Device Control 4
		"\\NAK", "\x15", // Negative Acknowledgement
		"\\SYN", "\x16", // Synchronous Idle
		"\\ETB", "\x17", // End of Transmit Block
		"\\CAN", "\x18", // Cancel
		"\\EM", "\x19", // End of Medium
		"\\SUB", "\x1A", // Substitute
		"\\ESC", "\x1B", // Escape
		"\\FS", "\x1C", // File Separator
		"\\GS", "\x1D", // Group Separator
		"\\RS", "\x1E", // Record Separator
		"\\US", "\x1F", // Unit Separator
	)
	return r.Replace(s)
}
