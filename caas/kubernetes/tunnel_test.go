// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"io"
	"sync"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/tools/portforward"

	"github.com/juju/juju/internal/testhelpers"
)

type tunnelSuite struct {
	testhelpers.IsolationSuite
}

func TestTunnelSuite(t *testing.T) {
	tc.Run(t, &tunnelSuite{})
}

func (s *tunnelSuite) TestCloseIsConcurrentSafe(c *tc.C) {
	tunnel := newTestTunnel()
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			tunnel.Close()
		})
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for concurrent closes")
	}
	select {
	case <-tunnel.stopChan:
	default:
		c.Fatalf("stop channel was not closed")
	}
}

func (s *tunnelSuite) TestForwardPortBindsIPv4LocalhostAndUsesAssignedPort(c *tc.C) {
	var gotAddresses []string
	var gotPorts []string
	done := make(chan struct{})
	forwarder := &fakePortForwarder{
		done:        done,
		signalReady: true,
		waitForStop: true,
		forwardedPorts: []portforward.ForwardedPort{{
			Local:  33419,
			Remote: 17070,
		}},
	}
	s.PatchValue(&newPortForwarder, func(
		_ httpstream.Dialer,
		addresses []string,
		ports []string,
		stopChan <-chan struct{},
		readyChan chan struct{},
		_, _ io.Writer,
	) (portForwarder, error) {
		gotAddresses = append([]string(nil), addresses...)
		gotPorts = append([]string(nil), ports...)
		forwarder.stopChan = stopChan
		forwarder.readyChan = readyChan
		return forwarder, nil
	})

	tunnel := newTestTunnel()
	err := tunnel.forwardPort(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotAddresses, tc.DeepEquals, []string{"127.0.0.1"})
	c.Check(gotPorts, tc.DeepEquals, []string{"0:17070"})
	c.Check(tunnel.LocalPort, tc.Equals, "33419")

	tunnel.Close()
	assertDone(c, done)
}

func (s *tunnelSuite) TestForwardPortReportsFailureBeforeReady(c *tc.C) {
	done := make(chan struct{})
	forwarder := &fakePortForwarder{
		done:       done,
		forwardErr: errors.New("lost connection to pod"),
	}
	s.PatchValue(&newPortForwarder, func(
		_ httpstream.Dialer,
		_ []string,
		_ []string,
		stopChan <-chan struct{},
		readyChan chan struct{},
		_, _ io.Writer,
	) (portForwarder, error) {
		forwarder.stopChan = stopChan
		forwarder.readyChan = readyChan
		return forwarder, nil
	})

	tunnel := newTestTunnel()
	err := tunnel.forwardPort(c.Context(), nil)
	c.Assert(err, tc.ErrorMatches, "forwarding ports: lost connection to pod")
	assertDone(c, done)
}

func (s *tunnelSuite) TestForwardPortStopsWhenAssignedPortCannotBeRead(c *tc.C) {
	done := make(chan struct{})
	forwarder := &fakePortForwarder{
		done:        done,
		signalReady: true,
		waitForStop: true,
		getPortsErr: errors.New("ports unavailable"),
	}
	s.PatchValue(&newPortForwarder, func(
		_ httpstream.Dialer,
		_ []string,
		_ []string,
		stopChan <-chan struct{},
		readyChan chan struct{},
		_, _ io.Writer,
	) (portForwarder, error) {
		forwarder.stopChan = stopChan
		forwarder.readyChan = readyChan
		return forwarder, nil
	})

	tunnel := newTestTunnel()
	err := tunnel.forwardPort(c.Context(), nil)
	c.Assert(err, tc.ErrorMatches, "getting forwarded ports: ports unavailable")
	assertDone(c, done)
}

func (s *tunnelSuite) TestForwardPortReportsPostReadyFailure(c *tc.C) {
	done := make(chan struct{})
	release := make(chan struct{})
	forwarder := &fakePortForwarder{
		done:        done,
		release:     release,
		signalReady: true,
		forwardErr:  errors.New("lost connection to pod"),
		forwardedPorts: []portforward.ForwardedPort{{
			Local:  33419,
			Remote: 17070,
		}},
	}
	s.PatchValue(&newPortForwarder, func(
		_ httpstream.Dialer,
		_ []string,
		_ []string,
		stopChan <-chan struct{},
		readyChan chan struct{},
		_, _ io.Writer,
	) (portForwarder, error) {
		forwarder.stopChan = stopChan
		forwarder.readyChan = readyChan
		return forwarder, nil
	})

	tunnel := newTestTunnel()
	err := tunnel.forwardPort(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)

	close(release)
	assertDone(c, done)
	c.Check(tunnel.ForwardError(), tc.ErrorMatches, "lost connection to pod")
}

type fakePortForwarder struct {
	done           chan struct{}
	forwardErr     error
	forwardedPorts []portforward.ForwardedPort
	getPortsErr    error
	readyChan      chan struct{}
	release        chan struct{}
	signalReady    bool
	stopChan       <-chan struct{}
	waitForStop    bool
}

func (f *fakePortForwarder) ForwardPorts() error {
	if f.done != nil {
		defer close(f.done)
	}
	if f.signalReady {
		close(f.readyChan)
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-f.stopChan:
			return nil
		}
	}
	if f.waitForStop {
		<-f.stopChan
	}
	return f.forwardErr
}

func (f *fakePortForwarder) GetPorts() ([]portforward.ForwardedPort, error) {
	if f.getPortsErr != nil {
		return nil, f.getPortsErr
	}
	return f.forwardedPorts, nil
}

func newTestTunnel() *Tunnel {
	return &Tunnel{
		Out:        io.Discard,
		readyChan:  make(chan struct{}),
		RemotePort: "17070",
		stopChan:   make(chan struct{}),
	}
}

func assertDone(c *tc.C, done <-chan struct{}) {
	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for forwarding goroutine")
	}
}
