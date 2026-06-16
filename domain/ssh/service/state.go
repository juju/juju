// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/juju/domain/ssh"
)

// State describes the persistence behaviour required by the SSH request
// service.
type State interface {
	InsertSSHConnRequest(context.Context, ssh.SSHConnRequest, time.Time) error
	GetSSHConnRequest(context.Context, string, time.Time) (ssh.SSHConnRequest, error)
	RemoveSSHConnRequest(context.Context, string) error
	PruneExpiredSSHConnRequests(context.Context, time.Time) error
	InitialWatchSSHConnRequestsStatement() (string, string)
}
