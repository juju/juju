// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"context"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/api"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type RemoteSuite struct {
	baseSuite

	apiConnect        chan struct{}
	apiConnectHandler func(context.Context) error

	apiConnection *MockConnection
}

func TestRemoteSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &RemoteSuite{})
}

func (s *RemoteSuite) TestControllerID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	controllerID := w.ControllerID()
	c.Assert(controllerID, tc.Equals, "0")

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) TestNotConnectedConnection(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	ctx, cancel := context.WithTimeout(c.Context(), testhelpers.ShortWait)
	defer cancel()

	var called bool
	err := w.Connection(ctx, func(ctx context.Context, c api.Connection) error {
		called = true
		return nil
	})
	c.Assert(err, tc.ErrorIs, context.DeadlineExceeded)
	c.Check(called, tc.IsFalse)

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) TestConnect(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	addr := &url.URL{Scheme: "wss", Host: "10.0.0.1"}

	s.apiConnection.EXPECT().Broken().Return(make(<-chan struct{})).MinTimes(1)
	s.apiConnection.EXPECT().Close().Return(nil)
	s.apiConnection.EXPECT().Addr().Return(addr)

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.UpdateAddresses([]string{addr.String()})

	select {
	case <-s.apiConnect:
	case <-c.Context().Done():
		c.Fatalf("waiting for API connect: %v", c.Context().Err())
	}

	s.ensureChanged(c)

	var conn api.Connection
	err := w.Connection(c.Context(), func(ctx context.Context, c api.Connection) error {
		conn = c
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(conn, tc.NotNil)
	c.Check(conn.Addr().String(), tc.DeepEquals, addr.String())

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) TestConnectWhenAlreadyContextCancelled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	var called bool
	err := w.Connection(ctx, func(ctx context.Context, c api.Connection) error {
		called = true
		return nil
	})
	c.Assert(err, tc.ErrorIs, context.Canceled)
	c.Check(called, tc.IsFalse)

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) TestConnectWhenAlreadyKilled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	workertest.CleanKill(c, w)

	var called bool
	err := w.Connection(c.Context(), func(ctx context.Context, c api.Connection) error {
		called = true
		return nil
	})
	c.Assert(err, tc.ErrorIs, tomb.ErrDying)
	c.Check(called, tc.IsFalse)
}

