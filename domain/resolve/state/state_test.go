// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/resolve"
	resolveerrors "github.com/juju/juju/domain/resolve/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/status"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type stateSuite struct {
	schematesting.ModelSuite

	state *State
}

var _ = tc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.state = NewState(s.TxnRunnerFactory())
}

func (s *stateSuite) TestGetUnitUUID(c *tc.C) {
	u1 := application.AddUnitArg{}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	gotUUID, err := s.state.GetUnitUUID(context.Background(), "foo/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotUUID, tc.Equals, unitUUID)
}

func (s *stateSuite) TestGetUnitUUIDNotFound(c *tc.C) {
	_, err := s.state.GetUnitUUID(context.Background(), "missing-uuid")
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotFound)
}

func (s *stateSuite) TestUnitResolveModeNoUnit(c *tc.C) {
	_, err := s.state.UnitResolveMode(context.Background(), "missing-uuid")
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestUnitResolveModeUnitNotResolved(c *tc.C) {
	u1 := application.AddUnitArg{}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	_, err := s.state.UnitResolveMode(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestResolveUnitNoUnit(c *tc.C) {
	err := s.state.ResolveUnit(context.Background(), "missing-uuid", resolve.ResolveModeRetryHooks)
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitAgentStatusNotFound)
}

func (s *stateSuite) TestResolveUnitNoStatus(c *tc.C) {
	u1 := application.AddUnitArg{}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	err := s.state.ResolveUnit(context.Background(), unitUUID, resolve.ResolveModeRetryHooks)
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitAgentStatusNotFound)
}

func (s *stateSuite) TestResolveUnitNotInError(c *tc.C) {
	u1 := application.AddUnitArg{
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusIdle,
			},
		},
	}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	err := s.state.ResolveUnit(context.Background(), unitUUID, resolve.ResolveModeRetryHooks)
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotInErrorState)
}

func (s *stateSuite) TestResolveUnitNoHooks(c *tc.C) {
	u1 := application.AddUnitArg{
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusError,
			},
		},
	}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	err := s.state.ResolveUnit(context.Background(), unitUUID, resolve.ResolveModeNoHooks)
	c.Assert(err, jc.ErrorIsNil)

	mode, err := s.state.UnitResolveMode(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mode, tc.Equals, resolve.ResolveModeNoHooks)
}

func (s *stateSuite) TestResolveUnitRetryHooks(c *tc.C) {
	u1 := application.AddUnitArg{
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusError,
			},
		},
	}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	err := s.state.ResolveUnit(context.Background(), unitUUID, resolve.ResolveModeRetryHooks)
	c.Assert(err, jc.ErrorIsNil)

	mode, err := s.state.UnitResolveMode(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mode, tc.Equals, resolve.ResolveModeRetryHooks)
}

func (s *stateSuite) TestResolveUnitAlreadyResolved(c *tc.C) {
	u1 := application.AddUnitArg{
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusError,
			},
		},
	}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	err := s.state.ResolveUnit(context.Background(), unitUUID, resolve.ResolveModeRetryHooks)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.ResolveUnit(context.Background(), unitUUID, resolve.ResolveModeNoHooks)
	c.Assert(err, jc.ErrorIsNil)

	mode, err := s.state.UnitResolveMode(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mode, tc.Equals, resolve.ResolveModeNoHooks)
}

