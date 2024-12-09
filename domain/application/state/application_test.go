// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	"github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/linklayerdevice"
	portstate "github.com/juju/juju/domain/port/state"
	domainsecret "github.com/juju/juju/domain/secret"
	secretstate "github.com/juju/juju/domain/secret/state"
	domainstorage "github.com/juju/juju/domain/storage"
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

	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		_, err := s.state.CreateApplication(ctx, "666", application.AddApplicationArg{
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
		})
		return err
	})
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
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
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
		})
		if err != nil {
			return err
		}

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
		})
		return err
	})
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
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
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
		})
		return err
	})
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
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		_, err := s.state.CreateApplication(ctx, "666", application.AddApplicationArg{
			Platform: platform,
			Charm: charm.Charm{
				Metadata: s.minimalMetadata(c, "666"),
				Manifest: s.minimalManifest(c),
				Source:   charm.LocalSource,
				Revision: 42,
			},
			Scale: 1,
		})
		return err
	})
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
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
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
		})
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, channel, scale, true)
}

func (s *applicationStateSuite) TestGetApplicationLife(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Dying)
	var (
		appLife life.Life
		gotID   coreapplication.ID
	)
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		var err error
		gotID, appLife, err = s.state.GetApplicationLife(ctx, "foo")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotID, gc.Equals, appID)
	c.Assert(appLife, gc.Equals, life.Dying)
}

func (s *applicationStateSuite) TestGetApplicationLifeNotFound(c *gc.C) {
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		_, _, err := s.state.GetApplicationLife(ctx, "foo")
		return err
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestUpsertCloudServiceNew(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)
	err := s.state.UpsertCloudService(context.Background(), "foo", "provider-id", network.SpaceAddresses{})
	c.Assert(err, jc.ErrorIsNil)
	var providerID string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT provider_id FROM cloud_service WHERE application_uuid = ?", appID).Scan(&providerID)
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
		err := tx.QueryRowContext(ctx, "SELECT provider_id FROM cloud_service WHERE application_uuid = ?", appID).Scan(&providerID)
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
		rows, err := tx.QueryContext(ctx, "SELECT provider_id FROM cloud_service WHERE application_uuid = ?", appID)
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
			ProviderId: "some-id",
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
	appID := s.createApplication(c, "foo", life.Alive)
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.InsertUnit(ctx, appID, u)
	})
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
		gotProviderId string
		gotValue      string
		gotType       int
		gotOrigin     int
		gotScope      int
		gotConfigType int
	)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `

SELECT cc.provider_id, a.address_value, a.type_id, a.origin_id,a.scope_id,a.config_type_id
FROM cloud_container cc
JOIN unit u ON cc.unit_uuid = u.uuid
JOIN link_layer_device lld ON lld.net_node_uuid = u.net_node_uuid
JOIN ip_address a ON a.device_uuid = lld.uuid
WHERE u.name=?`,

			unitName).Scan(
			&gotProviderId,
			&gotValue,
			&gotType,
			&gotOrigin,
			&gotScope,
			&gotConfigType,
		)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotProviderId, gc.Equals, providerID)
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
FROM cloud_container cc
JOIN unit u ON cc.unit_uuid = u.uuid
JOIN cloud_container_port ccp ON ccp.unit_uuid = cc.unit_uuid
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

