// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"io"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	environscmd "github.com/juju/juju/environs/cmd"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

var _ = gc.Suite(&controllerSuite{})

type controllerSuite struct {
	testing.BaseSuite
	mockBlockClient *mockBlockClient
}

func (s *controllerSuite) SetUpTest(c *gc.C) {
	s.mockBlockClient = &mockBlockClient{}
	s.PatchValue(&blockAPI, func(*modelcmd.ModelCommandBase) (listBlocksAPI, error) {
		err := s.mockBlockClient.loginError
		if err != nil {
			s.mockBlockClient.loginError = nil
			return nil, err
		}
		return s.mockBlockClient, nil
	})
}

type mockBlockClient struct {
	retryCount int
	numRetries int
	loginError error
}

var errOther = errors.New("other error")

func (c *mockBlockClient) List() ([]params.Block, error) {
	c.retryCount += 1
	if c.retryCount == 5 {
		return nil, &rpc.RequestError{Message: params.CodeUpgradeInProgress, Code: params.CodeUpgradeInProgress}
	}
	if c.numRetries < 0 {
		return nil, errOther
	}
	if c.retryCount < c.numRetries {
		return nil, &rpc.RequestError{Message: params.CodeUpgradeInProgress, Code: params.CodeUpgradeInProgress}
	}
	return []params.Block{}, nil
}

func (c *mockBlockClient) Close() error {
	return nil
}

func (s *controllerSuite) TestWaitForAgentAPIReadyRetries(c *gc.C) {
	s.PatchValue(&bootstrapReadyPollDelay, 1*time.Millisecond)
	s.PatchValue(&bootstrapReadyPollCount, 5)
	defaultSeriesVersion := version.Current
	// Force a dev version by having a non zero build number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	defaultSeriesVersion.Build = 1234
	s.PatchValue(&version.Current, defaultSeriesVersion)
	for _, t := range []struct {
		numRetries int
		err        error
	}{
		{0, nil}, // agent ready immediately
		{2, nil}, // agent ready after 2 polls
		{6, &rpc.RequestError{
			Message: params.CodeUpgradeInProgress,
			Code:    params.CodeUpgradeInProgress,
		}}, // agent ready after 6 polls but that's too long
		{-1, errOther}, // another error is returned
	} {
		s.mockBlockClient.numRetries = t.numRetries
		s.mockBlockClient.retryCount = 0
		runInCommand(c, func(ctx *cmd.Context, base *modelcmd.ModelCommandBase) {
			bootstrapCtx := environscmd.BootstrapContext(context.Background(), ctx)
			err := WaitForAgentInitialisation(bootstrapCtx, base, false, "controller")
			c.Check(errors.Cause(err), gc.DeepEquals, t.err)
		})
		expectedRetries := t.numRetries
		if t.numRetries <= 0 {
			expectedRetries = 1
		}
		// Only retry maximum of bootstrapReadyPollCount times.
		if expectedRetries > 5 {
			expectedRetries = 5
		}
		c.Check(s.mockBlockClient.retryCount, gc.Equals, expectedRetries)
	}
}

func (s *controllerSuite) TestWaitForAgentAPIReadyRetriesWithOpenEOFErr(c *gc.C) {
	s.mockBlockClient.numRetries = 0
	s.mockBlockClient.retryCount = 0
	s.mockBlockClient.loginError = io.EOF

	runInCommand(c, func(ctx *cmd.Context, base *modelcmd.ModelCommandBase) {
		bootstrapCtx := environscmd.BootstrapContext(context.Background(), ctx)
		err := WaitForAgentInitialisation(bootstrapCtx, base, false, "controller")
		c.Check(err, jc.ErrorIsNil)
	})
	c.Check(s.mockBlockClient.retryCount, gc.Equals, 1)
}

func (s *controllerSuite) TestWaitForAgentAPIReadyStopsRetriesWithOpenErr(c *gc.C) {
	s.mockBlockClient.numRetries = 0
	s.mockBlockClient.retryCount = 0
	s.mockBlockClient.loginError = errors.NewUnauthorized(nil, "")
	runInCommand(c, func(ctx *cmd.Context, base *modelcmd.ModelCommandBase) {
		bootstrapCtx := environscmd.BootstrapContext(context.Background(), ctx)
		err := WaitForAgentInitialisation(bootstrapCtx, base, false, "controller")
		c.Check(err, jc.ErrorIs, errors.Unauthorized)
	})
	c.Check(s.mockBlockClient.retryCount, gc.Equals, 0)
}

func (s *controllerSuite) TestWaitForAgentCancelled(c *gc.C) {
	s.mockBlockClient.numRetries = 2
	runInCommand(c, func(ctx *cmd.Context, base *modelcmd.ModelCommandBase) {
		stdCtx, cancel := context.WithCancel(context.Background())
		cancel()
		bootstrapCtx := environscmd.BootstrapContext(stdCtx, ctx)
		err := WaitForAgentInitialisation(bootstrapCtx, base, false, "controller")
		c.Check(err, gc.ErrorMatches, `unable to contact api server: .*`)
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
