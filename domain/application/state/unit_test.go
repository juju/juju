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
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	coremachinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	testing2 "github.com/juju/juju/core/network/testing"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	portstate "github.com/juju/juju/domain/port/state"
	"github.com/juju/juju/domain/status"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type unitStateSuite struct {
	baseSuite

	state *State
}

var _ = tc.Suite(&unitStateSuite{})

func (s *unitStateSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *unitStateSuite) assertContainerAddressValues(
	c *tc.C,
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
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotProviderID, tc.Equals, providerID)
	c.Assert(gotValue, tc.Equals, addressValue)
	c.Assert(gotType, tc.Equals, int(addressType))
	c.Assert(gotOrigin, tc.Equals, int(addressOrigin))
	c.Assert(gotScope, tc.Equals, int(addressScope))
	c.Assert(gotConfigType, tc.Equals, int(configType))
}

func (s *unitStateSuite) assertContainerPortValues(c *tc.C, unitName string, ports []string) {
	var gotPorts []string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotPorts, tc.SameContents, ports)
}

func (s *unitStateSuite) TestUpdateCAASUnitCloudContainer(c *tc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
		CloudContainer: &application.CloudContainer{
			ProviderID: "some-id",
			Ports:      ptr([]string{"666", "668"}),
			Address: ptr(application.ContainerAddress{
				Device: application.ContainerDevice{
					Name:              "placeholder",
					DeviceTypeID:      domainnetwork.DeviceTypeUnknown,
					VirtualPortTypeID: domainnetwork.NonVirtualPortType,
				},
				Value:       "10.6.6.6",
				AddressType: ipaddress.AddressTypeIPv4,
				ConfigType:  ipaddress.ConfigTypeDHCP,
				Scope:       ipaddress.ScopeMachineLocal,
				Origin:      ipaddress.OriginHost,
			}),
		},
	}
	s.createCAASApplication(c, "foo", life.Alive, u)

	err := s.state.UpdateCAASUnit(c.Context(), "foo/667", application.UpdateCAASUnitParams{})
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)

	cc := application.UpdateCAASUnitParams{
		ProviderID: ptr("another-id"),
		Ports:      ptr([]string{"666", "667"}),
		Address:    ptr("2001:db8::1"),
	}
	err = s.state.UpdateCAASUnit(c.Context(), "foo/666", cc)
	c.Assert(err, tc.ErrorIsNil)

	var (
		providerId string
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(providerId, tc.Equals, "another-id")

	s.assertContainerAddressValues(c, "foo/666", "another-id", "2001:db8::1",
		ipaddress.AddressTypeIPv6, ipaddress.OriginProvider, ipaddress.ScopeMachineLocal, ipaddress.ConfigTypeDHCP)
	s.assertContainerPortValues(c, "foo/666", []string{"666", "667"})
}

func (s *unitStateSuite) TestUpdateCAASUnitStatuses(c *tc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
		CloudContainer: &application.CloudContainer{
			ProviderID: "some-id",
			Ports:      ptr([]string{"666", "668"}),
			Address: ptr(application.ContainerAddress{
				Device: application.ContainerDevice{
					Name:              "placeholder",
					DeviceTypeID:      domainnetwork.DeviceTypeUnknown,
					VirtualPortTypeID: domainnetwork.NonVirtualPortType,
				},
				Value:       "10.6.6.6",
				AddressType: ipaddress.AddressTypeIPv4,
				ConfigType:  ipaddress.ConfigTypeDHCP,
				Scope:       ipaddress.ScopeMachineLocal,
				Origin:      ipaddress.OriginHost,
			}),
		},
	}
	s.createCAASApplication(c, "foo", life.Alive, u)

	unitUUID, err := s.state.GetUnitUUIDByName(c.Context(), u.UnitName)
	c.Assert(err, tc.ErrorIsNil)

	now := ptr(time.Now())
	params := application.UpdateCAASUnitParams{
		AgentStatus: ptr(status.StatusInfo[status.UnitAgentStatusType]{
			Status:  status.UnitAgentStatusIdle,
			Message: "agent status",
			Data:    []byte(`{"foo": "bar"}`),
			Since:   now,
		}),
		WorkloadStatus: ptr(status.StatusInfo[status.WorkloadStatusType]{
			Status:  status.WorkloadStatusWaiting,
			Message: "workload status",
			Data:    []byte(`{"foo": "bar"}`),
			Since:   now,
		}),
		K8sPodStatus: ptr(status.StatusInfo[status.K8sPodStatusType]{
			Status:  status.K8sPodStatusRunning,
			Message: "container status",
			Data:    []byte(`{"foo": "bar"}`),
			Since:   now,
		}),
	}
	err = s.state.UpdateCAASUnit(c.Context(), "foo/666", params)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUnitStatus(
		c, "unit_agent", unitUUID, int(status.UnitAgentStatusIdle), "agent status", now, []byte(`{"foo": "bar"}`),
	)
	s.assertUnitStatus(
		c, "unit_workload", unitUUID, int(status.WorkloadStatusWaiting), "workload status", now, []byte(`{"foo": "bar"}`),
	)
	s.assertUnitStatus(
		c, "k8s_pod", unitUUID, int(status.K8sPodStatusRunning), "container status", now, []byte(`{"foo": "bar"}`),
	)
}

func (s *unitStateSuite) TestRegisterCAASUnit(c *tc.C) {
	s.createCAASScalingApplication(c, "foo", life.Alive, 1)

	p := application.RegisterCAASUnitArg{
		UnitName:     "foo/666",
		PasswordHash: "passwordhash",
		ProviderID:   "some-id",
		Address:      ptr("10.6.6.6"),
		Ports:        ptr([]string{"666"}),
		OrderedScale: true,
		OrderedId:    0,
	}
	err := s.state.RegisterCAASUnit(c.Context(), "foo", p)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(err, tc.ErrorIsNil)
	s.assertCAASUnit(c, "foo/666", "passwordhash", "10.6.6.6", []string{"666"})
}

