// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/resource/testing"
	coreunit "github.com/juju/juju/core/unit"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/linklayerdevice"
	portstate "github.com/juju/juju/domain/port/state"
	"github.com/juju/juju/domain/resource"
	domainstorage "github.com/juju/juju/domain/storage"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type applicationStateSuite struct {
	baseSuite

	state *State
}

var _ = gc.Suite(&applicationStateSuite{})

func (s *applicationStateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *applicationStateSuite) TestGetModelType(c *gc.C) {
	modelUUID := uuid.MustNewUUID()
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, target_agent_version, name, type, cloud, cloud_type)
			VALUES (?, ?, ?, "test", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id(), jujuversion.Current.String())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	mt, err := s.state.GetModelType(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mt, gc.Equals, coremodel.ModelType("iaas"))
}

func (s *applicationStateSuite) TestCreateApplication(c *gc.C) {
	platform := application.Platform{
		Channel:      "666",
		OSType:       application.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &application.Channel{
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
	status, err := s.state.GetApplicationStatus(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status, gc.DeepEquals, &application.StatusInfo[application.WorkloadStatusType]{
		Status: application.WorkloadStatusUnset,
	})
}

func (s *applicationStateSuite) TestCreateApplicationWithConfigAndSettings(c *gc.C) {
	platform := application.Platform{
		Channel:      "666",
		OSType:       application.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &application.Channel{
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

func (s *applicationStateSuite) TestCreateApplicationWithStatus(c *gc.C) {
	platform := application.Platform{
		Channel:      "666",
		OSType:       application.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &application.Channel{
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
		Status: &application.StatusInfo[application.WorkloadStatusType]{
			Status:  application.WorkloadStatusActive,
			Message: "test",
			Data:    []byte(`{"foo": "bar"}`),
			Since:   ptr(now),
		},
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, channel, scale, false)

	// Status should be unset.
	status, err := s.state.GetApplicationStatus(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status, gc.DeepEquals, &application.StatusInfo[application.WorkloadStatusType]{
		Status:  application.WorkloadStatusActive,
		Message: "test",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(now),
	})
}

func (s *applicationStateSuite) TestCreateApplicationWithUnits(c *gc.C) {
	platform := application.Platform{
		Channel:      "666",
		OSType:       application.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &application.Channel{
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
		UnitName: "foo/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &application.StatusInfo[application.UnitAgentStatusType]{
				Status:  application.UnitAgentStatusExecuting,
				Message: "test",
				Data:    []byte(`{"foo": "bar"}`),
				Since:   ptr(time.Now()),
			},
			WorkloadStatus: &application.StatusInfo[application.WorkloadStatusType]{
				Status:  application.WorkloadStatusActive,
				Message: "test",
				Data:    []byte(`{"foo": "bar"}`),
				Since:   ptr(time.Now()),
			},
		},
	}}
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "666", a, us)
	c.Assert(err, jc.ErrorIsNil)
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, channel, scale, false)
}

func (s *applicationStateSuite) TestCreateApplicationsWithSameCharm(c *gc.C) {
	platform := application.Platform{
		Channel:      "666",
		OSType:       application.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &application.Channel{
		Track:  "track",
		Risk:   "stable",
		Branch: "branch",
	}
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "foo1", application.AddApplicationArg{
		Platform: platform,
		Channel:  channel,
		Charm: charm.Charm{
			Metadata:     s.minimalMetadata(c, "foo"),
			Manifest:     s.minimalManifest(c),
			Source:       charm.LocalSource,
			Revision:     42,
			Architecture: architecture.ARM64,
		},
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.CreateApplication(ctx, "foo2", application.AddApplicationArg{
		Platform: platform,
		Channel:  channel,
		Charm: charm.Charm{
			Metadata:     s.minimalMetadata(c, "foo"),
			Manifest:     s.minimalManifest(c),
			Source:       charm.LocalSource,
			Revision:     42,
			Architecture: architecture.ARM64,
		},
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	scale := application.ScaleState{}
	s.assertApplication(c, "foo1", platform, channel, scale, false)
	s.assertApplication(c, "foo2", platform, channel, scale, false)
}

func (s *applicationStateSuite) TestCreateApplicationWithoutChannel(c *gc.C) {
	platform := application.Platform{
		Channel:      "666",
		OSType:       application.Ubuntu,
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
	platform := application.Platform{
		Channel:      "666",
		OSType:       application.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &application.Channel{}
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "666", application.AddApplicationArg{
		Platform: platform,
		Charm: charm.Charm{
			Metadata: s.minimalMetadata(c, "666"),
			Manifest: s.minimalManifest(c),
			Source:   charm.LocalSource,
			Revision: 42,
		},
		Scale: 1,
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, channel, scale, false)
}

func (s *applicationStateSuite) TestCreateApplicationWithCharmStoragePath(c *gc.C) {
	platform := application.Platform{
		Channel:      "666",
		OSType:       application.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &application.Channel{}
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "666", application.AddApplicationArg{
		Platform: platform,
		Charm: charm.Charm{
			Metadata:    s.minimalMetadata(c, "666"),
			Manifest:    s.minimalManifest(c),
			Source:      charm.LocalSource,
			Revision:    42,
			ArchivePath: "/some/path",
			Available:   true,
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
FROM v_resource vr
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
FROM v_resource vr
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
FROM v_resource vr
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
FROM v_resource vr
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
		c.Check(err, jc.ErrorIs, sql.ErrNoRows)
		return nil
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

func (s *applicationStateSuite) TestUpsertCloudServiceNotFound(c *gc.C) {
	err := s.state.UpsertCloudService(context.Background(), "foo", "provider-id", network.SpaceAddresses{})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestInsertUnitCloudContainer(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
		CloudContainer: &application.CloudContainer{
			ProviderID: "some-id",
			Ports:      ptr([]string{"666", "667"}),
			Address: ptr(application.ContainerAddress{
				Device: application.ContainerDevice{
					Name:              "placeholder",
					DeviceTypeID:      linklayerdevice.DeviceTypeUnknown,
					VirtualPortTypeID: linklayerdevice.NonVirtualPortType,
				},
				Value:       "10.6.6.6",
				AddressType: ipaddress.AddressTypeIPv4,
				ConfigType:  ipaddress.ConfigTypeDHCP,
				Scope:       ipaddress.ScopeMachineLocal,
				Origin:      ipaddress.OriginHost,
			}),
		},
	}
	ctx := context.Background()

	appID := s.createApplication(c, "foo", life.Alive)
	err := s.state.InsertUnit(ctx, appID, u)
	c.Assert(err, jc.ErrorIsNil)
	s.assertContainerAddressValues(c, "foo/666", "some-id", "10.6.6.6",
		ipaddress.AddressTypeIPv4, ipaddress.OriginHost, ipaddress.ScopeMachineLocal, ipaddress.ConfigTypeDHCP)
	s.assertContainerPortValues(c, "foo/666", []string{"666", "667"})

}

func (s *applicationStateSuite) assertContainerAddressValues(
	c *gc.C,
	unitName, providerID, addressValue string,
	addressType ipaddress.AddressType,
	addressOrigin ipaddress.Origin,
	addressScope ipaddress.Scope,
	configType ipaddress.ConfigType,

) {
	var (
		gotProviderID string
		gotValue      string
		gotType       int
		gotOrigin     int
		gotScope      int
		gotConfigType int
	)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `

SELECT cc.provider_id, a.address_value, a.type_id, a.origin_id,a.scope_id,a.config_type_id
FROM k8s_pod cc
JOIN unit u ON cc.unit_uuid = u.uuid
JOIN link_layer_device lld ON lld.net_node_uuid = u.net_node_uuid
JOIN ip_address a ON a.device_uuid = lld.uuid
WHERE u.name=?`,

			unitName).Scan(
			&gotProviderID,
			&gotValue,
			&gotType,
			&gotOrigin,
			&gotScope,
			&gotConfigType,
		)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotProviderID, gc.Equals, providerID)
	c.Assert(gotValue, gc.Equals, addressValue)
	c.Assert(gotType, gc.Equals, int(addressType))
	c.Assert(gotOrigin, gc.Equals, int(addressOrigin))
	c.Assert(gotScope, gc.Equals, int(addressScope))
	c.Assert(gotConfigType, gc.Equals, int(configType))
}

func (s *applicationStateSuite) assertContainerPortValues(c *gc.C, unitName string, ports []string) {
	var gotPorts []string
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `

SELECT ccp.port
FROM k8s_pod cc
JOIN unit u ON cc.unit_uuid = u.uuid
JOIN k8s_pod_port ccp ON ccp.unit_uuid = cc.unit_uuid
WHERE u.name=?`,

			unitName)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var port string
			if err := rows.Scan(&port); err != nil {
				return err
			}
			gotPorts = append(gotPorts, port)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return rows.Close()
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotPorts, jc.SameContents, ports)
}

func (s *applicationStateSuite) TestUpdateCAASUnitCloudContainer(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
		CloudContainer: &application.CloudContainer{
			ProviderID: "some-id",
			Ports:      ptr([]string{"666", "668"}),
			Address: ptr(application.ContainerAddress{
				Device: application.ContainerDevice{
					Name:              "placeholder",
					DeviceTypeID:      linklayerdevice.DeviceTypeUnknown,
					VirtualPortTypeID: linklayerdevice.NonVirtualPortType,
				},
				Value:       "10.6.6.6",
				AddressType: ipaddress.AddressTypeIPv4,
				ConfigType:  ipaddress.ConfigTypeDHCP,
				Scope:       ipaddress.ScopeMachineLocal,
				Origin:      ipaddress.OriginHost,
			}),
		},
	}
	s.createApplication(c, "foo", life.Alive, u)

	err := s.state.UpdateCAASUnit(context.Background(), "foo/667", application.UpdateCAASUnitParams{})
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)

	cc := application.UpdateCAASUnitParams{
		ProviderID: ptr("another-id"),
		Ports:      ptr([]string{"666", "667"}),
		Address:    ptr("2001:db8::1"),
	}
	err = s.state.UpdateCAASUnit(context.Background(), "foo/666", cc)
	c.Assert(err, jc.ErrorIsNil)

	var (
		providerId string
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err = tx.QueryRowContext(ctx, `

SELECT provider_id FROM k8s_pod cc
JOIN unit u ON cc.unit_uuid = u.uuid
WHERE u.name=?`,

			"foo/666").Scan(&providerId)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(providerId, gc.Equals, "another-id")

	s.assertContainerAddressValues(c, "foo/666", "another-id", "2001:db8::1",
		ipaddress.AddressTypeIPv6, ipaddress.OriginProvider, ipaddress.ScopeMachineLocal, ipaddress.ConfigTypeDHCP)
	s.assertContainerPortValues(c, "foo/666", []string{"666", "667"})
}

func (s *applicationStateSuite) TestUpdateCAASUnitStatuses(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
		CloudContainer: &application.CloudContainer{
			ProviderID: "some-id",
			Ports:      ptr([]string{"666", "668"}),
			Address: ptr(application.ContainerAddress{
				Device: application.ContainerDevice{
					Name:              "placeholder",
					DeviceTypeID:      linklayerdevice.DeviceTypeUnknown,
					VirtualPortTypeID: linklayerdevice.NonVirtualPortType,
				},
				Value:       "10.6.6.6",
				AddressType: ipaddress.AddressTypeIPv4,
				ConfigType:  ipaddress.ConfigTypeDHCP,
				Scope:       ipaddress.ScopeMachineLocal,
				Origin:      ipaddress.OriginHost,
			}),
		},
	}
	s.createApplication(c, "foo", life.Alive, u)

	unitUUID, err := s.state.GetUnitUUIDByName(context.Background(), u.UnitName)
	c.Assert(err, jc.ErrorIsNil)

	now := ptr(time.Now())
	params := application.UpdateCAASUnitParams{
		AgentStatus: ptr(application.StatusInfo[application.UnitAgentStatusType]{
			Status:  application.UnitAgentStatusIdle,
			Message: "agent status",
			Data:    []byte(`{"foo": "bar"}`),
			Since:   now,
		}),
		WorkloadStatus: ptr(application.StatusInfo[application.WorkloadStatusType]{
			Status:  application.WorkloadStatusWaiting,
			Message: "workload status",
			Data:    []byte(`{"foo": "bar"}`),
			Since:   now,
		}),
		CloudContainerStatus: ptr(application.StatusInfo[application.CloudContainerStatusType]{
			Status:  application.CloudContainerStatusRunning,
			Message: "container status",
			Data:    []byte(`{"foo": "bar"}`),
			Since:   now,
		}),
	}
	err = s.state.UpdateCAASUnit(context.Background(), "foo/666", params)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitStatus(
		c, "unit_agent", unitUUID, int(application.UnitAgentStatusIdle), "agent status", now, []byte(`{"foo": "bar"}`),
	)
	s.assertUnitStatus(
		c, "unit_workload", unitUUID, int(application.WorkloadStatusWaiting), "workload status", now, []byte(`{"foo": "bar"}`),
	)
	s.assertUnitStatus(
		c, "k8s_pod", unitUUID, int(application.CloudContainerStatusRunning), "container status", now, []byte(`{"foo": "bar"}`),
	)
}

func (s *applicationStateSuite) TestInsertUnit(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)

	u := application.InsertUnitArg{
		UnitName: "foo/666",
		CloudContainer: &application.CloudContainer{
			ProviderID: "some-id",
		},
	}
	ctx := context.Background()

	err := s.state.InsertUnit(ctx, appID, u)
	c.Assert(err, jc.ErrorIsNil)

	var providerId string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err = tx.QueryRowContext(ctx, `
SELECT provider_id FROM k8s_pod cc
JOIN unit u ON cc.unit_uuid = u.uuid
WHERE u.name=?`,
			"foo/666").Scan(&providerId)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(providerId, gc.Equals, "some-id")

	err = s.state.InsertUnit(ctx, appID, u)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitAlreadyExists)
}

func (s *applicationStateSuite) TestInsertCAASUnit(c *gc.C) {
	appUUID := s.createScalingApplication(c, "foo", life.Alive, 1)

	unitName := coreunit.Name("foo/666")

	p := application.RegisterCAASUnitArg{
		UnitName:     unitName,
		PasswordHash: "passwordhash",
		ProviderID:   "some-id",
		Address:      ptr("10.6.6.6"),
		Ports:        ptr([]string{"666"}),
		OrderedScale: true,
		OrderedId:    0,
	}
	err := s.state.InsertCAASUnit(context.Background(), appUUID, p)
	c.Assert(err, jc.ErrorIsNil)

	var providerId string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err = tx.QueryRowContext(ctx, `
SELECT provider_id FROM k8s_pod cc
JOIN unit u ON cc.unit_uuid = u.uuid
WHERE u.name=?`,
			"foo/666").Scan(&providerId)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(providerId, gc.Equals, "some-id")
}

func (s *applicationStateSuite) TestInsertCAASUnitAlreadyExists(c *gc.C) {
	unitName := coreunit.Name("foo/0")

	_ = s.createApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: unitName,
	})

	p := application.RegisterCAASUnitArg{
		UnitName:     unitName,
		PasswordHash: "passwordhash",
		ProviderID:   "some-id",
		Address:      ptr("10.6.6.6"),
		Ports:        ptr([]string{"666"}),
		OrderedScale: true,
		OrderedId:    0,
	}
	err := s.state.InsertCAASUnit(context.Background(), "foo", p)
	c.Assert(err, jc.ErrorIsNil)

	var (
		providerId   string
		passwordHash string
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err = tx.QueryRowContext(ctx, `
SELECT provider_id FROM k8s_pod cc
JOIN unit u ON cc.unit_uuid = u.uuid
WHERE u.name=?`,
			"foo/0").Scan(&providerId)
		if err != nil {
			return err
		}

		err = tx.QueryRowContext(ctx, `
SELECT password_hash FROM unit
WHERE unit.name=?`,
			"foo/0").Scan(&passwordHash)

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(providerId, gc.Equals, "some-id")
	c.Check(passwordHash, gc.Equals, "passwordhash")
}

func (s *applicationStateSuite) TestSetUnitPassword(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	appID := s.createApplication(c, "foo", life.Alive)
	unitUUID := s.addUnit(c, appID, u)

	err := s.state.SetUnitPassword(context.Background(), unitUUID, application.PasswordInfo{
		PasswordHash: "secret",
	})
	c.Assert(err, jc.ErrorIsNil)

	var (
		password    string
		algorithmID int
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err = tx.QueryRowContext(ctx, `

SELECT password_hash, password_hash_algorithm_id FROM unit u
WHERE u.name=?`,

			"foo/666").Scan(&password, &algorithmID)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(password, gc.Equals, "secret")
	c.Check(algorithmID, gc.Equals, 0)
}

func (s *applicationStateSuite) TestGetUnitLife(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u)

	unitLife, err := s.state.GetUnitLife(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(unitLife, gc.Equals, life.Alive)
}

func (s *applicationStateSuite) TestGetUnitLifeNotFound(c *gc.C) {
	_, err := s.state.GetUnitLife(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationStateSuite) TestSetUnitLife(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	ctx := context.Background()
	s.createApplication(c, "foo", life.Alive, u)

	checkResult := func(want life.Life) {
		var gotLife life.Life
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			err := tx.QueryRowContext(ctx, "SELECT life_id FROM unit WHERE name=?", u.UnitName).
				Scan(&gotLife)
			return err
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(gotLife, jc.DeepEquals, want)
	}

	err := s.state.SetUnitLife(ctx, "foo/666", life.Dying)
	c.Assert(err, jc.ErrorIsNil)
	checkResult(life.Dying)

	err = s.state.SetUnitLife(ctx, "foo/666", life.Dead)
	c.Assert(err, jc.ErrorIsNil)
	checkResult(life.Dead)

	// Can't go backwards.
	err = s.state.SetUnitLife(ctx, "foo/666", life.Dying)
	c.Assert(err, jc.ErrorIsNil)
	checkResult(life.Dead)
}

func (s *applicationStateSuite) TestSetUnitLifeNotFound(c *gc.C) {
	err := s.state.SetUnitLife(context.Background(), "foo/666", life.Dying)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationStateSuite) TestDeleteUnit(c *gc.C) {
	// TODO(units) - add references to agents etc when those are fully cooked
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
		CloudContainer: &application.CloudContainer{
			ProviderID: "provider-id",
			Ports:      ptr([]string{"666", "668"}),
			Address: ptr(application.ContainerAddress{
				Device: application.ContainerDevice{
					Name:              "placeholder",
					DeviceTypeID:      linklayerdevice.DeviceTypeUnknown,
					VirtualPortTypeID: linklayerdevice.NonVirtualPortType,
				},
				Value:       "10.6.6.6",
				AddressType: ipaddress.AddressTypeIPv4,
				ConfigType:  ipaddress.ConfigTypeDHCP,
				Scope:       ipaddress.ScopeMachineLocal,
				Origin:      ipaddress.OriginHost,
			}),
		},
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &application.StatusInfo[application.UnitAgentStatusType]{
				Status:  application.UnitAgentStatusExecuting,
				Message: "test",
				Data:    []byte(`{"foo": "bar"}`),
				Since:   ptr(time.Now()),
			},
			WorkloadStatus: &application.StatusInfo[application.WorkloadStatusType]{
				Status:  application.WorkloadStatusActive,
				Message: "test",
				Data:    []byte(`{"foo": "bar"}`),
				Since:   ptr(time.Now()),
			},
		},
	}
	u2 := application.InsertUnitArg{
		UnitName: "foo/667",
	}
	s.createApplication(c, "foo", life.Alive, u1, u2)
	var (
		unitUUID    coreunit.UUID
		netNodeUUID string
		deviceUUID  string
	)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id=2 WHERE name=?", u1.UnitName); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT uuid, net_node_uuid FROM unit WHERE name=?", u1.UnitName).Scan(&unitUUID, &netNodeUUID); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT uuid FROM link_layer_device WHERE net_node_uuid=?", netNodeUUID).Scan(&deviceUUID); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		if err := s.state.setCloudContainerStatus(ctx, tx, unitUUID, &application.StatusInfo[application.CloudContainerStatusType]{
			Status:  application.CloudContainerStatusBlocked,
			Message: "test",
			Data:    []byte(`{"foo": "bar"}`),
			Since:   ptr(time.Now()),
		}); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	portSt := portstate.NewState(s.TxnRunnerFactory())
	err = portSt.UpdateUnitPorts(context.Background(), unitUUID, network.GroupedPortRanges{
		"endpoint": {
			{Protocol: "tcp", FromPort: 80, ToPort: 80},
			{Protocol: "udp", FromPort: 1000, ToPort: 1500},
		},
		"misc": {
			{Protocol: "tcp", FromPort: 8080, ToPort: 8080},
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)

	gotIsLast, err := s.state.DeleteUnit(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotIsLast, jc.IsFalse)

	var (
		unitCount                 int
		containerCount            int
		deviceCount               int
		addressCount              int
		portCount                 int
		agentStatusCount          int
		workloadStatusCount       int
		cloudContainerStatusCount int
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit WHERE name=?", u1.UnitName).Scan(&unitCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM k8s_pod WHERE unit_uuid=?", unitUUID).Scan(&containerCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM link_layer_device WHERE net_node_uuid=?", netNodeUUID).Scan(&deviceCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM ip_address WHERE device_uuid=?", deviceUUID).Scan(&addressCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM k8s_pod_port WHERE unit_uuid=?", unitUUID).Scan(&portCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit_agent_status WHERE unit_uuid=?", unitUUID).Scan(&agentStatusCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit_workload_status WHERE unit_uuid=?", unitUUID).Scan(&workloadStatusCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM k8s_pod_status WHERE unit_uuid=?", unitUUID).Scan(&cloudContainerStatusCount); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(addressCount, gc.Equals, 0)
	c.Check(portCount, gc.Equals, 0)
	c.Check(deviceCount, gc.Equals, 0)
	c.Check(containerCount, gc.Equals, 0)
	c.Check(agentStatusCount, gc.Equals, 0)
	c.Check(workloadStatusCount, gc.Equals, 0)
	c.Check(cloudContainerStatusCount, gc.Equals, 0)
	c.Check(unitCount, gc.Equals, 0)
}

func (s *applicationStateSuite) TestDeleteUnitLastUnitAppAlive(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id=2 WHERE name=?", u1.UnitName); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	gotIsLast, err := s.state.DeleteUnit(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotIsLast, jc.IsFalse)

	var unitCount int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit WHERE name=?", u1.UnitName).
			Scan(&unitCount); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitCount, gc.Equals, 0)
}

func (s *applicationStateSuite) TestDeleteUnitLastUnit(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Dying, u1)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id=2 WHERE name=?", u1.UnitName); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	gotIsLast, err := s.state.DeleteUnit(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotIsLast, jc.IsTrue)

	var unitCount int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit WHERE name=?", u1.UnitName).
			Scan(&unitCount); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitCount, gc.Equals, 0)
}

