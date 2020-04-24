// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewallrules

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/firewall"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.firewallrules")

// API provides the firewallrules facade APIs for v1.
type API struct {
	backend    Backend
	authorizer facade.Authorizer
	check      BlockChecker
}

// NewFacade provides the signature required for facade registration.
func NewFacade(ctx facade.Context) (*API, error) {
	backend, err := NewStateBackend(ctx.State())
	if err != nil {
		return nil, errors.Annotate(err, "getting state")
	}
	blockChecker := common.NewBlockChecker(ctx.State())
	return NewAPI(
		backend,
		ctx.Auth(),
		blockChecker,
	)
}

// NewAPI returns a new firewallrules API facade.
func NewAPI(
	backend Backend,
	authorizer facade.Authorizer,
	blockChecker BlockChecker,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	return &API{
		backend:    backend,
		authorizer: authorizer,
		check:      blockChecker,
	}, nil
}

func (api *API) checkPermission(tag names.Tag, perm permission.Access) error {
	allowed, err := api.authorizer.HasPermission(perm, tag)
	if err != nil {
		return errors.Trace(err)
	}
	if !allowed {
		return common.ErrPerm
	}
	return nil
}

func (api *API) checkAdmin() error {
	return api.checkPermission(api.backend.ModelTag(), permission.AdminAccess)
}

func (api *API) checkCanRead() error {
	return api.checkPermission(api.backend.ModelTag(), permission.ReadAccess)
}

// SetFirewallRules creates or updates the specified firewall rules.
func (api *API) SetFirewallRules(args params.FirewallRuleArgs) (params.ErrorResults, error) {
	var errResults params.ErrorResults
	if err := api.checkAdmin(); err != nil {
		return errResults, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return errResults, errors.Trace(err)
	}

	results := make([]params.ErrorResult, len(args.Args))
	for i, arg := range args.Args {
		logger.Debugf("saving firewall rule %+v", arg)
		err := api.backend.SaveFirewallRule(state.NewFirewallRule(
			firewall.WellKnownServiceType(arg.KnownService), arg.WhitelistCIDRS))
		results[i].Error = common.ServerError(err)
	}
	errResults.Results = results
	return errResults, nil
}

// ListFirewallRules returns all the firewall rules.
func (api *API) ListFirewallRules() (params.ListFirewallRulesResults, error) {
	var listResults params.ListFirewallRulesResults
	if err := api.checkCanRead(); err != nil {
		return listResults, errors.Trace(err)
	}
	rules, err := api.backend.ListFirewallRules()
	if err != nil {
		return listResults, errors.Trace(err)
	}
	listResults.Rules = make([]params.FirewallRule, len(rules))
	for i, r := range rules {
		listResults.Rules[i] = params.FirewallRule{
			KnownService:   params.KnownServiceValue(r.WellKnownService()),
			WhitelistCIDRS: r.WhitelistCIDRs(),
		}
	}
	return listResults, nil
}
