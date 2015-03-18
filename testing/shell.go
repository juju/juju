// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

// CheckWriteFileCommand verifies that the given shell command
// correctly writes the expected content to the given filename. The
// provided parse function decomposes file content into structured data
// that may be correctly compared regardless of ordering within the
// content. If parse is nil then the content lines are used un-parsed.
func CheckWriteFileCommand(c *gc.C, cmd, filename, expected string, parse func(lines []string) interface{}) {
	if parse == nil {
		parse = func(lines []string) interface{} {
			return lines
		}
	}

	lines := strings.Split(strings.TrimSpace(cmd), "\n")
	header := lines[0]
	footer := lines[len(lines)-1]
	parsed := parse(lines[1 : len(lines)-1])

	// Check the cat portion.
	c.Check(header, gc.Equals, "cat > "+filename+" << 'EOF'")
	c.Check(footer, gc.Equals, "EOF")

	// Check the conf portion.
	expectedParsed := parse(strings.Split(expected, "\n"))
	c.Check(parsed, jc.DeepEquals, expectedParsed)
}
