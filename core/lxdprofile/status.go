// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile

import (
	"strings"
)

const (
	// EmptyStatus represents the initial status
	EmptyStatus = ""

	// SuccessStatus defines if the lxd profile upgrade was a success
	SuccessStatus = "Success"

	// NotRequiredStatus defines when the lxd profile upgrade was not required
	NotRequiredStatus = "Not Required"

	// ErrorStatus defines when the lxd profile is in an error state
	ErrorStatus = "Error"
)

// UpgradeStatusFinished defines if the upgrade status has completed
func UpgradeStatusFinished(status string) bool {
	if status == SuccessStatus || status == NotRequiredStatus {
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

	return strings.HasPrefix(status, ErrorStatus)
}
