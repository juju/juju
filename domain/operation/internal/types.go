// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"time"

	coreoperation "github.com/juju/juju/core/operation"
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