func (s *applicationStateSuite) TestUpdateUnitContainer(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
		CloudContainer: &application.CloudContainer{
			ProviderId: "some-id",
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

	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.UpdateUnitContainer(ctx, "foo/667", &application.CloudContainer{})
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)

	cc := &application.CloudContainer{
		ProviderId: "another-id",
		Ports:      ptr([]string{"666", "667"}),
		Address: ptr(application.ContainerAddress{
			Device: application.ContainerDevice{
				Name:              "placeholder",
				DeviceTypeID:      linklayerdevice.DeviceTypeUnknown,
				VirtualPortTypeID: linklayerdevice.NonVirtualPortType,
			},
			Value:       "2001:db8::1",
			AddressType: ipaddress.AddressTypeIPv6,
			ConfigType:  ipaddress.ConfigTypeDHCP,
			Scope:       ipaddress.ScopeCloudLocal,
			Origin:      ipaddress.OriginProvider,
		}),
	}
	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.UpdateUnitContainer(ctx, "foo/666", cc)
	})
	c.Assert(err, jc.ErrorIsNil)

	var (
		providerId string
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err = tx.QueryRowContext(ctx, `

SELECT provider_id FROM cloud_container cc
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
		ipaddress.AddressTypeIPv6, ipaddress.OriginProvider, ipaddress.ScopeCloudLocal, ipaddress.ConfigTypeDHCP)
	s.assertContainerPortValues(c, "foo/666", []string{"666", "667"})
}

func (s *applicationStateSuite) TestInsertUnit(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)

	u := application.InsertUnitArg{
		UnitName: "foo/666",
		CloudContainer: &application.CloudContainer{
			ProviderId: "some-id",
		},
	}
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.InsertUnit(ctx, appID, u)
	})
	c.Assert(err, jc.ErrorIsNil)

	var (
		providerId string
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err = tx.QueryRowContext(ctx, `

SELECT provider_id FROM cloud_container cc
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

	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.InsertUnit(ctx, appID, u)
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitAlreadyExists)
}

func (s *applicationStateSuite) TestSetUnitPassword(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	appID := s.createApplication(c, "foo", life.Alive)
	unitUUID := s.addUnit(c, appID, u)

	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetUnitPassword(ctx, unitUUID, application.PasswordInfo{
			PasswordHash: "secret",
		})
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
	c.Assert(password, gc.Equals, "secret")
	c.Assert(algorithmID, gc.Equals, 0)
}

func (s *applicationStateSuite) TestGetUnitLife(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u)

	var unitLife life.Life
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		var err error
		unitLife, err = s.state.GetUnitLife(ctx, "foo/666")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitLife, gc.Equals, life.Alive)
}

func (s *applicationStateSuite) TestGetUnitLifeNotFound(c *gc.C) {
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		_, err := s.state.GetUnitLife(ctx, "foo/666")
		return err
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationStateSuite) TestSetUnitLife(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
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

	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetUnitLife(ctx, "foo/666", life.Dying)
	})
	c.Assert(err, jc.ErrorIsNil)
	checkResult(life.Dying)

	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetUnitLife(ctx, "foo/666", life.Dead)
	})
	c.Assert(err, jc.ErrorIsNil)
	checkResult(life.Dead)

	// Can't go backwards.
	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetUnitLife(ctx, "foo/666", life.Dying)
	})
	c.Assert(err, jc.ErrorIsNil)
	checkResult(life.Dead)
}

