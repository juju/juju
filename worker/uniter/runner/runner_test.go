// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/proxy"
	envtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6/hooks"

	"github.com/juju/juju/worker/common/charmrunner"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/context"
	runnertesting "github.com/juju/juju/worker/uniter/runner/testing"
)

type RunCommandSuite struct {
	ContextSuite
}

var _ = gc.Suite(&RunCommandSuite{})

var noProxies = proxy.Settings{}

func (s *RunCommandSuite) TestRunCommandsEnvStdOutAndErrAndRC(c *gc.C) {
	// TODO(bogdanteleaga): powershell throws another exit status code when
	// outputting to stderr using Write-Error. Either find another way to
	// output to stderr or change the checks
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: Have to figure out a good way to output to stderr from powershell")
	}
	ctx, err := s.contextFactory.HookContext(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)
	paths := runnertesting.NewRealPaths(c)
	runner := runner.NewRunner(ctx, paths)

	commands := `
echo $JUJU_CHARM_DIR
echo this is standard err >&2
exit 42
`
	result, err := runner.RunCommands(commands)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result.Code, gc.Equals, 42)
	c.Assert(strings.TrimRight(string(result.Stdout), "\r\n"), gc.Equals, paths.GetCharmDir())
	c.Assert(strings.TrimRight(string(result.Stderr), "\r\n"), gc.Equals, "this is standard err")
	c.Assert(ctx.GetProcess(), gc.NotNil)
}

type RunHookSuite struct {
	ContextSuite
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
	for i, t := range runHookTests {
		c.Logf("\ntest %d: %s; perm %v", i, t.summary, t.spec.perm)
		ctx, err := s.contextFactory.HookContext(hook.Info{Kind: hooks.ConfigChanged})
		c.Assert(err, jc.ErrorIsNil)

		paths := runnertesting.NewRealPaths(c)
		rnr := runner.NewRunner(ctx, paths)
		var hookExists bool
		if t.spec.perm != 0 {
			spec := t.spec
			spec.dir = "hooks"
			spec.name = hookName
			c.Logf("makeCharm %#v", spec)
			makeCharm(c, spec, paths.GetCharmDir())
			hookExists = true
		}
		t0 := time.Now()
		err = rnr.RunHook("something-happened")
		if t.err == "" && hookExists {
			c.Assert(err, jc.ErrorIsNil)
		} else if !hookExists {
			c.Assert(charmrunner.IsMissingHookError(err), jc.IsTrue)
		} else {
			c.Assert(err, gc.ErrorMatches, t.err)
		}
		if t.spec.background != "" && time.Now().Sub(t0) > 5*time.Second {
			c.Errorf("background process holding up hook execution")
		}
	}
}

type MockContext struct {
	runner.Context
	actionData      *context.ActionData
	actionParams    map[string]interface{}
	actionParamsErr error
	actionResults   map[string]interface{}
	expectPid       int
	flushBadge      string
	flushFailure    error
	flushResult     error
}

func (ctx *MockContext) UnitName() string {
	return "some-unit/999"
}

func (ctx *MockContext) HookVars(paths context.Paths) ([]string, error) {
	return []string{"VAR=value"}, nil
}

func (ctx *MockContext) ActionData() (*context.ActionData, error) {
	if ctx.actionData == nil {
		return nil, errors.New("blam")
	}
	return ctx.actionData, nil
}

func (ctx *MockContext) SetProcess(process context.HookProcess) {
	ctx.expectPid = process.Pid()
}

func (ctx *MockContext) Prepare() error {
	return nil
}

func (ctx *MockContext) Flush(badge string, failure error) error {
	ctx.flushBadge = badge
	ctx.flushFailure = failure
	return ctx.flushResult
}

func (ctx *MockContext) ActionParams() (map[string]interface{}, error) {
	return ctx.actionParams, ctx.actionParamsErr
}

func (ctx *MockContext) UpdateActionResults(keys []string, value string) error {
	for _, key := range keys {
		ctx.actionResults[key] = value
	}
	return nil
}

type RunMockContextSuite struct {
	envtesting.IsolationSuite
	paths runnertesting.RealPaths
}

var _ = gc.Suite(&RunMockContextSuite{})

func (s *RunMockContextSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.paths = runnertesting.NewRealPaths(c)
}

func (s *RunMockContextSuite) assertRecordedPid(c *gc.C, expectPid int) {
	path := filepath.Join(s.paths.GetCharmDir(), "pid")
	content, err := ioutil.ReadFile(path)
	c.Assert(err, jc.ErrorIsNil)
	expectContent := fmt.Sprintf("%d", expectPid)
	c.Assert(strings.TrimRight(string(content), "\r\n"), gc.Equals, expectContent)
}

