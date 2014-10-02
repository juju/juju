// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (

	"github.com/juju/errors"
	"launchpad.net/tomb"

	"github.com/juju/juju/worker"
)

type RestoreStatus string

var (
	UnknownRestoreStatus RestoreStatus = "UNKNOWN"
	PreparingRestore RestoreStatus = "PREPARING"
	RestoreInProgress RestoreStatus = "RESTORING"
)

type restoreContext struct {
	restoreStatus RestoreStatus
	tomb *tomb.Tomb
}

func NewRestoreContext(tomb *tomb.Tomb) *restoreContext {
	return &restoreContext{
		UnknownRestoreStatus, 
		tomb}
}

func (r *restoreContext) restoreRunning() bool {
	return r.restoreStatus == RestoreInProgress
}

func (r *restoreContext) restorePreparing() bool {
	return r.restoreStatus == PreparingRestore
}

// PrepareRestore will flag the agent to allow only one command:
// Restore, this will ensure that we can do all the file movements
// required for restore and no one will do changes while we do that.
// it will return error if the machine is already in this state.
func (r *restoreContext) PrepareRestore() error {
	if r.restoreStatus != UnknownRestoreStatus {
		return errors.Errorf("already in restore mode")
	}
	r.restoreStatus = PreparingRestore
	return nil
}

// BeginRestore will flag the agent to disallow all commands since
// restore should be running and therefore making changes that
// would override anything done.
func (r *restoreContext) BeginRestore() error {
	switch r.restoreStatus {
	case UnknownRestoreStatus:
		return errors.Errorf("not in restore mode, cannot begin restoration")
	case RestoreInProgress:
		return errors.Errorf("already restoring")
	}
	r.restoreStatus = RestoreInProgress
	return nil
}

// FinishRestore will restart jujud and err if restore flag is not true
func (r *restoreContext) FinishRestore() error {
	if r.restoreStatus != RestoreInProgress {
		return errors.Errorf("restore is not in progress")
	}
	r.tomb.Kill(worker.ErrTerminateAgent)
	return nil
}
