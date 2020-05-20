// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"github.com/juju/charm/v7"
	csparams "github.com/juju/charmrepo/v5/csclient/params"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/charms"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/resource"
)

// CharmClient defines a subset of the charms facade, as required
// by the upgrade-charm command and to GetMetaResources.
type CharmClient interface {
	CharmInfo(string) (*charms.CharmInfo, error)
}

// CharmAdder defines a subset of the api client needed to add a
// charm.
type CharmAdder interface {
	AddLocalCharm(*charm.URL, charm.Charm, bool) (*charm.URL, error) // not used in utils
	AddCharm(*charm.URL, csparams.Channel, bool) error
	AddCharmWithAuthorization(*charm.URL, csparams.Channel, *macaroon.Macaroon, bool) error
}

// charmrepoForDeploy is a stripped-down version of the
// gopkg.in/juju/charmrepo.v4 Interface interface. It is
// used by tests that embed a DeploySuiteBase.
type CharmrepoForDeploy interface {
	Get(charmURL *charm.URL, path string) (*charm.CharmArchive, error)
	GetBundle(bundleURL *charm.URL, path string) (charm.Bundle, error)
	ResolveWithPreferredChannel(*charm.URL, csparams.Channel) (*charm.URL, csparams.Channel, []string, error)
}

// MacaroonGetter defines a subset of a charmstore client,
// as required by different application commands.
type MacaroonGetter interface {
	Get(endpoint string, extra interface{}) error
}

// ModelExtractor provides everything we need to build a
// bundlechanges.Model from a model API connection.
type ModelExtractor interface {
	GetAnnotations(tags []string) ([]params.AnnotationsGetResult, error)
	GetConstraints(applications ...string) ([]constraints.Value, error)
	GetConfig(branchName string, applications ...string) ([]map[string]interface{}, error)
	Sequences() (map[string]int, error)
}

// ResourceLister defines a subset of the resources facade, as required
// by the upgrade-charm command and to deploy bundles.
type ResourceLister interface {
	ListResources([]string) ([]resource.ApplicationResources, error)
}

// URLResolver is the part of charmrepo.Charmstore that we need to
// resolve a charm url.
type URLResolver interface {
	ResolveWithPreferredChannel(*charm.URL, csparams.Channel) (*charm.URL, csparams.Channel, []string, error)
}
