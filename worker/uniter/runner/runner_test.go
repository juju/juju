// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	"bytes"
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

	"github.com/juju/juju/core/model"
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
	runner := runner.NewRunner(ctx, paths, nil)

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
	summary  string
	relid    int
	spec     hookSpec
	err      string
	hookType runner.HookHandlerType
}{
	{
		summary:  "missing hook is not an error",
		relid:    -1,
		hookType: runner.InvalidHookHandler,
	}, {
		summary: "report error indicated by hook's exit status",
		relid:   -1,
		spec: hookSpec{
			perm: 0700,
			code: 99,
		},
		err:      "exit status 99",
		hookType: runner.ExplicitHookHandler,
	}, {
		summary: "report error with invalid script",
		relid:   -1,
		spec: hookSpec{
			perm:           0700,
			code:           2,
			missingShebang: true,
		},
		err:      "fork/exec.*: exec format error",
		hookType: runner.ExplicitHookHandler,
	}, {
		summary: "output logging",
		relid:   -1,
		spec: hookSpec{
			perm:   0700,
			stdout: "stdout",
			stderr: "stderr",
		},
		hookType: runner.ExplicitHookHandler,
	}, {
		summary: "output logging with background process",
		relid:   -1,
		spec: hookSpec{
			perm:       0700,
			stdout:     "stdout",
			background: "not printed",
		},
		hookType: runner.ExplicitHookHandler,
	}, {
		summary: "long line split",
		relid:   -1,
		spec: hookSpec{
			perm:   0700,
			stdout: strings.Repeat("a", lineBufferSize+10),
		},
		hookType: runner.ExplicitHookHandler,
	},
}

func (s *RunHookSuite) TestRunHook(c *gc.C) {
	for i, t := range runHookTests {
		c.Logf("\ntest %d of %d: %s; perm %v", i, len(runHookTests)+1, t.summary, t.spec.perm)
		ctx, err := s.contextFactory.HookContext(hook.Info{Kind: hooks.ConfigChanged})
		c.Assert(err, jc.ErrorIsNil)

		paths := runnertesting.NewRealPaths(c)
		rnr := runner.NewRunner(ctx, paths, nil)
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
		hookType, err := rnr.RunHook("something-happened")
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
		c.Assert(hookType, gc.Equals, t.hookType)
	}
}

func (s *RunHookSuite) TestRunHookDispatchingHookHandler(c *gc.C) {
	ctx, err := s.contextFactory.HookContext(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)

	paths := runnertesting.NewRealPaths(c)
	rnr := runner.NewRunner(ctx, paths, nil)
	spec := hookSpec{
		name: "dispatch",
		perm: 0700,
	}
	c.Logf("makeCharm %#v", spec)
	makeCharm(c, spec, paths.GetCharmDir())

	hookType, err := rnr.RunHook("something-happened")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hookType, gc.Equals, runner.DispatchingHookHandler)
}

type MockContext struct {
	runner.Context
	actionData      *context.ActionData
	actionDataErr   error
	actionParams    map[string]interface{}
	actionParamsErr error
	actionResults   map[string]interface{}
	expectPid       int
	flushBadge      string
	flushFailure    error
	flushResult     error
	modelType       model.ModelType
}

func (ctx *MockContext) UnitName() string {
	return "some-unit/999"
}

func (ctx *MockContext) HookVars(paths context.Paths, _ bool, getEnv context.GetEnvFunc) ([]string, error) {
	pathKey := ""
	if runtime.GOOS == "windows" {
		pathKey = "Path"
	} else {
		pathKey = "PATH"
	}
	path := getEnv(pathKey)
	newPath := fmt.Sprintf("%s=pathypathpath;%s", pathKey, path)
	return []string{"VAR=value", newPath}, nil
}

func (ctx *MockContext) ActionData() (*context.ActionData, error) {
	if ctx.actionData == nil {
		return nil, errors.New("blam")
	}
	return ctx.actionData, ctx.actionDataErr
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

func (ctx *MockContext) ModelType() model.ModelType {
	if ctx.modelType == "" {
		return model.IAAS
	}
	return ctx.modelType
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
	_, actualErr := runner.NewRunner(ctx, s.paths, nil).RunHook("something-happened")
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
	_, actualErr := runner.NewRunner(ctx, s.paths, nil).RunHook("something-happened")
	c.Assert(actualErr, gc.Equals, expectErr)
	c.Assert(ctx.flushBadge, gc.Equals, "something-happened")
	c.Assert(ctx.flushFailure, gc.ErrorMatches, "exit status 123")
	s.assertRecordedPid(c, ctx.expectPid)
}

func (s *RunHookSuite) TestRunActionDispatchingHookHandler(c *gc.C) {
	ctx := &MockContext{
		actionData:    &context.ActionData{},
		actionResults: map[string]interface{}{},
	}

	paths := runnertesting.NewRealPaths(c)
	rnr := runner.NewRunner(ctx, paths, nil)
	spec := hookSpec{
		name: "dispatch",
		perm: 0700,
	}
	c.Logf("makeCharm %#v", spec)
	makeCharm(c, spec, paths.GetCharmDir())

	hookType, err := rnr.RunAction("something-happened")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hookType, gc.Equals, runner.DispatchingHookHandler)
}

