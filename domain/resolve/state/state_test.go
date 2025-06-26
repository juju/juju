// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

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

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.state = NewState(s.TxnRunnerFactory())
}

func (s *stateSuite) TestGetUnitUUID(c *tc.C) {
	u1 := application.AddIAASUnitArg{}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	gotUUID, err := s.state.GetUnitUUID(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotUUID, tc.Equals, unitUUID)
}

func (s *stateSuite) TestGetUnitUUIDNotFound(c *tc.C) {
	_, err := s.state.GetUnitUUID(c.Context(), "missing-uuid")
	c.Assert(err, tc.ErrorIs, resolveerrors.UnitNotFound)
}

func (s *stateSuite) TestUnitResolveModeNoUnit(c *tc.C) {
	_, err := s.state.UnitResolveMode(c.Context(), "missing-uuid")
	c.Assert(err, tc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestUnitResolveModeUnitNotResolved(c *tc.C) {
	u1 := application.AddIAASUnitArg{}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	_, err := s.state.UnitResolveMode(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestResolveUnitNoUnit(c *tc.C) {
	err := s.state.ResolveUnit(c.Context(), "missing-uuid", resolve.ResolveModeRetryHooks)
	c.Assert(err, tc.ErrorIs, resolveerrors.UnitAgentStatusNotFound)
}

func (s *stateSuite) TestResolveUnitNoStatus(c *tc.C) {
	u1 := application.AddIAASUnitArg{}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	err := s.state.ResolveUnit(c.Context(), unitUUID, resolve.ResolveModeRetryHooks)
	c.Assert(err, tc.ErrorIs, resolveerrors.UnitAgentStatusNotFound)
}

func (s *stateSuite) TestResolveUnitNotInError(c *tc.C) {
	u1 := application.AddIAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusIdle,
				},
			},
		},
	}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	err := s.state.ResolveUnit(c.Context(), unitUUID, resolve.ResolveModeRetryHooks)
	c.Assert(err, tc.ErrorIs, resolveerrors.UnitNotInErrorState)
}

func (s *stateSuite) TestResolveUnitNoHooks(c *tc.C) {
	u1 := application.AddIAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusError,
				},
			},
		},
	}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	err := s.state.ResolveUnit(c.Context(), unitUUID, resolve.ResolveModeNoHooks)
	c.Assert(err, tc.ErrorIsNil)

	mode, err := s.state.UnitResolveMode(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mode, tc.Equals, resolve.ResolveModeNoHooks)
}

func (s *stateSuite) TestResolveUnitRetryHooks(c *tc.C) {
	u1 := application.AddIAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusError,
				},
			},
		},
	}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	err := s.state.ResolveUnit(c.Context(), unitUUID, resolve.ResolveModeRetryHooks)
	c.Assert(err, tc.ErrorIsNil)

	mode, err := s.state.UnitResolveMode(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mode, tc.Equals, resolve.ResolveModeRetryHooks)
}

func (s *stateSuite) TestResolveUnitAlreadyResolved(c *tc.C) {
	u1 := application.AddIAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusError,
				},
			},
		},
	}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	err := s.state.ResolveUnit(c.Context(), unitUUID, resolve.ResolveModeRetryHooks)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.ResolveUnit(c.Context(), unitUUID, resolve.ResolveModeNoHooks)
	c.Assert(err, tc.ErrorIsNil)

	mode, err := s.state.UnitResolveMode(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mode, tc.Equals, resolve.ResolveModeNoHooks)
}

