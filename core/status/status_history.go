// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"time"

	"github.com/juju/collections/set"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// StatusHistoryFilter holds arguments that can be use to filter a status history backlog.
type StatusHistoryFilter struct {
	// Size indicates how many results are expected at most.
	Size int
	// FromDate indicates the earliest date from which logs are expected.
	FromDate *time.Time
	// Delta indicates the age of the oldest log expected.
	Delta *time.Duration
	// Exclude indicates the status messages that should be excluded
	// from the returned result.
	Exclude set.Strings
}

// Validate checks that the minimum requirements of a StatusHistoryFilter are met.
func (f *StatusHistoryFilter) Validate() error {
	s := f.Size > 0
	t := f.FromDate != nil
	d := f.Delta != nil

	switch {
	case !(s || t || d):
		return errors.Errorf("missing filter parameters %w", coreerrors.NotValid)
	case s && t:
		return errors.Errorf("Size and Date together %w", coreerrors.NotValid)
	case s && d:
		return errors.Errorf("Size and Delta together %w", coreerrors.NotValid)
	case t && d:
		return errors.Errorf("Date and Delta together %w", coreerrors.NotValid)
	}
	return nil
}

// InstanceStatusHistoryGetter instances can fetch their instance status history.
type InstanceStatusHistoryGetter interface {
	InstanceStatusHistory(filter StatusHistoryFilter) ([]StatusInfo, error)
}

// DetailedStatus holds status info about a machine or unit agent.
type DetailedStatus struct {
	Status Status
	Info   string
	Data   map[string]interface{}
	Since  *time.Time
	Kind   HistoryKind
}

// History holds many DetailedStatus,
type History []DetailedStatus

// HistoryKind represents the possible types of
// status history entries.
type HistoryKind string

// IMPORTANT DEV NOTE: when changing this HistoryKind list in any way, these may need to be revised:
//
// * HistoryKind.Valid()
// * AllHistoryKind()
// * command help for 'show-status-log' describing these kinds.
const (
	// KindModel represents the model itself.
	KindModel HistoryKind = "model"
	// KindApplication represents an entry for an application.
	KindApplication HistoryKind = "application"
	// KindSAAS represents an entry for a saas application.
	KindSAAS HistoryKind = "saas"
	// KindUnit represents agent and workload combined.
	KindUnit HistoryKind = "unit"
	// KindUnitAgent represent a unit agent status history entry.
	KindUnitAgent HistoryKind = "juju-unit"
	// KindWorkload represents a charm workload status history entry.
	KindWorkload HistoryKind = "workload"
	// KindMachineInstance represents an entry for a machine instance.
	KindMachineInstance HistoryKind = "machine"
	// KindMachine represents an entry for a machine agent.
	KindMachine HistoryKind = "juju-machine"
	// KindContainerInstance represents an entry for a container instance.
	KindContainerInstance HistoryKind = "container"
	// KindContainer represents an entry for a container agent.
	KindContainer HistoryKind = "juju-container"
	// KindFilesystem represents an entry for a filesystem.
	KindFilesystem HistoryKind = "filesystem"
	// KindVolume represents an entry for a volume.
	KindVolume HistoryKind = "volume"
)

// String returns a string representation of the HistoryKind.
func (k HistoryKind) String() string {
	return string(k)
}

// Valid will return true if the current kind is a valid one.
func (k HistoryKind) Valid() bool {
	switch k {
	case KindModel, KindUnit, KindUnitAgent, KindWorkload,
		KindApplication, KindSAAS,
		KindMachineInstance, KindMachine,
		KindContainerInstance, KindContainer,
		KindFilesystem, KindVolume:
		return true
	}
	return false
}

// AllHistoryKind will return all valid HistoryKinds.
func AllHistoryKind() map[HistoryKind]string {
	return map[HistoryKind]string{
		KindModel:             "statuses for the model itself",
		KindApplication:       "statuses for the specified application",
		KindSAAS:              "statuses for the specified SAAS application",
		KindUnit:              "statuses for specified unit and its workload",
		KindUnitAgent:         "statuses from the agent that is managing a unit",
		KindWorkload:          "statuses for unit's workload",
		KindMachineInstance:   "statuses that occur due to provisioning of a machine",
		KindMachine:           "status of the agent that is managing a machine",
		KindContainerInstance: "statuses from the agent that is managing containers",
		KindContainer:         "statuses from the containers only and not their host machines",
		KindFilesystem:        "statuses from the specified filesystem",
		KindVolume:            "statuses from the specified volume",
	}
}