func (s *applicationStateSuite) TestGetUnitUUIDByName(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	_ = s.createApplication(c, "foo", life.Alive, u1)

	unitUUID, err := s.state.GetUnitUUIDByName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitUUID, gc.NotNil)
}

func (s *applicationStateSuite) TestGetUnitUUIDByNameNotFound(c *gc.C) {
	_, err := s.state.GetUnitUUIDByName(context.Background(), "failme")
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
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

func (s *applicationStateSuite) assertUnitStatus(c *gc.C, statusType, unitUUID coreunit.UUID, statusID int, message string, since *time.Time, data []byte) {
	var (
		gotStatusID int
		gotMessage  string
		gotSince    *time.Time
		gotData     []byte
	)
	queryInfo := fmt.Sprintf(`SELECT status_id, message, data, updated_at FROM %s_status WHERE unit_uuid = ?`, statusType)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, queryInfo, unitUUID).
			Scan(&gotStatusID, &gotMessage, &gotData, &gotSince); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotStatusID, gc.Equals, statusID)
	c.Check(gotMessage, gc.Equals, message)
	c.Check(gotSince, jc.DeepEquals, since)
	c.Check(gotData, jc.DeepEquals, data)
}

func (s *applicationStateSuite) TestSetCloudContainerStatus(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)

	status := application.StatusInfo[application.CloudContainerStatusType]{
		Status:  application.CloudContainerStatusRunning,
		Message: "it's running",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	unitUUID, err := s.state.GetUnitUUIDByName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.setCloudContainerStatus(ctx, tx, unitUUID, &status)
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitStatus(
		c, "k8s_pod", unitUUID, int(status.Status), status.Message, status.Since, status.Data)
}

