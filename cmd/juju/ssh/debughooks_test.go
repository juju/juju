// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"context"
	"encoding/base64"
	"regexp"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	goyaml "gopkg.in/yaml.v2"

	apicharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/cmd/juju/ssh/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	jujussh "github.com/juju/juju/internal/network/ssh"
)

var _ = tc.Suite(&DebugHooksSuite{})

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
		argsMatch:       `ubuntu@0\.(private|public|1\.2\.3) exec sudo .+`, // can be any of the 3
	},
}, {
	info:        "proxy",
	args:        []string{"--proxy=true", "mysql/0"},
	hostChecker: validAddresses("0.private", "0.public", "0.1.2.3"), // set by setAddresses() and setLinkLayerDevicesAddresses()
	expected: &argsSpec{
		hostKeyChecking: "yes",
		knownHosts:      "0",
		withProxy:       true,
		argsMatch:       `ubuntu@0\.(private|public|1\.2\.3) exec sudo .+`, // can be any of the 3
	},
}, {
	info:        "pty enabled",
	args:        []string{"--pty=true", "mysql/0"},
	hostChecker: validAddresses("0.private", "0.public", "0.1.2.3"), // set by setAddresses() and setLinkLayerDevicesAddresses()
	expected: &argsSpec{
		hostKeyChecking: "yes",
		knownHosts:      "0",
		enablePty:       true,
		argsMatch:       `ubuntu@0\.(private|public|1\.2\.3) exec sudo .+`, // can be any of the 3
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
	error: `unit "mysql/0" contains neither hook nor action "invalid-hook", valid actions are \[anotherfakeaction fakeaction\] and valid hooks are .*`,
}, {
	info:  `no args at all`,
	args:  nil,
	error: `no unit name specified`,
}}

var meta = charm.Meta{
	Provides: map[string]charm.Relation{
		"server":       {Name: "server", Interface: "mysql", Role: charm.RoleProvider},
		"server-admin": {Name: "server", Interface: "mysql", Role: charm.RoleProvider},
	},
}

var actions = charm.Actions{
	ActionSpecs: map[string]charm.ActionSpec{
		"fakeaction":        {},
		"anotherfakeaction": {},
	},
}

func (s *DebugHooksSuite) TestDebugHooksCommand(c *tc.C) {

	for i, t := range debugHooksTests {
		c.Logf("test %d: %s\n\t%s\n", i, t.info, t.args)

		s.setHostChecker(t.hostChecker)

		ctrl := gomock.NewController(c)
		withProxy := false
		if t.expected != nil {
			withProxy = t.expected.withProxy
		}
		target := "mysql/0"
		if len(t.args) > 0 && t.args[0] == "nonexistent/123" {
			target = t.args[0]
		}
		ssh, app, status := s.setupModel(ctrl, withProxy, nil, nil, target)
		app.EXPECT().GetCharmURLOrigin(gomock.Any(), "mysql").DoAndReturn(func(ctx context.Context, curl string) (*charm.URL, apicharm.Origin, error) {
			if curl != "mysql" {
				return nil, apicharm.Origin{}, errors.NotFoundf(curl)
			}
			return charm.MustParseURL("mysql"), apicharm.Origin{}, nil
		})

		charmAPI := mocks.NewMockCharmAPI(ctrl)
		chInfo := &charms.CharmInfo{Meta: &meta, Actions: &actions}
		charmAPI.EXPECT().CharmInfo(gomock.Any(), "ch:mysql").Return(chInfo, nil)
		charmAPI.EXPECT().Close().Return(nil)

		hooksCmd := NewDebugHooksCommandForTest(app, ssh, status, charmAPI, t.hostChecker, baseTestingRetryStrategy, baseTestingRetryStrategy)

		ctx, err := cmdtesting.RunCommand(c, modelcmd.Wrap(hooksCmd), t.args...)
		if t.error != "" {
			c.Check(err, tc.ErrorMatches, t.error)
		} else {
			c.Check(err, jc.ErrorIsNil)
			if t.expected != nil {
				t.expected.check(c, cmdtesting.Stdout(ctx))
			}
		}
	}
}

func (s *DebugHooksSuite) TestDebugHooksArgFormatting(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ssh, app, status := s.setupModel(ctrl, false, nil, nil, "mysql/0")
	app.EXPECT().GetCharmURLOrigin(gomock.Any(), "mysql").Return(charm.MustParseURL("mysql"), apicharm.Origin{}, nil)

	charmAPI := mocks.NewMockCharmAPI(ctrl)
	chInfo := &charms.CharmInfo{Meta: &meta, Actions: &actions}
	charmAPI.EXPECT().CharmInfo(gomock.Any(), "ch:mysql").Return(chInfo, nil)
	charmAPI.EXPECT().Close().Return(nil)

	s.setHostChecker(validAddresses("0.public"))

	hooksCmd := NewDebugHooksCommandForTest(app, ssh, status, charmAPI, s.hostChecker, baseTestingRetryStrategy, baseTestingRetryStrategy)

	ctx, err := cmdtesting.RunCommand(c, modelcmd.Wrap(hooksCmd), "mysql/0", "install", "start")
	c.Check(err, jc.ErrorIsNil)
	base64Regex := regexp.MustCompile("echo ([A-Za-z0-9+/]+=*) \\| base64")
	c.Check(err, jc.ErrorIsNil)
	rawContent := base64Regex.FindString(cmdtesting.Stdout(ctx))
	c.Check(rawContent, tc.Not(tc.Equals), "")
	// Strip off the "echo " and " | base64"
	prefix := "echo "
	suffix := " | base64"
	c.Check(strings.HasPrefix(rawContent, prefix), jc.IsTrue)
	c.Check(strings.HasSuffix(rawContent, suffix), jc.IsTrue)
	b64content := rawContent[len(prefix) : len(rawContent)-len(suffix)]
	scriptContent, err := base64.StdEncoding.DecodeString(b64content)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(scriptContent), tc.Not(tc.Equals), "")
	// Inside the script is another base64 encoded string telling us the debug-hook args
	debugArgsRegex := regexp.MustCompile(`echo "([A-Z-a-z0-9+/]+=*)" \| base64.*-debug-hooks`)
	debugArgsCommand := debugArgsRegex.FindString(string(scriptContent))
	debugArgsB64 := debugArgsCommand[len(`echo "`):strings.Index(debugArgsCommand, `" | base64`)]
	yamlContent, err := base64.StdEncoding.DecodeString(debugArgsB64)
	c.Assert(err, jc.ErrorIsNil)
	var args map[string]interface{}
	err = goyaml.Unmarshal(yamlContent, &args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(args, tc.DeepEquals, map[string]interface{}{
		"hooks": []interface{}{"install", "start"},
	})
}
