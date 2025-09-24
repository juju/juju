// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"time"

	"github.com/juju/juju/core/machine"
	coreoperation "github.com/juju/juju/core/operation"
	"github.com/juju/juju/core/unit"
)

// TaskLogMessage is a timestamped message logged for a task.
type TaskLogMessage struct {
	Message   string
	Timestamp time.Time
}

// TransformToCore returns the log message in core operation format.
func (t TaskLogMessage) TransformToCore() coreoperation.TaskLogMessage {
	return coreoperation.TaskLogMessage{
		Message:   t.Message,
		Timestamp: t.Timestamp,
	}
}

// CompletedTask is the data required to finish a task.
type CompletedTask struct {
	TaskUUID  string
	StoreUUID string
	Status    string
	Message   string
}

// ReceiversWithResolvedLeaders represents receivers with matched leaders.
type ReceiversWithResolvedLeaders struct {
	Applications []string
	Machines     []machine.Name
	Units        []unit.Name
	LeaderUnits  []unit.Name
}
