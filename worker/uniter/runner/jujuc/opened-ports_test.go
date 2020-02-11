// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type OpenedPortsSuite struct {
	jujucSuite
}

var _ = gc.Suite(&OpenedPortsSuite{})

func (s *OpenedPortsSuite) TestRunAllFormats(c *gc.C) {
	expectedPorts := []network.PortRange{
		{10, 20, "tcp"},
		{80, 80, "tcp"},
		{53, 55, "udp"},
		{63, 63, "udp"},
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
		defer s.setupMocks(c).Finish()
		s.expectOpenPorts()
		stdout := ""
		stderr := ""
		if format == "" {
			stdout, stderr = s.runCommand(c)
		} else {
			stdout, stderr = s.runCommand(c, "--format", format)
		}
		c.Check(stdout, gc.Equals, expectedOutput)
		c.Check(stderr, gc.Equals, "")
	}
}

func (s *OpenedPortsSuite) TestBadArgs(c *gc.C) {
	com, err := jujuc.NewCommand(nil, cmdString("opened-ports"))
	c.Assert(err, jc.ErrorIsNil)
	err = cmdtesting.InitCommand(jujuc.NewJujucCommandWrappedForTest(com), []string{"foo"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["foo"\]`)
}

func (s *OpenedPortsSuite) TestHelp(c *gc.C) {
	opCmd, err := jujuc.NewCommand(nil, cmdString("opened-ports"))
	c.Assert(err, jc.ErrorIsNil)

	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(opCmd), ctx, []string{"--help"})
	c.Check(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `
Usage: opened-ports [options]

Summary:
lists all ports or ranges opened by the unit

Options:
--format  (= smart)
    Specify output format (json|smart|yaml)
-o, --output (= "")
    Specify an output file

Details:
Each list entry has format <port>/<protocol> (e.g. "80/tcp") or
<from>-<to>/<protocol> (e.g. "8080-8088/udp").
`[1:])
}

func (s *OpenedPortsSuite) runCommand(c *gc.C, args ...string) (stdout, stderr string) {
	com, err := jujuc.NewCommand(s.mockContext, cmdString("opened-ports"))
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, args)
	c.Assert(code, gc.Equals, 0)
	return bufferString(ctx.Stdout), bufferString(ctx.Stderr)
}

type op struct {
	p string
	f int
	t int
}

func (s *OpenedPortsSuite) expectOpenPorts() {
	ports := make([]network.PortRange, 4)
	for i, val := range []op{
		{p: "tcp", f: 10, t: 20},
		{p: "tcp", f: 80, t: 80},
		{p: "udp", f: 53, t: 55},
		{p: "udp", f: 63, t: 63},
	} {
		ports[i] = network.PortRange{
			FromPort: val.f,
			ToPort:   val.t,
			Protocol: val.p,
		}
	}
	s.mockContext.EXPECT().OpenedPorts().Return(ports)
}
