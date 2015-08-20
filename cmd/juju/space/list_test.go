// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"regexp"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/space"
	coretesting "github.com/juju/juju/testing"
)

type ListSuite struct {
	BaseSpaceSuite
}

var _ = gc.Suite(&ListSuite{})

func (s *ListSuite) SetUpTest(c *gc.C) {
	s.BaseSpaceSuite.SetUpTest(c)
	s.command = space.NewListCommand(s.api)
	c.Assert(s.command, gc.NotNil)
}

func (s *ListSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		about        string
		args         []string
		expectShort  bool
		expectFormat string
		expectErr    string
	}{{
		about:        "unrecognized arguments",
		args:         s.Strings("foo"),
		expectErr:    `unrecognized args: \["foo"\]`,
		expectFormat: "yaml",
	}, {
		about:        "invalid format",
		args:         s.Strings("--format", "foo"),
		expectErr:    `invalid value "foo" for flag --format: unknown format "foo"`,
		expectFormat: "yaml",
	}, {
		about:        "invalid format (value is case-sensitive)",
		args:         s.Strings("--format", "JSON"),
		expectErr:    `invalid value "JSON" for flag --format: unknown format "JSON"`,
		expectFormat: "yaml",
	}, {
		about:        "json format",
		args:         s.Strings("--format", "json"),
		expectFormat: "json",
	}, {
		about:        "yaml format",
		args:         s.Strings("--format", "yaml"),
		expectFormat: "yaml",
	}, {
		// --output and -o are tested separately in TestOutputFormats.
		about:        "both --output and -o specified (latter overrides former)",
		args:         s.Strings("--output", "foo", "-o", "bar"),
		expectFormat: "yaml",
	}} {
		c.Logf("test #%d: %s", i, test.about)
		// Create a new instance of the subcommand for each test, but
		// since we're not running the command no need to use
		// envcmd.Wrap().
		command := space.NewListCommand(s.api)
		err := coretesting.InitCommand(command, test.args)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
		c.Check(space.ListFormat(command), gc.Equals, test.expectFormat)
		c.Check(command.Short, gc.Equals, test.expectShort)

		// No API calls should be recorded at this stage.
		s.api.CheckCallNames(c)
	}
}

