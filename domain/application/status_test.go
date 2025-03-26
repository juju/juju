// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	schematesting "github.com/juju/juju/domain/schema/testing"
)

type statusSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&statusSuite{})

// TestCloudContainerStatusDBValues ensures there's no skew between what's in the
// database table for cloud container status and the typed consts used in the
// state packages.
func (s *statusSuite) TestCloudContainerStatusDBValues(c *gc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, status FROM k8s_pod_status_value")
	c.Assert(err, jc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[CloudContainerStatusType]string)
	for rows.Next() {
		var (
			id   int
			name string
		)
		err := rows.Scan(&id, &name)
		c.Assert(err, jc.ErrorIsNil)
		dbValues[CloudContainerStatusType(id)] = name
	}
	c.Assert(dbValues, jc.DeepEquals, map[CloudContainerStatusType]string{
		CloudContainerStatusUnset:   "unset",
		CloudContainerStatusWaiting: "waiting",
		CloudContainerStatusBlocked: "blocked",
		CloudContainerStatusRunning: "running",
	})
}

// TestUnitAgentStatusDBValues ensures there's no skew between what's in the
// database table for unit agent status and the typed consts used in the
// state packages.
func (s *statusSuite) TestUnitAgentStatusDBValues(c *gc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, status FROM unit_agent_status_value")
	c.Assert(err, jc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[UnitAgentStatusType]string)
	for rows.Next() {
		var (
			id   int
			name string
		)
		err := rows.Scan(&id, &name)
		c.Assert(err, jc.ErrorIsNil)
		dbValues[UnitAgentStatusType(id)] = name
	}
	c.Assert(dbValues, jc.DeepEquals, map[UnitAgentStatusType]string{
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
func (s *statusSuite) TestWorkloadStatusDBValues(c *gc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, status FROM workload_status_value")
	c.Assert(err, jc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[WorkloadStatusType]string)
	for rows.Next() {
		var (
			id   int
			name string
		)
		err := rows.Scan(&id, &name)
		c.Assert(err, jc.ErrorIsNil)
		dbValues[WorkloadStatusType(id)] = name
	}
	c.Assert(dbValues, jc.DeepEquals, map[WorkloadStatusType]string{
		WorkloadStatusUnset:       "unset",
		WorkloadStatusUnknown:     "unknown",
		WorkloadStatusMaintenance: "maintenance",
		WorkloadStatusWaiting:     "waiting",
		WorkloadStatusBlocked:     "blocked",
		WorkloadStatusActive:      "active",
		WorkloadStatusTerminated:  "terminated",
	})
}
