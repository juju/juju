// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	stdtesting "testing"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/devices"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package state -destination charm_mock_test.go github.com/juju/juju/internal/charm Charm


type baseSuite struct {
	schematesting.ModelSuite
}

func (s *baseSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := uuid.MustNewUUID()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, type, cloud, cloud_type)
			VALUES (?, ?, "test", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) minimalMetadata(c *tc.C, name string) charm.Metadata {
	return charm.Metadata{
		Name: name,
	}
}

func (s *baseSuite) minimalMetadataWithPeerRelation(c *tc.C, name string, relationNames ...string) charm.Metadata {
	peers := make(map[string]charm.Relation)
	for _, relation := range relationNames {
		peers[relation] = charm.Relation{
			Name:  relation,
			Role:  charm.RolePeer,
			Scope: charm.ScopeGlobal,
		}
	}
	return charm.Metadata{
		Name:  name,
		Peers: peers,
	}
}

func (s *baseSuite) minimalManifest(c *tc.C) charm.Manifest {
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

func (s *baseSuite) addApplicationArgForResources(c *tc.C,
	name string,
	charmResources map[string]charm.Resource,
	addResourcesArgs []application.AddApplicationResourceArg,
) application.AddApplicationArg {
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

	metadata := s.minimalMetadata(c, name)
	metadata.Resources = charmResources
	return application.AddApplicationArg{
		Platform: platform,
		Charm: charm.Charm{
			Metadata:      metadata,
			Manifest:      s.minimalManifest(c),
			Source:        charm.CharmHubSource,
			ReferenceName: name,
			Revision:      42,
			Architecture:  architecture.ARM64,
		},
		CharmDownloadInfo: &charm.DownloadInfo{
			Provenance:         charm.ProvenanceDownload,
			CharmhubIdentifier: "ident-1",
			DownloadURL:        "http://example.com/charm",
			DownloadSize:       666,
		},
		Scale:     1,
		Channel:   channel,
		Resources: addResourcesArgs,
	}
}

func (s *baseSuite) createObjectStoreBlob(c *tc.C, path string) objectstore.UUID {
	uuid := objectstoretesting.GenObjectStoreUUID(c)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO object_store_metadata (uuid, sha_256, sha_384, size) VALUES (?, 'foo', 'bar', 42)
`, uuid.String())
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO object_store_metadata_path (path, metadata_uuid) VALUES (?, ?)
`, path, uuid.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}

func (s *baseSuite) addApplicationArgForStorage(c *tc.C,
	name string,
	charmStorage []charm.Storage,
	addStorageArgs []application.ApplicationStorageArg) application.AddApplicationArg {
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

	metadata := s.minimalMetadata(c, name)
	metadata.Storage = make(map[string]charm.Storage)
	for _, stor := range charmStorage {
		metadata.Storage[stor.Name] = stor
	}
	args := application.AddApplicationArg{
		Platform: platform,
		Charm: charm.Charm{
			Metadata:      metadata,
			Manifest:      s.minimalManifest(c),
			Source:        charm.CharmHubSource,
			ReferenceName: name,
			Revision:      42,
			Architecture:  architecture.ARM64,
		},
		CharmDownloadInfo: &charm.DownloadInfo{
			Provenance:         charm.ProvenanceDownload,
			CharmhubIdentifier: "ident-1",
			DownloadURL:        "http://example.com/charm",
			DownloadSize:       666,
		},
		Scale:           1,
		Channel:         channel,
		Storage:         addStorageArgs,
		StoragePoolKind: make(map[string]storage.StorageKind),
	}
	registry := storage.ChainedProviderRegistry{
		dummystorage.StorageProviders(),
		provider.CommonStorageProviders(),
	}
	types, err := registry.StorageProviderTypes()
	c.Assert(err, tc.ErrorIsNil)
	for _, t := range types {
		p, err := registry.StorageProvider(t)
		c.Assert(err, tc.ErrorIsNil)
		if p.Supports(storage.StorageKindFilesystem) {
			args.StoragePoolKind[string(t)] = storage.StorageKindFilesystem
		} else {
			args.StoragePoolKind[string(t)] = storage.StorageKindBlock
		}
	}
	return args
}

func (s *baseSuite) createApplication(c *tc.C, name string, l life.Life, units ...application.InsertUnitArg) coreapplication.ID {
	state := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

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

	appID, err := state.CreateApplication(ctx, name, application.AddApplicationArg{
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
				ExtraBindings: map[string]charm.ExtraBinding{
					"extra": {
						Name: "extra",
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
		Devices: map[string]devices.Constraints{
			"dev0": {
				Type:       devices.DeviceType("type0"),
				Count:      42,
				Attributes: map[string]string{"k0": "v0", "k1": "v1"},
			},
			"dev1": {
				Type:       devices.DeviceType("type1"),
				Count:      3,
				Attributes: map[string]string{"k2": "v2"},
			},
			"dev2": {
				Type:  devices.DeviceType("type2"),
				Count: 1974,
			},
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	modelType, err := state.GetModelType(ctx)
	c.Assert(err, tc.ErrorIsNil)

	db, err := state.DB()
	c.Assert(err, tc.ErrorIsNil)
	for _, u := range units {
		err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			if modelType == coremodel.IAAS {
				return state.insertIAASUnit(ctx, tx, appID, u)
			}
			return state.insertCAASUnit(ctx, tx, appID, u)
		})
		c.Assert(err, tc.ErrorIsNil)
	}
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application SET life_id = ? WHERE name = ?", l, name)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, "UPDATE unit SET life_id = ? WHERE application_uuid = ?", l, appID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return appID
}

func (s *baseSuite) createScalingApplication(c *tc.C, name string, l life.Life, scale int) coreapplication.ID {
	state := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

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

	appID, err := state.CreateApplication(ctx, name, application.AddApplicationArg{
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
		Scale: scale,
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application SET life_id = ? WHERE name = ?", l, name)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return appID
}

func (s *baseSuite) assertApplication(
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
