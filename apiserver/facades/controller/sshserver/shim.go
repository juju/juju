// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"github.com/juju/errors"
	jujussh "github.com/juju/utils/v3/ssh"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/state"
)

type backend struct {
	*state.StatePool
}

// ControllerConfig gets the controller config from the systemState.
func (b backend) ControllerConfig() (controller.Config, error) {
	systemState, err := b.StatePool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return systemState.ControllerConfig()
}

// ControllerConfig gets the ssh server hostkey from the systemState.
func (b backend) SSHServerHostKey() (string, error) {
	systemState, err := b.StatePool.SystemState()
	if err != nil {
		return "", errors.Trace(err)
	}
	return systemState.SSHServerHostKey()
}

// WatchControllerConfig gets the controller config watcher from the systemState.
func (b backend) WatchControllerConfig() (state.NotifyWatcher, error) {
	systemState, err := b.StatePool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return systemState.WatchControllerConfig(), nil
}

// HostKeyForVirtualHostname gets the host key for a virtual hostname using the model state.
func (b backend) HostKeyForVirtualHostname(info virtualhostname.Info) ([]byte, error) {
	model, poolHelper, err := b.StatePool.GetModel(info.ModelUUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer poolHelper.Release()
	key, err := model.State().HostKeyForVirtualHostname(info)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return key.HostKey(), nil
}

// AuthorizedKeysForModel collects the authorized keys given a model uuid.
func (b backend) AuthorizedKeysForModel(uuid string) ([]string, error) {
	model, p, err := b.GetModel(uuid)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer p.Release()
	cfg, err := model.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}
	keys := jujussh.SplitAuthorisedKeys(cfg.AuthorizedKeys())
	return keys, nil
}
