// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/common"
)

type environInstance struct {
	id  instance.Id
	env *environ
}

var _ instance.Instance = (*environInstance)(nil)

func (inst *environInstance) Id() instance.Id {
	return inst.id
}

func (inst *environInstance) Status() string {
	_ = inst.env.getSnapshot()
	return "unknown (not implemented)"
}

func (inst *environInstance) Refresh() error {
	return nil
}

func (inst *environInstance) Addresses() ([]instance.Address, error) {
	_ = inst.env.getSnapshot()
	return nil, errNotImplemented
}

func (inst *environInstance) DNSName() (string, error) {
	// This method is likely to be replaced entirely by Addresses() at some point,
	// but remains necessary for now. It's probably smart to implement it in
	// terms of Addresses above, to minimise churn when it's removed.
	_ = inst.env.getSnapshot()
	return "", errNotImplemented
}

func (inst *environInstance) WaitDNSName() (string, error) {
	// This method is likely to be replaced entirely by Addresses() at some point,
	// but remains necessary for now. Until it's finally removed, you can probably
	// ignore this method; the common implementation should work.
	return common.WaitDNSName(inst)
}
