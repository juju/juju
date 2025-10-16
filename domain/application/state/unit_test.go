// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	stdtesting "testing"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/application/testing"
	coremachine "github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	internalapplication "github.com/juju/juju/domain/application/internal"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/life"
	domainmachine "github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	machinestate "github.com/juju/juju/domain/machine/state"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type unitStateSuite struct {
	baseSuite

	state *State
}

func TestUnitStateSuite(t *stdtesting.T) {
	tc.Run(t, &unitStateSuite{})
}

func (s *unitStateSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *unitStateSuite) assertContainerAddressValues(
	c *tc.C,
	unitName, providerID, addressValue, cidr string,
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
		gotCIDR       string
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `

SELECT cc.provider_id, a.address_value, a.type_id, a.origin_id,a.scope_id,a.config_type_id,s.cidr
FROM k8s_pod AS cc
JOIN unit AS u ON cc.unit_uuid = u.uuid
JOIN link_layer_device AS lld ON lld.net_node_uuid = u.net_node_uuid
JOIN ip_address AS a ON a.device_uuid = lld.uuid
JOIN subnet AS s ON a.subnet_uuid = s.uuid
WHERE u.name=?`,

			unitName).Scan(
			&gotProviderID,
			&gotValue,
			&gotType,
			&gotOrigin,
			&gotScope,
			&gotConfigType,
			&gotCIDR,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotProviderID, tc.Equals, providerID)
	c.Check(gotValue, tc.Equals, addressValue)
	c.Check(gotType, tc.Equals, int(addressType))
	c.Check(gotOrigin, tc.Equals, int(addressOrigin))
	c.Check(gotScope, tc.Equals, int(addressScope))
	c.Check(gotConfigType, tc.Equals, int(configType))
	c.Check(gotCIDR, tc.Equals, cidr)
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
		defer func() { _ = rows.Close() }()

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
	u := application.AddCAASUnitArg{
		CloudContainer: &application.CloudContainer{
			ProviderID: "some-id",
			Ports:      ptr([]string{"666", "668"}),
			Address: ptr(application.ContainerAddress{
				Device: application.ContainerDevice{
					Name:              "placeholder",
					DeviceTypeID:      domainnetwork.DeviceTypeUnknown,
					VirtualPortTypeID: domainnetwork.NonVirtualPortType,
				},
				Value:       "10.6.6.6/8",
				AddressType: ipaddress.AddressTypeIPv4,
				ConfigType:  ipaddress.ConfigTypeDHCP,
				Scope:       ipaddress.ScopeMachineLocal,
				Origin:      ipaddress.OriginHost,
			}),
		},
	}
	appUUID := s.createCAASApplication(c, "foo", life.Alive, u)

	err := s.state.UpdateCAASUnit(c.Context(), "foo/667", application.UpdateCAASUnitParams{})
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)

	unitNames, err := s.state.GetUnitNamesForApplication(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)

	cc := application.UpdateCAASUnitParams{
		ProviderID: ptr("another-id"),
		Ports:      ptr([]string{"666", "667"}),
		Address:    ptr("2001:db8::1/24"),
	}
	err = s.state.UpdateCAASUnit(c.Context(), unitNames[0], cc)
	c.Assert(err, tc.ErrorIsNil)

	var (
		providerId string
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err = tx.QueryRowContext(ctx, `

SELECT provider_id FROM k8s_pod cc
JOIN unit u ON cc.unit_uuid = u.uuid
WHERE u.name=?`,

			unitNames[0].String()).Scan(&providerId)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(providerId, tc.Equals, "another-id")

	s.assertContainerAddressValues(c, unitNames[0].String(), "another-id", "2001:db8::1/24", "::/0",
		ipaddress.AddressTypeIPv6, ipaddress.OriginProvider, ipaddress.ScopeMachineLocal, ipaddress.ConfigTypeDHCP)
	s.assertContainerPortValues(c, unitNames[0].String(), []string{"666", "667"})
}

func (s *unitStateSuite) TestUpdateCAASUnitStatuses(c *tc.C) {
	unitName, unitUUID := s.createNamedCAASUnit(c)

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
	err := s.state.UpdateCAASUnit(c.Context(), unitName, params)
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
	s.createCAASScalingApplication(c, "bar", life.Alive, 1)

	// Allow scaling.
	err := s.state.SetApplicationScalingState(c.Context(), "bar", 1, true)
	c.Assert(err, tc.ErrorIsNil)

	p := application.RegisterCAASUnitArg{
		UnitName:     "bar/0",
		PasswordHash: "passwordhash",
		ProviderID:   "some-id",
		Address:      ptr("10.6.6.6/8"),
		Ports:        ptr([]string{"0"}),
		OrderedScale: true,
		OrderedId:    0,
	}
	err = s.state.RegisterCAASUnit(c.Context(), "bar", p)
	c.Assert(err, tc.ErrorIsNil)

	s.assertCAASUnit(c, "bar/0", "passwordhash", "10.6.6.6/8", []string{"0"})
}

func (s *unitStateSuite) TestRegisterCAASUnitErrorNotScaling(c *tc.C) {
	s.createCAASScalingApplication(c, "foo", life.Alive, 1)

	p := application.RegisterCAASUnitArg{
		UnitName:     "foo/0",
		PasswordHash: "passwordhash",
		ProviderID:   "some-id",
		Address:      ptr("10.6.6.6/8"),
		Ports:        ptr([]string{"0"}),
		OrderedScale: true,
		OrderedId:    0,
	}
	err := s.state.RegisterCAASUnit(c.Context(), "foo", p)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotAssigned)

	// Assert the unit does not exist.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		name := "foo/0"
		return tx.QueryRowContext(ctx, "SELECT name FROM unit WHERE name = ?", name).Scan(&name)
	})
	c.Assert(err, tc.ErrorIs, sql.ErrNoRows)
}