func (s *applicationStateSuite) TestSetUnitLifeNotFound(c *gc.C) {
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetUnitLife(ctx, "foo/666", life.Dying)
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationStateSuite) TestDeleteUnit(c *gc.C) {
	// TODO(units) - add references to agents etc when those are fully cooked
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
		CloudContainer: &application.CloudContainer{
			ProviderId: "provider-id",
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
			AgentStatus: application.UnitAgentStatusInfo{
				StatusID: application.UnitAgentStatusExecuting,
				StatusInfo: application.StatusInfo{
					Message: "test",
					Data:    map[string]string{"foo": "bar"},
					Since:   time.Now(),
				},
			},
			WorkloadStatus: application.UnitWorkloadStatusInfo{
				StatusID: application.UnitWorkloadStatusActive,
				StatusInfo: application.StatusInfo{
					Message: "test",
					Data:    map[string]string{"foo": "bar"},
					Since:   time.Now(),
				},
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

	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		if err := s.state.SetCloudContainerStatus(ctx, unitUUID, application.CloudContainerStatusStatusInfo{
			StatusID: application.CloudContainerStatusBlocked,
			StatusInfo: application.StatusInfo{
				Message: "test",
				Data:    map[string]string{"foo": "bar"},
				Since:   time.Now(),
			},
		}); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	portSt := portstate.NewState(s.TxnRunnerFactory())
	err = portSt.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return portSt.UpdateUnitPorts(ctx, unitUUID, network.GroupedPortRanges{
			"endpoint": {
				{Protocol: "tcp", FromPort: 80, ToPort: 80},
				{Protocol: "udp", FromPort: 1000, ToPort: 1500},
			},
			"misc": {
				{Protocol: "tcp", FromPort: 8080, ToPort: 8080},
			},
		}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	var gotIsLast bool
	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		var err error
		gotIsLast, err = s.state.DeleteUnit(ctx, "foo/666")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotIsLast, jc.IsFalse)

	var (
		unitCount                     int
		containerCount                int
		deviceCount                   int
		addressCount                  int
		portCount                     int
		agentStatusCount              int
		agentStatusDataCount          int
		workloadStatusCount           int
		workloadStatusDataCount       int
		cloudContainerStatusCount     int
		cloudContainerStatusDataCount int
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit WHERE name=?", u1.UnitName).Scan(&unitCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM cloud_container WHERE unit_uuid=?", unitUUID).Scan(&containerCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM link_layer_device WHERE net_node_uuid=?", netNodeUUID).Scan(&deviceCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM ip_address WHERE device_uuid=?", deviceUUID).Scan(&addressCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM cloud_container_port WHERE unit_uuid=?", unitUUID).Scan(&portCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit_agent_status WHERE unit_uuid=?", unitUUID).Scan(&agentStatusCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit_agent_status_data WHERE unit_uuid=?", unitUUID).Scan(&agentStatusDataCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit_workload_status WHERE unit_uuid=?", unitUUID).Scan(&workloadStatusCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit_workload_status_data WHERE unit_uuid=?", unitUUID).Scan(&workloadStatusDataCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM cloud_container_status WHERE unit_uuid=?", unitUUID).Scan(&cloudContainerStatusCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM cloud_container_status_data WHERE unit_uuid=?", unitUUID).Scan(&cloudContainerStatusDataCount); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addressCount, gc.Equals, 0)
	c.Assert(portCount, gc.Equals, 0)
	c.Assert(deviceCount, gc.Equals, 0)
	c.Assert(containerCount, gc.Equals, 0)
	c.Assert(agentStatusCount, gc.Equals, 0)
	c.Assert(agentStatusDataCount, gc.Equals, 0)
	c.Assert(workloadStatusCount, gc.Equals, 0)
	c.Assert(workloadStatusDataCount, gc.Equals, 0)
	c.Assert(cloudContainerStatusCount, gc.Equals, 0)
	c.Assert(cloudContainerStatusDataCount, gc.Equals, 0)
	c.Assert(unitCount, gc.Equals, 0)
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

	var gotIsLast bool
	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		var err error
		gotIsLast, err = s.state.DeleteUnit(ctx, "foo/666")
		return err
	})
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

func (s *applicationStateSuite) createOwnedSecrets(c *gc.C) (appSecretURI *secrets.URI, unitSecretURI *secrets.URI) {
	mysqlID := s.createApplication(c, "mysql", life.Alive,
		application.InsertUnitArg{UnitName: "mysql/0"},
		application.InsertUnitArg{UnitName: "mysql/1"},
	)
	s.createApplication(c, "mariadb", life.Alive,
		application.InsertUnitArg{UnitName: "mariadb/0"},
		application.InsertUnitArg{UnitName: "mariadb/1"},
	)
	var mysqlUnitUUID coreunit.UUID
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "mysql/1").
			Scan(&mysqlUnitUUID); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	st := secretstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	ctx := context.Background()
	uri1 := secrets.NewURI()
	uri2 := secrets.NewURI()

	sp := domainsecret.UpsertSecretParams{
		Data:       secrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	err = s.state.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		err := st.CreateCharmApplicationSecret(ctx, 1, uri1, mysqlID, sp)
		if err != nil {
			return err
		}

		sp2 := domainsecret.UpsertSecretParams{
			Data:       secrets.SecretData{"foo": "bar"},
			RevisionID: ptr(uuid.MustNewUUID().String()),
		}
		return st.CreateCharmUnitSecret(ctx, 1, uri2, mysqlUnitUUID, sp2)
	})
	c.Assert(err, jc.ErrorIsNil)
	return uri1, uri2
}

