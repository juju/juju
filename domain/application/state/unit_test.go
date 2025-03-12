// Copyright 2025 Canonical Ltd.
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
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/linklayerdevice"
	portstate "github.com/juju/juju/domain/port/state"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type unitStateSuite struct {
	baseSuite

	state *State
}

var _ = gc.Suite(&unitStateSuite{})

func (s *unitStateSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *unitStateSuite) assertContainerAddressValues(
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

func (s *unitStateSuite) assertContainerPortValues(c *gc.C, unitName string, ports []string) {
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

func (s *unitStateSuite) TestUpdateCAASUnitCloudContainer(c *gc.C) {
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

func (s *unitStateSuite) TestUpdateCAASUnitStatuses(c *gc.C) {
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

func (s *unitStateSuite) TestRegisterCAASUnit(c *gc.C) {
	appUUID := s.createScalingApplication(c, "foo", life.Alive, 1)

	unitName := coreunit.Name("foo/666")

	p := application.RegisterCAASUnitArg{
		UnitName:         unitName,
		PasswordHash:     "passwordhash",
		ProviderID:       "some-id",
		Address:          ptr("10.6.6.6"),
		Ports:            ptr([]string{"666"}),
		OrderedScale:     true,
		OrderedId:        0,
		StorageParentDir: c.MkDir(),
	}
	err := s.state.RegisterCAASUnit(context.Background(), appUUID, p)
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

func (s *unitStateSuite) TestRegisterCAASUnitAlreadyExists(c *gc.C) {
	unitName := coreunit.Name("foo/0")

	_ = s.createApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: unitName,
	})

	p := application.RegisterCAASUnitArg{
		UnitName:         unitName,
		PasswordHash:     "passwordhash",
		ProviderID:       "some-id",
		Address:          ptr("10.6.6.6"),
		Ports:            ptr([]string{"666"}),
		OrderedScale:     true,
		OrderedId:        0,
		StorageParentDir: c.MkDir(),
	}
	err := s.state.RegisterCAASUnit(context.Background(), "foo", p)
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

func (s *unitStateSuite) TestSetUnitPassword(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u)
	var unitUUID coreunit.UUID
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", u.UnitName).Scan(&unitUUID)
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetUnitPassword(context.Background(), unitUUID, application.PasswordInfo{
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

func (s *unitStateSuite) TestGetUnitLife(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u)

	unitLife, err := s.state.GetUnitLife(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(unitLife, gc.Equals, life.Alive)
}

func (s *unitStateSuite) TestGetUnitLifeNotFound(c *gc.C) {
	_, err := s.state.GetUnitLife(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestSetUnitLife(c *gc.C) {
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

func (s *unitStateSuite) TestSetUnitLifeNotFound(c *gc.C) {
	err := s.state.SetUnitLife(context.Background(), "foo/666", life.Dying)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestDeleteUnit(c *gc.C) {
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
		unitConstraintCount       int
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
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit_constraint WHERE unit_uuid=?", unitUUID).Scan(&unitConstraintCount); err != nil {
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
	c.Check(unitConstraintCount, gc.Equals, 0)
}

func (s *unitStateSuite) TestDeleteUnitLastUnitAppAlive(c *gc.C) {
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

func (s *unitStateSuite) TestDeleteUnitLastUnit(c *gc.C) {
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

func (s *unitStateSuite) TestGetUnitUUIDByName(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	_ = s.createApplication(c, "foo", life.Alive, u1)

	unitUUID, err := s.state.GetUnitUUIDByName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitUUID, gc.NotNil)
}

func (s *unitStateSuite) TestGetUnitUUIDByNameNotFound(c *gc.C) {
	_, err := s.state.GetUnitUUIDByName(context.Background(), "failme")
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) assertUnitStatus(c *gc.C, statusType, unitUUID coreunit.UUID, statusID int, message string, since *time.Time, data []byte) {
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

func (s *unitStateSuite) TestSetCloudContainerStatus(c *gc.C) {
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

func (s *unitStateSuite) TestSetUnitAgentStatus(c *gc.C) {
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

	err = s.state.SetUnitAgentStatus(context.Background(), unitUUID, &status)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitStatus(
		c, "unit_agent", unitUUID, int(status.Status), status.Message, status.Since, status.Data)
}

func (s *unitStateSuite) TestSetUnitAgentStatusNotFound(c *gc.C) {
	status := application.StatusInfo[application.UnitAgentStatusType]{
		Status:  application.UnitAgentStatusExecuting,
		Message: "it's executing",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	unitUUID := unittesting.GenUnitUUID(c)

	err := s.state.SetUnitAgentStatus(context.Background(), unitUUID, &status)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestGetUnitAgentStatusUnset(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)

	unitUUID, err := s.state.GetUnitUUIDByName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.GetUnitAgentStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitStatusNotFound)
}

func (s *unitStateSuite) TestGetUnitAgentStatusDead(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Dead, u1)

	unitUUID, err := s.state.GetUnitUUIDByName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.GetUnitAgentStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitIsDead)
}

func (s *unitStateSuite) TestGetUnitAgentStatus(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)

	unitUUID, err := s.state.GetUnitUUIDByName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)

	status := &application.StatusInfo[application.UnitAgentStatusType]{
		Status:  application.UnitAgentStatusExecuting,
		Message: "it's executing",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err = s.state.SetUnitAgentStatus(context.Background(), unitUUID, status)
	c.Assert(err, jc.ErrorIsNil)

	gotStatus, err := s.state.GetUnitAgentStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(gotStatus.Present, jc.IsFalse)
	assertStatusInfoEqual(c, &gotStatus.StatusInfo, status)
}

func (s *unitStateSuite) TestGetUnitAgentStatusPresent(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)

	unitUUID, err := s.state.GetUnitUUIDByName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)

	status := &application.StatusInfo[application.UnitAgentStatusType]{
		Status:  application.UnitAgentStatusExecuting,
		Message: "it's executing",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err = s.state.SetUnitAgentStatus(context.Background(), unitUUID, status)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetUnitPresence(context.Background(), coreunit.Name("foo/666"))
	c.Assert(err, jc.ErrorIsNil)

	gotStatus, err := s.state.GetUnitAgentStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(gotStatus.Present, jc.IsTrue)
	assertStatusInfoEqual(c, &gotStatus.StatusInfo, status)

	err = s.state.DeleteUnitPresence(context.Background(), coreunit.Name("foo/666"))
	c.Assert(err, jc.ErrorIsNil)

	gotStatus, err = s.state.GetUnitAgentStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(gotStatus.Present, jc.IsFalse)
	assertStatusInfoEqual(c, &gotStatus.StatusInfo, status)
}

func (s *unitStateSuite) TestGetUnitWorkloadStatusUnitNotFound(c *gc.C) {
	_, err := s.state.GetUnitWorkloadStatus(context.Background(), "missing-uuid")
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestGetUnitWorkloadStatusDead(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Dead, u1)

	unitUUID, err := s.state.GetUnitUUIDByName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.GetUnitWorkloadStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitIsDead)
}

func (s *unitStateSuite) TestGetUnitWorkloadStatusUnsetStatus(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)

	unitUUID, err := s.state.GetUnitUUIDByName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.GetUnitWorkloadStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitStatusNotFound)
}

func (s *unitStateSuite) TestSetWorkloadStatus(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)

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

	gotStatus, err := s.state.GetUnitWorkloadStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotStatus.Present, jc.IsFalse)
	assertStatusInfoEqual(c, &gotStatus.StatusInfo, status)

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
	c.Check(gotStatus.Present, jc.IsFalse)
	assertStatusInfoEqual(c, &gotStatus.StatusInfo, status)
}

func (s *unitStateSuite) TestSetWorkloadStatusPresent(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)

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

	err = s.state.SetUnitPresence(context.Background(), coreunit.Name("foo/666"))
	c.Assert(err, jc.ErrorIsNil)

	gotStatus, err := s.state.GetUnitWorkloadStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotStatus.Present, jc.IsTrue)
	assertStatusInfoEqual(c, &gotStatus.StatusInfo, status)

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
	c.Check(gotStatus.Present, jc.IsTrue)
	assertStatusInfoEqual(c, &gotStatus.StatusInfo, status)
}

func (s *unitStateSuite) TestSetUnitWorkloadStatusNotFound(c *gc.C) {
	status := application.StatusInfo[application.WorkloadStatusType]{
		Status:  application.WorkloadStatusTerminated,
		Message: "it's terminated",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err := s.state.SetUnitWorkloadStatus(context.Background(), "missing-uuid", &status)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestGetUnitCloudContainerStatusUnset(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)

	unitUUID, err := s.state.GetUnitUUIDByName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.GetUnitCloudContainerStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitStatusNotFound)
}

func (s *unitStateSuite) TestGetUnitCloudContainerStatusUnitNotFound(c *gc.C) {
	_, err := s.state.GetUnitCloudContainerStatus(context.Background(), "missing-uuid")
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestGetUnitCloudContainerStatusDead(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Dead, u1)

	unitUUID, err := s.state.GetUnitUUIDByName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.GetUnitCloudContainerStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitIsDead)
}

func (s *unitStateSuite) TestGetUnitCloudContainerStatus(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)

	now := time.Now()

	unitUUID, err := s.state.GetUnitUUIDByName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)

	s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.setCloudContainerStatus(ctx, tx, unitUUID, &application.StatusInfo[application.CloudContainerStatusType]{
			Status:  application.CloudContainerStatusRunning,
			Message: "it's running",
			Data:    []byte(`{"foo": "bar"}`),
			Since:   &now,
		})
	})

	status, err := s.state.GetUnitCloudContainerStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	assertStatusInfoEqual(c, status, &application.StatusInfo[application.CloudContainerStatusType]{
		Status:  application.CloudContainerStatusRunning,
		Message: "it's running",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   &now,
	})
}

