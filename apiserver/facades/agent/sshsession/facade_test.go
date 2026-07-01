// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

import (
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/watchertest"
	domainssh "github.com/juju/juju/domain/ssh"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type facadeSuite struct {
	testhelpers.IsolationSuite

	service         *MockSSHConnRequestService
	controllerCfg   *MockControllerConfigService
	hostKeyService  *MockControllerSSHHostKeyService
	watcherRegistry *MockWatcherRegistry
}

func TestFacadeSuite(t *testing.T) {
	tc.Run(t, &facadeSuite{})
}

func (s *facadeSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.service = NewMockSSHConnRequestService(ctrl)
	s.controllerCfg = NewMockControllerConfigService(ctrl)
	s.hostKeyService = NewMockControllerSSHHostKeyService(ctrl)
	s.watcherRegistry = NewMockWatcherRegistry(ctrl)
	return ctrl
}

func (s *facadeSuite) newFacade() *Facade {
	return newFacade(s.service, s.controllerCfg, s.hostKeyService, s.watcherRegistry)
}

func (s *facadeSuite) TestWatchSSHConnRequest(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	changes := make(chan []string, 1)
	changes <- []string{"tunnel-0", "tunnel-1"}
	w := watchertest.NewMockStringsWatcher(changes)
	defer w.Kill()

	s.service.EXPECT().WatchSSHConnRequest(gomock.Any()).Return(w, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("watcher-id", nil)

	result, err := s.newFacade().WatchSSHConnRequest(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.StringsWatcherId, tc.Equals, "watcher-id")
	c.Check(result.Changes, tc.DeepEquals, []string{"tunnel-0", "tunnel-1"})
}

func (s *facadeSuite) TestGetSSHConnRequest(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := domainssh.SSHConnRequest{
		TunnelID:            "tunnel-0",
		MachineName:         "0",
		SSHUsername:         "juju-reverse-tunnel",
		SSHPassword:         "jwt",
		ControllerAddresses: network.NewSpaceAddresses("10.0.0.1", "10.0.0.2"),
		UnitPort:            22,
		EphemeralPublicKey:  []byte("eph-pub"),
	}
	s.service.EXPECT().GetSSHConnRequest(gomock.Any(), "tunnel-0").Return(req, nil)

	result, err := s.newFacade().GetSSHConnRequest(c.Context(), params.SSHConnRequestArg{TunnelID: "tunnel-0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.MachineName, tc.Equals, "0")
	c.Check(result.ControllerAddresses, tc.DeepEquals, []string{"10.0.0.1", "10.0.0.2"})
	c.Check(result.Username, tc.Equals, "juju-reverse-tunnel")
	c.Check(result.Password, tc.Equals, "jwt")
	c.Check(result.UnitPort, tc.Equals, 22)
	c.Check(result.EphemeralPublicKey, tc.DeepEquals, []byte("eph-pub"))
}

func (s *facadeSuite) TestControllerSSHPort(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.controllerCfg.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{
		controller.SSHServerPort: 2223,
	}, nil)

	result, err := s.newFacade().ControllerSSHPort(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Port, tc.Equals, 2223)
}

func (s *facadeSuite) TestControllerPublicKey(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.hostKeyService.EXPECT().SSHServerHostKey(gomock.Any()).Return(jujutesting.SSHServerHostKey, nil)

	result, err := s.newFacade().ControllerPublicKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// The returned key must be the public part of the controller host key.
	signer, err := gossh.ParsePrivateKey([]byte(jujutesting.SSHServerHostKey))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.PublicKey, tc.DeepEquals, signer.PublicKey().Marshal())

	_, err = gossh.ParsePublicKey(result.PublicKey)
	c.Assert(err, tc.ErrorIsNil)
}
