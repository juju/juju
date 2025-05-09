// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"maps"
	"slices"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/resource/testing"
	"github.com/juju/juju/core/semversion"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/resource"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/status"
	statusstate "github.com/juju/juju/domain/status/state"
	domainstorage "github.com/juju/juju/domain/storage"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type modelSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&modelSuite{})

func (s *modelSuite) TestGetModelType(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, type, cloud, cloud_type)
			VALUES (?, ?, "test", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	mt, err := st.GetModelType(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mt, gc.Equals, coremodel.IAAS)
}

type applicationStateSuite struct {
	baseSuite

	state *State
}

var _ = gc.Suite(&applicationStateSuite{})

func (s *applicationStateSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *applicationStateSuite) TestCreateApplication(c *gc.C) {
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
	ctx := context.Background()

	id, err := s.state.CreateApplication(ctx, "666", application.AddApplicationArg{
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
		Scale:   1,
		Channel: channel,
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, channel, scale, false)

	// Ensure that config is empty and trust is false.
	config, settings, err := s.state.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, gc.HasLen, 0)
	c.Check(settings, gc.DeepEquals, application.ApplicationSettings{Trust: false})

	// Status should be unset.
	statusState := statusstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	sts, err := statusState.GetApplicationStatus(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sts, gc.DeepEquals, status.StatusInfo[status.WorkloadStatusType]{
		Status: status.WorkloadStatusUnset,
	})
}

func (s *applicationStateSuite) TestCreateApplicationWithConfigAndSettings(c *gc.C) {
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
	ctx := context.Background()

	id, err := s.state.CreateApplication(ctx, "666", application.AddApplicationArg{
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
		Scale:   1,
		Channel: channel,
		Config: map[string]application.ApplicationConfig{
			"foo": {
				Value: "bar",
				Type:  charm.OptionString,
			},
		},
		Settings: application.ApplicationSettings{
			Trust: true,
		},
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, channel, scale, false)

	// Ensure that config is empty and trust is false.
	config, settings, err := s.state.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, gc.DeepEquals, map[string]application.ApplicationConfig{
		"foo": {
			Value: "bar",
			Type:  charm.OptionString,
		},
	})
	c.Check(settings, gc.DeepEquals, application.ApplicationSettings{Trust: true})
}

func (s *applicationStateSuite) TestCreateApplicationWithPeerRelation(c *gc.C) {
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
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "666", application.AddApplicationArg{
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
		Scale:   1,
		Channel: channel,
	}, nil)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("Failed to create application: %s", errors.ErrorStack(err)))
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, channel, scale, false)

	s.assertPeerRelation(c, "666", map[string]int{"pollux": 1, "castor": 0})
}

func (s *applicationStateSuite) TestCreateApplicationWithStatus(c *gc.C) {
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
	ctx := context.Background()

	now := time.Now().UTC()
	id, err := s.state.CreateApplication(ctx, "666", application.AddApplicationArg{
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
		Scale:   1,
		Channel: channel,
		Status: &status.StatusInfo[status.WorkloadStatusType]{
			Status:  status.WorkloadStatusActive,
			Message: "test",
			Data:    []byte(`{"foo": "bar"}`),
			Since:   ptr(now),
		},
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, channel, scale, false)

	statusState := statusstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	sts, err := statusState.GetApplicationStatus(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sts, gc.DeepEquals, status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "test",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(now),
	})
}

func (s *applicationStateSuite) TestCreateApplicationWithUnits(c *gc.C) {
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
	a := application.AddApplicationArg{
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
		Scale:   1,
		Channel: channel,
	}
	us := []application.AddUnitArg{{
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
	}}
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "foo", a, us)
	c.Assert(err, jc.ErrorIsNil)
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "foo", platform, channel, scale, false)
}

func (s *applicationStateSuite) TestCreateApplicationsWithSameCharm(c *gc.C) {
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
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "foo1", application.AddApplicationArg{
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
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.CreateApplication(ctx, "foo2", application.AddApplicationArg{
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
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	scale := application.ScaleState{}
	s.assertApplication(c, "foo1", platform, channel, scale, false)
	s.assertApplication(c, "foo2", platform, channel, scale, false)
}

func (s *applicationStateSuite) TestCreateApplicationWithoutChannel(c *gc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "666", application.AddApplicationArg{
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
		Scale: 1,
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, nil, scale, false)
}

func (s *applicationStateSuite) TestCreateApplicationWithEmptyChannel(c *gc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{}
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "666", application.AddApplicationArg{
		Platform: platform,
		Charm: charm.Charm{
			Metadata:      s.minimalMetadata(c, "666"),
			Manifest:      s.minimalManifest(c),
			Source:        charm.LocalSource,
			Revision:      42,
			ReferenceName: "666",
		},
		Scale: 1,
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, channel, scale, false)
}

func (s *applicationStateSuite) TestCreateApplicationWithCharmStoragePath(c *gc.C) {
	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{}
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "666", application.AddApplicationArg{
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
		Scale: 1,
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, channel, scale, true)
}

// TestCreateApplicationWithResolvedResources tests creation of an application with
// specified resources.
// It verifies that the charm_resource table is populated, alongside the
// resource and application_resource table with data from charm and arguments.
func (s *applicationStateSuite) TestCreateApplicationWithResolvedResources(c *gc.C) {
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
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "666", s.addApplicationArgForResources(c, "666",
		charmResources, addResourcesArgs), nil)
	c.Assert(err, jc.ErrorIsNil)
	// Check expected resources are added
	assertTxn := func(comment string, do func(ctx context.Context, tx *sql.Tx) error) {
		err := s.TxnRunner().StdTxn(context.Background(), do)
		c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) %s: %s", comment,
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
		defer rows.Close()
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
	c.Check(foundCharmResources, jc.SameContents, slices.Collect(maps.Values(charmResources)),
		gc.Commentf("(Assert) mismatch between charm resources and inserted resources"))
	c.Check(foundAppAvailableResources, jc.SameContents, addResourcesArgs,
		gc.Commentf("(Assert) mismatch between app available app resources and inserted resources"))
	expectedPotentialResources := make([]application.AddApplicationResourceArg, 0, len(addResourcesArgs))
	for _, res := range addResourcesArgs {
		expectedPotentialResources = append(expectedPotentialResources, application.AddApplicationResourceArg{
			Name:     res.Name,
			Revision: nil,                       // nil revision
			Origin:   charmresource.OriginStore, // origin should always be store
		})
	}
	c.Check(foundAppPotentialResources, jc.SameContents, expectedPotentialResources,
		gc.Commentf("(Assert) mismatch between potential app resources and inserted resources"))
}

// TestCreateApplicationWithResolvedResources tests creation of an application with
// pending resources, where SetCharm has been called first.
// It verifies that the charm_resource table is populated, alongside the
// resource and application_resource table with data from charm and arguments.
// The pending_application_resource table should have no entries with the appName.
func (s *applicationStateSuite) TestCreateApplicationWithPendingResources(c *gc.C) {
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

	ctx := context.Background()

	appName := "666"
	args := s.addApplicationArgForResources(c, appName,
		charmResources, nil)

	charmID, _, err := s.state.SetCharm(ctx, args.Charm, nil, false)
	c.Assert(err, jc.ErrorIsNil)

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

	_, err = s.state.CreateApplication(ctx, appName, args, nil)
	c.Assert(err, jc.ErrorIsNil)
	// Check expected resources are added
	assertTxn := func(comment string, do func(ctx context.Context, tx *sql.Tx) error) {
		err := s.TxnRunner().StdTxn(context.Background(), do)
		c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) %s: %s", comment,
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
	c.Check(foundCharmResources, jc.SameContents, slices.Collect(maps.Values(charmResources)),
		gc.Commentf("(Assert) mismatch between charm resources and inserted resources"))
	c.Check(foundAppAvailableResources, jc.SameContents, addResources,
		gc.Commentf("(Assert) mismatch between app available app resources and inserted resources"))
	expectedPotentialResources := make([]resource.AddResourceDetails, 0, len(addResources))
	for _, res := range addResources {
		expectedPotentialResources = append(expectedPotentialResources, resource.AddResourceDetails{
			Name:     res.Name,
			Revision: nil, // nil revision
		})
	}
	c.Check(foundAppPotentialResources, jc.SameContents, expectedPotentialResources,
		gc.Commentf("(Assert) mismatch between potential app resources and inserted resources"))

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
	c *gc.C,
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

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		for _, res := range resources {
			insertStmt := `
INSERT INTO resource (uuid, charm_uuid, charm_resource_name, revision,
       origin_type_id, state_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
`
			_, err := tx.ExecContext(ctx, insertStmt,
				res.UUID, charmUUID, res.Name, res.Revision, 1, 0, res.CreatedAt)
			c.Assert(err, gc.IsNil)
			if err != nil {
				return err
			}

			linkStmt := `
INSERT INTO pending_application_resource (application_name, resource_uuid)
VALUES (?, ?)
`
			_, err = tx.ExecContext(ctx, linkStmt, appName, res.UUID)
			c.Assert(err, gc.IsNil)
			if err != nil {
				return err
			}
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
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
func (s *applicationStateSuite) TestCreateApplicationWithExistingCharmWithResources(c *gc.C) {
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
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "666", s.addApplicationArgForResources(c, "666",
		charmResources, addResourcesArgs), nil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.CreateApplication(ctx, "667", s.addApplicationArgForResources(c, "666",
		charmResources, addResourcesArgs), nil)
	c.Check(err, jc.ErrorIsNil, gc.Commentf("Failed to create second "+
		"application. Maybe the charm UUID is not properly fetched to pass to "+
		"resources ?"))
}

// TestCreateApplicationWithResourcesMissingResourceArg verifies resource
// handling during app creation.
// If a resource is missing from argument, it is added anyway from charm
// resources and is assumed to be of origin store with no revision.
func (s *applicationStateSuite) TestCreateApplicationWithResourcesMissingResourceArg(c *gc.C) {
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
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "666", s.addApplicationArgForResources(c, "666",
		charmResources, addResourceArgs), nil)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) unexpected error: %s",
		errors.ErrorStack(err)))
}

