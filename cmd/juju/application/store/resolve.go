// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"github.com/juju/charm/v7"
	csparams "github.com/juju/charmrepo/v5/csclient/params"
	"github.com/juju/errors"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.cmd.juju.application.store")

// ResolveCharmFunc is the type of a function that resolves a charm URL
// with an optionally specified preferred channel.
type ResolveCharmFunc func(
	resolveWithChannel func(*charm.URL, csparams.Channel) (*charm.URL, csparams.Channel, []string, error),
	url *charm.URL,
	preferredChannel csparams.Channel,
) (*charm.URL, csparams.Channel, []string, error)

func ResolveCharm(
	resolveWithChannel func(*charm.URL, csparams.Channel) (*charm.URL, csparams.Channel, []string, error),
	url *charm.URL,
	preferredChannel csparams.Channel,
) (*charm.URL, csparams.Channel, []string, error) {
	if url.Schema != "cs" {
		return nil, csparams.NoChannel, nil, errors.Errorf("unknown schema for charm URL %q", url)
	}

	resultURL, channel, supportedSeries, err := resolveWithChannel(url, preferredChannel)
	if err != nil {
		return nil, csparams.NoChannel, nil, errors.Trace(err)
	}
	if resultURL.Series != "" && len(supportedSeries) == 0 {
		supportedSeries = []string{resultURL.Series}
	}
	return resultURL, channel, supportedSeries, nil
}

// ResolveBundleURL tries to interpret maybeBundle as a eharmstorr
// bundle. If it turns out to be a bundle, the resolved URL and
// channel are returned. If it isn't but there wasn't a problem
// checking it, it returns a nil charm URL.
func ResolveBundleURL(cstore URLResolver, maybeBundle string, preferredChannel csparams.Channel) (*charm.URL, csparams.Channel, error) {
	userRequestedURL, err := charm.ParseURL(maybeBundle)
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	// Charm or bundle has been supplied as a URL so we resolve and
	// deploy using the store.
	storeCharmOrBundleURL, channel, _, err := ResolveCharm(cstore.ResolveWithPreferredChannel, userRequestedURL, preferredChannel)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	if storeCharmOrBundleURL.Series != "bundle" {
		logger.Debugf(
			`cannot interpret as charmstore bundle: %v (series) != "bundle"`,
			storeCharmOrBundleURL.Series,
		)
		return nil, "", errors.NotValidf("charmstore bundle %q", maybeBundle)
	}
	return storeCharmOrBundleURL, channel, nil
}

// ResolvedBundle decorates a charm.Bundle instance with a type that implements
// the charm.BundleDataSource interface.
type ResolvedBundle struct {
	parts []*charm.BundleDataPart
}

func NewResolvedBundle(b charm.Bundle) ResolvedBundle {
	return ResolvedBundle{
		parts: []*charm.BundleDataPart{
			{
				Data:        b.Data(),
				PresenceMap: make(charm.FieldPresenceMap),
			},
		},
	}
}

// Parts implements charm.BundleDataSource.
func (rb ResolvedBundle) Parts() []*charm.BundleDataPart {
	return rb.parts
}

// BasePath implements charm.BundleDataSource.
func (ResolvedBundle) BasePath() string {
	return ""
}

// ResolveInclude implements charm.BundleDataSource.
func (ResolvedBundle) ResolveInclude(_ string) ([]byte, error) {
	return nil, errors.NotSupportedf("remote bundle includes")
}