func (s *unitStateSuite) TestGetUnitWorkloadStatusesForApplication(c *gc.C) {
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
	result, ok := results["foo/666"]
	c.Assert(ok, jc.IsTrue)
	c.Check(result.Present, jc.IsFalse)
	assertStatusInfoEqual(c, &result.StatusInfo, status)
}

func (s *unitStateSuite) TestGetUnitWorkloadStatusesForApplicationMultipleUnits(c *gc.C) {
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
	c.Assert(results, gc.HasLen, 2, gc.Commentf("expected 2, got %d", len(results)))

	result1, ok := results["foo/666"]
	c.Assert(ok, jc.IsTrue)
	c.Check(result1.Present, jc.IsFalse)
	assertStatusInfoEqual(c, &result1.StatusInfo, status1)

	result2, ok := results["foo/667"]
	c.Assert(ok, jc.IsTrue)
	c.Check(result2.Present, jc.IsFalse)
	assertStatusInfoEqual(c, &result2.StatusInfo, status2)
}

func (s *unitStateSuite) TestGetUnitWorkloadStatusesForApplicationMultipleUnitsPresent(c *gc.C) {
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
	err = s.state.SetUnitPresence(context.Background(), coreunit.Name("foo/667"))
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.state.GetUnitWorkloadStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 2, gc.Commentf("expected 2, got %d", len(results)))

	result1, ok := results["foo/666"]
	c.Assert(ok, jc.IsTrue)
	c.Check(result1.Present, jc.IsFalse)
	assertStatusInfoEqual(c, &result1.StatusInfo, status1)

	result2, ok := results["foo/667"]
	c.Assert(ok, jc.IsTrue)
	c.Check(result2.Present, jc.IsTrue)
	assertStatusInfoEqual(c, &result2.StatusInfo, status2)
}

