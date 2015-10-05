// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"

	"github.com/juju/juju/container/lxd/lxd_client"
	"github.com/juju/juju/provider/common"
)

type rawProvider struct {
	lxdInstances
	common.Firewaller
}

type lxdInstances interface {
	Instances(string, ...string) ([]lxd_client.Instance, error)
	AddInstance(lxd_client.InstanceSpec) (*lxd_client.Instance, error)
	RemoveInstances(string, ...string) error
}

func newRawProvider(ecfg *environConfig) (*rawProvider, error) {
	client, err := newClient(ecfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	firewaller, err := newFirewaller(ecfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	raw := &rawProvider{
		lxdInstances: client,
		Firewaller:   firewaller,
	}
	return raw, nil
}

func newClient(ecfg *environConfig) (*lxd_client.Client, error) {
	clientCfg := ecfg.clientConfig()

	client, err := lxd_client.Connect(clientCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return client, nil
}

func newFirewaller(ecfg *environConfig) (common.Firewaller, error) {
	return common.NewFirewaller(), nil
}