func (s *unitStateSuite) assertCAASUnit(c *tc.C, name, passwordHash, addressValue string, ports []string) {
	var (
		gotPasswordHash  string
		gotAddress       string
		gotAddressType   ipaddress.AddressType
		gotAddressScope  ipaddress.Scope
		gotAddressOrigin ipaddress.Origin
		gotPorts         []string
	)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT password_hash FROM unit WHERE name = ?", name).Scan(&gotPasswordHash)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, `
SELECT address_value, type_id, scope_id, origin_id FROM ip_address ipa
JOIN link_layer_device lld ON lld.uuid = ipa.device_uuid
JOIN unit u ON u.net_node_uuid = lld.net_node_uuid WHERE u.name = ?
`, name).
			Scan(&gotAddress, &gotAddressType, &gotAddressScope, &gotAddressOrigin)
		if err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `
SELECT port FROM k8s_pod_port ccp
JOIN k8s_pod cc ON cc.unit_uuid = ccp.unit_uuid
JOIN unit u ON u.uuid = cc.unit_uuid WHERE u.name = ?
`, name)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var port string
			err = rows.Scan(&port)
			if err != nil {
				return err
			}
			gotPorts = append(gotPorts, port)
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotPasswordHash, tc.Equals, passwordHash)
	c.Check(gotAddress, tc.Equals, addressValue)
	c.Check(gotAddressType, tc.Equals, ipaddress.AddressTypeIPv4)
	c.Check(gotAddressScope, tc.Equals, ipaddress.ScopeMachineLocal)
	c.Check(gotAddressOrigin, tc.Equals, ipaddress.OriginProvider)
	c.Check(gotPorts, tc.DeepEquals, ports)
}

func (s *unitStateSuite) TestRegisterCAASUnitAlreadyExists(c *tc.C) {
	unitName := coreunit.Name("foo/0")

	_ = s.createCAASApplication(c, "foo", life.Alive, application.InsertUnitArg{
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
	err := s.state.RegisterCAASUnit(c.Context(), "foo", p)
	c.Assert(err, tc.ErrorIsNil)

	var (
		providerId   string
		passwordHash string
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Check(providerId, tc.Equals, "some-id")
	c.Check(passwordHash, tc.Equals, "passwordhash")
}

func (s *unitStateSuite) TestRegisterCAASUnitReplaceDead(c *tc.C) {
	s.createCAASApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
	})

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 2 WHERE name = ?", "foo/0")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	p := application.RegisterCAASUnitArg{
		UnitName:     coreunit.Name("foo/0"),
		PasswordHash: "passwordhash",
		ProviderID:   "foo-0",
		Address:      ptr("10.6.6.6"),
		Ports:        ptr([]string{"666"}),
		OrderedScale: true,
		OrderedId:    0,
	}
	err = s.state.RegisterCAASUnit(c.Context(), "foo", p)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitAlreadyExists)
}

func (s *unitStateSuite) TestRegisterCAASUnitApplicationNotALive(c *tc.C) {
	s.createCAASApplication(c, "foo", life.Dying, application.InsertUnitArg{
		UnitName: "foo/0",
	})
	p := application.RegisterCAASUnitArg{
		UnitName:     "foo/0",
		PasswordHash: "passwordhash",
		ProviderID:   "foo-0",
		Address:      ptr("10.6.6.6"),
		Ports:        ptr([]string{"666"}),
		OrderedScale: true,
		OrderedId:    0,
	}

	err := s.state.RegisterCAASUnit(c.Context(), "foo", p)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotAlive)
}

func (s *unitStateSuite) TestRegisterCAASUnitExceedsScale(c *tc.C) {
	appUUID := s.createCAASApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
	})

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE application_scale
SET scale = ?, scale_target = ?
WHERE application_uuid = ?`, 1, 3, appUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	p := application.RegisterCAASUnitArg{
		UnitName:     "foo/2",
		PasswordHash: "passwordhash",
		ProviderID:   "foo-2",
		Address:      ptr("10.6.6.6"),
		Ports:        ptr([]string{"666"}),
		OrderedScale: true,
		OrderedId:    2,
	}

	err = s.state.RegisterCAASUnit(c.Context(), "foo", p)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotAssigned)
}