func (s *applicationStateSuite) TestGetSecretsForApplication(c *gc.C) {
	uri1, _ := s.createOwnedSecrets(c)
	var gotURIs []*secrets.URI
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		var err error
		gotURIs, err = s.state.GetSecretsForApplication(ctx, "mysql")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotURIs, jc.SameContents, []*secrets.URI{uri1})
}

func (s *applicationStateSuite) TestGetSecretsForUnit(c *gc.C) {
	_, uri2 := s.createOwnedSecrets(c)
	var gotURIs []*secrets.URI
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		var err error
		gotURIs, err = s.state.GetSecretsForUnit(ctx, "mysql/1")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotURIs, jc.SameContents, []*secrets.URI{uri2})
}

func (s *applicationStateSuite) TestGetSecretsNone(c *gc.C) {
	s.createOwnedSecrets(c)
	var gotURIs []*secrets.URI
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		var err error
		gotURIs, err = s.state.GetSecretsForUnit(ctx, "mariadb/1")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotURIs, gc.HasLen, 0)
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

	var gotIsLast bool
	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		var err error
		gotIsLast, err = s.state.DeleteUnit(ctx, "foo/666")
		return err
	})
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

func (s *applicationStateSuite) TestGetUnitUUIDs(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	u2 := application.InsertUnitArg{
		UnitName: "foo/667",
	}
	s.createApplication(c, "foo", life.Alive, u1, u2)

	unitUUIDs, err := s.state.GetUnitUUIDs(context.Background(), []coreunit.Name{u1.UnitName, u2.UnitName})
	c.Assert(err, jc.ErrorIsNil)

	var gotUUIDs []coreunit.UUID
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT uuid FROM unit WHERE name IN (?, ?)", u1.UnitName, u2.UnitName)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var uuid coreunit.UUID
			if err := rows.Scan(&uuid); err != nil {
				return err
			}
			gotUUIDs = append(gotUUIDs, uuid)
		}
		return rows.Err()
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotUUIDs, jc.SameContents, unitUUIDs)
}

func (s *applicationStateSuite) TestGetUnitUUIDsNotFound(c *gc.C) {
	_, err := s.state.GetUnitUUIDs(context.Background(), []coreunit.Name{coreunit.Name("foo/666")})
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationStateSuite) TestGetUnitUUIDsOneNotFound(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)

	_, err := s.state.GetUnitUUIDs(context.Background(), []coreunit.Name{u1.UnitName, coreunit.Name("foo/667")})
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationStateSuite) TestGetUnitNames(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	u2 := application.InsertUnitArg{
		UnitName: "foo/667",
	}
	s.createApplication(c, "foo", life.Alive, u1, u2)

	var unitUUIDs []coreunit.UUID
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT uuid FROM unit WHERE name IN (?, ?)", u1.UnitName, u2.UnitName)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var uuid coreunit.UUID
			if err := rows.Scan(&uuid); err != nil {
				return err
			}
			unitUUIDs = append(unitUUIDs, uuid)
		}
		return rows.Err()
	})
	c.Assert(err, jc.ErrorIsNil)

	unitNames, err := s.state.GetUnitNames(context.Background(), unitUUIDs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitNames, gc.HasLen, 2)
	c.Assert(unitNames, jc.SameContents, []coreunit.Name{u1.UnitName, u2.UnitName})
}

