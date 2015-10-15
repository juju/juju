// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/subnet"
	coretesting "github.com/juju/juju/testing"
)

type ListSuite struct {
	BaseSubnetSuite
}

var _ = gc.Suite(&ListSuite{})

func (s *ListSuite) SetUpTest(c *gc.C) {
	s.BaseSubnetSuite.SetUpTest(c)
	s.command, _ = subnet.NewListCommand(s.api)
	c.Assert(s.command, gc.NotNil)
}

func (s *ListSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		about        string
		args         []string
		expectSpace  string
		expectZone   string
		expectFormat string
		expectErr    string
	}{{
		about:        "too many arguments",
		args:         s.Strings("foo", "bar"),
		expectErr:    `unrecognized args: \["foo" "bar"\]`,
		expectFormat: "yaml",
	}, {
		about:        "invalid space name",
		args:         s.Strings("--space", "%inv$alid"),
		expectErr:    `"%inv\$alid" is not a valid space name`,
		expectFormat: "yaml",
	}, {
		about:        "valid space name",
		args:         s.Strings("--space", "my-space"),
		expectSpace:  "my-space",
		expectFormat: "yaml",
	}, {
		about:        "both space and zone given",
		args:         s.Strings("--zone", "zone1", "--space", "my-space"),
		expectSpace:  "my-space",
		expectZone:   "zone1",
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
		wrappedCommand, command := subnet.NewListCommand(s.api)
		err := coretesting.InitCommand(wrappedCommand, test.args)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
		c.Check(command.SpaceName, gc.Equals, test.expectSpace)
		c.Check(command.ZoneName, gc.Equals, test.expectZone)
		c.Check(command.Out.Name(), gc.Equals, test.expectFormat)

		// No API calls should be recorded at this stage.
		s.api.CheckCallNames(c)
	}
}

func (s *ListSuite) TestOutputFormats(c *gc.C) {
	outDir := c.MkDir()
	expectedYAML := `
subnets:
  10.10.0.0/16:
    type: ipv4
    status: terminating
    space: vlan-42
    zones:
    - zone1
  10.20.0.0/24:
    type: ipv4
    provider-id: subnet-foo
    status: in-use
    space: public
    zones:
    - zone1
    - zone2
  2001:db8::/32:
    type: ipv6
    provider-id: subnet-bar
    status: terminating
    space: dmz
    zones:
    - zone2
`[1:]
	expectedJSON := `{"subnets":{` +
		`"10.10.0.0/16":{` +
		`"type":"ipv4",` +
		`"status":"terminating",` +
		`"space":"vlan-42",` +
		`"zones":["zone1"]},` +

		`"10.20.0.0/24":{` +
		`"type":"ipv4",` +
		`"provider-id":"subnet-foo",` +
		`"status":"in-use",` +
		`"space":"public",` +
		`"zones":["zone1","zone2"]},` +

		`"2001:db8::/32":{` +
		`"type":"ipv6",` +
		`"provider-id":"subnet-bar",` +
		`"status":"terminating",` +
		`"space":"dmz",` +
		`"zones":["zone2"]}}}
`

	assertAPICalls := func() {
		// Verify the API calls and reset the recorded calls.
		s.api.CheckCallNames(c, "ListSubnets", "Close")
		s.api.ResetCalls()
	}
	makeArgs := func(format string, extraArgs ...string) []string {
		args := s.Strings(extraArgs...)
		if format != "" {
			args = append(args, "--format", format)
		}
		return args
	}
	assertOutput := func(format, expected string) {
		outFile := filepath.Join(outDir, "output")
		c.Assert(outFile, jc.DoesNotExist)
		defer os.Remove(outFile)
		// Check -o works.
		args := makeArgs(format, "-o", outFile)
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

		args = makeArgs(format, "-o", outFile1, "--output", outFile2)
		s.AssertRunSucceeds(c, "", "", args...)
		// Check only the last output file was used, and the output
		// file was overwritten.
		c.Assert(outFile1, jc.DoesNotExist)
		data, err = ioutil.ReadFile(outFile2)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(string(data), gc.Equals, expected)
		assertAPICalls()

		// Finally, check without --output.
		args = makeArgs(format)
		s.AssertRunSucceeds(c, "", expected, args...)
		assertAPICalls()
	}

	for i, test := range []struct {
		format   string
		expected string
	}{
		{"", expectedYAML}, // default format is YAML
		{"yaml", expectedYAML},
		{"json", expectedJSON},
	} {
		c.Logf("test #%d: format %q", i, test.format)
		assertOutput(test.format, test.expected)
	}
}

