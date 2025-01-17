// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"context"
	"sync/atomic"
	"time"

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

	apiConnect        chan struct{}
	apiConnectHandler func(context.Context) error

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

func (s *RemoteSuite) TestConnect(c *gc.C) {
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

func (s *RemoteSuite) TestConnectWhilstConnecting(c *gc.C) {
	defer s.setupMocks(c).Finish()

	var counter atomic.Int64
	s.apiConnectHandler = func(ctx context.Context) error {
		v := counter.Add(1)
		if v == 1 {
			// This should block the apiConnection.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(jujutesting.LongWait):
				c.Fatalf("timed out waiting for context to be done")
			}
		}
		close(s.apiConnect)
		return nil
	}

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	addrs0 := []string{"10.0.0.1"}
	addrs1 := []string{"10.0.0.2"}

	s.apiConnection.EXPECT().Broken().Return(make(<-chan struct{}))
	s.apiConnection.EXPECT().Close().Return(nil)
	s.apiConnection.EXPECT().Addr().Return(addrs1[0])

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	// UpdateAddresses will block the first connection, so we can trigger a
	// connection failure. The second UpdateAddresses should then cancel the
	// current connection and start a new one, that one should then succeed.
	w.UpdateAddresses(addrs0)
	w.UpdateAddresses(addrs1)

	select {
	case <-s.apiConnect:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for API connect")
	}

	c.Assert(w.Connection().Addr(), jc.DeepEquals, addrs1[0])

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) TestConnectBlocks(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.apiConnectHandler = func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(jujutesting.LongWait):
			c.Fatalf("timed out waiting for context to be done")
		}
		return nil
	}

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	addrs := []string{"10.0.0.1"}

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.UpdateAddresses(addrs)

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) TestConnectWithSameAddress(c *gc.C) {
	defer s.setupMocks(c).Finish()

	var counter atomic.Int64
	s.apiConnectHandler = func(ctx context.Context) error {
		counter.Add(1)

		select {
		case s.apiConnect <- struct{}{}:
		case <-time.After(time.Second):
			c.Fatalf("timed out waiting for API connect")
		}
		return nil
	}

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	addrs := []string{"10.0.0.1"}

	s.apiConnection.EXPECT().Broken().Return(make(<-chan struct{}))
	s.apiConnection.EXPECT().Close().Return(nil)

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.UpdateAddresses(addrs)

	select {
	case <-s.apiConnect:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for API connect")
	}

	w.UpdateAddresses(addrs)

	select {
	case <-s.apiConnect:
		c.Fatalf("the connection should not be called")
	case <-time.After(time.Second):
	}

	c.Assert(counter.Load(), gc.Equals, int64(1))

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.apiConnection = NewMockConnection(ctrl)

	s.apiConnect = make(chan struct{})
	s.apiConnectHandler = func(ctx context.Context) error {
		close(s.apiConnect)
		return nil
	}

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
			err := s.apiConnectHandler(ctx)
			if err != nil {
				return nil, err
			}
			return s.apiConnection, nil
		},
	}
}

func (s *RemoteSuite) expectClockAfter(ch <-chan time.Time) {
	s.clock.EXPECT().After(gomock.Any()).DoAndReturn(func(d time.Duration) <-chan time.Time {
		return ch
	}).AnyTimes()
}
