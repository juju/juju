// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/devices"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package state -destination charm_mock_test.go github.com/juju/juju/internal/charm Charm

type baseSuite struct {
	schematesting.ModelSuite
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
) application.AddIAASApplicationArg {
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
	return application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
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
			Channel:   channel,
			Resources: addResourcesArgs,
		},
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

func (s *baseSuite) addIAASApplicationArgForStorage(c *tc.C,
	name string,
	charmStorage []charm.Storage,
	directives []application.CreateApplicationStorageDirectiveArg,
) application.AddIAASApplicationArg {
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
	args := application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
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
			Channel:           channel,
			StorageDirectives: directives,
		},
	}
	return args
}

func (s *baseSuite) addCAASApplicationArgForStorage(c *tc.C,
	name string,
	charmStorage []charm.Storage,
	directives []application.CreateApplicationStorageDirectiveArg,
) application.AddCAASApplicationArg {
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
	args := application.AddCAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
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
			Channel:           channel,
			StorageDirectives: directives,
		},
		Scale: 1,
	}
	return args
}

func (s *baseSuite) createNamedIAASUnit(c *tc.C) (coreunit.Name, coreunit.UUID) {
	n, u := s.createNNamedIAASUnit(c, 1)
	return n[0], u[0]
}

func (s *baseSuite) createNNamedIAASUnit(c *tc.C, n int) ([]coreunit.Name, []coreunit.UUID) {
	state := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	_, unitUUIDS := s.createIAASApplicationWithNUnits(c, "foo", life.Alive, n)
	names := make([]coreunit.Name, 0, n)
	for _, unitUUID := range unitUUIDS {
		n, err := state.GetUnitNameForUUID(c.Context(), unitUUID)
		c.Assert(err, tc.ErrorIsNil)
		names = append(names, n)
	}
	return names, unitUUIDS
}

func (s *baseSuite) createNamedCAASUnit(c *tc.C) (coreunit.Name, coreunit.UUID) {
	state := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	_, unitUUIDS := s.createCAASApplicationWithNUnits(c, "foo", life.Alive, 1)
	name, err := state.GetUnitNameForUUID(c.Context(), unitUUIDS[0])
	c.Assert(err, tc.ErrorIsNil)
	return name, unitUUIDS[0]
}

func (s *baseSuite) createIAASApplication(c *tc.C, name string, l life.Life, units ...application.AddIAASUnitArg) coreapplication.ID {
	return s.createIAASApplicationWithEndpointBindings(c, name, l, nil, units...)
}

func (s *baseSuite) createIAASApplicationWithNUnits(
	c *tc.C,
	name string,
	l life.Life,
	unitCount int,
) (coreapplication.ID, []coreunit.UUID) {
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
	units := make([]application.AddIAASUnitArg, unitCount)

	appUUID, _, err := state.CreateIAASApplication(ctx, name, application.AddIAASApplicationArg{
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
		},
	}, units)
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application SET life_id = ? WHERE name = ?", l, name)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, "UPDATE unit SET life_id = ? WHERE application_uuid = ?", l, appUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	unitUUIDS, err := state.getApplicationUnits(ctx, appUUID)
	c.Assert(err, tc.ErrorIsNil)

	return appUUID, unitUUIDS
}

func (s *baseSuite) createIAASApplicationWithEndpointBindings(
	c *tc.C,
	name string,
	l life.Life,
	bindings map[string]network.SpaceName,
	units ...application.AddIAASUnitArg,
) coreapplication.ID {
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

	appID, _, err := state.CreateIAASApplication(ctx, name, application.AddIAASApplicationArg{
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
			EndpointBindings: bindings,
		},
	}, units)
	c.Assert(err, tc.ErrorIsNil)

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

