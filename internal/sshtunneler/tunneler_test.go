// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gossh "golang.org/x/crypto/ssh"
	gc "gopkg.in/check.v1"

	network "github.com/juju/juju/core/network"
	"github.com/juju/juju/pki/test"
	"github.com/juju/juju/state"
)

type sshTunnelerSuite struct {
	state      *MockState
	controller *MockControllerInfo
	dialer     *MockSSHDial
	clock      *MockClock
}

var _ = gc.Suite(&sshTunnelerSuite{})

func (s *sshTunnelerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.controller = NewMockControllerInfo(ctrl)
	s.dialer = NewMockSSHDial(ctrl)
	s.clock = NewMockClock(ctrl)

	return ctrl
}

func (s *sshTunnelerSuite) newTracker(c *gc.C) *Tracker {
	args := TrackerArgs{
		State:          s.state,
		ControllerInfo: s.controller,
		Dialer:         s.dialer,
		Clock:          s.clock,
	}
	tunnelTracker, err := NewTracker(args)
	c.Assert(err, jc.ErrorIsNil)
	return tunnelTracker
}

func (s *sshTunnelerSuite) TestTunneler(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTracker(c)

	sshConnArgs := state.SSHConnRequestArg{}

	// use a channel to wait for the tunnel request to be processed
	tunnelRequested := make(chan struct{})

	now := time.Now()

	machineHostKey, err := test.InsecureKeyProfile()
	c.Assert(err, jc.ErrorIsNil)
	sshPublicHostKey, err := gossh.NewPublicKey(machineHostKey.Public())
	c.Assert(err, jc.ErrorIsNil)

	var hostKeyCallback gossh.HostKeyCallback

	s.controller.EXPECT().Addresses().Return([]network.SpaceAddress{
		{MachineAddress: network.NewMachineAddress("1.2.3.4")},
	}, nil)
	s.state.EXPECT().InsertSSHConnRequest(gomock.Any()).DoAndReturn(
		func(sra state.SSHConnRequestArg) error {
			sshConnArgs = sra
			close(tunnelRequested)
			return nil
		},
	)
	s.state.EXPECT().MachineHostKeys(gomock.Any(), gomock.Any()).Return(
		[]string{string(gossh.MarshalAuthorizedKey(sshPublicHostKey))}, nil)
	s.dialer.EXPECT().Dial(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(c net.Conn, s1 string, s2 gossh.Signer, hkc gossh.HostKeyCallback) (*gossh.Client, error) {
			hostKeyCallback = hkc
			return nil, nil
		})
	s.clock.EXPECT().Now().AnyTimes().Return(now)

	tunnelReqArgs := RequestArgs{
		MachineID: "0",
		ModelUUID: "model-uuid",
	}

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx := context.Background()
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		_, err := tunnelTracker.RequestTunnel(ctx, tunnelReqArgs)
		c.Check(err, jc.ErrorIsNil)
	}()

	// wait for the tunnel request to be processed
	select {
	case <-tunnelRequested:
	case <-time.After(1 * time.Second):
		c.Error("timeout waiting for tunnel request to be processed")
	}

	var tunnels []string
	for uuid := range tunnelTracker.tracker {
		tunnels = append(tunnels, uuid)
	}
	c.Check(tunnels, gc.HasLen, 1)

	tunnelID, err := tunnelTracker.AuthenticateTunnel(ReverseTunnelUser, sshConnArgs.Password)
	c.Check(err, jc.ErrorIsNil)
	c.Check(tunnelID, gc.Equals, tunnels[0])

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	err = tunnelTracker.PushTunnel(ctx, tunnelID, nil)
	c.Check(err, jc.ErrorIsNil)

	wg.Wait()

	// Check that the host key callback correctly validates the machine's public host key
	c.Check(hostKeyCallback("", nil, sshPublicHostKey), jc.ErrorIsNil)

	c.Check(tunnelTracker.tracker, gc.HasLen, 0)
}

type mockConn struct {
	net.Conn
	atomic.Bool
}

func (m *mockConn) Close() error {
	m.Bool.Store(true)
	return nil
}