func (s *RemoteSuite) TestConnectMultipleWithFirstCancelled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// This test ensures that when the first connection is cancelled, the second
	// connection is not stalled.

	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()

	s.apiConnectHandler = func(ctx context.Context) error {
		cancel()
		close(s.apiConnect)
		return nil
	}

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	addr := &url.URL{Scheme: "wss", Host: "10.0.0.1"}

	s.apiConnection.EXPECT().Broken().Return(make(<-chan struct{})).MinTimes(1)
	s.apiConnection.EXPECT().Close().Return(nil)

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	var wg sync.WaitGroup
	wg.Add(2)

	res := make(chan error)
	seq := make(chan struct{})
	go func() {
		// Force the first connection to be enqueued, so that second connection
		// will be stalled.
		go func() {
			<-time.After(time.Millisecond * 100)
			close(seq)
		}()

		wg.Done()

		var called bool
		err := w.Connection(ctx, func(ctx context.Context, c api.Connection) error {
			called = true
			return nil
		})
		c.Assert(err, tc.ErrorIs, context.Canceled)
		c.Check(called, tc.IsFalse)
	}()
	go func() {
		// Wait for the first connection to be enqueued.
		<-seq
		wg.Done()

		err := w.Connection(c.Context(), func(ctx context.Context, c api.Connection) error {
			return nil
		})
		res <- err
	}()

	// Ensure both goroutines have started, before we start the test.
	sync := make(chan struct{})
	go func() {
		wg.Wait()
		close(sync)
	}()
	select {
	case <-sync:
	case <-c.Context().Done():
		c.Fatalf("waiting for connections to finish: %v", c.Context().Err())
	}

	select {
	case <-seq:
	case <-c.Context().Done():
		c.Fatalf("waiting for first connection to be cancelled: %v", c.Context().Err())
	}

	w.UpdateAddresses([]string{addr.String()})

	// This is our sequence point to ensure that we connect.
	select {
	case <-s.apiConnect:
	case <-c.Context().Done():
		c.Fatalf("waiting for API connect: %v", c.Context().Err())
	}

	s.ensureChanged(c)

	select {
	case err := <-res:
		c.Assert(err, tc.ErrorIsNil)
	case <-c.Context().Done():
		c.Fatalf("waiting for connection: %v", c.Context().Err())
	}

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) TestConnectWhilstConnecting(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var counter atomic.Int64
	s.apiConnectHandler = func(ctx context.Context) error {
		v := counter.Add(1)
		if v == 1 {
			// This should block the apiConnection.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-c.Context().Done():
				c.Fatalf("waiting for context to be done: %v", c.Context().Err())
			}
		}
		close(s.apiConnect)
		return nil
	}

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	addr0 := &url.URL{Scheme: "wss", Host: "10.0.0.1"}
	addr1 := &url.URL{Scheme: "wss", Host: "10.0.0.2"}

	s.apiConnection.EXPECT().Broken().Return(make(<-chan struct{})).MinTimes(1)
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
	case <-c.Context().Done():
		c.Fatalf("waiting for API connect: %v", c.Context().Err())
	}

	s.ensureChanged(c)

	var conn api.Connection
	err := w.Connection(c.Context(), func(ctx context.Context, c api.Connection) error {
		conn = c
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(conn, tc.NotNil)
	c.Check(conn.Addr().String(), tc.DeepEquals, addr1.String())

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) TestConnectBlocks(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.apiConnectHandler = func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.Context().Done():
			c.Fatalf("waiting for context to be done: %v", c.Context().Err())
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

func (s *RemoteSuite) TestConnectWithSameAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var counter atomic.Int64
	s.apiConnectHandler = func(ctx context.Context) error {
		counter.Add(1)

		select {
		case s.apiConnect <- struct{}{}:
		case <-c.Context().Done():
			c.Fatalf("waiting for API connect: %v", c.Context().Err())
		}
		return nil
	}

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	addr := &url.URL{Scheme: "wss", Host: "10.0.0.1"}

	s.apiConnection.EXPECT().Broken().Return(make(<-chan struct{})).MinTimes(1)
	s.apiConnection.EXPECT().Close().Return(nil).Times(2)

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.UpdateAddresses([]string{addr.String()})

	select {
	case <-s.apiConnect:
	case <-c.Context().Done():
		c.Fatalf("waiting for API connect: %v", c.Context().Err())
	}

	// Fix a race condition by making sure the connection has been correctly
	// established. Without this instruction, the second UpdateAddresses can
	// trigger a canceler(newChangeRequestError),
	// which cancels the establishment of the previous connection and makes the
	// test flaky
	err := w.Connection(c.Context(),
		func(ctx context.Context, c api.Connection) error {
			return nil
		})
	c.Assert(err, tc.ErrorIsNil)

	w.UpdateAddresses([]string{addr.String()})
	addr2 := &url.URL{Scheme: "wss", Host: "10.0.0.2"}
	w.UpdateAddresses([]string{addr2.String()})

	select {
	case <-s.apiConnect:
	case <-c.Context().Done():
		c.Fatalf("waiting for API connect: %v", c.Context().Err())
	}

	c.Assert(counter.Load(), tc.Equals, int64(2))

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) TestConnectWithBrokenConnection(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var counter atomic.Int64
	s.apiConnectHandler = func(ctx context.Context) error {
		counter.Add(1)

		select {
		case s.apiConnect <- struct{}{}:
		case <-c.Context().Done():
			c.Fatalf("waiting for API connect: %v", c.Context().Err())
		}
		return nil
	}

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	addr := &url.URL{Scheme: "wss", Host: "10.0.0.1"}

	broken := make(chan struct{})

	// We require the ordering here to simulate the first connection breaking,
	// then a new connection being made.
	gomock.InOrder(
		s.apiConnection.EXPECT().Broken().Return(broken),
		s.apiConnection.EXPECT().Close().Return(nil),
		s.apiConnection.EXPECT().Broken().Return(make(<-chan struct{})),
		s.apiConnection.EXPECT().Close().Return(nil),
	)

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.UpdateAddresses([]string{addr.String()})

	select {
	case <-s.apiConnect:
	case <-c.Context().Done():
		c.Fatalf("waiting for API connect: %v", c.Context().Err())
	}

	close(broken)

	select {
	case <-s.apiConnect:
	case <-c.Context().Done():
		c.Fatalf("waiting for API connect: %v", c.Context().Err())
	}

	c.Assert(counter.Load(), tc.Equals, int64(2))

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) TestReconnectWithBrokenConnectionMultipleUpdatesKeepProgress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	reconnectStarted := make(chan struct{})
	releaseReconnect := make(chan struct{})

	var attempts atomic.Int64
	s.apiConnectHandler = func(ctx context.Context) error {
		switch attempts.Add(1) {
		case 1, 3:
			select {
			case s.apiConnect <- struct{}{}:
			case <-c.Context().Done():
				c.Fatalf("waiting for API connect: %v", c.Context().Err())
			}
			return nil
		case 2:
			close(reconnectStarted)
			select {
			case <-releaseReconnect:
			case <-c.Context().Done():
				c.Fatalf("waiting for reconnect release: %v", c.Context().Err())
			}
			<-ctx.Done()
			return context.Cause(ctx)
		default:
			return nil
		}
	}

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	addr0 := &url.URL{Scheme: "wss", Host: "10.0.0.1"}
	addr1 := &url.URL{Scheme: "wss", Host: "10.0.0.2"}
	addr2 := &url.URL{Scheme: "wss", Host: "10.0.0.3"}

	broken := make(chan struct{})

	gomock.InOrder(
		s.apiConnection.EXPECT().Broken().Return(broken),
		s.apiConnection.EXPECT().Close().Return(nil),
		s.apiConnection.EXPECT().Broken().Return(make(<-chan struct{})),
		s.apiConnection.EXPECT().Close().Return(nil),
	)

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.UpdateAddresses([]string{addr0.String()})
	select {
	case <-s.apiConnect:
	case <-c.Context().Done():
		c.Fatalf("waiting for initial API connect: %v", c.Context().Err())
	}
	s.ensureChanged(c)

	close(broken)
	select {
	case <-reconnectStarted:
	case <-c.Context().Done():
		c.Fatalf("waiting for reconnect attempt: %v", c.Context().Err())
	}

	w.UpdateAddresses([]string{addr1.String()})

	secondUpdateDone := make(chan struct{})
	go func() {
		w.UpdateAddresses([]string{addr2.String()})
		close(secondUpdateDone)
	}()

	select {
	case <-secondUpdateDone:
	case <-c.Context().Done():
		c.Fatalf("waiting for second address update: %v", c.Context().Err())
	}

	close(releaseReconnect)

	select {
	case <-s.apiConnect:
	case <-c.Context().Done():
		c.Fatalf("waiting for reconnect API connect: %v", c.Context().Err())
	}
	s.ensureChanged(c)

	report := w.Report(c.Context())
	addresses, ok := report["addresses"].([]string)
	c.Assert(ok, tc.IsTrue)
	c.Assert(addresses, tc.DeepEquals, []string{addr2.String()})
	c.Assert(attempts.Load(), tc.GreaterThan, int64(2))
	c.Assert(attempts.Load(), tc.LessThan, int64(5))

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) TestReportReturnsAddressSnapshot(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	addr := &url.URL{Scheme: "wss", Host: "10.0.0.1"}

	s.apiConnection.EXPECT().Broken().Return(make(<-chan struct{})).MinTimes(1)
	s.apiConnection.EXPECT().Close().Return(nil)

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.UpdateAddresses([]string{addr.String()})

	select {
	case <-s.apiConnect:
	case <-c.Context().Done():
		c.Fatalf("waiting for API connect: %v", c.Context().Err())
	}

	s.ensureChanged(c)

	report := w.Report(c.Context())
	addresses, ok := report["addresses"].([]string)
	c.Assert(ok, tc.IsTrue)
	c.Assert(addresses, tc.DeepEquals, []string{addr.String()})

	addresses[0] = "wss://10.0.0.99"

	report = w.Report(c.Context())
	addresses, ok = report["addresses"].([]string)
	c.Assert(ok, tc.IsTrue)
	c.Assert(addresses, tc.DeepEquals, []string{addr.String()})

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.apiConnection = NewMockConnection(ctrl)

	s.apiConnect = make(chan struct{})
	s.apiConnectHandler = func(ctx context.Context) error {
		close(s.apiConnect)
		return nil
	}

	return ctrl
}

func (s *RemoteSuite) newRemoteServer(c *tc.C) RemoteServer {
	return newRemoteServer(s.newConfig(c), s.states)
}

func (s *RemoteSuite) newConfig(c *tc.C) RemoteServerConfig {
	return RemoteServerConfig{
		Clock:        s.clock,
		Logger:       loggertesting.WrapCheckLog(c),
		APIInfo:      &api.Info{},
		ControllerID: "0",
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
