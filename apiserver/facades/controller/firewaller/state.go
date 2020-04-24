// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"github.com/juju/names/v4"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/common/firewall"
	corefirewall "github.com/juju/juju/core/firewall"
	"github.com/juju/juju/state"
)

// State provides the subset of global state required by the
// remote firewaller facade.
type State interface {
	firewall.State

	ModelUUID() string

	GetMacaroon(entity names.Tag) (*macaroon.Macaroon, error)

	WatchOpenedPorts() state.StringsWatcher

	FindEntity(tag names.Tag) (state.Entity, error)

	FirewallRule(service corefirewall.WellKnownServiceType) (*state.FirewallRule, error)

	Subnet(id string) (Subnet, error)

	SubnetByCIDR(cidr string) (Subnet, error)
}

// TODO(wallyworld) - for tests, remove when remaining firewaller tests become unit tests.
func StateShim(st *state.State, m *state.Model) stateShim {
	return stateShim{st: st, State: firewall.StateShim(st, m)}
}

type stateShim struct {
	firewall.State
	st *state.State
}

func (st stateShim) ModelUUID() string {
	return st.st.ModelUUID()
}

func (st stateShim) GetMacaroon(entity names.Tag) (*macaroon.Macaroon, error) {
	r := st.st.RemoteEntities()
	return r.GetMacaroon(entity)
}

func (st stateShim) FindEntity(tag names.Tag) (state.Entity, error) {
	return st.st.FindEntity(tag)
}

func (st stateShim) WatchOpenedPorts() state.StringsWatcher {
	return st.st.WatchOpenedPorts()
}

func (st stateShim) FirewallRule(service corefirewall.WellKnownServiceType) (*state.FirewallRule, error) {
	api := state.NewFirewallRules(st.st)
	return api.Rule(service)
}

type Subnet interface {
	ID() string
	CIDR() string
}

func (st stateShim) Subnet(id string) (Subnet, error) {
	return st.st.Subnet(id)
}

func (st stateShim) SubnetByCIDR(cidr string) (Subnet, error) {
	return st.st.SubnetByCIDR(cidr)
}
