// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
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

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)
	s.state = NewState(s.TxnRunnerFactory())
}

func (s *stateSuite) TestGetUnitUUID(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	gotUUID, err := s.state.GetUnitUUID(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotUUID, gc.Equals, unitUUID)
}

func (s *stateSuite) TestGetUnitUUIDNotFound(c *gc.C) {
	_, err := s.state.GetUnitUUID(context.Background(), "missing-uuid")
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotFound)
}

func (s *stateSuite) TestUnitResolveModeNoUnit(c *gc.C) {
	_, err := s.state.UnitResolveMode(context.Background(), "missing-uuid")
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestUnitResolveModeUnitNotResolved(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	_, err := s.state.UnitResolveMode(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestResolveUnitNoUnit(c *gc.C) {
	err := s.state.ResolveUnit(context.Background(), "missing-uuid", resolve.ResolveModeRetryHooks)
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitAgentStatusNotFound)
}

func (s *stateSuite) TestResolveUnitNoStatus(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	err := s.state.ResolveUnit(context.Background(), unitUUID, resolve.ResolveModeRetryHooks)
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitAgentStatusNotFound)
}

func (s *stateSuite) TestResolveUnitNotInError(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
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

func (s *stateSuite) TestResolveUnitNoHooks(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
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
	c.Check(mode, gc.Equals, resolve.ResolveModeNoHooks)
}

func (s *stateSuite) TestResolveUnitRetryHooks(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
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
	c.Check(mode, gc.Equals, resolve.ResolveModeRetryHooks)
}

func (s *stateSuite) TestResolveUnitAlreadyResolved(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
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
	c.Check(mode, gc.Equals, resolve.ResolveModeNoHooks)
}

func (s *stateSuite) TestResolveAllUnitsNoUnits(c *gc.C) {
	err := s.state.ResolveAllUnits(context.Background(), resolve.ResolveModeRetryHooks)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestResolveAllUnitsNoUnitsInError(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusIdle,
			},
		},
	}
	u2 := application.AddUnitArg{
		UnitName: "foo/667",
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

func (s *stateSuite) TestResolveAllUnitsRetryHooks(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusError,
			},
		},
	}
	u2 := application.AddUnitArg{
		UnitName: "foo/667",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusError,
			},
		},
	}
	u3 := application.AddUnitArg{
		UnitName: "foo/668",
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
	c.Check(mode, gc.Equals, resolve.ResolveModeRetryHooks)

	mode, err = s.state.UnitResolveMode(context.Background(), unitUUIDs[1])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mode, gc.Equals, resolve.ResolveModeRetryHooks)

	_, err = s.state.UnitResolveMode(context.Background(), unitUUIDs[2])
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestResolveAllUnitsNoHooks(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusError,
			},
		},
	}
	u2 := application.AddUnitArg{
		UnitName: "foo/667",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusError,
			},
		},
	}
	u3 := application.AddUnitArg{
		UnitName: "foo/668",
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
	c.Check(mode, gc.Equals, resolve.ResolveModeNoHooks)

	mode, err = s.state.UnitResolveMode(context.Background(), unitUUIDs[1])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mode, gc.Equals, resolve.ResolveModeNoHooks)

	_, err = s.state.UnitResolveMode(context.Background(), unitUUIDs[2])
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestResolveAllUnitsAlreadyResolved(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusError,
			},
		},
	}
	u2 := application.AddUnitArg{
		UnitName: "foo/667",
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
	c.Check(mode, gc.Equals, resolve.ResolveModeNoHooks)

	mode, err = s.state.UnitResolveMode(context.Background(), unitUUIDs[1])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mode, gc.Equals, resolve.ResolveModeNoHooks)
}

func (s *stateSuite) TestClearResolvedNoUnit(c *gc.C) {
	err := s.state.ClearResolved(context.Background(), "missing-uuid")
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestClearResolvedUnitNotResolved(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	err := s.state.ClearResolved(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestClearResolvedRetryHooks(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
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

func (s *stateSuite) TestClearResolvedNoHooks(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
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

func (s *stateSuite) createApplication(c *gc.C, name string, units ...application.AddUnitArg) []coreunit.UUID {
	appState := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	platform := application.Platform{
		Channel:      "22.04/stable",
		OSType:       application.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &application.Channel{
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

	for _, u := range units {
		err := appState.AddIAASUnits(ctx, "", appID, u)
		c.Assert(err, jc.ErrorIsNil)
	}

	var unitUUIDs = make([]coreunit.UUID, len(units))
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		for i, u := range units {
			var uuid coreunit.UUID
			err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", u.UnitName).Scan(&uuid)
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

func (s *stateSuite) minimalManifest(c *gc.C) charm.Manifest {
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
