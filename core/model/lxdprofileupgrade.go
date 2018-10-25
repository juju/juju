// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import "github.com/juju/errors"

// LXDProfileUpgradeStatus is the current status of a lxd profile upgrade
type LXDProfileUpgradeStatus string

const (
	LXDProfileUpgradeNotStarted  LXDProfileUpgradeStatus = "not started"
	LXDProfileUpgradeNotRequired LXDProfileUpgradeStatus = "not required"
	LXDProfileUpgradeUpgrading   LXDProfileUpgradeStatus = "upgrading"
	LXDProfileUpgradeCompleted   LXDProfileUpgradeStatus = "completed"
	LXDProfileUpgradeError       LXDProfileUpgradeStatus = "error"
)

var LXDProfileUpgradeStatusOrder map[LXDProfileUpgradeStatus]int = map[LXDProfileUpgradeStatus]int{
	LXDProfileUpgradeNotStarted:  0,
	LXDProfileUpgradeNotRequired: 1,
	LXDProfileUpgradeUpgrading:   2,
	LXDProfileUpgradeCompleted:   3,
	LXDProfileUpgradeError:       4,
}

// ValidateLXDProfileUpgradeStatus validates a the input status as valid for a
// unit, returning the valid status or an error.
func ValidateLXDProfileUpgradeStatus(status LXDProfileUpgradeStatus) (LXDProfileUpgradeStatus, error) {
	if _, ok := LXDProfileUpgradeStatusOrder[status]; !ok {
		return LXDProfileUpgradeNotStarted, errors.NotValidf("upgrade series status of %q is", status)
	}
	return status, nil
}
