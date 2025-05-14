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

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"
	gossh "golang.org/x/crypto/ssh"

	network "github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/pki/test"
	"github.com/juju/juju/internal/testhelpers"
)

type sshTunnelerSuite struct {
	state      *MockState
	controller *MockControllerInfo
	dialer     *MockSSHDial
	clock      *MockClock
}

var _ = tc.Suite(&sshTunnelerSuite{})

func (s *sshTunnelerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.controller = NewMockControllerInfo(ctrl)
	s.dialer = NewMockSSHDial(ctrl)
	s.clock = NewMockClock(ctrl)

	return ctrl
}

func (s *sshTunnelerSuite) newTracker(c *tc.C) *Tracker {
	args := TrackerArgs{
		State:          s.state,
		ControllerInfo: s.controller,
		Dialer:         s.dialer,
		Clock:          s.clock,
	}
	tunnelTracker, err := NewTracker(args)
	c.Assert(err, tc.ErrorIsNil)
	return tunnelTracker
}

func (s *sshTunnelerSuite) TestTunneler(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTracker(c)

	sshConnArgs := sshRequestArgs{}

	// use a channel to wait for the tunnel request to be processed
	tunnelRequested := make(chan struct{})

	now := time.Now()

	machineHostKey, err := test.InsecureKeyProfile()
	c.Assert(err, tc.ErrorIsNil)
	sshPublicHostKey, err := gossh.NewPublicKey(machineHostKey.Public())
	c.Assert(err, tc.ErrorIsNil)

	var hostKeyCallback gossh.HostKeyCallback

	s.controller.EXPECT().Addresses().Return([]network.SpaceAddress{
		{MachineAddress: network.NewMachineAddress("1.2.3.4")},
	}, nil)
	s.state.EXPECT().InsertSSHConnRequest(gomock.Any()).DoAndReturn(
		func(sra sshRequestArgs) error {
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
		ctx := c.Context()
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		_, err := tunnelTracker.RequestTunnel(ctx, tunnelReqArgs)
		c.Check(err, tc.ErrorIsNil)
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
	c.Check(tunnels, tc.HasLen, 1)

	tunnelID, err := tunnelTracker.AuthenticateTunnel(reverseTunnelUser, sshConnArgs.Password)
	c.Check(err, tc.ErrorIsNil)
	c.Check(tunnelID, tc.Equals, tunnels[0])

	ctx := c.Context()
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	err = tunnelTracker.PushTunnel(ctx, tunnelID, nil)
	c.Check(err, tc.ErrorIsNil)

	wg.Wait()

	// Check that the host key callback correctly validates the machine's public host key
	c.Check(hostKeyCallback("", nil, sshPublicHostKey), tc.ErrorIsNil)

	c.Check(tunnelTracker.tracker, tc.HasLen, 0)
}

type mockConn struct {
	net.Conn
	atomic.Bool
}

func (m *mockConn) Close() error {
	m.Bool.Store(true)
	return nil
}

func (s *sshTunnelerSuite) TestTunnelIsClosedWhenDialFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTracker(c)

	sshConnArgs := sshRequestArgs{}

	// use a channel to wait for the tunnel request to be processed
	tunnelRequested := make(chan struct{})

	now := time.Now()

	s.controller.EXPECT().Addresses().Return([]network.SpaceAddress{
		{MachineAddress: network.NewMachineAddress("1.2.3.4")},
	}, nil)
	s.state.EXPECT().InsertSSHConnRequest(gomock.Any()).DoAndReturn(
		func(sra sshRequestArgs) error {
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
		ctx := c.Context()
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		_, err := tunnelTracker.RequestTunnel(ctx, tunnelReqArgs)
		c.Check(err, tc.ErrorMatches, `failed-to-connect`)
	}()

	// wait for the tunnel request to be processed
	select {
	case <-tunnelRequested:
	case <-time.After(1 * time.Second):
		c.Error("timeout waiting for tunnel request to be processed")
	}

	tunnelID, err := tunnelTracker.AuthenticateTunnel(reverseTunnelUser, sshConnArgs.Password)
	c.Check(err, tc.ErrorIsNil)

	ctx := c.Context()
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	mockConn := &mockConn{}
	err = tunnelTracker.PushTunnel(ctx, tunnelID, mockConn)
	c.Check(err, tc.ErrorIsNil)

	wg.Wait()

	c.Check(tunnelTracker.tracker, tc.HasLen, 0)
	c.Check(mockConn.Bool.Load(), tc.Equals, true)
}

func (s *sshTunnelerSuite) TestGenerateEphemeralSSHKey(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTracker(c)

	privateKey, publicKey, err := tunnelTracker.generateEphemeralSSHKey()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(privateKey, tc.Not(tc.IsNil))
	c.Assert(publicKey, tc.Not(tc.IsNil))
}

func (s *sshTunnelerSuite) TestAuthenticateTunnel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTracker(c)

	now := time.Now()
	deadline := now.Add(1 * time.Second)

	tunnelID := "test-tunnel-id"
	token, err := tunnelTracker.authn.generatePassword(tunnelID, now, deadline)
	c.Assert(err, tc.ErrorIsNil)

	s.clock.EXPECT().Now().AnyTimes().Return(now)
	authTunnelID, err := tunnelTracker.AuthenticateTunnel(reverseTunnelUser, token)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(authTunnelID, tc.Equals, tunnelID)
}

func (s *sshTunnelerSuite) TestPushTunnel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTracker(c)

	tunnelID := "test-tunnel-id"
	recv := make(chan net.Conn)
	tunnelTracker.tracker[tunnelID] = recv

	conn := &net.TCPConn{}

	go func() {
		select {
		case receivedConn := <-recv:
			c.Check(receivedConn, tc.Equals, conn)
		case <-time.After(1 * time.Second):
			c.Error("timeout waiting for tunnel")
		}
	}()

	ctx := c.Context()
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	err := tunnelTracker.PushTunnel(ctx, tunnelID, conn)
	c.Check(err, tc.ErrorIsNil)

}

