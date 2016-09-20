// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"
	"time"

	"github.com/juju/errors"
)

// StatusHistoryFilter holds arguments that can be use to filter a status history backlog.
type StatusHistoryFilter struct {
	Size  int
	Date  *time.Time
	Delta *time.Duration
}

// Validate checks that the minimum requirements of a StatusHistoryFilter are met.
func (f *StatusHistoryFilter) Validate() error {
	s := f.Size > 0
	t := f.Date != nil
	d := f.Delta != nil

	switch {
	case !(s || t || d):
		return errors.NotValidf("empty struct")
	case s && t:
		return errors.NotValidf("Size and Date together")
	case s && d:
		return errors.NotValidf("Size and Delta together")
	case t && d:
		return errors.NotValidf("Date and Delta together")
	}
	return nil
}

// StatusHistoryGetter instances can fetch their status history.
type StatusHistoryGetter interface {
	StatusHistory(filter StatusHistoryFilter) ([]StatusInfo, error)
}

// InstanceStatusHistoryGetter instances can fetch their instance status history.
type InstanceStatusHistoryGetter interface {
	InstanceStatusHistory(filter StatusHistoryFilter) ([]StatusInfo, error)
}

// DetailedStatus holds status info about a machine or unit agent.
type DetailedStatus struct {
	Status  Status
	Info    string
	Data    map[string]interface{}
	Since   *time.Time
	Kind    HistoryKind
	Version string
	// TODO(perrito666) make sure this is not used and remove.
	Life string
	Err  error
}

// History holds many DetailedStatus,
type History []DetailedStatus

// push will take an input DetailedStatus and put it as element
// 0 of the current history pushing all other elements by 1
// and return the last element, now out of the slice.
func (h *History) push(new DetailedStatus) DetailedStatus {
	ch := *h
	old := ch[0]
	for i := 0; i < len(ch)-1; i++ {
		ch[i] = ch[i+1]
	}
	ch[len(ch)-1] = new
	return old
}

// SquashLogs will find repetitions of N consequent status log entries into just
// one appearance of them and information about repetition.
func (h *History) SquashLogs(cycleSize int) History {
	statuses := *h
	if len(statuses) <= cycleSize {
		return statuses
	}
	buffer := History{}
	for i := 0; i < cycleSize; i++ {
		buffer = append(buffer, statuses[i])
	}
	result := []DetailedStatus{}
	// TODO(perrito666) 2016-05-02 lp:1558657
	now := time.Now()
	var repeat int
	var i int
	repeatStatus := DetailedStatus{
		Status: Idle,
		Info:   "",
		Since:  &now,
	}

	for i = cycleSize; i < len(statuses); {
		if i+cycleSize > len(statuses) {
			break
		}
		subset := statuses[i : i+cycleSize]
		repetition := true
		for j := range subset {
			if subset[j].Status != buffer[j].Status || subset[j].Info != buffer[j].Info {
				repetition = false
			}
		}
		if repetition {
			repeat++
			i = i + cycleSize
			continue
		}
		if repeat > 0 {
			rstatus := fmt.Sprintf("last %d statuses repeated %d times", cycleSize, repeat)
			repeat = 0
			repeatStatus.Info = rstatus
			for j := 0; j < cycleSize; j++ {
				result = append(result, buffer.push(subset[j]))
			}
			result = append(result, repeatStatus)
			i = i + cycleSize
			continue
		}
		result = append(result, buffer.push(statuses[i]))
		i++
	}
	for j := 0; j < cycleSize; j++ {
		result = append(result, buffer[j])
	}
	if repeat > 0 {
		rstatus := fmt.Sprintf("last %d statuses repeated %d times", cycleSize, repeat)
		repeatStatus.Info = rstatus
		result = append(result, repeatStatus)
	}
	if i < len(statuses)-1 {
		result = append(result, statuses[i+1:]...)
	}

	return result
}

// HistoryKind represents the possible types of
// status history entries.
type HistoryKind string

const (
	// KindUnit represents agent and workload combined.
	KindUnit HistoryKind = "unit"
	// KindUnitAgent represent a unit agent status history entry.
	KindUnitAgent HistoryKind = "juju-unit"
	// KindWorkload represents a charm workload status history entry.
	KindWorkload HistoryKind = "workload"
	// KindMachineInstance represents an entry for a machine instance.
	KindMachineInstance = "machine"
	// KindMachine represents an entry for a machine agent.
	KindMachine = "juju-machine"
	// KindContainerInstance represents an entry for a container instance.
	KindContainerInstance = "container"
	// KindContainer represents an entry for a container agent.
	KindContainer = "juju-container"
)

// String returns a string representation of the HistoryKind.
func (k HistoryKind) String() string {
	return string(k)
}

// Valid will return true if the current kind is a valid one.
func (k HistoryKind) Valid() bool {
	switch k {
	case KindUnit, KindUnitAgent, KindWorkload,
		KindMachineInstance, KindMachine,
		KindContainerInstance, KindContainer:
		return true
	}
	return false
}