func (s *sshTunnelerSuite) TestTunnelIsClosedWhenDialFails(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTracker(c)

	sshConnArgs := state.SSHConnRequestArg{}

	// use a channel to wait for the tunnel request to be processed
	tunnelRequested := make(chan struct{})

	now := time.Now()

	s.controller.EXPECT().Addresses().Return([]network.SpaceAddress{
		{MachineAddress: network.NewMachineAddress("1.2.3.4")},
	}, nil)
	s.state.EXPECT().InsertSSHConnRequest(gomock.Any()).DoAndReturn(
		func(sra state.SSHConnRequestArg) error {
			sshConnArgs = sra
			close(tunnelRequested)
			return nil
		},
	)
	s.state.EXPECT().MachineHostKeys(gomock.Any(), gomock.Any()).Return([]string{}, nil)
	s.dialer.EXPECT().Dial(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("failed-to-connect"))
	s.clock.EXPECT().Now().AnyTimes().Return(now)

	tunnelReqArgs := RequestArgs{
		MachineID: "0",
		ModelUUID: "model-uuid",
	}

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx := context.Background()
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		_, err := tunnelTracker.RequestTunnel(ctx, tunnelReqArgs)
		c.Check(err, gc.ErrorMatches, `failed-to-connect`)
	}()

	// wait for the tunnel request to be processed
	select {
	case <-tunnelRequested:
	case <-time.After(1 * time.Second):
		c.Error("timeout waiting for tunnel request to be processed")
	}

	tunnelID, err := tunnelTracker.AuthenticateTunnel(ReverseTunnelUser, sshConnArgs.Password)
	c.Check(err, jc.ErrorIsNil)

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	mockConn := &mockConn{}
	err = tunnelTracker.PushTunnel(ctx, tunnelID, mockConn)
	c.Check(err, jc.ErrorIsNil)

	wg.Wait()

	c.Check(tunnelTracker.tracker, gc.HasLen, 0)
	c.Check(mockConn.Bool.Load(), gc.Equals, true)
}

func (s *sshTunnelerSuite) TestGenerateEphemeralSSHKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTracker(c)

	privateKey, publicKey, err := tunnelTracker.generateEphemeralSSHKey()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(privateKey, gc.Not(gc.IsNil))
	c.Assert(publicKey, gc.Not(gc.IsNil))
}

func (s *sshTunnelerSuite) TestAuthenticateTunnel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTracker(c)

	now := time.Now()
	deadline := now.Add(1 * time.Second)

	tunnelID := "test-tunnel-id"
	token, err := tunnelTracker.authn.generatePassword(tunnelID, now, deadline)
	c.Assert(err, jc.ErrorIsNil)

	s.clock.EXPECT().Now().AnyTimes().Return(now)
	authTunnelID, err := tunnelTracker.AuthenticateTunnel(ReverseTunnelUser, token)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(authTunnelID, gc.Equals, tunnelID)
}

func (s *sshTunnelerSuite) TestPushTunnel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTracker(c)

	tunnelID := "test-tunnel-id"
	recv := make(chan net.Conn)
	tunnelTracker.tracker[tunnelID] = recv

	conn := &net.TCPConn{}

	go func() {
		select {
		case receivedConn := <-recv:
			c.Check(receivedConn, gc.Equals, conn)
		case <-time.After(1 * time.Second):
			c.Error("timeout waiting for tunnel")
		}
	}()

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	err := tunnelTracker.PushTunnel(ctx, tunnelID, conn)
	c.Check(err, jc.ErrorIsNil)

}

func (s *sshTunnelerSuite) TestDeleteTunnel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTracker(c)

	tunnelID := "test-tunnel-id"
	tunnelTracker.tracker[tunnelID] = nil

	tunnelTracker.delete(tunnelID)
	_, ok := tunnelTracker.tracker[tunnelID]
	c.Assert(ok, gc.Equals, false)
}

func (s *sshTunnelerSuite) TestAuthenticateTunnelInvalidUsername(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTracker(c)

	_, err := tunnelTracker.AuthenticateTunnel("invalid-username", "some-password")
	c.Assert(err, gc.ErrorMatches, "invalid username")
}

func (s *sshTunnelerSuite) TestPushTunnelInvalidTunnelID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTracker(c)

	err := tunnelTracker.PushTunnel(context.Background(), "invalid-tunnel-id", nil)
	c.Assert(err, gc.ErrorMatches, "tunnel not found")
}

