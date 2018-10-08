// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/core/lxdprofile"

	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"
)

// ValidateLXDProfileCharm implements the DeployStep interface.
type ValidateLXDProfileCharm struct{}

// SetFlags implements DeployStep.
func (r *ValidateLXDProfileCharm) SetFlags(f *gnuflag.FlagSet) {
}

// SetPlanURL implements DeployStep.
func (r *ValidateLXDProfileCharm) SetPlanURL(planURL string) {
	// noop
}

// RunPre obtains authorization to deploy this charm. The authorization, if received is not
// sent to the controller, rather it is kept as an attribute on RegisterMeteredCharm.
func (r *ValidateLXDProfileCharm) RunPre(api DeployStepAPI, bakeryClient *httpbakery.Client, ctx *cmd.Context, deployInfo DeploymentInfo) error {
	// if the charm info is not empty, we should use that to validate the
	// lxd profile.
	if charmInfo := deployInfo.CharmInfo; charmInfo != nil {
		if err := lxdprofile.ValidateCharmInfoLXDProfile(charmInfo); err != nil {
			// The force flag was provided, but we should let the user know that
			// this could deliver some unexpected results.
			if deployInfo.Force {
				logger.Debugf("force flag used to override validation error %v", err)
				return nil
			}
			return errors.Trace(err)
		}
	}
	return nil
}

// RunPost sends credentials obtained during the call to RunPre to the controller.
func (r *ValidateLXDProfileCharm) RunPost(api DeployStepAPI, bakeryClient *httpbakery.Client, ctx *cmd.Context, deployInfo DeploymentInfo, prevErr error) error {
	return nil
}