func (s *sshTunnelerSuite) TestDeleteTunnel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTracker(c)

	tunnelID := "test-tunnel-id"
	tunnelTracker.tracker[tunnelID] = nil

	tunnelTracker.delete(tunnelID)
	_, ok := tunnelTracker.tracker[tunnelID]
	c.Assert(ok, tc.Equals, false)
}

func (s *sshTunnelerSuite) TestAuthenticateTunnelInvalidUsername(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTracker(c)

	_, err := tunnelTracker.AuthenticateTunnel("invalid-username", "some-password")
	c.Assert(err, tc.ErrorMatches, "invalid username")
}

func (s *sshTunnelerSuite) TestPushTunnelInvalidTunnelID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTracker(c)

	err := tunnelTracker.PushTunnel(c.Context(), "invalid-tunnel-id", nil)
	c.Assert(err, tc.ErrorMatches, "tunnel not found")
}

func (s *sshTunnelerSuite) TestRequestTunnelTimeout(c *tc.C) {
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

	ctx := c.Context()
	ctx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
	defer cancel()

	_, err := tunnelTracker.RequestTunnel(ctx, tunnelReqArgs)
	c.Assert(err, tc.ErrorMatches, "waiting for tunnel: context deadline exceeded")
}

func (s *sshTunnelerSuite) TestRequestTunnelDeadline(c *tc.C) {
	defer s.setupMocks(c).Finish()

	restore := testhelpers.PatchValue(&maxTimeout, 1*time.Millisecond)
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

	_, err := tunnelTracker.RequestTunnel(c.Context(), tunnelReqArgs)
	c.Assert(err, tc.ErrorMatches, "waiting for tunnel: context deadline exceeded")
}

func (s *sshTunnelerSuite) TestPushTunnelTimeout(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTracker(c)

	tunnelID := "test-tunnel-id"
	recv := make(chan net.Conn)
	tunnelTracker.tracker[tunnelID] = recv

	conn := &net.TCPConn{}

	ctx := c.Context()
	ctx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
	defer cancel()

	err := tunnelTracker.PushTunnel(ctx, tunnelID, conn)
	c.Check(err, tc.ErrorMatches, `no one waiting for tunnel: context deadline exceeded`)
}

func (s *sshTunnelerSuite) TestInvalidMachineHostKey(c *tc.C) {
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

	_, err := tunnelTracker.RequestTunnel(c.Context(), tunnelReqArgs)
	c.Assert(err, tc.ErrorMatches, "failed to parse machine host key: ssh: no key found")
}

func (s *sshTunnelerSuite) TestNewTunnelTrackerValidation(c *tc.C) {
	// Test case: All arguments are valid
	args := TrackerArgs{
		State:          s.state,
		ControllerInfo: s.controller,
		Dialer:         s.dialer,
		Clock:          s.clock,
	}
	tunnelTracker, err := NewTracker(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tunnelTracker, tc.Not(tc.IsNil))

	// Test case: Missing State
	args.State = nil
	tunnelTracker, err = NewTracker(args)
	c.Assert(err, tc.ErrorMatches, "state is required")
	c.Assert(tunnelTracker, tc.IsNil)

	// Test case: Missing ControllerInfo
	args.State = s.state
	args.ControllerInfo = nil
	tunnelTracker, err = NewTracker(args)
	c.Assert(err, tc.ErrorMatches, "controller info is required")
	c.Assert(tunnelTracker, tc.IsNil)

	// Test case: Missing Dialer
	args.ControllerInfo = s.controller
	args.Dialer = nil
	tunnelTracker, err = NewTracker(args)
	c.Assert(err, tc.ErrorMatches, "dialer is required")
	c.Assert(tunnelTracker, tc.IsNil)

	// Test case: Missing Clock
	args.Dialer = s.dialer
	args.Clock = nil
	tunnelTracker, err = NewTracker(args)
	c.Assert(err, tc.ErrorMatches, "clock is required")
	c.Assert(tunnelTracker, tc.IsNil)
}
