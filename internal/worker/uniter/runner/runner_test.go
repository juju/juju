// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	"bytes"
	stdcontext "context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/tc"
	envtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4/exec"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/charm/hooks"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/runner"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	runnertesting "github.com/juju/juju/internal/worker/uniter/runner/testing"
	"github.com/juju/juju/juju/sockets"
)

type RunCommandSuite struct {
	ContextSuite
}

var _ = tc.Suite(&RunCommandSuite{})

func (s *RunCommandSuite) TestRunCommandsEnvStdOutAndErrAndRC(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupFactory(c, ctrl)

	ctx, err := s.contextFactory.HookContext(stdcontext.Background(), hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)
	paths := runnertesting.NewRealPaths(c)
	r := runner.NewRunner(ctx, paths)

	// Ensure the current process env is passed through to the command.
	s.PatchEnvironment("KUBERNETES_PORT", "443")

	commands := `
echo $JUJU_CHARM_DIR
echo $FOO
echo this is standard err >&2
exit 42
`
	result, err := r.RunCommands(stdcontext.Background(), commands)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result.Code, tc.Equals, 42)
	c.Assert(strings.ReplaceAll(string(result.Stdout), "\n", ""), tc.Equals, paths.GetCharmDir())
	c.Assert(strings.TrimRight(string(result.Stderr), "\n"), tc.Equals, "this is standard err")
	c.Assert(ctx.GetProcess(), tc.NotNil)
}

type RunHookSuite struct {
	ContextSuite
}

var _ = tc.Suite(&RunHookSuite{})

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
		summary: "report error with missing charm",
		relid:   -1,
		spec: hookSpec{
			charmMissing: true,
			perm:         0700,
		},
		err:      "charm missing from disk",
		hookType: runner.InvalidHookHandler,
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

type RestrictedWriter struct {
	Module string // what Module should be included in the log buffer
	Buffer bytes.Buffer
}

func (r *RestrictedWriter) Write(entry loggo.Entry) {
	if strings.HasPrefix(entry.Module, r.Module) {
		fmt.Fprintf(&r.Buffer, "%s %s %s\n", entry.Level.String(), entry.Module, entry.Message)
	}
}

func (s *RunHookSuite) TestRunHook(c *tc.C) {
	writer := &RestrictedWriter{Module: "unit.u/0.something-happened"}
	c.Assert(loggo.RegisterWriter("test", writer), jc.ErrorIsNil)

	for i, t := range runHookTests {
		ctrl := gomock.NewController(c)
		s.setupFactory(c, ctrl)

		writer.Buffer.Reset()
		c.Logf("\ntest %d of %d: %s; perm %v", i, len(runHookTests)+1, t.summary, t.spec.perm)
		ctx, err := s.contextFactory.HookContext(stdcontext.Background(), hook.Info{Kind: hooks.ConfigChanged})
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
		} else if !t.spec.charmMissing {
			makeCharmMetadata(c, paths.GetCharmDir())
		}
		t0 := time.Now()
		hookType, err := rnr.RunHook(stdcontext.Background(), "something-happened")
		if t.err == "" && hookExists {
			c.Assert(err, jc.ErrorIsNil)
		} else if !hookExists {
			c.Assert(charmrunner.IsMissingHookError(err), jc.IsTrue)
		} else {
			c.Assert(err, tc.ErrorMatches, t.err)
		}
		if t.spec.background != "" && time.Now().Sub(t0) > 5*time.Second {
			c.Errorf("background process holding up hook execution")
		}
		c.Assert(hookType, tc.Equals, t.hookType)
		if t.spec.stdout != "" {
			if len(t.spec.stdout) < lineBufferSize {
				c.Check(writer.Buffer.String(), jc.Contains,
					fmt.Sprintf("DEBUG unit.u/0.something-happened %s\n", t.spec.stdout))
			} else {
				// Lines longer than lineBufferSize get split into multiple log messages
				c.Check(writer.Buffer.String(), jc.Contains,
					fmt.Sprintf("DEBUG unit.u/0.something-happened %s\n", t.spec.stdout[:lineBufferSize]))
				c.Check(writer.Buffer.String(), jc.Contains,
					fmt.Sprintf("DEBUG unit.u/0.something-happened %s\n", t.spec.stdout[lineBufferSize:]))
			}
		}
		if t.spec.stderr != "" {
			c.Check(writer.Buffer.String(), jc.Contains,
				fmt.Sprintf("WARNING unit.u/0.something-happened %s\n", t.spec.stderr))
		}
		ctrl.Finish()
	}
}

