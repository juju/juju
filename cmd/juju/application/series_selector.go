// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/utils/series"
	"gopkg.in/juju/charm.v6-unstable"
)

const (
	msgUserRequestedSeries = "with the user specified series %q"
	msgBundleSeries        = "with the series %q defined by the bundle"
	msgDefaultCharmSeries  = "with the default charm metadata series %q"
	msgDefaultModelSeries  = "with the configured model default series %q"
	msgLatestLTSSeries     = "with the latest LTS series %q"
)

type modelConfig interface {
	DefaultSeries() (string, bool)
}

// seriesSelector is a helper type that determines what series the charm should
// be deployed to.
type seriesSelector struct {
	// seriesFlag is the series passed to the --series flag on the command line.
	seriesFlag string
	// charmURLSeries is the series specified as part of the charm URL, i.e.
	// cs:trusty/ubuntu.
	charmURLSeries string
	// conf is the configuration for the model we're deploying to.
	conf modelConfig
	// supportedSeries is the list of series the charm supports.
	supportedSeries []string
	// force indicates the user explicitly wants to deploy to a requested
	// series, regardless of whether the charm says it supports that series.
	force bool
	// from bundle specifies the deploy request comes from a bundle spec.
	fromBundle bool
}

// charmSeries determines what series to use with a charm.
// Order of preference is:
// - user requested with --series or defined by bundle when deploying
// - user requested in charm's url (e.g. juju deploy precise/ubuntu)
// - model default (if it matches supported series)
// - default from charm metadata supported series / series in url
// - default LTS
func (s seriesSelector) charmSeries() (selectedSeries string, err error) {
	// User has requested a series with --series.
	if s.seriesFlag != "" {
		return s.userRequested(s.seriesFlag)
	}

	// User specified a series in the charm URL, e.g.
	// juju deploy precise/ubuntu.
	if s.charmURLSeries != "" {
		return s.userRequested(s.charmURLSeries)
	}

	// No series explicitly requested by the user.
	// Use model default series, if explicitly set and supported by the charm.
	if defaultSeries, explicit := s.conf.DefaultSeries(); explicit {
		if isSeriesSupported(defaultSeries, s.supportedSeries) {
			logger.Infof(msgDefaultModelSeries, defaultSeries)
			return defaultSeries, nil
		}
	}

	// Use the charm's perferred series, if it has one.  In a multi-series
	// charm, the first series in the list is the preferred one.
	if len(s.supportedSeries) > 0 {
		logger.Infof(msgDefaultCharmSeries, s.supportedSeries[0])
		return s.supportedSeries[0], nil
	}

	// Charm hasn't specified a default (likely due to being a local charm
	// deployed by path).  Last chance, best we can do is default to LTS.

	// At this point, because we have no idea what series the charm supports,
	// *everything* requires --force.
	if !s.force {
		return "", s.unsupportedSeries(series.LatestLts())
	}

	latestLTS := series.LatestLts()
	logger.Infof(msgLatestLTSSeries, latestLTS)
	return latestLTS, nil
}

// userRequested checks the series the user has requested, and returns it if it
// is supported, or if they used --force.
func (s seriesSelector) userRequested(series string) (selectedSeries string, err error) {
	if !s.force && !isSeriesSupported(series, s.supportedSeries) {
		return "", s.unsupportedSeries(series)
	}

	// either it's a supported series or the user used --force, so just
	// give them what they asked for.
	if s.fromBundle {
		logger.Infof(msgBundleSeries, series)
		return series, nil
	}
	logger.Infof(msgUserRequestedSeries, series)
	return series, nil
}

func (s seriesSelector) unsupportedSeries(series string) error {
	supp := s.supportedSeries
	if len(supp) == 0 {
		supp = []string{"<none defined>"}
	}
	return charm.NewUnsupportedSeriesError(series, supp)
}
