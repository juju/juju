// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"github.com/juju/charm/v9"
	csparams "github.com/juju/charmrepo/v7/csclient/params"
	"gopkg.in/macaroon.v2"

	apicharm "github.com/juju/juju/api/client/charms"
	commoncharm "github.com/juju/juju/api/common/charm"
)

// CharmAdder defines a subset of the charm client needed to add a
// charm.
type CharmAdder interface {
	AddLocalCharm(*charm.URL, charm.Charm, bool) (*charm.URL, error) // not used in utils
	AddCharm(*charm.URL, commoncharm.Origin, bool) (commoncharm.Origin, error)
	AddCharmWithAuthorization(*charm.URL, commoncharm.Origin, *macaroon.Macaroon, bool) (commoncharm.Origin, error)
	CheckCharmPlacement(string, *charm.URL) error
}

// CharmrepoForDeploy is a stripped-down version of the
// gopkg.in/juju/charmrepo.v4 Interface interface. It is
// used by tests that embed a DeploySuiteBase.
type CharmrepoForDeploy interface {
	GetBundle(bundleURL *charm.URL, path string) (charm.Bundle, error)
	ResolveWithPreferredChannel(*charm.URL, csparams.Channel) (*charm.URL, csparams.Channel, []string, error)
}

// MacaroonGetter defines a subset of a charmstore client,
// as required by different application commands.
type MacaroonGetter interface {
	Get(endpoint string, extra interface{}) error
}

// CharmsAPI is functionality needed by the CharmAdapter from the Charms API.
type CharmsAPI interface {
	ResolveCharms(charms []apicharm.CharmToResolve) ([]apicharm.ResolvedCharm, error)
	GetDownloadInfo(curl *charm.URL, origin commoncharm.Origin, mac *macaroon.Macaroon) (apicharm.DownloadInfo, error)
}
