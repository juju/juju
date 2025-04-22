// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
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