func (s *applicationStateSuite) TestSetUnitAgentStatus(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)

	status := application.StatusInfo[application.UnitAgentStatusType]{
		Status:  application.UnitAgentStatusExecuting,
		Message: "it's executing",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	unitUUID, err := s.state.GetUnitUUIDByName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.setUnitAgentStatus(ctx, tx, unitUUID, &status)
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitStatus(
		c, "unit_agent", unitUUID, int(status.Status), status.Message, status.Since, status.Data)
}

func (s *applicationStateSuite) TestSetWorkloadStatus(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)

	unitUUID, err := s.state.GetUnitUUIDByName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.GetUnitWorkloadStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitStatusNotFound)

	status := &application.StatusInfo[application.WorkloadStatusType]{
		Status:  application.WorkloadStatusActive,
		Message: "it's active!",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err = s.state.SetUnitWorkloadStatus(context.Background(), unitUUID, status)
	c.Assert(err, jc.ErrorIsNil)

	gotStatus, err := s.state.GetUnitWorkloadStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	assertStatusInfoEqual(c, gotStatus, status)

	// Run SetUnitWorkloadStatus followed by GetUnitWorkloadStatus to ensure that
	// the new status overwrites the old one.
	status = &application.StatusInfo[application.WorkloadStatusType]{
		Status:  application.WorkloadStatusTerminated,
		Message: "it's terminated",
		Data:    []byte(`{"bar": "foo"}`),
		Since:   ptr(time.Now()),
	}

	err = s.state.SetUnitWorkloadStatus(context.Background(), unitUUID, status)
	c.Assert(err, jc.ErrorIsNil)

	gotStatus, err = s.state.GetUnitWorkloadStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	assertStatusInfoEqual(c, gotStatus, status)
}

