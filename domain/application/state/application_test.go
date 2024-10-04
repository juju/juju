// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/canonical/sqlair"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/linklayerdevice"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type applicationStateSuite struct {
	schematesting.ModelSuite

	state      *ApplicationState
	charmState *CharmState
}

var _ = gc.Suite(&applicationStateSuite{})

func (s *applicationStateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	s.state = NewApplicationState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	s.charmState = NewCharmState(s.TxnRunnerFactory())
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
		OSType:       charm.Ubuntu,
		Architecture: charm.ARM64,
	}
	channel := &application.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}
	origin := charm.CharmOrigin{
		Source:        charm.CharmHubSource,
		ReferenceName: "666",
		Revision:      42,
	}

	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		_, err := s.state.CreateApplication(ctx, "666", application.AddApplicationArg{
			Platform: platform,
			Origin:   origin,
			Charm: charm.Charm{
				Metadata: charm.Metadata{
					Name: "666",
				},
			},
			Scale:   1,
			Channel: channel,
		})
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, channel, scale, origin)
}

func (s *applicationStateSuite) TestCreateApplicationsWithSameCharm(c *gc.C) {
	platform := application.Platform{
		Channel:      "666",
		OSType:       charm.Ubuntu,
		Architecture: charm.ARM64,
	}
	channel := &application.Channel{
		Track:  "track",
		Risk:   "stable",
		Branch: "branch",
	}
	origin := charm.CharmOrigin{
		Source:   charm.LocalSource,
		Revision: 42,
	}
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		_, err := s.state.CreateApplication(ctx, "foo1", application.AddApplicationArg{
			Platform: platform,
			Channel:  channel,
			Origin:   origin,
			Charm: charm.Charm{
				Metadata: charm.Metadata{
					Name: "foo",
				},
			},
		})
		if err != nil {
			return err
		}

		_, err = s.state.CreateApplication(ctx, "foo2", application.AddApplicationArg{
			Platform: platform,
			Channel:  channel,
			Origin:   origin,
			Charm: charm.Charm{
				Metadata: charm.Metadata{
					Name: "foo",
				},
			},
		})
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	scale := application.ScaleState{}
	s.assertApplication(c, "foo1", platform, channel, scale, origin)
	s.assertApplication(c, "foo2", platform, channel, scale, origin)
}

func (s *applicationStateSuite) TestCreateApplicationWithoutChannel(c *gc.C) {
	platform := application.Platform{
		Channel:      "666",
		OSType:       charm.Ubuntu,
		Architecture: charm.ARM64,
	}
	origin := charm.CharmOrigin{
		Source:        charm.LocalSource,
		ReferenceName: "666",
		Revision:      42,
	}
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		_, err := s.state.CreateApplication(ctx, "666", application.AddApplicationArg{
			Platform: platform,
			Origin:   origin,
			Charm: charm.Charm{
				Metadata: charm.Metadata{
					Name: "666",
				},
			},
			Scale: 1,
		})
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, nil, scale, origin)
}

func (s *applicationStateSuite) TestCreateApplicationWithEmptyChannel(c *gc.C) {
	platform := application.Platform{
		Channel:      "666",
		OSType:       charm.Ubuntu,
		Architecture: charm.ARM64,
	}
	channel := &application.Channel{}
	origin := charm.CharmOrigin{
		Source:   charm.LocalSource,
		Revision: 42,
	}
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		_, err := s.state.CreateApplication(ctx, "666", application.AddApplicationArg{
			Platform: platform,
			Origin:   origin,
			Charm: charm.Charm{
				Metadata: charm.Metadata{
					Name: "666",
				},
			},
			Scale: 1,
		})
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, channel, scale, origin)
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

