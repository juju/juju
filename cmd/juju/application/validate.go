// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	charm "github.com/juju/charm/v7"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/macaroon-bakery.v2/httpbakery"

	apicharms "github.com/juju/juju/api/charms"
	"github.com/juju/juju/core/lxdprofile"
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
		if err := lxdprofile.ValidateLXDProfile(lxdCharmInfoProfiler{
			CharmInfo: charmInfo,
		}); err != nil {
			// The force flag was provided, but we should let the user know that
			// this could deliver some unexpected results.
			if deployInfo.Force {
				logger.Debugf("force option used to override validation error %v", err)
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

// lxdCharmProfiler massages a charm.Charm into a LXDProfiler inside of the
// core package.
type lxdCharmProfiler struct {
	Charm charm.Charm
}

// LXDProfile implements core.lxdprofile.LXDProfiler
func (p lxdCharmProfiler) LXDProfile() lxdprofile.LXDProfile {
	if p.Charm == nil {
		return nil
	}
	if profiler, ok := p.Charm.(charm.LXDProfiler); ok {
		profile := profiler.LXDProfile()
		if profile == nil {
			return nil
		}
		return profile
	}
	return nil
}

// lxdCharmInfoProfiler massages a *apicharms.CharmInfo into a LXDProfiler
// inside of the core package.
type lxdCharmInfoProfiler struct {
	CharmInfo *apicharms.CharmInfo
}

// LXDProfile implements core.lxdprofile.LXDProfiler
func (p lxdCharmInfoProfiler) LXDProfile() lxdprofile.LXDProfile {
	if p.CharmInfo == nil || p.CharmInfo.LXDProfile == nil {
		return nil
	}
	return p.CharmInfo.LXDProfile
}