func (s *ListSuite) TestOutputFormats(c *gc.C) {
	outDir := c.MkDir()
	expectedYAML := `
spaces:
  space1:
    2001:db8::/32:
      type: ipv6
      provider-id: subnet-public
      status: terminating
      zones:
      - zone2
    invalid:
      type: unknown
      provider-id: no-such
      status: 'error: invalid subnet CIDR: invalid'
      zones:
      - zone1
  space2:
    4.3.2.0/28:
      type: ipv4
      provider-id: vlan-42
      status: terminating
      zones:
      - zone1
    10.1.2.0/24:
      type: ipv4
      provider-id: subnet-private
      status: in-use
      zones:
      - zone1
      - zone2
`[1:]
	unwrap := regexp.MustCompile(`[\s+\n]`)
	expectedJSON := unwrap.ReplaceAllLiteralString(`
{
  "spaces": {
    "space1": {
      "2001:db8::/32": {
        "type": "ipv6",
        "provider-id": "subnet-public",
        "status": "terminating",
        "zones": ["zone2"]
      },
      "invalid": {
        "type": "unknown",
        "provider-id": "no-such",
        "status": "error: invalid subnet CIDR: invalid",
        "zones": ["zone1"]
      }
    },
    "space2": {
      "10.1.2.0/24": {
        "type": "ipv4",
        "provider-id": "subnet-private",
        "status": "in-use",
        "zones": ["zone1","zone2"]
      },
      "4.3.2.0/28": {
        "type": "ipv4",
        "provider-id": "vlan-42",
        "status": "terminating",
        "zones": ["zone1"]
      }
    }
  }
}
`, "") + "\n"
	// Work around the big unwrap hammer above.
	expectedJSON = strings.Replace(
		expectedJSON,
		"error:invalidsubnetCIDR:invalid",
		"error: invalid subnet CIDR: invalid",
		1,
	)
	expectedShortYAML := `
spaces:
- space1
- space2
`[1:]

	expectedShortJSON := unwrap.ReplaceAllLiteralString(`
{
  "spaces": [
    "space1",
    "space2"
  ]
}
`, "") + "\n"

	assertAPICalls := func() {
		// Verify the API calls and reset the recorded calls.
		s.api.CheckCallNames(c, "ListSpaces", "Close")
		s.api.ResetCalls()
	}
	makeArgs := func(format string, short bool, extraArgs ...string) []string {
		args := s.Strings(extraArgs...)
		if format != "" {
			args = append(args, "--format", format)
		}
		if short == true {
			args = append(args, "--short")
		}
		return args
	}
	assertOutput := func(format, expected string, short bool) {
		outFile := filepath.Join(outDir, "output")
		c.Assert(outFile, jc.DoesNotExist)
		defer os.Remove(outFile)
		// Check -o works.
		var args []string
		args = makeArgs(format, short, "-o", outFile)
		s.AssertRunSucceeds(c, "", "", args...)
		assertAPICalls()

		data, err := ioutil.ReadFile(outFile)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(string(data), gc.Equals, expected)

		// Check the last output argument takes precedence when both
		// -o and --output are given (and also that --output works the
		// same as -o).
		outFile1 := filepath.Join(outDir, "output1")
		c.Assert(outFile1, jc.DoesNotExist)
		defer os.Remove(outFile1)
		outFile2 := filepath.Join(outDir, "output2")
		c.Assert(outFile2, jc.DoesNotExist)
		defer os.Remove(outFile2)
		// Write something in outFile2 to verify its contents are
		// overwritten.
		err = ioutil.WriteFile(outFile2, []byte("some contents"), 0644)
		c.Assert(err, jc.ErrorIsNil)

		args = makeArgs(format, short, "-o", outFile1, "--output", outFile2)
		s.AssertRunSucceeds(c, "", "", args...)
		// Check only the last output file was used, and the output
		// file was overwritten.
		c.Assert(outFile1, jc.DoesNotExist)
		data, err = ioutil.ReadFile(outFile2)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(string(data), gc.Equals, expected)
		assertAPICalls()

		// Finally, check without --output.
		args = makeArgs(format, short)
		s.AssertRunSucceeds(c, "", expected, args...)
		assertAPICalls()
	}

	for i, test := range []struct {
		format   string
		expected string
		short    bool
	}{
		{"", expectedYAML, false}, // default format is YAML
		{"yaml", expectedYAML, false},
		{"json", expectedJSON, false},
		{"", expectedShortYAML, true}, // default format is YAML
		{"yaml", expectedShortYAML, true},
		{"json", expectedShortJSON, true},
	} {
		c.Logf("test #%d: format %q, short %v", i, test.format, test.short)
		assertOutput(test.format, test.expected, test.short)
	}
}

func (s *ListSuite) TestRunWhenNoSpacesExistSucceeds(c *gc.C) {
	s.api.Spaces = s.api.Spaces[0:0]

	s.AssertRunSucceeds(c,
		`no spaces to display\n`,
		"", // empty stdout.
	)

	s.api.CheckCallNames(c, "ListSpaces", "Close")
	s.api.CheckCall(c, 0, "ListSpaces")
}

func (s *ListSuite) TestRunWhenSpacesAPIFails(c *gc.C) {
	s.api.SetErrors(errors.NewNotSupported(nil, "spaces not supported by the provider"))

	err := s.AssertRunFails(c, "cannot list spaces: spaces not supported by the provider")
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)

	s.api.CheckCallNames(c, "ListSpaces", "Close")
	s.api.CheckCall(c, 0, "ListSpaces")
}

func (s *ListSuite) TestRunAPIConnectFails(c *gc.C) {
	s.command = space.NewListCommand(nil)
	s.AssertRunFails(c,
		"cannot connect to the API server: no environment specified",
	)
	// No API calls recoreded.
	s.api.CheckCallNames(c)
}
