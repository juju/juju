package server_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd/jujuc/server"
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

func pushLog(debug bool) (buf *bytes.Buffer, pop func()) {
	oldTarget, oldDebug := log.Target, log.Debug
	buf = new(bytes.Buffer)
	log.Target, log.Debug = stdlog.New(buf, "", 0), debug
	return buf, func() {
		log.Target, log.Debug = oldTarget, oldDebug
	}
}

func AssertLog(c *C, ctx *server.Context, badge string, logDebug, callDebug, expectMsg bool) {
	buf, pop := pushLog(logDebug)
	defer pop()
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

func AssertLogs(c *C, ctx *server.Context, badge string) {
	AssertLog(c, ctx, badge, true, true, true)
	AssertLog(c, ctx, badge, true, false, true)
	AssertLog(c, ctx, badge, false, true, false)
	AssertLog(c, ctx, badge, false, false, true)
}

func (s *LogSuite) TestLog(c *C) {
	local := &server.Context{LocalUnitName: "minecraft/0"}
	AssertLogs(c, local, "minecraft/0")
	relation := &server.Context{LocalUnitName: "minecraft/0", RelationName: "bot"}
	AssertLogs(c, relation, "minecraft/0 bot")
}

type ExecSuite struct {
	outPath string
}

var _ = Suite(&ExecSuite{})

// makeCharm constructs a fake charm dir containing a single named hook with
// permissions perm and exit code code. It returns the charm directory and the
// path to which the hook script will write environment variables.
func makeCharm(c *C, hookName string, perm os.FileMode, code int) (charmDir, outPath string) {
	charmDir = c.MkDir()
	hooksDir := filepath.Join(charmDir, "hooks")
	err := os.Mkdir(hooksDir, 0755)
	c.Assert(err, IsNil)
	hook, err := os.OpenFile(filepath.Join(hooksDir, hookName), os.O_CREATE|os.O_WRONLY, perm)
	c.Assert(err, IsNil)
	defer hook.Close()
	outPath = filepath.Join(c.MkDir(), "hook.out")
	_, err = fmt.Fprintf(hook, "#!/bin/bash\nenv > %s\nexit %d", outPath, code)
	c.Assert(err, IsNil)
	return charmDir, outPath
}

func AssertEnvContains(c *C, lines []string, env map[string]string) {
	for k, v := range env {
		sought := k + "=" + v
		found := false
		for _, line := range lines {
			if line == sought {
				found = true
				continue
			}
		}
		comment := Commentf("expected to find %v among %v", sought, lines)
		c.Assert(found, Equals, true, comment)
	}
}

func AssertEnv(c *C, outPath string, env map[string]string) {
	out, err := ioutil.ReadFile(outPath)
	c.Assert(err, IsNil)
	lines := strings.Split(string(out), "\n")
	AssertEnvContains(c, lines, env)
	AssertEnvContains(c, lines, map[string]string{
		"PATH":                     os.Getenv("PATH"),
		"DEBIAN_FRONTEND":          "noninteractive",
		"APT_LISTCHANGES_FRONTEND": "none",
	})
}

func (s *ExecSuite) TestNoHook(c *C) {
	ctx := &server.Context{}
	err := ctx.RunHook("tree-fell-in-forest", c.MkDir(), "")
	c.Assert(err, IsNil)
}

func (s *ExecSuite) TestNonExecutableHook(c *C) {
	ctx := &server.Context{}
	charmDir, _ := makeCharm(c, "something-happened", 0600, 0)
	err := ctx.RunHook("something-happened", charmDir, "")
	c.Assert(err, ErrorMatches, `exec: ".*/something-happened": permission denied`)
}

func (s *ExecSuite) TestBadHook(c *C) {
	ctx := &server.Context{Id: "ctx-id"}
	charmDir, outPath := makeCharm(c, "occurrence-occurred", 0700, 99)
	socketPath := "/path/to/socket"
	err := ctx.RunHook("occurrence-occurred", charmDir, socketPath)
	c.Assert(err, ErrorMatches, "exit status 99")
	AssertEnv(c, outPath, map[string]string{
		"CHARM_DIR":         charmDir,
		"JUJU_AGENT_SOCKET": socketPath,
		"JUJU_CONTEXT_ID":   "ctx-id",
	})
}

func (s *ExecSuite) TestGoodHookWithVars(c *C) {
	ctx := &server.Context{
		Id:             "some-id",
		LocalUnitName:  "local/99",
		RemoteUnitName: "remote/123",
		RelationName:   "rel",
	}
	charmDir, outPath := makeCharm(c, "something-happened", 0700, 0)
	socketPath := "/path/to/socket"
	err := ctx.RunHook("something-happened", charmDir, socketPath)
	c.Assert(err, IsNil)
	AssertEnv(c, outPath, map[string]string{
		"CHARM_DIR":         charmDir,
		"JUJU_AGENT_SOCKET": socketPath,
		"JUJU_CONTEXT_ID":   "some-id",
		"JUJU_UNIT_NAME":    "local/99",
		"JUJU_REMOTE_UNIT":  "remote/123",
		"JUJU_RELATION":     "rel",
	})
}