func (s *RunMockContextSuite) TestRunHookFlushSuccess(c *gc.C) {
	expectErr := errors.New("pew pew pew")
	ctx := &MockContext{
		flushResult: expectErr,
	}
	makeCharm(c, hookSpec{
		dir:  "hooks",
		name: hookName,
		perm: 0700,
	}, s.paths.GetCharmDir())
	actualErr := runner.NewRunner(ctx, s.paths).RunHook("something-happened")
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
		name: hookName,
		perm: 0700,
		code: 123,
	}, s.paths.GetCharmDir())
	actualErr := runner.NewRunner(ctx, s.paths).RunHook("something-happened")
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
		name: hookName,
		perm: 0700,
	}, s.paths.GetCharmDir())
	actualErr := runner.NewRunner(ctx, s.paths).RunAction("something-happened")
	c.Assert(actualErr, gc.Equals, expectErr)
	c.Assert(ctx.flushBadge, gc.Equals, "something-happened")
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
		name: hookName,
		perm: 0700,
		code: 123,
	}, s.paths.GetCharmDir())
	actualErr := runner.NewRunner(ctx, s.paths).RunAction("something-happened")
	c.Assert(actualErr, gc.Equals, expectErr)
	c.Assert(ctx.flushBadge, gc.Equals, "something-happened")
	c.Assert(ctx.flushFailure, gc.ErrorMatches, "exit status 123")
	s.assertRecordedPid(c, ctx.expectPid)
}

func (s *RunMockContextSuite) TestRunActionParamsFailure(c *gc.C) {
	expectErr := errors.New("stork")
	ctx := &MockContext{
		actionData:      &context.ActionData{},
		actionParamsErr: expectErr,
	}
	actualErr := runner.NewRunner(ctx, s.paths).RunAction("juju-run")
	c.Assert(errors.Cause(actualErr), gc.Equals, expectErr)
}

func (s *RunMockContextSuite) TestRunActionSuccessful(c *gc.C) {
	ctx := &MockContext{
		actionData: &context.ActionData{},
		actionParams: map[string]interface{}{
			"command": "echo 1",
			"timeout": 0,
		},
		actionResults: map[string]interface{}{},
	}
	err := runner.NewRunner(ctx, s.paths).RunAction("juju-run")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.flushBadge, gc.Equals, "juju-run")
	c.Assert(ctx.flushFailure, gc.IsNil)
	c.Assert(ctx.actionResults["Code"], gc.Equals, "0")
	c.Assert(strings.TrimRight(ctx.actionResults["Stdout"].(string), "\r\n"), gc.Equals, "1")
	c.Assert(ctx.actionResults["Stderr"], gc.Equals, "")
}

func (s *RunMockContextSuite) TestRunActionCancelled(c *gc.C) {
	timeout := 1 * time.Nanosecond
	ctx := &MockContext{
		actionData: &context.ActionData{},
		actionParams: map[string]interface{}{
			"command": "sleep 10",
			"timeout": float64(timeout.Nanoseconds()),
		},
		actionResults: map[string]interface{}{},
	}
	err := runner.NewRunner(ctx, s.paths).RunAction("juju-run")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.flushBadge, gc.Equals, "juju-run")
	c.Assert(ctx.flushFailure, gc.Equals, exec.ErrCancelled)
	c.Assert(ctx.actionResults["Code"], gc.Equals, nil)
	c.Assert(ctx.actionResults["Stdout"], gc.Equals, nil)
	c.Assert(ctx.actionResults["Stderr"], gc.Equals, nil)
}

func (s *RunMockContextSuite) TestRunCommandsFlushSuccess(c *gc.C) {
	expectErr := errors.New("pew pew pew")
	ctx := &MockContext{
		flushResult: expectErr,
	}
	_, actualErr := runner.NewRunner(ctx, s.paths).RunCommands(echoPidScript)
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
	_, actualErr := runner.NewRunner(ctx, s.paths).RunCommands(echoPidScript + "; exit 123")
	c.Assert(actualErr, gc.Equals, expectErr)
	c.Assert(ctx.flushBadge, gc.Equals, "run commands")
	c.Assert(ctx.flushFailure, gc.IsNil) // exit code in _ result, as tested elsewhere
	s.assertRecordedPid(c, ctx.expectPid)
}
