// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/domain/ssh"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type serviceSuite struct {
	testhelpers.IsolationSuite

	state          *MockState
	watcherFactory *MockWatcherFactory
	stringsWatcher *MockStringsWatcher
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestInsertSSHConnRequest(c *tc.C) {
	defer s.setupMocks(c).Finish()

	req := ssh.SSHConnRequest{
		TunnelID:           "tunnel-0",
		MachineID:          "0",
		Expires:            time.Now().UTC().Add(time.Minute),
		Username:           "juju-reverse-tunnel",
		Password:           "secret",
		UnitPort:           0,
		EphemeralPublicKey: []byte("pub"),
	}

	s.state.EXPECT().InsertSSHConnRequest(gomock.Any(), req, gomock.Any()).Return(nil)

	err := NewService(s.state, s.watcherFactory, clock.WallClock).InsertSSHConnRequest(c.Context(), req)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestInsertSSHConnRequestInvalidMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	req := ssh.SSHConnRequest{
		TunnelID:  "tunnel-0",
		MachineID: "invalid",
		Expires:   time.Now().UTC().Add(time.Minute),
		Username:  "juju-reverse-tunnel",
		Password:  "secret",
	}

	err := NewService(s.state, s.watcherFactory, clock.WallClock).InsertSSHConnRequest(c.Context(), req)
	c.Assert(err, tc.ErrorMatches, `validating ssh connection request: validating machine id "invalid": machine name`)
}

func (s *serviceSuite) TestGetSSHConnRequestEmptyTunnelID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.watcherFactory, clock.WallClock).GetSSHConnRequest(c.Context(), "")
	c.Assert(err, tc.ErrorMatches, `empty tunnel id`)
}

func (s *serviceSuite) TestWatchSSHConnRequest(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().PruneExpiredSSHConnRequests(gomock.Any(), gomock.Any()).Return(nil)
	s.state.EXPECT().InitialWatchSSHConnRequestsStatement().Return("ssh_connection_request", "SELECT tunnel_id FROM ssh_connection_request")
	s.watcherFactory.EXPECT().NewNamespaceWatcher(gomock.Any(), gomock.Any(), "ssh connection request watcher", gomock.Any()).Return(s.stringsWatcher, nil)

	w, err := NewService(s.state, s.watcherFactory, clock.WallClock).WatchSSHConnRequest(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.Equals, s.stringsWatcher)
}

func (s *serviceSuite) TestWatchSSHConnRequestPruneError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().PruneExpiredSSHConnRequests(gomock.Any(), gomock.Any()).Return(errors.New("boom"))

	_, err := NewService(s.state, s.watcherFactory, clock.WallClock).WatchSSHConnRequest(c.Context())
	c.Assert(err, tc.ErrorMatches, `pruning expired ssh connection requests: boom`)
}

func (s *serviceSuite) TestRemoveSSHConnRequestEmptyTunnelID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.state, s.watcherFactory, clock.WallClock).RemoveSSHConnRequest(c.Context(), "")
	c.Assert(err, tc.ErrorMatches, `empty tunnel id`)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)
	s.stringsWatcher = NewMockStringsWatcher(ctrl)

	return ctrl
}
