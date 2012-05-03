package server_test

import (
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd/jujuc/server"
	"launchpad.net/juju/go/state"
	"os"
	"path/filepath"
	"strings"
)

type GetCommandSuite struct{}

var _ = Suite(&GetCommandSuite{})

var getCommandTests = []struct {
	name string
	err  string
}{
	{"close-port", ""},
	{"config-get", ""},
	{"juju-log", ""},
	{"open-port", ""},
	{"unit-get", ""},
	{"random", "unknown command: random"},
}

func (s *GetCommandSuite) TestGetCommand(c *C) {
	ctx := &server.Context{
		Id:            "ctxid",
		State:         &state.State{},
		LocalUnitName: "minecraft/0",
	}
	for _, t := range getCommandTests {
		com, err := ctx.NewCommand(t.name)
		if t.err == "" {
			// At this level, just check basic sanity; commands are tested in
			// more detail elsewhere.
			c.Assert(err, IsNil)
			c.Assert(com.Info().Name, Equals, t.name)
		} else {
			c.Assert(com, IsNil)
			c.Assert(err, ErrorMatches, t.err)
		}
	}
}

type RunHookSuite struct {
	outPath string
}

var _ = Suite(&RunHookSuite{})

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

func (s *RunHookSuite) TestNoHook(c *C) {
	ctx := &server.Context{}
	err := ctx.RunHook("tree-fell-in-forest", c.MkDir(), "")
	c.Assert(err, IsNil)
}

func (s *RunHookSuite) TestNonExecutableHook(c *C) {
	ctx := &server.Context{}
	charmDir, _ := makeCharm(c, "something-happened", 0600, 0)
	err := ctx.RunHook("something-happened", charmDir, "")
	c.Assert(err, ErrorMatches, `exec: ".*/something-happened": permission denied`)
}

func (s *RunHookSuite) TestBadHook(c *C) {
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

func (s *RunHookSuite) TestGoodHookWithVars(c *C) {
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
