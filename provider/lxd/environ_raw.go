// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/errors"

	"github.com/juju/juju/container/lxd/lxdclient"
	"github.com/juju/juju/provider/common"
)

type rawProvider struct {
	lxdInstances
	common.Firewaller
	policyProvider
}

type lxdInstances interface {
	Instances(string, ...string) ([]lxdclient.Instance, error)
	AddInstance(lxdclient.InstanceSpec) (*lxdclient.Instance, error)
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

	policy := &lxdPolicyProvider{}

	raw := &rawProvider{
		lxdInstances:   client,
		Firewaller:     firewaller,
		policyProvider: policy,
	}
	return raw, nil
}

func newClient(ecfg *environConfig) (*lxdclient.Client, error) {
	clientCfg, err := ecfg.clientConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := clientCfg.Write(); err != nil {
		return nil, errors.Trace(err)
	}

	client, err := lxdclient.Connect(clientCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return client, nil
}

func newFirewaller(ecfg *environConfig) (common.Firewaller, error) {
	return common.NewFirewaller(), nil
}
