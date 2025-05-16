// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"encoding/base64"
	"regexp"
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	goyaml "gopkg.in/yaml.v2"

	apicharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/cmd/juju/ssh/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/cmd/cmdtesting"
)

func TestDebugCodeSuite(t *stdtesting.T) { tc.Run(t, &DebugCodeSuite{}) }

type DebugCodeSuite struct {
	SSHMachineSuite
}

func (s *DebugCodeSuite) TestArgFormatting(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ssh, app, status := s.setupModel(ctrl, false, false, nil, nil, "mysql/0")
	app.EXPECT().GetCharmURLOrigin(gomock.Any(), "mysql").Return(charm.MustParseURL("mysql"), apicharm.Origin{}, nil)

	charmAPI := mocks.NewMockCharmAPI(ctrl)
	chInfo := &charms.CharmInfo{Meta: &meta, Actions: &actions}
	charmAPI.EXPECT().CharmInfo(gomock.Any(), "ch:mysql").Return(chInfo, nil)
	charmAPI.EXPECT().Close().Return(nil)

	s.setHostChecker(validAddresses("0.public"))

	debugCmd := NewDebugCodeCommandForTest(app, ssh, status, charmAPI, s.hostChecker, baseTestingRetryStrategy, baseTestingRetryStrategy)

	ctx, err := cmdtesting.RunCommand(c, modelcmd.Wrap(debugCmd), "--at=foo,bar", "mysql/0", "install", "start")
	c.Assert(err, tc.ErrorIsNil)
	base64Regex := regexp.MustCompile("echo ([A-Za-z0-9+/]+=*) \\| base64")
	c.Check(err, tc.ErrorIsNil)
	rawContent := base64Regex.FindString(cmdtesting.Stdout(ctx))
	c.Check(rawContent, tc.Not(tc.Equals), "")
	// Strip off the "echo " and " | base64"
	prefix := "echo "
	suffix := " | base64"
	c.Check(strings.HasPrefix(rawContent, prefix), tc.IsTrue)
	c.Check(strings.HasSuffix(rawContent, suffix), tc.IsTrue)
	b64content := rawContent[len(prefix) : len(rawContent)-len(suffix)]
	scriptContent, err := base64.StdEncoding.DecodeString(b64content)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(scriptContent), tc.Not(tc.Equals), "")
	// Inside the script is another base64 encoded string telling us the debug-hook args
	debugArgsRegex := regexp.MustCompile(`echo "([A-Z-a-z0-9+/]+=*)" \| base64.*-debug-hooks`)
	debugArgsCommand := debugArgsRegex.FindString(string(scriptContent))
	debugArgsB64 := debugArgsCommand[len(`echo "`):strings.Index(debugArgsCommand, `" | base64`)]
	yamlContent, err := base64.StdEncoding.DecodeString(debugArgsB64)
	c.Assert(err, tc.ErrorIsNil)
	var args map[string]interface{}
	err = goyaml.Unmarshal(yamlContent, &args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(args, tc.DeepEquals, map[string]interface{}{
		"hooks":    []interface{}{"install", "start"},
		"debug-at": "foo,bar",
	})
}
