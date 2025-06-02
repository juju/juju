// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
)

type statusSuite struct {
	schematesting.ModelSuite
}

func TestStatusSuite(t *testing.T) {
	tc.Run(t, &statusSuite{})
}

// TestK8sPodStatusDBValues ensures there's no skew between what's in the
// database table for cloud container status and the typed consts used in the
// state packages.
func (s *statusSuite) TestK8sPodStatusDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, status FROM k8s_pod_status_value")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[K8sPodStatusType]string)
	for rows.Next() {
		var (
			id   int
			name string
		)
		err := rows.Scan(&id, &name)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[K8sPodStatusType(id)] = name
	}
	c.Assert(dbValues, tc.DeepEquals, map[K8sPodStatusType]string{
		K8sPodStatusUnset:   "unset",
		K8sPodStatusWaiting: "waiting",
		K8sPodStatusBlocked: "blocked",
		K8sPodStatusRunning: "running",
	})
}

// TestUnitAgentStatusDBValues ensures there's no skew between what's in the
// database table for unit agent status and the typed consts used in the
// state packages.
func (s *statusSuite) TestUnitAgentStatusDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, status FROM unit_agent_status_value")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[UnitAgentStatusType]string)
	for rows.Next() {
		var (
			id   int
			name string
		)
		err := rows.Scan(&id, &name)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[UnitAgentStatusType(id)] = name
	}
	c.Assert(dbValues, tc.DeepEquals, map[UnitAgentStatusType]string{
		UnitAgentStatusAllocating: "allocating",
		UnitAgentStatusExecuting:  "executing",
		UnitAgentStatusIdle:       "idle",
		UnitAgentStatusError:      "error",
		UnitAgentStatusFailed:     "failed",
		UnitAgentStatusLost:       "lost",
		UnitAgentStatusRebooting:  "rebooting",
	})
}

// TestWorkloadStatusDBValues ensures there's no skew between what's in the
// database table for unit workload status and the typed consts used in the
// state packages.
func (s *statusSuite) TestWorkloadStatusDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, status FROM workload_status_value")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[WorkloadStatusType]string)
	for rows.Next() {
		var (
			id   int
			name string
		)
		err := rows.Scan(&id, &name)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[WorkloadStatusType(id)] = name
	}
	c.Assert(dbValues, tc.DeepEquals, map[WorkloadStatusType]string{
		WorkloadStatusUnset:       "unset",
		WorkloadStatusUnknown:     "unknown",
		WorkloadStatusMaintenance: "maintenance",
		WorkloadStatusWaiting:     "waiting",
		WorkloadStatusBlocked:     "blocked",
		WorkloadStatusActive:      "active",
		WorkloadStatusTerminated:  "terminated",
		WorkloadStatusError:       "error",
	})
}

func (s *statusSuite) TestMachineStatusValues(c *tc.C) {
	db := s.DB()

	// Check that the status values in the machine_status_value table match
	// the instance status values in core status.
	rows, err := db.QueryContext(c.Context(), "SELECT id, status FROM machine_status_value ORDER BY id")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()
	var statusValues []struct {
		ID   int
		Name string
	}
	for rows.Next() {
		var statusValue struct {
			ID   int
			Name string
		}
		err = rows.Scan(&statusValue.ID, &statusValue.Name)
		c.Assert(err, tc.ErrorIsNil)
		statusValues = append(statusValues, statusValue)
	}
	c.Assert(statusValues, tc.HasLen, 5)
	c.Check(statusValues[0].ID, tc.Equals, 0)
	c.Check(statusValues[0].Name, tc.Equals, "error")
	c.Check(statusValues[1].ID, tc.Equals, 1)
	c.Check(statusValues[1].Name, tc.Equals, "started")
	c.Check(statusValues[2].ID, tc.Equals, 2)
	c.Check(statusValues[2].Name, tc.Equals, "pending")
	c.Check(statusValues[3].ID, tc.Equals, 3)
	c.Check(statusValues[3].Name, tc.Equals, "stopped")
	c.Check(statusValues[4].ID, tc.Equals, 4)
	c.Check(statusValues[4].Name, tc.Equals, "down")
}

func (s *statusSuite) TestMachineStatusValuesConversion(c *tc.C) {
	tests := []struct {
		statusValue string
		expected    int
	}{
		{statusValue: "error", expected: 0},
		{statusValue: "started", expected: 1},
		{statusValue: "pending", expected: 2},
		{statusValue: "stopped", expected: 3},
		{statusValue: "down", expected: 4},
	}

	for _, test := range tests {
		a, err := DecodeMachineStatus(test.statusValue)
		c.Assert(err, tc.ErrorIsNil)
		b, err := EncodeMachineStatus(a)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(b, tc.Equals, test.expected)
	}
}

