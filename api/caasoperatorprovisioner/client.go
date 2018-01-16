// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
)

// Client allows access to the CAAS operator provisioner API endpoint.
type Client struct {
	facade base.FacadeCaller
}

// NewClient returns a client used to access the CAAS Operator Provisioner API.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "CAASOperatorProvisioner")
	return &Client{
		facade: facadeCaller,
	}
}

// WatchApplications returns a StringsWatcher that notifies of
// changes to the lifecycles of CAAS applications in the current model.
func (c *Client) WatchApplications() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	if err := c.facade.FacadeCall("WatchApplications", nil, &result); err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// ApplicationPassword holds parameters for setting
// an application password.
type ApplicationPassword struct {
	Name     string
	Password string
}

// SetPasswords sets API passwords for the specified applications.
func (c *Client) SetPasswords(appPasswords []ApplicationPassword) (params.ErrorResults, error) {
	var result params.ErrorResults
	args := params.EntityPasswords{Changes: make([]params.EntityPassword, len(appPasswords))}
	for i, p := range appPasswords {
		args.Changes[i] = params.EntityPassword{
			Tag: names.NewApplicationTag(p.Name).String(), Password: p.Password,
		}
	}
	err := c.facade.FacadeCall("SetPasswords", args, &result)
	if err != nil {
		return params.ErrorResults{}, err
	}
	if len(result.Results) != len(args.Changes) {
		return params.ErrorResults{}, errors.Errorf("expected %d result(s), got %d", len(args.Changes), len(result.Results))
	}
	return result, nil
}

// UpdateUnits updates the state model to reflect the state of the units
// as reported by the cloud.
func (c *Client) UpdateUnits(arg params.UpdateApplicationUnits) error {
	var result params.ErrorResults
	args := params.UpdateApplicationUnitArgs{Args: []params.UpdateApplicationUnits{arg}}
	err := c.facade.FacadeCall("UpdateApplicationsUnits", args, &result)
	if err != nil {
		return errors.Trace(err)
	}
	if len(result.Results) != len(args.Args) {
		return errors.Errorf("expected %d result(s), got %d", len(args.Args), len(result.Results))
	}
	return result.OneError()
}
