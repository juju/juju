// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debug_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/worker/apiuniter/debug"
)

type DebugHooksServerSuite struct {
	ctx     *debug.HooksContext
	fakebin string
	tmpdir  string
	oldenv  []string
}

var _ = gc.Suite(&DebugHooksServerSuite{})

// echocommand outputs its name and arguments to stdout for verification,
// and exits with the value of $EXIT_CODE
var echocommand = `#!/bin/bash
echo $(basename $0) $@
exit $EXIT_CODE
`

var fakecommands = []string{"tmux"}

func (s *DebugHooksServerSuite) setenv(key, value string) {
	s.oldenv = append(s.oldenv, key, os.Getenv(key))
	os.Setenv(key, value)
}

func (s *DebugHooksServerSuite) SetUpTest(c *gc.C) {
	s.fakebin = c.MkDir()
	s.tmpdir = c.MkDir()
	s.setenv("PATH", s.fakebin+":"+os.Getenv("PATH"))
	s.setenv("TMPDIR", s.tmpdir)
	s.setenv("TEST_RESULT", "")
	for _, name := range fakecommands {
		err := ioutil.WriteFile(filepath.Join(s.fakebin, name), []byte(echocommand), 0777)
		c.Assert(err, gc.IsNil)
	}
	s.ctx = debug.NewHooksContext("foo/8")
	s.ctx.FlockDir = s.tmpdir
	s.setenv("JUJU_UNIT_NAME", s.ctx.Unit)
}

func (s *DebugHooksServerSuite) TearDownTest(c *gc.C) {
	if len(s.oldenv) > 0 {
		for i := len(s.oldenv) - 2; i >= 0; i = i - 2 {
			os.Setenv(s.oldenv[i], s.oldenv[i+1])
		}
		s.oldenv = nil
	}
}

func (s *DebugHooksServerSuite) TestFindSession(c *gc.C) {
	// Test "tmux has-session" failure. The error
	// message is the output of tmux has-session.
	os.Setenv("EXIT_CODE", "1")
	session, err := s.ctx.FindSession()
	c.Assert(session, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta("tmux has-session -t "+s.ctx.Unit+"\n"))
	os.Setenv("EXIT_CODE", "")

	// tmux session exists, but missing debug-hooks file: error.
	session, err = s.ctx.FindSession()
	c.Assert(session, gc.IsNil)
	c.Assert(err, gc.NotNil)
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	// Hooks file is present, empty.
	err = ioutil.WriteFile(s.ctx.ClientFileLock(), []byte{}, 0777)
	c.Assert(err, gc.IsNil)
	session, err = s.ctx.FindSession()
	c.Assert(session, gc.NotNil)
	c.Assert(err, gc.IsNil)
	// If session.hooks is empty, it'll match anything.
	c.Assert(session.MatchHook(""), jc.IsTrue)
	c.Assert(session.MatchHook("something"), jc.IsTrue)

	// Hooks file is present, non-empty
	err = ioutil.WriteFile(s.ctx.ClientFileLock(), []byte(`hooks: [foo, bar, baz]`), 0777)
	c.Assert(err, gc.IsNil)
	session, err = s.ctx.FindSession()
	c.Assert(session, gc.NotNil)
	c.Assert(err, gc.IsNil)
	// session should only match "foo", "bar" or "baz".
	c.Assert(session.MatchHook(""), jc.IsFalse)
	c.Assert(session.MatchHook("something"), jc.IsFalse)
	c.Assert(session.MatchHook("foo"), jc.IsTrue)
	c.Assert(session.MatchHook("bar"), jc.IsTrue)
	c.Assert(session.MatchHook("baz"), jc.IsTrue)
	c.Assert(session.MatchHook("foo bar baz"), jc.IsFalse)
}

