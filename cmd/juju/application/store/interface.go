// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"github.com/juju/charm/v12"

	apicharm "github.com/juju/juju/api/client/charms"
	commoncharm "github.com/juju/juju/api/common/charm"
)

// CharmAdder defines a subset of the charm client needed to add a
// charm.
type CharmAdder interface {
	AddLocalCharm(*charm.URL, charm.Charm, bool) (*charm.URL, error) // not used in utils
	AddCharm(*charm.URL, commoncharm.Origin, bool) (commoncharm.Origin, error)
	CheckCharmPlacement(string, *charm.URL) error
}

// CharmsAPI is functionality needed by the CharmAdapter from the Charms API.
type CharmsAPI interface {
	ResolveCharms(charms []apicharm.CharmToResolve) ([]apicharm.ResolvedCharm, error)
	GetDownloadInfo(curl *charm.URL, origin commoncharm.Origin) (apicharm.DownloadInfo, error)
}