func (s *applicationStateSuite) TestSetUnitWorkloadStatusNotFound(c *gc.C) {
	status := application.StatusInfo[application.WorkloadStatusType]{
		Status:  application.WorkloadStatusTerminated,
		Message: "it's terminated",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err := s.state.SetUnitWorkloadStatus(context.Background(), "missing-uuid", &status)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationStateSuite) TestGetUnitWorkloadStatusesForApplication(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	appId := s.createApplication(c, "foo", life.Alive, u1)

	unitUUID, err := s.state.GetUnitUUIDByName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)

	status := &application.StatusInfo[application.WorkloadStatusType]{
		Status:  application.WorkloadStatusActive,
		Message: "it's active!",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}
	err = s.state.SetUnitWorkloadStatus(context.Background(), unitUUID, status)
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.state.GetUnitWorkloadStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, gc.HasLen, 1)
	result, ok := results[unitUUID]
	c.Assert(ok, jc.IsTrue)
	assertStatusInfoEqual(c, &result, status)
}

func (s *applicationStateSuite) TestGetUnitWorkloadStatusesForApplicationMultipleUnits(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	u2 := application.InsertUnitArg{
		UnitName: "foo/667",
	}
	appId := s.createApplication(c, "foo", life.Alive, u1, u2)

	unitUUID1, err := s.state.GetUnitUUIDByName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)
	unitUUID2, err := s.state.GetUnitUUIDByName(context.Background(), u2.UnitName)
	c.Assert(err, jc.ErrorIsNil)

	status1 := &application.StatusInfo[application.WorkloadStatusType]{
		Status:  application.WorkloadStatusActive,
		Message: "it's active!",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}
	err = s.state.SetUnitWorkloadStatus(context.Background(), unitUUID1, status1)
	c.Assert(err, jc.ErrorIsNil)

	status2 := &application.StatusInfo[application.WorkloadStatusType]{
		Status:  application.WorkloadStatusTerminated,
		Message: "it's terminated",
		Data:    []byte(`{"bar": "foo"}`),
		Since:   ptr(time.Now()),
	}
	err = s.state.SetUnitWorkloadStatus(context.Background(), unitUUID2, status2)
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.state.GetUnitWorkloadStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 2)

	result1, ok := results[unitUUID1]
	c.Assert(ok, jc.IsTrue)
	assertStatusInfoEqual(c, &result1, status1)

	result2, ok := results[unitUUID2]
	c.Assert(ok, jc.IsTrue)
	assertStatusInfoEqual(c, &result2, status2)
}

