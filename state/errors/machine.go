// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
)

const (
	HasAssignedUnitsError = errors.ConstError("has assigned units")

	// HasAttachmentsError indicates that an attempt to destroy
	// a machine failed due to it having storage attachments.
	HasAttachmentsError = errors.ConstError("machine has attachments")

	// HasContainersError indicates that the machine had attempted to be
	// destroyed with containers still running.
	HasContainersError = errors.ConstError("machine is hosting containers")

	// IsControllerMemberError indicates the machine had attempted to be
	// destroyed whilst still considered a controller.
	IsControllerMemberError = errors.ConstError("machine is still a controller member")
)

// NewHasAssignedUnitsError creates a new error that satisfies HasAssignedUnitsError.
func NewHasAssignedUnitsError(machineId string, unitNames []string) error {
	return errors.WithType(
		fmt.Errorf("machine %s has unit %q assigned",
			machineId,
			unitNames[0]),
		HasAssignedUnitsError,
	)
}

// NewHasContainersError creates a new error that satisfies HasContainersError.
func NewHasContainersError(machineId string, containerIds []string) error {
	return errors.WithType(
		fmt.Errorf("machine %s is hosting containers %q",
			machineId,
			strings.Join(containerIds, ",")),
		HasContainersError,
	)
}

// NewHasAttachmentsError creates a new error that satisfies HasAttachmentsError.
func NewHasAttachmentsError(machineId string, attachments []names.Tag) error {
	return errors.WithType(
		fmt.Errorf(
			"machine %s has attachments %s",
			machineId,
			attachments),
		HasAttachmentsError,
	)
}

const (
	voting    = "voting"
	nonvoting = "non-voting"
)

// NewIsControllerMemberError creates a new error that satisfies IsControllerMemberError.
func NewIsControllerMemberError(machineId string, isVoting bool) error {
	status := nonvoting
	if isVoting {
		status = voting
	}
	return errors.WithType(
		fmt.Errorf(
			"machine %s is still a %s controller member",
			machineId, status),
		IsControllerMemberError,
	)
}
