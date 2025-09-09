// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/machine/internal"
	"github.com/juju/juju/internal/uuid"
)

type lxdProfileStateSuite struct {
	baseSuite
}

func TestLXDProfileStateSuite(t *stdtesting.T) {
	tc.Run(t, &lxdProfileStateSuite{})
}

func (s *lxdProfileStateSuite) TestGetLXDProfilesForMachine(c *tc.C) {
	// Arrange
	// 2 applications with unit's on a single machine.
	// Only one charm has an LXD Profile
	netNodeUUID := s.addNetNode(c)
	machineName := "42"
	s.addMachine(c, machineName, netNodeUUID)

	profileText := []byte{'H', 'e', 'l', 'l', 'o'}
	charmUUIDOne := s.addCharmWithProfile(c, "testing-profile", 7, profileText)
	appName := "purple"
	appUUIDOne := s.addApplication(c, appName, charmUUIDOne)
	s.addUnit(c, appUUIDOne, charmUUIDOne, netNodeUUID)

	charmUUIDTwo := s.addCharm(c, "testing", 36)
	appUUIDTWO := s.addApplication(c, "green", charmUUIDTwo)
	s.addUnit(c, appUUIDTWO, charmUUIDTwo, netNodeUUID)

	// Act
	obtained, err := s.state.GetLXDProfilesForMachine(c.Context(), machineName)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(obtained, tc.DeepEquals, []internal.CreateLXDProfileDetails{
		{
			ApplicationName: appName,
			CharmRevision:   7,
			LXDProfile:      profileText,
		},
	})
}

func (s *lxdProfileStateSuite) TestGetLXDProfilesForMachineNoProfiles(c *tc.C) {
	// Arrange
	// 2 applications with unit's on a single machine.
	// No charms have an LXD Profile
	netNodeUUID := s.addNetNode(c)
	machineName := "42"
	s.addMachine(c, machineName, netNodeUUID)

	charmUUIDOne := s.addCharm(c, "testing-profile", 7)
	appUUIDOne := s.addApplication(c, "purple", charmUUIDOne)
	s.addUnit(c, appUUIDOne, charmUUIDOne, netNodeUUID)

	charmUUIDTWO := s.addCharm(c, "testing", 36)
	appUUIDTWO := s.addApplication(c, "green", charmUUIDOne)
	s.addUnit(c, appUUIDTWO, charmUUIDTWO, netNodeUUID)

	// Act
	obtained, err := s.state.GetLXDProfilesForMachine(c.Context(), machineName)

	// Assert
	// No failure and no return values.
	c.Assert(err, tc.IsNil)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *lxdProfileStateSuite) TestGetLXDProfilesForMachineError(c *tc.C) {}

func (s *lxdProfileStateSuite) addApplication(c *tc.C, appName, charmUUID string) string {
	appUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?,?,?,?,?)`,
		appUUID, appName, life.Alive, charmUUID, network.AlphaSpaceId)
	return appUUID
}

func (s *lxdProfileStateSuite) addCharmWithProfile(c *tc.C, charmName string, revision int, profile []byte) string {
	charmUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO charm (uuid, reference_name, revision, lxd_profile) VALUES (?,?,?,?)`,
		charmUUID, charmName, revision, profile)
	return charmUUID
}

func (s *lxdProfileStateSuite) addCharm(c *tc.C, charmName string, revision int) string {
	charmUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO charm (uuid, reference_name, revision) VALUES (?,?,?)`,
		charmUUID, charmName, revision)
	return charmUUID
}

func (s *lxdProfileStateSuite) addUnit(c *tc.C, appUUID, charmUUID, netNodeUUID string) {
	unitUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `
INSERT INTO unit (uuid, name, life_id, net_node_uuid, application_uuid, charm_uuid) 
VALUES (?,?,?,?,?,?)`,
		unitUUID, unitUUID, life.Alive, netNodeUUID, appUUID, charmUUID)
}
