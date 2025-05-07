// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/life"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type exposedStateSuite struct {
	baseSuite

	state *State
}

var _ = tc.Suite(&exposedStateSuite{})

func (s *exposedStateSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *exposedStateSuite) TestApplicationNotExposed(c *tc.C) {
	appUUID := coreapplication.ID(uuid.MustNewUUID().String())

	isExposed, err := s.state.IsApplicationExposed(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsFalse)
}

func (s *exposedStateSuite) TestApplicationExposedToSpace(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)
	s.createExposedEndpointSpace(c, appID)

	isExposed, err := s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsTrue)
}

func (s *exposedStateSuite) TestApplicationExposedCIDR(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)
	s.createExposedEndpointCIDR(c, appID, "10.0.0.0/24")

	isExposed, err := s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsTrue)
}

func (s *exposedStateSuite) TestExposedEndpointsEmpty(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)

	exposedEndpoints, err := s.state.GetExposedEndpoints(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exposedEndpoints, tc.IsNil)
}

func (s *exposedStateSuite) TestExposedEndpointsOnlySpace(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)
	s.createExposedEndpointSpace(c, appID)

	exposedEndpoints, err := s.state.GetExposedEndpoints(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(len(exposedEndpoints), tc.Equals, 1)
	c.Check(exposedEndpoints["endpoint0"].ExposeToSpaceIDs.SortedValues(), tc.DeepEquals, []string{"space0-uuid"})
}

func (s *exposedStateSuite) TestExposedEndpointsOnlyCIDR(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)
	s.createExposedEndpointCIDR(c, appID, "10.0.0.0/24")

	exposedEndpoints, err := s.state.GetExposedEndpoints(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(len(exposedEndpoints), tc.Equals, 1)
	c.Check(exposedEndpoints["endpoint0"].ExposeToCIDRs.SortedValues(), tc.DeepEquals, []string{"10.0.0.0/24"})
}

func (s *exposedStateSuite) TestExposedEndpointsFull(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)
	s.createExposedEndpointSpace(c, appID)
	s.createExposedEndpointCIDR(c, appID, "10.0.0.0/24")
	s.createExposedEndpointCIDR(c, appID, "10.0.1.0/24")

	exposedEndpoints, err := s.state.GetExposedEndpoints(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(len(exposedEndpoints), tc.Equals, 1)
	c.Check(exposedEndpoints["endpoint0"].ExposeToCIDRs.SortedValues(), tc.DeepEquals, []string{"10.0.0.0/24", "10.0.1.0/24"})
	c.Check(exposedEndpoints["endpoint0"].ExposeToSpaceIDs.SortedValues(), tc.DeepEquals, []string{"space0-uuid"})
}

