// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"io"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

var _ = gc.Suite(&controllerSuite{})

type controllerSuite struct {
	testing.BaseSuite
}

func (s *controllerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(&bootstrapReadyPollDelay, 1*time.Millisecond)
	defaultSeriesVersion := version.Current
	// Force a dev version by having a non zero build number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	defaultSeriesVersion.Build = 1234
	s.PatchValue(&version.Current, defaultSeriesVersion)
}

func (s *controllerSuite) TestWaitForAgentAPIReadyImmediate(c *gc.C) {
	count := 0
	tryAPI := func(c *modelcmd.ModelCommandBase) error {
		count++
		return nil
	}
	runInCommand(c, func(ctx *cmd.Context, base *modelcmd.ModelCommandBase) {
		bootstrapCtx := modelcmd.BootstrapContext(context.Background(), ctx)
		err := WaitForAgentInitialisation(bootstrapCtx, base, false, "arthur", tryAPI)
		c.Assert(err, jc.ErrorIsNil)
	})
}

func (s *controllerSuite) TestWaitForAgentAPIReadyAllErrors(c *gc.C) {
	count := 0
	tryAPI := func(c *modelcmd.ModelCommandBase) error {
		count++
		switch count {
		case 1:
			return io.EOF
		case 2:
			return api.ConnectionOpenTimedOut
		case 3:
			return api.ConnectionDialTimedOut
		case 4:
			return rpc.ErrShutdown
		case 5:
			return &rpc.RequestError{
				Message: params.CodeUpgradeInProgress,
				Code:    params.CodeUpgradeInProgress,
			}
		case 6:
			return dialError(syscall.EHOSTUNREACH)
		case 7:
			return dialError(syscall.ENETUNREACH)
		case 8:
			return dialError(syscall.ECONNREFUSED)
		case 9:
			return errors.Trace(&opaqueError{err: dialError(syscall.ECONNREFUSED)})
		}
		return nil
	}
	runInCommand(c, func(ctx *cmd.Context, base *modelcmd.ModelCommandBase) {
		bootstrapCtx := modelcmd.BootstrapContext(context.Background(), ctx)
		err := WaitForAgentInitialisation(bootstrapCtx, base, false, "arthur", tryAPI)
		c.Assert(err, jc.ErrorIsNil)
	})
}

func dialError(errno syscall.Errno) error {
	return &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: os.NewSyscallError("connect", errno),
	}
}

// opaqueError hides the underlying error message so retry classification
// cannot fall back to matching the error text.
type opaqueError struct {
	err error
}

func (e *opaqueError) Error() string { return "api dial failed" }

func (e *opaqueError) Unwrap() error { return e.err }

func (s *controllerSuite) TestIsRetryableNetworkError(c *gc.C) {
	for _, errno := range []syscall.Errno{
		syscall.EHOSTUNREACH,
		syscall.ENETUNREACH,
		syscall.ECONNREFUSED,
	} {
		c.Check(isRetryableNetworkError(dialError(errno)), jc.IsTrue,
			gc.Commentf("errno %v", errno))
	}
	c.Check(isRetryableNetworkError(dialError(syscall.ECONNRESET)), jc.IsFalse)
	c.Check(isRetryableNetworkError(errors.New("no route to host")), jc.IsFalse)
	c.Check(isRetryableNetworkError(nil), jc.IsFalse)
}

func (s *controllerSuite) TestWaitForAgentAPIReadyUnknownError(c *gc.C) {
	count := 0
	tryAPI := func(c *modelcmd.ModelCommandBase) error {
		count++
		return errors.New("foobar")
	}
	runInCommand(c, func(ctx *cmd.Context, base *modelcmd.ModelCommandBase) {
		bootstrapCtx := modelcmd.BootstrapContext(context.Background(), ctx)
		err := WaitForAgentInitialisation(bootstrapCtx, base, false, "arthur", tryAPI)
		c.Assert(err, jc.ErrorIs, unknownError)
		c.Assert(err, gc.ErrorMatches, `.*foobar`)
	})
}

func (s *controllerSuite) TestWaitForAgentAPIReadyExhaustedRetries(c *gc.C) {
	count := 0
	tryAPI := func(c *modelcmd.ModelCommandBase) error {
		count++
		return api.ConnectionOpenTimedOut
	}
	runInCommand(c, func(ctx *cmd.Context, base *modelcmd.ModelCommandBase) {
		bootstrapCtx := modelcmd.BootstrapContext(context.Background(), ctx)
		err := WaitForAgentInitialisation(bootstrapCtx, base, false, "arthur", tryAPI)
		c.Assert(err, gc.ErrorMatches, `unable to contact api server after.*`)
	})
}

func runInCommand(c *gc.C, run func(ctx *cmd.Context, base *modelcmd.ModelCommandBase)) {
	cmd := &testCommand{
		run: run,
	}
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	cmd.SetAPIOpen(func(*api.Info, api.DialOpts) (api.Connection, error) {
		return nil, errors.New("no API available")
	})

	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(cmd))
	c.Assert(err, jc.ErrorIsNil)
}

type testCommand struct {
	modelcmd.ModelCommandBase
	run func(ctx *cmd.Context, base *modelcmd.ModelCommandBase)
}

func (c *testCommand) Run(ctx *cmd.Context) error {
	c.run(ctx, &c.ModelCommandBase)
	return nil
}

func (c *testCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name: "test",
	})
}
