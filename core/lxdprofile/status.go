// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile

import (
	"fmt"
	"strings"
)

const (
	// EmptyStatus represents the initial status
	EmptyStatus = ""

	// SuccessStatus defines if the lxd profile upgrade was a success
	SuccessStatus = "Success"

	// NotRequiredStatus defines when the lxd profile upgrade was not required
	NotRequiredStatus = "Not Required"

	// NotKnownStatus defines a state where the document for the lxd profile
	// is removed, or never existed, but we don't know what the status should be.
	NotKnownStatus = "Not known"

	// ErrorStatus defines when the lxd profile is in an error state
	ErrorStatus = "Error"

	// NotSupportedStatus defines when a machine does not support lxd profiles.
	NotSupportedStatus = "Not Supported"
)

// AnnotateErrorStatus annotates an existing error with the correct status
func AnnotateErrorStatus(err error) string {
	return fmt.Sprintf("%s: %s", ErrorStatus, err.Error())
}

// UpgradeStatusFinished defines if the upgrade has completed
func UpgradeStatusFinished(status string) bool {
	if status == SuccessStatus || status == NotRequiredStatus || status == NotSupportedStatus {
		return true
	}
	return false
}

// UpgradeStatusTerminal defines if the status is in a terminal state. Success
// or not required is also considered terminal.
func UpgradeStatusTerminal(status string) bool {
	if UpgradeStatusFinished(status) {
		return true
	}

	return UpgradeStatusErrorred(status)
}

// UpgradeStatusErrorred defines if the status is in a error state.
func UpgradeStatusErrorred(status string) bool {
	return strings.HasPrefix(status, ErrorStatus)
}
