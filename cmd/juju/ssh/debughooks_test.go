// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"encoding/base64"
	"regexp"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/retry"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	jujussh "github.com/juju/juju/network/ssh"
)

var _ = gc.Suite(&DebugHooksSuite{})

type DebugHooksSuite struct {
	SSHMachineSuite
}

var baseTestingRetryStrategy = retry.CallArgs{
	Clock:    clock.WallClock,
	Attempts: 5,
	Delay:    time.Millisecond,
}

var debugHooksTests = []struct {
	info        string
	args        []string
	hostChecker jujussh.ReachableChecker
	error       string
	expected    *argsSpec
}{{
	info:        "unit name without hook",
	args:        []string{"mysql/0"},
	hostChecker: validAddresses("0.private", "0.public", "0.1.2.3"), // set by setAddresses() and setLinkLayerDevicesAddresses()
	expected: &argsSpec{
		hostKeyChecking: "yes",
		knownHosts:      "0",
		argsMatch:       `ubuntu@0\.(private|public|1\.2\.3) sudo .+`, // can be any of the 3
	},
}, {
	info:        "proxy",
	args:        []string{"--proxy=true", "mysql/0"},
	hostChecker: validAddresses("0.private", "0.public", "0.1.2.3"), // set by setAddresses() and setLinkLayerDevicesAddresses()
	expected: &argsSpec{
		hostKeyChecking: "yes",
		knownHosts:      "0",
		withProxy:       true,
		argsMatch:       `ubuntu@0\.(private|public|1\.2\.3) sudo .+`, // can be any of the 3
	},
}, {
	info:        "pty enabled",
	args:        []string{"--pty=true", "mysql/0"},
	hostChecker: validAddresses("0.private", "0.public", "0.1.2.3"), // set by setAddresses() and setLinkLayerDevicesAddresses()
	expected: &argsSpec{
		hostKeyChecking: "yes",
		knownHosts:      "0",
		enablePty:       true,
		argsMatch:       `ubuntu@0\.(private|public|1\.2\.3) sudo .+`, // can be any of the 3
	},
}, {
	info:        `"*" is a valid hook name: it means hook everything`,
	args:        []string{"mysql/0", "*"},
	hostChecker: validAddresses("0.public"),
	expected:    nil,
}, {
	info:        `"*" mixed with named hooks is equivalent to "*"`,
	args:        []string{"mysql/0", "*", "relation-get"},
	hostChecker: validAddresses("0.public"),
	expected:    nil,
}, {
	info:        `multiple named hooks may be specified`,
	args:        []string{"mysql/0", "start", "stop"},
	hostChecker: validAddresses("0.public"),
	expected:    nil,
}, {
	info:        `relation hooks have the relation name prefixed`,
	args:        []string{"mysql/0", "juju-info-relation-joined"},
	hostChecker: validAddresses("0.public"),
	expected:    nil,
}, {
	info:  `invalid unit syntax`,
	args:  []string{"mysql"},
	error: `"mysql" is not a valid unit name`,
}, {
	info:  `invalid unit`,
	args:  []string{"nonexistent/123"},
	error: `unit "nonexistent/123" not found`,
}, {
	info:  `invalid hook`,
	args:  []string{"mysql/0", "invalid-hook"},
	error: `unit "mysql/0" contains neither hook nor action "invalid-hook", valid actions are [anotherfakeaction fakeaction] and valid hooks are [collect-metrics config-changed install juju-info-relation-broken juju-info-relation-changed juju-info-relation-created juju-info-relation-departed juju-info-relation-joined leader-deposed leader-elected leader-settings-changed meter-status-changed metrics-client-relation-broken metrics-client-relation-changed metrics-client-relation-created metrics-client-relation-departed metrics-client-relation-joined post-series-upgrade pre-series-upgrade remove server-admin-relation-broken server-admin-relation-changed server-admin-relation-created server-admin-relation-departed server-admin-relation-joined server-relation-broken server-relation-changed server-relation-created server-relation-departed server-relation-joined start stop update-status upgrade-charm]`,
}, {
	info:  `no args at all`,
	args:  nil,
	error: `no unit name specified`,
}}

func (s *DebugHooksSuite) TestDebugHooksCommand(c *gc.C) {
	s.setupModel(c)

	for i, t := range debugHooksTests {
		c.Logf("test %d: %s\n\t%s\n", i, t.info, t.args)

		s.setHostChecker(t.hostChecker)

		ctx, err := cmdtesting.RunCommand(c, NewDebugHooksCommand(s.hostChecker, baseTestingRetryStrategy), t.args...)
		if t.error != "" {
			c.Check(err, gc.ErrorMatches, regexp.QuoteMeta(t.error))
		} else {
			c.Check(err, jc.ErrorIsNil)
			if t.expected != nil {
				t.expected.check(c, cmdtesting.Stdout(ctx))
			}
		}
	}
}

func (s *DebugHooksSuite) TestDebugHooksArgFormatting(c *gc.C) {
	s.setupModel(c)
	s.setHostChecker(validAddresses("0.public"))
	ctx, err := cmdtesting.RunCommand(c, NewDebugHooksCommand(s.hostChecker, baseTestingRetryStrategy),
		"mysql/0", "install", "start")
	c.Check(err, jc.ErrorIsNil)
	base64Regex := regexp.MustCompile("echo ([A-Za-z0-9+/]+=*) \\| base64")
	c.Check(err, jc.ErrorIsNil)
	rawContent := base64Regex.FindString(cmdtesting.Stdout(ctx))
	c.Check(rawContent, gc.Not(gc.Equals), "")
	// Strip off the "echo " and " | base64"
	prefix := "echo "
	suffix := " | base64"
	c.Check(strings.HasPrefix(rawContent, prefix), jc.IsTrue)
	c.Check(strings.HasSuffix(rawContent, suffix), jc.IsTrue)
	b64content := rawContent[len(prefix) : len(rawContent)-len(suffix)]
	scriptContent, err := base64.StdEncoding.DecodeString(b64content)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(scriptContent), gc.Not(gc.Equals), "")
	// Inside the script is another base64 encoded string telling us the debug-hook args
	debugArgsRegex := regexp.MustCompile(`echo "([A-Z-a-z0-9+/]+=*)" \| base64.*-debug-hooks`)
	debugArgsCommand := debugArgsRegex.FindString(string(scriptContent))
	debugArgsB64 := debugArgsCommand[len(`echo "`):strings.Index(debugArgsCommand, `" | base64`)]
	yamlContent, err := base64.StdEncoding.DecodeString(debugArgsB64)
	c.Assert(err, jc.ErrorIsNil)
	var args map[string]interface{}
	err = goyaml.Unmarshal(yamlContent, &args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(args, gc.DeepEquals, map[string]interface{}{
		"hooks": []interface{}{"install", "start"},
	})
}