func (s *unitStateSuite) TestRegisterCAASUnitErrorOutsideTargetScale(c *tc.C) {
	s.createCAASScalingApplication(c, "foo", life.Alive, 1)

	// Allow scaling.
	err := s.state.SetApplicationScalingState(c.Context(), "foo", 1, true)
	c.Assert(err, tc.ErrorIsNil)

	// Try to create a unit with a higher ordinal number than the desired scale.
	p := application.RegisterCAASUnitArg{
		UnitName:     "foo/2",
		PasswordHash: "passwordhash",
		ProviderID:   "some-id",
		Address:      ptr("10.6.6.6/8"),
		Ports:        ptr([]string{"0"}),
		OrderedScale: true,
		OrderedId:    2,
	}
	err = s.state.RegisterCAASUnit(c.Context(), "foo", p)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotAssigned)

	// Assert the unit does not exist.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		name := "foo/2"
		return tx.QueryRowContext(ctx, "SELECT name FROM unit WHERE name = ?", name).Scan(&name)
	})
	c.Assert(err, tc.ErrorIs, sql.ErrNoRows)
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
			return errors.Errorf("failed to get password hash: %v", err)
		}
		err = tx.QueryRowContext(ctx, `
SELECT address_value, type_id, scope_id, origin_id FROM ip_address ipa
JOIN link_layer_device lld ON lld.uuid = ipa.device_uuid
JOIN unit u ON u.net_node_uuid = lld.net_node_uuid WHERE u.name = ?
`, name).
			Scan(&gotAddress, &gotAddressType, &gotAddressScope, &gotAddressOrigin)
		if err != nil {
			return errors.Errorf("failed to get address value: %v", err)
		}
		rows, err := tx.QueryContext(ctx, `
SELECT port FROM k8s_pod_port ccp
JOIN k8s_pod cc ON cc.unit_uuid = ccp.unit_uuid
JOIN unit u ON u.uuid = cc.unit_uuid WHERE u.name = ?
`, name)
		if err != nil {
			return errors.Errorf("failed to get port: %v", err)
		}
		defer func() { _ = rows.Close() }()
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
	unitName, _ := s.createNamedCAASUnit(c)

	p := application.RegisterCAASUnitArg{
		UnitName:     unitName,
		PasswordHash: "passwordhash",
		ProviderID:   "some-id",
		Address:      ptr("10.6.6.6/8"),
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
	unitName, unitUUID := s.createNamedCAASUnit(c)
	s.setUnitLife(c, unitUUID, life.Dead)

	p := application.RegisterCAASUnitArg{
		UnitName:     unitName,
		PasswordHash: "passwordhash",
		ProviderID:   "foo-0",
		Address:      ptr("10.6.6.6/8"),
		Ports:        ptr([]string{"666"}),
		OrderedScale: true,
		OrderedId:    0,
	}
	err := s.state.RegisterCAASUnit(c.Context(), "foo", p)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitAlreadyExists)
}

func (s *unitStateSuite) TestRegisterCAASUnitApplicationNotAlive(c *tc.C) {
	s.createCAASApplication(c, "foo", life.Dying)
	p := application.RegisterCAASUnitArg{
		UnitName:     "foo/0",
		PasswordHash: "passwordhash",
		ProviderID:   "foo-0",
		Address:      ptr("10.6.6.6/8"),
		Ports:        ptr([]string{"666"}),
		OrderedScale: true,
		OrderedId:    0,
	}

	err := s.state.RegisterCAASUnit(c.Context(), "foo", p)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotAlive)
}

func (s *unitStateSuite) TestRegisterCAASUnitExceedsScale(c *tc.C) {
	appUUID, _ := s.createCAASApplicationWithNUnits(c, "foo", life.Alive, 1)

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
		Address:      ptr("10.6.6.6/0"),
		Ports:        ptr([]string{"666"}),
		OrderedScale: true,
		OrderedId:    2,
	}

	err = s.state.RegisterCAASUnit(c.Context(), "foo", p)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotAssigned)
}