func (s *RunMockContextSuite) TestRunActionFlushSuccess(c *gc.C) {
	expectErr := errors.New("pew pew pew")
	ctx := &MockContext{
		flushResult:   expectErr,
		actionData:    &context.ActionData{},
		actionResults: map[string]interface{}{},
	}
	makeCharm(c, hookSpec{
		dir:    "actions",
		name:   hookName,
		perm:   0700,
		stdout: "hello",
		stderr: "world",
	}, s.paths.GetCharmDir())
	hookType, actualErr := runner.NewRunner(ctx, s.paths, nil).RunAction("something-happened")
	c.Assert(actualErr, gc.Equals, expectErr)
	c.Assert(hookType, gc.Equals, runner.ExplicitHookHandler)
	c.Assert(ctx.flushBadge, gc.Equals, "something-happened")
	c.Assert(ctx.flushFailure, gc.IsNil)
	s.assertRecordedPid(c, ctx.expectPid)
	c.Assert(ctx.actionResults, jc.DeepEquals, map[string]interface{}{
		"Code": "0", "Stderr": "world\n", "Stdout": "hello\n",
	})
}

func (s *RunMockContextSuite) TestRunActionFlushCharmActionsCAASSuccess(c *gc.C) {
	expectErr := errors.New("pew pew pew")
	ctx := &MockContext{
		flushResult:   expectErr,
		actionData:    &context.ActionData{},
		actionResults: map[string]interface{}{},
		modelType:     model.CAAS,
	}
	makeCharm(c, hookSpec{
		dir:  "actions",
		name: hookName,
		perm: 0700,
	}, s.paths.GetCharmDir())

	execCount := 0
	execFunc := func(params runner.ExecParams) (*exec.ExecResponse, error) {
		execCount++
		switch execCount {
		case 1:
			return &exec.ExecResponse{}, nil
		case 2:
			return &exec.ExecResponse{
				Stdout: bytes.NewBufferString("hello").Bytes(),
				Stderr: bytes.NewBufferString("world").Bytes(),
			}, nil
		}
		c.Fatal("invalid count")
		return nil, nil
	}
	_, actualErr := runner.NewRunner(ctx, s.paths, execFunc).RunAction("something-happened")
	c.Assert(execCount, gc.Equals, 2)
	c.Assert(actualErr, gc.Equals, expectErr)
	c.Assert(ctx.flushBadge, gc.Equals, "something-happened")
	c.Assert(ctx.flushFailure, gc.IsNil)
	c.Assert(ctx.actionResults, jc.DeepEquals, map[string]interface{}{
		"Code": "0", "Stderr": "world", "Stdout": "hello",
	})
}

func (s *RunMockContextSuite) TestRunActionFlushCharmActionsCAASFailed(c *gc.C) {
	ctx := &MockContext{
		flushResult: errors.New("pew pew pew"),
		actionData:  &context.ActionData{},
		modelType:   model.CAAS,
	}
	makeCharm(c, hookSpec{
		dir:  "actions",
		name: hookName,
		perm: 0700,
	}, s.paths.GetCharmDir())
	execCount := 0
	execFunc := func(params runner.ExecParams) (*exec.ExecResponse, error) {
		execCount++
		switch execCount {
		case 1:
			return &exec.ExecResponse{}, nil
		case 2:
			return nil, errors.Errorf("failed exec")
		}
		c.Fatal("invalid count")
		return nil, nil
	}
	_, actualErr := runner.NewRunner(ctx, s.paths, execFunc).RunAction("something-happened")
	c.Assert(execCount, gc.Equals, 2)
	c.Assert(actualErr, gc.Equals, ctx.flushResult)
	c.Assert(ctx.flushBadge, gc.Equals, "something-happened")
	c.Assert(ctx.flushFailure, gc.ErrorMatches, "failed exec")
}

