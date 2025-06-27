// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/rpc/params"
)

// MachineRebootService is an interface that defines methods for managing machine reboots.
type MachineRebootService interface {
	// RequireMachineReboot sets the machine referenced by its UUID as requiring a reboot.
	RequireMachineReboot(ctx context.Context, uuid machine.UUID) error

	// ClearMachineReboot removes the reboot flag of the machine referenced by its UUID if a reboot has previously been required.
	ClearMachineReboot(ctx context.Context, uuid machine.UUID) error

	// IsMachineRebootRequired checks if the machine referenced by its UUID requires a reboot.
	IsMachineRebootRequired(ctx context.Context, uuid machine.UUID) (bool, error)

	// ShouldRebootOrShutdown determines whether a machine should reboot or shutdown
	ShouldRebootOrShutdown(ctx context.Context, uuid machine.UUID) (machine.RebootAction, error)

	// GetMachineUUID returns the UUID of a machine identified by its name.
	// It returns an errors.MachineNotFound if the machine does not exist.
	GetMachineUUID(ctx context.Context, machineName machine.Name) (machine.UUID, error)
}

// RebootRequester implements the RequestReboot API method
type RebootRequester struct {
	machineService MachineRebootService
	auth           GetAuthFunc
}

func NewRebootRequester(machineService MachineRebootService, auth GetAuthFunc) *RebootRequester {
	return &RebootRequester{
		machineService: machineService,
		auth:           auth,
	}
}

func (r *RebootRequester) oneRequest(ctx context.Context, tag names.Tag) error {
	if tag.Kind() != names.MachineTagKind {
		return errors.Errorf("%q should be a %s", tag, names.MachineTagKind)
	}

	uuid, err := r.machineService.GetMachineUUID(ctx, machine.Name(tag.Id()))
	if err != nil {
		return errors.Annotatef(err, "find machine uuid for machine %q", tag.Id())
	}
	err = r.machineService.RequireMachineReboot(ctx, uuid)
	return errors.Annotatef(err, "requires reboot for machine %q (%s)", tag.Id(), uuid)
}

// RequestReboot sets the reboot flag on the provided machines
func (r *RebootRequester) RequestReboot(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	auth, err := r.auth(ctx)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		if auth(tag) {
			err = r.oneRequest(ctx, tag)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// RebootActionGetter implements the GetRebootAction API method
type RebootActionGetter struct {
	auth           GetAuthFunc
	machineService MachineRebootService
}

func NewRebootActionGetter(machineService MachineRebootService, auth GetAuthFunc) *RebootActionGetter {
	return &RebootActionGetter{
		machineService: machineService,
		auth:           auth,
	}
}

func (r *RebootActionGetter) getOneAction(ctx context.Context, tag names.Tag) (params.RebootAction, error) {
	if tag.Kind() != names.MachineTagKind {
		return "", errors.Errorf("%q should be a %s", tag, names.MachineTagKind)
	}

	uuid, err := r.machineService.GetMachineUUID(ctx, machine.Name(tag.Id()))
	if err != nil {
		return "", errors.Annotatef(err, "find machine uuid for machine %q", tag.Id())
	}
	action, err := r.machineService.ShouldRebootOrShutdown(ctx, uuid)
	return params.RebootAction(action), errors.Annotatef(err, "get reboot action for machine %q (%s)", tag.Id(), uuid)
}

// GetRebootAction returns the action a machine agent should take.
// If a reboot flag is set on the machine, then that machine is
// expected to reboot (params.ShouldReboot).
// a reboot flag set on the machine parent or grandparent, will
// cause the machine to shutdown (params.ShouldShutdown).
// If no reboot flag is set, the machine should do nothing (params.ShouldDoNothing).
func (r *RebootActionGetter) GetRebootAction(ctx context.Context, args params.Entities) (params.RebootActionResults, error) {
	result := params.RebootActionResults{
		Results: make([]params.RebootActionResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	auth, err := r.auth(ctx)
	if err != nil {
		return params.RebootActionResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		if auth(tag) {
			result.Results[i].Result, err = r.getOneAction(ctx, tag)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// RebootFlagClearer implements the ClearReboot API call
type RebootFlagClearer struct {
	machineService MachineRebootService
	auth           GetAuthFunc
}

func NewRebootFlagClearer(machineService MachineRebootService, auth GetAuthFunc) *RebootFlagClearer {
	return &RebootFlagClearer{
		machineService: machineService,
		auth:           auth,
	}
}

func (r *RebootFlagClearer) clearOneFlag(ctx context.Context, tag names.Tag) error {
	if tag.Kind() != names.MachineTagKind {
		return errors.Errorf("%q should be a %s", tag, names.MachineTagKind)
	}

	uuid, err := r.machineService.GetMachineUUID(ctx, machine.Name(tag.Id()))
	if err != nil {
		return errors.Annotatef(err, "find machine uuid for machine %q", tag.Id())
	}
	err = r.machineService.ClearMachineReboot(ctx, uuid)
	return errors.Annotatef(err, "clear reboot flag for machine %q (%s)", tag.Id(), uuid)
}

// ClearReboot will clear the reboot flag on provided machines, if it exists.
func (r *RebootFlagClearer) ClearReboot(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	auth, err := r.auth(ctx)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		if auth(tag) {
			err = r.clearOneFlag(ctx, tag)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}