func (s *applicationStateSuite) TestGetUnitNamesNotFound(c *gc.C) {
	uuid := unittesting.GenUnitUUID(c)
	_, err := s.state.GetUnitNames(context.Background(), []coreunit.UUID{uuid})
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationStateSuite) TestGetUnitNamesOneNotFound(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)

	var existingUUID coreunit.UUID
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", u1.UnitName).
			Scan(&existingUUID); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	missingUUID := unittesting.GenUnitUUID(c)

	_, err = s.state.GetUnitNames(context.Background(), []coreunit.UUID{existingUUID, missingUUID})
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationStateSuite) TestGetApplicationIDByUnitName(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	expectedAppUUID := s.createApplication(c, "foo", life.Alive, u1)

	obtainedAppUUID, err := s.state.GetApplicationIDByUnitName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedAppUUID, gc.Equals, expectedAppUUID)
}

func (s *applicationStateSuite) TestGetApplicationIDByUnitNameUnitNotFound(c *gc.C) {
	_, err := s.state.GetApplicationIDByUnitName(context.Background(), "failme")
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationStateSuite) TestGetCharmModifiedVersion(c *gc.C) {
	appUUID := s.createApplication(c, "foo", life.Alive)
	s.addCharmModifiedVersion(c, appUUID, 7)

	charmModifiedVersion, err := s.state.GetCharmModifiedVersion(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charmModifiedVersion, gc.Equals, 7)
}

func (s *applicationStateSuite) TestGetCharmModifiedVersionApplicationNotFound(c *gc.C) {
	_, err := s.state.GetCharmModifiedVersion(context.Background(), applicationtesting.GenApplicationUUID(c))
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) assertUnitStatus(

	c *gc.C, statusType, unitUUID coreunit.UUID, statusID int, message string, since time.Time, data map[string]string,

) {
	var (
		gotStatusID int
		gotMessage  string
		gotSince    time.Time
		gotData     = make(map[string]string)
	)
	queryInfo := fmt.Sprintf(`

SELECT status_id, message, updated_at FROM %s_status WHERE unit_uuid = ?

	`, statusType)
	queryData := fmt.Sprintf(`

SELECT key, data FROM %s_status_data WHERE unit_uuid = ?

		`, statusType)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, queryInfo, unitUUID).
			Scan(&gotStatusID, &gotMessage, &gotSince); err != nil {
			return err
		}
		rows, err := tx.QueryContext(context.Background(), queryData, unitUUID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key, value string
			if err := rows.Scan(&key, &value); err != nil {
				return err
			}
			gotData[key] = value
		}
		return rows.Err()
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotStatusID, gc.Equals, statusID)
	c.Assert(gotMessage, gc.Equals, message)
	c.Assert(gotSince, jc.DeepEquals, since)
	c.Assert(gotData, jc.DeepEquals, data)
}

func (s *applicationStateSuite) TestSetCloudContainerStatus(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)

	status := application.CloudContainerStatusStatusInfo{
		StatusID: application.CloudContainerStatusRunning,
		StatusInfo: application.StatusInfo{
			Message: "it's running",
			Data:    map[string]string{"foo": "bar"},
			Since:   time.Now(),
		},
	}

	unitUUIDs, err := s.state.GetUnitUUIDs(context.Background(), []coreunit.Name{u1.UnitName})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitUUIDs, gc.HasLen, 1)
	unitUUID := unitUUIDs[0]

	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetCloudContainerStatus(ctx, unitUUID, status)
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitStatus(
		c, "cloud_container", unitUUID, int(status.StatusID), status.Message, status.Since, status.Data)
}

