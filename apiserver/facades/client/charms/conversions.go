// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/charm/v13"
	"github.com/juju/errors"

	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/rpc/params"
)

func convertOrigin(origin corecharm.Origin) (params.CharmOrigin, error) {
	var track *string
	if origin.Channel != nil && origin.Channel.Track != "" {
		track = &origin.Channel.Track
	}
	var branch *string
	if origin.Channel != nil && origin.Channel.Branch != "" {
		branch = &origin.Channel.Branch
	}
	var risk string
	if origin.Channel != nil {
		risk = string(origin.Channel.Risk)
	}
	var base corebase.Base
	if origin.Platform.Channel != "" {
		var err error
		base, err = corebase.ParseBase(origin.Platform.OS, origin.Platform.Channel)
		if err != nil {
			return params.CharmOrigin{}, errors.Trace(err)
		}
	}
	return params.CharmOrigin{
		Source:       string(origin.Source),
		Type:         origin.Type,
		ID:           origin.ID,
		Hash:         origin.Hash,
		Risk:         risk,
		Revision:     origin.Revision,
		Track:        track,
		Branch:       branch,
		Architecture: origin.Platform.Architecture,
		Base:         params.Base{Name: base.OS, Channel: base.Channel.String()},
		InstanceKey:  origin.InstanceKey,
	}, nil
}

// ConvertParamsOrigin converts a params struct to a core charm origin.
func ConvertParamsOrigin(origin params.CharmOrigin) (corecharm.Origin, error) {
	var track string
	if origin.Track != nil {
		track = *origin.Track
	}
	var branch string
	if origin.Branch != nil {
		branch = *origin.Branch
	}

	base, err := corebase.ParseBase(origin.Base.Name, origin.Base.Channel)
	if err != nil {
		return corecharm.Origin{}, errors.Trace(err)
	}
	return corecharm.Origin{
		Source:   corecharm.Source(origin.Source),
		Type:     origin.Type,
		ID:       origin.ID,
		Hash:     origin.Hash,
		Revision: origin.Revision,
		Channel: &charm.Channel{
			Track:  track,
			Risk:   charm.Risk(origin.Risk),
			Branch: branch,
		},
		Platform: corecharm.Platform{
			Architecture: origin.Architecture,
			OS:           base.OS,
			Channel:      base.Channel.Track,
		},
		InstanceKey: origin.InstanceKey,
	}, nil
}
