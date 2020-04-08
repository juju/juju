// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debug

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
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

var fakecommands = []string{"sleep", "tmux"}

func (s *DebugHooksServerSuite) SetUpTest(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: Currently debug does not work on windows")
	}
	s.fakebin = c.MkDir()

	// Create a clean $TMPDIR for the debug hooks scripts.
	s.tmpdir = filepath.Join(c.MkDir(), "debug-hooks")
	err := os.RemoveAll(s.tmpdir)
	c.Assert(err, jc.ErrorIsNil)
	err = os.MkdirAll(s.tmpdir, 0755)
	c.Assert(err, jc.ErrorIsNil)

	s.PatchEnvPathPrepend(s.fakebin)
	s.PatchEnvironment("TMPDIR", s.tmpdir)
	s.PatchEnvironment("TEST_RESULT", "")
	for _, name := range fakecommands {
		err := ioutil.WriteFile(filepath.Join(s.fakebin, name), []byte(echocommand), 0777)
		c.Assert(err, jc.ErrorIsNil)
	}
	s.ctx = NewHooksContext("foo/8")
	s.ctx.FlockDir = c.MkDir()
	s.PatchEnvironment("JUJU_UNIT_NAME", s.ctx.Unit)
}

func (s *DebugHooksServerSuite) TestFindSession(c *gc.C) {
	// Test "tmux has-session" failure. The error
	// message is the output of tmux has-session.
	_ = os.Setenv("EXIT_CODE", "1")
	session, err := s.ctx.FindSession()
	c.Assert(session, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta("tmux has-session -t "+s.ctx.Unit+"\n"))
	_ = os.Setenv("EXIT_CODE", "")

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
	err = session.RunHook("myhook", s.tmpdir, os.Environ(), "myhook")
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
	err = session.RunHook("myhook", s.tmpdir, os.Environ(), "myhook")
	waitForFlock()
	c.Assert(clientExited, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "signal: [kK]illed")
}

func (s *DebugHooksServerSuite) TestRunHook(c *gc.C) {
	const hookName = "myhook"
	// JUJU_DISPATCH_PATH is written in context.HookVars and not part of
	// what's being tested here.
	s.PatchEnvironment("JUJU_DISPATCH_PATH", "hooks/"+hookName)
	err := ioutil.WriteFile(s.ctx.ClientFileLock(), []byte{}, 0777)
	c.Assert(err, jc.ErrorIsNil)
	var output bytes.Buffer
	session, err := s.ctx.FindSessionWithWriter(&output)
	c.Assert(session, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)

	flockRequestCh := make(chan chan struct{})
	s.PatchValue(&waitClientExit, func(*ServerSession) {
		<-<-flockRequestCh
	})
	defer close(flockRequestCh)

	runHookCh := make(chan error)
	go func() {
		runHookCh <- session.RunHook(hookName, s.tmpdir, os.Environ(), hookName)
	}()

	flockCh := make(chan struct{})
	select {
	case flockRequestCh <- flockCh:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for flock to be requested")
	}
	defer close(flockCh)

	// Look for the debug hooks temporary dir, inside $TMPDIR.
	tmpdir, err := os.Open(s.tmpdir)
	if err != nil {
		c.Fatalf("Failed to open $TMPDIR: %s", err)
	}
	defer func() { _ = tmpdir.Close() }()
	entries, err := tmpdir.Readdir(-1)
	if err != nil {
		c.Fatalf("Failed to read $TMPDIR: %s", err)
	}
	c.Assert(entries, gc.HasLen, 1)
	c.Assert(entries[0].IsDir(), jc.IsTrue)
	c.Assert(strings.HasPrefix(entries[0].Name(), "juju-debug-hooks-"), jc.IsTrue)

	debugDir := filepath.Join(s.tmpdir, entries[0].Name())
	hookScript := filepath.Join(debugDir, "hook.sh")
	_, err = os.Stat(hookScript)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the debug hooks script exports the environment,
	// and the values are as expected. When RunHook completes,
	// it removes the temporary directory in which the scripts
	// reside; so we must wait for it to be written before we
	// wait for RunHook to return.
	timeout := time.After(testing.LongWait)
	envsh := filepath.Join(debugDir, "env.sh")
	for {
		// Wait for env.sh to show up, and have some content. If it exists and
		// is size 0, we managed to see it at exactly the time it is being
		// written.
		if st, err := os.Stat(envsh); err == nil {
			if st.Size() != 0 {
				break
			}
		}
		select {
		case <-time.After(time.Millisecond):
		case <-timeout:
			c.Fatal("timed out waiting for env.sh to be written")
		}
	}
	s.verifyEnvshFile(c, envsh, hookName)

	// Write the hook.pid file, causing the debug hooks script to exit.
	hookpid := filepath.Join(debugDir, "hook.pid")
	err = ioutil.WriteFile(hookpid, []byte("not a pid"), 0777)
	c.Assert(err, jc.ErrorIsNil)

	// RunHook should complete without waiting to be
	// killed, and despite the exit lock being held.
	select {
	case err := <-runHookCh:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatal("RunHook did not complete")
	}
}

func (s *DebugHooksServerSuite) verifyEnvshFile(c *gc.C, envshPath string, hookName string) {
	data, err := ioutil.ReadFile(envshPath)
	c.Assert(err, jc.ErrorIsNil)
	contents := string(data)
	c.Assert(contents, jc.Contains, fmt.Sprintf("JUJU_UNIT_NAME=%q", s.ctx.Unit))
	c.Assert(contents, jc.Contains, fmt.Sprintf(`PS1="%s:hooks/%s %% "`, s.ctx.Unit, hookName))
}