func (s *unitStateSuite) TestRegisterCAASUnitExceedsScaleTarget(c *tc.C) {
	appUUID := s.createCAASApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
	})

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE application_scale
SET scaling = ?, scale = ?, scale_target = ?
WHERE application_uuid = ?`, true, 3, 1, appUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	p := application.RegisterCAASUnitArg{
		UnitName:     "foo/2",
		PasswordHash: "passwordhash",
		ProviderID:   "foo-2",
		Address:      ptr("10.6.6.6"),
		Ports:        ptr([]string{"666"}),
		OrderedScale: true,
		OrderedId:    2,
	}

	err = s.state.RegisterCAASUnit(c.Context(), "foo", p)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotAssigned)
}

func (s *unitStateSuite) TestGetUnitLife(c *tc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createIAASApplication(c, "foo", life.Alive, u)

	unitLife, err := s.state.GetUnitLife(c.Context(), "foo/666")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(unitLife, tc.Equals, life.Alive)
}

func (s *unitStateSuite) TestGetUnitLifeNotFound(c *tc.C) {
	_, err := s.state.GetUnitLife(c.Context(), "foo/666")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestSetUnitLife(c *tc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	ctx := c.Context()
	s.createIAASApplication(c, "foo", life.Alive, u)

	checkResult := func(want life.Life) {
		var gotLife life.Life
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			err := tx.QueryRowContext(ctx, "SELECT life_id FROM unit WHERE name=?", u.UnitName).
				Scan(&gotLife)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(gotLife, tc.DeepEquals, want)
	}

	err := s.state.SetUnitLife(ctx, "foo/666", life.Dying)
	c.Assert(err, tc.ErrorIsNil)
	checkResult(life.Dying)

	err = s.state.SetUnitLife(ctx, "foo/666", life.Dead)
	c.Assert(err, tc.ErrorIsNil)
	checkResult(life.Dead)

	// Can't go backwards.
	err = s.state.SetUnitLife(ctx, "foo/666", life.Dying)
	c.Assert(err, tc.ErrorIsNil)
	checkResult(life.Dead)
}

func (s *unitStateSuite) TestSetUnitLifeNotFound(c *tc.C) {
	err := s.state.SetUnitLife(c.Context(), "foo/666", life.Dying)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestDeleteUnit(c *tc.C) {
	// TODO(units) - add references to agents etc when those are fully cooked
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
		CloudContainer: &application.CloudContainer{
			ProviderID: "provider-id",
			Ports:      ptr([]string{"666", "668"}),
			Address: ptr(application.ContainerAddress{
				Device: application.ContainerDevice{
					Name:              "placeholder",
					DeviceTypeID:      domainnetwork.DeviceTypeUnknown,
					VirtualPortTypeID: domainnetwork.NonVirtualPortType,
				},
				Value:       "10.6.6.6",
				AddressType: ipaddress.AddressTypeIPv4,
				ConfigType:  ipaddress.ConfigTypeDHCP,
				Scope:       ipaddress.ScopeMachineLocal,
				Origin:      ipaddress.OriginHost,
			}),
		},
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
	}
	u2 := application.InsertUnitArg{
		UnitName: "foo/667",
	}
	s.createIAASApplication(c, "foo", life.Alive, u1, u2)
	var (
		unitUUID    coreunit.UUID
		netNodeUUID string
		deviceUUID  string
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		if err := s.state.setK8sPodStatus(ctx, tx, unitUUID, &status.StatusInfo[status.K8sPodStatusType]{
			Status:  status.K8sPodStatusRunning,
			Message: "test",
			Data:    []byte(`{"foo": "bar"}`),
			Since:   ptr(time.Now()),
		}); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	portSt := portstate.NewState(s.TxnRunnerFactory())
	err = portSt.UpdateUnitPorts(c.Context(), unitUUID, network.GroupedPortRanges{
		"endpoint": {
			{Protocol: "tcp", FromPort: 80, ToPort: 80},
			{Protocol: "udp", FromPort: 1000, ToPort: 1500},
		},
		"misc": {
			{Protocol: "tcp", FromPort: 8080, ToPort: 8080},
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	gotIsLast, err := s.state.DeleteUnit(c.Context(), "foo/666")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotIsLast, tc.IsFalse)

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
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addressCount, tc.Equals, 0)
	c.Check(portCount, tc.Equals, 0)
	c.Check(deviceCount, tc.Equals, 0)
	c.Check(containerCount, tc.Equals, 0)
	c.Check(agentStatusCount, tc.Equals, 0)
	c.Check(workloadStatusCount, tc.Equals, 0)
	c.Check(cloudContainerStatusCount, tc.Equals, 0)
	c.Check(unitCount, tc.Equals, 0)
	c.Check(unitConstraintCount, tc.Equals, 0)
}

func (s *unitStateSuite) TestDeleteUnitLastUnitAppAlive(c *tc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createIAASApplication(c, "foo", life.Alive, u1)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id=2 WHERE name=?", u1.UnitName); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	gotIsLast, err := s.state.DeleteUnit(c.Context(), "foo/666")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotIsLast, tc.IsFalse)

	var unitCount int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit WHERE name=?", u1.UnitName).
			Scan(&unitCount); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitCount, tc.Equals, 0)
}

func (s *unitStateSuite) TestDeleteUnitLastUnit(c *tc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createIAASApplication(c, "foo", life.Dying, u1)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id=2 WHERE name=?", u1.UnitName); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	gotIsLast, err := s.state.DeleteUnit(c.Context(), "foo/666")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotIsLast, tc.IsTrue)

	var unitCount int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit WHERE name=?", u1.UnitName).
			Scan(&unitCount); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitCount, tc.Equals, 0)
}

func (s *unitStateSuite) TestGetUnitUUIDByName(c *tc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	_ = s.createIAASApplication(c, "foo", life.Alive, u1)

	unitUUID, err := s.state.GetUnitUUIDByName(c.Context(), u1.UnitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitUUID, tc.NotNil)
}

func (s *unitStateSuite) TestGetUnitUUIDByNameNotFound(c *tc.C) {
	_, err := s.state.GetUnitUUIDByName(c.Context(), "failme")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) assertUnitStatus(c *tc.C, statusType, unitUUID coreunit.UUID, statusID int, message string, since *time.Time, data []byte) {
	var (
		gotStatusID int
		gotMessage  string
		gotSince    *time.Time
		gotData     []byte
	)
	queryInfo := fmt.Sprintf(`SELECT status_id, message, data, updated_at FROM %s_status WHERE unit_uuid = ?`, statusType)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, queryInfo, unitUUID).
			Scan(&gotStatusID, &gotMessage, &gotData, &gotSince); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotStatusID, tc.Equals, statusID)
	c.Check(gotMessage, tc.Equals, message)
	c.Check(gotSince, tc.DeepEquals, since)
	c.Check(gotData, tc.DeepEquals, data)
}

func (s *unitStateSuite) TestAddUnitsApplicationNotFound(c *tc.C) {
	uuid := testing.GenApplicationUUID(c)
	_, err := s.state.AddIAASUnits(c.Context(), uuid, application.AddUnitArg{})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *unitStateSuite) TestAddUnitsApplicationNotAlive(c *tc.C) {
	appID := s.createIAASApplication(c, "foo", life.Dying)

	_, err := s.state.AddIAASUnits(c.Context(), appID, application.AddUnitArg{})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotAlive)
}

func (s *unitStateSuite) TestAddIAASUnits(c *tc.C) {
	s.assertAddUnits(c, model.IAAS)
}

func (s *unitStateSuite) TestAddCAASUnits(c *tc.C) {
	s.assertAddUnits(c, model.CAAS)
}

func (s *unitStateSuite) assertAddUnits(c *tc.C, modelType model.ModelType) {
	appID := s.createIAASApplication(c, "foo", life.Alive)

	now := ptr(time.Now())
	u := application.AddUnitArg{
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusExecuting,
				Message: "test",
				Data:    []byte(`{"foo": "bar"}`),
				Since:   now,
			},
			WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "test",
				Data:    []byte(`{"foo": "bar"}`),
				Since:   now,
			},
		},
	}

	var (
		unitNames []coreunit.Name
		err       error
	)
	if modelType == model.IAAS {
		unitNames, err = s.state.AddIAASUnits(c.Context(), appID, u)
	} else {
		unitNames, err = s.state.AddCAASUnits(c.Context(), appID, u)
	}
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitNames, tc.HasLen, 1)
	unitName := unitNames[0]
	c.Check(unitName, tc.Equals, coreunit.Name("foo/0"))

	var unitUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", unitName).Scan(&unitUUID)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
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

func (s *unitStateSuite) TestInitialWatchStatementUnitLife(c *tc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	u2 := application.InsertUnitArg{
		UnitName: "foo/667",
	}
	s.createIAASApplication(c, "foo", life.Alive, u1, u2)

	var unitID1, unitID2 string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/666").Scan(&unitID1); err != nil {
			return err
		}
		err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/667").Scan(&unitID2)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	table, queryFunc := s.state.InitialWatchStatementUnitLife("foo")
	c.Assert(table, tc.Equals, "unit")

	result, err := queryFunc(c.Context(), s.TxnRunner())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []string{unitID1, unitID2})
}

func (s *unitStateSuite) TestGetUnitRefreshAttributes(c *tc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createIAASApplication(c, "foo", life.Alive, u)

	cc := application.UpdateCAASUnitParams{
		ProviderID: ptr("another-id"),
		Ports:      ptr([]string{"666", "667"}),
		Address:    ptr("2001:db8::1"),
	}
	err := s.state.UpdateCAASUnit(c.Context(), "foo/666", cc)
	c.Assert(err, tc.ErrorIsNil)

	refreshAttributes, err := s.state.GetUnitRefreshAttributes(c.Context(), u.UnitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(refreshAttributes, tc.DeepEquals, application.UnitAttributes{
		Life:        life.Alive,
		ProviderID:  "another-id",
		ResolveMode: "none",
	})
}

func (s *unitStateSuite) TestGetUnitRefreshAttributesNoProviderID(c *tc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createIAASApplication(c, "foo", life.Alive, u)

	refreshAttributes, err := s.state.GetUnitRefreshAttributes(c.Context(), u.UnitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(refreshAttributes, tc.DeepEquals, application.UnitAttributes{
		Life:        life.Alive,
		ResolveMode: "none",
	})
}

func (s *unitStateSuite) TestGetUnitRefreshAttributesWithResolveMode(c *tc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createIAASApplication(c, "foo", life.Alive, u)

	unitUUID, err := s.state.GetUnitUUIDByName(c.Context(), u.UnitName)
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO unit_resolved (unit_uuid, mode_id) VALUES (?, 0)", unitUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	refreshAttributes, err := s.state.GetUnitRefreshAttributes(c.Context(), u.UnitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(refreshAttributes, tc.DeepEquals, application.UnitAttributes{
		Life:        life.Alive,
		ResolveMode: "retry-hooks",
	})
}

func (s *unitStateSuite) TestGetUnitRefreshAttributesDeadLife(c *tc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createIAASApplication(c, "foo", life.Alive, u)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 2 WHERE name = ?", u.UnitName)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	refreshAttributes, err := s.state.GetUnitRefreshAttributes(c.Context(), u.UnitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(refreshAttributes, tc.DeepEquals, application.UnitAttributes{
		Life:        life.Dead,
		ResolveMode: "none",
	})
}

func (s *unitStateSuite) TestGetUnitRefreshAttributesDyingLife(c *tc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createIAASApplication(c, "foo", life.Alive, u)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 1 WHERE name = ?", u.UnitName)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	refreshAttributes, err := s.state.GetUnitRefreshAttributes(c.Context(), u.UnitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(refreshAttributes, tc.DeepEquals, application.UnitAttributes{
		Life:        life.Dying,
		ResolveMode: "none",
	})
}

func (s *unitStateSuite) TestSetConstraintFull(c *tc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createIAASApplication(c, "foo", life.Alive, u)
	var unitUUID coreunit.UUID
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", u.UnitName).Scan(&unitUUID)
	})
	c.Assert(err, tc.ErrorIsNil)

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

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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

	err = s.state.SetUnitConstraints(c.Context(), unitUUID, cons)
	c.Assert(err, tc.ErrorIsNil)
	constraintSpaces, constraintTags, constraintZones := s.assertUnitConstraints(c, unitUUID, cons)

	c.Check(constraintSpaces, tc.DeepEquals, []applicationSpace{
		{SpaceName: "space0", SpaceExclude: false},
		{SpaceName: "space1", SpaceExclude: true},
	})
	c.Check(constraintTags, tc.DeepEquals, []string{"tag0", "tag1"})
	c.Check(constraintZones, tc.DeepEquals, []string{"zone0", "zone1"})
}

func (s *unitStateSuite) TestSetConstraintInvalidContainerType(c *tc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createIAASApplication(c, "foo", life.Alive, u)
	var unitUUID coreunit.UUID
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", u.UnitName).Scan(&unitUUID)
	})
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.Constraints{
		Container: ptr(instance.ContainerType("invalid-container-type")),
	}
	err = s.state.SetUnitConstraints(c.Context(), unitUUID, cons)
	c.Assert(err, tc.ErrorIs, applicationerrors.InvalidUnitConstraints)
}

func (s *unitStateSuite) TestSetConstraintInvalidSpace(c *tc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createIAASApplication(c, "foo", life.Alive, u)
	var unitUUID coreunit.UUID
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", u.UnitName).Scan(&unitUUID)
	})
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.Constraints{
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "invalid-space", Exclude: false},
		}),
	}
	err = s.state.SetUnitConstraints(c.Context(), unitUUID, cons)
	c.Assert(err, tc.ErrorIs, applicationerrors.InvalidUnitConstraints)
}

func (s *unitStateSuite) TestSetConstraintsUnitNotFound(c *tc.C) {
	err := s.state.SetUnitConstraints(c.Context(), "foo", constraints.Constraints{Mem: ptr(uint64(8))})
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestGetAllUnitNamesNoUnits(c *tc.C) {
	names, err := s.state.GetAllUnitNames(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(names, tc.DeepEquals, []coreunit.Name{})
}

func (s *unitStateSuite) TestGetAllUnitNames(c *tc.C) {
	s.createIAASApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/666",
	}, application.InsertUnitArg{
		UnitName: "foo/667",
	})
	s.createIAASApplication(c, "bar", life.Alive, application.InsertUnitArg{
		UnitName: "bar/666",
	})

	names, err := s.state.GetAllUnitNames(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(names, tc.SameContents, []coreunit.Name{"foo/666", "foo/667", "bar/666"})
}

func (s *unitStateSuite) TestGetUnitNamesForApplicationNotFound(c *tc.C) {
	_, err := s.state.GetUnitNamesForApplication(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *unitStateSuite) TestGetUnitNamesForApplicationDead(c *tc.C) {
	appUUID := s.createIAASApplication(c, "deadapp", life.Dead)
	_, err := s.state.GetUnitNamesForApplication(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationIsDead)
}

func (s *unitStateSuite) TestGetUnitNamesForApplicationNoUnits(c *tc.C) {
	appUUID := s.createIAASApplication(c, "foo", life.Alive)
	names, err := s.state.GetUnitNamesForApplication(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(names, tc.DeepEquals, []coreunit.Name{})
}

func (s *unitStateSuite) TestGetUnitNamesForApplication(c *tc.C) {
	appUUID := s.createIAASApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/666",
	}, application.InsertUnitArg{
		UnitName: "foo/667",
	})
	s.createIAASApplication(c, "bar", life.Alive, application.InsertUnitArg{
		UnitName: "bar/666",
	})

	names, err := s.state.GetUnitNamesForApplication(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(names, tc.SameContents, []coreunit.Name{"foo/666", "foo/667"})
}

func (s *unitStateSuite) TestGetUnitNamesForNetNodeNotFound(c *tc.C) {
	_, err := s.state.GetUnitNamesForNetNode(c.Context(), "doink")
	c.Assert(err, tc.ErrorIs, applicationerrors.NetNodeNotFound)
}

func (s *unitStateSuite) TestGetUnitNamesForNetNodeNoUnits(c *tc.C) {
	var netNode network.NetNodeUUID
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		netNode, err = s.state.placeMachine(ctx, tx, deployment.Placement{
			Type: deployment.PlacementTypeUnset,
		})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(netNode, tc.Not(tc.Equals), "")

	names, err := s.state.GetUnitNamesForNetNode(c.Context(), netNode)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(names, tc.DeepEquals, []coreunit.Name{})
}

func (s *unitStateSuite) TestGetUnitNamesForNetNode(c *tc.C) {
	s.createIAASApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
		Placement: deployment.Placement{
			Directive: "0",
		},
	}, application.InsertUnitArg{
		UnitName: "foo/1",
		Placement: deployment.Placement{
			Type:      deployment.PlacementTypeMachine,
			Directive: "0",
		},
	}, application.InsertUnitArg{
		UnitName: "foo/2",
		Placement: deployment.Placement{
			Directive: "1",
		},
	})

	netNodeUUID, err := s.state.GetMachineNetNodeUUIDFromName(c.Context(), "0")
	c.Assert(err, tc.ErrorIsNil)

	names, err := s.state.GetUnitNamesForNetNode(c.Context(), netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(names, tc.DeepEquals, []coreunit.Name{"foo/0", "foo/1"})
}

func (s *unitStateSuite) TestGetUnitWorkloadVersion(c *tc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createIAASApplication(c, "foo", life.Alive, u)

	workloadVersion, err := s.state.GetUnitWorkloadVersion(c.Context(), u.UnitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(workloadVersion, tc.Equals, "")
}

func (s *unitStateSuite) TestGetUnitWorkloadVersionNotFound(c *tc.C) {
	_, err := s.state.GetUnitWorkloadVersion(c.Context(), coreunit.Name("foo/666"))
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestSetUnitWorkloadVersion(c *tc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createIAASApplication(c, "foo", life.Alive, u)

	err := s.state.SetUnitWorkloadVersion(c.Context(), u.UnitName, "v1.0.0")
	c.Assert(err, tc.ErrorIsNil)

	workloadVersion, err := s.state.GetUnitWorkloadVersion(c.Context(), u.UnitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(workloadVersion, tc.Equals, "v1.0.0")
}

func (s *unitStateSuite) TestSetUnitWorkloadVersionMultiple(c *tc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	u2 := application.InsertUnitArg{
		UnitName: "foo/667",
	}
	appID := s.createIAASApplication(c, "foo", life.Alive, u1, u2)

	s.assertApplicationWorkloadVersion(c, appID, "")

	err := s.state.SetUnitWorkloadVersion(c.Context(), u1.UnitName, "v1.0.0")
	c.Assert(err, tc.ErrorIsNil)

	s.assertApplicationWorkloadVersion(c, appID, "v1.0.0")

	err = s.state.SetUnitWorkloadVersion(c.Context(), u2.UnitName, "v2.0.0")
	c.Assert(err, tc.ErrorIsNil)

	s.assertApplicationWorkloadVersion(c, appID, "v2.0.0")

	workloadVersion, err := s.state.GetUnitWorkloadVersion(c.Context(), u1.UnitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(workloadVersion, tc.Equals, "v1.0.0")

	workloadVersion, err = s.state.GetUnitWorkloadVersion(c.Context(), u2.UnitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(workloadVersion, tc.Equals, "v2.0.0")

	s.assertApplicationWorkloadVersion(c, appID, "v2.0.0")
}

func (s *unitStateSuite) TestGetUnitMachineUUID(c *tc.C) {
	unitName := coreunittesting.GenNewName(c, "foo/666")
	appUUID := s.createIAASApplication(c, "foo", life.Alive)
	unitUUID := s.addUnit(c, unitName, appUUID)
	_, machineUUID := s.addMachineToUnit(c, unitUUID)

	machine, err := s.state.GetUnitMachineUUID(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine, tc.Equals, machineUUID)
}

func (s *unitStateSuite) TestGetUnitMachineUUIDNotAssigned(c *tc.C) {
	unitName := coreunittesting.GenNewName(c, "foo/666")
	appUUID := s.createIAASApplication(c, "foo", life.Alive)
	s.addUnit(c, unitName, appUUID)

	_, err := s.state.GetUnitMachineUUID(c.Context(), unitName)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitMachineNotAssigned)
}

func (s *unitStateSuite) TestGetUnitMachineUUIDUnitNotFound(c *tc.C) {
	unitName := coreunittesting.GenNewName(c, "foo/666")

	_, err := s.state.GetUnitMachineUUID(c.Context(), unitName)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestGetUnitMachineUUIDIsDead(c *tc.C) {
	unitName := coreunittesting.GenNewName(c, "foo/666")
	appUUID := s.createIAASApplication(c, "foo", life.Alive)
	s.addUnitWithLife(c, unitName, appUUID, life.Dead)

	_, err := s.state.GetUnitMachineUUID(c.Context(), unitName)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitIsDead)
}

func (s *unitStateSuite) TestGetUnitMachineName(c *tc.C) {
	unitName := coreunittesting.GenNewName(c, "foo/666")
	appUUID := s.createIAASApplication(c, "foo", life.Alive)
	unitUUID := s.addUnit(c, unitName, appUUID)
	machineName, _ := s.addMachineToUnit(c, unitUUID)

	machine, err := s.state.GetUnitMachineName(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine, tc.Equals, machineName)
}

func (s *unitStateSuite) TestGetUnitMachineNameNotAssigned(c *tc.C) {
	unitName := coreunittesting.GenNewName(c, "foo/666")
	appUUID := s.createIAASApplication(c, "foo", life.Alive)
	s.addUnit(c, unitName, appUUID)

	_, err := s.state.GetUnitMachineName(c.Context(), unitName)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitMachineNotAssigned)
}

func (s *unitStateSuite) TestGetUnitMachineNameUnitNotFound(c *tc.C) {
	unitName := coreunittesting.GenNewName(c, "foo/666")

	_, err := s.state.GetUnitMachineName(c.Context(), unitName)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestGetUnitMachineNameIsDead(c *tc.C) {
	unitName := coreunittesting.GenNewName(c, "foo/666")
	appUUID := s.createIAASApplication(c, "foo", life.Alive)
	s.addUnitWithLife(c, unitName, appUUID, life.Dead)

	_, err := s.state.GetUnitMachineName(c.Context(), unitName)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitIsDead)
}

func (s *unitStateSuite) assertApplicationWorkloadVersion(c *tc.C, appID coreapplication.ID, expected string) {
	var version string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT version FROM application_workload_version WHERE application_uuid=?", appID).Scan(&version)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(version, tc.Equals, expected)
}

func (s *unitStateSuite) TestSetUnitWorkloadVersionNotFound(c *tc.C) {
	err := s.state.SetUnitWorkloadVersion(c.Context(), coreunit.Name("foo/666"), "v1.0.0")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestGetUnitAndK8sServiceAddressesIncludingK8sService(c *tc.C) {
	appID := s.createIAASApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
	})
	unitUUID, err := s.state.GetUnitUUIDByName(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		insertNetNode0 := `INSERT INTO net_node (uuid) VALUES (?)`
		_, err := tx.ExecContext(ctx, insertNetNode0, "pod-net-node-uuid")
		if err != nil {
			return err
		}
		insertNetNode1 := `INSERT INTO net_node (uuid) VALUES (?)`
		_, err = tx.ExecContext(ctx, insertNetNode1, "svc-net-node-uuid")
		if err != nil {
			return err
		}
		updateUnit := `UPDATE unit SET net_node_uuid = ? WHERE name = ?`
		_, err = tx.ExecContext(ctx, updateUnit, "pod-net-node-uuid", "foo/0")
		if err != nil {
			return err
		}
		insertSvc := `INSERT INTO k8s_service (uuid, net_node_uuid, application_uuid, provider_id) VALUES (?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertSvc, "svc-uuid", "svc-net-node-uuid", appID, "provider-id")
		if err != nil {
			return err
		}
		insertLLD0 := `INSERT INTO link_layer_device (uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id) VALUES (?, ?, ?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertLLD0, "lld0-uuid", "pod-net-node-uuid", "lld0-name", 1500, "00:11:22:33:44:55", 0, 0)
		if err != nil {
			return err
		}
		insertLLD1 := `INSERT INTO link_layer_device (uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id) VALUES (?, ?, ?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertLLD1, "lld1-uuid", "svc-net-node-uuid", "lld1-name", 1500, "00:11:22:33:44:66", 0, 0)
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
		insertIPAddress0 := `INSERT INTO ip_address (uuid, device_uuid, address_value, net_node_uuid, type_id, scope_id, origin_id, config_type_id, subnet_uuid) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertIPAddress0, "ip-address0-uuid", "lld0-uuid", "10.0.0.1", "pod-net-node-uuid", 0, 3, 0, 1, "subnet-uuid")
		if err != nil {
			return err
		}
		insertIPAddress1 := `INSERT INTO ip_address (uuid, device_uuid, address_value, net_node_uuid, type_id, scope_id, origin_id, config_type_id, subnet_uuid) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertIPAddress1, "ip-address1-uuid", "lld1-uuid", "10.0.0.2", "svc-net-node-uuid", 0, 1, 1, 1, "subnet-uuid")
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	addr, err := s.state.GetUnitAndK8sServiceAddresses(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addr, tc.DeepEquals, network.SpaceAddresses{
		{
			SpaceID: "space0-uuid",
			Origin:  network.OriginMachine,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.1",
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
				ConfigType: network.ConfigDHCP,
			},
		},
		{
			SpaceID: "space0-uuid",
			Origin:  network.OriginProvider,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.2",
				Type:       network.IPv4Address,
				Scope:      network.ScopePublic,
				ConfigType: network.ConfigDHCP,
			},
		},
	})
}