func (s *applicationStateSuite) TestUpsertCloudService(c *gc.C) {
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
	err = s.state.UpsertCloudService(context.Background(), "foo", "provider-id", network.SpaceAddresses{})
	c.Assert(err, jc.ErrorIsNil)
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
JOIN unit u ON cc.net_node_uuid = u.net_node_uuid
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
JOIN unit u ON cc.net_node_uuid = u.net_node_uuid
JOIN cloud_container_port ccp ON ccp.cloud_container_uuid = cc.net_node_uuid
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
JOIN unit u ON cc.net_node_uuid = u.net_node_uuid
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
JOIN unit u ON cc.net_node_uuid = u.net_node_uuid
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
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM cloud_container WHERE net_node_uuid=?", netNodeUUID).Scan(&containerCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM link_layer_device WHERE net_node_uuid=?", netNodeUUID).Scan(&deviceCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM ip_address WHERE device_uuid=?", deviceUUID).Scan(&addressCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM cloud_container_port WHERE cloud_container_uuid=?", netNodeUUID).Scan(&portCount); err != nil {
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

	unitUUIDs, err := s.state.GetUnitUUIDs(context.Background(), []string{u1.UnitName, u2.UnitName})
	c.Assert(err, jc.ErrorIsNil)

	var gotUUIDs []coreunit.UUID
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT uuid FROM unit WHERE name IN (?, ?)", u1.UnitName, u2.UnitName)
		defer rows.Close()
		if err != nil {
			return err
		}
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
	_, err := s.state.GetUnitUUIDs(context.Background(), []string{"foo/666"})
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationStateSuite) TestGetUnitUUIDsOneNotFound(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)

	_, err := s.state.GetUnitUUIDs(context.Background(), []string{u1.UnitName, "foo/667"})
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
		defer rows.Close()
		if err != nil {
			return err
		}
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
	c.Assert(unitNames, jc.SameContents, []string{u1.UnitName, u2.UnitName})
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

	unitUUIDs, err := s.state.GetUnitUUIDs(context.Background(), []string{u1.UnitName})
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

	unitUUIDs, err := s.state.GetUnitUUIDs(context.Background(), []string{u1.UnitName})
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

	unitUUIDs, err := s.state.GetUnitUUIDs(context.Background(), []string{u1.UnitName})
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
		c, "unit_agent", unitUUID,
		int(u.UnitStatusArg.AgentStatus.StatusID), u.UnitStatusArg.AgentStatus.Message,
		u.UnitStatusArg.AgentStatus.Since, u.UnitStatusArg.AgentStatus.Data)
	s.assertUnitStatus(
		c, "unit_workload", unitUUID,
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

	var unitID1, unitID2, unitID3 string
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
	c.Assert(got, jc.DeepEquals, map[string]life.Life{
		unitID2: life.Alive,
	})

	got, err = s.state.GetApplicationUnitLife(context.Background(), "foo", unitID1, unitID2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, map[string]life.Life{
		unitID1: life.Dead,
		unitID2: life.Alive,
	})

	got, err = s.state.GetApplicationUnitLife(context.Background(), "foo", unitID2, unitID3)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, map[string]life.Life{
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
	origin := charm.CharmOrigin{
		Source:   charm.LocalSource,
		Revision: 42,
	}

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

	_, err := s.charmState.SetCharm(context.Background(), charm.Charm{
		Metadata:   expectedMetadata,
		Manifest:   expectedManifest,
		Actions:    expectedActions,
		Config:     expectedConfig,
		LXDProfile: expectedLXDProfile,
	}, charm.SetStateArgs{
		Source:        origin.Source,
		ReferenceName: expectedMetadata.Name,
		Revision:      origin.Revision,
		Hash:          "",
		ArchivePath:   "",
		Version:       "",
		Platform: charm.Platform{
			OSType:       charm.Ubuntu,
			Architecture: charm.AMD64,
			Channel:      "22.04",
		},
	})
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
	origin := charm.CharmOrigin{
		Source:   charm.LocalSource,
		Revision: 42,
	}

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

	revision := 42
	var appID coreapplication.ID
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		var err error
		appID, err = s.state.CreateApplication(ctx, "foo", application.AddApplicationArg{
			Charm: charm.Charm{
				Metadata:   expectedMetadata,
				Manifest:   expectedManifest,
				Actions:    expectedActions,
				Config:     expectedConfig,
				LXDProfile: expectedLXDProfile,
			},
			Origin: origin,
			Channel: &application.Channel{
				Track:  "track",
				Risk:   "stable",
				Branch: "branch",
			},
			Platform: application.Platform{
				OSType:       charm.Ubuntu,
				Architecture: charm.AMD64,
				Channel:      "22.04",
			},
		})
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	ch, origin, platform, err := s.state.GetCharmByApplicationID(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ch, gc.DeepEquals, charm.Charm{
		Metadata:   expectedMetadata,
		Manifest:   expectedManifest,
		Actions:    expectedActions,
		Config:     expectedConfig,
		LXDProfile: expectedLXDProfile,
	})
	c.Check(origin, gc.DeepEquals, charm.CharmOrigin{
		Source:   charm.LocalSource,
		Revision: revision,
		Platform: charm.Platform{
			OSType:       charm.Ubuntu,
			Channel:      "22.04",
			Architecture: charm.AMD64,
		},
	})
	c.Check(platform, gc.DeepEquals, application.Platform{
		OSType:       charm.Ubuntu,
		Architecture: charm.AMD64,
		Channel:      "22.04",
	})

	// Ensure that the charm platform is also set AND it's the same as the
	// application platform.
	var gotPlatform application.Platform
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT cp.os_id, cp.channel, cp.architecture_id
FROM application
LEFT JOIN charm_platform AS cp ON application.charm_uuid = cp.charm_uuid
WHERE uuid = ?
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
	origin := charm.CharmOrigin{
		Revision: 42,
	}

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

	revision := 42
	var appID coreapplication.ID
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		var err error
		appID, err = s.state.CreateApplication(ctx, "foo", application.AddApplicationArg{
			Charm: charm.Charm{
				Metadata: expectedMetadata,
				Manifest: expectedManifest,
				Actions:  expectedActions,
				Config:   expectedConfig,
			},
			Origin: origin,
			Platform: application.Platform{
				OSType:       charm.Ubuntu,
				Architecture: charm.AMD64,
				Channel:      "22.04",
			},
		})
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	ch, origin, _, err := s.state.GetCharmByApplicationID(context.Background(), appID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ch, gc.DeepEquals, charm.Charm{
		Metadata: expectedMetadata,
		Manifest: expectedManifest,
		Actions:  expectedActions,
		Config:   expectedConfig,
	})
	c.Check(origin, gc.DeepEquals, charm.CharmOrigin{
		Source:   charm.CharmHubSource,
		Revision: revision,
		Platform: charm.Platform{
			OSType:       charm.Ubuntu,
			Channel:      "22.04",
			Architecture: charm.AMD64,
		},
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
			},
		})
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	id := applicationtesting.GenApplicationUUID(c)

	_, _, _, err = s.state.GetCharmByApplicationID(context.Background(), id)
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

func (s *applicationStateSuite) assertApplication(
	c *gc.C,
	name string,
	platform application.Platform,
	channel *application.Channel,
	scale application.ScaleState,
	origin charm.CharmOrigin,
) {
	var (
		gotName      string
		gotUUID      string
		gotCharmUUID string
		gotPlatform  application.Platform
		gotChannel   application.Channel
		gotScale     application.ScaleState
		gotOrigin    charm.CharmOrigin
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
		err = tx.QueryRowContext(ctx, "SELECT source, reference_name, revision FROM v_charm_origin WHERE charm_uuid=?", gotCharmUUID).
			Scan(&gotOrigin.Source, &gotOrigin.ReferenceName, &gotOrigin.Revision)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotName, gc.Equals, name)
	c.Check(gotPlatform, jc.DeepEquals, platform)
	c.Check(gotScale, jc.DeepEquals, scale)
	c.Check(gotOrigin, gc.DeepEquals, origin)

	// Channel is optional, so we need to check it separately.
	if channel != nil {
		c.Check(gotChannel, gc.DeepEquals, *channel)
	} else {
		// Ensure it's empty if the original origin channel isn't set.
		// Prevent the db from sending back bogus values.
		c.Check(gotChannel, gc.DeepEquals, application.Channel{})
	}
}

func (s *applicationStateSuite) createApplication(c *gc.C, name string, l life.Life, units ...application.InsertUnitArg) coreapplication.ID {
	platform := application.Platform{
		Channel:      "22.04/stable",
		OSType:       charm.Ubuntu,
		Architecture: charm.ARM64,
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
				},
			},
			Origin: charm.CharmOrigin{
				ReferenceName: name,
				Source:        charm.CharmHubSource,
				Revision:      42,
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

func (s *applicationStateSuite) addUnit(c *gc.C, appID coreapplication.ID, u application.InsertUnitArg) string {
	err := s.state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.state.InsertUnit(ctx, appID, u)
	})
	c.Assert(err, jc.ErrorIsNil)

	var unitUUID string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", u.UnitName).Scan(&unitUUID)
	})
	c.Assert(err, jc.ErrorIsNil)
	return unitUUID
}
