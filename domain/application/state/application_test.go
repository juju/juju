// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"slices"
	stdtesting "testing"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/resource/testing"
	"github.com/juju/juju/core/semversion"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	removalstatemodel "github.com/juju/juju/domain/removal/state/model"
	"github.com/juju/juju/domain/resource"
	"github.com/juju/juju/domain/status"
	statusstate "github.com/juju/juju/domain/status/state"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type applicationStateSuite struct {
	baseSuite

	state *State
}

func TestApplicationStateSuite(t *stdtesting.T) {
	tc.Run(t, &applicationStateSuite{})
}

func (s *applicationStateSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *applicationStateSuite) TestCreateIAASApplication(c *tc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}
	ctx := c.Context()

	cons := constraints.Constraints{
		CpuCores: ptr(uint64(42)),
		Mem:      ptr(uint64(3072)),
	}

	id, machineNames, err := s.state.CreateIAASApplication(ctx, "666", application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Platform: platform,
			Charm: charm.Charm{
				Metadata:      s.minimalMetadata(c, "666"),
				Manifest:      s.minimalManifest(c),
				Source:        charm.CharmHubSource,
				ReferenceName: "666",
				Revision:      42,
				Architecture:  architecture.ARM64,
			},
			CharmDownloadInfo: &charm.DownloadInfo{
				Provenance:         charm.ProvenanceDownload,
				CharmhubIdentifier: "ident-1",
				DownloadURL:        "http://example.com/charm",
				DownloadSize:       666,
			},
			Channel:     channel,
			Constraints: cons,
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	s.assertIAASApplication(c, "666", platform, channel, false)

	// We didn't create any units, so there are no machine names.
	c.Assert(machineNames, tc.HasLen, 0)

	// Ensure that config is empty and trust is false.
	config, settings, err := s.state.GetApplicationConfigAndSettings(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.HasLen, 0)
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{Trust: false})

	// Status should be unset.
	statusState := statusstate.NewModelState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	sts, err := statusState.GetApplicationStatus(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sts, tc.DeepEquals, status.StatusInfo[status.WorkloadStatusType]{
		Status: status.WorkloadStatusUnset,
	})

	// Constraints should be set.
	constraints, err := s.state.GetApplicationConstraints(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(constraints, tc.DeepEquals, cons)
}

func (s *applicationStateSuite) TestCreateCAASApplication(c *tc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}
	ctx := c.Context()

	cons := constraints.Constraints{
		CpuCores: ptr(uint64(42)),
		Mem:      ptr(uint64(3072)),
	}
	id, err := s.state.CreateCAASApplication(ctx, "666", application.AddCAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Platform: platform,
			Charm: charm.Charm{
				Metadata:      s.minimalMetadata(c, "666"),
				Manifest:      s.minimalManifest(c),
				Source:        charm.CharmHubSource,
				ReferenceName: "666",
				Revision:      42,
				Architecture:  architecture.ARM64,
			},
			CharmDownloadInfo: &charm.DownloadInfo{
				Provenance:         charm.ProvenanceDownload,
				CharmhubIdentifier: "ident-1",
				DownloadURL:        "http://example.com/charm",
				DownloadSize:       666,
			},
			Channel:     channel,
			Constraints: cons,
		},
		Scale: 1,
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	scale := application.ScaleState{Scale: 1}
	s.assertCAASApplication(c, "666", platform, channel, scale, false)

	// Ensure that config is empty and trust is false.
	config, settings, err := s.state.GetApplicationConfigAndSettings(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.HasLen, 0)
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{Trust: false})

	// Status should be unset.
	statusState := statusstate.NewModelState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	sts, err := statusState.GetApplicationStatus(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sts, tc.DeepEquals, status.StatusInfo[status.WorkloadStatusType]{
		Status: status.WorkloadStatusUnset,
	})

	// Constraints should be set.
	constraints, err := s.state.GetApplicationConstraints(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(constraints, tc.DeepEquals, cons)
}

func (s *applicationStateSuite) TestCreateApplicationWithConfigAndSettings(c *tc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}
	ctx := c.Context()

	id, _, err := s.state.CreateIAASApplication(ctx, "666", application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Platform: platform,
			Charm: charm.Charm{
				Metadata:      s.minimalMetadata(c, "666"),
				Manifest:      s.minimalManifest(c),
				Source:        charm.CharmHubSource,
				ReferenceName: "666",
				Revision:      42,
				Architecture:  architecture.ARM64,
			},
			CharmDownloadInfo: &charm.DownloadInfo{
				Provenance:         charm.ProvenanceDownload,
				CharmhubIdentifier: "ident-1",
				DownloadURL:        "http://example.com/charm",
				DownloadSize:       666,
			},
			Channel: channel,
			Config: map[string]application.ApplicationConfig{
				"foo": {
					Value: ptr("bar"),
					Type:  charm.OptionString,
				},
			},
			Settings: application.ApplicationSettings{
				Trust: true,
			},
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	s.assertIAASApplication(c, "666", platform, channel, false)

	// Ensure that config is empty and trust is false.
	config, settings, err := s.state.GetApplicationConfigAndSettings(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, map[string]application.ApplicationConfig{
		"foo": {
			Value: ptr("bar"),
			Type:  charm.OptionString,
		},
	})
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{Trust: true})
}

func (s *applicationStateSuite) TestCreateApplicationWithPeerRelation(c *tc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}
	ctx := c.Context()

	_, _, err := s.state.CreateIAASApplication(ctx, "666", application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Platform: platform,
			Charm: charm.Charm{
				Metadata:      s.minimalMetadataWithPeerRelation(c, "666", "castor", "pollux"),
				Manifest:      s.minimalManifest(c),
				Source:        charm.CharmHubSource,
				ReferenceName: "666",
				Revision:      42,
				Architecture:  architecture.ARM64,
			},
			CharmDownloadInfo: &charm.DownloadInfo{
				Provenance:         charm.ProvenanceDownload,
				CharmhubIdentifier: "ident-1",
				DownloadURL:        "http://example.com/charm",
				DownloadSize:       666,
			},
			Channel: channel,
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("Failed to create application: %s", errors.ErrorStack(err)))
	s.assertIAASApplication(c, "666", platform, channel, false)

	s.assertPeerRelation(c, "666", map[string]int{"pollux": 1, "castor": 0})
}

func (s *applicationStateSuite) TestCreateApplicationWithStatus(c *tc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}
	ctx := c.Context()

	now := time.Now().UTC()
	id, _, err := s.state.CreateIAASApplication(ctx, "666", application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Platform: platform,
			Charm: charm.Charm{
				Metadata:      s.minimalMetadata(c, "666"),
				Manifest:      s.minimalManifest(c),
				Source:        charm.CharmHubSource,
				ReferenceName: "666",
				Revision:      42,
				Architecture:  architecture.ARM64,
			},
			CharmDownloadInfo: &charm.DownloadInfo{
				Provenance:         charm.ProvenanceDownload,
				CharmhubIdentifier: "ident-1",
				DownloadURL:        "http://example.com/charm",
				DownloadSize:       666,
			},
			Channel: channel,
			Status: &status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "test",
				Data:    []byte(`{"foo": "bar"}`),
				Since:   ptr(now),
			},
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	s.assertIAASApplication(c, "666", platform, channel, false)

	statusState := statusstate.NewModelState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	sts, err := statusState.GetApplicationStatus(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sts, tc.DeepEquals, status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "test",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(now),
	})
}

func (s *applicationStateSuite) TestCreateApplicationWithUnits(c *tc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}
	a := application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Platform: platform,
			Charm: charm.Charm{
				Metadata:      s.minimalMetadata(c, "666"),
				Manifest:      s.minimalManifest(c),
				Source:        charm.CharmHubSource,
				ReferenceName: "666",
				Revision:      42,
				Architecture:  architecture.ARM64,
			},
			CharmDownloadInfo: &charm.DownloadInfo{
				Provenance:         charm.ProvenanceDownload,
				CharmhubIdentifier: "ident-1",
				DownloadURL:        "http://example.com/charm",
				DownloadSize:       666,
			},
			Channel: channel,
		},
	}
	machineNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	us := []application.AddIAASUnitArg{{
		MachineUUID:        tc.Must(c, coremachine.NewUUID),
		MachineNetNodeUUID: machineNetNodeUUID,
		AddUnitArg: application.AddUnitArg{
			NetNodeUUID: machineNetNodeUUID,
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status:  status.UnitAgentStatusExecuting,
					Message: "test",
					Data:    []byte(`{"foo": "bar"}`),
					Since:   ptr(time.Now()),
				},
				WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusActive,
					Message: "test",
					Data:    []byte(`{"foo": "bar"}`),
					Since:   ptr(time.Now()),
				},
			},
		},
	}}
	ctx := c.Context()

	_, _, err := s.state.CreateIAASApplication(ctx, "foo", a, us)
	c.Assert(err, tc.ErrorIsNil)
	s.assertIAASApplication(c, "foo", platform, channel, false)
}

func (s *applicationStateSuite) TestCreateApplicationsWithSameCharm(c *tc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "stable",
		Branch: "branch",
	}
	ctx := c.Context()

	_, _, err := s.state.CreateIAASApplication(ctx, "foo1", application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Platform: platform,
			Channel:  channel,
			Charm: charm.Charm{
				Metadata:      s.minimalMetadata(c, "foo"),
				Manifest:      s.minimalManifest(c),
				Source:        charm.LocalSource,
				Revision:      42,
				Architecture:  architecture.ARM64,
				ReferenceName: "foo",
			},
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	_, _, err = s.state.CreateIAASApplication(ctx, "foo2", application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Platform: platform,
			Channel:  channel,
			Charm: charm.Charm{
				Metadata:      s.minimalMetadata(c, "foo"),
				Manifest:      s.minimalManifest(c),
				Source:        charm.LocalSource,
				Revision:      42,
				Architecture:  architecture.ARM64,
				ReferenceName: "foo",
			},
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	s.assertIAASApplication(c, "foo1", platform, channel, false)
	s.assertIAASApplication(c, "foo2", platform, channel, false)
}

func (s *applicationStateSuite) TestCreateApplicationWithoutChannel(c *tc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	ctx := c.Context()

	_, _, err := s.state.CreateIAASApplication(ctx, "666", application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Platform: platform,
			Charm: charm.Charm{
				Metadata: charm.Metadata{
					Name: "666",
				},
				Manifest:      s.minimalManifest(c),
				Source:        charm.LocalSource,
				ReferenceName: "666",
				Revision:      42,
			},
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	s.assertIAASApplication(c, "666", platform, nil, false)
}

func (s *applicationStateSuite) TestCreateApplicationWithEmptyChannel(c *tc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{}
	ctx := c.Context()

	_, _, err := s.state.CreateIAASApplication(ctx, "666", application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Platform: platform,
			Charm: charm.Charm{
				Metadata:      s.minimalMetadata(c, "666"),
				Manifest:      s.minimalManifest(c),
				Source:        charm.LocalSource,
				Revision:      42,
				ReferenceName: "666",
			},
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	s.assertIAASApplication(c, "666", platform, channel, false)
}

func (s *applicationStateSuite) TestCreateApplicationWithCharmStoragePath(c *tc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{}
	ctx := c.Context()

	_, _, err := s.state.CreateIAASApplication(ctx, "666", application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Platform: platform,
			Charm: charm.Charm{
				Metadata:      s.minimalMetadata(c, "666"),
				Manifest:      s.minimalManifest(c),
				Source:        charm.LocalSource,
				Revision:      42,
				ArchivePath:   "/some/path",
				Available:     true,
				ReferenceName: "666",
			},
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	s.assertIAASApplication(c, "666", platform, channel, true)
}

// TestCreateApplicationWithResolvedResources tests creation of an application with
// specified resources.
// It verifies that the charm_resource table is populated, alongside the
// resource and application_resource table with data from charm and arguments.
func (s *applicationStateSuite) TestCreateApplicationWithResolvedResources(c *tc.C) {
	charmResources := map[string]charm.Resource{
		"some-file": {
			Name:        "foo-file",
			Type:        "file",
			Path:        "/some/path/foo.txt",
			Description: "A file",
		},
		"some-image": {
			Name: "my-image",
			Type: "oci-image",
			Path: "repo.org/my-image:tag",
		},
	}
	rev := 42
	addResourcesArgs := []application.AddApplicationResourceArg{
		{
			Name:   "foo-file",
			Origin: charmresource.OriginUpload,
		},
		{
			Name:     "my-image",
			Revision: &rev,
			Origin:   charmresource.OriginStore,
		},
	}
	ctx := c.Context()

	_, _, err := s.state.CreateIAASApplication(
		ctx,
		"666",
		s.addApplicationArgForResources(c, "666", charmResources, addResourcesArgs),
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)
	// Check expected resources are added
	assertTxn := func(comment string, do func(ctx context.Context, tx *sql.Tx) error) {
		err := s.TxnRunner().StdTxn(c.Context(), do)
		c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) %s: %s", comment,
			errors.ErrorStack(err)))
	}
	var (
		appUUID   string
		charmUUID string
	)
	assertTxn("Fetch application and charm UUID", func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT uuid, charm_uuid
FROM application
WHERE name=?`, "666").Scan(&appUUID, &charmUUID)
	})
	var (
		foundCharmResources        []charm.Resource
		foundAppAvailableResources []application.AddApplicationResourceArg
		foundAppPotentialResources []application.AddApplicationResourceArg
	)
	assertTxn("Fetch charm resources", func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT cr.name, crk.name as kind, path, description
FROM charm_resource cr
JOIN charm_resource_kind crk ON crk.id=cr.kind_id
WHERE charm_uuid=?`, charmUUID)
		defer func() { _ = rows.Close() }()
		if err != nil {
			return errors.Capture(err)
		}
		for rows.Next() {
			var res charm.Resource
			if err := rows.Scan(&res.Name, &res.Type, &res.Path, &res.Description); err != nil {
				return errors.Capture(err)
			}
			foundCharmResources = append(foundCharmResources, res)
		}
		return nil
	})
	assertTxn("Fetch application available resources", func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT vr.name, revision, origin_type
FROM v_application_resource vr
WHERE application_uuid = ?
AND state = 'available'`, appUUID)
		if err != nil {
			return errors.Capture(err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var res application.AddApplicationResourceArg
			var originName string
			if err := rows.Scan(&res.Name, &res.Revision, &originName); err != nil {
				return errors.Capture(err)
			}
			if res.Origin, err = charmresource.ParseOrigin(originName); err != nil {
				return errors.Capture(err)
			}
			foundAppAvailableResources = append(foundAppAvailableResources, res)
		}
		return nil
	})

	assertTxn("Fetch application potential resources", func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT vr.name, revision, origin_type
FROM v_application_resource vr
WHERE application_uuid = ?
AND state = 'potential'`, appUUID)
		if err != nil {
			return errors.Capture(err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var res application.AddApplicationResourceArg
			var originName string
			if err := rows.Scan(&res.Name, &res.Revision, &originName); err != nil {
				return errors.Capture(err)
			}
			if res.Origin, err = charmresource.ParseOrigin(originName); err != nil {
				return errors.Capture(err)
			}
			foundAppPotentialResources = append(foundAppPotentialResources, res)
		}
		return nil
	})
	c.Check(foundCharmResources, tc.SameContents, slices.Collect(maps.Values(charmResources)),
		tc.Commentf("(Assert) mismatch between charm resources and inserted resources"))
	c.Check(foundAppAvailableResources, tc.SameContents, addResourcesArgs,
		tc.Commentf("(Assert) mismatch between app available app resources and inserted resources"))
	expectedPotentialResources := make([]application.AddApplicationResourceArg, 0, len(addResourcesArgs))
	for _, res := range addResourcesArgs {
		expectedPotentialResources = append(expectedPotentialResources, application.AddApplicationResourceArg{
			Name:     res.Name,
			Revision: nil,                       // nil revision
			Origin:   charmresource.OriginStore, // origin should always be store
		})
	}
	c.Check(foundAppPotentialResources, tc.SameContents, expectedPotentialResources,
		tc.Commentf("(Assert) mismatch between potential app resources and inserted resources"))
}

