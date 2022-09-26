// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/charm/v8"
	"github.com/juju/errors"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/series"
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
	var chSeries string
	if origin.Platform.Channel != "" {
		var err error
		chSeries, err = series.VersionSeries(origin.Platform.Channel)
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
		OS:           origin.Platform.OS,
		Channel:      origin.Platform.Channel,
		// TODO(juju3) - remove series
		Series:      chSeries,
		InstanceKey: origin.InstanceKey,
	}, nil
}

func convertParamsOrigin(origin params.CharmOrigin) corecharm.Origin {
	var track string
	if origin.Track != nil {
		track = *origin.Track
	}
	var branch string
	if origin.Branch != nil {
		branch = *origin.Branch
	}
	if origin.Channel == "" && origin.Series != "" {
		origin.Channel, _ = series.VersionSeries(origin.Series)
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
			OS:           origin.OS,
			Channel:      origin.Channel,
		},
		InstanceKey: origin.InstanceKey,
	}
}
