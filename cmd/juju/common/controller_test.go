// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"io"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	cmdtesting "github.com/juju/juju/cmd/testing"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc"
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
		if s.mockBlockClient.discoveringSpacesError > 0 {
			s.mockBlockClient.discoveringSpacesError -= 1
			return nil, errors.New("spaces are still being discovered")
		}
		return s.mockBlockClient, nil
	})
}

type mockBlockClient struct {
	retryCount             int
	numRetries             int
	discoveringSpacesError int
	loginError             error
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
		cmd := &modelcmd.ModelCommandBase{}
		cmd.SetClientStore(jujuclienttesting.NewMemStore())
		err := WaitForAgentInitialisation(cmdtesting.NullContext(c), cmd, "controller", "default")
		c.Check(errors.Cause(err), gc.DeepEquals, t.err)
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

func (s *controllerSuite) TestWaitForAgentAPIReadyWaitsForSpaceDiscovery(c *gc.C) {
	s.mockBlockClient.discoveringSpacesError = 2
	cmd := &modelcmd.ModelCommandBase{}
	cmd.SetClientStore(jujuclienttesting.NewMemStore())
	err := WaitForAgentInitialisation(cmdtesting.NullContext(c), cmd, "controller", "default")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockBlockClient.discoveringSpacesError, gc.Equals, 0)
}

func (s *controllerSuite) TestWaitForAgentAPIReadyRetriesWithOpenEOFErr(c *gc.C) {
	s.mockBlockClient.numRetries = 0
	s.mockBlockClient.retryCount = 0
	s.mockBlockClient.loginError = io.EOF
	cmd := &modelcmd.ModelCommandBase{}
	cmd.SetClientStore(jujuclienttesting.NewMemStore())
	err := WaitForAgentInitialisation(cmdtesting.NullContext(c), cmd, "controller", "default")
	c.Check(err, jc.ErrorIsNil)

	c.Check(s.mockBlockClient.retryCount, gc.Equals, 1)
}

func (s *controllerSuite) TestWaitForAgentAPIReadyStopsRetriesWithOpenErr(c *gc.C) {
	s.mockBlockClient.numRetries = 0
	s.mockBlockClient.retryCount = 0
	s.mockBlockClient.loginError = errors.NewUnauthorized(nil, "")
	cmd := &modelcmd.ModelCommandBase{}
	cmd.SetClientStore(jujuclienttesting.NewMemStore())
	err := WaitForAgentInitialisation(cmdtesting.NullContext(c), cmd, "controller", "default")
	c.Check(err, jc.Satisfies, errors.IsUnauthorized)

	c.Check(s.mockBlockClient.retryCount, gc.Equals, 0)
}