func (s *sshTunnelerSuite) TestRequestTunnelTimeout(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTracker(c)

	now := time.Now()
	s.clock.EXPECT().Now().Times(1).Return(now)
	s.controller.EXPECT().Addresses().Return([]network.SpaceAddress{
		{MachineAddress: network.NewMachineAddress("1.2.3.4")},
	}, nil)
	s.state.EXPECT().InsertSSHConnRequest(gomock.Any()).Return(nil)
	s.state.EXPECT().MachineHostKeys(gomock.Any(), gomock.Any()).Return([]string{}, nil)

	tunnelReqArgs := RequestArgs{
		MachineID: "0",
		ModelUUID: "model-uuid",
	}

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
	defer cancel()

	_, err := tunnelTracker.RequestTunnel(ctx, tunnelReqArgs)
	c.Assert(err, gc.ErrorMatches, "waiting for tunnel: context deadline exceeded")
}

func (s *sshTunnelerSuite) TestRequestTunnelDeadline(c *gc.C) {
	defer s.setupMocks(c).Finish()

	restore := testing.PatchValue(&maxTimeout, 1*time.Millisecond)
	defer restore()

	tunnelTracker := s.newTracker(c)

	now := time.Now()
	s.clock.EXPECT().Now().Times(1).Return(now)
	s.controller.EXPECT().Addresses().Return([]network.SpaceAddress{
		{MachineAddress: network.NewMachineAddress("1.2.3.4")},
	}, nil)
	s.state.EXPECT().InsertSSHConnRequest(gomock.Any()).Return(nil)
	s.state.EXPECT().MachineHostKeys(gomock.Any(), gomock.Any()).Return([]string{}, nil)

	tunnelReqArgs := RequestArgs{
		MachineID: "0",
		ModelUUID: "model-uuid",
	}

	_, err := tunnelTracker.RequestTunnel(context.Background(), tunnelReqArgs)
	c.Assert(err, gc.ErrorMatches, "waiting for tunnel: context deadline exceeded")
}

func (s *sshTunnelerSuite) TestPushTunnelTimeout(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTracker(c)

	tunnelID := "test-tunnel-id"
	recv := make(chan net.Conn)
	tunnelTracker.tracker[tunnelID] = recv

	conn := &net.TCPConn{}

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
	defer cancel()

	err := tunnelTracker.PushTunnel(ctx, tunnelID, conn)
	c.Check(err, gc.ErrorMatches, `no one waiting for tunnel: context deadline exceeded`)
}

func (s *sshTunnelerSuite) TestInvalidMachineHostKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTracker(c)

	now := time.Now()
	s.clock.EXPECT().Now().Times(1).Return(now)
	s.controller.EXPECT().Addresses().Return([]network.SpaceAddress{
		{MachineAddress: network.NewMachineAddress("1.2.3.4")},
	}, nil)
	s.state.EXPECT().MachineHostKeys(gomock.Any(), gomock.Any()).Return([]string{"fake-host-key"}, nil)

	tunnelReqArgs := RequestArgs{
		MachineID: "0",
		ModelUUID: "model-uuid",
	}

	_, err := tunnelTracker.RequestTunnel(context.Background(), tunnelReqArgs)
	c.Assert(err, gc.ErrorMatches, "failed to parse machine host key: ssh: no key found")
}

func (s *sshTunnelerSuite) TestNewTunnelTrackerValidation(c *gc.C) {
	// Test case: All arguments are valid
	args := TrackerArgs{
		State:          s.state,
		ControllerInfo: s.controller,
		Dialer:         s.dialer,
		Clock:          s.clock,
	}
	tunnelTracker, err := NewTracker(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tunnelTracker, gc.Not(gc.IsNil))

	// Test case: Missing State
	args.State = nil
	tunnelTracker, err = NewTracker(args)
	c.Assert(err, gc.ErrorMatches, "state is required")
	c.Assert(tunnelTracker, gc.IsNil)

	// Test case: Missing ControllerInfo
	args.State = s.state
	args.ControllerInfo = nil
	tunnelTracker, err = NewTracker(args)
	c.Assert(err, gc.ErrorMatches, "controller info is required")
	c.Assert(tunnelTracker, gc.IsNil)

	// Test case: Missing Dialer
	args.ControllerInfo = s.controller
	args.Dialer = nil
	tunnelTracker, err = NewTracker(args)
	c.Assert(err, gc.ErrorMatches, "dialer is required")
	c.Assert(tunnelTracker, gc.IsNil)

	// Test case: Missing Clock
	args.Dialer = s.dialer
	args.Clock = nil
	tunnelTracker, err = NewTracker(args)
	c.Assert(err, gc.ErrorMatches, "clock is required")
	c.Assert(tunnelTracker, gc.IsNil)
}
