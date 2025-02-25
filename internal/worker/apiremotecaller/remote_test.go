// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"context"
	"net/url"
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

	ch := w.Connection(context.Background())
	select {
	case <-ch:
		c.Fatalf("expected connection to block")
	case <-time.After(jujutesting.ShortWait):
	}

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) TestConnect(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	addr := &url.URL{Scheme: "wss", Host: "10.0.0.1"}

	s.apiConnection.EXPECT().Broken().Return(make(<-chan struct{}))
	s.apiConnection.EXPECT().Close().Return(nil)
	s.apiConnection.EXPECT().Addr().Return(addr)

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.UpdateAddresses([]string{addr.String()})

	select {
	case <-s.apiConnect:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for API connect")
	}

	s.ensureChanged(c)

	ch := w.Connection(context.Background())

	select {
	case conn := <-ch:
		c.Assert(conn, gc.NotNil)
		c.Check(conn.Addr().String(), jc.DeepEquals, addr.String())
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for connection")
	}

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) TestConnectWhenAlreadyContextCancelled(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ch := w.Connection(ctx)

	select {
	case <-ch:
		c.Fatalf("expected connection to block")
	case <-time.After(jujutesting.ShortWait):
	}

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) TestConnectWhenAlreadyKilled(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	workertest.CleanKill(c, w)

	ch := w.Connection(context.Background())

	select {
	case <-ch:
		c.Fatalf("expected connection to block")
	case <-time.After(jujutesting.ShortWait):
	}
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

	addr0 := &url.URL{Scheme: "wss", Host: "10.0.0.1"}
	addr1 := &url.URL{Scheme: "wss", Host: "10.0.0.2"}

	s.apiConnection.EXPECT().Broken().Return(make(<-chan struct{}))
	s.apiConnection.EXPECT().Close().Return(nil)
	s.apiConnection.EXPECT().Addr().Return(addr1)

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	// UpdateAddresses will block the first connection, so we can trigger a
	// connection failure. The second UpdateAddresses should then cancel the
	// current connection and start a new one, that one should then succeed.
	w.UpdateAddresses([]string{addr0.String()})
	w.UpdateAddresses([]string{addr1.String()})

	select {
	case <-s.apiConnect:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for API connect")
	}

	s.ensureChanged(c)

	ch := w.Connection(context.Background())

	select {
	case conn := <-ch:
		c.Assert(conn, gc.NotNil)
		c.Check(conn.Addr().String(), jc.DeepEquals, addr1.String())
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for connection")
	}

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

	addr := &url.URL{Scheme: "wss", Host: "10.0.0.1"}

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.UpdateAddresses([]string{addr.String()})

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

	addr := &url.URL{Scheme: "wss", Host: "10.0.0.1"}

	s.apiConnection.EXPECT().Broken().Return(make(<-chan struct{}))
	s.apiConnection.EXPECT().Close().Return(nil)

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.UpdateAddresses([]string{addr.String()})

	select {
	case <-s.apiConnect:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for API connect")
	}

	w.UpdateAddresses([]string{addr.String()})

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