func (s *unitStateSuite) TestRegisterCAASUnitExceedsScaleWhileScalingWithoutError(c *tc.C) {
	appUUID, _ := s.createCAASApplicationWithNUnits(c, "foo", life.Alive, 1)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE application_scale
SET scaling = ?, scale = ?, scale_target = ?
WHERE application_uuid = ?`, true, 1, 3, appUUID)
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
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitStateSuite) TestRegisterCAASUnitExceedsScaleTarget(c *tc.C) {
	appUUID, _ := s.createCAASApplicationWithNUnits(c, "foo", life.Alive, 1)

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
		Address:      ptr("10.6.6.6/8"),
		Ports:        ptr([]string{"666"}),
		OrderedScale: true,
		OrderedId:    2,
	}

	err = s.state.RegisterCAASUnit(c.Context(), "foo", p)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotAssigned)
}

func (s *unitStateSuite) TestGetUnitLife(c *tc.C) {
	unitName, _ := s.createNamedIAASUnit(c)

	unitLife, err := s.state.GetUnitLife(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(unitLife, tc.Equals, life.Alive)
}

func (s *unitStateSuite) TestGetUnitLifeNotFound(c *tc.C) {
	_, err := s.state.GetUnitLife(c.Context(), "foo/666")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestGetUnitUUIDByName(c *tc.C) {
	unitName, unitUUID := s.createNamedIAASUnit(c)
	gotUUID, err := s.state.GetUnitUUIDByName(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotUUID, tc.Equals, unitUUID)
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
	_, _, err := s.state.AddIAASUnits(c.Context(), uuid, application.AddIAASUnitArg{})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *unitStateSuite) TestAddUnitsApplicationNotAlive(c *tc.C) {
	appID := s.createIAASApplication(c, "foo", life.Dying)

	_, _, err := s.state.AddIAASUnits(c.Context(), appID, application.AddIAASUnitArg{})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotAlive)
}

func (s *unitStateSuite) TestAddIAASUnits(c *tc.C) {
	appID := s.createIAASApplication(c, "foo", life.Alive)

	now := ptr(time.Now())
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	u := application.AddIAASUnitArg{
		MachineNetNodeUUID: netNodeUUID,
		MachineUUID:        tc.Must(c, coremachine.NewUUID),
		AddUnitArg: application.AddUnitArg{
			NetNodeUUID: netNodeUUID,
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
		},
	}

	unitNames, machineNames, err := s.state.AddIAASUnits(c.Context(), appID, u)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitNames, tc.HasLen, 1)
	unitName := unitNames[0]
	c.Check(unitName, tc.Equals, coreunit.Name("foo/0"))
	c.Assert(machineNames, tc.HasLen, 1)
	machineName := machineNames[0]
	c.Check(machineName, tc.Equals, coremachine.Name("0"))

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
		int(u.AgentStatus.Status), u.AgentStatus.Message,
		u.AgentStatus.Since, u.AgentStatus.Data)
	s.assertUnitStatus(
		c, "unit_workload", coreunit.UUID(unitUUID),
		int(u.WorkloadStatus.Status), u.WorkloadStatus.Message,
		u.WorkloadStatus.Since, u.WorkloadStatus.Data)
}

func (s *unitStateSuite) TestAddCAASUnits(c *tc.C) {
	appID := s.createIAASApplication(c, "foo", life.Alive)

	now := ptr(time.Now())
	u := application.AddCAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			NetNodeUUID: tc.Must(c, domainnetwork.NewNetNodeUUID),
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
		},
	}

	unitNames, err := s.state.AddCAASUnits(c.Context(), appID, u)
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
		int(u.AgentStatus.Status), u.AgentStatus.Message,
		u.AgentStatus.Since, u.AgentStatus.Data)
	s.assertUnitStatus(
		c, "unit_workload", coreunit.UUID(unitUUID),
		int(u.WorkloadStatus.Status), u.WorkloadStatus.Message,
		u.WorkloadStatus.Since, u.WorkloadStatus.Data)
}

func (s *unitStateSuite) TestAddIAASUnitsToSyntheticCMRApplication(c *tc.C) {
	appID := s.createIAASApplication(c, "foo", life.Alive)

	// Switch the source_id of a charm to a synthetic CMR charm.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE charm SET source_id = 2, architecture_id = NULL WHERE uuid = (
SELECT charm_uuid FROM application WHERE uuid = ?
)`, appID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	now := ptr(time.Now())
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	u := application.AddIAASUnitArg{
		MachineNetNodeUUID: netNodeUUID,
		MachineUUID:        tc.Must(c, coremachine.NewUUID),
		AddUnitArg: application.AddUnitArg{
			NetNodeUUID: netNodeUUID,
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
		},
	}

	_, _, err = s.state.AddIAASUnits(c.Context(), appID, u)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *unitStateSuite) TestAddCAASUnitsToSyntheticCMRApplication(c *tc.C) {
	appID := s.createIAASApplication(c, "foo", life.Alive)

	// Switch the source_id of a charm to a synthetic CMR charm.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE charm SET source_id = 2, architecture_id = NULL WHERE uuid = (
SELECT charm_uuid FROM application WHERE uuid = ?
)`, appID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	now := ptr(time.Now())
	u := application.AddCAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			NetNodeUUID: tc.Must(c, domainnetwork.NewNetNodeUUID),
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
		},
	}

	_, err = s.state.AddCAASUnits(c.Context(), appID, u)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *unitStateSuite) TestInitialWatchStatementUnitLife(c *tc.C) {
	_, unitUUIDs := s.createIAASApplicationWithNUnits(c, "foo", life.Alive, 2)

	table, queryFunc := s.state.InitialWatchStatementUnitLife("foo")
	c.Assert(table, tc.Equals, "unit")

	result, err := queryFunc(c.Context(), s.TxnRunner())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []string{
		unitUUIDs[0].String(),
		unitUUIDs[1].String(),
	})
}

func (s *unitStateSuite) TestUpdateUnitCharmUnitNotFound(c *tc.C) {
	err := s.state.UpdateUnitCharm(c.Context(), "foo/666", "bar")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestUpdateUnitCharmUnitIsDead(c *tc.C) {
	unitName, unitUUID := s.createNamedIAASUnit(c)
	s.setUnitLife(c, unitUUID, life.Dead)

	err := s.state.UpdateUnitCharm(c.Context(), unitName, "bar")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitIsDead)
}

func (s *unitStateSuite) TestUpdateUnitCharmNoCharm(c *tc.C) {
	unitName, _ := s.createNamedIAASUnit(c)

	err := s.state.UpdateUnitCharm(c.Context(), unitName, "bar")
	c.Assert(err, tc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *unitStateSuite) TestUpdateUnitCharm(c *tc.C) {
	unitName, _ := s.createNamedIAASUnit(c)

	id, _, err := s.state.AddCharm(c.Context(), charm.Charm{
		Metadata: charm.Metadata{
			Name: "bar",
		},
		Manifest:      s.minimalManifest(c),
		Source:        charm.LocalSource,
		Revision:      42,
		ReferenceName: "ubuntu",
		Hash:          "hash",
		ArchivePath:   "archive",
		Version:       "deadbeef",
	}, nil, false)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.UpdateUnitCharm(c.Context(), unitName, id)
	c.Assert(err, tc.ErrorIsNil)

	var gotUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT charm_uuid FROM unit WHERE name=?", unitName).Scan(&gotUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotUUID, tc.Equals, id.String())
}

func (s *unitStateSuite) TestGetUnitRefreshAttributes(c *tc.C) {
	s.createSubnetForCAASModel(c)
	unitName, _ := s.createNamedIAASUnit(c)

	cc := application.UpdateCAASUnitParams{
		ProviderID: ptr("another-id"),
		Ports:      ptr([]string{"666", "667"}),
		Address:    ptr("2001:db8::1/8"),
	}
	err := s.state.UpdateCAASUnit(c.Context(), unitName, cc)
	c.Assert(err, tc.ErrorIsNil)

	refreshAttributes, err := s.state.GetUnitRefreshAttributes(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(refreshAttributes, tc.DeepEquals, application.UnitAttributes{
		Life:        life.Alive,
		ProviderID:  "another-id",
		ResolveMode: "none",
	})
}

func (s *unitStateSuite) TestGetUnitRefreshAttributesNoProviderID(c *tc.C) {
	unitName, _ := s.createNamedIAASUnit(c)

	refreshAttributes, err := s.state.GetUnitRefreshAttributes(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(refreshAttributes, tc.DeepEquals, application.UnitAttributes{
		Life:        life.Alive,
		ResolveMode: "none",
	})
}

func (s *unitStateSuite) TestGetUnitRefreshAttributesWithResolveMode(c *tc.C) {
	unitName, unitUUID := s.createNamedIAASUnit(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO unit_resolved (unit_uuid, mode_id) VALUES (?, 0)", unitUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	refreshAttributes, err := s.state.GetUnitRefreshAttributes(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(refreshAttributes, tc.DeepEquals, application.UnitAttributes{
		Life:        life.Alive,
		ResolveMode: "retry-hooks",
	})
}

func (s *unitStateSuite) TestGetUnitRefreshAttributesDeadLife(c *tc.C) {
	unitName, _ := s.createNamedIAASUnit(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 2 WHERE name = ?", unitName.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	refreshAttributes, err := s.state.GetUnitRefreshAttributes(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(refreshAttributes, tc.DeepEquals, application.UnitAttributes{
		Life:        life.Dead,
		ResolveMode: "none",
	})
}

func (s *unitStateSuite) TestGetUnitRefreshAttributesDyingLife(c *tc.C) {
	unitName, _ := s.createNamedIAASUnit(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 1 WHERE name = ?", unitName.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	refreshAttributes, err := s.state.GetUnitRefreshAttributes(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(refreshAttributes, tc.DeepEquals, application.UnitAttributes{
		Life:        life.Dying,
		ResolveMode: "none",
	})
}

func (s *unitStateSuite) TestGetAllUnitNamesNoUnits(c *tc.C) {
	names, err := s.state.GetAllUnitNames(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(names, tc.DeepEquals, []coreunit.Name{})
}

func (s *unitStateSuite) TestGetAllUnitNames(c *tc.C) {
	_, fooUnitUUIDs := s.createIAASApplicationWithNUnits(c, "foo", life.Alive, 2)
	_, barUnitUUIDs := s.createIAASApplicationWithNUnits(c, "bar", life.Alive, 1)

	names := make([]coreunit.Name, 0, len(fooUnitUUIDs)+len(barUnitUUIDs))
	for _, uuid := range append(fooUnitUUIDs, barUnitUUIDs...) {
		n, err := s.state.GetUnitNameForUUID(c.Context(), uuid)
		c.Assert(err, tc.ErrorIsNil)
		names = append(names, n)
	}

	got, err := s.state.GetAllUnitNames(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.SameContents, names)
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
	appUUID, fooUnitUUIDs := s.createIAASApplicationWithNUnits(c, "foo", life.Alive, 3)

	names := make([]coreunit.Name, 0, len(fooUnitUUIDs))
	for _, uuid := range fooUnitUUIDs {
		n, err := s.state.GetUnitNameForUUID(c.Context(), uuid)
		c.Assert(err, tc.ErrorIsNil)
		names = append(names, n)
	}

	gotNames, err := s.state.GetUnitNamesForApplication(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotNames, tc.SameContents, names)
}

func (s *unitStateSuite) TestGetUnitNamesForNetNodeNotFound(c *tc.C) {
	_, err := s.state.GetUnitNamesForNetNode(c.Context(), "doink")
	c.Assert(err, tc.ErrorIs, applicationerrors.NetNodeNotFound)
}

func (s *unitStateSuite) TestGetUnitNamesForNetNodeNoUnits(c *tc.C) {
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		placeMachineArgs := domainmachine.PlaceMachineArgs{
			Directive: deployment.Placement{
				Type: deployment.PlacementTypeUnset,
			},
			MachineUUID: machinetesting.GenUUID(c),
			NetNodeUUID: netNodeUUID,
		}
		_, err = machinestate.PlaceMachine(ctx, tx, s.state, clock.WallClock, placeMachineArgs)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	names, err := s.state.GetUnitNamesForNetNode(c.Context(), netNodeUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(names, tc.DeepEquals, []coreunit.Name{})
}

func (s *unitStateSuite) TestGetUnitNamesForNetNode(c *tc.C) {
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	altNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	s.createIAASApplication(c, "foo", life.Alive,
		application.AddIAASUnitArg{
			MachineUUID:        machineUUID,
			MachineNetNodeUUID: netNodeUUID,
			AddUnitArg: application.AddUnitArg{
				NetNodeUUID: netNodeUUID,
				Placement: deployment.Placement{
					Directive: "0",
				},
			},
		},
		application.AddIAASUnitArg{
			MachineUUID:        machineUUID,
			MachineNetNodeUUID: netNodeUUID,
			AddUnitArg: application.AddUnitArg{
				NetNodeUUID: netNodeUUID,
				Placement: deployment.Placement{
					Type:      deployment.PlacementTypeMachine,
					Directive: "0",
				},
			},
		},
		application.AddIAASUnitArg{
			MachineUUID:        machinetesting.GenUUID(c),
			MachineNetNodeUUID: altNetNodeUUID,
			AddUnitArg: application.AddUnitArg{
				NetNodeUUID: altNetNodeUUID,
				Placement: deployment.Placement{
					Directive: "1",
				},
			},
		})

	names, err := s.state.GetUnitNamesForNetNode(c.Context(), netNodeUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(names, tc.DeepEquals, []coreunit.Name{"foo/0", "foo/1"})
}

func (s *unitStateSuite) TestGetUnitWorkloadVersion(c *tc.C) {
	unitName, _ := s.createNamedIAASUnit(c)

	workloadVersion, err := s.state.GetUnitWorkloadVersion(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(workloadVersion, tc.Equals, "")
}

func (s *unitStateSuite) TestGetUnitWorkloadVersionNotFound(c *tc.C) {
	_, err := s.state.GetUnitWorkloadVersion(c.Context(), coreunit.Name("foo/666"))
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestSetUnitWorkloadVersion(c *tc.C) {
	unitName, _ := s.createNamedIAASUnit(c)

	err := s.state.SetUnitWorkloadVersion(c.Context(), unitName, "v1.0.0")
	c.Assert(err, tc.ErrorIsNil)

	workloadVersion, err := s.state.GetUnitWorkloadVersion(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(workloadVersion, tc.Equals, "v1.0.0")
}

func (s *unitStateSuite) TestSetUnitWorkloadVersionMultiple(c *tc.C) {
	appID, unitUUIDs := s.createIAASApplicationWithNUnits(c, "foo", life.Alive, 2)
	unitNames := make([]coreunit.Name, 0, len(unitUUIDs))
	for _, uuid := range unitUUIDs {
		n, err := s.state.GetUnitNameForUUID(c.Context(), uuid)
		c.Assert(err, tc.ErrorIsNil)
		unitNames = append(unitNames, n)
	}

	s.assertApplicationWorkloadVersion(c, appID, "")

	err := s.state.SetUnitWorkloadVersion(c.Context(), unitNames[0], "v1.0.0")
	c.Assert(err, tc.ErrorIsNil)

	s.assertApplicationWorkloadVersion(c, appID, "v1.0.0")

	err = s.state.SetUnitWorkloadVersion(c.Context(), unitNames[1], "v2.0.0")
	c.Assert(err, tc.ErrorIsNil)

	s.assertApplicationWorkloadVersion(c, appID, "v2.0.0")

	workloadVersion, err := s.state.GetUnitWorkloadVersion(c.Context(), unitNames[0])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(workloadVersion, tc.Equals, "v1.0.0")

	workloadVersion, err = s.state.GetUnitWorkloadVersion(c.Context(), unitNames[1])
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

func (s *unitStateSuite) assertApplicationWorkloadVersion(c *tc.C, appID coreapplication.UUID, expected string) {
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

// TestGetUnitsK8sPodInfoNoK8sUnits ensures that if there are no Kubernetes
// units in the model that a nil error is returned with an empty result.
func (s *unitStateSuite) TestGetUnitsK8sPodInfoNoK8sUnits(c *tc.C) {
	s.createIAASApplicationWithNUnits(c, "iaas-app", life.Alive, 2)
	infos, err := s.state.GetUnitsK8sPodInfo(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(infos, tc.HasLen, 0)
}

func (s *unitStateSuite) TestGetUnitsK8sPodInfo(c *tc.C) {
	// Arrange: 2 applications with 1 unit each, and a third application with a dead unit.
	app1UUID := s.createCAASApplication(c, "foo", life.Alive, application.AddCAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			NetNodeUUID: tc.Must(c, domainnetwork.NewNetNodeUUID),
		},
		CloudContainer: &application.CloudContainer{
			ProviderID: "foo-id",
			Ports:      ptr([]string{"666", "668"}),
			Address: ptr(application.ContainerAddress{
				Value: "10.6.6.6/24",
			}),
		},
	})
	uuids, err := s.state.getApplicationUnits(c.Context(), app1UUID)
	c.Assert(err, tc.ErrorIsNil)
	app1Unit1UUID := uuids[0]

	app2UUID := s.createCAASApplication(c, "bar", life.Alive, application.AddCAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			NetNodeUUID: tc.Must(c, domainnetwork.NewNetNodeUUID),
		},
		CloudContainer: &application.CloudContainer{
			ProviderID: "bar-id",
			Ports:      ptr([]string{"777"}),
			Address: ptr(application.ContainerAddress{
				Value: "2001:0DB8::BEEF:FACE/128",
			}),
		},
	})
	uuids, err = s.state.getApplicationUnits(c.Context(), app2UUID)
	c.Assert(err, tc.ErrorIsNil)
	app2Unit1UUID := uuids[0]

	app3UUID := s.createCAASApplication(c, "zoo", life.Alive, application.AddCAASUnitArg{
		CloudContainer: &application.CloudContainer{
			ProviderID: "zoo-id",
			Ports:      ptr([]string{"666", "668"}),
			Address: ptr(application.ContainerAddress{
				Value: "10.6.6.8/24",
			}),
		},
	})
	uuids, err = s.state.getApplicationUnits(c.Context(), app3UUID)
	c.Assert(err, tc.ErrorIsNil)
	app3Unit1UUID := uuids[0]
	// Set the unit for the third app to Dead, to verify it is not returned.
	s.setUnitLife(c, app3Unit1UUID, life.Dead)

	// Act:
	k8sPodInfo, err := s.state.GetUnitsK8sPodInfo(c.Context())

	// Assert: only the 2 alive units are returned.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(k8sPodInfo, tc.DeepEquals, map[string]internalapplication.UnitK8sInformation{
		app1Unit1UUID.String(): {
			Addresses: []string{
				"10.6.6.6/24",
			},
			Ports:      []string{"666", "668"},
			ProviderID: "foo-id",
			UnitName:   "foo/0",
		},
		app2Unit1UUID.String(): {
			Addresses: []string{
				"2001:0DB8::BEEF:FACE/128",
			},
			Ports:      []string{"777"},
			ProviderID: "bar-id",
			UnitName:   "bar/0",
		},
	})
}

func (s *unitStateSuite) TestGetUnitK8sPodInfo(c *tc.C) {
	// Arrange:
	appUUID := s.createCAASApplication(c, "foo", life.Alive, application.AddCAASUnitArg{
		CloudContainer: &application.CloudContainer{
			ProviderID: "some-id",
			Ports:      ptr([]string{"666", "668"}),
			Address: ptr(application.ContainerAddress{
				Device: application.ContainerDevice{
					Name:              "placeholder",
					DeviceTypeID:      domainnetwork.DeviceTypeUnknown,
					VirtualPortTypeID: domainnetwork.NonVirtualPortType,
				},
				Value:       "10.6.6.6/24",
				AddressType: ipaddress.AddressTypeIPv4,
				ConfigType:  ipaddress.ConfigTypeDHCP,
				Scope:       ipaddress.ScopeMachineLocal,
				Origin:      ipaddress.OriginHost,
			}),
		},
	})
	uuids, err := s.state.getApplicationUnits(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	unitName, err := s.state.GetUnitNameForUUID(c.Context(), uuids[0])
	c.Assert(err, tc.ErrorIsNil)

	// Act:
	info, err := s.state.GetUnitK8sPodInfo(c.Context(), unitName)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.ProviderID, tc.Equals, "some-id")
	c.Check(info.Address, tc.Equals, "10.6.6.6/24")
	c.Check(info.Ports, tc.DeepEquals, []string{"666", "668"})
}

func (s *unitStateSuite) TestGetUnitK8sPodInfoNoInfo(c *tc.C) {
	// Arrange:
	unitName, _ := s.createNamedCAASUnit(c)

	// Act:
	info, err := s.state.GetUnitK8sPodInfo(c.Context(), unitName)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.ProviderID, tc.Equals, "")
	c.Check(info.Address, tc.Equals, "")
	c.Check(info.Ports, tc.DeepEquals, []string{})
}

func (s *unitStateSuite) TestGetUnitK8sPodInfoNotFound(c *tc.C) {
	_, err := s.state.GetUnitK8sPodInfo(c.Context(), coreunit.Name("foo/666"))
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestGetUnitK8sPodInfoDead(c *tc.C) {
	unitName, unitUUID := s.createNamedCAASUnit(c)
	s.setUnitLife(c, unitUUID, life.Dead)

	_, err := s.state.GetUnitK8sPodInfo(c.Context(), unitName)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitIsDead)
}

func (s *unitStateSuite) TestGetUnitNetNodesNotFound(c *tc.C) {
	_, err := s.state.GetUnitNetNodesByName(c.Context(), "unknown-unit")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestGetUnitNetNodesK8s(c *tc.C) {
	appUUID, unitUUIDS := s.createIAASApplicationWithNUnits(c, "foo", life.Alive, 1)
	unitName, err := s.state.GetUnitNameForUUID(c.Context(), unitUUIDS[0])
	c.Assert(err, tc.ErrorIsNil)

	unitNetNodeUUID := "unit-node-uuid"
	serviceNetNodeUUID := "service-node-uuid"

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
		_, err = tx.ExecContext(ctx, insertSvc, "svc-uuid", serviceNetNodeUUID, appUUID.String(), "provider-id")
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	netNodeUUID, err := s.state.GetUnitNetNodesByName(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(netNodeUUID, tc.SameContents, []string{serviceNetNodeUUID, unitNetNodeUUID})
}

func (s *unitStateSuite) TestGetUnitNetNodesMachine(c *tc.C) {
	unitName, _ := s.createNamedIAASUnit(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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

	netNodeUUID, err := s.state.GetUnitNetNodesByName(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(netNodeUUID, tc.SameContents, []string{"machine-net-node-uuid"})
}

// TestGetMachineUUIDAndNetNodeForNonExistentMachineName tests that if no
// machine exists by the supplied name then the caller gets back a
// [machineerrors.MachineNotFound] error.
func (s *unitStateSuite) TestGetMachineUUIDAndNetNodeForNonExistentMachineName(c *tc.C) {
	_, _, err := s.state.GetMachineUUIDAndNetNodeForName(c.Context(), "no-exist")
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetMachineUUIDAndNetNodeForName checks that the correct machine uuid and
// net node is returned for a machine matching the supplied name.
func (s *unitStateSuite) TestGetMachineUUIDAndNetNodeForName(c *tc.C) {
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)

	err := s.TxnRunner().StdTxn(
		c.Context(),
		func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(
				ctx, "INSERT INTO net_node (UUID) VALUES (?)", netNodeUUID.String(),
			)
			if err != nil {
				return err
			}

			_, err = tx.ExecContext(
				ctx,
				`