func (s *ListSuite) TestRunWhenNoneMatchSucceeds(c *gc.C) {
	// Simulate no subnets are using the "default" space.
	s.api.Subnets = s.api.Subnets[0:0]

	s.AssertRunSucceeds(c,
		`no subnets found matching requested criteria\n`,
		"", // empty stdout.
		"--space", "default",
	)

	s.api.CheckCallNames(c, "ListSubnets", "Close")
	tag := names.NewSpaceTag("default")
	s.api.CheckCall(c, 0, "ListSubnets", &tag, "")
}

func (s *ListSuite) TestRunWhenNoSubnetsExistSucceeds(c *gc.C) {
	s.api.Subnets = s.api.Subnets[0:0]

	s.AssertRunSucceeds(c,
		`no subnets to display\n`,
		"", // empty stdout.
	)

	s.api.CheckCallNames(c, "ListSubnets", "Close")
	s.api.CheckCall(c, 0, "ListSubnets", nil, "")
}

func (s *ListSuite) TestRunWithFilteringSucceeds(c *gc.C) {
	// Simulate one subnet is using the "public" space or "zone1".
	s.api.Subnets = s.api.Subnets[0:1]

	expected := `
subnets:
  10.20.0.0/24:
    type: ipv4
    provider-id: subnet-foo
    status: in-use
    space: public
    zones:
    - zone1
    - zone2
`[1:]

	// Filter by space name first.
	s.AssertRunSucceeds(c,
		"", // empty stderr.
		expected,
		"--space", "public",
	)

	s.api.CheckCallNames(c, "ListSubnets", "Close")
	tag := names.NewSpaceTag("public")
	s.api.CheckCall(c, 0, "ListSubnets", &tag, "")
	s.api.ResetCalls()

	// Now filter by zone.
	s.AssertRunSucceeds(c,
		"", // empty stderr.
		expected,
		"--zone", "zone1",
	)

	s.api.CheckCallNames(c, "ListSubnets", "Close")
	s.api.CheckCall(c, 0, "ListSubnets", nil, "zone1")
	s.api.ResetCalls()

	// Finally, filter by both space and zone.
	s.AssertRunSucceeds(c,
		"", // empty stderr.
		expected,
		"--zone", "zone1", "--space", "public",
	)

	s.api.CheckCallNames(c, "ListSubnets", "Close")
	tag = names.NewSpaceTag("public")
	s.api.CheckCall(c, 0, "ListSubnets", &tag, "zone1")
}

func (s *ListSuite) TestRunWhenListSubnetFails(c *gc.C) {
	s.api.SetErrors(errors.NotSupportedf("foo"))

	// Ensure the error cause is preserved.
	err := s.AssertRunFails(c, "cannot list subnets: foo not supported")
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)

	s.api.CheckCallNames(c, "ListSubnets", "Close")
	s.api.CheckCall(c, 0, "ListSubnets", nil, "")
}

func (s *ListSuite) TestRunWhenASubnetHasInvalidCIDRFails(c *gc.C) {
	// This cannot happen in practice, as CIDRs are validated before
	// adding a subnet, but this test ensures 100% coverage.
	s.api.Subnets = s.api.Subnets[0:1]
	s.api.Subnets[0].CIDR = "invalid"

	s.AssertRunFails(c, `subnet "invalid" has invalid CIDR`)

	s.api.CheckCallNames(c, "ListSubnets", "Close")
	s.api.CheckCall(c, 0, "ListSubnets", nil, "")
}

func (s *ListSuite) TestRunWhenASubnetHasInvalidSpaceFails(c *gc.C) {
	// This cannot happen in practice, as space names are validated
	// before adding a subnet, but this test ensures 100% coverage.
	s.api.Subnets = s.api.Subnets[0:1]
	s.api.Subnets[0].SpaceTag = "foo"

	s.AssertRunFails(c, `subnet "10.20.0.0/24" has invalid space: "foo" is not a valid tag`)

	s.api.CheckCallNames(c, "ListSubnets", "Close")
	s.api.CheckCall(c, 0, "ListSubnets", nil, "")
}
