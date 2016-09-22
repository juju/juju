// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"
)

var aboutToRestoreError = errors.New("juju restore is in progress - functionality is limited to avoid data loss")
var restoreInProgressError = errors.New("juju restore is in progress - API is disabled to prevent data loss")

// aboutToRestoreMethodsOnly can be used with restrictRoot to restrict
// the API to the methods allowed when the server is in
// state.RestorePreparing mode.
func aboutToRestoreMethodsOnly(facadeName string, methodName string) error {
	fullName := facadeName + "." + methodName
	if !allowedMethodsAboutToRestore.Contains(fullName) {
		return aboutToRestoreError
	}
	return nil
}

var allowedMethodsAboutToRestore = set.NewStrings(
	"Client.FullStatus",     // for "juju status"
	"Client.ModelGet",       // for "juju ssh"
	"Client.PrivateAddress", // for "juju ssh"
	"Client.PublicAddress",  // for "juju ssh"
	"Client.WatchDebugLog",  // for "juju debug-log"
	"Backups.Restore",       // for "juju backups restore"
	"Backups.FinishRestore", // for "juju backups restore"
	"Pinger.Ping",           // for connection health checks
)
