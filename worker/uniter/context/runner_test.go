// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/errors"
	envtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/context"
)

type RunCommandSuite struct {
	HookContextSuite
}

var _ = gc.Suite(&RunCommandSuite{})

func (s *RunCommandSuite) getHookContext(c *gc.C) *context.HookContext {
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	return s.HookContextSuite.getHookContext(c, uuid.String(), -1, "", noProxies)
}

func (s *RunCommandSuite) TestRunCommandsEnvStdOutAndErrAndRC(c *gc.C) {
	ctx := s.getHookContext(c)
	paths := NewRealPaths(c)
	runner := context.NewRunner(ctx, paths)

	commands := `
echo $JUJU_CHARM_DIR
echo this is standard err >&2
exit 42
`
	result, err := runner.RunCommands(commands)
	c.Assert(err, gc.IsNil)

	c.Assert(result.Code, gc.Equals, 42)
	c.Assert(string(result.Stdout), gc.Equals, paths.charm+"\n")
	c.Assert(string(result.Stderr), gc.Equals, "this is standard err\n")
	c.Assert(ctx.GetProcess(), gc.NotNil)
}

type RunHookSuite struct {
	HookContextSuite
}

var _ = gc.Suite(&RunHookSuite{})

// LineBufferSize matches the constant used when creating
// the bufio line reader.
const lineBufferSize = 4096

var runHookTests = []struct {
	summary string
	relid   int
	remote  string
	spec    hookSpec
	err     string
}{
	{
		summary: "missing hook is not an error",
		relid:   -1,
	}, {
		summary: "report failure to execute hook",
		relid:   -1,
		spec:    hookSpec{perm: 0600},
		err:     `exec: .*something-happened": permission denied`,
	}, {
		summary: "report error indicated by hook's exit status",
		relid:   -1,
		spec: hookSpec{
			perm: 0700,
			code: 99,
		},
		err: "exit status 99",
	}, {
		summary: "output logging",
		relid:   -1,
		spec: hookSpec{
			perm:   0700,
			stdout: "stdout",
			stderr: "stderr",
		},
	}, {
		summary: "output logging with background process",
		relid:   -1,
		spec: hookSpec{
			perm:       0700,
			stdout:     "stdout",
			background: "not printed",
		},
	}, {
		summary: "long line split",
		relid:   -1,
		spec: hookSpec{
			perm:   0700,
			stdout: strings.Repeat("a", lineBufferSize+10),
		},
	},
}

func (s *RunHookSuite) TestRunHook(c *gc.C) {
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	for i, t := range runHookTests {
		c.Logf("\ntest %d: %s; perm %v", i, t.summary, t.spec.perm)
		ctx := s.getHookContext(c, uuid.String(), t.relid, t.remote, noProxies)
		paths := NewRealPaths(c)
		runner := context.NewRunner(ctx, paths)
		var hookExists bool
		if t.spec.perm != 0 {
			spec := t.spec
			spec.dir = "hooks"
			spec.name = "something-happened"
			c.Logf("makeCharm %#v", spec)
			makeCharm(c, spec, paths.charm)
			hookExists = true
		}
		t0 := time.Now()
		err := runner.RunHook("something-happened")
		if t.err == "" && hookExists {
			c.Assert(err, gc.IsNil)
		} else if !hookExists {
			c.Assert(context.IsMissingHookError(err), jc.IsTrue)
		} else {
			c.Assert(err, gc.ErrorMatches, t.err)
		}
		if t.spec.background != "" && time.Now().Sub(t0) > 5*time.Second {
			c.Errorf("background process holding up hook execution")
		}
	}
}

type MockContext struct {
	context.Context
	actionData   *context.ActionData
	expectPid    int
	flushBadge   string
	flushFailure error
	flushResult  error
}

func (ctx *MockContext) UnitName() string {
	return "some-unit/999"
}

func (ctx *MockContext) HookVars(paths context.Paths) []string {
	return []string{"VAR=value"}
}

func (ctx *MockContext) ActionData() (*context.ActionData, error) {
	if ctx.actionData == nil {
		return nil, errors.New("blam")
	}
	return ctx.actionData, nil
}

func (ctx *MockContext) SetProcess(process *os.Process) {
	ctx.expectPid = process.Pid
}

func (ctx *MockContext) FlushContext(badge string, failure error) error {
	ctx.flushBadge = badge
	ctx.flushFailure = failure
	return ctx.flushResult
}

type RunMockContextSuite struct {
	envtesting.IsolationSuite
	paths RealPaths
}

var _ = gc.Suite(&RunMockContextSuite{})

func (s *RunMockContextSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.paths = NewRealPaths(c)
}