func (s *stateSuite) TestResolveAllUnitsNoUnits(c *tc.C) {
	err := s.state.ResolveAllUnits(context.Background(), resolve.ResolveModeRetryHooks)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestResolveAllUnitsNoUnitsInError(c *tc.C) {
	u1 := application.AddUnitArg{
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusIdle,
			},
		},
	}
	u2 := application.AddUnitArg{
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusIdle,
			},
		},
	}
	unitUUIDs := s.createApplication(c, "foo", u1, u2)

	err := s.state.ResolveAllUnits(context.Background(), resolve.ResolveModeRetryHooks)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.UnitResolveMode(context.Background(), unitUUIDs[0])
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotResolved)

	_, err = s.state.UnitResolveMode(context.Background(), unitUUIDs[1])
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestResolveAllUnitsRetryHooks(c *tc.C) {
	u1 := application.AddUnitArg{
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusError,
			},
		},
	}
	u2 := application.AddUnitArg{
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusError,
			},
		},
	}
	u3 := application.AddUnitArg{
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusIdle,
			},
		},
	}
	unitUUIDs := s.createApplication(c, "foo", u1, u2, u3)

	err := s.state.ResolveAllUnits(context.Background(), resolve.ResolveModeRetryHooks)
	c.Assert(err, jc.ErrorIsNil)

	mode, err := s.state.UnitResolveMode(context.Background(), unitUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mode, tc.Equals, resolve.ResolveModeRetryHooks)

	mode, err = s.state.UnitResolveMode(context.Background(), unitUUIDs[1])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mode, tc.Equals, resolve.ResolveModeRetryHooks)

	_, err = s.state.UnitResolveMode(context.Background(), unitUUIDs[2])
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestResolveAllUnitsNoHooks(c *tc.C) {
	u1 := application.AddUnitArg{
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusError,
			},
		},
	}
	u2 := application.AddUnitArg{
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusError,
			},
		},
	}
	u3 := application.AddUnitArg{
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusIdle,
			},
		},
	}
	unitUUIDs := s.createApplication(c, "foo", u1, u2, u3)

	err := s.state.ResolveAllUnits(context.Background(), resolve.ResolveModeNoHooks)
	c.Assert(err, jc.ErrorIsNil)

	mode, err := s.state.UnitResolveMode(context.Background(), unitUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mode, tc.Equals, resolve.ResolveModeNoHooks)

	mode, err = s.state.UnitResolveMode(context.Background(), unitUUIDs[1])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mode, tc.Equals, resolve.ResolveModeNoHooks)

	_, err = s.state.UnitResolveMode(context.Background(), unitUUIDs[2])
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestResolveAllUnitsAlreadyResolved(c *tc.C) {
	u1 := application.AddUnitArg{
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusError,
			},
		},
	}
	u2 := application.AddUnitArg{
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusError,
			},
		},
	}

	unitUUIDs := s.createApplication(c, "foo", u1, u2)
	err := s.state.ResolveUnit(context.Background(), unitUUIDs[0], resolve.ResolveModeRetryHooks)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.ResolveAllUnits(context.Background(), resolve.ResolveModeNoHooks)
	c.Assert(err, jc.ErrorIsNil)

	mode, err := s.state.UnitResolveMode(context.Background(), unitUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mode, tc.Equals, resolve.ResolveModeNoHooks)

	mode, err = s.state.UnitResolveMode(context.Background(), unitUUIDs[1])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mode, tc.Equals, resolve.ResolveModeNoHooks)
}

func (s *stateSuite) TestClearResolvedNoUnit(c *tc.C) {
	err := s.state.ClearResolved(context.Background(), "missing-uuid")
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestClearResolvedUnitNotResolved(c *tc.C) {
	u1 := application.AddUnitArg{}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	err := s.state.ClearResolved(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestClearResolvedRetryHooks(c *tc.C) {
	u1 := application.AddUnitArg{
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusError,
			},
		},
	}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	err := s.state.ResolveUnit(context.Background(), unitUUID, resolve.ResolveModeRetryHooks)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.ClearResolved(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.UnitResolveMode(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestClearResolvedNoHooks(c *tc.C) {
	u1 := application.AddUnitArg{
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusError,
			},
		},
	}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	err := s.state.ResolveUnit(context.Background(), unitUUID, resolve.ResolveModeNoHooks)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.ClearResolved(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.UnitResolveMode(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) createApplication(c *tc.C, name string, units ...application.AddUnitArg) []coreunit.UUID {
	appState := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	platform := deployment.Platform{
		Channel:      "22.04/stable",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "stable",
		Branch: "branch",
	}
	ctx := context.Background()

	appID, err := appState.CreateApplication(ctx, name, application.AddApplicationArg{
		Platform: platform,
		Channel:  channel,
		Charm: charm.Charm{
			Metadata: charm.Metadata{
				Name: name,
				Provides: map[string]charm.Relation{
					"endpoint": {
						Name:  "endpoint",
						Role:  charm.RoleProvider,
						Scope: charm.ScopeGlobal,
					},
					"misc": {
						Name:  "misc",
						Role:  charm.RoleProvider,
						Scope: charm.ScopeGlobal,
					},
				},
			},
			Manifest:      s.minimalManifest(c),
			ReferenceName: name,
			Source:        charm.CharmHubSource,
			Revision:      42,
			Hash:          "hash",
		},
		CharmDownloadInfo: &charm.DownloadInfo{
			Provenance:         charm.ProvenanceDownload,
			CharmhubIdentifier: "ident",
			DownloadURL:        "https://example.com",
			DownloadSize:       42,
		},
		Scale: len(units),
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	charmUUID, err := appState.GetCharmIDByApplicationName(ctx, name)
	c.Assert(err, jc.ErrorIsNil)

	unitNames, err := appState.AddIAASUnits(ctx, "", appID, charmUUID, units...)
	c.Assert(err, jc.ErrorIsNil)

	var unitUUIDs = make([]coreunit.UUID, len(units))
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		for i, unitName := range unitNames {
			var uuid coreunit.UUID
			err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", unitName).Scan(&uuid)
			if err != nil {
				return err
			}
			unitUUIDs[i] = uuid
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	return unitUUIDs
}

func (s *stateSuite) minimalManifest(c *tc.C) charm.Manifest {
	return charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Risk: charm.RiskStable,
				},
				Architectures: []string{"amd64"},
			},
		},
	}
}
