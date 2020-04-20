// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"encoding/base64"
	"regexp"
	"runtime"
	"strings"

	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"
)

var _ = gc.Suite(&DebugCodeSuite{})

type DebugCodeSuite struct {
	SSHCommonSuite
}

func (s *DebugCodeSuite) TestArgFormatting(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: Skipping on windows for now")
	}
	s.setupModel(c)
	s.setHostChecker(validAddresses("0.public"))
	ctx, err := cmdtesting.RunCommand(c, newDebugCodeCommand(s.hostChecker),
		"--at=foo,bar", "mysql/0", "install", "start")
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(string(scriptContent), gc.Not(gc.Equals), "")
	// Inside the script is another base64 encoded string telling us the debug-hook args
	debugArgsRegex := regexp.MustCompile(`echo "([A-Z-a-z0-9+/]+=*)" \| base64.*-debug-hooks`)
	debugArgsCommand := debugArgsRegex.FindString(string(scriptContent))
	debugArgsB64 := debugArgsCommand[len(`echo "`):strings.Index(debugArgsCommand, `" | base64`)]
	yamlContent, err := base64.StdEncoding.DecodeString(debugArgsB64)
	var args map[string]interface{}
	err = goyaml.Unmarshal(yamlContent, &args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(args, gc.DeepEquals, map[string]interface{}{
		"hooks":    []interface{}{"install", "start"},
		"debug-at": "foo,bar",
	})
}