INSERT INTO machine (uuid, name, net_node_uuid, life_id)
VALUES (?, ?, ?, 1)
			`,
				machineUUID.String(), "10", netNodeUUID.String(),
			)
			return err
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	gotMachineUUID, gotNetNodeUUID, err := s.state.GetMachineUUIDAndNetNodeForName(c.Context(), "10")
	c.Check(err, tc.ErrorIsNil)
	c.Check(gotMachineUUID, tc.Equals, machineUUID)
	c.Check(gotNetNodeUUID, tc.Equals, netNodeUUID)
}

func (s *unitStateSuite) GetAllUnitCloudContainerIDsForApplication(c *tc.C) {
	appID := s.createCAASApplication(c, "foo", life.Alive, application.AddCAASUnitArg{
		CloudContainer: &application.CloudContainer{
			ProviderID: "a",
		},
	}, application.AddCAASUnitArg{
		CloudContainer: &application.CloudContainer{
			ProviderID: "b",
		},
	})

	_ = s.createCAASApplication(c, "bar", life.Alive, application.AddCAASUnitArg{
		CloudContainer: &application.CloudContainer{
			ProviderID: "c",
		},
	})

	result, err := s.state.GetAllUnitCloudContainerIDsForApplication(c.Context(), appID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, map[coreunit.Name]string{
		"foo/0": "a",
		"foo/1": "b",
	})
}

// TestGetUnitMachineUUIDandNetNodeUnitNotFound wants to see that when a caller
// calls [State.getUnitMachineIdentifiers] with a unit uuid that does not
// exist in the model the caller gets back an error satisfying
// [applicationerrors.UnitNotFound].
func (s *unitStateSuite) TestGetUnitMachineIdentifiersUnitNotFound(c *tc.C) {
	unitUUID := coreunittesting.GenUnitUUID(c)

	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := s.state.getUnitMachineIdentifiers(
			ctx, tx, unitUUID,
		)
		return err
	})
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestGetUnitMachineUUIDandNetNodeUnit is a happy path test for
// [State.getUnitMachineIdentifiers].
func (s *unitStateSuite) TestGetUnitMachineIdentifiers(c *tc.C) {
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	appUUID := s.createIAASApplication(c, "myapp", life.Alive, application.AddIAASUnitArg{
		MachineNetNodeUUID: netNodeUUID,
		MachineUUID:        machineUUID,
		AddUnitArg: application.AddUnitArg{
			NetNodeUUID: netNodeUUID,
		},
	})

	unitUUIDs, err := s.state.getApplicationUnits(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitUUIDs, tc.HasLen, 1)

	var recievedIdentifiers internalapplication.MachineIdentifiers
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		recievedIdentifiers, err = s.state.getUnitMachineIdentifiers(
			ctx, tx, unitUUIDs[0],
		)
		return err
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(recievedIdentifiers, tc.Equals, internalapplication.MachineIdentifiers{
		Name:        coremachine.Name("0"),
		NetNodeUUID: netNodeUUID,
		UUID:        machineUUID,
	})
}

// TestGetUnitUUIDAndNetNodeForNameNotFound tests that asking for the uuid and
// netnode for a unit name that does not exist in the model results in a
// [applicationerrors.UnitNotFound] error.
func (s *unitStateSuite) TestGetUnitUUIDAndNetNodeForNameNotFound(c *tc.C) {
	_, _, err := s.state.GetUnitUUIDAndNetNodeForName(
		c.Context(), coreunit.Name("does-not-exist"),
	)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestGetUnitUUIDAndNetNodeForName tests the happy path of getting a units uuid
// and net node by name.
func (s *unitStateSuite) TestGetUnitUUIDAndNetNodeForName(c *tc.C) {
	unitName, unitUUID := s.createNamedCAASUnit(c)

	gotUnitUUID, gotNetNodeUUID, err := s.state.GetUnitUUIDAndNetNodeForName(
		c.Context(), unitName,
	)

	c.Check(err, tc.ErrorIsNil)
	c.Check(gotUnitUUID, tc.Equals, unitUUID)
	c.Check(gotNetNodeUUID, tc.IsNonZeroUUID)
}

// TestCheckCAASUnitNotRegistered tests that when provided with a unit name that
// doesn't exist in the model that the caller gets back 'false' and no error.
func (s *unitStateSuite) TestCheckCAASUnitNotRegistered(c *tc.C) {
	unitName := coreunit.Name("foo/0")
	isRegistered, _, _, err := s.state.GetCAASUnitRegistered(
		c.Context(), unitName,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(isRegistered, tc.Equals, false)
}

// TestCheckCAASUnitRegistered tests that when provided with a unit name that
// exists in the model is registered returns true with the correct uuid and
// net node.
func (s *unitStateSuite) TestCheckCAASUnitRegistered(c *tc.C) {
	unitName, unitUUID := s.createNamedCAASUnit(c)
	isRegistered, gotUUID, gotNetNodeUUID, err := s.state.
		GetCAASUnitRegistered(c.Context(), unitName)
	c.Check(err, tc.ErrorIsNil)
	c.Check(isRegistered, tc.IsTrue)
	c.Check(gotUUID, tc.Equals, unitUUID)
	c.Check(gotNetNodeUUID, tc.IsNonZeroUUID)
}

type unitStateSubordinateSuite struct {
	baseSuite

	state *State
}

func TestUnitStateSubordinateSuite(t *stdtesting.T) {
	tc.Run(t, &unitStateSubordinateSuite{})
}

func (s *unitStateSubordinateSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *unitStateSubordinateSuite) createPrincipalUnit(
	c *tc.C,
) (coreunit.UUID, domainnetwork.NetNodeUUID) {
	uuid, netNodeUUID := s.createNPrincipalUnits(c, 1)
	return uuid[0], netNodeUUID[0]
}

func (s *unitStateSubordinateSuite) createNPrincipalUnits(
	c *tc.C, n int,
) ([]coreunit.UUID, []domainnetwork.NetNodeUUID) {
	netNodeUUIDs := make([]domainnetwork.NetNodeUUID, 0, n)
	args := make([]application.AddIAASUnitArg, 0, n)

	for range n {
		netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
		args = append(args, application.AddIAASUnitArg{
			AddUnitArg: application.AddUnitArg{
				NetNodeUUID: netNodeUUID,
			},
			MachineNetNodeUUID: netNodeUUID,
			MachineUUID:        machinetesting.GenUUID(c),
		})
		netNodeUUIDs = append(netNodeUUIDs, netNodeUUID)
	}

	appUUID := s.createIAASApplication(c, "principal", life.Alive, args...)

	unitUUIDs, err := s.state.getApplicationUnits(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitUUIDs, tc.HasLen, n)

	return unitUUIDs, netNodeUUIDs
}

func (s *unitStateSubordinateSuite) TestAddIAASSubordinateUnit(c *tc.C) {
	// Arrange:
	pUnitUUID, netNodeUUID := s.createPrincipalUnit(c)

	sAppID := s.createSubordinateApplication(c, "subordinate", life.Alive)

	// Act:
	sUnitName, machineNames, err := s.state.AddIAASSubordinateUnit(c.Context(), application.SubordinateUnitArg{
		NetNodeUUID:       netNodeUUID,
		SubordinateAppID:  sAppID,
		PrincipalUnitUUID: pUnitUUID,
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sUnitName, tc.Equals, coreunittesting.GenNewName(c, "subordinate/0"))

	sUnitUUID, err := s.state.GetUnitUUIDByName(c.Context(), sUnitName)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUnitPrincipal(c, pUnitUUID, sUnitName)
	s.assertUnitMachinesMatch(c, pUnitUUID, sUnitUUID)

	c.Assert(machineNames, tc.HasLen, 1)
	c.Check(machineNames[0], tc.Equals, coremachine.Name("0"))
}

// TestAddIAASSubordinateUnitSecondSubordinate tests that a second subordinate unit
// can be added to an app with no issues.
func (s *unitStateSubordinateSuite) TestAddIAASSubordinateUnitSecondSubordinate(c *tc.C) {
	// Arrange: add subordinate application.
	sAppID := s.createSubordinateApplication(c, "subordinate", life.Alive)
	principalUUIDs, netNodeUUIDs := s.createNPrincipalUnits(c, 2)

	_, _, err := s.state.AddIAASSubordinateUnit(c.Context(), application.SubordinateUnitArg{
		NetNodeUUID:       netNodeUUIDs[0],
		SubordinateAppID:  sAppID,
		PrincipalUnitUUID: principalUUIDs[0],
	})
	c.Assert(err, tc.ErrorIsNil)

	// Act: Add a second subordinate unit
	sUnitName2, machineNames, err := s.state.AddIAASSubordinateUnit(c.Context(), application.SubordinateUnitArg{
		NetNodeUUID:       netNodeUUIDs[1],
		SubordinateAppID:  sAppID,
		PrincipalUnitUUID: principalUUIDs[1],
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sUnitName2, tc.Equals, coreunittesting.GenNewName(c, "subordinate/1"))

	sUnitUUID2, err := s.state.GetUnitUUIDByName(c.Context(), sUnitName2)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUnitPrincipal(c, principalUUIDs[1], sUnitName2)
	s.assertUnitMachinesMatch(c, principalUUIDs[1], sUnitUUID2)

	c.Assert(machineNames, tc.HasLen, 1)
	c.Check(machineNames[0], tc.Equals, coremachine.Name("1"))
}

func (s *unitStateSubordinateSuite) TestAddIAASSubordinateUnitTwiceToSameUnit(c *tc.C) {
	// Arrange:
	pUnitUUID, netNodeUUID := s.createPrincipalUnit(c)

	sAppID := s.createSubordinateApplication(c, "subordinate", life.Alive)

	// Arrange: Add the first subordinate.
	_, _, err := s.state.AddIAASSubordinateUnit(c.Context(), application.SubordinateUnitArg{
		NetNodeUUID:       netNodeUUID,
		SubordinateAppID:  sAppID,
		PrincipalUnitUUID: pUnitUUID,
	})
	c.Assert(err, tc.ErrorIsNil)

	// Act: try adding a second subordinate to the same unit.
	_, _, err = s.state.AddIAASSubordinateUnit(c.Context(), application.SubordinateUnitArg{
		NetNodeUUID:       netNodeUUID,
		SubordinateAppID:  sAppID,
		PrincipalUnitUUID: pUnitUUID,
	})

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitAlreadyHasSubordinate)
}

func (s *unitStateSubordinateSuite) TestAddIAASSubordinateUnitWithoutMachine(c *tc.C) {
	// Arrange:
	pUnitName := coreunittesting.GenNewName(c, "foo/666")
	pAppUUID := s.createIAASApplication(c, "principal", life.Alive)
	pUnitUUID := s.addUnit(c, pUnitName, pAppUUID)

	sAppID := s.createSubordinateApplication(c, "subordinate", life.Alive)

	// Act:
	_, _, err := s.state.AddIAASSubordinateUnit(c.Context(), application.SubordinateUnitArg{
		SubordinateAppID:  sAppID,
		PrincipalUnitUUID: pUnitUUID,
	})

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitMachineNotAssigned)
}

func (s *unitStateSubordinateSuite) TestAddIAASSubordinateUnitApplicationNotAlive(c *tc.C) {
	// Arrange:§
	pUnitUUID := coreunittesting.GenUnitUUID(c)

	sAppID := s.createSubordinateApplication(c, "subordinate", life.Dying)

	// Act:
	_, _, err := s.state.AddIAASSubordinateUnit(c.Context(), application.SubordinateUnitArg{
		SubordinateAppID:  sAppID,
		PrincipalUnitUUID: pUnitUUID,
	})

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotAlive)
}

func (s *unitStateSubordinateSuite) TestAddIAASSubordinateUnitPrincipalNotFound(c *tc.C) {
	// Arrange:
	pUnitUUID := coreunittesting.GenUnitUUID(c)

	sAppID := s.createSubordinateApplication(c, "subordinate", life.Alive)

	// Act:
	_, _, err := s.state.AddIAASSubordinateUnit(c.Context(), application.SubordinateUnitArg{
		SubordinateAppID:  sAppID,
		PrincipalUnitUUID: pUnitUUID,
	})

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
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

func (s *unitStateSubordinateSuite) assertUnitMachinesMatch(c *tc.C, unit1, unit2 coreunit.UUID) {
	m1 := s.getUnitMachine(c, unit1)
	m2 := s.getUnitMachine(c, unit2)
	c.Assert(m1, tc.Equals, m2)
}

func (s *unitStateSubordinateSuite) getUnitMachine(c *tc.C, unitUUID coreunit.UUID) string {
	var machineUUID string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {

		err := tx.QueryRow(`
SELECT machine.uuid
FROM unit
JOIN machine ON unit.net_node_uuid = machine.net_node_uuid
WHERE unit.uuid = ?
`, unitUUID).Scan(&machineUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return machineUUID
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

func (s *unitStateSubordinateSuite) createSubordinateApplication(c *tc.C, name string, l life.Life) coreapplication.UUID {
	state := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	appID, machineNames, err := state.CreateIAASApplication(c.Context(), name, application.AddIAASApplicationArg{
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
	c.Check(machineNames, tc.HasLen, 0)

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