func (s *unitStateSuite) TestGetUnitAndK8sServiceAddressesWithoutK8sService(c *tc.C) {
	_ = s.createIAASApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
	})
	unitUUID, err := s.state.GetUnitUUIDByName(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		insertNetNode0 := `INSERT INTO net_node (uuid) VALUES (?)`
		_, err := tx.ExecContext(ctx, insertNetNode0, "machine-net-node-uuid")
		if err != nil {
			return err
		}
		updateUnit := `UPDATE unit SET net_node_uuid = ? WHERE name = ?`
		_, err = tx.ExecContext(ctx, updateUnit, "machine-net-node-uuid", "foo/0")
		if err != nil {
			return err
		}
		insertLLD0 := `INSERT INTO link_layer_device (uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id) VALUES (?, ?, ?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertLLD0, "lld0-uuid", "machine-net-node-uuid", "lld0-name", 1500, "00:11:22:33:44:55", 0, 0)
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
		insertIPAddress0 := `INSERT INTO ip_address (uuid, net_node_uuid, device_uuid, address_value, type_id, scope_id, origin_id, config_type_id, subnet_uuid) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertIPAddress0, "ip-address0-uuid", "machine-net-node-uuid", "lld0-uuid", "10.0.0.1", 0, 3, 1, 1, "subnet-uuid")
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	addr, err := s.state.GetUnitAndK8sServiceAddresses(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addr, tc.DeepEquals, network.SpaceAddresses{
		{
			SpaceID: "space0-uuid",
			Origin:  network.OriginProvider,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.1",
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
				ConfigType: network.ConfigDHCP,
			},
		},
	})
}

