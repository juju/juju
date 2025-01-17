// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"context"
	time "time"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type RemoteSuite struct {
	baseSuite

	apiConnect chan struct{}

	apiConnection *MockConnection
}

var _ = gc.Suite(&RemoteSuite{})

func (s *RemoteSuite) TestNotConnectedConnection(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	c.Assert(w.Connection(), gc.IsNil)

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) TestUpdateAddress(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	addrs := []string{"10.0.0.1"}

	s.apiConnection.EXPECT().Broken().Return(make(<-chan struct{}))
	s.apiConnection.EXPECT().Close().Return(nil)
	s.apiConnection.EXPECT().Addr().Return(addrs[0])

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.UpdateAddresses(addrs)

	select {
	case <-s.apiConnect:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for API connect")
	}

	c.Assert(w.Connection().Addr(), jc.DeepEquals, addrs[0])

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.apiConnection = NewMockConnection(ctrl)
	s.apiConnect = make(chan struct{})

	return ctrl
}

func (s *RemoteSuite) newRemoteServer(c *gc.C) RemoteServer {
	return newRemoteServer(s.newConfig(c), s.states)
}

func (s *RemoteSuite) newConfig(c *gc.C) RemoteServerConfig {
	return RemoteServerConfig{
		Clock:   s.clock,
		Logger:  loggertesting.WrapCheckLog(c),
		APIInfo: &api.Info{},
		APIOpener: func(ctx context.Context, i *api.Info, do api.DialOpts) (api.Connection, error) {
			close(s.apiConnect)
			return s.apiConnection, nil
		},
	}
}

func (s *RemoteSuite) expectClockAfter(ch <-chan time.Time) {
	s.clock.EXPECT().After(gomock.Any()).DoAndReturn(func(d time.Duration) <-chan time.Time {
		return ch
	}).AnyTimes()
}