// TestCreateApplicationWithResolvedResources tests creation of an application with
// pending resources, where AddCharm has been called first.
// It verifies that the charm_resource table is populated, alongside the
// resource and application_resource table with data from charm and arguments.
// The pending_application_resource table should have no entries with the appName.
func (s *applicationStateSuite) TestCreateApplicationWithPendingResources(c *tc.C) {
	charmResources := map[string]charm.Resource{
		"some-file": {
			Name:        "foo-file",
			Type:        "file",
			Path:        "/some/path/foo.txt",
			Description: "A file",
		},
		"some-image": {
			Name: "my-image",
			Type: "oci-image",
			Path: "repo.org/my-image:tag",
		},
	}

	ctx := c.Context()

	appName := "666"
	args := s.addApplicationArgForResources(c, appName,
		charmResources, nil)

	charmID, _, err := s.state.AddCharm(ctx, args.Charm, nil, false)
	c.Assert(err, tc.ErrorIsNil)

	addResources := []resource.AddResourceDetails{
		{
			Name:     "foo-file",
			Revision: ptr(75),
		}, {
			Name:     "my-image",
			Revision: ptr(42),
		},
	}

	args.PendingResources = s.addResourcesBeforeApplication(c, appName, charmID.String(), addResources)

	_, _, err = s.state.CreateIAASApplication(ctx, appName, args, nil)
	c.Assert(err, tc.ErrorIsNil)
	// Check expected resources are added
	assertTxn := func(comment string, do func(ctx context.Context, tx *sql.Tx) error) {
		err := s.TxnRunner().StdTxn(c.Context(), do)
		c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) %s: %s", comment,
			errors.ErrorStack(err)))
	}
	var (
		appUUID   string
		charmUUID string
	)
	assertTxn("Fetch application and charm UUID", func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT uuid, charm_uuid
FROM application
WHERE name=?`, appName).Scan(&appUUID, &charmUUID)
	})
	var (
		foundCharmResources        []charm.Resource
		foundAppAvailableResources []resource.AddResourceDetails
		foundAppPotentialResources []resource.AddResourceDetails
	)
	assertTxn("Fetch charm resources", func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT cr.name, crk.name as kind, path, description
FROM charm_resource cr
JOIN charm_resource_kind crk ON crk.id=cr.kind_id
WHERE charm_uuid=?`, charmUUID)
		defer func() { _ = rows.Close() }()
		if err != nil {
			return errors.Capture(err)
		}
		for rows.Next() {
			var res charm.Resource
			if err := rows.Scan(&res.Name, &res.Type, &res.Path, &res.Description); err != nil {
				return errors.Capture(err)
			}
			foundCharmResources = append(foundCharmResources, res)
		}
		return nil
	})
	assertTxn("Fetch application available resources", func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT vr.name, revision
FROM v_application_resource vr
WHERE application_uuid = ?
AND state = 'available'`, appUUID)
		if err != nil {
			return errors.Capture(err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var res resource.AddResourceDetails

			if err := rows.Scan(&res.Name, &res.Revision); err != nil {
				return errors.Capture(err)
			}
			foundAppAvailableResources = append(foundAppAvailableResources, res)
		}
		return nil
	})

	assertTxn("Fetch application potential resources", func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT vr.name, revision
FROM v_application_resource vr
WHERE application_uuid = ?
AND state = 'potential'`, appUUID)
		if err != nil {
			return errors.Capture(err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var res resource.AddResourceDetails
			if err := rows.Scan(&res.Name, &res.Revision); err != nil {
				return errors.Capture(err)
			}
			foundAppPotentialResources = append(foundAppPotentialResources, res)
		}
		return nil
	})
	c.Check(foundCharmResources, tc.SameContents, slices.Collect(maps.Values(charmResources)),
		tc.Commentf("(Assert) mismatch between charm resources and inserted resources"))
	c.Check(foundAppAvailableResources, tc.SameContents, addResources,
		tc.Commentf("(Assert) mismatch between app available app resources and inserted resources"))
	expectedPotentialResources := make([]resource.AddResourceDetails, 0, len(addResources))
	for _, res := range addResources {
		expectedPotentialResources = append(expectedPotentialResources, resource.AddResourceDetails{
			Name:     res.Name,
			Revision: nil, // nil revision
		})
	}
	c.Check(foundAppPotentialResources, tc.SameContents, expectedPotentialResources,
		tc.Commentf("(Assert) mismatch between potential app resources and inserted resources"))

	assertTxn("No pending application resources", func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT resource_uuid FROM pending_application_resource WHERE application_name = ?", appName).Scan(nil)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	})
}

// addResourcesBeforeApplication mimics the behavior of AddResourcesBeforeApplication
// from the resource domain for testing CreateApplication.
func (s *applicationStateSuite) addResourcesBeforeApplication(
	c *tc.C,
	appName, charmUUID string,
	appResources []resource.AddResourceDetails,
) []coreresource.UUID {
	resources := make([]addPendingResource, len(appResources))
	resourceUUIDs := make([]coreresource.UUID, len(appResources))
	for i, r := range appResources {
		resourceUUIDs[i] = testing.GenResourceUUID(c)
		resources[i] = addPendingResource{
			UUID:      resourceUUIDs[i].String(),
			Name:      r.Name,
			Revision:  r.Revision,
			CreatedAt: time.Now(),
		}
	}

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, res := range resources {
			insertStmt := `
INSERT INTO resource (uuid, charm_uuid, charm_resource_name, revision,
       origin_type_id, state_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
`
			_, err := tx.ExecContext(ctx, insertStmt,
				res.UUID, charmUUID, res.Name, res.Revision, 1, 0, res.CreatedAt)
			c.Assert(err, tc.IsNil)
			if err != nil {
				return err
			}

			linkStmt := `
INSERT INTO pending_application_resource (application_name, resource_uuid)
VALUES (?, ?)
`
			_, err = tx.ExecContext(ctx, linkStmt, appName, res.UUID)
			c.Assert(err, tc.IsNil)
			if err != nil {
				return err
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return resourceUUIDs
}

// addPendingResource holds the data required to add a pending
// resource into the resource table.
type addPendingResource struct {
	UUID      string
	Name      string
	Revision  *int
	CreatedAt time.Time
}

// TestCreateApplicationWithExistingCharmWithResources ensures that two
// applications with resources can be created from the same charm.
func (s *applicationStateSuite) TestCreateApplicationWithExistingCharmWithResources(c *tc.C) {
	charmResources := map[string]charm.Resource{
		"some-file": {
			Name:        "foo-file",
			Type:        "file",
			Path:        "/some/path/foo.txt",
			Description: "A file",
		},
	}
	addResourcesArgs := []application.AddApplicationResourceArg{
		{
			Name:   "foo-file",
			Origin: charmresource.OriginUpload,
		},
	}
	ctx := c.Context()

	_, _, err := s.state.CreateIAASApplication(ctx, "666", s.addApplicationArgForResources(c, "666",
		charmResources, addResourcesArgs), nil)
	c.Assert(err, tc.ErrorIsNil)

	_, _, err = s.state.CreateIAASApplication(ctx, "667", s.addApplicationArgForResources(c, "666",
		charmResources, addResourcesArgs), nil)
	c.Check(err, tc.ErrorIsNil, tc.Commentf("Failed to create second "+
		"application. Maybe the charm UUID is not properly fetched to pass to "+
		"resources ?"))
}

// TestCreateApplicationWithResourcesMissingResourceArg verifies resource
// handling during app creation.
// If a resource is missing from argument, it is added anyway from charm
// resources and is assumed to be of origin store with no revision.
func (s *applicationStateSuite) TestCreateApplicationWithResourcesMissingResourceArg(c *tc.C) {
	charmResources := map[string]charm.Resource{
		"some-file": {
			Name:        "foo-file",
			Type:        "file",
			Path:        "/some/path/foo.txt",
			Description: "A file",
		},
		"some-image": {
			Name: "my-image",
			Type: "oci-image",
			Path: "repo.org/my-image:tag",
		},
	}
	addResourceArgs := []application.AddApplicationResourceArg{
		{
			Name:   "foo-file",
			Origin: charmresource.OriginUpload,
		},
		// Missing some-image
	}
	ctx := c.Context()

	_, _, err := s.state.CreateIAASApplication(ctx, "666", s.addApplicationArgForResources(c, "666",
		charmResources, addResourceArgs), nil)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) unexpected error: %s",
		errors.ErrorStack(err)))
}

// TestCreateApplicationWithResourcesTooMuchResourceArgs verifies error handling
// for invalid resources.
// It fails if there is resources args that doesn't refer to actual resources
// in charm.
func (s *applicationStateSuite) TestCreateApplicationWithResourcesTooMuchResourceArgs(c *tc.C) {
	s.createSubnetForCAASModel(c)
	charmResources := map[string]charm.Resource{
		"some-file": {
			Name:        "foo-file",
			Type:        "file",
			Path:        "/some/path/foo.txt",
			Description: "A file",
		},
		// No image resource
	}
	rev := 42
	addResourcesArgs := []application.AddApplicationResourceArg{
		{
			Name:   "foo-file",
			Origin: charmresource.OriginUpload,
		},
		// Not a charm resource
		{
			Name:     "my-image",
			Revision: &rev,
			Origin:   charmresource.OriginStore,
		},
	}
	ctx := c.Context()

	_, _, err := s.state.CreateIAASApplication(ctx, "666", s.addApplicationArgForResources(c, "666",
		charmResources, addResourcesArgs), nil)
	c.Assert(err, tc.ErrorMatches,
		`.*inserting resource "my-image": resource not found in charm metadata.*`,
		tc.Commentf("(Assert) unexpected error: %s",
			errors.ErrorStack(err)))
}

