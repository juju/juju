// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// Backend provides required state for the Facade.
type Backend interface {
	ControllerConfig() (controller.Config, error)
	WatchControllerConfig() (state.NotifyWatcher, error)
	SSHServerHostKey() (string, error)
	HostKeyForVirtualHostname(info virtualhostname.Info) ([]byte, error)
	AuthorizedKeysForModel(uuid string) ([]string, error)
	K8sNamespaceAndPodName(modelUUID string, unitName string) (string, string, error)
	ModelAccess(userTag names.UserTag, uuid string) (permission.UserAccess, error)
	ControllerAccess(userTag names.UserTag) (permission.UserAccess, error)
	MachineExists(modelUUID, machineID string) (bool, error)
	UnitExists(modelUUID, unitName string) (bool, error)
	ModelType(modelUUID string) (state.ModelType, error)
}

// Facade allows model config manager clients to watch controller config changes and fetch controller config.
type Facade struct {
	resources facade.Resources

	backend Backend
}

// NewFacade returns a new SSHServer facade to be registered for use within
// the worker.
func NewFacade(ctx facade.Context, backend Backend) *Facade {
	return &Facade{
		resources: ctx.Resources(),
		backend:   backend,
	}
}

// ControllerConfig returns the current controller config.
func (f *Facade) ControllerConfig() (params.ControllerConfigResult, error) {
	result := params.ControllerConfigResult{}
	config, err := f.backend.ControllerConfig()
	if err != nil {
		return result, err
	}
	result.Config = params.ControllerConfig(config)
	return result, nil
}

// WatchControllerConfig creates a watcher and returns it's ID for watching upon.
func (f *Facade) WatchControllerConfig() (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	w, err := f.backend.WatchControllerConfig()
	if err != nil {
		return result, err
	}
	if _, ok := <-w.Changes(); ok {
		result.NotifyWatcherId = f.resources.Register(w)
	} else {
		return result, watcher.EnsureErr(w)
	}
	return result, nil
}

// SSHServerHostKey returns the controller's SSH server host key.
func (f *Facade) SSHServerHostKey() (params.StringResult, error) {
	result := params.StringResult{}
	key, err := f.backend.SSHServerHostKey()
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
	}
	result.Result = key
	return result, nil
}

// VirtualHostKey returns the virtual private host key for the target virtual hostname.
func (facade *Facade) VirtualHostKey(arg params.SSHVirtualHostKeyRequestArg) (params.SSHHostKeyResult, error) {
	var res params.SSHHostKeyResult

	info, err := virtualhostname.Parse(arg.Hostname)
	if err != nil {
		res.Error = apiservererrors.ServerError(errors.Annotate(err, "failed to parse hostname"))
		return res, nil
	}

	key, err := facade.backend.HostKeyForVirtualHostname(info)
	if err != nil {
		res.Error = apiservererrors.ServerError(err)
		return res, nil
	}

	return params.SSHHostKeyResult{HostKey: key}, nil
}

// ListAuthorizedKeysForModel returns the authorized keys for the model.
func (f *Facade) ListAuthorizedKeysForModel(args params.ListAuthorizedKeysArgs) (params.ListAuthorizedKeysResult, error) {
	authKeys, err := f.backend.AuthorizedKeysForModel(args.ModelUUID)
	if err != nil {
		return params.ListAuthorizedKeysResult{
			Error: apiservererrors.ServerError(errors.Annotate(err, "failed to get authorized keys for model")),
		}, nil
	}
	if len(authKeys) == 0 {
		return params.ListAuthorizedKeysResult{
			Error: apiservererrors.ServerError(errors.NotValidf("no authorized keys for model")),
		}, nil
	}
	return params.ListAuthorizedKeysResult{
		AuthorizedKeys: authKeys,
	}, nil
}