func (s *applicationStateSuite) TestSetUnitAgentStatus(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)

	status := application.UnitAgentStatusInfo{
		StatusID: application.UnitAgentStatusExecuting,
		StatusInfo: application.StatusInfo{
			Message: "it's executing",
			Data:    map[string]string{"foo": "bar"},
			Since:   time.Now(),
		},
	}

	unitUUIDs, err := s.state.GetUnitUUIDs(context.Background(), []coreunit.Name{u1.UnitName})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitUUIDs, gc.HasLen, 1)
	unitUUID := unitUUIDs[0]

	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetUnitAgentStatus(ctx, unitUUID, status)
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitStatus(
		c, "unit_agent", unitUUID, int(status.StatusID), status.Message, status.Since, status.Data)
}

func (s *applicationStateSuite) TestSetUnitWorkloadStatus(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)

	status := application.UnitWorkloadStatusInfo{
		StatusID: application.UnitWorkloadStatusTerminated,
		StatusInfo: application.StatusInfo{
			Message: "it's terminated",
			Data:    map[string]string{"foo": "bar"},
			Since:   time.Now(),
		},
	}

	unitUUIDs, err := s.state.GetUnitUUIDs(context.Background(), []coreunit.Name{u1.UnitName})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitUUIDs, gc.HasLen, 1)
	unitUUID := unitUUIDs[0]

	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetUnitWorkloadStatus(ctx, unitUUID, status)
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitStatus(
		c, "unit_workload", unitUUID, int(status.StatusID), status.Message, status.Since, status.Data)
}

func (s *applicationStateSuite) TestGetApplicationScaleState(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	appID := s.createApplication(c, "foo", life.Alive, u)

	var scaleState application.ScaleState
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		var err error
		scaleState, err = s.state.GetApplicationScaleState(ctx, appID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(scaleState, jc.DeepEquals, application.ScaleState{
		Scale: 1,
	})
}

func (s *applicationStateSuite) TestGetApplicationScaleStateNotFound(c *gc.C) {
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		_, err := s.state.GetApplicationScaleState(ctx, coreapplication.ID(uuid.MustNewUUID().String()))
		return err
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestSetDesiredApplicationScale(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)

	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetDesiredApplicationScale(ctx, appID, 666)
	})
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
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetDesiredApplicationScale(ctx, appID, 666)
	})
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

	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetApplicationScalingState(ctx, appID, nil, 668, true)
	})
	c.Assert(err, jc.ErrorIsNil)
	checkResult(application.ScaleState{
		Scale:       666,
		ScaleTarget: 668,
		Scaling:     true,
	})

	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetApplicationScalingState(ctx, appID, ptr(667), 668, true)
	})
	c.Assert(err, jc.ErrorIsNil)
	checkResult(application.ScaleState{
		Scale:       667,
		ScaleTarget: 668,
		Scaling:     true,
	})
}

func (s *applicationStateSuite) TestSetApplicationLife(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)

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

	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetApplicationLife(ctx, appID, life.Dying)
	})
	c.Assert(err, jc.ErrorIsNil)
	checkResult(life.Dying)

	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetApplicationLife(ctx, appID, life.Dead)
	})
	c.Assert(err, jc.ErrorIsNil)
	checkResult(life.Dead)

	// Can't go backwards.
	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.SetApplicationLife(ctx, appID, life.Dying)
	})
	c.Assert(err, jc.ErrorIsNil)
	checkResult(life.Dead)
}