func (s *unitStateSuite) TestGetUnitWorkloadStatusesForApplicationNotFound(c *gc.C) {
	_, err := s.state.GetUnitWorkloadStatusesForApplication(context.Background(), "missing")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *unitStateSuite) TestGetUnitWorkloadStatusesForApplicationNoUnits(c *gc.C) {
	appId := s.createApplication(c, "foo", life.Alive)

	results, err := s.state.GetUnitWorkloadStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 0)
}

func (s *unitStateSuite) TestGetUnitWorkloadAndCloudContainerStatusesForApplication(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	appId := s.createApplication(c, "foo", life.Alive, u1)

	unitUUID, err := s.state.GetUnitUUIDByName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)

	workloadStatus := &application.StatusInfo[application.WorkloadStatusType]{
		Status:  application.WorkloadStatusActive,
		Message: "it's active",
		Data:    []byte(`{"bar": "foo"}`),
		Since:   ptr(time.Now()),
	}
	cloudContainerStatus := &application.StatusInfo[application.CloudContainerStatusType]{
		Status:  application.CloudContainerStatusRunning,
		Message: "it's running",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}
	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		err := s.state.setCloudContainerStatus(ctx, tx, unitUUID, cloudContainerStatus)
		if err != nil {
			return err
		}
		return s.state.setUnitWorkloadStatus(ctx, tx, unitUUID, workloadStatus)
	})
	c.Assert(err, jc.ErrorIsNil)

	workloadResults, containerResults, err := s.state.GetUnitWorkloadAndCloudContainerStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(workloadResults, gc.HasLen, 1)
	workloadResult, ok := workloadResults["foo/666"]
	c.Assert(ok, jc.IsTrue)

	c.Assert(containerResults, gc.HasLen, 1)
	containerResult, ok := containerResults["foo/666"]
	c.Assert(ok, jc.IsTrue)

	assertStatusInfoEqual(c, &workloadResult.StatusInfo, workloadStatus)
	assertStatusInfoEqual(c, &containerResult, cloudContainerStatus)
}

