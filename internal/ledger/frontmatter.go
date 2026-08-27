package ledger

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

const frontmatterDelimiter = "---"

// ErrMalformedFrontmatter is returned when a document appears to open a
// frontmatter block (starts with "---") but the block cannot be fully
// parsed (e.g. no closing delimiter). Callers get this as a typed error
// alongside whatever partial frontmatter/body ParseFrontmatter could
// recover, so tools like `doctor` can report the problem without crashing.
type ErrMalformedFrontmatter struct {
	Reason string
}

func (e *ErrMalformedFrontmatter) Error() string {
	return fmt.Sprintf("ledger: malformed frontmatter: %s", e.Reason)
}

// ParseFrontmatter splits a document into its YAML frontmatter block and
// markdown body. A document with no opening "---" delimiter is treated as
// having no frontmatter at all (not an error): the whole content is
// returned as body.
//
// Parsing is tolerant: if the frontmatter opens but never closes, the
// function still returns the best-effort recovered frontmatter/body plus a
// non-nil *ErrMalformedFrontmatter, and never panics.
func ParseFrontmatter(data []byte) (frontmatter []byte, body []byte, err error) {
	if !hasOpeningDelimiter(data) {
		return nil, data, nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	// Consume the opening delimiter line.
	if !scanner.Scan() {
		return nil, nil, &ErrMalformedFrontmatter{Reason: "empty document"}
	}

	var fmLines []string
	closed := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimRight(line, "\r") == frontmatterDelimiter {
			closed = true
			break
		}
		fmLines = append(fmLines, line)
	}

	frontmatter = []byte(joinLines(fmLines))

	if !closed {
		return frontmatter, nil, &ErrMalformedFrontmatter{Reason: "missing closing delimiter"}
	}

	// Everything after the closing delimiter line is the body.
	rest := readRemainder(scanner)
	return frontmatter, rest, nil
}

// SerializeFrontmatter combines a raw YAML frontmatter block and a markdown
// body back into a document in the "---\n<yaml>---\n<body>" format.
func SerializeFrontmatter(frontmatter []byte, body []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString(frontmatterDelimiter)
	buf.WriteByte('\n')
	buf.Write(frontmatter)
	if len(frontmatter) > 0 && frontmatter[len(frontmatter)-1] != '\n' {
		buf.WriteByte('\n')
	}
	buf.WriteString(frontmatterDelimiter)
	buf.WriteByte('\n')
	buf.Write(body)
	return buf.Bytes()
}

func hasOpeningDelimiter(data []byte) bool {
	firstLine := data
	if idx := bytes.IndexByte(data, '\n'); idx >= 0 {
		firstLine = data[:idx]
	}
	firstLine = bytes.TrimRight(firstLine, "\r")
	return string(firstLine) == frontmatterDelimiter
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// readRemainder drains whatever is left in the scanner's underlying reader
// after the last Scan() call that consumed the closing delimiter line.
func readRemainder(scanner *bufio.Scanner) []byte {
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) == 0 {
		return nil
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}