func (s *unitStateSuite) TestGetUnitAndK8sServiceAddressesNotFound(c *tc.C) {
	_, err := s.state.GetUnitAndK8sServiceAddresses(c.Context(), coreunit.UUID("foo"))
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestGetUnitAddresses(c *tc.C) {
	_ = s.createIAASApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
	})
	unitUUID, err := s.state.GetUnitUUIDByName(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		insertNetNode0 := `INSERT INTO net_node (uuid) VALUES (?)`
		_, err := tx.ExecContext(ctx, insertNetNode0, "machine-net-node-uuid")
		if err != nil {
			return err
		}
		updateUnit := `UPDATE unit SET net_node_uuid = ? WHERE name = ?`
		_, err = tx.ExecContext(ctx, updateUnit, "machine-net-node-uuid", "foo/0")
		if err != nil {
			return err
		}
		insertLLD0 := `INSERT INTO link_layer_device (uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id) VALUES (?, ?, ?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertLLD0, "lld0-uuid", "machine-net-node-uuid", "lld0-name", 1500, "00:11:22:33:44:55", 0, 0)
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
		insertIPAddress0 := `INSERT INTO ip_address (uuid, device_uuid, address_value, net_node_uuid, type_id, scope_id, origin_id, config_type_id, subnet_uuid) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertIPAddress0, "ip-address0-uuid", "lld0-uuid", "10.0.0.1", "machine-net-node-uuid", 0, 3, 1, 1, "subnet-uuid")
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	addr, err := s.state.GetUnitAddresses(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addr, tc.DeepEquals, network.SpaceAddresses{
		{
			SpaceID: "space0-uuid",
			Origin:  network.OriginProvider,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.1",
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
				ConfigType: network.ConfigDHCP,
			},
		},
	})
}

