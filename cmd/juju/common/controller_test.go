// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"io"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/jujuclient/jujuclienttesting"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/version"
	environscmd "github.com/juju/juju/environs/cmd"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
)

func TestControllerSuite(t *stdtesting.T) {
	tc.Run(t, &controllerSuite{})
}

type controllerSuite struct {
	testing.BaseSuite
}

func (s *controllerSuite) TestWaitForAgentAPIReadyRetries(c *tc.C) {
	s.PatchValue(&bootstrapReadyPollDelay, 1*time.Millisecond)
	defaultSeriesVersion := version.Current
	// Force a dev version by having a non zero build number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	defaultSeriesVersion.Build = 1234
	s.PatchValue(&version.Current, defaultSeriesVersion)

	c.Run("Immediate", func(c *stdtesting.T) {
		count := 0
		tryAPI := func(ctx context.Context, c *modelcmd.ModelCommandBase) error {
			count++
			return nil
		}
		runInCommand(&tc.TBC{TB: c}, func(ctx *cmd.Context, base *modelcmd.ModelCommandBase) {
			bootstrapCtx := environscmd.BootstrapContext(c.Context(), ctx)
			err := WaitForAgentInitialisation(bootstrapCtx, base, false, "arthur", tryAPI)
			tc.Assert(c, err, tc.ErrorIsNil)
		})
	})

	c.Run("AllErrors", func(c *stdtesting.T) {
		count := 0
		tryAPI := func(ctx context.Context, c *modelcmd.ModelCommandBase) error {
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
			}
			return nil
		}
		runInCommand(&tc.TBC{TB: c}, func(ctx *cmd.Context, base *modelcmd.ModelCommandBase) {
			bootstrapCtx := environscmd.BootstrapContext(c.Context(), ctx)
			err := WaitForAgentInitialisation(bootstrapCtx, base, false, "arthur", tryAPI)
			tc.Assert(c, err, tc.ErrorIsNil)
		})
	})

	c.Run("UnknownError", func(c *stdtesting.T) {
		count := 0
		tryAPI := func(ctx context.Context, c *modelcmd.ModelCommandBase) error {
			count++
			return errors.New("foobar")
		}
		runInCommand(&tc.TBC{TB: c}, func(ctx *cmd.Context, base *modelcmd.ModelCommandBase) {
			bootstrapCtx := environscmd.BootstrapContext(c.Context(), ctx)
			err := WaitForAgentInitialisation(bootstrapCtx, base, false, "arthur", tryAPI)
			tc.Assert(c, err, tc.ErrorIs, unknownError)
			tc.Assert(c, err, tc.ErrorMatches, `.*foobar`)
		})
	})

	c.Run("ExhaustedRetries", func(c *stdtesting.T) {
		count := 0
		tryAPI := func(ctx context.Context, c *modelcmd.ModelCommandBase) error {
			count++
			return api.ConnectionOpenTimedOut
		}
		runInCommand(&tc.TBC{TB: c}, func(ctx *cmd.Context, base *modelcmd.ModelCommandBase) {
			bootstrapCtx := environscmd.BootstrapContext(c.Context(), ctx)
			err := WaitForAgentInitialisation(bootstrapCtx, base, false, "arthur", tryAPI)
			tc.Assert(c, err, tc.ErrorMatches, `unable to contact api server after.*`)
		})
	})
}

func runInCommand(c tc.LikeC, run func(ctx *cmd.Context, base *modelcmd.ModelCommandBase)) {
	cmd := &testCommand{
		run: run,
	}
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	cmd.SetAPIOpen(func(context.Context, *api.Info, api.DialOpts) (api.Connection, error) {
		return nil, errors.New("no API available")
	})

	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(cmd))
	tc.Assert(c, err, tc.ErrorIsNil)
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