func (s *RunMockContextSuite) TestRunActionFlushFailure(c *gc.C) {
	expectErr := errors.New("pew pew pew")
	ctx := &MockContext{
		flushResult:   expectErr,
		actionData:    &context.ActionData{},
		actionResults: map[string]interface{}{},
	}
	makeCharm(c, hookSpec{
		dir:  "actions",
		name: hookName,
		perm: 0700,
		code: 123,
	}, s.paths.GetCharmDir())
	_, actualErr := runner.NewRunner(ctx, s.paths, nil).RunAction("something-happened")
	c.Assert(actualErr, gc.Equals, expectErr)
	c.Assert(ctx.flushBadge, gc.Equals, "something-happened")
	c.Assert(ctx.flushFailure, gc.ErrorMatches, "exit status 123")
	s.assertRecordedPid(c, ctx.expectPid)
}

func (s *RunMockContextSuite) TestRunActionDataFailure(c *gc.C) {
	expectErr := errors.New("stork")
	ctx := &MockContext{
		actionData:    &context.ActionData{},
		actionDataErr: expectErr,
	}
	_, actualErr := runner.NewRunner(ctx, s.paths, nil).RunAction("juju-run")
	c.Assert(errors.Cause(actualErr), gc.Equals, expectErr)
}

func (s *RunMockContextSuite) TestRunActionSuccessful(c *gc.C) {
	params := map[string]interface{}{
		"command": "echo 1",
		"timeout": 0,
	}
	ctx := &MockContext{
		actionData: &context.ActionData{
			Params: params,
		},
		actionParams:  params,
		actionResults: map[string]interface{}{},
	}
	_, err := runner.NewRunner(ctx, s.paths, nil).RunAction("juju-run")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.flushBadge, gc.Equals, "juju-run")
	c.Assert(ctx.flushFailure, gc.IsNil)
	c.Assert(ctx.actionResults["Code"], gc.Equals, "0")
	c.Assert(strings.TrimRight(ctx.actionResults["Stdout"].(string), "\r\n"), gc.Equals, "1")
	c.Assert(ctx.actionResults["Stderr"], gc.Equals, nil)
}

func (s *RunMockContextSuite) TestRunActionError(c *gc.C) {
	params := map[string]interface{}{
		"command": "echo 1\nexit 3",
		"timeout": 0,
	}
	ctx := &MockContext{
		actionData: &context.ActionData{
			Params: params,
		},
		actionParams:  params,
		actionResults: map[string]interface{}{},
	}
	_, err := runner.NewRunner(ctx, s.paths, nil).RunAction("juju-run")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.flushBadge, gc.Equals, "juju-run")
	c.Assert(ctx.flushFailure, gc.IsNil)
	c.Assert(ctx.actionResults["Code"], gc.Equals, "3")
	c.Assert(strings.TrimRight(ctx.actionResults["Stdout"].(string), "\r\n"), gc.Equals, "1")
	c.Assert(ctx.actionResults["Stderr"], gc.Equals, nil)
}

func (s *RunMockContextSuite) TestRunActionCancelled(c *gc.C) {
	timeout := 1 * time.Nanosecond
	params := map[string]interface{}{
		"command": "sleep 10",
		"timeout": float64(timeout.Nanoseconds()),
	}
	ctx := &MockContext{
		actionData: &context.ActionData{
			Params: params,
		},
		actionParams:  params,
		actionResults: map[string]interface{}{},
	}
	_, err := runner.NewRunner(ctx, s.paths, nil).RunAction("juju-run")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.flushBadge, gc.Equals, "juju-run")
	c.Assert(ctx.flushFailure, gc.Equals, exec.ErrCancelled)
	c.Assert(ctx.actionResults["Code"], gc.Equals, "0")
	c.Assert(ctx.actionResults["Stdout"], gc.Equals, nil)
	c.Assert(ctx.actionResults["Stderr"], gc.Equals, nil)
}

func (s *RunMockContextSuite) TestRunCommandsFlushSuccess(c *gc.C) {
	expectErr := errors.New("pew pew pew")
	ctx := &MockContext{
		flushResult: expectErr,
	}
	_, actualErr := runner.NewRunner(ctx, s.paths, nil).RunCommands(echoPidScript)
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
	_, actualErr := runner.NewRunner(ctx, s.paths, nil).RunCommands(echoPidScript + "; exit 123")
	c.Assert(actualErr, gc.Equals, expectErr)
	c.Assert(ctx.flushBadge, gc.Equals, "run commands")
	c.Assert(ctx.flushFailure, gc.IsNil) // exit code in _ result, as tested elsewhere
	s.assertRecordedPid(c, ctx.expectPid)
}