func (s *applicationStateSuite) TestIsControllerApplication(c *tc.C) {
	appID := s.createIAASApplication(c, "foo", life.Dying)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO application_controller (application_uuid) VALUES (?)`,
			appID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	isController, err := s.state.IsControllerApplication(c.Context(), appID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isController, tc.IsTrue)
}

func (s *applicationStateSuite) TestIsControllerApplicationFalse(c *tc.C) {
	// Existing application:
	appID := s.createIAASApplication(c, "foo", life.Dying)
	isController, err := s.state.IsControllerApplication(c.Context(), appID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isController, tc.IsFalse)

	// Non-existing application:
	missingAppID := applicationtesting.GenApplicationUUID(c)
	isController, err = s.state.IsControllerApplication(c.Context(), missingAppID)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
	c.Assert(isController, tc.IsFalse)
}

func (s *applicationStateSuite) TestGetApplicationLifeByName(c *tc.C) {
	appID := s.createIAASApplication(c, "foo", life.Dying)
	gotID, appLife, err := s.state.GetApplicationLifeByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotID, tc.Equals, appID)
	c.Assert(appLife, tc.Equals, life.Dying)
}

func (s *applicationStateSuite) TestGetApplicationLifeByNameNotFound(c *tc.C) {
	_, _, err := s.state.GetApplicationLifeByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetApplicationLife(c *tc.C) {
	appID := s.createIAASApplication(c, "foo", life.Dying)
	appLife, err := s.state.GetApplicationLife(c.Context(), appID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appLife, tc.Equals, life.Dying)
}

func (s *applicationStateSuite) TestGetApplicationLifeNotFound(c *tc.C) {
	appID := applicationtesting.GenApplicationUUID(c)
	_, err := s.state.GetApplicationLife(c.Context(), appID)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestCheckAllApplicationsAndUnitsAreAliveEmptyModel(c *tc.C) {
	err := s.state.CheckAllApplicationsAndUnitsAreAlive(c.Context())
	c.Check(err, tc.ErrorIsNil)
}

func (s *applicationStateSuite) TestCheckAllApplicationsAndUnitsAreAlive(c *tc.C) {
	// Arrange: Some apps with units
	s.createIAASApplicationWithNUnits(c, "foo", life.Alive, 3)
	s.createIAASApplicationWithNUnits(c, "bar", life.Alive, 3)

	// Act:
	err := s.state.CheckAllApplicationsAndUnitsAreAlive(c.Context())

	// Assert:
	c.Check(err, tc.ErrorIsNil)
}

func (s *applicationStateSuite) TestCheckAllApplicationsAndUnitsAreAliveWithDyingApplications(c *tc.C) {
	// Arrange: Some apps with units, where some are dying
	s.createIAASApplicationWithNUnits(c, "foo", life.Dying, 1)
	s.createIAASApplicationWithNUnits(c, "bar", life.Dying, 1)
	s.createIAASApplicationWithNUnits(c, "baz", life.Alive, 1)

	// Act:
	err := s.state.CheckAllApplicationsAndUnitsAreAlive(c.Context())

	// Assert: An error of correct type, mentioning the correct applications, is returned
	c.Check(err, tc.ErrorIs, applicationerrors.ApplicationNotAlive)
	c.Check(err, tc.ErrorMatches, `.*application\(s\) "(bar, foo|foo, bar)" are not alive`)
}

func (s *applicationStateSuite) TestCheckAllApplicationsAndUnitsAreAliveWithDyingUnits(c *tc.C) {
	// Arrange: an application with some dying units
	_, units := s.createIAASApplicationWithNUnits(c, "foo", life.Alive, 3)

	removalState := removalstatemodel.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	_, err := removalState.EnsureUnitNotAliveCascade(c.Context(), units[0].String(), false)
	c.Assert(err, tc.ErrorIsNil)
	_, err = removalState.EnsureUnitNotAliveCascade(c.Context(), units[1].String(), false)
	c.Assert(err, tc.ErrorIsNil)

	// Act:
	err = s.state.CheckAllApplicationsAndUnitsAreAlive(c.Context())

	// Assert: an error of correct type, mentioning the correct unit, is returned.
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotAlive)
	c.Assert(err, tc.ErrorMatches, `.*unit\(s\) "(foo/0, foo/1|foo/1, foo/0)" are not alive`)
}

func (s *applicationStateSuite) TestUpsertCloudServiceNew(c *tc.C) {
	appID := s.createCAASApplication(c, "foo", life.Alive)
	err := s.state.UpsertCloudService(c.Context(), "foo", "provider-id", network.ProviderAddresses{})
	c.Assert(err, tc.ErrorIsNil)
	var providerID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT provider_id FROM k8s_service WHERE application_uuid = ?", appID).Scan(&providerID)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(providerID, tc.Equals, "provider-id")
}

func (s *applicationStateSuite) TestUpsertCloudServiceExisting(c *tc.C) {
	appID := s.createCAASApplication(c, "foo", life.Alive)
	s.createSubnetForCAASModel(c)
	err := s.state.UpsertCloudService(c.Context(), "foo", "provider-id", network.ProviderAddresses{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.UpsertCloudService(c.Context(), "foo", "provider-id", network.ProviderAddresses{})
	c.Assert(err, tc.ErrorIsNil)
	var providerID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT provider_id FROM k8s_service WHERE application_uuid = ?", appID).Scan(&providerID)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(providerID, tc.Equals, "provider-id")
}

func (s *applicationStateSuite) TestUpsertCloudServiceAnother(c *tc.C) {
	appID := s.createCAASApplication(c, "foo", life.Alive)
	s.createCAASApplication(c, "bar", life.Alive)
	err := s.state.UpsertCloudService(c.Context(), "foo", "provider-id", network.ProviderAddresses{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.UpsertCloudService(c.Context(), "foo", "another-provider-id", network.ProviderAddresses{})
	c.Assert(err, tc.ErrorIsNil)
	var providerIds []string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT provider_id FROM k8s_service WHERE application_uuid = ?", appID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var providerId string
			if err := rows.Scan(&providerId); err != nil {
				return err
			}
			providerIds = append(providerIds, providerId)
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(providerIds, tc.SameContents, []string{"provider-id", "another-provider-id"})
}

// TestUpsertAnotherCloudServiceNotWipingIpAddresses is a regression test where
// calling UpsertCloudService on an application would wipe out any IP address.
func (s *applicationStateSuite) TestUpsertAnotherCloudServiceNotWipingIpAddresses(c *tc.C) {
	appName := "foo"
	s.createCAASApplication(c, appName, life.Alive, application.AddCAASUnitArg{})
	err := s.state.UpdateCAASUnit(c.Context(), unit.Name(fmt.Sprintf("%s/0", appName)), application.UpdateCAASUnitParams{
		ProviderID: ptr("provider-id-unit"),
		Address:    ptr("10.0.0.2"),
	})
	c.Assert(err, tc.ErrorIsNil)

	k8sPodInfo, err := s.state.GetUnitK8sPodInfo(c.Context(), unit.Name(fmt.Sprintf("%s/0", appName)))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(k8sPodInfo.Address, tc.Equals, "10.0.0.2")
	err = s.state.UpsertCloudService(c.Context(), appName, "provider-id",
		network.ProviderAddresses{
			{
				MachineAddress: network.NewMachineAddress("10.0.0.1/24"),
			},
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	k8sPodInfo, err = s.state.GetUnitK8sPodInfo(c.Context(), unit.Name(fmt.Sprintf("%s/0", appName)))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(k8sPodInfo.Address, tc.Equals, "10.0.0.2")
}

func (s *applicationStateSuite) TestUpsertCloudServiceUpdateExistingEmptyAddresses(c *tc.C) {
	appID := s.createCAASApplication(c, "foo", life.Alive)
	s.createCAASApplication(c, "bar", life.Alive)
	err := s.state.UpsertCloudService(c.Context(), "foo", "provider-id", network.ProviderAddresses{
		{
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.1/8",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
			},
		},
		{
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.2/8",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv6Address,
				Scope:      network.ScopeLinkLocal,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	checkAddresses := func(c *tc.C, expectedAddresses ...string) {
		var resultAddresses []string
		err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			rows, err := tx.QueryContext(ctx, `
SELECT address_value
FROM ip_address
JOIN link_layer_device ON link_layer_device.uuid = ip_address.device_uuid
JOIN net_node ON net_node.uuid = link_layer_device.net_node_uuid
JOIN k8s_service ON k8s_service.net_node_uuid = net_node.uuid
WHERE application_uuid = ?
			`, appID)
			if err != nil {
				return err
			}
			defer func() { _ = rows.Close() }()

			for rows.Next() {
				var addressVal string
				if err := rows.Scan(&addressVal); err != nil {
					return err
				}
				resultAddresses = append(resultAddresses, addressVal)
			}
			return rows.Err()
		})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(resultAddresses, tc.SameContents, expectedAddresses)
	}

	checkAddresses(c, "10.0.0.1/8", "10.0.0.2/8")

	err = s.state.UpsertCloudService(c.Context(), "foo", "provider-id", network.ProviderAddresses{})
	c.Assert(err, tc.ErrorIsNil)
	// Since no addresses were passed as input, the previous addresses should
	// be returned.
	checkAddresses(c, "10.0.0.1/8", "10.0.0.2/8")
}

func (s *applicationStateSuite) TestUpsertCloudServiceUpdateExistingWithAddresses(c *tc.C) {
	appID := s.createCAASApplication(c, "foo", life.Alive)
	s.createCAASApplication(c, "bar", life.Alive)
	err := s.state.UpsertCloudService(c.Context(), "foo", "provider-id", network.ProviderAddresses{
		{
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.1/24",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
			},
		},
		{
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.2/24",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv6Address,
				Scope:      network.ScopeLinkLocal,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	checkAddresses := func(c *tc.C, expectedAddresses ...string) {
		var resultAddresses []string
		err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			rows, err := tx.QueryContext(ctx, `
SELECT address_value
FROM ip_address
JOIN link_layer_device ON link_layer_device.uuid = ip_address.device_uuid
JOIN net_node ON net_node.uuid = link_layer_device.net_node_uuid
JOIN k8s_service ON k8s_service.net_node_uuid = net_node.uuid
WHERE application_uuid = ?
			`, appID)
			if err != nil {
				return err
			}
			defer func() { _ = rows.Close() }()

			for rows.Next() {
				var addressVal string
				if err := rows.Scan(&addressVal); err != nil {
					return err
				}
				resultAddresses = append(resultAddresses, addressVal)
			}
			return rows.Err()
		})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(resultAddresses, tc.SameContents, expectedAddresses)
	}

	checkAddresses(c, "10.0.0.1/24", "10.0.0.2/24")

	err = s.state.UpsertCloudService(c.Context(), "foo", "provider-id", network.ProviderAddresses{
		{
			MachineAddress: network.MachineAddress{
				Value:      "192.168.0.0/24",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
			},
		},
		{
			MachineAddress: network.MachineAddress{
				Value:      "192.168.0.1/24",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv6Address,
				Scope:      network.ScopeLinkLocal,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	// Since no addresses were passed as input, the previous addresses should
	// be returned.
	checkAddresses(c, "192.168.0.0/24", "192.168.0.1/24")
}

func (s *applicationStateSuite) TestUpsertCloudServiceNotFound(c *tc.C) {
	err := s.state.UpsertCloudService(c.Context(), "foo", "provider-id", network.ProviderAddresses{})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetApplicationIDByUnitName(c *tc.C) {
	expectedAppUUID, _ := s.createIAASApplicationWithNUnits(c, "foo", life.Alive, 1)

	obtainedAppUUID, err := s.state.GetApplicationIDByUnitName(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedAppUUID, tc.Equals, expectedAppUUID)
}

func (s *applicationStateSuite) TestGetApplicationIDByUnitNameUnitUnitNotFound(c *tc.C) {
	_, err := s.state.GetApplicationIDByUnitName(c.Context(), "failme")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationStateSuite) TestGetApplicationIDAndNameByUnitName(c *tc.C) {
	expectedAppUUID, _ := s.createIAASApplicationWithNUnits(c, "foo", life.Alive, 1)

	appUUID, appName, err := s.state.GetApplicationIDAndNameByUnitName(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(appUUID, tc.Equals, expectedAppUUID)
	c.Check(appName, tc.Equals, "foo")
}

func (s *applicationStateSuite) TestGetApplicationIDAndNameByUnitNameNotFound(c *tc.C) {
	_, _, err := s.state.GetApplicationIDAndNameByUnitName(c.Context(), "failme")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationStateSuite) TestGetCharmModifiedVersion(c *tc.C) {
	appUUID := s.createIAASApplication(c, "foo", life.Alive)
	s.addCharmModifiedVersion(c, appUUID, 7)

	charmModifiedVersion, err := s.state.GetCharmModifiedVersion(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(charmModifiedVersion, tc.Equals, 7)
}

func (s *applicationStateSuite) TestGetCharmModifiedVersionNull(c *tc.C) {
	appUUID := s.createIAASApplication(c, "foo", life.Alive)

	charmModifiedVersion, err := s.state.GetCharmModifiedVersion(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(charmModifiedVersion, tc.Equals, 0)
}

func (s *applicationStateSuite) TestGetCharmModifiedVersionApplicationNotFound(c *tc.C) {
	_, err := s.state.GetCharmModifiedVersion(c.Context(), applicationtesting.GenApplicationUUID(c))
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetApplicationScaleState(c *tc.C) {
	appID, _ := s.createCAASApplicationWithNUnits(c, "foo", life.Alive, 1)

	scaleState, err := s.state.GetApplicationScaleState(c.Context(), appID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(scaleState, tc.DeepEquals, application.ScaleState{
		Scale: 1,
	})
}

func (s *applicationStateSuite) TestGetApplicationScaleStateNotFound(c *tc.C) {
	_, err := s.state.GetApplicationScaleState(c.Context(), coreapplication.ID(uuid.MustNewUUID().String()))
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestSetDesiredApplicationScale(c *tc.C) {
	appID := s.createCAASApplication(c, "foo", life.Alive)

	err := s.state.SetDesiredApplicationScale(c.Context(), appID, 666)
	c.Assert(err, tc.ErrorIsNil)

	var gotScale int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale FROM application_scale WHERE application_uuid=?", appID).
			Scan(&gotScale)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotScale, tc.DeepEquals, 666)
}

func (s *applicationStateSuite) TestUpdateApplicationScale(c *tc.C) {
	appID := s.createCAASApplication(c, "foo", life.Alive)

	err := s.state.SetDesiredApplicationScale(c.Context(), appID, 666)
	c.Assert(err, tc.ErrorIsNil)

	newScale, err := s.state.UpdateApplicationScale(c.Context(), appID, 2)
	c.Assert(err, tc.ErrorIsNil)

	var gotScale int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale FROM application_scale WHERE application_uuid=?", appID).
			Scan(&gotScale)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotScale, tc.DeepEquals, 666+2)
	c.Check(newScale, tc.DeepEquals, 666+2)
}

func (s *applicationStateSuite) TestUpdateApplicationScaleInvalidScale(c *tc.C) {
	appID := s.createCAASApplication(c, "foo", life.Alive)

	err := s.state.SetDesiredApplicationScale(c.Context(), appID, 666)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.state.UpdateApplicationScale(c.Context(), appID, -667)
	c.Assert(err, tc.ErrorMatches, `scale change invalid: cannot remove more units than currently exist`)
}

func (s *applicationStateSuite) TestSetApplicationScalingStateAlreadyScaling(c *tc.C) {
	appID := s.createCAASApplication(c, "foo", life.Dead)

	// Set up the initial scale value.
	err := s.state.SetDesiredApplicationScale(c.Context(), appID, 666)
	c.Assert(err, tc.ErrorIsNil)

	checkResult := func(want application.ScaleState) {
		var got application.ScaleState
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			err := tx.QueryRowContext(ctx, "SELECT scale, scaling, scale_target FROM application_scale WHERE application_uuid=?", appID).
				Scan(&got.Scale, &got.Scaling, &got.ScaleTarget)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(got, tc.DeepEquals, want)
	}

	err = s.state.SetApplicationScalingState(c.Context(), "foo", 42, true)
	c.Assert(err, tc.ErrorIsNil)
	checkResult(application.ScaleState{
		Scale:       42,
		ScaleTarget: 42,
		Scaling:     true,
	})

	// Set scaling state but use the same target value as current scale.
	err = s.state.SetApplicationScalingState(c.Context(), "foo", 42, true)
	c.Assert(err, tc.ErrorIsNil)
	checkResult(application.ScaleState{
		Scale:       42,
		ScaleTarget: 42,
		Scaling:     true,
	})
}

func (s *applicationStateSuite) TestSetApplicationScalingStateInconsistent(c *tc.C) {
	appID := s.createCAASApplication(c, "foo", life.Alive)

	// Set up the initial scale value.
	err := s.state.SetDesiredApplicationScale(c.Context(), appID, 666)
	c.Assert(err, tc.ErrorIsNil)

	// Set scaling state but use a target value different than the current
	// scale.
	err = s.state.SetApplicationScalingState(c.Context(), "foo", 42, true)
	c.Assert(err, tc.ErrorMatches, "scaling state is inconsistent")
}

func (s *applicationStateSuite) TestSetApplicationScalingStateAppDying(c *tc.C) {
	appID := s.createCAASApplication(c, "foo", life.Dying)

	// Set up the initial scale value.
	err := s.state.SetDesiredApplicationScale(c.Context(), appID, 666)
	c.Assert(err, tc.ErrorIsNil)

	checkResult := func(want application.ScaleState) {
		var got application.ScaleState
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			err := tx.QueryRowContext(ctx, "SELECT scale, scaling, scale_target FROM application_scale WHERE application_uuid=?", appID).
				Scan(&got.Scale, &got.Scaling, &got.ScaleTarget)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(got, tc.DeepEquals, want)
	}

	err = s.state.SetApplicationScalingState(c.Context(), "foo", 42, true)
	c.Assert(err, tc.ErrorIsNil)
	checkResult(application.ScaleState{
		Scale:       42,
		ScaleTarget: 42,
		Scaling:     true,
	})
}

// This test is exactly like TestSetApplicationScalingStateAppDying but the app
// is dead instead of dying.
func (s *applicationStateSuite) TestSetApplicationScalingStateAppDead(c *tc.C) {
	appID := s.createCAASApplication(c, "foo", life.Dead)

	// Set up the initial scale value.
	err := s.state.SetDesiredApplicationScale(c.Context(), appID, 666)
	c.Assert(err, tc.ErrorIsNil)

	checkResult := func(want application.ScaleState) {
		var got application.ScaleState
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			err := tx.QueryRowContext(ctx, "SELECT scale, scaling, scale_target FROM application_scale WHERE application_uuid=?", appID).
				Scan(&got.Scale, &got.Scaling, &got.ScaleTarget)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(got, tc.DeepEquals, want)
	}

	err = s.state.SetApplicationScalingState(c.Context(), "foo", 42, true)
	c.Assert(err, tc.ErrorIsNil)
	checkResult(application.ScaleState{
		Scale:       42,
		ScaleTarget: 42,
		Scaling:     true,
	})
}

func (s *applicationStateSuite) TestSetApplicationScalingStateNotScaling(c *tc.C) {
	appID := s.createCAASApplication(c, "foo", life.Alive)

	// Set up the initial scale value.
	err := s.state.SetDesiredApplicationScale(c.Context(), appID, 666)
	c.Assert(err, tc.ErrorIsNil)

	checkResult := func(want application.ScaleState) {
		var got application.ScaleState
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			err := tx.QueryRowContext(ctx, "SELECT scale, scaling, scale_target FROM application_scale WHERE application_uuid=?", appID).
				Scan(&got.Scale, &got.Scaling, &got.ScaleTarget)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(got, tc.DeepEquals, want)
	}

	err = s.state.SetApplicationScalingState(c.Context(), "foo", 668, false)
	c.Assert(err, tc.ErrorIsNil)
	checkResult(application.ScaleState{
		Scale:       666,
		ScaleTarget: 668,
		Scaling:     false,
	})
}

func (s *applicationStateSuite) TestGetApplicationUnitLife(c *tc.C) {
	_, fooUnits := s.createIAASApplicationWithNUnits(c, "foo", life.Alive, 2)
	_, barUnits := s.createIAASApplicationWithNUnits(c, "bar", life.Alive, 1)

	s.setUnitLife(c, fooUnits[0], life.Dead)

	got, err := s.state.GetApplicationUnitLife(c.Context(), "foo", fooUnits[1])
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, map[string]int{
		fooUnits[1].String(): 0,
	})

	got, err = s.state.GetApplicationUnitLife(c.Context(), "foo", fooUnits[:]...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, map[string]int{
		fooUnits[0].String(): 2,
		fooUnits[1].String(): 0,
	})

	got, err = s.state.GetApplicationUnitLife(c.Context(), "foo", fooUnits[1], barUnits[0])
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, map[string]int{
		fooUnits[1].String(): 0,
	})

	got, err = s.state.GetApplicationUnitLife(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.HasLen, 0)
}

func (s *applicationStateSuite) TestGetAllUnitLifeForApplication(c *tc.C) {
	fooAppID, fooUnits := s.createIAASApplicationWithNUnits(c, "foo", life.Alive, 2)
	barAppID, _ := s.createIAASApplicationWithNUnits(c, "bar", life.Alive, 1)
	s.setUnitLife(c, fooUnits[0], life.Dead)

	fooUnitLife, err := s.state.GetAllUnitLifeForApplication(c.Context(), fooAppID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fooUnitLife, tc.DeepEquals, map[string]int{
		"foo/0": 2,
		"foo/1": 0,
	})

	barUnitLife, err := s.state.GetAllUnitLifeForApplication(c.Context(), barAppID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(barUnitLife, tc.DeepEquals, map[string]int{
		"bar/0": 0,
	})

	_, err = s.state.GetAllUnitLifeForApplication(c.Context(),
		applicationtesting.GenApplicationUUID(c))
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetCharmIDByApplicationName(c *tc.C) {
	expectedMetadata := charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: semversion.MustParse("4.0.0"),
		Assumes:        []byte("null"),
	}
	expectedManifest := charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Track: "latest",
					Risk:  charm.RiskEdge,
				},
				Architectures: []string{"amd64", "arm64"},
			},
		},
	}
	expectedActions := charm.Actions{
		Actions: map[string]charm.Action{
			"action1": {
				Description:    "description",
				Parallel:       true,
				ExecutionGroup: "group",
				Params:         []byte(`{}`),
			},
		},
	}
	expectedConfig := charm.Config{
		Options: map[string]charm.Option{
			"option1": {
				Type:        "string",
				Description: "description",
				Default:     "default",
			},
		},
	}
	expectedLXDProfile := []byte("[{}]")

	s.createIAASApplication(c, "foo", life.Alive)

	_, _, err := s.state.AddCharm(c.Context(), charm.Charm{
		Metadata:      expectedMetadata,
		Manifest:      expectedManifest,
		Actions:       expectedActions,
		Config:        expectedConfig,
		LXDProfile:    expectedLXDProfile,
		Source:        charm.LocalSource,
		ReferenceName: expectedMetadata.Name,
		Revision:      42,
		Architecture:  architecture.AMD64,
	}, nil, false)
	c.Assert(err, tc.ErrorIsNil)

	chID, err := s.state.GetCharmIDByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(chID.Validate(), tc.ErrorIsNil)
}

func (s *applicationStateSuite) TestGetCharmIDByApplicationNameError(c *tc.C) {
	_, err := s.state.GetCharmIDByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetCharmByApplicationID(c *tc.C) {

	expectedMetadata := charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: semversion.MustParse("4.0.0"),
		Assumes:        []byte("null"),
	}
	expectedManifest := charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Track: "latest",
					Risk:  charm.RiskEdge,
				},
				Architectures: []string{"amd64", "arm64"},
			},
		},
	}
	expectedActions := charm.Actions{
		Actions: map[string]charm.Action{
			"action1": {
				Description:    "description",
				Parallel:       true,
				ExecutionGroup: "group",
				Params:         []byte(`{}`),
			},
		},
	}
	expectedConfig := charm.Config{
		Options: map[string]charm.Option{
			"option1": {
				Type:        "string",
				Description: "description",
				Default:     "default",
			},
		},
	}
	expectedLXDProfile := []byte("[{}]")
	platform := deployment.Platform{
		OSType:       deployment.Ubuntu,
		Architecture: architecture.AMD64,
		Channel:      "22.04",
	}
	ctx := c.Context()

	appID, _, err := s.state.CreateIAASApplication(ctx, "foo", application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Charm: charm.Charm{
				Metadata:      expectedMetadata,
				Manifest:      expectedManifest,
				Actions:       expectedActions,
				Config:        expectedConfig,
				LXDProfile:    expectedLXDProfile,
				Source:        charm.LocalSource,
				Revision:      42,
				Architecture:  architecture.AMD64,
				ReferenceName: "ubuntu",
			},
			Channel: &deployment.Channel{
				Track:  "track",
				Risk:   "stable",
				Branch: "branch",
			},
			Platform: platform,
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	// Add the implicit juju-info relation inserted with the charm.
	expectedMetadata.Provides = jujuInfoRelation()

	ch, err := s.state.GetCharmByApplicationID(c.Context(), appID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ch, tc.DeepEquals, charm.Charm{
		Metadata:      expectedMetadata,
		Manifest:      expectedManifest,
		Actions:       expectedActions,
		Config:        expectedConfig,
		LXDProfile:    expectedLXDProfile,
		Source:        charm.LocalSource,
		Revision:      42,
		Architecture:  architecture.AMD64,
		ReferenceName: "ubuntu",
	})

	// Ensure that the charm platform is also set AND it's the same as the
	// application platform.
	var gotPlatform deployment.Platform
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `

SELECT os_id, channel, architecture_id
FROM application_platform
WHERE application_uuid = ?
`, appID.String()).Scan(&gotPlatform.OSType, &gotPlatform.Channel, &gotPlatform.Architecture)

		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotPlatform, tc.DeepEquals, platform)
}

func (s *applicationStateSuite) TestCreateApplicationDefaultSourceIsCharmhub(c *tc.C) {
	expectedMetadata := charm.Metadata{
		Name:    "ubuntu",
		RunAs:   charm.RunAsRoot,
		Assumes: []byte{},
	}
	expectedManifest := charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Track: "latest",
					Risk:  charm.RiskEdge,
				},
				Architectures: []string{"amd64", "arm64"},
			},
		},
	}
	expectedActions := charm.Actions{
		Actions: map[string]charm.Action{
			"action1": {
				Description:    "description",
				Parallel:       true,
				ExecutionGroup: "group",
				Params:         []byte(`{}`),
			},
		},
	}
	expectedConfig := charm.Config{
		Options: map[string]charm.Option{
			"option1": {
				Type:        "string",
				Description: "description",
				Default:     "default",
			},
		},
	}
	ctx := c.Context()

	appID, _, err := s.state.CreateIAASApplication(ctx, "foo", application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Charm: charm.Charm{
				Metadata:      expectedMetadata,
				Manifest:      expectedManifest,
				Actions:       expectedActions,
				Config:        expectedConfig,
				Revision:      42,
				Source:        charm.LocalSource,
				Architecture:  architecture.AMD64,
				ReferenceName: "ubuntu",
			},
			Platform: deployment.Platform{
				OSType:       deployment.Ubuntu,
				Architecture: architecture.AMD64,
				Channel:      "22.04",
			},
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	// Add the implicit juju-info relation inserted with the charm.
	expectedMetadata.Provides = jujuInfoRelation()

	ch, err := s.state.GetCharmByApplicationID(c.Context(), appID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ch, tc.DeepEquals, charm.Charm{
		Metadata:      expectedMetadata,
		Manifest:      expectedManifest,
		Actions:       expectedActions,
		Config:        expectedConfig,
		Revision:      42,
		Source:        charm.LocalSource,
		Architecture:  architecture.AMD64,
		ReferenceName: "ubuntu",
	})
}

func (s *applicationStateSuite) TestSetCharmThenGetCharmByApplicationNameInvalidName(c *tc.C) {
	expectedMetadata := charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: semversion.MustParse("4.0.0"),
		Assumes:        []byte("null"),
	}
	ctx := c.Context()

	_, _, err := s.state.CreateIAASApplication(ctx, "foo", application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Charm: charm.Charm{
				Metadata:      expectedMetadata,
				Manifest:      s.minimalManifest(c),
				Source:        charm.LocalSource,
				ReferenceName: "ubuntu",
			},
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	id := applicationtesting.GenApplicationUUID(c)

	_, err = s.state.GetCharmByApplicationID(c.Context(), id)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestCheckCharmExistsNotFound(c *tc.C) {
	id := charmtesting.GenCharmID(c)
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.checkCharmExists(ctx, tx, charmID{
			UUID: id,
		})
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *applicationStateSuite) TestInitialWatchStatementApplicationsWithPendingCharms(c *tc.C) {
	name, query := s.state.InitialWatchStatementApplicationsWithPendingCharms()
	c.Check(name, tc.Equals, "application")

	id := s.createIAASApplication(c, "foo", life.Alive)

	result, err := query(c.Context(), s.TxnRunner())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{id.String()})
}

func (s *applicationStateSuite) TestInitialWatchStatementApplicationsWithPendingCharmsIfAvailable(c *tc.C) {
	// These use the same charm, so once you set one applications charm, you
	// set both.

	name, query := s.state.InitialWatchStatementApplicationsWithPendingCharms()
	c.Check(name, tc.Equals, "application")

	_ = s.createIAASApplication(c, "foo", life.Alive)
	id1 := s.createIAASApplication(c, "bar", life.Alive)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `

UPDATE charm SET available = TRUE
FROM application AS a
INNER JOIN charm AS c ON a.charm_uuid = c.uuid
WHERE a.uuid=?`, id1.String())

		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	result, err := query(c.Context(), s.TxnRunner())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

func (s *applicationStateSuite) TestInitialWatchStatementApplicationsWithPendingCharmsNothing(c *tc.C) {
	name, query := s.state.InitialWatchStatementApplicationsWithPendingCharms()
	c.Check(name, tc.Equals, "application")

	result, err := query(c.Context(), s.TxnRunner())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

func (s *applicationStateSuite) TestGetApplicationsWithPendingCharmsFromUUIDsIfPending(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	expected, err := s.state.GetApplicationsWithPendingCharmsFromUUIDs(c.Context(), []coreapplication.ID{id})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(expected, tc.DeepEquals, []coreapplication.ID{id})
}

func (s *applicationStateSuite) TestGetApplicationsWithPendingCharmsFromUUIDsIfAvailable(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `

UPDATE charm SET available = TRUE
FROM application AS a
INNER JOIN charm AS c ON a.charm_uuid = c.uuid
WHERE a.uuid=?`, id.String())

		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	expected, err := s.state.GetApplicationsWithPendingCharmsFromUUIDs(c.Context(), []coreapplication.ID{id})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(expected, tc.HasLen, 0)
}

func (s *applicationStateSuite) TestGetApplicationsWithPendingCharmsFromUUIDsNotFound(c *tc.C) {
	expected, err := s.state.GetApplicationsWithPendingCharmsFromUUIDs(c.Context(), []coreapplication.ID{"foo"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(expected, tc.HasLen, 0)
}

func (s *applicationStateSuite) TestGetApplicationsWithPendingCharmsFromUUIDsForSameCharm(c *tc.C) {
	// These use the same charm, so once you set one applications charm, you
	// set both.

	id0 := s.createIAASApplication(c, "foo", life.Alive)
	id1 := s.createIAASApplication(c, "bar", life.Alive)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `

UPDATE charm SET available = TRUE
FROM application AS a
INNER JOIN charm AS c ON a.charm_uuid = c.uuid
WHERE a.uuid=?`, id1.String())

		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	expected, err := s.state.GetApplicationsWithPendingCharmsFromUUIDs(c.Context(), []coreapplication.ID{id0, id1})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(expected, tc.HasLen, 0)
}

func (s *applicationStateSuite) TestGetAsyncCharmDownloadInfo(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	charmUUID, err := s.state.GetCharmIDByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	info, err := s.state.GetAsyncCharmDownloadInfo(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(info, tc.DeepEquals, application.CharmDownloadInfo{
		CharmUUID: charmUUID,
		Name:      "foo",
		SHA256:    "hash",
		DownloadInfo: charm.DownloadInfo{
			Provenance:         charm.ProvenanceDownload,
			CharmhubIdentifier: "ident",
			DownloadURL:        "https://example.com",
			DownloadSize:       42,
		},
	})
}

func (s *applicationStateSuite) TestGetAsyncCharmDownloadInfoNoApplication(c *tc.C) {
	id := applicationtesting.GenApplicationUUID(c)

	_, err := s.state.GetAsyncCharmDownloadInfo(c.Context(), id)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetAsyncCharmDownloadInfoAlreadyDone(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	charmUUID, err := s.state.GetCharmIDByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetCharmAvailable(c.Context(), charmUUID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.state.GetAsyncCharmDownloadInfo(c.Context(), id)
	c.Assert(err, tc.ErrorIs, applicationerrors.CharmAlreadyAvailable)
}

func (s *applicationStateSuite) TestResolveCharmDownload(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	objectStoreUUID := s.createObjectStoreBlob(c, "archive")

	info, err := s.state.GetAsyncCharmDownloadInfo(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)

	actions := charm.Actions{
		Actions: map[string]charm.Action{
			"action": {
				Description:    "description",
				Parallel:       true,
				ExecutionGroup: "group",
				Params:         []byte(`{}`),
			},
		},
	}

	err = s.state.ResolveCharmDownload(c.Context(), info.CharmUUID, application.ResolvedCharmDownload{
		Actions:         actions,
		LXDProfile:      []byte("profile"),
		ObjectStoreUUID: objectStoreUUID,
		ArchivePath:     "archive",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Ensure the charm is now available.
	available, err := s.state.IsCharmAvailable(c.Context(), info.CharmUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(available, tc.Equals, true)

	ch, err := s.state.GetCharmByApplicationID(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(ch.Actions, tc.DeepEquals, actions)
	c.Check(ch.LXDProfile, tc.DeepEquals, []byte("profile"))
	c.Check(ch.ArchivePath, tc.DeepEquals, "archive")
}

func (s *applicationStateSuite) TestResolveCharmDownloadAlreadyResolved(c *tc.C) {
	s.createIAASApplication(c, "foo", life.Alive)

	objectStoreUUID := s.createObjectStoreBlob(c, "archive")

	charmUUID, err := s.state.GetCharmIDByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetCharmAvailable(c.Context(), charmUUID)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.ResolveCharmDownload(c.Context(), charmUUID, application.ResolvedCharmDownload{
		LXDProfile:      []byte("profile"),
		ObjectStoreUUID: objectStoreUUID,
		ArchivePath:     "archive",
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.CharmAlreadyResolved)
}

func (s *applicationStateSuite) TestResolveCharmDownloadNotFound(c *tc.C) {
	s.createIAASApplication(c, "foo", life.Alive)

	objectStoreUUID := s.createObjectStoreBlob(c, "archive")

	err := s.state.ResolveCharmDownload(c.Context(), "foo", application.ResolvedCharmDownload{
		LXDProfile:      []byte("profile"),
		ObjectStoreUUID: objectStoreUUID,
		ArchivePath:     "archive",
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *applicationStateSuite) TestGetAsyncCharmDownloadInfoLocalCharm(c *tc.C) {
	platform := deployment.Platform{
		Channel:      "22.04/stable",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Risk: deployment.RiskStable,
	}
	ctx := c.Context()

	appID, _, err := s.state.CreateIAASApplication(ctx, "foo", application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Platform: platform,
			Channel:  channel,
			Charm: charm.Charm{
				Metadata: charm.Metadata{
					Name: "foo",
				},
				Manifest:      s.minimalManifest(c),
				ReferenceName: "foo",
				Source:        charm.LocalSource,
				Revision:      42,
			},
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.state.GetAsyncCharmDownloadInfo(c.Context(), appID)
	c.Assert(err, tc.ErrorIs, applicationerrors.CharmProvenanceNotValid)
}

func (s *applicationStateSuite) TestGetApplicationsForRevisionUpdater(c *tc.C) {
	// Create a few applications.
	s.createIAASApplication(c, "foo", life.Alive)
	s.createIAASApplicationWithNUnits(c, "bar", life.Alive, 1)

	// Get the applications for the revision updater.
	apps, err := s.state.GetApplicationsForRevisionUpdater(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(apps, tc.DeepEquals, []application.RevisionUpdaterApplication{{
		Name: "foo",
		CharmLocator: charm.CharmLocator{
			Name:         "foo",
			Revision:     42,
			Source:       charm.CharmHubSource,
			Architecture: architecture.AMD64,
		},
		Origin: application.Origin{
			Channel: deployment.Channel{
				Track:  "track",
				Risk:   "stable",
				Branch: "branch",
			},
			Platform: deployment.Platform{
				Channel:      "22.04/stable",
				OSType:       deployment.Ubuntu,
				Architecture: architecture.ARM64,
			},
			Revision: 42,
			ID:       "ident",
		},
		NumUnits: 0,
	}, {
		Name: "bar",
		CharmLocator: charm.CharmLocator{
			Name:         "bar",
			Revision:     42,
			Source:       charm.CharmHubSource,
			Architecture: architecture.AMD64,
		},
		Origin: application.Origin{
			Channel: deployment.Channel{
				Track:  "track",
				Risk:   "stable",
				Branch: "branch",
			},
			Platform: deployment.Platform{
				Channel:      "22.04/stable",
				OSType:       deployment.Ubuntu,
				Architecture: architecture.ARM64,
			},
			Revision: 42,
			ID:       "ident",
		},
		NumUnits: 1,
	}})
}

func (s *applicationStateSuite) TestGetApplicationConfigAndSettings(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := `INSERT INTO application_config (application_uuid, key, value, type_id) VALUES (?, ?, ?, ?)`
		_, err := tx.ExecContext(ctx, stmt, id.String(), "key", "value", 0)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: ptr("value"),
		},
	})
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestGetApplicationConfigAndSettingsWithTrust(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := `INSERT INTO application_config (application_uuid, key, value, type_id) VALUES (?, ?, ?, ?)`
		_, err := tx.ExecContext(ctx, stmt, id.String(), "key", "value", 0)
		if err != nil {
			return err
		}

		stmt = `
INSERT INTO application_setting (application_uuid, trust)
VALUES (?, true)
ON CONFLICT(application_uuid) DO UPDATE SET
	trust = excluded.trust;
`
		_, err = tx.ExecContext(ctx, stmt, id.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: ptr("value"),
		},
	})
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{
		Trust: true,
	})
}

func (s *applicationStateSuite) TestGetApplicationConfigAndSettingsForSyntheticCMRApplication(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	// Switch the source_id of a charm to a synthetic CMR charm.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE charm SET source_id = 2, architecture_id = NULL WHERE uuid = (
SELECT charm_uuid FROM application WHERE uuid = ?
)`, id)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	_, _, err = s.state.GetApplicationConfigAndSettings(c.Context(), id)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetApplicationConfigAndSettingsNotFound(c *tc.C) {
	// If the application is not found, it should return application not found.
	id := applicationtesting.GenApplicationUUID(c)
	_, _, err := s.state.GetApplicationConfigAndSettings(c.Context(), id)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetApplicationConfigAndSettingsNoConfig(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	// If there is no config, we should always return the trust. This comes
	// from the application_setting table.

	config, settings, err := s.state.GetApplicationConfigAndSettings(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.HasLen, 0)
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestGetApplicationConfigAndSettingsForApplications(c *tc.C) {
	id0 := s.createIAASApplication(c, "foo", life.Alive)
	id1 := s.createIAASApplication(c, "bar", life.Alive)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := `INSERT INTO application_config (application_uuid, key, value, type_id) VALUES (?, ?, ?, ?)`
		if _, err := tx.ExecContext(ctx, stmt, id0.String(), "a", "b", 0); err != nil {
			return err
		}
		stmt = `INSERT INTO application_config (application_uuid, key, value, type_id) VALUES (?, ?, ?, ?)`
		if _, err := tx.ExecContext(ctx, stmt, id0.String(), "c", "d", 2); err != nil {
			return err
		}
		stmt = `INSERT INTO application_config (application_uuid, key, value, type_id) VALUES (?, ?, ?, ?)`
		if _, err := tx.ExecContext(ctx, stmt, id1.String(), "e", "f", 1); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(c.Context(), id0)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, map[string]application.ApplicationConfig{
		"a": {
			Type:  charm.OptionString,
			Value: ptr("b"),
		},
		"c": {
			Type:  charm.OptionFloat,
			Value: ptr("d"),
		},
	})
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{})

	config, settings, err = s.state.GetApplicationConfigAndSettings(c.Context(), id1)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, map[string]application.ApplicationConfig{
		"e": {
			Type:  charm.OptionInt,
			Value: ptr("f"),
		},
	})
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestGetApplicationConfigWithDefaults(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	s.insertApplicationConfigWithDefault(c, id, "key1", "value1", "defaultValue1", charm.OptionString)
	s.insertCharmConfig(c, id, "key2", "defaultValue2", charm.OptionString)

	config, err := s.state.GetApplicationConfigWithDefaults(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, map[string]application.ApplicationConfig{
		"key1": {
			Type:  charm.OptionString,
			Value: ptr("value1"),
		},
		"key2": {
			Type:  charm.OptionString,
			Value: ptr("defaultValue2"),
		},
	})
}

