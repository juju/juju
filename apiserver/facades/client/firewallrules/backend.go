// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewallrules

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/state"
)

// Backend defines the state functionality required by the firewallrules
// facade. For details on the methods, see the methods on state.State
// with the same names.
type Backend interface {
	ModelTag() names.ModelTag
	SaveFirewallRule(state.FirewallRule) error
	ListFirewallRules() ([]*state.FirewallRule, error)
}

// BlockChecker defines the block-checking functionality required by
// the firewallrules facade. This is implemented by
// apiserver/common.BlockChecker.
type BlockChecker interface {
	ChangeAllowed() error
}

type stateShim struct {
	*state.State
	*state.IAASModel
}

// NewStateBackend converts a state.State into a Backend.
func NewStateBackend(st *state.State) (Backend, error) {
	im, err := st.IAASModel()
	if err != nil {
		return nil, err
	}
	return &stateShim{
		State:     st,
		IAASModel: im,
	}, nil
}

func (s stateShim) SaveFirewallRule(rule state.FirewallRule) error {
	api := state.NewFirewallRules(s.State)
	return api.Save(rule)
}

func (s stateShim) ListFirewallRules() ([]*state.FirewallRule, error) {
	api := state.NewFirewallRules(s.State)
	return api.AllRules()
}
