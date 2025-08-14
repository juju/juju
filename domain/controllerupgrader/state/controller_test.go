// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/uuid"
)

// controllerStateSuite is a collection of tests to assert the contracts of
// offered by [ControllerState].
type controllerStateSuite struct {
	schematesting.ControllerSuite
}

// TestControllerStateSuite runs the tests in [controllerStateSuite].
func TestControllerStateSuite(t *testing.T) {
	tc.Run(t, &controllerStateSuite{})
}

// addControllerNodeAgentVersion adds a controller node to the cluster and sets
// its current reported agent version to the given value. The new controller
// node id is returned.
func (s *controllerStateSuite) addControllerNodeAgentVersion(
	c *tc.C, version string,
) string {
	id, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(
		"INSERT INTO controller_node (controller_id) VALUES (?)", id.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO controller_node_agent_version (controller_id, version, architecture_id)
VALUES (?, ?, ?)
`,
		id.String(), version, 0,
	)
	c.Assert(err, tc.ErrorIsNil)

	return id.String()
}

// setInitialControllerTargetVersion establishes a controller uuid and sets the
// initial target version of the controller.
func (s *controllerStateSuite) setInitialControllerTargetVersion(
	c *tc.C, version string,
) {
	controllerUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	modelUUID := coremodel.GenUUID(c)

	_, err = s.DB().Exec("INSERT INTO controller (uuid, model_uuid, target_version) VALUES (?, ?, ?)", controllerUUID.String(), modelUUID.String(), version)
	c.Assert(err, tc.ErrorIsNil)
}

// TestGetControllerNodeVersionsEmpty tests that when no controller node
// versions have been reported an empty value is returned with no error.
func (s *controllerStateSuite) TestGetControllerNodeVersionsEmpty(c *tc.C) {
	st := NewControllerState(s.TxnRunnerFactory())
	versions, err := st.GetControllerNodeVersions(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(versions, tc.HasLen, 0)
}

// TestGetControllerNodeVersions verifies that the controller node versions are
// reported correctly when two nodes have their version recorded.
func (s *controllerStateSuite) TestGetControllerNodeVersions(c *tc.C) {
	st := NewControllerState(s.TxnRunnerFactory())

	c1Version, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)
	id1 := s.addControllerNodeAgentVersion(c, c1Version.String())
	c2Version, err := semversion.Parse("4.0.4")
	c.Assert(err, tc.ErrorIsNil)
	id2 := s.addControllerNodeAgentVersion(c, c2Version.String())

	// Get the versions.
	versions, err := st.GetControllerNodeVersions(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(versions, tc.DeepEquals, map[string]semversion.Number{
		id1: c1Version,
		id2: c2Version,
	})
}

// TestSetAndGetControllerVersion tests that the controller version can be
// retrieved with no errors and can also be set (upgraded) with no errors.
// This is a happy path test of:
// - [ControllerState.GetControllerVersion]
// - [ControllerState.SetControllerTargetVersion]
func (s *controllerStateSuite) TestSetAndGetControllerVersion(c *tc.C) {
	initialVersion, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)
	upgradeVersion, err := semversion.Parse("4.0.4")
	c.Assert(err, tc.ErrorIsNil)

	s.setInitialControllerTargetVersion(c, initialVersion.String())
	st := NewControllerState(s.TxnRunnerFactory())

	// Check initial version is reported correctly.
	ver, err := st.GetControllerVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver, tc.Equals, initialVersion)

	// Upgrade version.
	err = st.SetControllerTargetVersion(c.Context(), upgradeVersion)
	c.Check(err, tc.ErrorIsNil)

	// Check upgraded version is reported correctly.
	ver, err = st.GetControllerVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver, tc.Equals, upgradeVersion)
}

// TestSetControllerVersionMultipleSetSafe tests that setting the controller
// target version multiple times to the same value is safe and doesn't produce
// an error.
//
// This is a requirement of the service layer caller neededing to be able to get
// state back in a consistent state.
func (s *controllerStateSuite) TestSetControllerVersionMultipleSetSafe(c *tc.C) {
	initialVersion, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)
	upgradeVersion, err := semversion.Parse("4.0.4")
	c.Assert(err, tc.ErrorIsNil)

	s.setInitialControllerTargetVersion(c, initialVersion.String())
	st := NewControllerState(s.TxnRunnerFactory())

	// Upgrade version #1.
	err = st.SetControllerTargetVersion(c.Context(), upgradeVersion)
	c.Check(err, tc.ErrorIsNil)

	// Upgrade version #2.
	err = st.SetControllerTargetVersion(c.Context(), upgradeVersion)
	c.Check(err, tc.ErrorIsNil)

	// Upgrade version #3.
	err = st.SetControllerTargetVersion(c.Context(), upgradeVersion)
	c.Check(err, tc.ErrorIsNil)

	// Check upgraded version is reported correctly.
	ver, err := st.GetControllerVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver, tc.Equals, upgradeVersion)
}