func (s *applicationStateSuite) TestGetApplicationConfigWithDefaultsNotFound(c *tc.C) {
	// If the application is not found, it should return application not found.
	id := applicationtesting.GenApplicationUUID(c)
	_, err := s.state.GetApplicationConfigWithDefaults(c.Context(), id)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetApplicationConfigWithDefaultsNoConfig(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	// If there is no config, we should always return the trust. This comes
	// from the application_setting table.

	config, err := s.state.GetApplicationConfigWithDefaults(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.HasLen, 0)
}

func (s *applicationStateSuite) TestGetApplicationTrustSetting(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := `INSERT INTO application_config (application_uuid, key, value, type_id) VALUES (?, ?, ?, ?)`
		_, err := tx.ExecContext(ctx, stmt, id.String(), "key", "value", 0)
		if err != nil {
			return err
		}

		stmt = `
INSERT INTO application_setting (application_uuid, trust)
VALUES (?, true)
ON CONFLICT(application_uuid) DO UPDATE SET
	trust = excluded.trust;
`
		_, err = tx.ExecContext(ctx, stmt, id.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	trust, err := s.state.GetApplicationTrustSetting(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(trust, tc.IsTrue)
}

func (s *applicationStateSuite) TestGetApplicationTrustSettingNoRow(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := `INSERT INTO application_config (application_uuid, key, value, type_id) VALUES (?, ?, ?, ?)`
		_, err := tx.ExecContext(ctx, stmt, id.String(), "key", "value", 0)
		if err != nil {
			return err
		}
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	trust, err := s.state.GetApplicationTrustSetting(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(trust, tc.IsFalse)
}

func (s *applicationStateSuite) TestGetApplicationTrustSettingNoApplication(c *tc.C) {
	// If the application is not found, it should return application not found.
	id := applicationtesting.GenApplicationUUID(c)
	_, err := s.state.GetApplicationTrustSetting(c.Context(), id)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetApplicationConfigHash(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	// No config, so the hash should just be the trust value.

	hash, err := s.state.GetApplicationConfigHash(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(hash, tc.Equals, "fcbcf165908dd18a9e49f7ff27810176db8e9f63b4352213741664245224f8aa")
}

func (s *applicationStateSuite) TestGetApplicationConfigHashNotFound(c *tc.C) {
	id := applicationtesting.GenApplicationUUID(c)
	_, err := s.state.GetApplicationConfigHash(c.Context(), id)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestUpdateApplicationConfigAndSettingsNoApplication(c *tc.C) {
	// If the application is not found, it should return application not found.
	id := applicationtesting.GenApplicationUUID(c)
	err := s.state.UpdateApplicationConfigAndSettings(c.Context(), id, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: ptr("value"),
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestUpdateApplicationConfigAndSettingsApplicationIsDead(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Dead)

	err := s.state.UpdateApplicationConfigAndSettings(c.Context(), id, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: ptr("value"),
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationIsDead)
}

func (s *applicationStateSuite) TestUpdateApplicationConfigAndSettingsNoop(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.state.UpdateApplicationConfigAndSettings(c.Context(), id, map[string]application.ApplicationConfig{}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, tc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, map[string]application.ApplicationConfig{})
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestUpdateApplicationConfigAndSettings(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.state.UpdateApplicationConfigAndSettings(c.Context(), id, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: ptr("value"),
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, tc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: ptr("value"),
		},
	})
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{})

	sha256, err := s.state.GetApplicationConfigHash(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sha256, tc.Equals, "6e1b3adca7459d700abb8e270b06ee7fc96f83436bb533ad4540a3a6eb66cf1b")
}

func (s *applicationStateSuite) TestUpdateApplicationConfigAndSettingsMultipleConfigOptions(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.state.UpdateApplicationConfigAndSettings(c.Context(), id, map[string]application.ApplicationConfig{
		"foo": {
			Type:  charm.OptionString,
			Value: ptr("bar"),
		},
		"doink": {
			Type:  charm.OptionInt,
			Value: ptr("17"),
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, tc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, map[string]application.ApplicationConfig{
		"foo": {
			Type:  charm.OptionString,
			Value: ptr("bar"),
		},
		"doink": {
			Type:  charm.OptionInt,
			Value: ptr("17"),
		},
	})
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestUpdateApplicationConfigAndSettingsChangesIdempotent(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.state.UpdateApplicationConfigAndSettings(c.Context(), id, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: ptr("value"),
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.UpdateApplicationConfigAndSettings(c.Context(), id, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: ptr("value"),
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, tc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: ptr("value"),
		},
	})
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestUpdateApplicationConfigAndSettingsMerges(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.state.UpdateApplicationConfigAndSettings(c.Context(), id, map[string]application.ApplicationConfig{
		"foo": {
			Type:  charm.OptionString,
			Value: ptr("bar"),
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, tc.ErrorIsNil)

	sha256, err := s.state.GetApplicationConfigHash(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sha256, tc.Equals, "3fe07426e3e5c57aa18fc4a3d7e412ee31ea150e71d343fbcbe3a406350d3297")

	err = s.state.UpdateApplicationConfigAndSettings(c.Context(), id, map[string]application.ApplicationConfig{
		"bar": {
			Type:  charm.OptionString,
			Value: ptr("foo"),
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, tc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, map[string]application.ApplicationConfig{
		"foo": {
			Type:  charm.OptionString,
			Value: ptr("bar"),
		},
		"bar": {
			Type:  charm.OptionString,
			Value: ptr("foo"),
		},
	})
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{})

	sha256, err = s.state.GetApplicationConfigHash(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sha256, tc.Equals, "8324209a0e1897b4d1f56e4f4b172af181496d377ceef179362999720148841e")
}

func (s *applicationStateSuite) TestUpdateApplicationConfigAndSettingsOverwritesIfSet(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.state.UpdateApplicationConfigAndSettings(c.Context(), id, map[string]application.ApplicationConfig{
		"foo": {
			Type:  charm.OptionString,
			Value: ptr("bar"),
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.UpdateApplicationConfigAndSettings(c.Context(), id, map[string]application.ApplicationConfig{
		"foo": {
			Type:  charm.OptionString,
			Value: ptr("baz"),
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, tc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, map[string]application.ApplicationConfig{
		"foo": {
			Type:  charm.OptionString,
			Value: ptr("baz"),
		},
	})
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestUpdateApplicationConfigAndSettingsupdatesTrust(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.state.UpdateApplicationConfigAndSettings(c.Context(), id, map[string]application.ApplicationConfig{},
		application.UpdateApplicationSettingsArg{
			Trust: ptr(true),
		})
	c.Assert(err, tc.ErrorIsNil)

	_, settings, err := s.state.GetApplicationConfigAndSettings(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{Trust: true})

	// Follow up by checking a nil value does not change the setting.

	err = s.state.UpdateApplicationConfigAndSettings(c.Context(), id, map[string]application.ApplicationConfig{},
		application.UpdateApplicationSettingsArg{
			Trust: nil,
		})
	c.Assert(err, tc.ErrorIsNil)

	_, settings, err = s.state.GetApplicationConfigAndSettings(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{Trust: true})
}

func (s *applicationStateSuite) TestUnsetApplicationConfigKeys(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.state.UpdateApplicationConfigAndSettings(c.Context(), id, map[string]application.ApplicationConfig{
		"a": {
			Type:  charm.OptionString,
			Value: ptr("b"),
		},
		"c": {
			Type:  charm.OptionString,
			Value: ptr("d1"),
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.UnsetApplicationConfigKeys(c.Context(), id, []string{"a"})
	c.Assert(err, tc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, map[string]application.ApplicationConfig{
		"c": {
			Type:  charm.OptionString,
			Value: ptr("d1"),
		},
	})
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestUnsetApplicationConfigKeysApplicationNotFound(c *tc.C) {
	// If the application is not found, it should return application not found.
	id := applicationtesting.GenApplicationUUID(c)
	err := s.state.UnsetApplicationConfigKeys(c.Context(), id, []string{"a"})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestUnsetApplicationConfigKeysIncludingTrust(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.state.UpdateApplicationConfigAndSettings(c.Context(), id,
		map[string]application.ApplicationConfig{},
		application.UpdateApplicationSettingsArg{Trust: ptr(true)},
	)
	c.Assert(err, tc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.HasLen, 0)
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{
		Trust: true,
	})

	err = s.state.UnsetApplicationConfigKeys(c.Context(), id, []string{"a", "trust"})
	c.Assert(err, tc.ErrorIsNil)

	config, settings, err = s.state.GetApplicationConfigAndSettings(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, map[string]application.ApplicationConfig{})
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestUnsetApplicationConfigKeysIgnoredKeys(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.state.UpdateApplicationConfigAndSettings(c.Context(), id, map[string]application.ApplicationConfig{
		"a": {
			Type:  charm.OptionString,
			Value: ptr("b"),
		},
		"c": {
			Type:  charm.OptionString,
			Value: ptr("d1"),
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.UnsetApplicationConfigKeys(c.Context(), id, []string{"a", "x", "y"})
	c.Assert(err, tc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, map[string]application.ApplicationConfig{
		"c": {
			Type:  charm.OptionString,
			Value: ptr("d1"),
		},
	})
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestGetCharmConfigByApplicationID(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	cid, err := s.state.GetCharmIDByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := `INSERT INTO charm_config (charm_uuid, key, default_value, type_id) VALUES (?, ?, ?, ?)`
		_, err := tx.ExecContext(ctx, stmt, cid.String(), "key", "value", 0)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	charmID, config, err := s.state.GetCharmConfigByApplicationID(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(charmID, tc.Equals, cid)
	c.Check(config, tc.DeepEquals, charm.Config{
		Options: map[string]charm.Option{
			"key": {
				Type:    charm.OptionString,
				Default: "value",
			},
		},
	})
}

func (s *applicationStateSuite) TestGetCharmConfigByApplicationIDSyntheticCMRApplication(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	// Switch the source_id of a charm to a synthetic CMR charm.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE charm SET source_id = 2, architecture_id = NULL WHERE uuid = (
SELECT charm_uuid FROM application WHERE uuid = ?
)`, id)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.state.GetCharmIDByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetCharmConfigByApplicationIDApplicationNotFound(c *tc.C) {
	// If the application is not found, it should return application not found.
	id := applicationtesting.GenApplicationUUID(c)
	_, _, err := s.state.GetCharmConfigByApplicationID(c.Context(), id)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestCheckApplicationCharm(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	cid, err := s.state.GetCharmIDByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.checkApplicationCharm(c.Context(), tx, applicationID{ID: id}, charmID{UUID: cid})
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationStateSuite) TestCheckApplicationCharmDifferentCharm(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.checkApplicationCharm(c.Context(), tx, applicationID{ID: id}, charmID{UUID: "other"})
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationHasDifferentCharm)
}

func (s *applicationStateSuite) TestGetApplicationName(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	name, err := s.state.GetApplicationName(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(name, tc.Equals, "foo")
}

func (s *applicationStateSuite) TestGetApplicationNameNotFound(c *tc.C) {
	_, err := s.state.GetApplicationName(c.Context(), applicationtesting.GenApplicationUUID(c))
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetApplicationIDByName(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	gotID, err := s.state.GetApplicationIDByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotID, tc.Equals, id)
}

func (s *applicationStateSuite) TestGetApplicationIDByNameNotFound(c *tc.C) {
	_, err := s.state.GetApplicationIDByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestHashConfigAndSettings(c *tc.C) {
	tests := []struct {
		name     string
		config   []applicationConfig
		settings applicationSettings
		expected string
	}{{
		name:     "empty",
		config:   []applicationConfig{},
		settings: applicationSettings{},
		expected: "fcbcf165908dd18a9e49f7ff27810176db8e9f63b4352213741664245224f8aa",
	}, {
		name: "config",
		config: []applicationConfig{
			{
				Key:   "key",
				Type:  "string",
				Value: sql.Null[string]{V: "value", Valid: true},
			},
		},
		settings: applicationSettings{},
		expected: "6e1b3adca7459d700abb8e270b06ee7fc96f83436bb533ad4540a3a6eb66cf1b",
	}, {
		name: "multiple config",
		config: []applicationConfig{
			{
				Key:   "key",
				Type:  "string",
				Value: sql.Null[string]{V: "value", Valid: true},
			},
			{
				Key:   "key2",
				Type:  "int",
				Value: sql.Null[string]{V: "42", Valid: true},
			},
			{
				Key:   "key3",
				Type:  "float",
				Value: sql.Null[string]{V: "3.14", Valid: true},
			},
			{
				Key:   "key4",
				Type:  "boolean",
				Value: sql.Null[string]{V: "true", Valid: true},
			},
			{
				Key:   "key5",
				Type:  "secret",
				Value: sql.Null[string]{V: "secret", Valid: true},
			},
		},
		settings: applicationSettings{},
		expected: "9b9903f0119ef26b5be2add51497994472dc8810efe2b706e632d1c5eb05c6f7",
	}, {
		name:   "trust",
		config: []applicationConfig{},
		settings: applicationSettings{
			Trust: true,
		},
		expected: "b5bea41b6c623f7c09f1bf24dcae58ebab3c0cdd90ad966bc43a45b44867e12b",
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.name)
		hash, err := hashConfigAndSettings(test.config, test.settings)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(hash, tc.Equals, test.expected)
	}
}

func (s *applicationStateSuite) TestConstraintFull(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		addConstraintStmt := `INSERT INTO "constraint" (uuid, arch, cpu_cores, cpu_power, mem, root_disk, root_disk_source, instance_role, instance_type, container_type_id, virt_type, allocate_public_ip, image_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
		_, err := tx.ExecContext(ctx, addConstraintStmt, "constraint-uuid", "amd64", 2, 42, 8, 256, "root-disk-source", "instance-role", "instance-type", 1, "virt-type", true, "image-id")
		if err != nil {
			return err
		}

		addTagConsStmt := `INSERT INTO constraint_tag (constraint_uuid, tag) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, addTagConsStmt, "constraint-uuid", "tag0")
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, addTagConsStmt, "constraint-uuid", "tag1")
		if err != nil {
			return err
		}
		addSpaceStmt := `INSERT INTO space (uuid, name) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, addSpaceStmt, "space0-uuid", "space0")
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, addSpaceStmt, "space1-uuid", "space1")
		if err != nil {
			return err
		}
		addSpaceConsStmt := `INSERT INTO constraint_space (constraint_uuid, space, exclude) VALUES (?, ?, ?)`
		_, err = tx.ExecContext(ctx, addSpaceConsStmt, "constraint-uuid", "space0", false)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, addSpaceConsStmt, "constraint-uuid", "space1", true)
		if err != nil {
			return err
		}
		addZoneConsStmt := `INSERT INTO constraint_zone (constraint_uuid, zone) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, addZoneConsStmt, "constraint-uuid", "zone0")
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, addZoneConsStmt, "constraint-uuid", "zone1")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetApplicationConstraints(c.Context(), id, constraints.Constraints{
		AllocatePublicIP: ptr(true),
		Arch:             ptr("amd64"),
		CpuCores:         ptr(uint64(2)),
		CpuPower:         ptr(uint64(42)),
		Mem:              ptr(uint64(8)),
		ImageID:          ptr("image-id"),
		Tags:             ptr([]string{"tag0", "tag1"}),
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "space0", Exclude: false},
			{SpaceName: "space1", Exclude: true},
		}),
		Zones:          ptr([]string{"zone0", "zone1"}),
		RootDisk:       ptr(uint64(256)),
		RootDiskSource: ptr("root-disk-source"),
		InstanceRole:   ptr("instance-role"),
		InstanceType:   ptr("instance-type"),
		Container:      ptr(instance.LXD),
		VirtType:       ptr("virt-type"),
	})
	c.Assert(err, tc.ErrorIsNil)

	cons, err := s.state.GetApplicationConstraints(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*cons.Tags, tc.SameContents, []string{"tag0", "tag1"})
	c.Check(*cons.Spaces, tc.SameContents, []constraints.SpaceConstraint{
		{SpaceName: "space0", Exclude: false},
		{SpaceName: "space1", Exclude: true},
	})
	c.Check(*cons.Zones, tc.SameContents, []string{"zone0", "zone1"})
	c.Check(cons.Arch, tc.DeepEquals, ptr("amd64"))
	c.Check(cons.CpuCores, tc.DeepEquals, ptr(uint64(2)))
	c.Check(cons.CpuPower, tc.DeepEquals, ptr(uint64(42)))
	c.Check(cons.Mem, tc.DeepEquals, ptr(uint64(8)))
	c.Check(cons.RootDisk, tc.DeepEquals, ptr(uint64(256)))
	c.Check(cons.RootDiskSource, tc.DeepEquals, ptr("root-disk-source"))
	c.Check(cons.InstanceRole, tc.DeepEquals, ptr("instance-role"))
	c.Check(cons.InstanceType, tc.DeepEquals, ptr("instance-type"))
	c.Check(cons.Container, tc.DeepEquals, ptr(instance.LXD))
	c.Check(cons.VirtType, tc.DeepEquals, ptr("virt-type"))
	c.Check(cons.AllocatePublicIP, tc.DeepEquals, ptr(true))
	c.Check(cons.ImageID, tc.DeepEquals, ptr("image-id"))
}

func (s *applicationStateSuite) TestConstraintPartial(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.state.SetApplicationConstraints(c.Context(), id, constraints.Constraints{
		AllocatePublicIP: ptr(true),
		Arch:             ptr("amd64"),
		CpuCores:         ptr(uint64(2)),
		ImageID:          ptr("image-id"),
	})
	c.Assert(err, tc.ErrorIsNil)

	cons, err := s.state.GetApplicationConstraints(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cons, tc.DeepEquals, constraints.Constraints{
		Arch:             ptr("amd64"),
		CpuCores:         ptr(uint64(2)),
		AllocatePublicIP: ptr(true),
		ImageID:          ptr("image-id"),
	})
}

func (s *applicationStateSuite) TestConstraintSingleValue(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.state.SetApplicationConstraints(c.Context(), id, constraints.Constraints{
		CpuCores: ptr(uint64(2)),
	})
	c.Assert(err, tc.ErrorIsNil)

	cons, err := s.state.GetApplicationConstraints(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cons, tc.DeepEquals, constraints.Constraints{
		CpuCores: ptr(uint64(2)),
	})
}

func (s *applicationStateSuite) TestConstraintEmpty(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	cons, err := s.state.GetApplicationConstraints(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cons, tc.DeepEquals, constraints.Constraints{})
}

func (s *applicationStateSuite) TestConstraintsApplicationNotFound(c *tc.C) {
	_, err := s.state.GetApplicationConstraints(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestSetConstraintFull(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	cons := constraints.Constraints{
		Arch:             ptr("amd64"),
		CpuCores:         ptr(uint64(2)),
		CpuPower:         ptr(uint64(42)),
		Mem:              ptr(uint64(8)),
		RootDisk:         ptr(uint64(256)),
		RootDiskSource:   ptr("root-disk-source"),
		InstanceRole:     ptr("instance-role"),
		InstanceType:     ptr("instance-type"),
		Container:        ptr(instance.LXD),
		VirtType:         ptr("virt-type"),
		AllocatePublicIP: ptr(true),
		ImageID:          ptr("image-id"),
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "space0", Exclude: false},
			{SpaceName: "space1", Exclude: true},
		}),
		Tags:  ptr([]string{"tag0", "tag1"}),
		Zones: ptr([]string{"zone0", "zone1"}),
	}

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		insertSpace0Stmt := `INSERT INTO space (uuid, name) VALUES (?, ?)`
		_, err := tx.ExecContext(ctx, insertSpace0Stmt, "space0-uuid", "space0")
		if err != nil {
			return err
		}
		insertSpace1Stmt := `INSERT INTO space (uuid, name) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, insertSpace1Stmt, "space1-uuid", "space1")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetApplicationConstraints(c.Context(), id, cons)
	c.Assert(err, tc.ErrorIsNil)

	type applicationSpace struct {
		SpaceName    string `db:"space"`
		SpaceExclude bool   `db:"exclude"`
	}
	var (
		applicationUUID                                                     string
		constraintUUID                                                      string
		constraintSpaces                                                    []applicationSpace
		constraintTags                                                      []string
		constraintZones                                                     []string
		arch, rootDiskSource, instanceRole, instanceType, virtType, imageID string
		cpuCores, cpuPower, mem, rootDisk, containerTypeID                  int
		allocatePublicIP                                                    bool
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT application_uuid, constraint_uuid FROM application_constraint WHERE application_uuid=?", id).Scan(&applicationUUID, &constraintUUID)
		if err != nil {
			return err
		}

		rows, err := tx.QueryContext(ctx, "SELECT space,exclude FROM constraint_space WHERE constraint_uuid=?", constraintUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var space applicationSpace
			if err := rows.Scan(&space.SpaceName, &space.SpaceExclude); err != nil {
				return err
			}
			constraintSpaces = append(constraintSpaces, space)
		}
		if rows.Err() != nil {
			return rows.Err()
		}

		rows, err = tx.QueryContext(ctx, "SELECT tag FROM constraint_tag WHERE constraint_uuid=?", constraintUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var tag string
			if err := rows.Scan(&tag); err != nil {
				return err
			}
			constraintTags = append(constraintTags, tag)
		}
		if rows.Err() != nil {
			return rows.Err()
		}

		rows, err = tx.QueryContext(ctx, "SELECT zone FROM constraint_zone WHERE constraint_uuid=?", constraintUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var zone string
			if err := rows.Scan(&zone); err != nil {
				return err
			}
			constraintZones = append(constraintZones, zone)
		}

		row := tx.QueryRowContext(ctx, "SELECT arch, cpu_cores, cpu_power, mem, root_disk, root_disk_source, instance_role, instance_type, container_type_id, virt_type, allocate_public_ip, image_id FROM \"constraint\" WHERE uuid=?", constraintUUID)
		err = row.Err()
		if err != nil {
			return err
		}
		if err := row.Scan(&arch, &cpuCores, &cpuPower, &mem, &rootDisk, &rootDiskSource, &instanceRole, &instanceType, &containerTypeID, &virtType, &allocatePublicIP, &imageID); err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(constraintUUID, tc.Not(tc.Equals), "")
	c.Check(applicationUUID, tc.Equals, id.String())

	c.Check(arch, tc.Equals, "amd64")
	c.Check(cpuCores, tc.Equals, 2)
	c.Check(cpuPower, tc.Equals, 42)
	c.Check(mem, tc.Equals, 8)
	c.Check(rootDisk, tc.Equals, 256)
	c.Check(rootDiskSource, tc.Equals, "root-disk-source")
	c.Check(instanceRole, tc.Equals, "instance-role")
	c.Check(instanceType, tc.Equals, "instance-type")
	c.Check(containerTypeID, tc.Equals, 1)
	c.Check(virtType, tc.Equals, "virt-type")
	c.Check(allocatePublicIP, tc.Equals, true)
	c.Check(imageID, tc.Equals, "image-id")

	c.Check(constraintSpaces, tc.DeepEquals, []applicationSpace{
		{SpaceName: "space0", SpaceExclude: false},
		{SpaceName: "space1", SpaceExclude: true},
	})
	c.Check(constraintTags, tc.DeepEquals, []string{"tag0", "tag1"})
	c.Check(constraintZones, tc.DeepEquals, []string{"zone0", "zone1"})

}

func (s *applicationStateSuite) TestSetConstraintInvalidContainerType(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	cons := constraints.Constraints{
		Container: ptr(instance.ContainerType("invalid-container-type")),
	}
	err := s.state.SetApplicationConstraints(c.Context(), id, cons)
	c.Assert(err, tc.ErrorIs, applicationerrors.InvalidApplicationConstraints)
}

func (s *applicationStateSuite) TestSetConstraintInvalidSpace(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	cons := constraints.Constraints{
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "invalid-space", Exclude: false},
		}),
	}
	err := s.state.SetApplicationConstraints(c.Context(), id, cons)
	c.Assert(err, tc.ErrorIs, applicationerrors.InvalidApplicationConstraints)
}

func (s *applicationStateSuite) TestSetConstraintsReplacesPrevious(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.state.SetApplicationConstraints(c.Context(), id, constraints.Constraints{
		Mem:      ptr(uint64(8)),
		CpuCores: ptr(uint64(2)),
	})
	c.Assert(err, tc.ErrorIsNil)

	cons, err := s.state.GetApplicationConstraints(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cons, tc.DeepEquals, constraints.Constraints{
		Mem:      ptr(uint64(8)),
		CpuCores: ptr(uint64(2)),
	})

	err = s.state.SetApplicationConstraints(c.Context(), id, constraints.Constraints{
		CpuPower: ptr(uint64(42)),
	})
	c.Assert(err, tc.ErrorIsNil)

	cons, err = s.state.GetApplicationConstraints(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cons, tc.DeepEquals, constraints.Constraints{
		CpuPower: ptr(uint64(42)),
	})
}

func (s *applicationStateSuite) TestSetConstraintsReplacesPreviousZones(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.state.SetApplicationConstraints(c.Context(), id, constraints.Constraints{
		Zones: ptr([]string{"zone0", "zone1"}),
	})
	c.Assert(err, tc.ErrorIsNil)

	cons, err := s.state.GetApplicationConstraints(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*cons.Zones, tc.SameContents, []string{"zone0", "zone1"})

	err = s.state.SetApplicationConstraints(c.Context(), id, constraints.Constraints{
		Tags: ptr([]string{"tag0", "tag1"}),
	})
	c.Assert(err, tc.ErrorIsNil)

	cons, err = s.state.GetApplicationConstraints(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*cons.Tags, tc.SameContents, []string{"tag0", "tag1"})
}

func (s *applicationStateSuite) TestSetConstraintsReplacesPreviousSameZone(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.state.SetApplicationConstraints(c.Context(), id, constraints.Constraints{
		Zones: ptr([]string{"zone0", "zone1"}),
	})
	c.Assert(err, tc.ErrorIsNil)

	cons, err := s.state.GetApplicationConstraints(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*cons.Zones, tc.SameContents, []string{"zone0", "zone1"})

	err = s.state.SetApplicationConstraints(c.Context(), id, constraints.Constraints{
		Zones: ptr([]string{"zone3"}),
	})
	c.Assert(err, tc.ErrorIsNil)

	cons, err = s.state.GetApplicationConstraints(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*cons.Zones, tc.SameContents, []string{"zone3"})
}

func (s *applicationStateSuite) TestSetConstraintsApplicationNotFound(c *tc.C) {
	err := s.state.SetApplicationConstraints(c.Context(), "foo", constraints.Constraints{Mem: ptr(uint64(8))})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetApplicationCharmOriginEmptyChannel(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM application_channel WHERE application_uuid=?", id)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	origin, err := s.state.GetApplicationCharmOrigin(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(origin, tc.DeepEquals, application.CharmOrigin{
		Name:   "foo",
		Source: charm.CharmHubSource,
		Platform: deployment.Platform{
			Channel:      "22.04/stable",
			OSType:       0,
			Architecture: 1,
		},
		Revision:           42,
		Hash:               "hash",
		CharmhubIdentifier: "ident",
	})
}

func (s *applicationStateSuite) TestGetApplicationCharmOriginRiskOnlyChannel(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application_channel SET track = '', branch = '' WHERE application_uuid=?", id)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	origin, err := s.state.GetApplicationCharmOrigin(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(origin, tc.DeepEquals, application.CharmOrigin{
		Name:   "foo",
		Source: charm.CharmHubSource,
		Platform: deployment.Platform{
			Channel:      "22.04/stable",
			OSType:       0,
			Architecture: 1,
		},
		Channel: &deployment.Channel{
			Risk: "stable",
		},
		Revision:           42,
		Hash:               "hash",
		CharmhubIdentifier: "ident",
	})
}

func (s *applicationStateSuite) TestGetApplicationCharmOriginInvalidRisk(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application_channel SET track = '', risk = 'boom', branch = '' WHERE application_uuid=?", id)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.state.GetApplicationCharmOrigin(c.Context(), id)
	c.Assert(err, tc.ErrorMatches, `decoding channel: decoding risk: unknown risk "boom"`)
}

func (s *applicationStateSuite) TestGetApplicationCharmOriginNoRevision(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	charmUUID, err := s.state.GetCharmIDByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE charm SET revision = -1 WHERE uuid=?", charmUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	origin, err := s.state.GetApplicationCharmOrigin(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(origin, tc.DeepEquals, application.CharmOrigin{
		Name:   "foo",
		Source: charm.CharmHubSource,
		Platform: deployment.Platform{
			Channel:      "22.04/stable",
			OSType:       0,
			Architecture: 1,
		},
		Channel: &deployment.Channel{
			Track:  "track",
			Risk:   "stable",
			Branch: "branch",
		},
		Revision:           -1,
		Hash:               "hash",
		CharmhubIdentifier: "ident",
	})
}

func (s *applicationStateSuite) TestGetApplicationCharmOriginNoCharmhubIdentifier(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	charmUUID, err := s.state.GetCharmIDByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE charm_download_info SET charmhub_identifier = NULL WHERE charm_uuid=?", charmUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	origin, err := s.state.GetApplicationCharmOrigin(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(origin, tc.DeepEquals, application.CharmOrigin{
		Name:   "foo",
		Source: charm.CharmHubSource,
		Platform: deployment.Platform{
			Channel:      "22.04/stable",
			OSType:       0,
			Architecture: 1,
		},
		Channel: &deployment.Channel{
			Track:  "track",
			Risk:   "stable",
			Branch: "branch",
		},
		Revision: 42,
		Hash:     "hash",
	})
}

func (s *applicationStateSuite) TestGetDeviceConstraintsAppNotFound(c *tc.C) {
	_, err := s.state.GetDeviceConstraints(c.Context(), coreapplication.ID("foo"))
	c.Assert(err, tc.ErrorMatches, applicationerrors.ApplicationNotFound.Error())
}

func (s *applicationStateSuite) TestGetDeviceConstraintsDeadApp(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Dead)

	_, err := s.state.GetDeviceConstraints(c.Context(), id)
	c.Assert(err, tc.ErrorMatches, applicationerrors.ApplicationIsDead.Error())
}

func (s *applicationStateSuite) TestGetDeviceConstraints(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		insertDeviceConstraint0 := `INSERT INTO device_constraint (uuid, application_uuid, name, type, count) VALUES (?, ?, ?, ?, ?)`
		_, err := tx.ExecContext(ctx, insertDeviceConstraint0, "dev3-uuid", id.String(), "dev3", "type3", 666)
		if err != nil {
			return err
		}

		insertDeviceConstraintAttrs0 := `INSERT INTO device_constraint_attribute (device_constraint_uuid, "key", value) VALUES (?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertDeviceConstraintAttrs0, "dev3-uuid", "k666", "v666")
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	cons, err := s.state.GetDeviceConstraints(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cons, tc.HasLen, 4)
	// Device constraint added by createIAASApplication().
	c.Check(cons["dev0"].Type, tc.Equals, devices.DeviceType("type0"))
	c.Check(cons["dev0"].Count, tc.Equals, 42)
	c.Check(cons["dev0"].Attributes, tc.DeepEquals, map[string]string{
		"k0": "v0",
		"k1": "v1",
	})
	c.Check(cons["dev1"].Type, tc.Equals, devices.DeviceType("type1"))
	c.Check(cons["dev1"].Count, tc.Equals, 3)
	c.Check(cons["dev1"].Attributes, tc.DeepEquals, map[string]string{"k2": "v2"})
	c.Check(cons["dev2"].Type, tc.Equals, devices.DeviceType("type2"))
	c.Check(cons["dev2"].Count, tc.Equals, 1974)
	c.Check(cons["dev2"].Attributes, tc.DeepEquals, map[string]string{})
	// Device constraint added manually via inserts.
	c.Check(cons["dev3"].Type, tc.Equals, devices.DeviceType("type3"))
	c.Check(cons["dev3"].Count, tc.Equals, 666)
	c.Check(cons["dev3"].Attributes, tc.DeepEquals, map[string]string{"k666": "v666"})
}

func (s *applicationStateSuite) TestGetDeviceConstraintsFromCreatedApp(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	cons, err := s.state.GetDeviceConstraints(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cons, tc.HasLen, 3)
	c.Check(cons["dev0"].Type, tc.Equals, devices.DeviceType("type0"))
	c.Check(cons["dev0"].Count, tc.Equals, 42)
	c.Check(cons["dev0"].Attributes, tc.DeepEquals, map[string]string{
		"k0": "v0",
		"k1": "v1",
	})
	c.Check(cons["dev1"].Type, tc.Equals, devices.DeviceType("type1"))
	c.Check(cons["dev1"].Count, tc.Equals, 3)
	c.Check(cons["dev1"].Attributes, tc.DeepEquals, map[string]string{"k2": "v2"})
	c.Check(cons["dev2"].Type, tc.Equals, devices.DeviceType("type2"))
	c.Check(cons["dev2"].Count, tc.Equals, 1974)
	c.Check(cons["dev2"].Attributes, tc.DeepEquals, map[string]string{})
}

func (s *applicationStateSuite) TestGetAddressesHashEmpty(c *tc.C) {
	appID, _ := s.createIAASApplicationWithNUnits(c, "foo", life.Alive, 1)

	hash, err := s.state.GetAddressesHash(c.Context(), appID, "net-node-uuid")
	c.Assert(err, tc.ErrorIsNil)
	// The resulting hash is not the empty string because it always contains
	// the default bindings.
	c.Check(hash, tc.Equals, "8652c267aea387455356c3dc0edf896cab692e6a3db590197e7bec120bdfe234")
}

func (s *applicationStateSuite) TestGetAddressesHash(c *tc.C) {
	appID, _ := s.createIAASApplicationWithNUnits(c, "foo", life.Alive, 1)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		insertNetNode := `INSERT INTO net_node (uuid) VALUES (?)`
		_, err := tx.ExecContext(ctx, insertNetNode, "net-node-uuid")
		if err != nil {
			return err
		}
		updateUnit := `UPDATE unit SET net_node_uuid = ? WHERE name = ?`
		_, err = tx.ExecContext(ctx, updateUnit, "net-node-uuid", "foo/0")
		if err != nil {
			return err
		}
		insertLLD := `INSERT INTO link_layer_device (uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id) VALUES (?, ?, ?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertLLD, "lld-uuid", "net-node-uuid", "lld-name", 1500, "00:11:22:33:44:55", 0, 0)
		if err != nil {
			return err
		}
		insertSpace := `INSERT INTO space (uuid, name) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, insertSpace, "space0-uuid", "space0")
		if err != nil {
			return err
		}
		insertSubnet := `INSERT INTO subnet (uuid, cidr, space_uuid) VALUES (?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertSubnet, "subnet-uuid", "10.0.0.0/24", "space0-uuid")
		if err != nil {
			return err
		}
		insertIPAddress := `INSERT INTO ip_address (uuid, device_uuid, address_value, net_node_uuid, type_id, scope_id, origin_id, config_type_id, subnet_uuid) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertIPAddress, "ip-address-uuid", "lld-uuid", "10.0.0.1", "net-node-uuid", 0, 0, 0, 0, "subnet-uuid")
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	hash, err := s.state.GetAddressesHash(c.Context(), appID, "net-node-uuid")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(hash, tc.Equals, "740b8e5ae149e6d2e3d962e2d0f7edca886ab2d16bd6e2374eb6b9bdfa9d2850")
}

func (s *applicationStateSuite) TestGetAddressesHashWithEndpointBindings(c *tc.C) {
	appID, _ := s.createIAASApplicationWithNUnits(c, "foo", life.Alive, 1)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		insertNetNode := `INSERT INTO net_node (uuid) VALUES (?)`
		_, err := tx.ExecContext(ctx, insertNetNode, "net-node-uuid")
		if err != nil {
			return err
		}
		updateUnit := `UPDATE unit SET net_node_uuid = ? WHERE name = ?`
		_, err = tx.ExecContext(ctx, updateUnit, "net-node-uuid", "foo/0")
		if err != nil {
			return err
		}
		insertLLD := `INSERT INTO link_layer_device (uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id) VALUES (?, ?, ?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertLLD, "lld-uuid", "net-node-uuid", "lld-name", 1500, "00:11:22:33:44:55", 0, 0)
		if err != nil {
			return err
		}
		insertSpace := `INSERT INTO space (uuid, name) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, insertSpace, "space0-uuid", "space0")
		if err != nil {
			return err
		}
		insertSubnet := `INSERT INTO subnet (uuid, cidr, space_uuid) VALUES (?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertSubnet, "subnet-uuid", "10.0.0.0/24", "space0-uuid")
		if err != nil {
			return err
		}
		insertIPAddress := `INSERT INTO ip_address (uuid, device_uuid, address_value, net_node_uuid, type_id, scope_id, origin_id, config_type_id, subnet_uuid) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertIPAddress, "ip-address-uuid", "lld-uuid", "10.0.0.1", "net-node-uuid", 0, 0, 0, 0, "subnet-uuid")
		if err != nil {
			return err
		}

		insertCharm := `INSERT INTO charm (uuid, reference_name) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, insertCharm, "charm0-uuid", "foo-charm")
		if err != nil {
			return err
		}
		insertCharmRelation := `INSERT INTO charm_relation (uuid, charm_uuid,  scope_id, role_id, name) VALUES (?, ?, ?, ?, ?)`
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
	c.Assert(err, tc.ErrorIsNil)

	hash, err := s.state.GetAddressesHash(c.Context(), appID, "net-node-uuid")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(hash, tc.Equals, "5e5d6453be08912c0cb0585e9d39e6fe21e154c0495c7f05b61137e7f3eab381")
}

func (s *applicationStateSuite) TestGetAddressesHashCloudService(c *tc.C) {
	appID := s.createCAASApplication(c, "foo", life.Alive)

	network.NewMachineAddress("10.0.0.1/24")
	err := s.state.UpsertCloudService(c.Context(), "foo", "provider-id", network.ProviderAddresses{
		{
			MachineAddress: network.NewMachineAddress("10.0.0.1/24"),
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	var netNodeUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT net_node_uuid FROM k8s_service WHERE application_uuid=?", appID).Scan(&netNodeUUID)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	hash, err := s.state.GetAddressesHash(c.Context(), appID, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(hash, tc.Equals, "6e97876f0c817d2ba3b4d736f3fceb639049997e609803028673eeaeeaa01cf5")
}

func (s *applicationStateSuite) TestGetAddressesHashCloudServiceWithEndpointBindings(c *tc.C) {
	appID := s.createCAASApplication(c, "foo", life.Alive)
	err := s.state.UpsertCloudService(c.Context(), "foo", "provider-id", network.ProviderAddresses{
		{
			MachineAddress: network.NewMachineAddress("10.0.0.1/24"),
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	var netNodeUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT net_node_uuid FROM k8s_service WHERE application_uuid=?", appID).Scan(&netNodeUUID)
		if err != nil {
			return err
		}

		insertSpace := `INSERT INTO space (uuid, name) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, insertSpace, "space0-uuid", "space0")
		if err != nil {
			return err
		}

		insertCharm := `INSERT INTO charm (uuid, reference_name) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, insertCharm, "charm0-uuid", "foo-charm")
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
	c.Assert(err, tc.ErrorIsNil)

	hash, err := s.state.GetAddressesHash(c.Context(), appID, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(hash, tc.Equals, "55d79355f44aa2d2799338219e2c2d2e67f61d3de026bf0415093d2de9d01afc")
}

func (s *applicationStateSuite) TestHashAddresses(c *tc.C) {
	hash, err := s.state.hashAddressesAndEndpoints(nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(hash, tc.Equals, "")

	hash0, err := s.state.hashAddressesAndEndpoints([]spaceAddress{
		{
			Value: "10.0.0.1",
		},
		{
			Value: "10.0.0.2",
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	hash1, err := s.state.hashAddressesAndEndpoints([]spaceAddress{
		{
			Value: "10.0.0.2",
		},
		{
			Value: "10.0.0.1",
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	// The hash should be consistent regardless of the order of the addresses.
	c.Check(hash0, tc.Equals, hash1)

	hash0, err = s.state.hashAddressesAndEndpoints([]spaceAddress{}, map[string]string{
		"foo": "bar",
		"foz": "baz",
	})
	c.Assert(err, tc.ErrorIsNil)
	hash1, err = s.state.hashAddressesAndEndpoints([]spaceAddress{}, map[string]string{
		"foz": "baz",
		"foo": "bar",
	})
	c.Assert(err, tc.ErrorIsNil)
	// The hash should be consistent regardless of the order of the endpoint
	// bindings.
	c.Check(hash0, tc.Equals, hash1)
}

func (s *applicationStateSuite) TestGetNetNodeFromK8sService(c *tc.C) {
	unitName, unitUUID := s.createNamedCAASUnit(c)

	err := s.state.UpsertCloudService(c.Context(), "foo", "provider-id", network.ProviderAddresses{
		{
			MachineAddress: network.NewMachineAddress("10.0.0.1/8"),
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	// Also insert the unit net node to make sure the k8s service one is
	// returned.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		insertNetNode := `INSERT INTO net_node (uuid) VALUES (?)`
		_, err := tx.ExecContext(ctx, insertNetNode, "net-node-uuid")
		if err != nil {
			return err
		}
		updateUnit := `UPDATE unit SET net_node_uuid = ? WHERE uuid = ?`
		_, err = tx.ExecContext(ctx, updateUnit, "net-node-uuid", unitUUID.String())
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	// Check the k8s service net node is returned (since the uuid is generated
	// we check that the unit net node uuid, which is manually crafted, is not
	// returned).
	netNode, err := s.state.GetNetNodeUUIDByUnitName(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(netNode, tc.Not(tc.Equals), "net-node-uuid")
}

func (s *applicationStateSuite) TestGetNetNodeFromUnit(c *tc.C) {
	unitName, unitUUID := s.createNamedIAASUnit(c)
	expectedNetNodeUUID := "net-node-uuid"

	// Insert the unit net node to make sure the k8s service one is
	// returned.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		insertNetNode := `INSERT INTO net_node (uuid) VALUES (?)`
		_, err := tx.ExecContext(ctx, insertNetNode, expectedNetNodeUUID)
		if err != nil {
			return err
		}
		updateUnit := `UPDATE unit SET net_node_uuid = ? WHERE uuid = ?`
		_, err = tx.ExecContext(ctx, updateUnit, expectedNetNodeUUID, unitUUID.String())
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	// Check the unit net node is returned.
	netNode, err := s.state.GetNetNodeUUIDByUnitName(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(netNode, tc.Equals, expectedNetNodeUUID)
}

func (s *applicationStateSuite) TestGetNetNodeUnitNotFound(c *tc.C) {
	_, err := s.state.GetNetNodeUUIDByUnitName(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationStateSuite) TestShouldAllowCharmUpgradeOnError(c *tc.C) {
	appUUID := s.createIAASApplication(c, "foo", life.Alive)
	s.setCharmUpgradeOnError(c, appUUID, true)
	v, err := s.state.ShouldAllowCharmUpgradeOnError(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(v, tc.IsTrue)
}

func (s *applicationStateSuite) TestShouldAllowCharmUpgradeOnErrorFalse(c *tc.C) {
	appUUID := s.createIAASApplication(c, "foo", life.Alive)
	s.setCharmUpgradeOnError(c, appUUID, false)
	v, err := s.state.ShouldAllowCharmUpgradeOnError(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(v, tc.IsFalse)
}

func (s *applicationStateSuite) TestShouldAllowCharmUpgradeOnErrorNotFound(c *tc.C) {
	_, err := s.state.ShouldAllowCharmUpgradeOnError(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) setCharmUpgradeOnError(c *tc.C, appUUID coreapplication.ID, v bool) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
UPDATE application
SET    charm_upgrade_on_error = ?
WHERE  uuid = ?
`, v, appUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationStateSuite) assertIAASApplication(
	c *tc.C,
	name string,
	platform deployment.Platform,
	channel *deployment.Channel,
	available bool,
) {
	var (
		gotName      string
		gotUUID      string
		gotCharmUUID string
		gotPlatform  deployment.Platform
		gotChannel   deployment.Channel
		gotAvailable bool
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT uuid, charm_uuid, name FROM application WHERE name=?", name).Scan(&gotUUID, &gotCharmUUID, &gotName)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, "SELECT channel, os_id, architecture_id FROM application_platform WHERE application_uuid=?", gotUUID).
			Scan(&gotPlatform.Channel, &gotPlatform.OSType, &gotPlatform.Architecture)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, "SELECT track, risk, branch FROM application_channel WHERE application_uuid=?", gotUUID).
			Scan(&gotChannel.Track, &gotChannel.Risk, &gotChannel.Branch)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		err = tx.QueryRowContext(ctx, "SELECT available FROM charm WHERE uuid=?", gotCharmUUID).Scan(&gotAvailable)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotName, tc.Equals, name)
	c.Check(gotPlatform, tc.DeepEquals, platform)
	c.Check(gotAvailable, tc.Equals, available)

	// Channel is optional, so we need to check it separately.
	if channel != nil {
		c.Check(gotChannel, tc.DeepEquals, *channel)
	} else {
		// Ensure it's empty if the original origin channel isn't set.
		// Prevent the db from sending back bogus values.
		c.Check(gotChannel, tc.DeepEquals, deployment.Channel{})
	}
}

func (s *applicationStateSuite) assertCAASApplication(
	c *tc.C,
	name string,
	platform deployment.Platform,
	channel *deployment.Channel,
	scale application.ScaleState,
	available bool,
) {
	var (
		gotName      string
		gotUUID      string
		gotCharmUUID string
		gotPlatform  deployment.Platform
		gotChannel   deployment.Channel
		gotScale     application.ScaleState
		gotAvailable bool
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT uuid, charm_uuid, name FROM application WHERE name=?", name).Scan(&gotUUID, &gotCharmUUID, &gotName)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, "SELECT scale, scaling, scale_target FROM application_scale WHERE application_uuid=?", gotUUID).
			Scan(&gotScale.Scale, &gotScale.Scaling, &gotScale.ScaleTarget)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, "SELECT channel, os_id, architecture_id FROM application_platform WHERE application_uuid=?", gotUUID).
			Scan(&gotPlatform.Channel, &gotPlatform.OSType, &gotPlatform.Architecture)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, "SELECT track, risk, branch FROM application_channel WHERE application_uuid=?", gotUUID).
			Scan(&gotChannel.Track, &gotChannel.Risk, &gotChannel.Branch)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		err = tx.QueryRowContext(ctx, "SELECT available FROM charm WHERE uuid=?", gotCharmUUID).Scan(&gotAvailable)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotName, tc.Equals, name)
	c.Check(gotPlatform, tc.DeepEquals, platform)
	c.Check(gotScale, tc.DeepEquals, scale)
	c.Check(gotAvailable, tc.Equals, available)

	// Channel is optional, so we need to check it separately.
	if channel != nil {
		c.Check(gotChannel, tc.DeepEquals, *channel)
	} else {
		// Ensure it's empty if the original origin channel isn't set.
		// Prevent the db from sending back bogus values.
		c.Check(gotChannel, tc.DeepEquals, deployment.Channel{})
	}
}

func (s *applicationStateSuite) addCharmModifiedVersion(c *tc.C, appID coreapplication.ID, charmModifiedVersion int) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application SET charm_modified_version = ? WHERE uuid = ?", charmModifiedVersion, appID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationStateSuite) insertApplicationConfigWithDefault(c *tc.C, appID coreapplication.ID, key, value, defaultValue string, optionType charm.OptionType) {
	t, err := encodeConfigType(optionType)
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO application_config (application_uuid, key, value, type_id) VALUES (?, ?, ?, ?)
`, appID, key, value, t)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	s.insertCharmConfig(c, appID, key, defaultValue, optionType)
}

func (s *applicationStateSuite) insertCharmConfig(c *tc.C, appID coreapplication.ID, key, defaultValue string, optionType charm.OptionType) {
	t, err := encodeConfigType(optionType)
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_config (charm_uuid, key, default_value, type_id)
SELECT charm_uuid, ?, ?, ?
FROM application
WHERE uuid = ?
`, key, defaultValue, t, appID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) assertPeerRelation(c *tc.C, appName string, peerRelationInput map[string]int) {
	type peerRelation struct {
		id     int
		name   string
		status corestatus.Status
	}
	var expected []peerRelation
	for name, id := range peerRelationInput {
		expected = append(expected, peerRelation{
			id:     id,
			name:   name,
			status: corestatus.Joining,
		})
	}

	var peerRelations []peerRelation
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT cr.name, r.relation_id, rst.name
FROM charm_relation cr
JOIN application_endpoint ae ON ae.charm_relation_uuid = cr.uuid
JOIN application a ON a.uuid = ae.application_uuid
JOIN relation_endpoint re ON  re.endpoint_uuid = ae.uuid
JOIN relation r ON r.uuid = re.relation_uuid
LEFT JOIN relation_status rs ON rs.relation_uuid = re.relation_uuid
LEFT JOIN relation_status_type rst ON rs.relation_status_type_id = rst.id
WHERE a.name = ?
AND cr.role_id = 2 -- peer relation
ORDER BY r.relation_id
`, appName)
		if err != nil {
			return errors.Capture(err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var row peerRelation
			var statusName *corestatus.Status // allows graceful error if status not set
			if err := rows.Scan(&row.name, &row.id, &statusName); err != nil {
				return errors.Capture(err)
			}
			row.status = deptr(statusName)
			peerRelations = append(peerRelations, row)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(peerRelations, tc.SameContents, expected)
}

func (s *applicationStateSuite) checkApplicationCharm(ctx context.Context, tx *sqlair.TX, ident applicationID, charmID charmID) error {
	query := `
SELECT COUNT(*) AS &countResult.count
FROM application
WHERE uuid = $applicationID.uuid
AND charm_uuid = $charmID.uuid;
	`
	stmt, err := sqlair.Prepare(query, countResult{}, ident, charmID)
	if err != nil {
		return errors.Errorf("preparing verification query: %w", err)
	}

	// Ensure that the charm is the same as the one we're trying to set.
	var count countResult
	if err := tx.Query(ctx, stmt, ident, charmID).Get(&count); err != nil {
		return errors.Errorf("verifying charm: %w", err)
	}
	if count.Count == 0 {
		return applicationerrors.ApplicationHasDifferentCharm
	}
	return nil
}