func (s *applicationStateSuite) TestGetUnitWorkloadStatusesForApplicationNotFound(c *gc.C) {
	_, err := s.state.GetUnitWorkloadStatusesForApplication(context.Background(), "missing")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetUnitWorkloadStatusesForApplicationNoUnits(c *gc.C) {
	appId := s.createApplication(c, "foo", life.Alive)

	results, err := s.state.GetUnitWorkloadStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 0)
}

func (s *applicationStateSuite) TestGetApplicationScaleState(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	appID := s.createApplication(c, "foo", life.Alive, u)

	scaleState, err := s.state.GetApplicationScaleState(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(scaleState, jc.DeepEquals, application.ScaleState{
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
	c.Assert(gotScale, jc.DeepEquals, 666)
}

func (s *applicationStateSuite) TestSetApplicationScalingState(c *gc.C) {
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

	err = s.state.SetApplicationScalingState(context.Background(), appID, nil, 668, true)
	c.Assert(err, jc.ErrorIsNil)
	checkResult(application.ScaleState{
		Scale:       666,
		ScaleTarget: 668,
		Scaling:     true,
	})

	err = s.state.SetApplicationScalingState(context.Background(), appID, ptr(667), 668, true)
	c.Assert(err, jc.ErrorIsNil)
	checkResult(application.ScaleState{
		Scale:       667,
		ScaleTarget: 668,
		Scaling:     true,
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
		appCount      int
		platformCount int
		channelCount  int
		scaleCount    int
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
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(appCount, gc.Equals, 0)
	c.Check(platformCount, gc.Equals, 0)
	c.Check(channelCount, gc.Equals, 0)
	c.Check(scaleCount, gc.Equals, 0)
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

func (s *applicationStateSuite) TestAddUnits(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)

	now := ptr(time.Now())
	u := application.AddUnitArg{
		UnitName: "foo/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &application.StatusInfo[application.UnitAgentStatusType]{
				Status:  application.UnitAgentStatusExecuting,
				Message: "test",
				Data:    []byte(`{"foo": "bar"}`),
				Since:   now,
			},
			WorkloadStatus: &application.StatusInfo[application.WorkloadStatusType]{
				Status:  application.WorkloadStatusActive,
				Message: "test",
				Data:    []byte(`{"foo": "bar"}`),
				Since:   now,
			},
		},
	}
	ctx := context.Background()

	err := s.state.AddUnits(ctx, appID, []application.AddUnitArg{u})
	c.Assert(err, jc.ErrorIsNil)

	var (
		unitUUID, unitName string
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT uuid, name FROM unit WHERE application_uuid=?", appID).Scan(&unitUUID, &unitName)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(unitName, gc.Equals, "foo/666")
	s.assertUnitStatus(
		c, "unit_agent", coreunit.UUID(unitUUID),
		int(u.UnitStatusArg.AgentStatus.Status), u.UnitStatusArg.AgentStatus.Message,
		u.UnitStatusArg.AgentStatus.Since, u.UnitStatusArg.AgentStatus.Data)
	s.assertUnitStatus(
		c, "unit_workload", coreunit.UUID(unitUUID),
		int(u.UnitStatusArg.WorkloadStatus.Status), u.UnitStatusArg.WorkloadStatus.Message,
		u.UnitStatusArg.WorkloadStatus.Since, u.UnitStatusArg.WorkloadStatus.Data)
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

func (s *applicationStateSuite) TestInitialWatchStatementUnitLife(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	u2 := application.InsertUnitArg{
		UnitName: "foo/667",
	}
	s.createApplication(c, "foo", life.Alive, u1, u2)

	var unitID1, unitID2 string
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/666").Scan(&unitID1); err != nil {
			return err
		}
		err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/667").Scan(&unitID2)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	table, queryFunc := s.state.InitialWatchStatementUnitLife("foo")
	c.Assert(table, gc.Equals, "unit")

	result, err := queryFunc(context.Background(), s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.SameContents, []string{unitID1, unitID2})
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
		MinJujuVersion: version.MustParse("4.0.0"),
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
		MinJujuVersion: version.MustParse("4.0.0"),
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
	platform := application.Platform{
		OSType:       application.Ubuntu,
		Architecture: architecture.AMD64,
		Channel:      "22.04",
	}
	ctx := context.Background()

	appID, err := s.state.CreateApplication(ctx, "foo", application.AddApplicationArg{
		Charm: charm.Charm{
			Metadata:     expectedMetadata,
			Manifest:     expectedManifest,
			Actions:      expectedActions,
			Config:       expectedConfig,
			LXDProfile:   expectedLXDProfile,
			Source:       charm.LocalSource,
			Revision:     42,
			Architecture: architecture.AMD64,
		},
		Channel: &application.Channel{
			Track:  "track",
			Risk:   "stable",
			Branch: "branch",
		},
		Platform: platform,
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	ch, err := s.state.GetCharmByApplicationID(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ch, gc.DeepEquals, charm.Charm{
		Metadata:     expectedMetadata,
		Manifest:     expectedManifest,
		Actions:      expectedActions,
		Config:       expectedConfig,
		LXDProfile:   expectedLXDProfile,
		Source:       charm.LocalSource,
		Revision:     42,
		Architecture: architecture.AMD64,
	})

	// Ensure that the charm platform is also set AND it's the same as the
	// application platform.
	var gotPlatform application.Platform
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
			Metadata:     expectedMetadata,
			Manifest:     expectedManifest,
			Actions:      expectedActions,
			Config:       expectedConfig,
			Revision:     42,
			Source:       charm.LocalSource,
			Architecture: architecture.AMD64,
		},
		Platform: application.Platform{
			OSType:       application.Ubuntu,
			Architecture: architecture.AMD64,
			Channel:      "22.04",
		},
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	ch, err := s.state.GetCharmByApplicationID(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ch, gc.DeepEquals, charm.Charm{
		Metadata:     expectedMetadata,
		Manifest:     expectedManifest,
		Actions:      expectedActions,
		Config:       expectedConfig,
		Revision:     42,
		Source:       charm.LocalSource,
		Architecture: architecture.AMD64,
	})
}

func (s *applicationStateSuite) TestSetCharmThenGetCharmByApplicationNameInvalidName(c *gc.C) {
	expectedMetadata := charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: version.MustParse("4.0.0"),
		Assumes:        []byte("null"),
	}
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "foo", application.AddApplicationArg{
		Charm: charm.Charm{
			Metadata: expectedMetadata,
			Manifest: s.minimalManifest(c),
			Source:   charm.LocalSource,
		},
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	id := applicationtesting.GenApplicationUUID(c)

	_, err = s.state.GetCharmByApplicationID(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestCheckCharmExistsNotFound(c *gc.C) {
	id := uuid.MustNewUUID().String()
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
	platform := application.Platform{
		Channel:      "22.04/stable",
		OSType:       application.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &application.Channel{
		Risk: application.RiskStable,
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
			Channel: application.Channel{
				Track:  "track",
				Risk:   "stable",
				Branch: "branch",
			},
			Platform: application.Platform{
				Channel:      "22.04/stable",
				OSType:       application.Ubuntu,
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
			Channel: application.Channel{
				Track:  "track",
				Risk:   "stable",
				Branch: "branch",
			},
			Platform: application.Platform{
				Channel:      "22.04/stable",
				OSType:       application.Ubuntu,
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

func (s *applicationStateSuite) TestSetApplicationConfigAndSettings(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	charmID, err := s.state.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetApplicationConfigAndSettings(context.Background(), id, charmID, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: "value",
		},
	}, application.ApplicationSettings{})
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

func (s *applicationStateSuite) TestSetApplicationConfigAndSettingsApplicationIsDead(c *gc.C) {
	id := s.createApplication(c, "foo", life.Dead)

	charmID, err := s.state.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetApplicationConfigAndSettings(context.Background(), id, charmID, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: "value",
		},
	}, application.ApplicationSettings{})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationIsDead)
}

func (s *applicationStateSuite) TestSetApplicationConfigAndSettingsChangesType(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	charmID, err := s.state.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetApplicationConfigAndSettings(context.Background(), id, charmID, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: "value",
		},
	}, application.ApplicationSettings{})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetApplicationConfigAndSettings(context.Background(), id, charmID, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionInt,
			Value: 2,
		},
	}, application.ApplicationSettings{})
	c.Assert(err, jc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, jc.DeepEquals, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionInt,
			Value: "2",
		},
	})
	c.Check(settings, jc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestSetApplicationConfigAndSettingsChangesIdempotent(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	charmID, err := s.state.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetApplicationConfigAndSettings(context.Background(), id, charmID, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: "value",
		},
	}, application.ApplicationSettings{})
	c.Assert(err, jc.ErrorIsNil)

	// The second call should not perform any updates, although it will still
	// write the trust setting.

	err = s.state.SetApplicationConfigAndSettings(context.Background(), id, charmID, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: "value",
		},
	}, application.ApplicationSettings{})
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

func (s *applicationStateSuite) TestSetApplicationConfigAndSettingsNoApplication(c *gc.C) {
	// If the application is not found, it should return application not found.
	id := applicationtesting.GenApplicationUUID(c)
	charmID := charmtesting.GenCharmID(c)
	err := s.state.SetApplicationConfigAndSettings(context.Background(), id, charmID, map[string]application.ApplicationConfig{
		"key": {
			Type:  charm.OptionString,
			Value: "value",
		},
	}, application.ApplicationSettings{})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestSetApplicationConfigAndSettingsUpdatesRemoves(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	charmID, err := s.state.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetApplicationConfigAndSettings(context.Background(), id, charmID, map[string]application.ApplicationConfig{
		"a": {
			Type:  charm.OptionString,
			Value: "b",
		},
		"c": {
			Type:  charm.OptionString,
			Value: "d1",
		},
	}, application.ApplicationSettings{})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetApplicationConfigAndSettings(context.Background(), id, charmID, map[string]application.ApplicationConfig{
		"c": {
			Type:  charm.OptionString,
			Value: "d2",
		},
	}, application.ApplicationSettings{
		Trust: true,
	})
	c.Assert(err, jc.ErrorIsNil)

	config, settings, err := s.state.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, jc.DeepEquals, map[string]application.ApplicationConfig{
		"c": {
			Type:  charm.OptionString,
			Value: "d2",
		},
	})
	c.Check(settings, jc.DeepEquals, application.ApplicationSettings{
		Trust: true,
	})

	err = s.state.SetApplicationConfigAndSettings(context.Background(), id, charmID, map[string]application.ApplicationConfig{}, application.ApplicationSettings{})
	c.Assert(err, jc.ErrorIsNil)

	config, settings, err = s.state.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, gc.HasLen, 0)
	c.Check(settings, jc.DeepEquals, application.ApplicationSettings{})
}

