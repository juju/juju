// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debug

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type DebugHooksServerSuite struct {
	testing.BaseSuite
	ctx     *HooksContext
	fakebin string
	tmpdir  string
}

var _ = gc.Suite(&DebugHooksServerSuite{})

// echocommand outputs its name and arguments to stdout for verification,
// and exits with the value of $EXIT_CODE
var echocommand = `#!/bin/bash --norc
echo $(basename $0) $@
exit $EXIT_CODE
`

var fakecommands = []string{"tmux"}

func (s *DebugHooksServerSuite) SetUpTest(c *gc.C) {
	s.fakebin = c.MkDir()
	s.tmpdir = c.MkDir()
	s.PatchEnvPathPrepend(s.fakebin)
	s.PatchEnvironment("TMPDIR", s.tmpdir)
	s.PatchEnvironment("TEST_RESULT", "")
	for _, name := range fakecommands {
		err := ioutil.WriteFile(filepath.Join(s.fakebin, name), []byte(echocommand), 0777)
		c.Assert(err, jc.ErrorIsNil)
	}
	s.ctx = NewHooksContext("foo/8")
	s.ctx.FlockDir = s.tmpdir
	s.PatchEnvironment("JUJU_UNIT_NAME", s.ctx.Unit)
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
	c.Assert(err, jc.ErrorIsNil)
	session, err = s.ctx.FindSession()
	c.Assert(session, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)
	// If session.hooks is empty, it'll match anything.
	c.Assert(session.MatchHook(""), jc.IsTrue)
	c.Assert(session.MatchHook("something"), jc.IsTrue)

	// Hooks file is present, non-empty
	err = ioutil.WriteFile(s.ctx.ClientFileLock(), []byte(`hooks: [foo, bar, baz]`), 0777)
	c.Assert(err, jc.ErrorIsNil)
	session, err = s.ctx.FindSession()
	c.Assert(session, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)
	// session should only match "foo", "bar" or "baz".
	c.Assert(session.MatchHook(""), jc.IsFalse)
	c.Assert(session.MatchHook("something"), jc.IsFalse)
	c.Assert(session.MatchHook("foo"), jc.IsTrue)
	c.Assert(session.MatchHook("bar"), jc.IsTrue)
	c.Assert(session.MatchHook("baz"), jc.IsTrue)
	c.Assert(session.MatchHook("foo bar baz"), jc.IsFalse)
}

func (s *DebugHooksServerSuite) TestRunHookExceptional(c *gc.C) {
	err := ioutil.WriteFile(s.ctx.ClientFileLock(), []byte{}, 0777)
	c.Assert(err, jc.ErrorIsNil)
	session, err := s.ctx.FindSession()
	c.Assert(session, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)

	flockAcquired := make(chan struct{}, 1)
	waitForFlock := func() {
		select {
		case <-flockAcquired:
		case <-time.After(testing.ShortWait):
			c.Fatalf("timed out waiting for hook to acquire flock")
		}
	}

	// Run the hook in debug mode with no exit flock held.
	// The exit flock will be acquired immediately, and the
	// debug-hooks server process killed.
	s.PatchValue(&waitClientExit, func(*ServerSession) {
		flockAcquired <- struct{}{}
	})
	err = session.RunHook("myhook", s.tmpdir, os.Environ())
	c.Assert(err, gc.ErrorMatches, "signal: [kK]illed")
	waitForFlock()

	// Run the hook in debug mode, simulating the holding
	// of the exit flock. This simulates the client process
	// starting but not cleanly exiting (normally the .pid
	// file is updated, and the server waits on the client
	// process' death).
	ch := make(chan bool) // acquire the flock
	var clientExited bool
	s.PatchValue(&waitClientExit, func(*ServerSession) {
		clientExited = <-ch
		flockAcquired <- struct{}{}
	})
	go func() { ch <- true }() // asynchronously release the flock
	err = session.RunHook("myhook", s.tmpdir, os.Environ())
	waitForFlock()
	c.Assert(clientExited, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "signal: [kK]illed")
}

func (s *DebugHooksServerSuite) TestRunHook(c *gc.C) {
	err := ioutil.WriteFile(s.ctx.ClientFileLock(), []byte{}, 0777)
	c.Assert(err, jc.ErrorIsNil)
	session, err := s.ctx.FindSession()
	c.Assert(session, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)

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

	// Wait until either we find the debug dir, or the flock is released.
	ticker := time.Tick(10 * time.Millisecond)
	var debugdir os.FileInfo
	for debugdir == nil {
		select {
		case err = <-ch:
			// flock was released before we found the debug dir.
			c.Error("could not find hook.sh")

		case <-ticker:
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
	}

	envsh := filepath.Join(s.tmpdir, debugdir.Name(), "env.sh")
	s.verifyEnvshFile(c, envsh, hookName)

	hookpid := filepath.Join(s.tmpdir, debugdir.Name(), "hook.pid")
	err = ioutil.WriteFile(hookpid, []byte("not a pid"), 0777)
	c.Assert(err, jc.ErrorIsNil)

	// RunHook should complete without waiting to be
	// killed, and despite the exit lock being held.
	err = <-ch
	c.Assert(err, jc.ErrorIsNil)
	cmd.Process.Kill() // kill flock
}

func (s *DebugHooksServerSuite) verifyEnvshFile(c *gc.C, envshPath string, hookName string) {
	data, err := ioutil.ReadFile(envshPath)
	c.Assert(err, jc.ErrorIsNil)
	contents := string(data)
	c.Assert(contents, jc.Contains, fmt.Sprintf("JUJU_UNIT_NAME=%q", s.ctx.Unit))
	c.Assert(contents, jc.Contains, fmt.Sprintf("JUJU_HOOK_NAME=%q", hookName))
	c.Assert(contents, jc.Contains, fmt.Sprintf(`PS1="%s:%s %% "`, s.ctx.Unit, hookName))
}
