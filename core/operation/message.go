// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"encoding/json"
	"time"

	"github.com/juju/juju/internal/errors"
)

// TaskLogMessage is a timestamped message logged for a task.
type TaskLogMessage struct {
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// Encode returns a json encoded string with the contents of the TaskLogMessage.
func (t TaskLogMessage) Encode() (string, error) {
	encodedString, err := json.Marshal(TaskLogMessage{
		Message:   t.Message,
		Timestamp: t.Timestamp.UTC(),
	})
	return string(encodedString), errors.Capture(err)
}

// DecodeTaskLogEntry takes an json encoded string and returns a TaskLogMessage
// with unmarshalled contents.
func DecodeTaskLogEntry(encodedMessage string) (TaskLogMessage, error) {
	var taskLog TaskLogMessage
	err := json.Unmarshal([]byte(encodedMessage), &taskLog)
	return taskLog, errors.Capture(err)
}
