// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/rpcreflect"
)

var aboutToRestoreError = errors.New("juju restore is in progress - Juju functionality is limited to avoid data loss")
var restoreInProgressError = errors.New("juju restore is in progress - Juju api is off to prevent data loss")

// aboutToRestoreRoot a root that will only allow a limited
// set of methods to run, defined in allowedMethodsAboutToRestore.
type aboutToRestoreRoot struct {
	rpc.Root
}

// restoreRoot a root that will not allow calls whatsoever during restore.
type restoreInProgressRoot struct {
	rpc.Root
}

// newAboutToRestoreRoot creates a root where all API calls
// but restore will fail with aboutToRestoreError.
func newAboutToRestoreRoot(root rpc.Root) *aboutToRestoreRoot {
	return &aboutToRestoreRoot{
		Root: root,
	}
}

// newRestoreInProressRoot creates a root where all API calls
// but restore will fail with restoreInProgressError.
func newRestoreInProgressRoot(root rpc.Root) *restoreInProgressRoot {
	return &restoreInProgressRoot{
		Root: root,
	}
}

// FindMethod extended srvRoot.FindMethod. It returns aboutToRestoreError
// for all API calls except Client.Restore
// for use while Juju is preparing to restore a backup.
func (r *aboutToRestoreRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	caller, err := r.Root.FindMethod(rootName, version, methodName)
	if err != nil {
		return nil, err
	}
	if !isMethodAllowedAboutToRestore(rootName, methodName) {
		return nil, aboutToRestoreError
	}
	return caller, nil
}

var allowedMethodsAboutToRestore = set.NewStrings(
	"Client.FullStatus",     // for "juju status"
	"Client.ModelGet",       // for "juju ssh"
	"Client.PrivateAddress", // for "juju ssh"
	"Client.PublicAddress",  // for "juju ssh"
	"Client.WatchDebugLog",  // for "juju debug-log"
	"Backups.Restore",       // for "juju backups restore"
	"Backups.FinishRestore", // for "juju backups restore"
)

// isMethodAllowedAboutToRestore return true if this method is allowed when the server is in state.RestorePreparing mode
// at present only Backups.Restore is.
func isMethodAllowedAboutToRestore(rootName, methodName string) bool {
	fullName := rootName + "." + methodName
	return allowedMethodsAboutToRestore.Contains(fullName)
}

// FindMethod extended srvRoot.FindMethod. It returns restoreInProgressError
// for all API calls.
// for use while Juju is restoring a backup.
func (r *restoreInProgressRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	_, err := r.Root.FindMethod(rootName, version, methodName)
	if err != nil {
		return nil, err
	}
	return nil, restoreInProgressError
}