func (s *exposedStateSuite) TestExposedEndpointsWithWildcard(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)

	err := s.state.MergeExposeSettings(context.Background(), appID, map[string]application.ExposedEndpoint{
		"": {
			ExposeToSpaceIDs: set.NewStrings("space0-uuid"),
			ExposeToCIDRs:    set.NewStrings("192.168.0.0/24", "192.168.1.0/24"),
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	isExposed, err := s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsTrue)

	exposedEndpoints, err := s.state.GetExposedEndpoints(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(len(exposedEndpoints), tc.Equals, 1)
	c.Check(exposedEndpoints[""].ExposeToCIDRs.SortedValues(), tc.DeepEquals, []string{"192.168.0.0/24", "192.168.1.0/24"})
	c.Check(exposedEndpoints[""].ExposeToSpaceIDs.SortedValues(), tc.DeepEquals, []string{"space0-uuid"})
}

func (s *exposedStateSuite) TestExposedEndpointsWithWildcardMultipleTimes(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)

	err := s.state.MergeExposeSettings(context.Background(), appID, map[string]application.ExposedEndpoint{
		"": {
			ExposeToSpaceIDs: set.NewStrings("space0-uuid"),
			ExposeToCIDRs:    set.NewStrings("192.168.0.0/24", "192.168.1.0/24"),
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	isExposed, err := s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsTrue)

	exposedEndpoints, err := s.state.GetExposedEndpoints(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(len(exposedEndpoints), tc.Equals, 1)
	c.Check(exposedEndpoints[""].ExposeToCIDRs.SortedValues(), tc.DeepEquals, []string{"192.168.0.0/24", "192.168.1.0/24"})
	c.Check(exposedEndpoints[""].ExposeToSpaceIDs.SortedValues(), tc.DeepEquals, []string{"space0-uuid"})

	err = s.state.MergeExposeSettings(context.Background(), appID, map[string]application.ExposedEndpoint{
		"": {
			ExposeToCIDRs: set.NewStrings("10.0.0.0/24", "10.0.1.0/24"),
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	isExposed, err = s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsTrue)

	exposedEndpoints, err = s.state.GetExposedEndpoints(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(len(exposedEndpoints), tc.Equals, 1)
	c.Check(exposedEndpoints[""].ExposeToCIDRs.SortedValues(), tc.DeepEquals, []string{"10.0.0.0/24", "10.0.1.0/24"})
	c.Check(exposedEndpoints[""].ExposeToSpaceIDs.IsEmpty(), jc.IsTrue)
}

func (s *exposedStateSuite) TestEndpointsOneNotExists(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)
	s.createExposedEndpointSpace(c, appID)

	err := s.state.EndpointsExist(context.Background(), appID, set.NewStrings("endpoint0", "unknown-endpoint"))
	c.Assert(err, tc.ErrorMatches, "one or more of the provided endpoints .* do not exist")
}

func (s *exposedStateSuite) TestEndpointsAllNotExists(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)
	s.createExposedEndpointCIDR(c, appID, "10.0.0.0/24")

	err := s.state.EndpointsExist(context.Background(), appID, set.NewStrings("missing-endpoint", "unknown-endpoint"))
	c.Assert(err, tc.ErrorMatches, "one or more of the provided endpoints .* do not exist")
}

func (s *exposedStateSuite) TestEndpointsExist(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)
	s.createExposedEndpointSpace(c, appID)

	err := s.state.EndpointsExist(context.Background(), appID, set.NewStrings("endpoint0"))
	c.Assert(err, tc.IsNil)
}

func (s *exposedStateSuite) TestSpacesOneNotExists(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)
	s.createExposedEndpointSpace(c, appID)

	err := s.state.SpacesExist(context.Background(), set.NewStrings("space0-uuid", "unknown-space"))
	c.Assert(err, tc.ErrorMatches, "one or more of the provided spaces .* do not exist")
}

func (s *exposedStateSuite) TestSpacesAllNotExists(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)
	s.createExposedEndpointCIDR(c, appID, "10.0.0.0/24")

	err := s.state.SpacesExist(context.Background(), set.NewStrings("missing-space", "unknown-space"))
	c.Assert(err, tc.ErrorMatches, "one or more of the provided spaces .* do not exist")
}

func (s *exposedStateSuite) TestSpacesExist(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)
	s.createExposedEndpointSpace(c, appID)

	err := s.state.SpacesExist(context.Background(), set.NewStrings("space0-uuid"))
	c.Assert(err, tc.IsNil)
}

func (s *exposedStateSuite) TestGetSpaceUUIDByNameNotFound(c *tc.C) {
	_, err := s.state.GetSpaceUUIDByName(context.Background(), "missing-space")
	c.Assert(err, tc.ErrorMatches, "space not found.*")
}

func (s *exposedStateSuite) TestGetSpaceUUIDByName(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)

	uuid, err := s.state.GetSpaceUUIDByName(context.Background(), "space0")
	c.Assert(err, tc.IsNil)
	c.Check(uuid, tc.Equals, network.Id("space0-uuid"))
}