func (s *applicationStateSuite) TestUnsetApplicationConfigKeys(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	charmID, err := s.state.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetApplicationConfigAndSettings(context.Background(), id, charmID, map[string]application.ApplicationConfig{
		"a": {
			Type:  charm.OptionString,
			Value: "b",
		},
		"c": {
			Type:  charm.OptionString,
			Value: "d1",
		},
	}, application.ApplicationSettings{})
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

	charmID, err := s.state.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetApplicationConfigAndSettings(context.Background(), id, charmID,
		map[string]application.ApplicationConfig{},
		application.ApplicationSettings{Trust: true},
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

	charmID, err := s.state.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetApplicationConfigAndSettings(context.Background(), id, charmID, map[string]application.ApplicationConfig{
		"a": {
			Type:  charm.OptionString,
			Value: "b",
		},
		"c": {
			Type:  charm.OptionString,
			Value: "d1",
		},
	}, application.ApplicationSettings{})
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
		return s.state.checkApplicationCharm(context.Background(), tx, applicationID{ID: id}, charmID{UUID: cid.String()})
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
		config   map[string]application.ApplicationConfig
		settings application.ApplicationSettings
		expected string
	}{{
		name:     "empty",
		config:   map[string]application.ApplicationConfig{},
		settings: application.ApplicationSettings{},
		expected: "fcbcf165908dd18a9e49f7ff27810176db8e9f63b4352213741664245224f8aa",
	}, {
		name: "config",
		config: map[string]application.ApplicationConfig{
			"key": {
				Type:  charm.OptionString,
				Value: "value",
			},
		},
		settings: application.ApplicationSettings{},
		expected: "6e1b3adca7459d700abb8e270b06ee7fc96f83436bb533ad4540a3a6eb66cf1b",
	}, {
		name: "multiple config",
		config: map[string]application.ApplicationConfig{
			"key": {
				Type:  charm.OptionString,
				Value: "value",
			},
			"key2": {
				Type:  charm.OptionInt,
				Value: 42,
			},
			"key3": {
				Type:  charm.OptionFloat,
				Value: 3.14,
			},
			"key4": {
				Type:  charm.OptionBool,
				Value: true,
			},
			"key5": {
				Type:  charm.OptionSecret,
				Value: "secret",
			},
		},
		settings: application.ApplicationSettings{},
		expected: "5e78a3d09ac74eee2b554e2d57cddea7646f58de262e6d76b00005424427fe33",
	}, {
		name:   "trust",
		config: map[string]application.ApplicationConfig{},
		settings: application.ApplicationSettings{
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

		addTag0ConsStmt := `INSERT INTO constraint_tag (constraint_uuid, tag) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, addTag0ConsStmt, "constraint-uuid", "tag0")
		if err != nil {
			return err
		}
		addTag1ConsStmt := `INSERT INTO constraint_tag (constraint_uuid, tag) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, addTag1ConsStmt, "constraint-uuid", "tag1")
		if err != nil {
			return err
		}
		addSpace0Stmt := `INSERT INTO space (uuid, name) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, addSpace0Stmt, "space0-uuid", "space0")
		if err != nil {
			return err
		}
		addSpace1Stmt := `INSERT INTO space (uuid, name) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, addSpace1Stmt, "space1-uuid", "space1")
		if err != nil {
			return err
		}
		addSpace0ConsStmt := `INSERT INTO constraint_space (constraint_uuid, space, exclude) VALUES (?, ?, ?)`
		_, err = tx.ExecContext(ctx, addSpace0ConsStmt, "constraint-uuid", "space0", false)
		if err != nil {
			return err
		}
		addSpace1ConsStmt := `INSERT INTO constraint_space (constraint_uuid, space, exclude) VALUES (?, ?, ?)`
		_, err = tx.ExecContext(ctx, addSpace1ConsStmt, "constraint-uuid", "space1", false)
		if err != nil {
			return err
		}
		addZone0ConsStmt := `INSERT INTO constraint_zone (constraint_uuid, zone) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, addZone0ConsStmt, "constraint-uuid", "zone0")
		if err != nil {
			return err
		}
		addZone1ConsStmt := `INSERT INTO constraint_zone (constraint_uuid, zone) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, addZone1ConsStmt, "constraint-uuid", "zone1")
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
	c.Check(*cons.Spaces, jc.SameContents, []string{"space0", "space1"})
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
	c.Check(cons, jc.DeepEquals, constraints.Value{
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
	c.Check(cons, jc.DeepEquals, constraints.Value{
		CpuCores: ptr(uint64(2)),
	})
}

func (s *applicationStateSuite) TestConstraintEmpty(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	cons, err := s.state.GetApplicationConstraints(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cons, jc.DeepEquals, constraints.Value{})
}

func (s *applicationStateSuite) TestConstraintsApplicationNotFound(c *gc.C) {
	_, err := s.state.GetApplicationConstraints(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestSetConstraintFull(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	cons := constraints.Value{
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
		Spaces:           ptr([]string{"space0", "space1"}),
		Tags:             ptr([]string{"tag0", "tag1"}),
		Zones:            ptr([]string{"zone0", "zone1"}),
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

	var (
		applicationUUID                                                     string
		constraintUUID                                                      string
		constraintSpaces                                                    []string
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

		rows, err := tx.QueryContext(ctx, "SELECT space FROM constraint_space WHERE constraint_uuid=?", constraintUUID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var space string
			if err := rows.Scan(&space); err != nil {
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

	c.Check(constraintSpaces, jc.DeepEquals, []string{"space0", "space1"})
	c.Check(constraintTags, jc.DeepEquals, []string{"tag0", "tag1"})
	c.Check(constraintZones, jc.DeepEquals, []string{"zone0", "zone1"})

}

func (s *applicationStateSuite) TestSetConstraintInvalidContainerType(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	cons := constraints.Value{
		Container: ptr(instance.ContainerType("invalid-container-type")),
	}
	err := s.state.SetApplicationConstraints(context.Background(), id, cons)
	c.Assert(err, jc.ErrorIs, applicationerrors.InvalidApplicationConstraints)
}

func (s *applicationStateSuite) TestSetConstraintInvalidSpace(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	cons := constraints.Value{
		Spaces: ptr([]string{"invalid-space"}),
	}
	err := s.state.SetApplicationConstraints(context.Background(), id, cons)
	c.Assert(err, jc.ErrorIs, applicationerrors.InvalidApplicationConstraints)
}

func (s *applicationStateSuite) TestSetConstraintsReplacesPrevious(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.state.SetApplicationConstraints(context.Background(), id, constraints.Value{
		Mem:      ptr(uint64(8)),
		CpuCores: ptr(uint64(2)),
	})
	c.Assert(err, jc.ErrorIsNil)

	cons, err := s.state.GetApplicationConstraints(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cons, gc.DeepEquals, constraints.Value{
		Mem:      ptr(uint64(8)),
		CpuCores: ptr(uint64(2)),
	})

	err = s.state.SetApplicationConstraints(context.Background(), id, constraints.Value{
		CpuPower: ptr(uint64(42)),
	})
	c.Assert(err, jc.ErrorIsNil)

	cons, err = s.state.GetApplicationConstraints(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cons, gc.DeepEquals, constraints.Value{
		CpuPower: ptr(uint64(42)),
	})
}

func (s *applicationStateSuite) TestSetConstraintsReplacesPreviousZones(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.state.SetApplicationConstraints(context.Background(), id, constraints.Value{
		Zones: ptr([]string{"zone0", "zone1"}),
	})
	c.Assert(err, jc.ErrorIsNil)

	cons, err := s.state.GetApplicationConstraints(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*cons.Zones, jc.SameContents, []string{"zone0", "zone1"})

	err = s.state.SetApplicationConstraints(context.Background(), id, constraints.Value{
		Tags: ptr([]string{"tag0", "tag1"}),
	})
	c.Assert(err, jc.ErrorIsNil)

	cons, err = s.state.GetApplicationConstraints(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*cons.Tags, jc.SameContents, []string{"tag0", "tag1"})
}

func (s *applicationStateSuite) TestSetConstraintsReplacesPreviousSameZone(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.state.SetApplicationConstraints(context.Background(), id, constraints.Value{
		Zones: ptr([]string{"zone0", "zone1"}),
	})
	c.Assert(err, jc.ErrorIsNil)

	cons, err := s.state.GetApplicationConstraints(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*cons.Zones, jc.SameContents, []string{"zone0", "zone1"})

	err = s.state.SetApplicationConstraints(context.Background(), id, constraints.Value{
		Zones: ptr([]string{"zone3"}),
	})
	c.Assert(err, jc.ErrorIsNil)

	cons, err = s.state.GetApplicationConstraints(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*cons.Zones, jc.SameContents, []string{"zone3"})
}

func (s *applicationStateSuite) TestSetConstraintsApplicationNotFound(c *gc.C) {
	err := s.state.SetApplicationConstraints(context.Background(), "foo", constraints.Value{Mem: ptr(uint64(8))})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestSetApplicationStatus(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	now := time.Now().UTC()
	expected := &application.StatusInfo[application.WorkloadStatusType]{
		Status:  application.WorkloadStatusActive,
		Message: "message",
		Data:    []byte("data"),
		Since:   ptr(now),
	}

	err := s.state.SetApplicationStatus(context.Background(), id, expected)
	c.Assert(err, jc.ErrorIsNil)

	status, err := s.state.GetApplicationStatus(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status, jc.DeepEquals, expected)
}

func (s *applicationStateSuite) TestSetApplicationStatusMultipleTimes(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	err := s.state.SetApplicationStatus(context.Background(), id, &application.StatusInfo[application.WorkloadStatusType]{
		Status:  application.WorkloadStatusBlocked,
		Message: "blocked",
		Since:   ptr(time.Now().UTC()),
	})
	c.Assert(err, jc.ErrorIsNil)

	now := time.Now().UTC()
	expected := &application.StatusInfo[application.WorkloadStatusType]{
		Status:  application.WorkloadStatusActive,
		Message: "message",
		Data:    []byte("data"),
		Since:   ptr(now),
	}

	err = s.state.SetApplicationStatus(context.Background(), id, expected)
	c.Assert(err, jc.ErrorIsNil)

	status, err := s.state.GetApplicationStatus(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status, jc.DeepEquals, expected)
}

func (s *applicationStateSuite) TestSetApplicationStatusWithNoData(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	now := time.Now().UTC()
	expected := &application.StatusInfo[application.WorkloadStatusType]{
		Status:  application.WorkloadStatusActive,
		Message: "message",
		Since:   ptr(now),
	}

	err := s.state.SetApplicationStatus(context.Background(), id, expected)
	c.Assert(err, jc.ErrorIsNil)

	status, err := s.state.GetApplicationStatus(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status, jc.DeepEquals, expected)
}

func (s *applicationStateSuite) TestSetApplicationStatusApplicationNotFound(c *gc.C) {
	now := time.Now().UTC()
	expected := &application.StatusInfo[application.WorkloadStatusType]{
		Status:  application.WorkloadStatusActive,
		Message: "message",
		Data:    []byte("data"),
		Since:   ptr(now),
	}

	err := s.state.SetApplicationStatus(context.Background(), "foo", expected)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestSetApplicationStatusInvalidStatus(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	expected := application.StatusInfo[application.WorkloadStatusType]{
		Status: application.WorkloadStatusType(99),
	}

	err := s.state.SetApplicationStatus(context.Background(), id, &expected)
	c.Assert(err, gc.ErrorMatches, `unknown status.*`)
}

func (s *applicationStateSuite) TestGetApplicationStatusApplicationNotFound(c *gc.C) {
	_, err := s.state.GetApplicationStatus(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetApplicationStatusNotSet(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	status, err := s.state.GetApplicationStatus(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status, gc.DeepEquals, &application.StatusInfo[application.WorkloadStatusType]{
		Status: application.WorkloadStatusUnset,
	})
}

func (s *applicationStateSuite) assertApplication(
	c *gc.C,
	name string,
	platform application.Platform,
	channel *application.Channel,
	scale application.ScaleState,
	available bool,
) {
	var (
		gotName      string
		gotUUID      string
		gotCharmUUID string
		gotPlatform  application.Platform
		gotChannel   application.Channel
		gotScale     application.ScaleState
		gotAvailable bool
	)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotName, gc.Equals, name)
	c.Check(gotPlatform, jc.DeepEquals, platform)
	c.Check(gotScale, jc.DeepEquals, scale)
	c.Check(gotAvailable, gc.Equals, available)

	// Channel is optional, so we need to check it separately.
	if channel != nil {
		c.Check(gotChannel, gc.DeepEquals, *channel)
	} else {
		// Ensure it's empty if the original origin channel isn't set.
		// Prevent the db from sending back bogus values.
		c.Check(gotChannel, gc.DeepEquals, application.Channel{})
	}
}

func (s *applicationStateSuite) addCharmModifiedVersion(c *gc.C, appID coreapplication.ID, charmModifiedVersion int) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application SET charm_modified_version = ? WHERE uuid = ?", charmModifiedVersion, appID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationStateSuite) addUnit(c *gc.C, appID coreapplication.ID, u application.InsertUnitArg) coreunit.UUID {
	err := s.state.InsertUnit(context.Background(), appID, u)
	c.Assert(err, jc.ErrorIsNil)

	var unitUUID string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", u.UnitName).Scan(&unitUUID)
	})
	c.Assert(err, jc.ErrorIsNil)
	return coreunit.UUID(unitUUID)
}
