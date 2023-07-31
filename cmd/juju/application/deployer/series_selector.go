// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/version"
)

const (
	msgUserRequestedSeries = "with the user specified series %q"
	msgBundleSeries        = "with the series %q defined by the bundle"
	msgLatestLTSSeries     = "with the latest LTS series %q"
)

type modelConfig interface {
	// DefaultBase returns the configured default base
	// for the environment, and whether the default base was
	// explicitly configured on the environment.
	DefaultBase() (string, bool)
}

// seriesSelector is a helper type that determines what series the charm should
// be deployed to.
//
// TODO: This type should really have a Validate method, as the force flag is
// really only valid if the seriesFlag is specified. There is code and tests
// that allow the force flag when series isn't specified, but they should
// really be cleaned up. The `deploy` CLI command has tests to ensure that
// --force is only valid with --corebase.
type seriesSelector struct {
	// seriesFlag is the series passed to the --series flag on the command line.
	seriesFlag string
	// charmURLSeries is the series specified as part of the charm URL, i.e.
	// ch:jammy/ubuntu.
	charmURLSeries string
	// conf is the configuration for the model we're deploying to.
	conf modelConfig
	// supportedSeries is the list of series the charm supports.
	supportedSeries []string
	// supportedJujuSeries is the list of series that juju supports.
	supportedJujuSeries set.Strings
	// force indicates the user explicitly wants to deploy to a requested
	// series, regardless of whether the charm says it supports that corebase.
	force bool
	// from bundle specifies the deploy request comes from a bundle spec.
	fromBundle bool
}

// charmSeries determines what series to use with a charm.
// Order of preference is:
//   - user requested with --series or defined by bundle when deploying
//   - user requested in charm's url (e.g. juju deploy jammy/ubuntu)
//   - model default, if set, acts like --series
//   - default from charm metadata supported series / series in url
//   - default LTS
func (s seriesSelector) charmSeries() (selectedSeries string, err error) {
	// TODO(sidecar): handle systems

	// User has requested a series with --corebase.
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
	if defaultBase, explicit := s.conf.DefaultBase(); explicit {
		base, err := corebase.ParseBaseFromString(defaultBase)
		if err != nil {
			return "", errors.Trace(err)
		}

		defaultSeries, err := corebase.GetSeriesFromBase(base)
		if err != nil {
			return "", errors.Trace(err)
		}
		return s.userRequested(defaultSeries)
	}

	// Next fall back to the charm's list of series, filtered to what's supported
	// by Juju. Preserve the order of the supported series from the charm
	// metadata, as the order could be out of order compared to Ubuntu series
	// order (precise, xenial, bionic, trusty, etc).
	var supportedSeries []string
	for _, charmSeries := range s.supportedSeries {
		if s.supportedJujuSeries.Contains(charmSeries) {
			supportedSeries = append(supportedSeries, charmSeries)
		}
	}
	defaultSeries, err := corecharm.SeriesForCharm("", supportedSeries)
	if err == nil {
		return defaultSeries, nil
	}

	// Charm hasn't specified a default (likely due to being a local charm
	// deployed by path). Last chance, best we can do is default to LTS.

	// At this point, because we have no idea what series the charm supports,
	// *everything* requires --force.
	if !s.force {
		logger.Tracef("juju supported series %s", s.supportedJujuSeries.SortedValues())
		logger.Tracef("charm supported series %s", s.supportedSeries)
		if corecharm.IsMissingSeriesError(err) && len(s.supportedSeries) > 0 {
			return "", errors.Errorf("the charm defined series %q not supported", strings.Join(s.supportedSeries, ", "))
		}

		// We know err is not nil due to above, so return the error
		// returned to us from the charm call.
		return "", err
	}

	latestLTS := version.DefaultSupportedLTS()
	logger.Infof(msgLatestLTSSeries, latestLTS)
	return latestLTS, nil
}

// userRequested checks the series the user has requested, and returns it if it
// is supported, or if they used --force.
func (s seriesSelector) userRequested(requestedSeries string) (string, error) {
	// TODO(sidecar): handle computed series
	series, err := corecharm.SeriesForCharm(requestedSeries, s.supportedSeries)
	if s.force {
		series = requestedSeries
	} else if err != nil {
		if corecharm.IsUnsupportedSeriesError(err) {
			supported := s.supportedJujuSeries.Intersection(set.NewStrings(s.supportedSeries...))
			if supported.IsEmpty() {
				return "", errors.NewNotSupported(nil, fmt.Sprintf("series: %s", requestedSeries))
			}
			return "", errors.Errorf(
				"series %q is not supported, supported series are: %s",
				requestedSeries, strings.Join(supported.SortedValues(), ","),
			)
		}
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
	if len(s.supportedJujuSeries) == 0 {
		// programming error
		return errors.Errorf("expected supported juju series to exist")
	}

	if !s.supportedJujuSeries.Contains(seriesName) {
		return errors.NotSupportedf("series: %s", seriesName)
	}
	return nil
}
