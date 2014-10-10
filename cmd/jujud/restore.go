// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"github.com/juju/errors"
	"launchpad.net/tomb"
)

// RestoreStatus is the type of the possible statuses for restore mode.
type RestoreStatus string

const (
	// UnknownRestoreStatus is the initial status of the context.
	UnknownRestoreStatus RestoreStatus = "UNKNOWN"
	// PreparingRestore status is an intermediate state it signals that
	// preparations for restore are happening and as such no api commands
	// should be accepted besides the actual "Restore".
	PreparingRestore RestoreStatus = "PREPARING"
	// RestoreInProgress indicates that no api command should be allowed
	// since the restore process will wipe the state db.
	RestoreInProgress RestoreStatus = "RESTORING"
)

type restoreContext struct {
	restoreStatus RestoreStatus
	tomb          *tomb.Tomb
}

// NewRestoreContext returns a restoreContext in UnknownRestoreStatus and
// holding a reference to the provided tomb which will be used when
// restore process finishes to restart jujud.
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

// HandleCall will receive the FindMethod params and enable restore
// mode when appropriate
func (r *restoreContext) HandleCall(rootName, methodName string) error {
	if rootName != "Backups" {
		return nil
	}
	switch methodName {
	case "PrepareRestore":
		return r.PrepareRestore()
	case "Restore":
		return r.BeginRestore()
	case "FinishRestore":
		return r.FinishRestore()
	}
	return nil

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

// FinishRestore will restart jujud and err if restore flag is not true.
func (r *restoreContext) FinishRestore() error {
	//TODO (perrito666) better check that restore actually took place
	return nil
}