func (s *DebugHooksServerSuite) TestRunHookExceptional(c *gc.C) {
        c.Skip("this fails occasionally on the bot bug #1224768 (timing related)")
	err := ioutil.WriteFile(s.ctx.ClientFileLock(), []byte{}, 0777)
	c.Assert(err, gc.IsNil)
	session, err := s.ctx.FindSession()
	c.Assert(session, gc.NotNil)
	c.Assert(err, gc.IsNil)

	// Run the hook in debug mode with no exit flock held.
	// The exit flock will be acquired immediately, and the
	// debug-hooks server process killed.
	err = session.RunHook("myhook", s.tmpdir, os.Environ())
	c.Assert(err, gc.ErrorMatches, "signal: killed")

	// Run the hook in debug mode with the exit flock held.
	// This simulates the client process starting but not
	// cleanly exiting (normally the .pid file is updated,
	// and the server waits on the client process' death).
	cmd := exec.Command("flock", s.ctx.ClientExitFileLock(), "-c", "sleep 1s")
	c.Assert(cmd.Start(), gc.IsNil)
	expected := time.Now().Add(time.Second)
	err = session.RunHook("myhook", s.tmpdir, os.Environ())
	after := time.Now()
	c.Assert(after, jc.TimeBetween(expected.Add(-100*time.Millisecond), expected.Add(100*time.Millisecond)))
	c.Assert(err, gc.ErrorMatches, "signal: killed")
	c.Assert(cmd.Wait(), gc.IsNil)
}

func (s *DebugHooksServerSuite) TestRunHook(c *gc.C) {
        c.Skip("this fails occasionally on the bot bug #1224768 (unable to find hook.sh)")
	err := ioutil.WriteFile(s.ctx.ClientFileLock(), []byte{}, 0777)
	c.Assert(err, gc.IsNil)
	session, err := s.ctx.FindSession()
	c.Assert(session, gc.NotNil)
	c.Assert(err, gc.IsNil)

	const hookName = "myhook"

	// Run the hook in debug mode with the exit flock held,
	// and also create the .pid file. We'll populate it with
	// an invalid PID; this will cause the server process to
	// exit cleanly (as if the PID were real and no longer running).
	cmd := exec.Command("flock", s.ctx.ClientExitFileLock(), "-c", "sleep 5s")
	c.Assert(cmd.Start(), gc.IsNil)
	ch := make(chan error)
	go func() {
		ch <- session.RunHook(hookName, s.tmpdir, os.Environ())
	}()
	var debugdir os.FileInfo
	for i := 0; i < 10; i++ {
		tmpdir, err := os.Open(s.tmpdir)
		if err != nil {
			c.Fatalf("Failed to open $TMPDIR: %s", err)
		}
		fi, err := tmpdir.Readdir(-1)
		if err != nil {
			c.Fatalf("Failed to read $TMPDIR: %s", err)
		}
		tmpdir.Close()
		for _, fi := range fi {
			if fi.IsDir() {
				hooksh := filepath.Join(s.tmpdir, fi.Name(), "hook.sh")
				if _, err = os.Stat(hooksh); err == nil {
					debugdir = fi
					break
				}
			}
		}
		if debugdir != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if debugdir == nil {
		c.Error("could not find hook.sh")
	} else {
		envsh := filepath.Join(s.tmpdir, debugdir.Name(), "env.sh")
		s.verifyEnvshFile(c, envsh, hookName)

		hookpid := filepath.Join(s.tmpdir, debugdir.Name(), "hook.pid")
		err = ioutil.WriteFile(hookpid, []byte("not a pid"), 0777)
		c.Assert(err, gc.IsNil)

		// RunHook should complete without waiting to be
		// killed, and despite the exit lock being held.
		err = <-ch
		c.Assert(err, gc.IsNil)
	}
	cmd.Process.Kill() // kill flock
}

func (s *DebugHooksServerSuite) verifyEnvshFile(c *gc.C, envshPath string, hookName string) {
	data, err := ioutil.ReadFile(envshPath)
	c.Assert(err, gc.IsNil)
	contents := string(data)
	c.Assert(contents, jc.Contains, fmt.Sprintf("JUJU_UNIT_NAME=%q", s.ctx.Unit))
	c.Assert(contents, jc.Contains, fmt.Sprintf("JUJU_HOOK_NAME=%q", hookName))
	c.Assert(contents, jc.Contains, fmt.Sprintf(`PS1="%s:%s %% "`, s.ctx.Unit, hookName))
}