func (s *baseSuite) createSubnetForCAASModel(c *tc.C) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// Only insert the subnet it if doesn't exist.
		var rowCount int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM subnet`).Scan(&rowCount); err != nil {
			return err
		}
		if rowCount != 0 {
			return nil
		}

		subnetUUID := uuid.MustNewUUID().String()
		_, err := tx.ExecContext(ctx, "INSERT INTO subnet (uuid, cidr) VALUES (?, ?)", subnetUUID, "0.0.0.0/0")
		if err != nil {
			return err
		}
		subnetUUID2 := uuid.MustNewUUID().String()
		_, err = tx.ExecContext(ctx, "INSERT INTO subnet (uuid, cidr) VALUES (?, ?)", subnetUUID2, "::/0")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) createCAASApplicationWithNUnits(
	c *tc.C, name string, l life.Life, unitCount int,
) (coreapplication.ID, []coreunit.UUID) {
	appUUID := s.createCAASApplication(
		c, name, l, make([]application.AddCAASUnitArg, unitCount)...,
	)
	state := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	uuids, err := state.getApplicationUnits(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	return appUUID, uuids
}

func (s *baseSuite) createCAASApplication(c *tc.C, name string, l life.Life, units ...application.AddCAASUnitArg) coreapplication.ID {
	s.createSubnetForCAASModel(c)
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

	appID, err := state.CreateCAASApplication(ctx, name, application.AddCAASApplicationArg{
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
		},
		Scale: len(units),
	}, units)
	c.Assert(err, tc.ErrorIsNil)

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

func (s *baseSuite) createCAASScalingApplication(c *tc.C, name string, l life.Life, scale int) coreapplication.ID {
	s.createSubnetForCAASModel(c)
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

	appID, err := state.CreateCAASApplication(ctx, name, application.AddCAASApplicationArg{
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

func (s *baseSuite) addUnit(c *tc.C, unitName coreunit.Name, appUUID coreapplication.ID) coreunit.UUID {
	return s.addUnitWithLife(c, unitName, appUUID, life.Alive)
}

func (s *baseSuite) addUnitWithLife(c *tc.C, unitName coreunit.Name, appUUID coreapplication.ID, l life.Life) coreunit.UUID {
	unitUUID := coreunittesting.GenUnitUUID(c)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		netNodeUUID := uuid.MustNewUUID().String()
		_, err := tx.Exec(`
INSERT INTO net_node (uuid)
VALUES (?)
`, netNodeUUID)
		if err != nil {
			return err
		}

		_, err = tx.Exec(`
INSERT INTO unit (uuid, name, life_id, net_node_uuid, application_uuid, charm_uuid)
SELECT ?, ?, ?, ?, uuid, charm_uuid
FROM application
WHERE uuid = ?
`, unitUUID, unitName, l, netNodeUUID, appUUID)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return unitUUID
}

func (s *baseSuite) addMachineToUnit(c *tc.C, unitUUID coreunit.UUID) (coremachine.Name, coremachine.UUID) {
	machineUUID := coremachine.GenUUID(c)
	machineName := coremachine.Name("0")
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO machine (uuid, name, life_id, net_node_uuid)
SELECT ?, ?, ?, net_node_uuid
FROM unit
WHERE uuid = ?
`, machineUUID, machineName, 0 /* alive */, unitUUID)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return machineName, machineUUID
}

func (s *baseSuite) assertUnitPrincipal(c *tc.C, principalName, subordinateName coreunit.Name) {
	var foundPrincipalName coreunit.Name
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT u1.name
FROM unit u1
JOIN unit_principal up ON up.principal_uuid = u1.uuid
JOIN unit u2 ON u2.uuid = up.unit_uuid
WHERE u2.name = ?
`, subordinateName).Scan(&foundPrincipalName)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(foundPrincipalName, tc.Equals, principalName)
}

func (s *baseSuite) setUnitLife(c *tc.C, uuid coreunit.UUID, life life.Life) {
	_, err := s.DB().ExecContext(
		c.Context(),
		"UPDATE unit SET life_id = ? WHERE uuid = ?",
		int(life),
		uuid.String(),
	)
	c.Assert(err, tc.ErrorIsNil)
}
