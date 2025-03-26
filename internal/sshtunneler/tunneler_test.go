// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"context"
	"net"
	"sync"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/lestrrat-go/jwx/v2/jwa"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	network "github.com/juju/juju/core/network"
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

func (s *sshTunnelerSuite) newTunnelTracker(c *gc.C) *TunnelTracker {
	args := TunnelTrackerArgs{
		State:          s.state,
		ControllerInfo: s.controller,
		Dialer:         s.dialer,
		Clock:          s.clock,
		SharedSecret:   []byte("test-secret"),
		JWTAlg:         jwa.HS256,
	}
	tunnelTracker, err := NewTunnelTracker(args)
	c.Assert(err, jc.ErrorIsNil)
	return tunnelTracker
}

func (s *sshTunnelerSuite) TestTunneler(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTunnelTracker(c)

	sshConnArgs := state.SSHConnRequestArg{}

	now := time.Now()

	s.controller.EXPECT().Addresses().Return([]network.SpaceAddress{
		{MachineAddress: network.NewMachineAddress("1.2.3.4")},
	})
	s.state.EXPECT().InsertSSHConnRequest(gomock.Any()).DoAndReturn(
		func(sra state.SSHConnRequestArg) error {
			sshConnArgs = sra
			return nil
		},
	)
	s.dialer.EXPECT().Dial(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)
	s.clock.EXPECT().Now().AnyTimes().Return(now)

	tunnelReqArgs := RequestArgs{
		MachineID: "0",
		ModelUUID: "model-uuid",
	}

	req, err := tunnelTracker.RequestTunnel(tunnelReqArgs)
	c.Assert(err, jc.ErrorIsNil)

	var tunnels []string
	for uuid := range tunnelTracker.tracker {
		tunnels = append(tunnels, uuid)
	}
	c.Assert(tunnels, gc.HasLen, 1)

	tID, err := tunnelTracker.AuthenticateTunnel("reverse-tunnel", sshConnArgs.Password)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tID, gc.Equals, tunnels[0])

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := tunnelTracker.PushTunnel(context.Background(), tID, nil)
		c.Check(err, jc.ErrorIsNil)
	}()

	_, err = req.Wait(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	wg.Wait()

	c.Assert(tunnelTracker.tracker, gc.HasLen, 0)
}

func (s *sshTunnelerSuite) TestGenerateEphemeralSSHKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTunnelTracker(c)

	privateKey, publicKey, err := tunnelTracker.generateEphemeralSSHKey()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(privateKey, gc.Not(gc.IsNil))
	c.Assert(publicKey, gc.Not(gc.IsNil))
}

func (s *sshTunnelerSuite) TestAuthenticateTunnel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTunnelTracker(c)

	now := time.Now()
	s.clock.EXPECT().Now().AnyTimes().Return(now)

	expiry := now.Add(maxTimeout)
	tunnelID := "test-tunnel-id"
	token, err := tunnelTracker.authn.generatePassword(tunnelID, expiry)
	c.Assert(err, jc.ErrorIsNil)

	authTunnelID, err := tunnelTracker.AuthenticateTunnel(reverseTunnelUser, token)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(authTunnelID, gc.Equals, tunnelID)
}

func (s *sshTunnelerSuite) TestPushTunnel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTunnelTracker(c)

	tunnelID := "test-tunnel-id"
	tunnelReq := &tunnelRequest{
		recv: make(chan net.Conn),
	}
	tunnelTracker.tracker[tunnelID] = tunnelReq

	conn := &net.TCPConn{}

	go func() {
		select {
		case receivedConn := <-tunnelReq.recv:
			c.Check(receivedConn, gc.Equals, conn)
		case <-time.After(1 * time.Second):
			c.Fatal("timeout waiting for tunnel")
		}
	}()

	err := tunnelTracker.PushTunnel(context.Background(), tunnelID, conn)
	c.Check(err, jc.ErrorIsNil)

}

func (s *sshTunnelerSuite) TestDeleteTunnel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTunnelTracker(c)

	tunnelID := "test-tunnel-id"
	tunnelReq := &tunnelRequest{}
	tunnelTracker.tracker[tunnelID] = tunnelReq

	tunnelTracker.delete(tunnelID)
	_, ok := tunnelTracker.tracker[tunnelID]
	c.Assert(ok, gc.Equals, false)
}

func (s *sshTunnelerSuite) TestRequestTunnel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTunnelTracker(c)

	now := time.Now()
	s.clock.EXPECT().Now().AnyTimes().Return(now)
	s.controller.EXPECT().Addresses().Return([]network.SpaceAddress{
		{MachineAddress: network.NewMachineAddress("1.2.3.4")},
	})
	s.state.EXPECT().InsertSSHConnRequest(gomock.Any()).Return(nil)

	tunnelReqArgs := RequestArgs{
		MachineID: "0",
		ModelUUID: "model-uuid",
	}

	req, err := tunnelTracker.RequestTunnel(tunnelReqArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(req, gc.Not(gc.IsNil))
	c.Check(req.privateKey, gc.Not(gc.IsNil))
}

func (s *sshTunnelerSuite) TestAuthenticateTunnelInvalidUsername(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTunnelTracker(c)

	_, err := tunnelTracker.AuthenticateTunnel("invalid-username", "some-password")
	c.Assert(err, gc.ErrorMatches, "invalid username")
}

func (s *sshTunnelerSuite) TestPushTunnelInvalidTunnelID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker := s.newTunnelTracker(c)

	err := tunnelTracker.PushTunnel(context.Background(), "invalid-tunnel-id", nil)
	c.Assert(err, gc.ErrorMatches, "tunnel not found")
}

func (s *sshTunnelerSuite) TestWaitTimeout(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelReq := &tunnelRequest{
		recv:    make(chan net.Conn),
		cleanup: func() {},
	}

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
	defer cancel()
	_, err := tunnelReq.Wait(ctx)
	c.Assert(err, gc.ErrorMatches, "waiting for tunnel: context deadline exceeded")
}
