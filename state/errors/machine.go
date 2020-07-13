// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
)

type hasAssignedUnitsError struct {
	machineId string
	unitNames []string
}

func NewHasAssignedUnitsError(machineId string, unitNames []string) error {
	return &hasAssignedUnitsError{
		machineId: machineId,
		unitNames: unitNames,
	}
}

func (e *hasAssignedUnitsError) Error() string {
	return fmt.Sprintf("machine %s has unit %q assigned", e.machineId, e.unitNames[0])
}

func IsHasAssignedUnitsError(err error) bool {
	_, ok := errors.Cause(err).(*hasAssignedUnitsError)
	return ok
}

type hasContainersError struct {
	machineId    string
	containerIds []string
}

func NewHasContainersError(machineId string, containerIds []string) error {
	return &hasContainersError{
		machineId:    machineId,
		containerIds: containerIds,
	}
}

func (e *hasContainersError) Error() string {
	return fmt.Sprintf("machine %s is hosting containers %q", e.machineId, strings.Join(e.containerIds, ","))
}

// IshasContainersError reports whether or not the error is a
// hasContainersError, indicating that an attempt to destroy
// a machine failed due to it having containers.
func IsHasContainersError(err error) bool {
	_, ok := errors.Cause(err).(*hasContainersError)
	return ok
}

// hasAttachmentsError is the error returned by EnsureDead if the machine
// has attachments to resources that must be cleaned up first.
type hasAttachmentsError struct {
	machineId   string
	attachments []names.Tag
}

func NewHasAttachmentsError(machineId string, attachments []names.Tag) error {
	return &hasAttachmentsError{
		machineId:   machineId,
		attachments: attachments,
	}
}

func (e *hasAttachmentsError) Error() string {
	return fmt.Sprintf(
		"machine %s has attachments %s",
		e.machineId, e.attachments,
	)
}

// IsHasAttachmentsError reports whether or not the error is a
// hasAttachmentsError, indicating that an attempt to destroy
// a machine failed due to it having storage attachments.
func IsHasAttachmentsError(err error) bool {
	_, ok := errors.Cause(err).(*hasAttachmentsError)
	return ok
}
