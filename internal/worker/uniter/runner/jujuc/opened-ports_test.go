// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"strings"

	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type OpenedPortsSuite struct {
	ContextSuite
}

var _ = gc.Suite(&OpenedPortsSuite{})

func (s *OpenedPortsSuite) TestRunAllFormats(c *gc.C) {
	expectedPorts := []network.PortRange{
		network.MustParsePortRange("10-20/tcp"),
		network.MustParsePortRange("80/tcp"),
		network.MustParsePortRange("53-55/udp"),
		network.MustParsePortRange("63/udp"),
	}
	network.SortPortRanges(expectedPorts)
	portsAsStrings := make([]string, len(expectedPorts))
	for i, portRange := range expectedPorts {
		portsAsStrings[i] = portRange.String()
	}
	defaultOutput := strings.Join(portsAsStrings, "\n") + "\n"
	jsonOutput := `["` + strings.Join(portsAsStrings, `","`) + `"]` + "\n"
	yamlOutput := "- " + strings.Join(portsAsStrings, "\n- ") + "\n"

	formatToOutput := map[string]string{
		"":      defaultOutput,
		"smart": defaultOutput,
		"json":  jsonOutput,
		"yaml":  yamlOutput,
	}
	for format, expectedOutput := range formatToOutput {
		c.Logf("testing format %q", format)
		hctx := s.getContextAndOpenPorts(c)
		stdout := ""
		stderr := ""
		if format == "" {
			stdout, stderr = s.runCommand(c, hctx)
		} else {
			stdout, stderr = s.runCommand(c, hctx, "--format", format)
		}
		c.Check(stdout, gc.Equals, expectedOutput)
		c.Check(stderr, gc.Equals, "")
	}
}

func (s *OpenedPortsSuite) TestRunAllFormatsWithEndpointDetails(c *gc.C) {
	portsAsStrings := []string{
		"10-20/tcp (foo)",
		"80/tcp (*)",
		"53-55/udp (*)",
		"63/udp (bar)",
	}
	defaultOutput := strings.Join(portsAsStrings, "\n") + "\n"
	jsonOutput := `["` + strings.Join(portsAsStrings, `","`) + `"]` + "\n"
	yamlOutput := "- " + strings.Join(portsAsStrings, "\n- ") + "\n"

	formatToOutput := map[string]string{
		"":      defaultOutput,
		"smart": defaultOutput,
		"json":  jsonOutput,
		"yaml":  yamlOutput,
	}
	for format, expectedOutput := range formatToOutput {
		c.Logf("testing format %q", format)
		hctx := s.getContextAndOpenPorts(c)
		stdout := ""
		stderr := ""
		if format == "" {
			stdout, stderr = s.runCommand(c, hctx, "--endpoints")
		} else {
			stdout, stderr = s.runCommand(c, hctx, "--endpoints", "--format", format)
		}
		c.Check(stdout, gc.Equals, expectedOutput)
		c.Check(stderr, gc.Equals, "")
	}
}

func (s *OpenedPortsSuite) TestBadArgs(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "opened-ports")
	c.Assert(err, jc.ErrorIsNil)
	err = cmdtesting.InitCommand(jujuc.NewJujucCommandWrappedForTest(com), []string{"foo"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["foo"\]`)
}

func (s *OpenedPortsSuite) TestHelp(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	openedPorts, err := jujuc.NewCommand(hctx, "opened-ports")
	c.Assert(err, jc.ErrorIsNil)
	flags := cmdtesting.NewFlagSet()
	c.Assert(string(openedPorts.Info().Help(flags)), gc.Equals, `
Usage: opened-ports

Summary:
list all ports or port ranges opened by the unit

Details:
opened-ports lists all ports or port ranges opened by a unit.

By default, the port range listing does not include information about the 
application endpoints that each port range applies to. Each list entry is
formatted as <port>/<protocol> (e.g. "80/tcp") or <from>-<to>/<protocol> 
(e.g. "8080-8088/udp").

If the --endpoints option is specified, each entry in the port list will be
augmented with a comma-delimited list of endpoints that the port range 
applies to (e.g. "80/tcp (endpoint1, endpoint2)"). If a port range applies to
all endpoints, this will be indicated by the presence of a '*' character
(e.g. "80/tcp (*)").
`[1:])
}

func (s *OpenedPortsSuite) getContextAndOpenPorts(c *gc.C) *Context {
	hctx := s.GetHookContext(c, -1, "")
	hctx.OpenPortRange("", network.MustParsePortRange("80/tcp"))
	hctx.OpenPortRange("foo", network.MustParsePortRange("10-20/tcp"))
	hctx.OpenPortRange("bar", network.MustParsePortRange("63/udp"))
	hctx.OpenPortRange("", network.MustParsePortRange("53-55/udp"))
	return hctx
}

func (s *OpenedPortsSuite) runCommand(c *gc.C, hctx *Context, args ...string) (stdout, stderr string) {
	com, err := jujuc.NewCommand(hctx, "opened-ports")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, args)
	c.Assert(code, gc.Equals, 0)
	return bufferString(ctx.Stdout), bufferString(ctx.Stderr)
}