func (s *stateSuite) TestResolveAllUnitsNoUnits(c *tc.C) {
	err := s.state.ResolveAllUnits(c.Context(), resolve.ResolveModeRetryHooks)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestResolveAllUnitsNoUnitsInError(c *tc.C) {
	u1 := application.AddIAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusIdle,
				},
			},
		},
	}
	u2 := application.AddIAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusIdle,
				},
			},
		},
	}
	unitUUIDs := s.createApplication(c, "foo", u1, u2)

	err := s.state.ResolveAllUnits(c.Context(), resolve.ResolveModeRetryHooks)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.state.UnitResolveMode(c.Context(), unitUUIDs[0])
	c.Assert(err, tc.ErrorIs, resolveerrors.UnitNotResolved)

	_, err = s.state.UnitResolveMode(c.Context(), unitUUIDs[1])
	c.Assert(err, tc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestResolveAllUnitsRetryHooks(c *tc.C) {
	u1 := application.AddIAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusError,
				},
			},
		},
	}
	u2 := application.AddIAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusError,
				},
			},
		},
	}
	u3 := application.AddIAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusIdle,
				},
			},
		},
	}
	unitUUIDs := s.createApplication(c, "foo", u1, u2, u3)

	err := s.state.ResolveAllUnits(c.Context(), resolve.ResolveModeRetryHooks)
	c.Assert(err, tc.ErrorIsNil)

	mode, err := s.state.UnitResolveMode(c.Context(), unitUUIDs[0])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mode, tc.Equals, resolve.ResolveModeRetryHooks)

	mode, err = s.state.UnitResolveMode(c.Context(), unitUUIDs[1])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mode, tc.Equals, resolve.ResolveModeRetryHooks)

	_, err = s.state.UnitResolveMode(c.Context(), unitUUIDs[2])
	c.Assert(err, tc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestResolveAllUnitsNoHooks(c *tc.C) {
	u1 := application.AddIAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusError,
				},
			},
		},
	}
	u2 := application.AddIAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusError,
				},
			},
		},
	}
	u3 := application.AddIAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusIdle,
				},
			},
		},
	}
	unitUUIDs := s.createApplication(c, "foo", u1, u2, u3)

	err := s.state.ResolveAllUnits(c.Context(), resolve.ResolveModeNoHooks)
	c.Assert(err, tc.ErrorIsNil)

	mode, err := s.state.UnitResolveMode(c.Context(), unitUUIDs[0])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mode, tc.Equals, resolve.ResolveModeNoHooks)

	mode, err = s.state.UnitResolveMode(c.Context(), unitUUIDs[1])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mode, tc.Equals, resolve.ResolveModeNoHooks)

	_, err = s.state.UnitResolveMode(c.Context(), unitUUIDs[2])
	c.Assert(err, tc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestResolveAllUnitsAlreadyResolved(c *tc.C) {
	u1 := application.AddIAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusError,
				},
			},
		},
	}
	u2 := application.AddIAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusError,
				},
			},
		},
	}

	unitUUIDs := s.createApplication(c, "foo", u1, u2)
	err := s.state.ResolveUnit(c.Context(), unitUUIDs[0], resolve.ResolveModeRetryHooks)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.ResolveAllUnits(c.Context(), resolve.ResolveModeNoHooks)
	c.Assert(err, tc.ErrorIsNil)

	mode, err := s.state.UnitResolveMode(c.Context(), unitUUIDs[0])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mode, tc.Equals, resolve.ResolveModeNoHooks)

	mode, err = s.state.UnitResolveMode(c.Context(), unitUUIDs[1])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mode, tc.Equals, resolve.ResolveModeNoHooks)
}

func (s *stateSuite) TestClearResolvedNoUnit(c *tc.C) {
	err := s.state.ClearResolved(c.Context(), "missing-uuid")
	c.Assert(err, tc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestClearResolvedUnitNotResolved(c *tc.C) {
	u1 := application.AddIAASUnitArg{}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	err := s.state.ClearResolved(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestClearResolvedRetryHooks(c *tc.C) {
	u1 := application.AddIAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusError,
				},
			},
		},
	}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	err := s.state.ResolveUnit(c.Context(), unitUUID, resolve.ResolveModeRetryHooks)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.ClearResolved(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.state.UnitResolveMode(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) TestClearResolvedNoHooks(c *tc.C) {
	u1 := application.AddIAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusError,
				},
			},
		},
	}
	unitUUIDs := s.createApplication(c, "foo", u1)
	unitUUID := unitUUIDs[0]

	err := s.state.ResolveUnit(c.Context(), unitUUID, resolve.ResolveModeNoHooks)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.ClearResolved(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.state.UnitResolveMode(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *stateSuite) createApplication(c *tc.C, name string, units ...application.AddIAASUnitArg) []coreunit.UUID {
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
	ctx := c.Context()

	appID, err := appState.CreateCAASApplication(ctx, name, application.AddCAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
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
		},
		Scale: len(units),
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	unitNames, _, err := appState.AddIAASUnits(ctx, appID, units...)
	c.Assert(err, tc.ErrorIsNil)

	var unitUUIDs = make([]coreunit.UUID, len(units))
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Assert(err, tc.ErrorIsNil)

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