func (s *unitStateSuite) TestGetUnitNetNodesNotFound(c *tc.C) {
	_, err := s.state.GetUnitNetNodes(c.Context(), "unknown-unit-uuid")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestGetUnitNetNodesK8s(c *tc.C) {
	appID := s.createIAASApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
	})
	unitUUID, err := s.state.GetUnitUUIDByName(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)

	unitNetNodeUUID := testing2.GenNetNodeUUID(c)
	serviceNetNodeUUID := testing2.GenNetNodeUUID(c)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		insertNetNode0 := `INSERT INTO net_node (uuid) VALUES (?)`
		_, err := tx.ExecContext(ctx, insertNetNode0, unitNetNodeUUID)
		if err != nil {
			return err
		}
		insertNetNode1 := `INSERT INTO net_node (uuid) VALUES (?)`
		_, err = tx.ExecContext(ctx, insertNetNode1, serviceNetNodeUUID)
		if err != nil {
			return err
		}
		updateUnit := `UPDATE unit SET net_node_uuid = ? WHERE name = ?`
		_, err = tx.ExecContext(ctx, updateUnit, unitNetNodeUUID, "foo/0")
		if err != nil {
			return err
		}
		insertSvc := `INSERT INTO k8s_service (uuid, net_node_uuid, application_uuid, provider_id) VALUES (?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertSvc, "svc-uuid", serviceNetNodeUUID, appID, "provider-id")
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	netNodeUUID, err := s.state.GetUnitNetNodes(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(netNodeUUID, tc.SameContents, []network.NetNodeUUID{serviceNetNodeUUID, unitNetNodeUUID})
}

func (s *unitStateSuite) TestGetUnitNetNodesMachine(c *tc.C) {
	_ = s.createIAASApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
	})
	unitUUID, err := s.state.GetUnitUUIDByName(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		insertNetNode0 := `INSERT INTO net_node (uuid) VALUES (?)`
		_, err := tx.ExecContext(ctx, insertNetNode0, "machine-net-node-uuid")
		if err != nil {
			return err
		}
		updateUnit := `UPDATE unit SET net_node_uuid = ? WHERE name = ?`
		_, err = tx.ExecContext(ctx, updateUnit, "machine-net-node-uuid", "foo/0")
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	netNodeUUID, err := s.state.GetUnitNetNodes(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(netNodeUUID, tc.SameContents, []network.NetNodeUUID{"machine-net-node-uuid"})
}

func (s *unitStateSuite) TestGetUnitAddressesNotFound(c *tc.C) {
	_, err := s.state.GetUnitAddresses(c.Context(), coreunit.UUID("foo"))
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

type applicationSpace struct {
	SpaceName    string `db:"space"`
	SpaceExclude bool   `db:"exclude"`
}

func (s *unitStateSuite) assertUnitConstraints(c *tc.C, inUnitUUID coreunit.UUID, cons constraints.Constraints) ([]applicationSpace, []string, []string) {
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
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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

		row := tx.QueryRowContext(ctx, `