func (s *exposedStateSuite) TestUnsetExposeSettings(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)
	s.createExposedEndpointSpace(c, appID)
	s.createExposedEndpointCIDR(c, appID, "10.0.0.0/24")

	isExposed, err := s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsTrue)

	err = s.state.UnsetExposeSettings(context.Background(), appID, set.NewStrings("endpoint0"))
	c.Assert(err, jc.ErrorIsNil)

	isExposed, err = s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsFalse)
}

func (s *exposedStateSuite) TestUnsetExposeSettingsOnlyOneEndpoint(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)
	s.createExposedEndpointSpace(c, appID)
	s.createExposedEndpointCIDR(c, appID, "10.0.0.0/24")

	err := s.state.MergeExposeSettings(context.Background(), appID, map[string]application.ExposedEndpoint{
		"": {
			ExposeToCIDRs: set.NewStrings("192.168.0.0/24", "192.168.1.0/24"),
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.UnsetExposeSettings(context.Background(), appID, set.NewStrings("endpoint0"))
	c.Assert(err, jc.ErrorIsNil)

	isExposed, err := s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsTrue)

	exposedEndpoints, err := s.state.GetExposedEndpoints(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(len(exposedEndpoints), tc.Equals, 1)
	c.Check(exposedEndpoints[""].ExposeToCIDRs.SortedValues(), tc.DeepEquals, []string{"192.168.0.0/24", "192.168.1.0/24"})
	c.Check(exposedEndpoints[""].ExposeToSpaceIDs.IsEmpty(), jc.IsTrue)
}

func (s *exposedStateSuite) TestUnsetExposeSettingsAllEndpoints(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)
	s.createExposedEndpointSpace(c, appID)
	s.createExposedEndpointCIDR(c, appID, "10.0.0.0/24")

	isExposed, err := s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsTrue)

	err = s.state.UnsetExposeSettings(context.Background(), appID, set.NewStrings())
	c.Assert(err, jc.ErrorIsNil)

	isExposed, err = s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsFalse)
}