func (s *statusSuite) TestMachineStatusValuesAgainstDB(c *tc.C) {
	m := make(map[string]int)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT status, id FROM machine_status_value")
		if err != nil {
			return errors.Capture(err)
		}
		defer rows.Close()

		for rows.Next() {
			var status string
			var id int
			err = rows.Scan(&status, &id)
			if err != nil {
				return errors.Capture(err)
			}
			m[status] = id
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	tests := []struct {
		statusValue string
		expected    int
	}{
		{statusValue: "error", expected: 0},
		{statusValue: "started", expected: 1},
		{statusValue: "pending", expected: 2},
		{statusValue: "stopped", expected: 3},
		{statusValue: "down", expected: 4},
	}

	for _, test := range tests {
		a, err := DecodeMachineStatus(test.statusValue)
		c.Assert(err, tc.ErrorIsNil)
		b, err := EncodeMachineStatus(a)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(b, tc.Equals, test.expected)

		c.Check(m[test.statusValue], tc.Equals, b)
	}
}

func (s *statusSuite) TestInstanceStatusValues(c *tc.C) {
	db := s.DB()

	// Check that the status values in the machine_cloud_instance_status_value table match
	// the instance status values in core status.
	rows, err := db.QueryContext(c.Context(), "SELECT id, status FROM machine_cloud_instance_status_value ORDER BY id")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()
	var statusValues []struct {
		ID   int
		Name string
	}
	for rows.Next() {
		var statusValue struct {
			ID   int
			Name string
		}
		err = rows.Scan(&statusValue.ID, &statusValue.Name)
		c.Assert(err, tc.ErrorIsNil)
		statusValues = append(statusValues, statusValue)
	}
	c.Assert(statusValues, tc.HasLen, 5)
	c.Check(statusValues[0].ID, tc.Equals, 0)
	c.Check(statusValues[0].Name, tc.Equals, "unknown")
	c.Check(statusValues[1].ID, tc.Equals, 1)
	c.Check(statusValues[1].Name, tc.Equals, "pending")
	c.Check(statusValues[2].ID, tc.Equals, 2)
	c.Check(statusValues[2].Name, tc.Equals, "allocating")
	c.Check(statusValues[3].ID, tc.Equals, 3)
	c.Check(statusValues[3].Name, tc.Equals, "running")
	c.Check(statusValues[4].ID, tc.Equals, 4)
	c.Check(statusValues[4].Name, tc.Equals, "provisioning error")
}

func (s *statusSuite) TestInstanceStatusValuesConversion(c *tc.C) {
	tests := []struct {
		statusValue string
		expected    int
	}{
		{statusValue: "", expected: 0},
		{statusValue: "unknown", expected: 0},
		{statusValue: "pending", expected: 1},
		{statusValue: "allocating", expected: 2},
		{statusValue: "running", expected: 3},
		{statusValue: "provisioning error", expected: 4},
	}

	for _, test := range tests {
		a, err := DecodeCloudInstanceStatus(test.statusValue)
		c.Assert(err, tc.ErrorIsNil)

		b, err := EncodeCloudInstanceStatus(a)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(b, tc.Equals, test.expected)
	}
}

func (s *statusSuite) TestInstanceStatusValuesAgainstDB(c *tc.C) {
	m := make(map[string]int)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT status, id FROM machine_cloud_instance_status_value")
		if err != nil {
			return errors.Capture(err)
		}
		defer rows.Close()

		for rows.Next() {
			var status string
			var id int
			err = rows.Scan(&status, &id)
			if err != nil {
				return errors.Capture(err)
			}
			m[status] = id
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	tests := []struct {
		statusValue string
		expected    int
	}{
		{statusValue: "unknown", expected: 0},
		{statusValue: "pending", expected: 1},
		{statusValue: "allocating", expected: 2},
		{statusValue: "running", expected: 3},
		{statusValue: "provisioning error", expected: 4},
	}

	for _, test := range tests {
		a, err := DecodeCloudInstanceStatus(test.statusValue)
		c.Assert(err, tc.ErrorIsNil)

		b, err := EncodeCloudInstanceStatus(a)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(b, tc.Equals, test.expected)

		c.Check(m[test.statusValue], tc.Equals, b)
	}
}