func (s *RunHookSuite) TestRunHookDispatchingHookHandler(c *tc.C) {
	ctx, err := s.contextFactory.HookContext(stdcontext.Background(), hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)

	paths := runnertesting.NewRealPaths(c)
	rnr := runner.NewRunner(ctx, paths)
	spec := hookSpec{
		name: "dispatch",
		perm: 0700,
	}
	c.Logf("makeCharm %#v", spec)
	makeCharm(c, spec, paths.GetCharmDir())

	hookType, err := rnr.RunHook(stdcontext.Background(), "something-happened")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hookType, tc.Equals, runner.DispatchingHookHandler)
}

type MockContext struct {
	context.Context
	id              string
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

func (ctx *MockContext) GetLoggerByName(module string) logger.Logger {
	return internallogger.GetLogger(module)
}

func (ctx *MockContext) Id() string {
	return ctx.id
}

func (ctx *MockContext) GetLogger(module string) loggo.Logger {
	return loggo.GetLogger(module)
}

func (ctx *MockContext) UnitName() string {
	return "some-unit/999"
}

func (ctx *MockContext) HookVars(
	_ stdcontext.Context,
	paths context.Paths,
	envVars context.Environmenter,
) ([]string, error) {
	path := envVars.Getenv("PATH")
	newPath := fmt.Sprintf("PATH=pathypathpath;%s", path)
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

func (ctx *MockContext) Prepare(stdcontext.Context) error {
	return nil
}

func (ctx *MockContext) Flush(_ stdcontext.Context, badge string, failure error) error {
	ctx.flushBadge = badge
	ctx.flushFailure = failure
	return ctx.flushResult
}

func (ctx *MockContext) ActionParams() (map[string]interface{}, error) {
	return ctx.actionParams, ctx.actionParamsErr
}

func (ctx *MockContext) UpdateActionResults(keys []string, value interface{}) error {
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

var _ = tc.Suite(&RunMockContextSuite{})

func (s *RunMockContextSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.paths = runnertesting.NewRealPaths(c)
}

func (s *RunMockContextSuite) assertRecordedPid(c *tc.C, expectPid int) {
	path := filepath.Join(s.paths.GetCharmDir(), "pid")
	content, err := os.ReadFile(path)
	c.Assert(err, jc.ErrorIsNil)
	expectContent := fmt.Sprintf("%d", expectPid)
	c.Assert(strings.TrimRight(string(content), "\r\n"), tc.Equals, expectContent)
}

func (s *RunMockContextSuite) TestBadContextId(c *tc.C) {
	params := map[string]interface{}{
		"command": "echo 1",
		"timeout": 0,
	}
	ctx := &MockContext{
		id:        "foo-context",
		modelType: model.IAAS,
		actionData: &context.ActionData{
			Params: params,
		},
		actionParams:  params,
		actionResults: map[string]interface{}{},
	}
	start := make(chan struct{})
	done := make(chan struct{})
	result := make(chan error)

	execFunc := func(params runner.ExecParams) (*exec.ExecResponse, error) {
		close(start)
		select {
		case <-done:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting to complete")
		}
		return &exec.ExecResponse{}, nil
	}
	go func() {
		defer close(done)
		select {
		case <-start:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting to start")
		}
		socket := s.paths.GetJujucServerSocket()

		client, err := sockets.Dial(socket)
		c.Assert(err, jc.ErrorIsNil)
		defer client.Close()

		req := jujuc.Request{
			ContextId:   "whatever",
			Dir:         c.MkDir(),
			CommandName: "remote",
		}

		var resp exec.ExecResponse
		err = client.Call("Jujuc.Main", req, &resp)

		go func() {
			result <- err
		}()
	}()
	_, err := runner.NewRunner(ctx, s.paths, runner.WithExecutor(execFunc)).RunAction(stdcontext.Background(), "juju-run")
	c.Assert(err, jc.ErrorIsNil)

	select {
	case err := <-result:
		c.Assert(err, tc.ErrorMatches, `.*wrong context ID; got "whatever"`)
	case <-time.After(5 * time.Second):
		c.Fatal("timed out waiting for jujuc to finish")
	}
}

func (s *RunMockContextSuite) TestRunHookFlushSuccess(c *tc.C) {
	expectErr := errors.New("pew pew pew")
	ctx := &MockContext{
		flushResult: expectErr,
	}
	makeCharm(c, hookSpec{
		dir:  "hooks",
		name: hookName,
		perm: 0700,
	}, s.paths.GetCharmDir())
	_, actualErr := runner.NewRunner(ctx, s.paths).RunHook(stdcontext.Background(), "something-happened")
	c.Assert(actualErr, tc.Equals, expectErr)
	c.Assert(ctx.flushBadge, tc.Equals, "something-happened")
	c.Assert(ctx.flushFailure, tc.IsNil)
	s.assertRecordedPid(c, ctx.expectPid)
}

func (s *RunMockContextSuite) TestRunHookFlushFailure(c *tc.C) {
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
	_, actualErr := runner.NewRunner(ctx, s.paths).RunHook(stdcontext.Background(), "something-happened")
	c.Assert(actualErr, tc.Equals, expectErr)
	c.Assert(ctx.flushBadge, tc.Equals, "something-happened")
	c.Assert(ctx.flushFailure, tc.ErrorMatches, "exit status 123")
	s.assertRecordedPid(c, ctx.expectPid)
}

func (s *RunHookSuite) TestRunActionDispatchingHookHandler(c *tc.C) {
	ctx := &MockContext{
		actionData:    &context.ActionData{},
		actionResults: map[string]interface{}{},
	}

	paths := runnertesting.NewRealPaths(c)
	rnr := runner.NewRunner(ctx, paths)
	spec := hookSpec{
		name: "dispatch",
		perm: 0700,
	}
	c.Logf("makeCharm %#v", spec)
	makeCharm(c, spec, paths.GetCharmDir())

	hookType, err := rnr.RunAction(stdcontext.Background(), "something-happened")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hookType, tc.Equals, runner.DispatchingHookHandler)
}

func (s *RunMockContextSuite) TestRunActionFlushSuccess(c *tc.C) {
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
	hookType, actualErr := runner.NewRunner(ctx, s.paths).RunAction(stdcontext.Background(), "something-happened")
	c.Assert(actualErr, tc.Equals, expectErr)
	c.Assert(hookType, tc.Equals, runner.ExplicitHookHandler)
	c.Assert(ctx.flushBadge, tc.Equals, "something-happened")
	c.Assert(ctx.flushFailure, tc.IsNil)
	s.assertRecordedPid(c, ctx.expectPid)
	c.Assert(ctx.actionResults, jc.DeepEquals, map[string]interface{}{
		"return-code": 0, "stderr": "world\n", "stdout": "hello\n",
	})
}

func (s *RunMockContextSuite) TestRunActionFlushFailure(c *tc.C) {
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
	_, actualErr := runner.NewRunner(ctx, s.paths).RunAction(stdcontext.Background(), "something-happened")
	c.Assert(actualErr, tc.Equals, expectErr)
	c.Assert(ctx.flushBadge, tc.Equals, "something-happened")
	c.Assert(ctx.flushFailure, tc.ErrorMatches, "exit status 123")
	s.assertRecordedPid(c, ctx.expectPid)
}

func (s *RunMockContextSuite) TestRunActionDataFailure(c *tc.C) {
	expectErr := errors.New("stork")
	ctx := &MockContext{
		actionData:    &context.ActionData{},
		actionDataErr: expectErr,
	}
	_, actualErr := runner.NewRunner(ctx, s.paths).RunAction(stdcontext.Background(), "juju-exec")
	c.Assert(errors.Cause(actualErr), tc.Equals, expectErr)
}

func (s *RunMockContextSuite) TestRunActionSuccessful(c *tc.C) {
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
	_, err := runner.NewRunner(ctx, s.paths).RunAction(stdcontext.Background(), "juju-exec")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.flushBadge, tc.Equals, "juju-exec")
	c.Assert(ctx.flushFailure, tc.IsNil)
	c.Assert(ctx.actionResults["return-code"], tc.Equals, 0)
	c.Assert(strings.TrimRight(ctx.actionResults["stdout"].(string), "\r\n"), tc.Equals, "1")
	c.Assert(ctx.actionResults["stderr"], tc.Equals, nil)
}

func (s *RunMockContextSuite) TestRunActionError(c *tc.C) {
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
	_, err := runner.NewRunner(ctx, s.paths).RunAction(stdcontext.Background(), "juju-exec")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.flushBadge, tc.Equals, "juju-exec")
	c.Assert(ctx.flushFailure, tc.IsNil)
	c.Assert(ctx.actionResults["return-code"], tc.Equals, 3)
	c.Assert(strings.TrimRight(ctx.actionResults["stdout"].(string), "\r\n"), tc.Equals, "1")
	c.Assert(ctx.actionResults["stderr"], tc.Equals, nil)
}