func (s *exposedStateSuite) TestMergeExposeSettingsNewEntry(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)

	isExposed, err := s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsFalse)

	err = s.state.MergeExposeSettings(context.Background(), appID, map[string]application.ExposedEndpoint{
		"endpoint0": {
			ExposeToSpaceIDs: set.NewStrings("space0-uuid"),
			ExposeToCIDRs:    set.NewStrings("10.0.0.0/24", "10.0.1.0/24"),
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	isExposed, err = s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsTrue)

	exposedEndpoints, err := s.state.GetExposedEndpoints(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exposedEndpoints["endpoint0"].ExposeToCIDRs.SortedValues(), tc.DeepEquals, []string{"10.0.0.0/24", "10.0.1.0/24"})
	c.Check(exposedEndpoints["endpoint0"].ExposeToSpaceIDs.SortedValues(), tc.DeepEquals, []string{"space0-uuid"})
}

func (s *exposedStateSuite) TestMergeExposeSettingsExistingOverwriteCIDR(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)
	s.createExposedEndpointSpace(c, appID)
	s.createExposedEndpointCIDR(c, appID, "10.0.0.0/24")

	err := s.state.MergeExposeSettings(context.Background(), appID, map[string]application.ExposedEndpoint{
		"endpoint0": {
			ExposeToCIDRs: set.NewStrings("192.168.0.0/24", "192.168.1.0/24"),
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	isExposed, err := s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsTrue)

	exposedEndpoints, err := s.state.GetExposedEndpoints(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exposedEndpoints["endpoint0"].ExposeToCIDRs.SortedValues(), tc.DeepEquals, []string{"192.168.0.0/24", "192.168.1.0/24"})
	c.Check(exposedEndpoints["endpoint0"].ExposeToSpaceIDs.IsEmpty(), jc.IsTrue)
}

func (s *exposedStateSuite) TestMergeExposeSettingsExistingOverwriteSpace(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)
	s.createExposedEndpointSpace(c, appID)
	// Create a new space
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		insertSpace := `INSERT INTO space (uuid, name) VALUES (?, ?)`
		_, err := tx.ExecContext(ctx, insertSpace, "space1-uuid", "space1")
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.MergeExposeSettings(context.Background(), appID, map[string]application.ExposedEndpoint{
		"endpoint0": {
			ExposeToSpaceIDs: set.NewStrings("space1-uuid"),
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	isExposed, err := s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsTrue)

	exposedEndpoints, err := s.state.GetExposedEndpoints(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exposedEndpoints["endpoint0"].ExposeToCIDRs.IsEmpty(), jc.IsTrue)
	c.Check(exposedEndpoints["endpoint0"].ExposeToSpaceIDs.SortedValues(), tc.DeepEquals, []string{"space1-uuid"})
}

func (s *exposedStateSuite) TestMergeExposeSettingsWildcard(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)
	// Create a new space
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		insertSpace := `INSERT INTO space (uuid, name) VALUES (?, ?)`
		_, err := tx.ExecContext(ctx, insertSpace, "space1-uuid", "space1")
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	isExposed, err := s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsFalse)

	err = s.state.MergeExposeSettings(context.Background(), appID, map[string]application.ExposedEndpoint{
		"": {
			ExposeToCIDRs:    set.NewStrings("192.168.0.0/24", "192.168.1.0/24"),
			ExposeToSpaceIDs: set.NewStrings("space1-uuid"),
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	isExposed, err = s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsTrue)

	exposedEndpoints, err := s.state.GetExposedEndpoints(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exposedEndpoints[""].ExposeToCIDRs.SortedValues(), tc.DeepEquals, []string{"192.168.0.0/24", "192.168.1.0/24"})
	c.Check(exposedEndpoints[""].ExposeToSpaceIDs.SortedValues(), tc.DeepEquals, []string{"space1-uuid"})
}

func (s *exposedStateSuite) TestMergeExposeSettingsWildcardOverwrite(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)
	// Create a new space
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		insertSpace := `INSERT INTO space (uuid, name) VALUES (?, ?)`
		_, err := tx.ExecContext(ctx, insertSpace, "space1-uuid", "space1")
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	isExposed, err := s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsFalse)

	err = s.state.MergeExposeSettings(context.Background(), appID, map[string]application.ExposedEndpoint{
		"": {
			ExposeToCIDRs:    set.NewStrings("192.168.0.0/24", "192.168.1.0/24"),
			ExposeToSpaceIDs: set.NewStrings("space0-uuid", "space1-uuid"),
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	isExposed, err = s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsTrue)

	exposedEndpoints, err := s.state.GetExposedEndpoints(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exposedEndpoints[""].ExposeToCIDRs.SortedValues(), tc.DeepEquals, []string{"192.168.0.0/24", "192.168.1.0/24"})
	c.Check(exposedEndpoints[""].ExposeToSpaceIDs.SortedValues(), tc.DeepEquals, []string{"space0-uuid", "space1-uuid"})

	// Overwrite the wildcard endpoint.
	err = s.state.MergeExposeSettings(context.Background(), appID, map[string]application.ExposedEndpoint{
		"": {
			ExposeToCIDRs: set.NewStrings("10.0.0.0/24", "10.0.1.0/24"),
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	isExposed, err = s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsTrue)

	exposedEndpoints, err = s.state.GetExposedEndpoints(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exposedEndpoints[""].ExposeToCIDRs.SortedValues(), tc.DeepEquals, []string{"10.0.0.0/24", "10.0.1.0/24"})
	c.Check(exposedEndpoints[""].ExposeToSpaceIDs.IsEmpty(), jc.IsTrue)
}

func (s *exposedStateSuite) TestMergeExposeSettingsDifferentEndpointsNotOverwritten(c *tc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.setUpEndpoint(c, appID)
	// Create a new endpoint
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		insertCharmRelation := `INSERT INTO charm_relation (uuid, charm_uuid, scope_id, role_id, name) VALUES (?, ?, ?, ?, ?)`
		_, err := tx.ExecContext(ctx, insertCharmRelation, "charm-relation1-uuid", "charm0-uuid", "0", "0", "endpoint1")
		if err != nil {
			return err
		}
		insertEndpoint := `INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid) VALUES (?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertEndpoint, "app-endpoint1-uuid", appID, "space0-uuid", "charm-relation1-uuid")
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	isExposed, err := s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsFalse)

	err = s.state.MergeExposeSettings(context.Background(), appID, map[string]application.ExposedEndpoint{
		"endpoint0": {
			ExposeToCIDRs:    set.NewStrings("192.168.0.0/24", "192.168.1.0/24"),
			ExposeToSpaceIDs: set.NewStrings("space0-uuid"),
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	isExposed, err = s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsTrue)

	exposedEndpoints, err := s.state.GetExposedEndpoints(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exposedEndpoints["endpoint0"].ExposeToCIDRs.SortedValues(), tc.DeepEquals, []string{"192.168.0.0/24", "192.168.1.0/24"})
	c.Check(exposedEndpoints["endpoint0"].ExposeToSpaceIDs.SortedValues(), tc.DeepEquals, []string{"space0-uuid"})

	// Overwrite the wildcard endpoint.
	err = s.state.MergeExposeSettings(context.Background(), appID, map[string]application.ExposedEndpoint{
		"endpoint1": {
			ExposeToCIDRs: set.NewStrings("10.0.0.0/24", "10.0.1.0/24"),
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	isExposed, err = s.state.IsApplicationExposed(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isExposed, jc.IsTrue)

	exposedEndpoints, err = s.state.GetExposedEndpoints(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(len(exposedEndpoints), tc.Equals, 2)
	c.Check(exposedEndpoints["endpoint0"].ExposeToCIDRs.SortedValues(), tc.DeepEquals, []string{"192.168.0.0/24", "192.168.1.0/24"})
	c.Check(exposedEndpoints["endpoint0"].ExposeToSpaceIDs.SortedValues(), tc.DeepEquals, []string{"space0-uuid"})
	c.Check(exposedEndpoints["endpoint1"].ExposeToCIDRs.SortedValues(), tc.DeepEquals, []string{"10.0.0.0/24", "10.0.1.0/24"})
	c.Check(exposedEndpoints["endpoint1"].ExposeToSpaceIDs.IsEmpty(), jc.IsTrue)
}

func (s *exposedStateSuite) setUpEndpoint(c *tc.C, appID coreapplication.ID) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		insertSpace := `INSERT INTO space (uuid, name) VALUES (?, ?)`
		_, err := tx.ExecContext(ctx, insertSpace, "space0-uuid", "space0")
		if err != nil {
			return err
		}
		insertCharm := `INSERT INTO charm (uuid, reference_name) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, insertCharm, "charm0-uuid", "foo")
		if err != nil {
			return err
		}
		insertCharmRelation := `INSERT INTO charm_relation (uuid, charm_uuid, scope_id, role_id, name) VALUES (?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertCharmRelation, "charm-relation0-uuid", "charm0-uuid", "0", "0", "endpoint0")
		if err != nil {
			return err
		}
		insertEndpoint := `INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid) VALUES (?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertEndpoint, "app-endpoint0-uuid", appID, "space0-uuid", "charm-relation0-uuid")
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *exposedStateSuite) createExposedEndpointSpace(c *tc.C, appID coreapplication.ID) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		insertExposedSpace := `
INSERT INTO application_exposed_endpoint_space
(application_uuid, application_endpoint_uuid, space_uuid)
VALUES (?, ?, ?)`
		_, err := tx.ExecContext(ctx, insertExposedSpace, appID, "app-endpoint0-uuid", "space0-uuid")
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *exposedStateSuite) createExposedEndpointCIDR(c *tc.C, appID coreapplication.ID, cidr string) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		insertExposedCIDR := `
INSERT INTO application_exposed_endpoint_cidr
(application_uuid, application_endpoint_uuid, cidr)
VALUES (?, ?, ?)`
		_, err := tx.ExecContext(ctx, insertExposedCIDR, appID, "app-endpoint0-uuid", cidr)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}