// TestCreateApplicationWithResourcesTooMuchResourceArgs verifies error handling
// for invalid resources.
// It fails if there is resources args that doesn't refer to actual resources
// in charm.
func (s *applicationStateSuite) TestCreateApplicationWithResourcesTooMuchResourceArgs(c *gc.C) {
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
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "666", s.addApplicationArgForResources(c, "666",
		charmResources, addResourcesArgs), nil)
	c.Assert(err, gc.ErrorMatches,
		`.*inserting resource "my-image": FOREIGN KEY constraint failed.*`,
		gc.Commentf("(Assert) unexpected error: %s",
			errors.ErrorStack(err)))
}

func (s *applicationStateSuite) TestGetApplicationLife(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Dying)
	gotID, appLife, err := s.state.GetApplicationLife(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotID, gc.Equals, appID)
	c.Assert(appLife, gc.Equals, life.Dying)
}

func (s *applicationStateSuite) TestGetApplicationLifeNotFound(c *gc.C) {
	_, _, err := s.state.GetApplicationLife(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestUpsertCloudServiceNew(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	err := s.state.UpsertCloudService(context.Background(), "foo", "provider-id", network.SpaceAddresses{})
	c.Assert(err, jc.ErrorIsNil)
	var providerID string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT provider_id FROM k8s_service WHERE application_uuid = ?", appID).Scan(&providerID)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(providerID, gc.Equals, "provider-id")
}

func (s *applicationStateSuite) TestUpsertCloudServiceExisting(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	err := s.state.UpsertCloudService(context.Background(), "foo", "provider-id", network.SpaceAddresses{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.UpsertCloudService(context.Background(), "foo", "provider-id", network.SpaceAddresses{})
	c.Assert(err, jc.ErrorIsNil)
	var providerID string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT provider_id FROM k8s_service WHERE application_uuid = ?", appID).Scan(&providerID)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(providerID, gc.Equals, "provider-id")
}

func (s *applicationStateSuite) TestUpsertCloudServiceAnother(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.createApplication(c, "bar", life.Alive)
	err := s.state.UpsertCloudService(context.Background(), "foo", "provider-id", network.SpaceAddresses{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.UpsertCloudService(context.Background(), "foo", "another-provider-id", network.SpaceAddresses{})
	c.Assert(err, jc.ErrorIsNil)
	var providerIds []string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(providerIds, jc.SameContents, []string{"provider-id", "another-provider-id"})
}

func (s *applicationStateSuite) TestUpsertCloudServiceUpdateExistingEmptyAddresses(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.createApplication(c, "bar", life.Alive)
	err := s.state.UpsertCloudService(context.Background(), "foo", "provider-id", network.SpaceAddresses{
		network.SpaceAddress{
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.1",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
			},
		},
		network.SpaceAddress{
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.2",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv6Address,
				Scope:      network.ScopeLinkLocal,
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	checkAddresses := func(c *gc.C, expectedAddresses ...string) {
		var resultAddresses []string
		err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(resultAddresses, jc.SameContents, expectedAddresses)
	}

	checkAddresses(c, "10.0.0.1", "10.0.0.2")

	err = s.state.UpsertCloudService(context.Background(), "foo", "provider-id", network.SpaceAddresses{})
	c.Assert(err, jc.ErrorIsNil)
	// Since no addresses were passed as input, the previous addresses should
	// be returned.
	checkAddresses(c, "10.0.0.1", "10.0.0.2")
}

func (s *applicationStateSuite) TestUpsertCloudServiceUpdateExistingWithAddresses(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	s.createApplication(c, "bar", life.Alive)
	err := s.state.UpsertCloudService(context.Background(), "foo", "provider-id", network.SpaceAddresses{
		network.SpaceAddress{
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.1",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
			},
		},
		network.SpaceAddress{
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.2",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv6Address,
				Scope:      network.ScopeLinkLocal,
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	checkAddresses := func(c *gc.C, expectedAddresses ...string) {
		var resultAddresses []string
		err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(resultAddresses, jc.SameContents, expectedAddresses)
	}

	checkAddresses(c, "10.0.0.1", "10.0.0.2")

	err = s.state.UpsertCloudService(context.Background(), "foo", "provider-id", network.SpaceAddresses{
		network.SpaceAddress{
			MachineAddress: network.MachineAddress{
				Value:      "192.168.0.0",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
			},
		},
		network.SpaceAddress{
			MachineAddress: network.MachineAddress{
				Value:      "192.168.0.1",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv6Address,
				Scope:      network.ScopeLinkLocal,
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	// Since no addresses were passed as input, the previous addresses should
	// be returned.
	checkAddresses(c, "192.168.0.0", "192.168.0.1")
}

func (s *applicationStateSuite) TestUpsertCloudServiceNotFound(c *gc.C) {
	err := s.state.UpsertCloudService(context.Background(), "foo", "provider-id", network.SpaceAddresses{})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestCloudServiceAddresses(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)

	expectedAddresses := network.SpaceAddresses{
		{
			SpaceID: network.AlphaSpaceId,
			Origin:  network.OriginProvider,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.1",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			Origin:  network.OriginProvider,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.2",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv6Address,
				Scope:      network.ScopeLinkLocal,
			},
		},
	}
	err := s.state.UpsertCloudService(context.Background(), "foo", "provider-id", network.SpaceAddresses{
		{
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.1",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
			},
		},
		{
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.2",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv6Address,
				Scope:      network.ScopeLinkLocal,
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	addresses, err := s.state.GetCloudServiceAddresses(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(addresses, gc.HasLen, 2)
	c.Check(addresses, gc.DeepEquals, expectedAddresses)
}

func (s *applicationStateSuite) TestGetApplicationIDByUnitName(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	expectedAppUUID := s.createApplication(c, "foo", life.Alive, u1)

	obtainedAppUUID, err := s.state.GetApplicationIDByUnitName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtainedAppUUID, gc.Equals, expectedAppUUID)
}

func (s *applicationStateSuite) TestGetApplicationIDByUnitNameUnitUnitNotFound(c *gc.C) {
	_, err := s.state.GetApplicationIDByUnitName(context.Background(), "failme")
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationStateSuite) TestGetApplicationIDAndNameByUnitName(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	expectedAppUUID := s.createApplication(c, "foo", life.Alive, u1)

	appUUID, appName, err := s.state.GetApplicationIDAndNameByUnitName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(appUUID, gc.Equals, expectedAppUUID)
	c.Check(appName, gc.Equals, "foo")
}

func (s *applicationStateSuite) TestGetApplicationIDAndNameByUnitNameNotFound(c *gc.C) {
	_, _, err := s.state.GetApplicationIDAndNameByUnitName(context.Background(), "failme")
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationStateSuite) TestGetCharmModifiedVersion(c *gc.C) {
	appUUID := s.createApplication(c, "foo", life.Alive)
	s.addCharmModifiedVersion(c, appUUID, 7)

	charmModifiedVersion, err := s.state.GetCharmModifiedVersion(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(charmModifiedVersion, gc.Equals, 7)
}

func (s *applicationStateSuite) TestGetCharmModifiedVersionNull(c *gc.C) {
	appUUID := s.createApplication(c, "foo", life.Alive)

	charmModifiedVersion, err := s.state.GetCharmModifiedVersion(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(charmModifiedVersion, gc.Equals, 0)
}

func (s *applicationStateSuite) TestGetCharmModifiedVersionApplicationNotFound(c *gc.C) {
	_, err := s.state.GetCharmModifiedVersion(context.Background(), applicationtesting.GenApplicationUUID(c))
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetApplicationScaleState(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	appID := s.createApplication(c, "foo", life.Alive, u)

	scaleState, err := s.state.GetApplicationScaleState(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(scaleState, jc.DeepEquals, application.ScaleState{
		Scale: 1,
	})
}

func (s *applicationStateSuite) TestGetApplicationScaleStateNotFound(c *gc.C) {
	_, err := s.state.GetApplicationScaleState(context.Background(), coreapplication.ID(uuid.MustNewUUID().String()))
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestSetDesiredApplicationScale(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)

	err := s.state.SetDesiredApplicationScale(context.Background(), appID, 666)
	c.Assert(err, jc.ErrorIsNil)

	var gotScale int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale FROM application_scale WHERE application_uuid=?", appID).
			Scan(&gotScale)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotScale, jc.DeepEquals, 666)
}

func (s *applicationStateSuite) TestUpdateApplicationScale(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)

	err := s.state.SetDesiredApplicationScale(context.Background(), appID, 666)
	c.Assert(err, jc.ErrorIsNil)

	newScale, err := s.state.UpdateApplicationScale(context.Background(), appID, 2)
	c.Assert(err, jc.ErrorIsNil)

	var gotScale int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale FROM application_scale WHERE application_uuid=?", appID).
			Scan(&gotScale)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotScale, jc.DeepEquals, 666+2)
	c.Check(newScale, jc.DeepEquals, 666+2)
}

func (s *applicationStateSuite) TestUpdateApplicationScaleInvalidScale(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)

	err := s.state.SetDesiredApplicationScale(context.Background(), appID, 666)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.UpdateApplicationScale(context.Background(), appID, -667)
	c.Assert(err, gc.ErrorMatches, `scale change invalid: cannot remove more units than currently exist`)
}

func (s *applicationStateSuite) TestSetApplicationScalingStateAlreadyScaling(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	appID := s.createApplication(c, "foo", life.Dead, u)

	// Set up the initial scale value.
	err := s.state.SetDesiredApplicationScale(context.Background(), appID, 666)
	c.Assert(err, jc.ErrorIsNil)

	checkResult := func(want application.ScaleState) {
		var got application.ScaleState
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			err := tx.QueryRowContext(ctx, "SELECT scale, scaling, scale_target FROM application_scale WHERE application_uuid=?", appID).
				Scan(&got.Scale, &got.Scaling, &got.ScaleTarget)
			return err
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(got, jc.DeepEquals, want)
	}

	err = s.state.SetApplicationScalingState(context.Background(), "foo", 42, true)
	c.Assert(err, jc.ErrorIsNil)
	checkResult(application.ScaleState{
		Scale:       42,
		ScaleTarget: 42,
		Scaling:     true,
	})

	// Set scaling state but use the same target value as current scale.
	err = s.state.SetApplicationScalingState(context.Background(), "foo", 42, true)
	c.Assert(err, jc.ErrorIsNil)
	checkResult(application.ScaleState{
		Scale:       42,
		ScaleTarget: 42,
		Scaling:     true,
	})
}

func (s *applicationStateSuite) TestSetApplicationScalingStateInconsistent(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	appID := s.createApplication(c, "foo", life.Alive, u)

	// Set up the initial scale value.
	err := s.state.SetDesiredApplicationScale(context.Background(), appID, 666)
	c.Assert(err, jc.ErrorIsNil)

	// Set scaling state but use a target value different than the current
	// scale.
	err = s.state.SetApplicationScalingState(context.Background(), "foo", 42, true)
	c.Assert(err, gc.ErrorMatches, "scaling state is inconsistent")
}

func (s *applicationStateSuite) TestSetApplicationScalingStateAppDying(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	appID := s.createApplication(c, "foo", life.Dying, u)

	// Set up the initial scale value.
	err := s.state.SetDesiredApplicationScale(context.Background(), appID, 666)
	c.Assert(err, jc.ErrorIsNil)

	checkResult := func(want application.ScaleState) {
		var got application.ScaleState
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			err := tx.QueryRowContext(ctx, "SELECT scale, scaling, scale_target FROM application_scale WHERE application_uuid=?", appID).
				Scan(&got.Scale, &got.Scaling, &got.ScaleTarget)
			return err
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(got, jc.DeepEquals, want)
	}

	err = s.state.SetApplicationScalingState(context.Background(), "foo", 42, true)
	c.Assert(err, jc.ErrorIsNil)
	checkResult(application.ScaleState{
		Scale:       42,
		ScaleTarget: 42,
		Scaling:     true,
	})
}

// This test is exactly like TestSetApplicationScalingStateAppDying but the app
// is dead instead of dying.
func (s *applicationStateSuite) TestSetApplicationScalingStateAppDead(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	appID := s.createApplication(c, "foo", life.Dead, u)

	// Set up the initial scale value.
	err := s.state.SetDesiredApplicationScale(context.Background(), appID, 666)
	c.Assert(err, jc.ErrorIsNil)

	checkResult := func(want application.ScaleState) {
		var got application.ScaleState
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			err := tx.QueryRowContext(ctx, "SELECT scale, scaling, scale_target FROM application_scale WHERE application_uuid=?", appID).
				Scan(&got.Scale, &got.Scaling, &got.ScaleTarget)
			return err
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(got, jc.DeepEquals, want)
	}

	err = s.state.SetApplicationScalingState(context.Background(), "foo", 42, true)
	c.Assert(err, jc.ErrorIsNil)
	checkResult(application.ScaleState{
		Scale:       42,
		ScaleTarget: 42,
		Scaling:     true,
	})
}

func (s *applicationStateSuite) TestSetApplicationScalingStateNotScaling(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	appID := s.createApplication(c, "foo", life.Alive, u)

	// Set up the initial scale value.
	err := s.state.SetDesiredApplicationScale(context.Background(), appID, 666)
	c.Assert(err, jc.ErrorIsNil)

	checkResult := func(want application.ScaleState) {
		var got application.ScaleState
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			err := tx.QueryRowContext(ctx, "SELECT scale, scaling, scale_target FROM application_scale WHERE application_uuid=?", appID).
				Scan(&got.Scale, &got.Scaling, &got.ScaleTarget)
			return err
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(got, jc.DeepEquals, want)
	}

	err = s.state.SetApplicationScalingState(context.Background(), "foo", 668, false)
	c.Assert(err, jc.ErrorIsNil)
	checkResult(application.ScaleState{
		Scale:       666,
		ScaleTarget: 668,
		Scaling:     false,
	})
}

func (s *applicationStateSuite) TestSetApplicationLife(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	ctx := context.Background()

	checkResult := func(want life.Life) {
		var gotLife life.Life
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			err := tx.QueryRowContext(ctx, "SELECT life_id FROM application WHERE uuid=?", appID).
				Scan(&gotLife)
			return err
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(gotLife, jc.DeepEquals, want)
	}

	err := s.state.SetApplicationLife(ctx, appID, life.Dying)
	c.Assert(err, jc.ErrorIsNil)
	checkResult(life.Dying)

	err = s.state.SetApplicationLife(ctx, appID, life.Dead)
	c.Assert(err, jc.ErrorIsNil)
	checkResult(life.Dead)

	// Can't go backwards.
	err = s.state.SetApplicationLife(ctx, appID, life.Dying)
	c.Assert(err, jc.ErrorIsNil)
	checkResult(life.Dead)
}

func (s *applicationStateSuite) TestDeleteApplication(c *gc.C) {
	// TODO(units) - add references to constraints, storage etc when those are fully cooked
	ctx := context.Background()
	s.createApplication(c, "foo", life.Alive)

	err := s.state.DeleteApplication(ctx, "foo")
	c.Assert(err, jc.ErrorIsNil)

	var (
		appCount              int
		platformCount         int
		channelCount          int
		scaleCount            int
		appEndpointCount      int
		appExtraEndpointCount int
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT count(*) FROM application WHERE name=?", "foo").Scan(&appCount)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, `

SELECT count(*) FROM application a
JOIN application_platform ap ON a.uuid = ap.application_uuid
WHERE a.name=?`,

			"foo").Scan(&platformCount)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, `

SELECT count(*) FROM application a
JOIN application_channel ap ON a.uuid = ap.application_uuid
WHERE a.name=?`,

			"666").Scan(&channelCount)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, `

SELECT count(*) FROM application a
JOIN application_scale asc ON a.uuid = asc.application_uuid
WHERE a.name=?`,

			"666").Scan(&scaleCount)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, `

SELECT count(*) FROM application a
JOIN application_channel ac ON a.uuid = ac.application_uuid
WHERE a.name=?`,

			"foo").Scan(&channelCount)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, `

SELECT count(*) FROM application a
JOIN application_endpoint ae ON a.uuid = ae.application_uuid
WHERE a.name=?`,
			"foo").Scan(&appEndpointCount)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, `

SELECT count(*) FROM application a
JOIN application_extra_endpoint ae ON a.uuid = ae.application_uuid
WHERE a.name=?`,
			"foo").Scan(&appExtraEndpointCount)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(appCount, gc.Equals, 0)
	c.Check(platformCount, gc.Equals, 0)
	c.Check(channelCount, gc.Equals, 0)
	c.Check(scaleCount, gc.Equals, 0)
	c.Check(appEndpointCount, gc.Equals, 0)
	c.Check(appExtraEndpointCount, gc.Equals, 0)
}

func (s *applicationStateSuite) TestDeleteApplicationTwice(c *gc.C) {
	ctx := context.Background()
	s.createApplication(c, "foo", life.Alive)

	err := s.state.DeleteApplication(ctx, "foo")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.DeleteApplication(ctx, "foo")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestDeleteDeadApplication(c *gc.C) {
	ctx := context.Background()
	s.createApplication(c, "foo", life.Dead)

	err := s.state.DeleteApplication(ctx, "foo")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.DeleteApplication(ctx, "foo")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestDeleteApplicationWithUnits(c *gc.C) {
	ctx := context.Background()
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u)

	err := s.state.DeleteApplication(ctx, "foo")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationHasUnits)
	c.Assert(err, gc.ErrorMatches, `.*cannot delete application "foo" as it still has 1 unit\(s\)`)

	var appCount int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT count(*) FROM application WHERE name=?", "foo").Scan(&appCount)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(appCount, gc.Equals, 1)
}

func (s *applicationStateSuite) TestGetApplicationUnitLife(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	u2 := application.InsertUnitArg{
		UnitName: "foo/667",
	}
	u3 := application.InsertUnitArg{
		UnitName: "bar/667",
	}
	s.createApplication(c, "foo", life.Alive, u1, u2)
	s.createApplication(c, "bar", life.Alive, u3)

	var unitID1, unitID2, unitID3 coreunit.UUID
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 2 WHERE name=?", "foo/666"); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/666").Scan(&unitID1); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/667").Scan(&unitID2); err != nil {
			return err
		}
		err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "bar/667").Scan(&unitID3)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	got, err := s.state.GetApplicationUnitLife(context.Background(), "foo", unitID2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, map[coreunit.UUID]life.Life{
		unitID2: life.Alive,
	})

	got, err = s.state.GetApplicationUnitLife(context.Background(), "foo", unitID1, unitID2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, map[coreunit.UUID]life.Life{
		unitID1: life.Dead,
		unitID2: life.Alive,
	})

	got, err = s.state.GetApplicationUnitLife(context.Background(), "foo", unitID2, unitID3)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, map[coreunit.UUID]life.Life{
		unitID2: life.Alive,
	})

	got, err = s.state.GetApplicationUnitLife(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.HasLen, 0)
}

func (s *applicationStateSuite) TestStorageDefaultsNone(c *gc.C) {
	defaults, err := s.state.StorageDefaults(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(defaults, jc.DeepEquals, domainstorage.StorageDefaults{})
}

func (s *applicationStateSuite) TestStorageDefaults(c *gc.C) {
	db := s.DB()
	_, err := db.ExecContext(context.Background(), "INSERT INTO model_config (key, value) VALUES (?, ?)",
		"storage-default-block-source", "ebs-fast")
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.ExecContext(context.Background(), "INSERT INTO model_config (key, value) VALUES (?, ?)",
		"storage-default-filesystem-source", "elastic-fs")
	c.Assert(err, jc.ErrorIsNil)

	defaults, err := s.state.StorageDefaults(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(defaults, jc.DeepEquals, domainstorage.StorageDefaults{
		DefaultBlockSource:      ptr("ebs-fast"),
		DefaultFilesystemSource: ptr("elastic-fs"),
	})
}

func (s *applicationStateSuite) TestGetCharmIDByApplicationName(c *gc.C) {
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

	s.createApplication(c, "foo", life.Alive)

	_, _, err := s.state.SetCharm(context.Background(), charm.Charm{
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
	c.Assert(err, jc.ErrorIsNil)

	chID, err := s.state.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(chID.Validate(), jc.ErrorIsNil)
}

func (s *applicationStateSuite) TestGetCharmIDByApplicationNameError(c *gc.C) {
	_, err := s.state.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetCharmByApplicationID(c *gc.C) {

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
	ctx := context.Background()

	appID, err := s.state.CreateApplication(ctx, "foo", application.AddApplicationArg{
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
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Add the implicit juju-info relation inserted with the charm.
	expectedMetadata.Provides = jujuInfoRelation()

	ch, err := s.state.GetCharmByApplicationID(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ch, gc.DeepEquals, charm.Charm{
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
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotPlatform, gc.DeepEquals, platform)
}

func (s *applicationStateSuite) TestCreateApplicationDefaultSourceIsCharmhub(c *gc.C) {
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
	ctx := context.Background()

	appID, err := s.state.CreateApplication(ctx, "foo", application.AddApplicationArg{
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
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Add the implicit juju-info relation inserted with the charm.
	expectedMetadata.Provides = jujuInfoRelation()

	ch, err := s.state.GetCharmByApplicationID(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ch, gc.DeepEquals, charm.Charm{
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

func (s *applicationStateSuite) TestSetCharmThenGetCharmByApplicationNameInvalidName(c *gc.C) {
	expectedMetadata := charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: semversion.MustParse("4.0.0"),
		Assumes:        []byte("null"),
	}
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "foo", application.AddApplicationArg{
		Charm: charm.Charm{
			Metadata:      expectedMetadata,
			Manifest:      s.minimalManifest(c),
			Source:        charm.LocalSource,
			ReferenceName: "ubuntu",
		},
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	id := applicationtesting.GenApplicationUUID(c)

	_, err = s.state.GetCharmByApplicationID(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestCheckCharmExistsNotFound(c *gc.C) {
	id := charmtesting.GenCharmID(c)
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.checkCharmExists(ctx, tx, charmID{
			UUID: id,
		})
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *applicationStateSuite) TestInitialWatchStatementApplicationsWithPendingCharms(c *gc.C) {
	name, query := s.state.InitialWatchStatementApplicationsWithPendingCharms()
	c.Check(name, gc.Equals, "application")

	id := s.createApplication(c, "foo", life.Alive)

	result, err := query(context.Background(), s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.SameContents, []string{id.String()})
}

func (s *applicationStateSuite) TestInitialWatchStatementApplicationsWithPendingCharmsIfAvailable(c *gc.C) {
	// These use the same charm, so once you set one applications charm, you
	// set both.

	name, query := s.state.InitialWatchStatementApplicationsWithPendingCharms()
	c.Check(name, gc.Equals, "application")

	_ = s.createApplication(c, "foo", life.Alive)
	id1 := s.createApplication(c, "bar", life.Alive)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `

UPDATE charm SET available = TRUE
FROM application AS a
INNER JOIN charm AS c ON a.charm_uuid = c.uuid
WHERE a.uuid=?`, id1.String())

		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := query(context.Background(), s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.HasLen, 0)
}

func (s *applicationStateSuite) TestInitialWatchStatementApplicationsWithPendingCharmsNothing(c *gc.C) {
	name, query := s.state.InitialWatchStatementApplicationsWithPendingCharms()
	c.Check(name, gc.Equals, "application")

	result, err := query(context.Background(), s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.HasLen, 0)
}

func (s *applicationStateSuite) TestGetApplicationsWithPendingCharmsFromUUIDsIfPending(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	expected, err := s.state.GetApplicationsWithPendingCharmsFromUUIDs(context.Background(), []coreapplication.ID{id})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(expected, jc.DeepEquals, []coreapplication.ID{id})
}

func (s *applicationStateSuite) TestGetApplicationsWithPendingCharmsFromUUIDsIfAvailable(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `

UPDATE charm SET available = TRUE
FROM application AS a
INNER JOIN charm AS c ON a.charm_uuid = c.uuid
WHERE a.uuid=?`, id.String())

		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	expected, err := s.state.GetApplicationsWithPendingCharmsFromUUIDs(context.Background(), []coreapplication.ID{id})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(expected, gc.HasLen, 0)
}

func (s *applicationStateSuite) TestGetApplicationsWithPendingCharmsFromUUIDsNotFound(c *gc.C) {
	expected, err := s.state.GetApplicationsWithPendingCharmsFromUUIDs(context.Background(), []coreapplication.ID{"foo"})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(expected, gc.HasLen, 0)
}

func (s *applicationStateSuite) TestGetApplicationsWithPendingCharmsFromUUIDsForSameCharm(c *gc.C) {
	// These use the same charm, so once you set one applications charm, you
	// set both.

	id0 := s.createApplication(c, "foo", life.Alive)
	id1 := s.createApplication(c, "bar", life.Alive)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `

UPDATE charm SET available = TRUE
FROM application AS a
INNER JOIN charm AS c ON a.charm_uuid = c.uuid
WHERE a.uuid=?`, id1.String())

		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	expected, err := s.state.GetApplicationsWithPendingCharmsFromUUIDs(context.Background(), []coreapplication.ID{id0, id1})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(expected, gc.HasLen, 0)
}

func (s *applicationStateSuite) TestGetAsyncCharmDownloadInfo(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	charmUUID, err := s.state.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	info, err := s.state.GetAsyncCharmDownloadInfo(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(info, jc.DeepEquals, application.CharmDownloadInfo{
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

func (s *applicationStateSuite) TestGetAsyncCharmDownloadInfoNoApplication(c *gc.C) {
	id := applicationtesting.GenApplicationUUID(c)

	_, err := s.state.GetAsyncCharmDownloadInfo(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetAsyncCharmDownloadInfoAlreadyDone(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	charmUUID, err := s.state.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetCharmAvailable(context.Background(), charmUUID)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.GetAsyncCharmDownloadInfo(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmAlreadyAvailable)
}

func (s *applicationStateSuite) TestResolveCharmDownload(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	objectStoreUUID := s.createObjectStoreBlob(c, "archive")

	info, err := s.state.GetAsyncCharmDownloadInfo(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

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

	err = s.state.ResolveCharmDownload(context.Background(), info.CharmUUID, application.ResolvedCharmDownload{
		Actions:         actions,
		LXDProfile:      []byte("profile"),
		ObjectStoreUUID: objectStoreUUID,
		ArchivePath:     "archive",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the charm is now available.
	available, err := s.state.IsCharmAvailable(context.Background(), info.CharmUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(available, gc.Equals, true)

	ch, err := s.state.GetCharmByApplicationID(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(ch.Actions, gc.DeepEquals, actions)
	c.Check(ch.LXDProfile, gc.DeepEquals, []byte("profile"))
	c.Check(ch.ArchivePath, gc.DeepEquals, "archive")
}

func (s *applicationStateSuite) TestResolveCharmDownloadAlreadyResolved(c *gc.C) {
	s.createApplication(c, "foo", life.Alive)

	objectStoreUUID := s.createObjectStoreBlob(c, "archive")

	charmUUID, err := s.state.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetCharmAvailable(context.Background(), charmUUID)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.ResolveCharmDownload(context.Background(), charmUUID, application.ResolvedCharmDownload{
		LXDProfile:      []byte("profile"),
		ObjectStoreUUID: objectStoreUUID,
		ArchivePath:     "archive",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmAlreadyResolved)
}

func (s *applicationStateSuite) TestResolveCharmDownloadNotFound(c *gc.C) {
	s.createApplication(c, "foo", life.Alive)

	objectStoreUUID := s.createObjectStoreBlob(c, "archive")

	err := s.state.ResolveCharmDownload(context.Background(), "foo", application.ResolvedCharmDownload{
		LXDProfile:      []byte("profile"),
		ObjectStoreUUID: objectStoreUUID,
		ArchivePath:     "archive",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *applicationStateSuite) TestGetAsyncCharmDownloadInfoLocalCharm(c *gc.C) {
	platform := deployment.Platform{
		Channel:      "22.04/stable",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Risk: deployment.RiskStable,
	}
	ctx := context.Background()

	appID, err := s.state.CreateApplication(ctx, "foo", application.AddApplicationArg{
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
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.GetAsyncCharmDownloadInfo(context.Background(), appID)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmProvenanceNotValid)
}

func (s *applicationStateSuite) TestGetApplicationsForRevisionUpdater(c *gc.C) {
	// Create a few applications.
	s.createApplication(c, "foo", life.Alive)
	s.createApplication(c, "bar", life.Alive, application.InsertUnitArg{
		UnitName: "bar/0",
	})

	// Get the applications for the revision updater.
	apps, err := s.state.GetApplicationsForRevisionUpdater(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(apps, jc.DeepEquals, []application.RevisionUpdaterApplication{{
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

func (s *applicationStateSuite) TestGetApplicationConfigAndSettings(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := `INSERT INTO application_config (application_uuid, key, value, type_id) VALUES (?, ?, ?, ?)`
		_, err := tx.ExecContext(ctx, stmt, id.String(), "key", "value", 0)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, jc.DeepEquals, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: "value",
		},
	})
	c.Check(settings, jc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestGetApplicationConfigAndSettingsWithTrust(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Assert(err, jc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, jc.DeepEquals, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: "value",
		},
	})
	c.Check(settings, jc.DeepEquals, application.ApplicationSettings{
		Trust: true,
	})
}

func (s *applicationStateSuite) TestGetApplicationConfigAndSettingsNotFound(c *gc.C) {
	// If the application is not found, it should return application not found.
	id := applicationtesting.GenApplicationUUID(c)
	_, _, err := s.state.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetApplicationConfigAndSettingsNoConfig(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	// If there is no config, we should always return the trust. This comes
	// from the application_setting table.

	config, settings, err := s.state.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, gc.HasLen, 0)
	c.Check(settings, jc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestGetApplicationConfigAndSettingsForApplications(c *gc.C) {
	id0 := s.createApplication(c, "foo", life.Alive)
	id1 := s.createApplication(c, "bar", life.Alive)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Assert(err, jc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(context.Background(), id0)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, jc.DeepEquals, map[string]application.ApplicationConfig{
		"a": {
			Type:  charm.OptionString,
			Value: "b",
		},
		"c": {
			Type:  charm.OptionFloat,
			Value: "d",
		},
	})
	c.Check(settings, jc.DeepEquals, application.ApplicationSettings{})

	config, settings, err = s.state.GetApplicationConfigAndSettings(context.Background(), id1)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, jc.DeepEquals, map[string]application.ApplicationConfig{
		"e": {
			Type:  charm.OptionInt,
			Value: "f",
		},
	})
	c.Check(settings, jc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestGetApplicationConfigWithDefaults(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	s.insertApplicationConfigWithDefault(c, id, "key1", "value1", "defaultValue1", applicationcharm.OptionString)
	s.insertCharmConfig(c, id, "key2", "defaultValue2", applicationcharm.OptionString)

	config, err := s.state.GetApplicationConfigWithDefaults(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, jc.DeepEquals, map[string]application.ApplicationConfig{
		"key1": {
			Type:  charm.OptionString,
			Value: "value1",
		},
		"key2": {
			Type:  charm.OptionString,
			Value: "defaultValue2",
		},
	})
}

func (s *applicationStateSuite) TestGetApplicationConfigWithDefaultsNotFound(c *gc.C) {
	// If the application is not found, it should return application not found.
	id := applicationtesting.GenApplicationUUID(c)
	_, err := s.state.GetApplicationConfigWithDefaults(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetApplicationConfigWithDefaultsNoConfig(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	// If there is no config, we should always return the trust. This comes
	// from the application_setting table.

	config, err := s.state.GetApplicationConfigWithDefaults(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, gc.HasLen, 0)
}

func (s *applicationStateSuite) TestGetApplicationTrustSetting(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Assert(err, jc.ErrorIsNil)

	trust, err := s.state.GetApplicationTrustSetting(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(trust, jc.IsTrue)
}

func (s *applicationStateSuite) TestGetApplicationTrustSettingNoRow(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := `INSERT INTO application_config (application_uuid, key, value, type_id) VALUES (?, ?, ?, ?)`
		_, err := tx.ExecContext(ctx, stmt, id.String(), "key", "value", 0)
		if err != nil {
			return err
		}
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	trust, err := s.state.GetApplicationTrustSetting(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(trust, jc.IsFalse)
}

func (s *applicationStateSuite) TestGetApplicationTrustSettingNoApplication(c *gc.C) {
	// If the application is not found, it should return application not found.
	id := applicationtesting.GenApplicationUUID(c)
	_, err := s.state.GetApplicationTrustSetting(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetApplicationConfigHash(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	// No config, so the hash should just be the trust value.

	hash, err := s.state.GetApplicationConfigHash(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(hash, gc.Equals, "fcbcf165908dd18a9e49f7ff27810176db8e9f63b4352213741664245224f8aa")
}

func (s *applicationStateSuite) TestGetApplicationConfigHashNotFound(c *gc.C) {
	id := applicationtesting.GenApplicationUUID(c)
	_, err := s.state.GetApplicationConfigHash(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestUpdateApplicationConfigAndSettingsNoApplication(c *gc.C) {
	// If the application is not found, it should return application not found.
	id := applicationtesting.GenApplicationUUID(c)
	err := s.state.UpdateApplicationConfigAndSettings(context.Background(), id, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: "value",
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestUpdateApplicationConfigAndSettingsApplicationIsDead(c *gc.C) {
	id := s.createApplication(c, "foo", life.Dead)

	err := s.state.UpdateApplicationConfigAndSettings(context.Background(), id, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: "value",
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationIsDead)
}

func (s *applicationStateSuite) TestUpdateApplicationConfigAndSettingsNoop(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.state.UpdateApplicationConfigAndSettings(context.Background(), id, map[string]application.ApplicationConfig{}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, jc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, jc.DeepEquals, map[string]application.ApplicationConfig{})
	c.Check(settings, jc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestUpdateApplicationConfigAndSettings(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.state.UpdateApplicationConfigAndSettings(context.Background(), id, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: "value",
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, jc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, jc.DeepEquals, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: "value",
		},
	})
	c.Check(settings, jc.DeepEquals, application.ApplicationSettings{})

	sha256, err := s.state.GetApplicationConfigHash(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sha256, gc.Equals, "6e1b3adca7459d700abb8e270b06ee7fc96f83436bb533ad4540a3a6eb66cf1b")
}

func (s *applicationStateSuite) TestUpdateApplicationConfigAndSettingsMultipleConfigOptions(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.state.UpdateApplicationConfigAndSettings(context.Background(), id, map[string]application.ApplicationConfig{
		"foo": {
			Type:  charm.OptionString,
			Value: "bar",
		},
		"doink": {
			Type:  charm.OptionInt,
			Value: 17,
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, jc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, jc.DeepEquals, map[string]application.ApplicationConfig{
		"foo": {
			Type:  charm.OptionString,
			Value: "bar",
		},
		"doink": {
			Type:  charm.OptionInt,
			Value: "17",
		},
	})
	c.Check(settings, jc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestUpdateApplicationConfigAndSettingsChangesIdempotent(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.state.UpdateApplicationConfigAndSettings(context.Background(), id, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: "value",
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.UpdateApplicationConfigAndSettings(context.Background(), id, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: "value",
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, jc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, jc.DeepEquals, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: "value",
		},
	})
	c.Check(settings, jc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestUpdateApplicationConfigAndSettingsMerges(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.state.UpdateApplicationConfigAndSettings(context.Background(), id, map[string]application.ApplicationConfig{
		"foo": {
			Type:  charm.OptionString,
			Value: "bar",
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, jc.ErrorIsNil)

	sha256, err := s.state.GetApplicationConfigHash(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sha256, gc.Equals, "3fe07426e3e5c57aa18fc4a3d7e412ee31ea150e71d343fbcbe3a406350d3297")

	err = s.state.UpdateApplicationConfigAndSettings(context.Background(), id, map[string]application.ApplicationConfig{
		"bar": {
			Type:  charm.OptionString,
			Value: "foo",
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, jc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, jc.DeepEquals, map[string]application.ApplicationConfig{
		"foo": {
			Type:  charm.OptionString,
			Value: "bar",
		},
		"bar": {
			Type:  charm.OptionString,
			Value: "foo",
		},
	})
	c.Check(settings, jc.DeepEquals, application.ApplicationSettings{})

	sha256, err = s.state.GetApplicationConfigHash(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sha256, gc.Equals, "8324209a0e1897b4d1f56e4f4b172af181496d377ceef179362999720148841e")
}

func (s *applicationStateSuite) TestUpdateApplicationConfigAndSettingsOverwritesIfSet(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.state.UpdateApplicationConfigAndSettings(context.Background(), id, map[string]application.ApplicationConfig{
		"foo": {
			Type:  charm.OptionString,
			Value: "bar",
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.UpdateApplicationConfigAndSettings(context.Background(), id, map[string]application.ApplicationConfig{
		"foo": {
			Type:  charm.OptionString,
			Value: "baz",
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, jc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, jc.DeepEquals, map[string]application.ApplicationConfig{
		"foo": {
			Type:  charm.OptionString,
			Value: "baz",
		},
	})
	c.Check(settings, jc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestUpdateApplicationConfigAndSettingsupdatesTrust(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.state.UpdateApplicationConfigAndSettings(context.Background(), id, map[string]application.ApplicationConfig{},
		application.UpdateApplicationSettingsArg{
			Trust: ptr(true),
		})
	c.Assert(err, jc.ErrorIsNil)

	_, settings, err := s.state.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(settings, jc.DeepEquals, application.ApplicationSettings{Trust: true})

	// Follow up by checking a nil value does not change the setting.

	err = s.state.UpdateApplicationConfigAndSettings(context.Background(), id, map[string]application.ApplicationConfig{},
		application.UpdateApplicationSettingsArg{
			Trust: nil,
		})
	c.Assert(err, jc.ErrorIsNil)

	_, settings, err = s.state.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(settings, jc.DeepEquals, application.ApplicationSettings{Trust: true})
}

func (s *applicationStateSuite) TestUnsetApplicationConfigKeys(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.state.UpdateApplicationConfigAndSettings(context.Background(), id, map[string]application.ApplicationConfig{
		"a": {
			Type:  charm.OptionString,
			Value: "b",
		},
		"c": {
			Type:  charm.OptionString,
			Value: "d1",
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.UnsetApplicationConfigKeys(context.Background(), id, []string{"a"})
	c.Assert(err, jc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, jc.DeepEquals, map[string]application.ApplicationConfig{
		"c": {
			Type:  charm.OptionString,
			Value: "d1",
		},
	})
	c.Check(settings, jc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestUnsetApplicationConfigKeysApplicationNotFound(c *gc.C) {
	// If the application is not found, it should return application not found.
	id := applicationtesting.GenApplicationUUID(c)
	err := s.state.UnsetApplicationConfigKeys(context.Background(), id, []string{"a"})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestUnsetApplicationConfigKeysIncludingTrust(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.state.UpdateApplicationConfigAndSettings(context.Background(), id,
		map[string]application.ApplicationConfig{},
		application.UpdateApplicationSettingsArg{Trust: ptr(true)},
	)
	c.Assert(err, jc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, gc.HasLen, 0)
	c.Check(settings, jc.DeepEquals, application.ApplicationSettings{
		Trust: true,
	})

	err = s.state.UnsetApplicationConfigKeys(context.Background(), id, []string{"a", "trust"})
	c.Assert(err, jc.ErrorIsNil)

	config, settings, err = s.state.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, jc.DeepEquals, map[string]application.ApplicationConfig{})
	c.Check(settings, jc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestUnsetApplicationConfigKeysIgnoredKeys(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.state.UpdateApplicationConfigAndSettings(context.Background(), id, map[string]application.ApplicationConfig{
		"a": {
			Type:  charm.OptionString,
			Value: "b",
		},
		"c": {
			Type:  charm.OptionString,
			Value: "d1",
		},
	}, application.UpdateApplicationSettingsArg{})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.UnsetApplicationConfigKeys(context.Background(), id, []string{"a", "x", "y"})
	c.Assert(err, jc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, jc.DeepEquals, map[string]application.ApplicationConfig{
		"c": {
			Type:  charm.OptionString,
			Value: "d1",
		},
	})
	c.Check(settings, jc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestGetCharmConfigByApplicationID(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	cid, err := s.state.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := `INSERT INTO charm_config (charm_uuid, key, default_value, type_id) VALUES (?, ?, ?, ?)`
		_, err := tx.ExecContext(ctx, stmt, cid.String(), "key", "value", 0)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	charmID, config, err := s.state.GetCharmConfigByApplicationID(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(charmID, gc.Equals, cid)
	c.Check(config, jc.DeepEquals, charm.Config{
		Options: map[string]charm.Option{
			"key": {
				Type:    charm.OptionString,
				Default: "value",
			},
		},
	})
}

func (s *applicationStateSuite) TestGetCharmConfigByApplicationIDApplicationNotFound(c *gc.C) {
	// If the application is not found, it should return application not found.
	id := applicationtesting.GenApplicationUUID(c)
	_, _, err := s.state.GetCharmConfigByApplicationID(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestCheckApplicationCharm(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	cid, err := s.state.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.checkApplicationCharm(context.Background(), tx, applicationID{ID: id}, charmID{UUID: cid})
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationStateSuite) TestCheckApplicationCharmDifferentCharm(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.checkApplicationCharm(context.Background(), tx, applicationID{ID: id}, charmID{UUID: "other"})
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationHasDifferentCharm)
}

func (s *applicationStateSuite) TestGetApplicationIDByName(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	gotID, err := s.state.GetApplicationIDByName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotID, gc.Equals, id)
}

func (s *applicationStateSuite) TestGetApplicationIDByNameNotFound(c *gc.C) {
	_, err := s.state.GetApplicationIDByName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestHashConfigAndSettings(c *gc.C) {
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
				Value: "value",
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
				Value: "value",
			},
			{
				Key:   "key2",
				Type:  "int",
				Value: 42,
			},
			{
				Key:   "key3",
				Type:  "float",
				Value: 3.14,
			},
			{
				Key:   "key4",
				Type:  "boolean",
				Value: true,
			},
			{
				Key:   "key5",
				Type:  "secret",
				Value: "secret",
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
		c.Assert(err, jc.ErrorIsNil)
		c.Check(hash, gc.Equals, test.expected)
	}
}

func (s *applicationStateSuite) TestConstraintFull(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
		if err != nil {
			return err
		}

		addAppConstraintStmt := `INSERT INTO application_constraint (application_uuid, constraint_uuid) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, addAppConstraintStmt, id, "constraint-uuid")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	cons, err := s.state.GetApplicationConstraints(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*cons.Tags, jc.SameContents, []string{"tag0", "tag1"})
	c.Check(*cons.Spaces, jc.SameContents, []constraints.SpaceConstraint{
		{SpaceName: "space0", Exclude: false},
		{SpaceName: "space1", Exclude: true},
	})
	c.Check(*cons.Zones, jc.SameContents, []string{"zone0", "zone1"})
	c.Check(cons.Arch, jc.DeepEquals, ptr("amd64"))
	c.Check(cons.CpuCores, jc.DeepEquals, ptr(uint64(2)))
	c.Check(cons.CpuPower, jc.DeepEquals, ptr(uint64(42)))
	c.Check(cons.Mem, jc.DeepEquals, ptr(uint64(8)))
	c.Check(cons.RootDisk, jc.DeepEquals, ptr(uint64(256)))
	c.Check(cons.RootDiskSource, jc.DeepEquals, ptr("root-disk-source"))
	c.Check(cons.InstanceRole, jc.DeepEquals, ptr("instance-role"))
	c.Check(cons.InstanceType, jc.DeepEquals, ptr("instance-type"))
	c.Check(cons.Container, jc.DeepEquals, ptr(instance.LXD))
	c.Check(cons.VirtType, jc.DeepEquals, ptr("virt-type"))
	c.Check(cons.AllocatePublicIP, jc.DeepEquals, ptr(true))
	c.Check(cons.ImageID, jc.DeepEquals, ptr("image-id"))
}

func (s *applicationStateSuite) TestConstraintPartial(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		addConstraintStmt := `INSERT INTO "constraint" (uuid, arch, cpu_cores, allocate_public_ip, image_id) VALUES (?, ?, ?, ?, ?)`
		_, err := tx.ExecContext(ctx, addConstraintStmt, "constraint-uuid", "amd64", 2, true, "image-id")
		if err != nil {
			return err
		}
		addAppConstraintStmt := `INSERT INTO application_constraint (application_uuid, constraint_uuid) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, addAppConstraintStmt, id, "constraint-uuid")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	cons, err := s.state.GetApplicationConstraints(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cons, jc.DeepEquals, constraints.Constraints{
		Arch:             ptr("amd64"),
		CpuCores:         ptr(uint64(2)),
		AllocatePublicIP: ptr(true),
		ImageID:          ptr("image-id"),
	})
}

func (s *applicationStateSuite) TestConstraintSingleValue(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		addConstraintStmt := `INSERT INTO "constraint" (uuid, cpu_cores) VALUES (?, ?)`
		_, err := tx.ExecContext(ctx, addConstraintStmt, "constraint-uuid", 2)
		if err != nil {
			return err
		}
		addAppConstraintStmt := `INSERT INTO application_constraint (application_uuid, constraint_uuid) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, addAppConstraintStmt, id, "constraint-uuid")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	cons, err := s.state.GetApplicationConstraints(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cons, jc.DeepEquals, constraints.Constraints{
		CpuCores: ptr(uint64(2)),
	})
}

func (s *applicationStateSuite) TestConstraintEmpty(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	cons, err := s.state.GetApplicationConstraints(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cons, jc.DeepEquals, constraints.Constraints{})
}

func (s *applicationStateSuite) TestConstraintsApplicationNotFound(c *gc.C) {
	_, err := s.state.GetApplicationConstraints(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestSetConstraintFull(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

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

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		insertSpace0Stmt := `INSERT INTO space (uuid, name) VALUES (?, ?)`
		_, err := tx.ExecContext(ctx, insertSpace0Stmt, "space0-uuid", "space0")
		if err != nil {
			return err
		}
		insertSpace1Stmt := `INSERT INTO space (uuid, name) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, insertSpace1Stmt, "space1-uuid", "space1")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetApplicationConstraints(context.Background(), id, cons)
	c.Assert(err, jc.ErrorIsNil)

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
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT application_uuid, constraint_uuid FROM application_constraint WHERE application_uuid=?", id).Scan(&applicationUUID, &constraintUUID)
		if err != nil {
			return err
		}

		rows, err := tx.QueryContext(ctx, "SELECT space,exclude FROM constraint_space WHERE constraint_uuid=?", constraintUUID)
		if err != nil {
			return err
		}
		defer rows.Close()
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
		defer rows.Close()
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
		defer rows.Close()
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
	c.Assert(err, jc.ErrorIsNil)

	c.Check(constraintUUID, gc.Not(gc.Equals), "")
	c.Check(applicationUUID, gc.Equals, id.String())

	c.Check(arch, gc.Equals, "amd64")
	c.Check(cpuCores, gc.Equals, 2)
	c.Check(cpuPower, gc.Equals, 42)
	c.Check(mem, gc.Equals, 8)
	c.Check(rootDisk, gc.Equals, 256)
	c.Check(rootDiskSource, gc.Equals, "root-disk-source")
	c.Check(instanceRole, gc.Equals, "instance-role")
	c.Check(instanceType, gc.Equals, "instance-type")
	c.Check(containerTypeID, gc.Equals, 1)
	c.Check(virtType, gc.Equals, "virt-type")
	c.Check(allocatePublicIP, gc.Equals, true)
	c.Check(imageID, gc.Equals, "image-id")

	c.Check(constraintSpaces, jc.DeepEquals, []applicationSpace{
		{SpaceName: "space0", SpaceExclude: false},
		{SpaceName: "space1", SpaceExclude: true},
	})
	c.Check(constraintTags, jc.DeepEquals, []string{"tag0", "tag1"})
	c.Check(constraintZones, jc.DeepEquals, []string{"zone0", "zone1"})

}

func (s *applicationStateSuite) TestSetConstraintInvalidContainerType(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	cons := constraints.Constraints{
		Container: ptr(instance.ContainerType("invalid-container-type")),
	}
	err := s.state.SetApplicationConstraints(context.Background(), id, cons)
	c.Assert(err, jc.ErrorIs, applicationerrors.InvalidApplicationConstraints)
}

func (s *applicationStateSuite) TestSetConstraintInvalidSpace(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	cons := constraints.Constraints{
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "invalid-space", Exclude: false},
		}),
	}
	err := s.state.SetApplicationConstraints(context.Background(), id, cons)
	c.Assert(err, jc.ErrorIs, applicationerrors.InvalidApplicationConstraints)
}

func (s *applicationStateSuite) TestSetConstraintsReplacesPrevious(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.state.SetApplicationConstraints(context.Background(), id, constraints.Constraints{
		Mem:      ptr(uint64(8)),
		CpuCores: ptr(uint64(2)),
	})
	c.Assert(err, jc.ErrorIsNil)

	cons, err := s.state.GetApplicationConstraints(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cons, gc.DeepEquals, constraints.Constraints{
		Mem:      ptr(uint64(8)),
		CpuCores: ptr(uint64(2)),
	})

	err = s.state.SetApplicationConstraints(context.Background(), id, constraints.Constraints{
		CpuPower: ptr(uint64(42)),
	})
	c.Assert(err, jc.ErrorIsNil)

	cons, err = s.state.GetApplicationConstraints(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cons, gc.DeepEquals, constraints.Constraints{
		CpuPower: ptr(uint64(42)),
	})
}

func (s *applicationStateSuite) TestSetConstraintsReplacesPreviousZones(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.state.SetApplicationConstraints(context.Background(), id, constraints.Constraints{
		Zones: ptr([]string{"zone0", "zone1"}),
	})
	c.Assert(err, jc.ErrorIsNil)

	cons, err := s.state.GetApplicationConstraints(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*cons.Zones, jc.SameContents, []string{"zone0", "zone1"})

	err = s.state.SetApplicationConstraints(context.Background(), id, constraints.Constraints{
		Tags: ptr([]string{"tag0", "tag1"}),
	})
	c.Assert(err, jc.ErrorIsNil)

	cons, err = s.state.GetApplicationConstraints(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*cons.Tags, jc.SameContents, []string{"tag0", "tag1"})
}

func (s *applicationStateSuite) TestSetConstraintsReplacesPreviousSameZone(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.state.SetApplicationConstraints(context.Background(), id, constraints.Constraints{
		Zones: ptr([]string{"zone0", "zone1"}),
	})
	c.Assert(err, jc.ErrorIsNil)

	cons, err := s.state.GetApplicationConstraints(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*cons.Zones, jc.SameContents, []string{"zone0", "zone1"})

	err = s.state.SetApplicationConstraints(context.Background(), id, constraints.Constraints{
		Zones: ptr([]string{"zone3"}),
	})
	c.Assert(err, jc.ErrorIsNil)

	cons, err = s.state.GetApplicationConstraints(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*cons.Zones, jc.SameContents, []string{"zone3"})
}

func (s *applicationStateSuite) TestSetConstraintsApplicationNotFound(c *gc.C) {
	err := s.state.SetApplicationConstraints(context.Background(), "foo", constraints.Constraints{Mem: ptr(uint64(8))})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetApplicationCharmOriginEmptyChannel(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM application_channel WHERE application_uuid=?", id)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	origin, err := s.state.GetApplicationCharmOrigin(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(origin, gc.DeepEquals, application.CharmOrigin{
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

func (s *applicationStateSuite) TestGetApplicationCharmOriginRiskOnlyChannel(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application_channel SET track = '', branch = '' WHERE application_uuid=?", id)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	origin, err := s.state.GetApplicationCharmOrigin(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(origin, gc.DeepEquals, application.CharmOrigin{
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

func (s *applicationStateSuite) TestGetApplicationCharmOriginInvalidRisk(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application_channel SET track = '', risk = 'boom', branch = '' WHERE application_uuid=?", id)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.GetApplicationCharmOrigin(context.Background(), id)
	c.Assert(err, gc.ErrorMatches, `decoding channel: decoding risk: unknown risk "boom"`)
}

func (s *applicationStateSuite) TestGetApplicationCharmOriginNoRevision(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	charmUUID, err := s.state.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE charm SET revision = -1 WHERE uuid=?", charmUUID.String())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	origin, err := s.state.GetApplicationCharmOrigin(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(origin, gc.DeepEquals, application.CharmOrigin{
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

func (s *applicationStateSuite) TestGetApplicationCharmOriginNoCharmhubIdentifier(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	charmUUID, err := s.state.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE charm_download_info SET charmhub_identifier = NULL WHERE charm_uuid=?", charmUUID.String())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	origin, err := s.state.GetApplicationCharmOrigin(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(origin, gc.DeepEquals, application.CharmOrigin{
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

func (s *applicationStateSuite) TestGetDeviceConstraintsAppNotFound(c *gc.C) {
	_, err := s.state.GetDeviceConstraints(context.Background(), coreapplication.ID("foo"))
	c.Assert(err, gc.ErrorMatches, applicationerrors.ApplicationNotFound.Error())
}

func (s *applicationStateSuite) TestGetDeviceConstraintsDeadApp(c *gc.C) {
	id := s.createApplication(c, "foo", life.Dead)

	_, err := s.state.GetDeviceConstraints(context.Background(), id)
	c.Assert(err, gc.ErrorMatches, applicationerrors.ApplicationIsDead.Error())
}

func (s *applicationStateSuite) TestGetDeviceConstraints(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Assert(err, jc.ErrorIsNil)

	cons, err := s.state.GetDeviceConstraints(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cons, gc.HasLen, 4)
	// Device constraint added by createApplication().
	c.Check(cons["dev0"].Type, gc.Equals, devices.DeviceType("type0"))
	c.Check(cons["dev0"].Count, gc.Equals, 42)
	c.Check(cons["dev0"].Attributes, gc.DeepEquals, map[string]string{
		"k0": "v0",
		"k1": "v1",
	})
	c.Check(cons["dev1"].Type, gc.Equals, devices.DeviceType("type1"))
	c.Check(cons["dev1"].Count, gc.Equals, 3)
	c.Check(cons["dev1"].Attributes, gc.DeepEquals, map[string]string{"k2": "v2"})
	c.Check(cons["dev2"].Type, gc.Equals, devices.DeviceType("type2"))
	c.Check(cons["dev2"].Count, gc.Equals, 1974)
	c.Check(cons["dev2"].Attributes, gc.DeepEquals, map[string]string{})
	// Device constraint added manually via inserts.
	c.Check(cons["dev3"].Type, gc.Equals, devices.DeviceType("type3"))
	c.Check(cons["dev3"].Count, gc.Equals, 666)
	c.Check(cons["dev3"].Attributes, gc.DeepEquals, map[string]string{"k666": "v666"})
}

func (s *applicationStateSuite) TestGetDeviceConstraintsFromCreatedApp(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	cons, err := s.state.GetDeviceConstraints(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cons, gc.HasLen, 3)
	c.Check(cons["dev0"].Type, gc.Equals, devices.DeviceType("type0"))
	c.Check(cons["dev0"].Count, gc.Equals, 42)
	c.Check(cons["dev0"].Attributes, gc.DeepEquals, map[string]string{
		"k0": "v0",
		"k1": "v1",
	})
	c.Check(cons["dev1"].Type, gc.Equals, devices.DeviceType("type1"))
	c.Check(cons["dev1"].Count, gc.Equals, 3)
	c.Check(cons["dev1"].Attributes, gc.DeepEquals, map[string]string{"k2": "v2"})
	c.Check(cons["dev2"].Type, gc.Equals, devices.DeviceType("type2"))
	c.Check(cons["dev2"].Count, gc.Equals, 1974)
	c.Check(cons["dev2"].Attributes, gc.DeepEquals, map[string]string{})
}

func (s *applicationStateSuite) TestGetAddressesHashEmpty(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
	})

	hash, err := s.state.GetAddressesHash(context.Background(), appID, "net-node-uuid")
	c.Assert(err, jc.ErrorIsNil)
	// The resulting hash is not the empty string because it always contains
	// the default bindings.
	c.Check(hash, gc.Equals, "5ec8be1eeb06c2f67dc76a85843d4461bd51668aab3f27df2af8b3e89a28d703")
}

func (s *applicationStateSuite) TestGetAddressesHash(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
	})

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
		insertIPAddress := `INSERT INTO ip_address (uuid, device_uuid, address_value, type_id, scope_id, origin_id, config_type_id, subnet_uuid) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertIPAddress, "ip-address-uuid", "lld-uuid", "10.0.0.1", 0, 0, 0, 0, "subnet-uuid")
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	hash, err := s.state.GetAddressesHash(context.Background(), appID, "net-node-uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(hash, gc.Equals, "7166b95ec684e8452e796e2d82bfa7c6f74c7597a4c56af8d763c4be4fcefc77")
}

func (s *applicationStateSuite) TestGetAddressesHashWithEndpointBindings(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
	})

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
		insertIPAddress := `INSERT INTO ip_address (uuid, device_uuid, address_value, type_id, scope_id, origin_id, config_type_id, subnet_uuid) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertIPAddress, "ip-address-uuid", "lld-uuid", "10.0.0.1", 0, 0, 0, 0, "subnet-uuid")
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
	c.Assert(err, jc.ErrorIsNil)

	hash, err := s.state.GetAddressesHash(context.Background(), appID, "net-node-uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(hash, gc.Equals, "2b94c712836ade07adcac5d9742f7a77e989d74893168bdbf9aa956f670c8290")
}

func (s *applicationStateSuite) TestGetAddressesHashCloudService(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
	})

	err := s.state.UpsertCloudService(context.Background(), "foo", "provider-id", network.NewSpaceAddresses("10.0.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	var netNodeUUID string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT net_node_uuid FROM k8s_service WHERE application_uuid=?", appID).Scan(&netNodeUUID)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	hash, err := s.state.GetAddressesHash(context.Background(), appID, netNodeUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(hash, gc.Equals, "aba7ff161442bff8b0b29c4a88599eeb32eaa89b3e53d80a35ee8b4efd367d14")
}

func (s *applicationStateSuite) TestGetAddressesHashCloudServiceWithEndpointBindings(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
	})

	err := s.state.UpsertCloudService(context.Background(), "foo", "provider-id", network.NewSpaceAddresses("10.0.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	var netNodeUUID string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Assert(err, jc.ErrorIsNil)

	hash, err := s.state.GetAddressesHash(context.Background(), appID, netNodeUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(hash, gc.Equals, "7b3da67858305e4297f5cc6968a34e383af675dff6bb85a15c95ff39923ad31d")
}

func (s *applicationStateSuite) TestHashAddresses(c *gc.C) {
	hash, err := s.state.hashAddressesAndEndpoints(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(hash, gc.Equals, "")

	hash0, err := s.state.hashAddressesAndEndpoints([]spaceAddress{
		{
			Value: "10.0.0.1",
		},
		{
			Value: "10.0.0.2",
		},
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	hash1, err := s.state.hashAddressesAndEndpoints([]spaceAddress{
		{
			Value: "10.0.0.2",
		},
		{
			Value: "10.0.0.1",
		},
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	// The hash should be consistent regardless of the order of the addresses.
	c.Check(hash0, gc.Equals, hash1)

	hash0, err = s.state.hashAddressesAndEndpoints([]spaceAddress{}, map[string]string{
		"foo": "bar",
		"foz": "baz",
	})
	c.Assert(err, jc.ErrorIsNil)
	hash1, err = s.state.hashAddressesAndEndpoints([]spaceAddress{}, map[string]string{
		"foz": "baz",
		"foo": "bar",
	})
	c.Assert(err, jc.ErrorIsNil)
	// The hash should be consistent regardless of the order of the endpoint
	// bindings.
	c.Check(hash0, gc.Equals, hash1)
}

func (s *applicationStateSuite) TestGetNetNodeFromK8sService(c *gc.C) {
	_ = s.createApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
	})

	err := s.state.UpsertCloudService(context.Background(), "foo", "provider-id", network.NewSpaceAddresses("10.0.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	// Also insert the unit net node to make sure the k8s service one is
	// returned.
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check the k8s service net node is returned (since the uuid is generated
	// we check that the unit net node uuid, which is manually crafted, is not
	// returned).
	netNode, err := s.state.GetNetNodeUUIDByUnitName(context.Background(), "foo/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(netNode, gc.Not(gc.Equals), "net-node-uuid")
}

func (s *applicationStateSuite) TestGetNetNodeFromUnit(c *gc.C) {
	_ = s.createApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
	})

	// Insert the unit net node to make sure the k8s service one is
	// returned.
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check the unit net node is returned.
	netNode, err := s.state.GetNetNodeUUIDByUnitName(context.Background(), "foo/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(netNode, gc.Equals, "net-node-uuid")
}

func (s *applicationStateSuite) TestGetNetNodeUnitNotFound(c *gc.C) {
	_, err := s.state.GetNetNodeUUIDByUnitName(context.Background(), "foo/0")
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationStateSuite) addCharmModifiedVersion(c *gc.C, appID coreapplication.ID, charmModifiedVersion int) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application SET charm_modified_version = ? WHERE uuid = ?", charmModifiedVersion, appID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationStateSuite) insertApplicationConfigWithDefault(c *gc.C, appID coreapplication.ID, key, value, defaultValue string, optionType charm.OptionType) {
	t, err := encodeConfigType(optionType)
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO application_config (application_uuid, key, value, type_id) VALUES (?, ?, ?, ?)
`, appID, key, value, t)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	s.insertCharmConfig(c, appID, key, defaultValue, optionType)
}

func (s *applicationStateSuite) insertCharmConfig(c *gc.C, appID coreapplication.ID, key, defaultValue string, optionType charm.OptionType) {
	t, err := encodeConfigType(optionType)
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_config (charm_uuid, key, default_value, type_id)
SELECT charm_uuid, ?, ?, ?
FROM application
WHERE uuid = ?
`, key, defaultValue, t, appID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *baseSuite) assertPeerRelation(c *gc.C, appName string, peerRelationInput map[string]int) {
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
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
		defer rows.Close()
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
	c.Assert(err, jc.ErrorIsNil)

	c.Check(peerRelations, jc.SameContents, expected)
}
