// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"time"

	"github.com/juju/juju/domain/operation"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination migrations_mock_test.go github.com/juju/juju/domain/operation/modelmigration ImportService

// taskMessage is a description.ActionMessage implementation for operation.TaskLog.
type taskMessage struct {
	operation.TaskLog
}

// Timestamp implements description.ActionMessage.
func (t taskMessage) Timestamp() time.Time {
	return t.TaskLog.Timestamp
}

// Message implements description.ActionMessage.
func (t taskMessage) Message() string {
	return t.TaskLog.Message
}
