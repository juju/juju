// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/operation"
)

// StartTask marks a task as running and logs the time it was started.
func (s *Service) StartTask(ctx context.Context, id string) error {
	return errors.NotImplemented
}

// FinishTask saves the result of a completed task.
func (s *Service) FinishTask(ctx context.Context, result operation.CompletedTaskResult) error {
	return errors.NotImplemented
}

// ReceiverFromTask returns a receiver string for the task identified.
// The string should satisfy the ActionReceiverTag type.
func (s *Service) ReceiverFromTask(ctx context.Context, id string) (string, error) {
	return "", errors.NotImplemented
}
