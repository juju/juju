// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/rpc/rpcreflect"
	"github.com/juju/juju/state"
)

var aboutToRestoreError = errors.New("juju restore is in progress - Juju functionality is limited to avoid data loss")
var restoreInProgressError = errors.New("juju restore is in progress - Juju api is off to prevent data loss")

type aboutToRestoreRoot struct {
	srvRoot
}

type restoreInProgressRoot struct {
	srvRoot
}

// newAboutToRestoreRoot creates a root where all API calls
// but restore will fail with aboutToRestoreError.
func newAboutToRestoreRoot(root *initialRoot, entity state.Entity) *aboutToRestoreRoot {
	return &aboutToRestoreRoot{
		srvRoot: *newSrvRoot(root, entity),
	}
}

// newRestoreInProressRoot creates a root where all API calls
// but restore will fail with restoreInProgressError.
func newRestoreInProgressRoot(root *initialRoot, entity state.Entity) *restoreInProgressRoot {
	return &restoreInProgressRoot{
		srvRoot: *newSrvRoot(root, entity),
	}
}

// FindMethod extended srvRoot.FindMethod. It returns aboutToRestoreError
// for all API calls except Client.Restore
// for use while Juju is preparing to restore a backup.
func (r *aboutToRestoreRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	if _, _, err := r.lookupMethod(rootName, version, methodName); err != nil {
		return nil, err
	}
	if !isMethodAllowedAboutToRestore(rootName, methodName) {
		return nil, aboutToRestoreError
	}
	return r.srvRoot.FindMethod(rootName, version, methodName)
}

var allowedMethodsAboutToRestore = set.NewStrings(
	"Client.Restore", // for "juju restore"
)

func isMethodAllowedAboutToRestore(rootName, methodName string) bool {
	fullName := rootName + "." + methodName
	return allowedMethodsAboutToRestore.Contains(fullName)
}

// FindMethod extended srvRoot.FindMethod. It returns restoreInProgressError
// for all API calls.
// for use while Juju is restoring a backup.
func (r *restoreInProgressRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	if _, _, err := r.lookupMethod(rootName, version, methodName); err != nil {
		return nil, err
	}
	return nil, restoreInProgressError
}
