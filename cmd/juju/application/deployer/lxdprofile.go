// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"github.com/juju/charm/v13"

	apicharms "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/core/lxdprofile"
)

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

// ValidateLXDProfileCharm implements the DeployStep interface.
type ValidateLXDProfileCharm struct{}

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
