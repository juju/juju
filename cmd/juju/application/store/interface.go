// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"context"

	apicharm "github.com/juju/juju/api/client/charms"
	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/internal/charm"
)

// CharmAdder defines a subset of the charm client needed to add a
// charm.
type CharmAdder interface {
	AddLocalCharm(context.Context, *charm.URL, charm.Charm, bool) (*charm.URL, error) // not used in utils
	AddCharm(context.Context, *charm.URL, commoncharm.Origin, bool) (commoncharm.Origin, error)
}

// CharmsAPI is functionality needed by the CharmAdaptor from the Charms API.
type CharmsAPI interface {
	ResolveCharms(ctx context.Context, charms []apicharm.CharmToResolve) ([]apicharm.ResolvedCharm, error)
	GetDownloadInfo(ctx context.Context, curl *charm.URL, origin commoncharm.Origin) (apicharm.DownloadInfo, error)
}