func (s *RunMockContextSuite) assertRecordedPid(c *gc.C, expectPid int) {
	path := filepath.Join(s.paths.charm, "pid")
	content, err := ioutil.ReadFile(path)
	c.Assert(err, gc.IsNil)
	expectContent := fmt.Sprintf("%d\n", expectPid)
	c.Assert(string(content), gc.Equals, expectContent)
}

func (s *RunMockContextSuite) TestRunHookFlushSuccess(c *gc.C) {
	expectErr := errors.New("pew pew pew")
	ctx := &MockContext{
		flushResult: expectErr,
	}
	makeCharm(c, hookSpec{
		dir:  "hooks",
		name: "something-happened",
		perm: 0700,
	}, s.paths.charm)
	actualErr := context.NewRunner(ctx, s.paths).RunHook("something-happened")
	c.Assert(actualErr, gc.Equals, expectErr)
	c.Assert(ctx.flushBadge, gc.Equals, "something-happened")
	c.Assert(ctx.flushFailure, gc.IsNil)
	s.assertRecordedPid(c, ctx.expectPid)
}

func (s *RunMockContextSuite) TestRunHookFlushFailure(c *gc.C) {
	expectErr := errors.New("pew pew pew")
	ctx := &MockContext{
		flushResult: expectErr,
	}
	makeCharm(c, hookSpec{
		dir:  "hooks",
		name: "something-happened",
		perm: 0700,
		code: 123,
	}, s.paths.charm)
	actualErr := context.NewRunner(ctx, s.paths).RunHook("something-happened")
	c.Assert(actualErr, gc.Equals, expectErr)
	c.Assert(ctx.flushBadge, gc.Equals, "something-happened")
	c.Assert(ctx.flushFailure, gc.ErrorMatches, "exit status 123")
	s.assertRecordedPid(c, ctx.expectPid)
}

func (s *RunMockContextSuite) TestRunActionFlushSuccess(c *gc.C) {
	expectErr := errors.New("pew pew pew")
	ctx := &MockContext{
		flushResult: expectErr,
		actionData:  &context.ActionData{},
	}
	makeCharm(c, hookSpec{
		dir:  "actions",
		name: "do-something",
		perm: 0700,
	}, s.paths.charm)
	actualErr := context.NewRunner(ctx, s.paths).RunAction("do-something")
	c.Assert(actualErr, gc.Equals, expectErr)
	c.Assert(ctx.flushBadge, gc.Equals, "do-something")
	c.Assert(ctx.flushFailure, gc.IsNil)
	s.assertRecordedPid(c, ctx.expectPid)
}

func (s *RunMockContextSuite) TestRunActionFlushFailure(c *gc.C) {
	expectErr := errors.New("pew pew pew")
	ctx := &MockContext{
		flushResult: expectErr,
		actionData:  &context.ActionData{},
	}
	makeCharm(c, hookSpec{
		dir:  "actions",
		name: "do-something",
		perm: 0700,
		code: 123,
	}, s.paths.charm)
	actualErr := context.NewRunner(ctx, s.paths).RunAction("do-something")
	c.Assert(actualErr, gc.Equals, expectErr)
	c.Assert(ctx.flushBadge, gc.Equals, "do-something")
	c.Assert(ctx.flushFailure, gc.ErrorMatches, "exit status 123")
	s.assertRecordedPid(c, ctx.expectPid)
}

func (s *RunMockContextSuite) TestRunCommandsFlushSuccess(c *gc.C) {
	expectErr := errors.New("pew pew pew")
	ctx := &MockContext{
		flushResult: expectErr,
	}
	_, actualErr := context.NewRunner(ctx, s.paths).RunCommands("echo $$ > pid")
	c.Assert(actualErr, gc.Equals, expectErr)
	c.Assert(ctx.flushBadge, gc.Equals, "run commands")
	c.Assert(ctx.flushFailure, gc.IsNil)
	s.assertRecordedPid(c, ctx.expectPid)
}

func (s *RunMockContextSuite) TestRunCommandsFlushFailure(c *gc.C) {
	expectErr := errors.New("pew pew pew")
	ctx := &MockContext{
		flushResult: expectErr,
	}
	_, actualErr := context.NewRunner(ctx, s.paths).RunCommands("echo $$ > pid; exit 123")
	c.Assert(actualErr, gc.Equals, expectErr)
	c.Assert(ctx.flushBadge, gc.Equals, "run commands")
	c.Assert(ctx.flushFailure, gc.IsNil) // exit code in _ result, as tested elsewhere
	s.assertRecordedPid(c, ctx.expectPid)
}