func (s *applicationStateSuite) TestDeleteApplication(c *gc.C) {
	// TODO(units) - add references to constraints, storage etc when those are fully cooked
	s.createApplication(c, "foo", life.Alive)

	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.DeleteApplication(ctx, "foo")
	})
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
	s.createApplication(c, "foo", life.Alive)

	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.DeleteApplication(ctx, "foo")
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.DeleteApplication(ctx, "foo")
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestDeleteDeadApplication(c *gc.C) {
	s.createApplication(c, "foo", life.Dead)

	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.DeleteApplication(ctx, "foo")
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.DeleteApplication(ctx, "foo")
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestDeleteApplicationWithUnits(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u)

	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.DeleteApplication(ctx, "foo")
	})
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

	u := application.AddUnitArg{
		UnitName: "foo/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: application.UnitAgentStatusInfo{
				StatusID: application.UnitAgentStatusExecuting,
				StatusInfo: application.StatusInfo{
					Message: "test",
					Data:    map[string]string{"foo": "bar"},
					Since:   time.Now(),
				},
			},
			WorkloadStatus: application.UnitWorkloadStatusInfo{
				StatusID: application.UnitWorkloadStatusActive,
				StatusInfo: application.StatusInfo{
					Message: "test",
					Data:    map[string]string{"foo": "bar"},
					Since:   time.Now(),
				},
			},
		},
	}
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.AddUnits(ctx, appID, u)
	})
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
		int(u.UnitStatusArg.AgentStatus.StatusID), u.UnitStatusArg.AgentStatus.Message,
		u.UnitStatusArg.AgentStatus.Since, u.UnitStatusArg.AgentStatus.Data)
	s.assertUnitStatus(
		c, "unit_workload", coreunit.UUID(unitUUID),
		int(u.UnitStatusArg.WorkloadStatus.StatusID), u.UnitStatusArg.WorkloadStatus.Message,
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

	_, err := s.state.SetCharm(context.Background(), charm.Charm{
		Metadata:      expectedMetadata,
		Manifest:      expectedManifest,
		Actions:       expectedActions,
		Config:        expectedConfig,
		LXDProfile:    expectedLXDProfile,
		Source:        charm.LocalSource,
		ReferenceName: expectedMetadata.Name,
		Revision:      42,
		Architecture:  architecture.AMD64,
	}, nil)
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

	var appID coreapplication.ID
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		var err error
		appID, err = s.state.CreateApplication(ctx, "foo", application.AddApplicationArg{
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
		})
		return err
	})
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

	var appID coreapplication.ID
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		var err error
		appID, err = s.state.CreateApplication(ctx, "foo", application.AddApplicationArg{
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
		})
		return err
	})
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

	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		_, err := s.state.CreateApplication(ctx, "foo", application.AddApplicationArg{
			Charm: charm.Charm{
				Metadata: expectedMetadata,
				Manifest: s.minimalManifest(c),
				Source:   charm.LocalSource,
			},
		})
		return err
	})
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
		Hash:      "hash",
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
	var appID coreapplication.ID
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		var err error
		appID, err = s.state.CreateApplication(ctx, "foo", application.AddApplicationArg{
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
		})
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.GetAsyncCharmDownloadInfo(context.Background(), appID)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmProvenanceNotValid)
}

func (s *applicationStateSuite) createApplication(c *gc.C, name string, l life.Life, units ...application.InsertUnitArg) coreapplication.ID {
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
	var appID coreapplication.ID
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		var err error
		appID, err = s.state.CreateApplication(ctx, name, application.AddApplicationArg{
			Platform: platform,
			Channel:  channel,
			Charm: charm.Charm{
				Metadata: charm.Metadata{
					Name: name,
					Provides: map[string]charm.Relation{
						"endpoint": {
							Name:  "endpoint",
							Key:   "endpoint",
							Role:  charm.RoleProvider,
							Scope: charm.ScopeGlobal,
						},
						"misc": {
							Name:  "misc",
							Key:   "misc",
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
		})
		if err != nil {
			return err
		}
		for _, u := range units {
			if err := s.state.InsertUnit(ctx, appID, u); err != nil {
				return err
			}
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application SET life_id = ? WHERE name = ?", l, name)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	return appID
}

func (s *applicationStateSuite) createObjectStoreBlob(c *gc.C, path string) objectstore.UUID {
	uuid := objectstoretesting.GenObjectStoreUUID(c)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Assert(err, jc.ErrorIsNil)
	return uuid
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
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.InsertUnit(ctx, appID, u)
	})
	c.Assert(err, jc.ErrorIsNil)

	var unitUUID string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", u.UnitName).Scan(&unitUUID)
	})
	c.Assert(err, jc.ErrorIsNil)
	return coreunit.UUID(unitUUID)
}