func (s *RunMockContextSuite) TestRunActionCancelled(c *tc.C) {
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
	_, err := runner.NewRunner(ctx, s.paths).RunAction(stdcontext.Background(), "juju-exec")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.flushBadge, tc.Equals, "juju-exec")
	c.Assert(ctx.flushFailure, tc.Equals, exec.ErrCancelled)
	c.Assert(ctx.actionResults["return-code"], tc.Equals, 0)
	c.Assert(ctx.actionResults["stdout"], tc.Equals, nil)
	c.Assert(ctx.actionResults["stderr"], tc.Equals, nil)
}

func (s *RunMockContextSuite) TestRunCommandsFlushSuccess(c *tc.C) {
	expectErr := errors.New("pew pew pew")
	ctx := &MockContext{
		flushResult: expectErr,
	}
	_, actualErr := runner.NewRunner(ctx, s.paths).RunCommands(stdcontext.Background(), echoPidScript)
	c.Assert(actualErr, tc.Equals, expectErr)
	c.Assert(ctx.flushBadge, tc.Equals, "run commands")
	c.Assert(ctx.flushFailure, tc.IsNil)
	s.assertRecordedPid(c, ctx.expectPid)
}

func (s *RunMockContextSuite) TestRunCommandsFlushFailure(c *tc.C) {
	expectErr := errors.New("pew pew pew")
	ctx := &MockContext{
		flushResult: expectErr,
	}
	_, actualErr := runner.NewRunner(ctx, s.paths).RunCommands(stdcontext.Background(), echoPidScript+"; exit 123")
	c.Assert(actualErr, tc.Equals, expectErr)
	c.Assert(ctx.flushBadge, tc.Equals, "run commands")
	c.Assert(ctx.flushFailure, tc.IsNil) // exit code in _ result, as tested elsewhere
	s.assertRecordedPid(c, ctx.expectPid)
}
