package hook_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/hook"
	"launchpad.net/juju/go/log"
	stdlog "log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type LogSuite struct{}

var _ = Suite(&LogSuite{})

func saveLog(debug bool) (*bytes.Buffer, func()) {
	oldTarget, oldDebug := log.Target, log.Debug
	buf := bytes.NewBuffer([]byte{})
	log.Target, log.Debug = stdlog.New(buf, "", 0), debug
	return buf, func() {
		log.Target, log.Debug = oldTarget, oldDebug
	}
}

func AssertLog(c *C, ctx *hook.Context, badge string, logDebug, callDebug, expectMsg bool) {
	buf, restore := saveLog(logDebug)
	defer restore()
	msg := "the chickens are restless"
	ctx.Log(callDebug, msg)
	expect := ""
	if expectMsg {
		var logBadge string
		if callDebug {
			logBadge = "JUJU:DEBUG"
		} else {
			logBadge = "JUJU"
		}
		expect = fmt.Sprintf("%s %s: %s\n", logBadge, badge, msg)
	}
	c.Assert(buf.String(), Equals, expect)
}

func AssertLogs(c *C, ctx *hook.Context, badge string) {
	AssertLog(c, ctx, badge, true, true, true)
	AssertLog(c, ctx, badge, true, false, true)
	AssertLog(c, ctx, badge, false, true, false)
	AssertLog(c, ctx, badge, false, false, true)
}

func (s *LogSuite) TestLog(c *C) {
	local := &hook.Context{Local: "minecraft/0"}
	AssertLogs(c, local, "Context<minecraft/0>")
	relation := &hook.Context{Local: "minecraft/0", Relation: "bot"}
	AssertLogs(c, relation, "Context<minecraft/0, bot>")
}

type ExecSuite struct {
	outPath string
}

var _ = Suite(&ExecSuite{})

var template = `#!/bin/bash
printenv > %s
exit %d
`

func makeCharm(c *C, hookName string, perm os.FileMode, code int) (string, string) {
	charmDir := c.MkDir()
	hooksDir := filepath.Join(charmDir, "hooks")
	err := os.Mkdir(hooksDir, 0755)
	c.Assert(err, IsNil)
	hookPath := filepath.Join(hooksDir, hookName)
	outPath := filepath.Join(c.MkDir(), "hook.out")
	hook := fmt.Sprintf(template, outPath, code)
	err = ioutil.WriteFile(hookPath, []byte(hook), perm)
	c.Assert(err, IsNil)
	return charmDir, outPath
}

func getInfo(charmDir, remoteUnit string) *hook.ExecInfo {
	return &hook.ExecInfo{"ctx-id", "/path/to/socket", charmDir, remoteUnit}
}

func AssertEnvContains(c *C, lines []string, env map[string]string) {
	for k, v := range env {
		sought := k + "=" + v
		found := false
		for i := 0; i < len(lines); i++ {
			if lines[i] == sought {
				found = true
				continue
			}
		}
		comment := Commentf("expected to find %v among %v", sought, lines)
		c.Assert(found, Equals, true, comment)
	}
}

func (s *ExecSuite) AssertEnv(c *C, outPath string, env map[string]string) {
	out, err := ioutil.ReadFile(outPath)
	c.Assert(err, IsNil)
	lines := strings.Split(string(out), "\n")
	AssertEnvContains(c, lines, env)
	AssertEnvContains(c, lines, map[string]string{
		"PATH":                     os.Getenv("PATH"),
		"JUJU_CONTEXT_ID":          "ctx-id",
		"JUJU_AGENT_SOCKET":        "/path/to/socket",
		"DEBIAN_FRONTEND":          "noninteractive",
		"APT_LISTCHANGES_FRONTEND": "none",
	})
}

func (s *ExecSuite) TestNoHook(c *C) {
	info := getInfo(c.MkDir(), "")
	ctx := &hook.Context{}
	err := ctx.Exec("tree-fell-in-forest", info)
	c.Assert(err, IsNil)
}

func (s *ExecSuite) TestNonExecutableHook(c *C) {
	charmDir, _ := makeCharm(c, "something-happened", 0600, 0)
	info := getInfo(charmDir, "")
	ctx := &hook.Context{}
	err := ctx.Exec("something-happened", info)
	c.Assert(err, ErrorMatches, `exec: ".*/something-happened": permission denied`)
}

func (s *ExecSuite) TestBadHook(c *C) {
	charmDir, outPath := makeCharm(c, "occurrence-occurred", 0700, 99)
	info := getInfo(charmDir, "")
	ctx := &hook.Context{}
	err := ctx.Exec("occurrence-occurred", info)
	c.Assert(err, ErrorMatches, "exit status 99")
	s.AssertEnv(c, outPath, map[string]string{"CHARM_DIR": charmDir})
}

func (s *ExecSuite) TestGoodHookWithVars(c *C) {
	charmDir, outPath := makeCharm(c, "something-happened", 0700, 0)
	info := getInfo(charmDir, "remote/123")
	ctx := &hook.Context{Local: "local/99", Relation: "rel"}
	err := ctx.Exec("something-happened", info)
	c.Assert(err, IsNil)
	s.AssertEnv(c, outPath, map[string]string{
		"CHARM_DIR":        charmDir,
		"JUJU_UNIT_NAME":   "local/99",
		"JUJU_RELATION":    "rel",
		"JUJU_REMOTE_UNIT": "remote/123",
	})
}
