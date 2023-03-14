// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/series"
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

// Logger defines the logging methods needed
type SeriesSelectorLogger interface {
	Infof(string, ...interface{})
	Tracef(string, ...interface{})
}

// SeriesSelector is a helper type that determines what series the charm should
// be deployed to.
//
// TODO: This type should really have a Validate method, as the Force flag is
// really only valid if the SeriesFlag is specified. There is code and tests
// that allow the Force flag when series isn't specified, but they should
// really be cleaned up. The `deploy` CLI command has tests to ensure that
// --Force is only valid with --series.
type SeriesSelector struct {
	// SeriesFlag is the series passed to the --series flag on the command line.
	SeriesFlag string
	// CharmURLSeries is the series specified as part of the charm URL, i.e.
	// ch:jammy/ubuntu.
	CharmURLSeries string
	// Conf is the configuration for the model we're deploying to.
	Conf modelConfig
	// SupportedSeries is the list of series the charm supports.
	SupportedSeries []string
	// SupportedJujuSeries is the list of series that juju supports.
	SupportedJujuSeries set.Strings
	// Force indicates the user explicitly wants to deploy to a requested
	// series, regardless of whether the charm says it supports that series.
	Force bool
	// from bundle specifies the deploy request comes from a bundle spec.
	FromBundle bool
	Logger     SeriesSelectorLogger
}

// charmSeries determines what series to use with a charm.
// Order of preference is:
//   - user requested with --series or defined by bundle when deploying
//   - user requested in charm's url (e.g. juju deploy jammy/ubuntu)
//   - model default, if set, acts like --series
//   - default from charm metadata supported series / series in url
//   - default LTS
func (s SeriesSelector) CharmSeries() (selectedSeries string, err error) {
	// TODO(sidecar): handle systems

	// User has requested a series with --series.
	if s.SeriesFlag != "" {
		return s.userRequested(s.SeriesFlag)
	}

	// User specified a series in the charm URL, e.g.
	// juju deploy precise/ubuntu.
	if s.CharmURLSeries != "" {
		return s.userRequested(s.CharmURLSeries)
	}

	// No series explicitly requested by the user.
	// Use model default series, if explicitly set and supported by the charm.
	if defaultBase, explicit := s.Conf.DefaultBase(); explicit {
		base, err := series.ParseBaseFromString(defaultBase)
		if err != nil {
			return "", errors.Trace(err)
		}

		defaultSeries, err := series.GetSeriesFromBase(base)
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
	for _, charmSeries := range s.SupportedSeries {
		if s.SupportedJujuSeries.Contains(charmSeries) {
			supportedSeries = append(supportedSeries, charmSeries)
		}
	}
	defaultSeries, err := SeriesForCharm("", supportedSeries)
	if err == nil {
		return defaultSeries, nil
	}

	// Charm hasn't specified a default (likely due to being a local charm
	// deployed by path). Last chance, best we can do is default to LTS.

	// At this point, because we have no idea what series the charm supports,
	// *everything* requires --Force.
	if !s.Force {
		s.Logger.Tracef("juju supported series %s", s.SupportedJujuSeries.SortedValues())
		s.Logger.Tracef("charm supported series %s", s.SupportedSeries)
		if IsMissingSeriesError(err) && len(s.SupportedSeries) > 0 {
			return "", errors.Errorf("the charm defined series %q not supported", strings.Join(s.SupportedSeries, ", "))
		}

		// We know err is not nil due to above, so return the error
		// returned to us from the charm call.
		return "", err
	}

	latestLTS := version.DefaultSupportedLTS()
	s.Logger.Infof(msgLatestLTSSeries, latestLTS)
	return latestLTS, nil
}

// userRequested checks the series the user has requested, and returns it if it
// is supported, or if they used --Force.
func (s SeriesSelector) userRequested(requestedSeries string) (string, error) {
	// TODO(sidecar): handle computed series
	series, err := SeriesForCharm(requestedSeries, s.SupportedSeries)
	if s.Force {
		series = requestedSeries
	} else if err != nil {
		if IsUnsupportedSeriesError(err) {
			supported := s.SupportedJujuSeries.Intersection(set.NewStrings(s.SupportedSeries...))
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

	// either it's a supported series or the user used --Force, so just
	// give them what they asked for.
	if s.FromBundle {
		s.Logger.Infof(msgBundleSeries, series)
		return series, nil
	}
	s.Logger.Infof(msgUserRequestedSeries, series)
	return series, nil
}

func (s SeriesSelector) validateSeries(seriesName string) error {
	if len(s.SupportedJujuSeries) == 0 {
		// programming error
		return errors.Errorf("expected supported juju series to exist")
	}

	if !s.SupportedJujuSeries.Contains(seriesName) {
		return errors.NotSupportedf("series: %s", seriesName)
	}
	return nil
}