func (s *RunMockContextSuite) TestRunActionCAASSuccess(c *gc.C) {
	params := map[string]interface{}{
		"command":          "echo 1",
		"timeout":          0,
		"workload-context": true,
	}
	ctx := &MockContext{
		modelType: model.CAAS,
		actionData: &context.ActionData{
			Params: params,
		},
		actionParams:  params,
		actionResults: map[string]interface{}{},
	}
	execCount := 0
	execFunc := func(params runner.ExecParams) (*exec.ExecResponse, error) {
		execCount++
		switch execCount {
		case 1:
			return &exec.ExecResponse{}, nil
		case 2:
			return &exec.ExecResponse{
				Stdout: bytes.NewBufferString("1").Bytes(),
			}, nil
		}
		c.Fatal("invalid count")
		return nil, nil
	}
	_, err := runner.NewRunner(ctx, s.paths, execFunc).RunAction("juju-run")
	c.Assert(execCount, gc.Equals, 2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.flushBadge, gc.Equals, "juju-run")
	c.Assert(ctx.actionResults["Code"], gc.Equals, "0")
	c.Assert(strings.TrimRight(ctx.actionResults["Stdout"].(string), "\r\n"), gc.Equals, "1")
	c.Assert(ctx.actionResults["Stderr"], gc.Equals, nil)
}

func (s *RunMockContextSuite) TestRunActionCAASCorrectEnv(c *gc.C) {
	params := map[string]interface{}{
		"command":          "echo 1",
		"timeout":          0,
		"workload-context": true,
	}
	ctx := &MockContext{
		modelType: model.CAAS,
		actionData: &context.ActionData{
			Params: params,
		},
		actionParams:  params,
		actionResults: map[string]interface{}{},
	}
	execCount := 0
	execFunc := func(params runner.ExecParams) (*exec.ExecResponse, error) {
		execCount++
		switch execCount {
		case 1:
			c.Assert(params.Commands, gc.DeepEquals, []string{"unset _; export"})
			return &exec.ExecResponse{
				Stdout: []byte(`
export BLA='bla'
export PATH='important-path'
`[1:]),
			}, nil
		case 2:
			path := ""
			for _, v := range params.Env {
				if strings.HasPrefix(v, "PATH=") {
					path = v
				}
			}
			c.Assert(path, gc.Equals, "PATH=pathypathpath;important-path")
			return &exec.ExecResponse{
				Stdout: bytes.NewBufferString("1").Bytes(),
			}, nil
		}
		c.Fatal("invalid count")
		return nil, nil
	}
	_, err := runner.NewRunner(ctx, s.paths, execFunc).RunAction("juju-run")
	c.Assert(execCount, gc.Equals, 2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.flushBadge, gc.Equals, "juju-run")
	c.Assert(ctx.actionResults["Code"], gc.Equals, "0")
	c.Assert(strings.TrimRight(ctx.actionResults["Stdout"].(string), "\r\n"), gc.Equals, "1")
	c.Assert(ctx.actionResults["Stderr"], gc.Equals, nil)
}

func (s *RunMockContextSuite) TestRunActionOnWorkloadIgnoredIAAS(c *gc.C) {
	params := map[string]interface{}{
		"command":          "echo 1",
		"timeout":          0,
		"workload-context": true,
	}
	ctx := &MockContext{
		modelType: model.IAAS,
		actionData: &context.ActionData{
			Params: params,
		},
		actionParams:  params,
		actionResults: map[string]interface{}{},
	}
	_, err := runner.NewRunner(ctx, s.paths, nil).RunAction("juju-run")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.flushBadge, gc.Equals, "juju-run")
	c.Assert(ctx.flushFailure, gc.IsNil)
	c.Assert(ctx.actionResults["Code"], gc.Equals, "0")
	c.Assert(strings.TrimRight(ctx.actionResults["Stdout"].(string), "\r\n"), gc.Equals, "1")
	c.Assert(ctx.actionResults["Stderr"], gc.Equals, nil)
}

func (s *RunMockContextSuite) TestOperatorActionCAASSuccess(c *gc.C) {
	expectErr := errors.New("pew pew pew")
	params := map[string]interface{}{
		"workload-context": false,
	}
	ctx := &MockContext{
		modelType: model.CAAS,
		actionData: &context.ActionData{
			Params: params,
		},
		actionParams:  params,
		actionResults: map[string]interface{}{},
		flushResult:   expectErr}
	makeCharm(c, hookSpec{
		dir:    "actions",
		name:   hookName,
		perm:   0700,
		stdout: "hello",
		stderr: "world",
	}, s.paths.GetCharmDir())
	hookType, actualErr := runner.NewRunner(ctx, s.paths, nil).RunAction("something-happened")
	c.Assert(actualErr, gc.Equals, expectErr)
	c.Assert(hookType, gc.Equals, runner.ExplicitHookHandler)
	c.Assert(ctx.flushBadge, gc.Equals, "something-happened")
	c.Assert(ctx.flushFailure, gc.IsNil)
	s.assertRecordedPid(c, ctx.expectPid)
	c.Assert(ctx.actionResults, jc.DeepEquals, map[string]interface{}{
		"Code": "0", "Stderr": "world\n", "Stdout": "hello\n",
	})
}
