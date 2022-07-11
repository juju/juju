// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
)

const (
	HasAssignedUnitsError = errors.ConstError("has assigned units")

	// HasAttachmentsError indicates that an attempt to destroy
	// a machine failed due to it having storage attachments.
	HasAttachmentsError = errors.ConstError("machine has attachments")

	// HasContainersError indicates that the machine had attempted to be
	// destroyed with containers still running.
	HasContainersError = errors.ConstError("machine is hosting containers")
)

func NewHasAssignedUnitsError(machineId string, unitNames []string) error {
	return errors.WithType(
		fmt.Errorf("machine %s has unit %q assigned",
			machineId,
			unitNames[0]),
		HasAssignedUnitsError,
	)
}

// NewHasContainersError creates a new error that satisfies HasContainersError
func NewHasContainersError(machineId string, containerIds []string) error {
	return errors.WithType(
		fmt.Errorf("machine %s is hosting containers %q",
			machineId,
			strings.Join(containerIds, ",")),
		HasContainersError,
	)
}

func NewHasAttachmentsError(machineId string, attachments []names.Tag) error {
	return errors.WithType(
		fmt.Errorf(
			"machine %s has attachments %s",
			machineId,
			attachments),
		HasAttachmentsError,
	)
}
