// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
)

type statusSuite struct {
	schematesting.ModelSuite
}

var _ = tc.Suite(&statusSuite{})

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
