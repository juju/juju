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
	unitNames := make([]string, len(e.unitNames))
	for i, unitName := range e.unitNames {
		unitNames[i] = fmt.Sprintf("%q", unitName)
	}
	return fmt.Sprintf("machine %s has unit(s) %s assigned", e.machineId, strings.Join(unitNames, ", "))
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
	containerIds := make([]string, len(e.containerIds))
	for i, containerId := range e.containerIds {
		containerIds[i] = fmt.Sprintf("%q", containerId)
	}
	return fmt.Sprintf("machine %s is hosting container(s) %s", e.machineId, strings.Join(containerIds, ", "))
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