func (s *unitStateSuite) TestGetUnitCloudContainerStatusForApplicationMultipleUnits(c *gc.C) {
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

	status1 := &application.StatusInfo[application.CloudContainerStatusType]{
		Status:  application.CloudContainerStatusRunning,
		Message: "it's running!",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}
	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.setCloudContainerStatus(ctx, tx, unitUUID1, status1)
	})
	c.Assert(err, jc.ErrorIsNil)

	status2 := &application.StatusInfo[application.CloudContainerStatusType]{
		Status:  application.CloudContainerStatusBlocked,
		Message: "it's blocked",
		Data:    []byte(`{"bar": "foo"}`),
		Since:   ptr(time.Now()),
	}
	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.setCloudContainerStatus(ctx, tx, unitUUID2, status2)
	})
	c.Assert(err, jc.ErrorIsNil)

	_, results, err := s.state.GetUnitWorkloadAndCloudContainerStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 2)

	result1, ok := results["foo/666"]
	c.Assert(ok, jc.IsTrue)
	assertStatusInfoEqual(c, &result1, status1)

	result2, ok := results["foo/667"]
	c.Assert(ok, jc.IsTrue)
	assertStatusInfoEqual(c, &result2, status2)
}

func (s *unitStateSuite) TestGetUnitWorkloadAndCloudContainerStatusesForApplicationNotFound(c *gc.C) {
	_, _, err := s.state.GetUnitWorkloadAndCloudContainerStatusesForApplication(context.Background(), "missing")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *unitStateSuite) TestGetUnitWorkloadAndCloudContainerStatusesForApplicationNoUnits(c *gc.C) {
	appId := s.createApplication(c, "foo", life.Alive)

	_, results, err := s.state.GetUnitWorkloadAndCloudContainerStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 0)
}

func (s *unitStateSuite) TestGetUnitWorkloadAndCloudContainerStatusesForApplicationUnitsWithoutStatuses(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	u2 := application.InsertUnitArg{
		UnitName: "foo/667",
	}
	appId := s.createApplication(c, "foo", life.Alive, u1, u2)

	_, results, err := s.state.GetUnitWorkloadAndCloudContainerStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 0)
}

