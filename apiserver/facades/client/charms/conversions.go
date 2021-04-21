// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/charm/v9"

	"github.com/juju/juju/apiserver/params"
	corecharm "github.com/juju/juju/core/charm"
)

func convertOrigin(origin corecharm.Origin) params.CharmOrigin {
	var track *string
	if origin.Channel != nil && origin.Channel.Track != "" {
		track = &origin.Channel.Track
	}
	var risk string
	if origin.Channel != nil {
		risk = string(origin.Channel.Risk)
	}
	return params.CharmOrigin{
		Source:       string(origin.Source),
		Type:         origin.Type,
		ID:           origin.ID,
		Hash:         origin.Hash,
		Risk:         risk,
		Revision:     origin.Revision,
		Track:        track,
		Architecture: origin.Platform.Architecture,
		OS:           origin.Platform.OS,
		Series:       origin.Platform.Series,
	}
}

func convertParamsOrigin(origin params.CharmOrigin) corecharm.Origin {
	var track string
	if origin.Track != nil {
		track = *origin.Track
	}
	return corecharm.Origin{
		Source:   corecharm.Source(origin.Source),
		Type:     origin.Type,
		ID:       origin.ID,
		Hash:     origin.Hash,
		Revision: origin.Revision,
		Channel: &charm.Channel{
			Track: track,
			Risk:  charm.Risk(origin.Risk),
		},
		Platform: corecharm.Platform{
			Architecture: origin.Architecture,
			OS:           origin.OS,
			Series:       origin.Series,
		},
	}
}