SELECT arch, cpu_cores, cpu_power, mem, root_disk, root_disk_source, instance_role, instance_type, virt_type, allocate_public_ip, image_id
FROM "constraint"
WHERE uuid=?`, constraintUUID)
		err = row.Err()
		if err != nil {
			return err
		}
		if err := row.Scan(&arch, &cpuCores, &cpuPower, &mem, &rootDisk, &rootDiskSource, &instanceRole, &instanceType, &virtType, &allocatePublicIP, &imageID); err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(constraintUUID, tc.Not(tc.Equals), "")
	c.Check(unitUUID, tc.Equals, inUnitUUID.String())

	c.Check(arch.String, tc.Equals, deptr(cons.Arch))
	c.Check(uint64(cpuCores.Int64), tc.Equals, deptr(cons.CpuCores))
	c.Check(uint64(cpuPower.Int64), tc.Equals, deptr(cons.CpuPower))
	c.Check(uint64(mem.Int64), tc.Equals, deptr(cons.Mem))
	c.Check(uint64(rootDisk.Int64), tc.Equals, deptr(cons.RootDisk))
	c.Check(rootDiskSource.String, tc.Equals, deptr(cons.RootDiskSource))
	c.Check(instanceRole.String, tc.Equals, deptr(cons.InstanceRole))
	c.Check(instanceType.String, tc.Equals, deptr(cons.InstanceType))
	c.Check(virtType.String, tc.Equals, deptr(cons.VirtType))
	c.Check(allocatePublicIP.Bool, tc.Equals, deptr(cons.AllocatePublicIP))
	c.Check(imageID.String, tc.Equals, deptr(cons.ImageID))

	return constraintSpaces, constraintTags, constraintZones
}

func (s *unitStateSuite) addUnit(c *tc.C, unitName coreunit.Name, appUUID coreapplication.ID) coreunit.UUID {
	return s.addUnitWithLife(c, unitName, appUUID, life.Alive)
}

func (s *unitStateSuite) addUnitWithLife(c *tc.C, unitName coreunit.Name, appUUID coreapplication.ID, l life.Life) coreunit.UUID {
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

func (s *unitStateSuite) addMachineToUnit(c *tc.C, unitUUID coreunit.UUID) (coremachine.Name, coremachine.UUID) {
	machineUUID := coremachinetesting.GenUUID(c)
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

type unitStateSubordinateSuite struct {
	unitStateSuite
}

var _ = tc.Suite(&unitStateSubordinateSuite{})

func (s *unitStateSubordinateSuite) TestAddIAASSubordinateUnit(c *tc.C) {
	// Arrange:
	pUnitName := coreunittesting.GenNewName(c, "foo/666")
	s.createIAASApplication(c, "principal", life.Alive, application.InsertUnitArg{
		UnitName: pUnitName,
	})

	sAppID := s.createSubordinateApplication(c, "subordinate", life.Alive)

	// Act:
	sUnitName, err := s.state.AddIAASSubordinateUnit(c.Context(), application.SubordinateUnitArg{
		SubordinateAppID:  sAppID,
		PrincipalUnitName: pUnitName,
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sUnitName, tc.Equals, coreunittesting.GenNewName(c, "subordinate/0"))
	s.assertUnitPrincipal(c, pUnitName, sUnitName)
	s.assertUnitMachinesMatch(c, pUnitName, sUnitName)
}

// TestAddIAASSubordinateUnitSecondSubordinate tests that a second subordinate unit
// can be added to an app with no issues.
func (s *unitStateSubordinateSuite) TestAddIAASSubordinateUnitSecondSubordinate(c *tc.C) {
	// Arrange: add subordinate application.
	sAppID := s.createSubordinateApplication(c, "subordinate", life.Alive)

	// Arrange: add principal app and add a subordinate unit on it.
	pUnitName1 := coreunittesting.GenNewName(c, "foo/666")
	pUnitName2 := coreunittesting.GenNewName(c, "foo/667")
	s.createIAASApplication(c, "principal", life.Alive, application.InsertUnitArg{
		UnitName: pUnitName1,
	}, application.InsertUnitArg{
		UnitName: pUnitName2,
	})
	_, err := s.state.AddIAASSubordinateUnit(c.Context(), application.SubordinateUnitArg{
		SubordinateAppID:  sAppID,
		PrincipalUnitName: pUnitName1,
	})
	c.Assert(err, tc.ErrorIsNil)

	// Act: Add a second subordinate unit
	sUnitName2, err := s.state.AddIAASSubordinateUnit(c.Context(), application.SubordinateUnitArg{
		SubordinateAppID:  sAppID,
		PrincipalUnitName: pUnitName2,
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sUnitName2, tc.Equals, coreunittesting.GenNewName(c, "subordinate/1"))
	s.assertUnitPrincipal(c, pUnitName2, sUnitName2)
	s.assertUnitMachinesMatch(c, pUnitName2, sUnitName2)
}

func (s *unitStateSubordinateSuite) TestAddIAASSubordinateUnitTwiceToSameUnit(c *tc.C) {
	// Arrange:
	pUnitName := coreunittesting.GenNewName(c, "foo/666")
	s.createIAASApplication(c, "principal", life.Alive, application.InsertUnitArg{
		UnitName: pUnitName,
	})

	sAppID := s.createSubordinateApplication(c, "subordinate", life.Alive)

	// Arrange: Add the first subordinate.
	_, err := s.state.AddIAASSubordinateUnit(c.Context(), application.SubordinateUnitArg{
		SubordinateAppID:  sAppID,
		PrincipalUnitName: pUnitName,
	})
	c.Assert(err, tc.ErrorIsNil)

	// Act: try adding a second subordinate to the same unit.
	_, err = s.state.AddIAASSubordinateUnit(c.Context(), application.SubordinateUnitArg{
		SubordinateAppID:  sAppID,
		PrincipalUnitName: pUnitName,
	})

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitAlreadyHasSubordinate)
}

func (s *unitStateSubordinateSuite) TestAddIAASSubordinateUnitWithoutMachine(c *tc.C) {
	// Arrange:
	pUnitName := coreunittesting.GenNewName(c, "foo/666")
	pAppUUID := s.createIAASApplication(c, "principal", life.Alive)
	s.addUnit(c, pUnitName, pAppUUID)

	sAppID := s.createSubordinateApplication(c, "subordinate", life.Alive)

	// Act:
	_, err := s.state.AddIAASSubordinateUnit(c.Context(), application.SubordinateUnitArg{
		SubordinateAppID:  sAppID,
		PrincipalUnitName: pUnitName,
	})

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.MachineNotFound)
}

func (s *unitStateSubordinateSuite) TestAddIAASSubordinateUnitApplicationNotAlive(c *tc.C) {
	// Arrange:
	pUnitName := coreunittesting.GenNewName(c, "foo/666")

	sAppID := s.createSubordinateApplication(c, "subordinate", life.Dying)

	// Act:
	_, err := s.state.AddIAASSubordinateUnit(c.Context(), application.SubordinateUnitArg{
		SubordinateAppID:  sAppID,
		PrincipalUnitName: pUnitName,
	})

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotAlive)
}

func (s *unitStateSubordinateSuite) TestAddIAASSubordinateUnitPrincipalNotFound(c *tc.C) {
	// Arrange:
	pUnitName := coreunittesting.GenNewName(c, "foo/666")

	sAppID := s.createSubordinateApplication(c, "subordinate", life.Alive)

	// Act:
	_, err := s.state.AddIAASSubordinateUnit(c.Context(), application.SubordinateUnitArg{
		SubordinateAppID:  sAppID,
		PrincipalUnitName: pUnitName,
	})

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSubordinateSuite) TestDeleteUnitDeletesASubordinate(c *tc.C) {
	// Arrange:
	pUnitName := coreunittesting.GenNewName(c, "foo/666")
	s.createIAASApplication(c, "principal", life.Alive, application.InsertUnitArg{
		UnitName: pUnitName,
	})

	sAppID := s.createSubordinateApplication(c, "subordinate", life.Alive)

	sUnitName, err := s.state.AddIAASSubordinateUnit(c.Context(), application.SubordinateUnitArg{
		SubordinateAppID:  sAppID,
		PrincipalUnitName: pUnitName,
	})
	c.Assert(err, tc.ErrorIsNil)

	// Act:
	_, err = s.state.DeleteUnit(c.Context(), sUnitName)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitStateSubordinateSuite) TestDeleteUnitDeleteUnitWithSubordinate(c *tc.C) {
	// Arrange:
	pUnitName := coreunittesting.GenNewName(c, "foo/666")
	s.createIAASApplication(c, "principal", life.Alive, application.InsertUnitArg{
		UnitName: pUnitName,
	})

	sAppID := s.createSubordinateApplication(c, "subordinate", life.Alive)

	_, err := s.state.AddIAASSubordinateUnit(c.Context(), application.SubordinateUnitArg{
		SubordinateAppID:  sAppID,
		PrincipalUnitName: pUnitName,
	})
	c.Assert(err, tc.ErrorIsNil)

	// Act:
	_, err = s.state.DeleteUnit(c.Context(), pUnitName)

	// Assert
	c.Assert(err, tc.NotNil)
}

func (s *unitStateSubordinateSuite) TestIsSubordinateApplication(c *tc.C) {
	// Arrange:
	appID := s.createSubordinateApplication(c, "sub", life.Alive)

	// Act:
	isSub, err := s.state.IsSubordinateApplication(c.Context(), appID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isSub, tc.IsTrue)
}

func (s *unitStateSubordinateSuite) TestIsSubordinateApplicationFalse(c *tc.C) {
	// Arrange:
	appID := s.createIAASApplication(c, "notSubordinate", life.Alive)

	// Act:
	isSub, err := s.state.IsSubordinateApplication(c.Context(), appID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isSub, tc.IsFalse)
}

func (s *unitStateSubordinateSuite) TestIsSubordinateApplicationNotFound(c *tc.C) {
	// Act:
	_, err := s.state.IsSubordinateApplication(c.Context(), "notfound")

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *unitStateSubordinateSuite) TestGetUnitPrincipal(c *tc.C) {
	principalAppID := s.createIAASApplication(c, "principal", life.Alive)
	subAppID := s.createSubordinateApplication(c, "sub", life.Alive)
	principalName := coreunittesting.GenNewName(c, "principal/0")
	subName := coreunittesting.GenNewName(c, "sub/0")
	principalUUID := s.addUnit(c, principalName, principalAppID)
	subUUID := s.addUnit(c, subName, subAppID)
	s.addUnitPrincipal(c, principalUUID, subUUID)

	foundPrincipalName, ok, err := s.state.GetUnitPrincipal(c.Context(), subName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(foundPrincipalName, tc.Equals, principalName)
	c.Check(ok, tc.IsTrue)
}

func (s *unitStateSubordinateSuite) TestGetUnitPrincipalSubordinateNotPrincipal(c *tc.C) {
	principalAppID := s.createIAASApplication(c, "principal", life.Alive)
	subAppID := s.createSubordinateApplication(c, "sub", life.Alive)
	principalName := coreunittesting.GenNewName(c, "principal/0")
	subName := coreunittesting.GenNewName(c, "sub/0")
	s.addUnit(c, principalName, principalAppID)
	s.addUnit(c, subName, subAppID)

	_, ok, err := s.state.GetUnitPrincipal(c.Context(), subName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ok, tc.IsFalse)
}

func (s *unitStateSubordinateSuite) TestGetUnitPrincipalNoUnitExists(c *tc.C) {
	subName := coreunittesting.GenNewName(c, "sub/0")

	_, ok, err := s.state.GetUnitPrincipal(c.Context(), subName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ok, tc.IsFalse)
}

func (s *unitStateSubordinateSuite) TestGetUnitSubordinates(c *tc.C) {
	principalAppID := s.createIAASApplication(c, "principal", life.Alive)
	subAppID1 := s.createSubordinateApplication(c, "sub1", life.Alive)
	subAppID2 := s.createSubordinateApplication(c, "sub2", life.Alive)
	principalName := coreunittesting.GenNewName(c, "principal/0")
	subName1 := coreunittesting.GenNewName(c, "sub1/0")
	subName2 := coreunittesting.GenNewName(c, "sub2/0")
	subName3 := coreunittesting.GenNewName(c, "sub2/1")
	principalUnitUUID := s.addUnit(c, principalName, principalAppID)
	subUnitUUID1 := s.addUnit(c, subName1, subAppID1)
	subUnitUUID2 := s.addUnit(c, subName2, subAppID2)
	subUnitUUID3 := s.addUnit(c, subName3, subAppID2)
	s.addUnitPrincipal(c, principalUnitUUID, subUnitUUID1)
	s.addUnitPrincipal(c, principalUnitUUID, subUnitUUID2)
	s.addUnitPrincipal(c, principalUnitUUID, subUnitUUID3)

	names, err := s.state.GetUnitSubordinates(c.Context(), principalName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(names, tc.SameContents, []coreunit.Name{subName1, subName2, subName3})
}

func (s *unitStateSubordinateSuite) TestGetUnitSubordinatesEmpty(c *tc.C) {
	principalAppID := s.createIAASApplication(c, "principal", life.Alive)
	principalName := coreunittesting.GenNewName(c, "principal/0")
	s.addUnit(c, principalName, principalAppID)

	names, err := s.state.GetUnitSubordinates(c.Context(), principalName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(names, tc.HasLen, 0)
}

func (s *unitStateSubordinateSuite) TestGetUnitSubordinatesNotFound(c *tc.C) {
	principalName := coreunittesting.GenNewName(c, "principal/0")

	_, err := s.state.GetUnitSubordinates(c.Context(), principalName)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSubordinateSuite) assertUnitMachinesMatch(c *tc.C, unit1, unit2 coreunit.Name) {
	m1 := s.getUnitMachine(c, unit1)
	m2 := s.getUnitMachine(c, unit2)
	c.Assert(m1, tc.Equals, m2)
}

func (s *unitStateSubordinateSuite) getUnitMachine(c *tc.C, unitName coreunit.Name) string {
	var machineName string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {

		err := tx.QueryRow(`
SELECT machine.name
FROM unit
JOIN machine ON unit.net_node_uuid = machine.net_node_uuid
WHERE unit.name = ?
`, unitName).Scan(&machineName)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return machineName
}

func (s *unitStateSubordinateSuite) addUnitPrincipal(c *tc.C, principal, sub coreunit.UUID) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO unit_principal (principal_uuid, unit_uuid)
VALUES (?, ?) 
`, principal, sub)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitStateSubordinateSuite) assertUnitPrincipal(c *tc.C, principalName, subordinateName coreunit.Name) {
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

func (s *unitStateSubordinateSuite) createSubordinateApplication(c *tc.C, name string, l life.Life) coreapplication.ID {
	state := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	appID, err := state.CreateIAASApplication(c.Context(), name, application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Charm: charm.Charm{
				Metadata: charm.Metadata{
					Name:        name,
					Subordinate: true,
				},
				Manifest:      s.minimalManifest(c),
				ReferenceName: name,
				Source:        charm.CharmHubSource,
				Revision:      42,
			},
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application SET life_id = ? WHERE name = ?", l, name)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return appID
}

func deptr[T any](v *T) T {
	var zero T
	if v == nil {
		return zero
	}
	return *v
}
