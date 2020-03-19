// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"gopkg.in/juju/charm.v6"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/os/series"
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
//
// TODO: This type should really have a Validate method, as the force flag is
// really only valid if the seriesFlag is specified. There is code and tests
// that allow the force flag when series isn't specified, but they should
// really be cleaned up. The `deploy` CLI command has tests to ensure that
// --force is only valid with --series.
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
	// supportedJujuSeries is the list of series that juju supports.
	supportedJujuSeries set.Strings
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
		if _, err := charm.SeriesForCharm(defaultSeries, s.supportedSeries); err == nil {
			// validate the series we get from the charm
			if err := s.validateSeries(defaultSeries); err != nil {
				return "", err
			}
			logger.Infof(msgDefaultModelSeries, defaultSeries)
			return defaultSeries, nil
		}
	}

	// we want to preseve the order of the supported series from the charm
	// metadata, so the order could be out of order ubuntu series order.
	// i.e. precise, xenial, bionic, trusty
	var supportedSeries []string
	for _, charmSeries := range s.supportedSeries {
		if s.supportedJujuSeries.Contains(charmSeries) {
			supportedSeries = append(supportedSeries, charmSeries)
		}
	}
	defaultSeries, err := charm.SeriesForCharm("", supportedSeries)
	if err == nil {
		return defaultSeries, nil
	}

	// Charm hasn't specified a default (likely due to being a local charm
	// deployed by path).  Last chance, best we can do is default to LTS.

	// At this point, because we have no idea what series the charm supports,
	// *everything* requires --force.
	if !s.force {
		// We know err is not nil due to above, so return the error
		// returned to us from the charm call.
		return "", err
	}

	latestLTS := series.DefaultSupportedLTS()
	logger.Infof(msgLatestLTSSeries, latestLTS)
	return latestLTS, nil
}

// userRequested checks the series the user has requested, and returns it if it
// is supported, or if they used --force.
func (s seriesSelector) userRequested(requestedSeries string) (string, error) {
	series, err := charm.SeriesForCharm(requestedSeries, s.supportedSeries)
	if s.force {
		series = requestedSeries
	} else if err != nil {
		return "", err
	}

	// validate the series we get from the charm
	if err := s.validateSeries(series); err != nil {
		return "", err
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

func (s seriesSelector) validateSeries(seriesName string) error {
	// if we're forcing then we don't need the following validation checks.
	if len(s.supportedJujuSeries) == 0 {
		// programming error
		return errors.Errorf("expected supported juju series to exist")
	}
	if s.force {
		return nil
	}

	if !s.supportedJujuSeries.Contains(seriesName) {
		return errors.NotSupportedf("series: %s", seriesName)
	}
	return nil
}