func (s *unitStateSuite) TestAddUnitsApplicationNotFound(c *gc.C) {
	u := application.AddUnitArg{
		UnitName: "foo/666",
	}
	uuid := testing.GenApplicationUUID(c)
	err := s.state.AddIAASUnits(context.Background(), c.MkDir(), uuid, u)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *unitStateSuite) TestAddUnitsApplicationNotALive(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Dying)
	u := application.AddUnitArg{
		UnitName: "foo/666",
	}
	err := s.state.AddIAASUnits(context.Background(), c.MkDir(), appID, u)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotAlive)
}

func (s *unitStateSuite) TestAddIAASUnits(c *gc.C) {
	s.assertAddUnits(c, model.IAAS)
}

func (s *unitStateSuite) TestAddCAASUnits(c *gc.C) {
	s.assertAddUnits(c, model.CAAS)
}

func (s *unitStateSuite) assertAddUnits(c *gc.C, modelType model.ModelType) {
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

	var err error
	if modelType == model.IAAS {
		err = s.state.AddIAASUnits(ctx, c.MkDir(), appID, u)
	} else {
		err = s.state.AddCAASUnits(ctx, c.MkDir(), appID, u)
	}
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
	s.assertUnitConstraints(c, coreunit.UUID(unitUUID), constraints.Constraints{})
}

func (s *unitStateSuite) TestInitialWatchStatementUnitLife(c *gc.C) {
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

func (s *unitStateSuite) TestSetConstraintFull(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u)
	var unitUUID coreunit.UUID
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", u.UnitName).Scan(&unitUUID)
	})
	c.Assert(err, jc.ErrorIsNil)

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

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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

	err = s.state.SetUnitConstraints(context.Background(), unitUUID, cons)
	c.Assert(err, jc.ErrorIsNil)
	constraintSpaces, constraintTags, constraintZones := s.assertUnitConstraints(c, unitUUID, cons)

	c.Check(constraintSpaces, jc.DeepEquals, []applicationSpace{
		{SpaceName: "space0", SpaceExclude: false},
		{SpaceName: "space1", SpaceExclude: true},
	})
	c.Check(constraintTags, jc.DeepEquals, []string{"tag0", "tag1"})
	c.Check(constraintZones, jc.DeepEquals, []string{"zone0", "zone1"})
}

func (s *unitStateSuite) TestSetConstraintInvalidContainerType(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u)
	var unitUUID coreunit.UUID
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", u.UnitName).Scan(&unitUUID)
	})
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.Constraints{
		Container: ptr(instance.ContainerType("invalid-container-type")),
	}
	err = s.state.SetUnitConstraints(context.Background(), unitUUID, cons)
	c.Assert(err, jc.ErrorIs, applicationerrors.InvalidUnitConstraints)
}

func (s *unitStateSuite) TestSetConstraintInvalidSpace(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u)
	var unitUUID coreunit.UUID
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", u.UnitName).Scan(&unitUUID)
	})
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.Constraints{
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "invalid-space", Exclude: false},
		}),
	}
	err = s.state.SetUnitConstraints(context.Background(), unitUUID, cons)
	c.Assert(err, jc.ErrorIs, applicationerrors.InvalidUnitConstraints)
}

func (s *unitStateSuite) TestSetConstraintsUnitNotFound(c *gc.C) {
	err := s.state.SetUnitConstraints(context.Background(), "foo", constraints.Constraints{Mem: ptr(uint64(8))})
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestSetUnitPresence(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	u2 := application.InsertUnitArg{
		UnitName: "foo/667",
	}
	s.createApplication(c, "foo", life.Alive, u1, u2)

	err := s.state.SetUnitPresence(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIsNil)

	var lastSeen time.Time
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT last_seen FROM v_unit_agent_presence WHERE name=?", "foo/666").Scan(&lastSeen); err != nil {
			return err
		}
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(lastSeen.IsZero(), jc.IsFalse)
	c.Check(lastSeen.After(time.Now().Add(-time.Minute)), jc.IsTrue)
}

