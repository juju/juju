// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

import (
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gossh "golang.org/x/crypto/ssh"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/watchertest"
	domainssh "github.com/juju/juju/domain/ssh"
	"github.com/juju/juju/internal/errors"
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
	// The facade always derives the machine from the authenticated tag, so the
	// tests authenticate as machine "0".
	authorizer := apiservertesting.FakeAuthorizer{Tag: names.NewMachineTag("0")}
	return newFacade(authorizer, s.service, s.controllerCfg, s.hostKeyService, s.watcherRegistry)
}

func (s *facadeSuite) TestWatchSSHConnRequest(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	changes := make(chan []string, 1)
	changes <- []string{"tunnel-0", "tunnel-1"}
	w := watchertest.NewMockStringsWatcher(changes)
	defer w.Kill()

	s.service.EXPECT().WatchSSHConnRequest(gomock.Any(), coremachine.Name("0")).Return(w, nil)
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
	// The machine name is derived from authentication ("0") and passed to the
	// service, so scoping happens in state rather than after fetching.
	s.service.EXPECT().GetSSHConnRequest(gomock.Any(), coremachine.Name("0"), "tunnel-0").Return(req, nil)

	result, err := s.newFacade().GetSSHConnRequest(c.Context(), params.SSHConnRequestArg{TunnelID: "tunnel-0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.MachineName, tc.Equals, "0")
	c.Check(result.ControllerAddresses, tc.DeepEquals, []string{"10.0.0.1", "10.0.0.2"})
	c.Check(result.Username, tc.Equals, "juju-reverse-tunnel")
	c.Check(result.Password, tc.Equals, "jwt")
	c.Check(result.UnitPort, tc.Equals, 22)
	c.Check(result.EphemeralPublicKey, tc.DeepEquals, []byte("eph-pub"))
}

// TestGetSSHConnRequestScopedToMachine verifies the facade passes the
// authenticated machine name ("0") down to the service, so scoping is enforced
// in state up front. A request targeting another machine is reported by state
// as not found rather than being fetched and rejected afterwards, so its
// credentials are never returned.
func (s *facadeSuite) TestGetSSHConnRequestScopedToMachine(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.service.EXPECT().
		GetSSHConnRequest(gomock.Any(), coremachine.Name("0"), "tunnel-1").
		Return(domainssh.SSHConnRequest{}, errors.Errorf("not found").Add(coreerrors.NotFound))

	_, err := s.newFacade().GetSSHConnRequest(c.Context(), params.SSHConnRequestArg{TunnelID: "tunnel-1"})
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

// TestWatchSSHConnRequestNonMachineDenied verifies the watcher rejects a caller
// whose auth tag is not a machine tag, since the machine identity must come
// from authentication.
func (s *facadeSuite) TestWatchSSHConnRequestNonMachineDenied(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	authorizer := apiservertesting.FakeAuthorizer{Tag: names.NewUnitTag("app/0")}
	facade := newFacade(authorizer, s.service, s.controllerCfg, s.hostKeyService, s.watcherRegistry)

	_, err := facade.WatchSSHConnRequest(c.Context())
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *facadeSuite) TestControllerSSHPort(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.controllerCfg.EXPECT().GetSSHServerPort(gomock.Any()).Return(2223, nil)

	result, err := s.newFacade().ControllerSSHPort(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Port, tc.Equals, 2223)
}

func (s *facadeSuite) TestControllerPublicKey(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// The service derives and caches the public key; the facade simply returns
	// what it is given. Derive the expected bytes from the known private key.
	signer, err := gossh.ParsePrivateKey([]byte(jujutesting.SSHServerHostKey))
	c.Assert(err, tc.ErrorIsNil)
	publicKey := signer.PublicKey().Marshal()

	s.hostKeyService.EXPECT().SSHServerHostPublicKey(gomock.Any()).Return(publicKey, nil)

	result, err := s.newFacade().ControllerPublicKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.PublicKey, tc.DeepEquals, publicKey)

	_, err = gossh.ParsePublicKey(result.PublicKey)
	c.Assert(err, tc.ErrorIsNil)
}