// ResolveK8sExecInfo returns the namespace and pod name for the given model UUID and unit name.
func (f *Facade) ResolveK8sExecInfo(arg params.SSHK8sExecArg) (params.SSHK8sExecResult, error) {
	result := params.SSHK8sExecResult{}
	var err error
	result.Namespace, result.PodName, err = f.backend.K8sNamespaceAndPodName(arg.ModelUUID, arg.UnitName)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}
	return result, nil
}

// CheckSSHAccess checks whether the specified user has SSH access to the given destination
// by consulting the state.
func (f *Facade) CheckSSHAccess(arg params.CheckSSHAccessArg) params.BoolResult {
	result := params.BoolResult{}

	ok := names.IsValidUser(arg.User)
	if !ok {
		result.Error = apiservererrors.ServerError(errors.NotValidf("invalid user %q", arg.User))
		return result
	}
	userTag := names.NewUserTag(arg.User)

	destination, err := virtualhostname.Parse(arg.Destination)
	if err != nil {
		result.Error = apiservererrors.ServerError(errors.Annotate(err, "failed to parse destination"))
		return result
	}

	modelAccess, err := f.backend.ModelAccess(userTag, destination.ModelUUID())
	if err != nil && !errors.Is(err, errors.NotFound) {
		result.Error = apiservererrors.ServerError(errors.Annotate(err, "failed to get model access"))
		return result
	}

	if modelAccess.Access == permission.AdminAccess {
		result.Result = true
		return result
	}

	controllerAccess, err := f.backend.ControllerAccess(userTag)
	if err != nil && !errors.Is(err, errors.NotFound) {
		result.Error = apiservererrors.ServerError(errors.Annotate(err, "failed to get controller access"))
		return result
	}

	if controllerAccess.Access == permission.AdminAccess {
		result.Result = true
	}

	return result
}

// ValidateVirtualHostname validates that the components
// of the destination virtual hostname exist.
func (f *Facade) ValidateVirtualHostname(arg params.ValidateVirtualHostnameArg) params.ErrorResult {
	result := params.ErrorResult{}
	destination, err := virtualhostname.Parse(arg.Hostname)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	switch destination.Target() {
	case virtualhostname.MachineTarget:
		err = f.validateMachineTarget(destination)
	case virtualhostname.UnitTarget:
		err = f.validateUnitTarget(destination)
	case virtualhostname.ContainerTarget:
		err = f.validateContainerTarget(destination)
	default:
		err = errors.NotValidf("target %q", destination.Target())
	}

	if err != nil {
		result.Error = apiservererrors.ServerError(errors.Annotate(err, "failed to validate destination"))
	}
	return result
}

func (f *Facade) validateMachineTarget(destination virtualhostname.Info) error {
	machineID, _ := destination.Machine()
	ok, err := f.backend.MachineExists(destination.ModelUUID(), strconv.Itoa(machineID))
	if err != nil {
		return err
	}
	if !ok {
		return errors.NotFoundf("machine with ID %d", machineID)
	}
	return nil
}

func (f *Facade) validateUnitTarget(destination virtualhostname.Info) error {
	modelType, err := f.backend.ModelType(destination.ModelUUID())
	if err != nil {
		return err
	}

	if modelType == state.ModelTypeCAAS {
		return errors.NotValidf("missing container name for a K8s unit")
	}

	if modelType != state.ModelTypeIAAS {
		return errors.NotValidf("unknown model type %q", modelType)
	}

	unitName, _ := destination.Unit()
	ok, err := f.backend.UnitExists(destination.ModelUUID(), unitName)
	if err != nil {
		return err
	}
	if !ok {
		return errors.NotFoundf("unit %q", unitName)
	}
	return nil
}

func (f *Facade) validateContainerTarget(destination virtualhostname.Info) error {
	// We don't validate the container name since that's
	// a bit more work and will be handled by the K8s executor.
	unitName, _ := destination.Unit()
	ok, err := f.backend.UnitExists(destination.ModelUUID(), unitName)
	if err != nil {
		return err
	}
	if !ok {
		return errors.NotFoundf("unit %q", unitName)
	}
	return nil
}