func (s *unitStateSuite) TestSetUnitPresenceNotFound(c *gc.C) {
	err := s.state.SetUnitPresence(context.Background(), "foo/665")
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestDeleteUnitPresenceNotFound(c *gc.C) {
	err := s.state.DeleteUnitPresence(context.Background(), "foo/665")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *unitStateSuite) TestDeleteUnitPresence(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	u2 := application.InsertUnitArg{
		UnitName: "foo/667",
	}
	s.createApplication(c, "foo", life.Alive, u1, u2)

	err := s.state.SetUnitPresence(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIsNil)

	var lastSeen time.Time
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT last_seen FROM v_unit_agent_presence WHERE name=?", "foo/666").Scan(&lastSeen); err != nil {
			return err
		}
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(lastSeen.IsZero(), jc.IsFalse)
	c.Check(lastSeen.After(time.Now().Add(-time.Minute)), jc.IsTrue)

	err = s.state.DeleteUnitPresence(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIsNil)

	var count int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM v_unit_agent_presence WHERE name=?", "foo/666").Scan(&count); err != nil {
			return err
		}
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(count, gc.Equals, 0)
}

type applicationSpace struct {
	SpaceName    string `db:"space"`
	SpaceExclude bool   `db:"exclude"`
}

func (s *unitStateSuite) assertUnitConstraints(c *gc.C, inUnitUUID coreunit.UUID, cons constraints.Constraints) ([]applicationSpace, []string, []string) {
	var (
		unitUUID                                                            string
		constraintUUID                                                      string
		constraintSpaces                                                    []applicationSpace
		constraintTags                                                      []string
		constraintZones                                                     []string
		arch, rootDiskSource, instanceRole, instanceType, virtType, imageID sql.NullString
		cpuCores, cpuPower, mem, rootDisk                                   sql.NullInt64
		allocatePublicIP                                                    sql.NullBool
	)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT unit_uuid, constraint_uuid FROM unit_constraint WHERE unit_uuid=?", inUnitUUID).Scan(&unitUUID, &constraintUUID)
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

		row := tx.QueryRowContext(ctx, "SELECT arch, cpu_cores, cpu_power, mem, root_disk, root_disk_source, instance_role, instance_type, virt_type, allocate_public_ip, image_id FROM \"constraint\" WHERE uuid=?", constraintUUID)
		err = row.Err()
		if err != nil {
			return err
		}
		if err := row.Scan(&arch, &cpuCores, &cpuPower, &mem, &rootDisk, &rootDiskSource, &instanceRole, &instanceType, &virtType, &allocatePublicIP, &imageID); err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(constraintUUID, gc.Not(gc.Equals), "")
	c.Check(unitUUID, gc.Equals, inUnitUUID.String())

	c.Check(arch.String, gc.Equals, deptr(cons.Arch))
	c.Check(uint64(cpuCores.Int64), gc.Equals, deptr(cons.CpuCores))
	c.Check(uint64(cpuPower.Int64), gc.Equals, deptr(cons.CpuPower))
	c.Check(uint64(mem.Int64), gc.Equals, deptr(cons.Mem))
	c.Check(uint64(rootDisk.Int64), gc.Equals, deptr(cons.RootDisk))
	c.Check(rootDiskSource.String, gc.Equals, deptr(cons.RootDiskSource))
	c.Check(instanceRole.String, gc.Equals, deptr(cons.InstanceRole))
	c.Check(instanceType.String, gc.Equals, deptr(cons.InstanceType))
	c.Check(virtType.String, gc.Equals, deptr(cons.VirtType))
	c.Check(allocatePublicIP.Bool, gc.Equals, deptr(cons.AllocatePublicIP))
	c.Check(imageID.String, gc.Equals, deptr(cons.ImageID))

	return constraintSpaces, constraintTags, constraintZones
}

func deptr[T any](v *T) T {
	var zero T
	if v == nil {
		return zero
	}
	return *v
}
