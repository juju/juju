// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package seriesselector

import (
	"github.com/juju/charm/v8"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/version"
)

// CharmSeriesArgs holds the arguments passed to CharmSeries.
//
// TODO: This type should really have a Validate method, as the force flag is
// really only valid if the SeriesFlag is specified. There is code and tests
// that allow the force flag when series isn't specified, but they should
// really be cleaned up. The `deploy` CLI command has tests to ensure that
// --force is only valid with --series.
type CharmSeriesArgs struct {
	// SeriesFlag is the series passed to the --series flag on the command line.
	SeriesFlag string

	// CharmURLSeries is the series specified as part of the charm URL, i.e.
	// cs:trusty/ubuntu.
	CharmURLSeries string

	// Config is the configuration for the model we're deploying to.
	Config ModelConfig

	// SupportedSeries is the list of series the charm supports.
	SupportedSeries []string

	// SupportedJujuSeries is the list of series that Juju supports.
	SupportedJujuSeries set.Strings

	// Force indicates the user explicitly wants to deploy to a requested
	// series, regardless of whether the charm says it supports that series.
	Force bool

	// From bundle specifies the deploy request comes from a bundle spec.
	FromBundle bool

	// Logger is the logger to log to.
	Logger Logger
}

// ModelConfig is the subset of config.Config required by this package.
type ModelConfig interface {
	// DefaultSeries returns the configured default Ubuntu series
	// for the environment, and whether the default series was
	// explicitly configured on the environment.
	DefaultSeries() (string, bool)
}

// Logger is the logger interface required by this package.
type Logger interface {
	Infof(format string, params ...interface{})
}

// CharmSeries determines what series to use with a charm.
// Order of preference is:
// - user requested with --series or defined by bundle when deploying
// - user requested in charm's url (e.g. juju deploy precise/ubuntu)
// - model default (if it matches supported series)
// - default from charm metadata supported series / series in url
// - default LTS
func CharmSeries(args CharmSeriesArgs) (selectedSeries string, err error) {
	// TODO(sidecar): handle systems

	// User has requested a series with --series.
	if args.SeriesFlag != "" {
		return userRequested(args, args.SeriesFlag)
	}

	// User specified a series in the charm URL, e.g.
	// juju deploy precise/ubuntu.
	if args.CharmURLSeries != "" {
		return userRequested(args, args.CharmURLSeries)
	}

	// No series explicitly requested by the user.
	// Use model default series, if explicitly set and supported by the charm.
	if defaultSeries, explicit := args.Config.DefaultSeries(); explicit {
		if _, err := charm.SeriesForCharm(defaultSeries, args.SupportedSeries); err == nil {
			// validate the series we get from the charm
			if err := validateSeries(args, defaultSeries); err != nil {
				return "", err
			}
			args.Logger.Infof("with the configured model default series %q", defaultSeries)
			return defaultSeries, nil
		}
	}

	// We want to preserve the order of the supported series from the charm
	// metadata, so the order could be out of order ubuntu series order.
	// i.e. precise, xenial, bionic, trusty
	var supportedSeries []string
	for _, charmSeries := range args.SupportedSeries {
		if args.SupportedJujuSeries.Contains(charmSeries) {
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
	if !args.Force {
		// We know err is not nil due to above, so return the error
		// returned to us from the charm call.
		return "", err
	}

	latestLTS := version.DefaultSupportedLTS()
	args.Logger.Infof("with the latest LTS series %q", latestLTS)
	return latestLTS, nil
}

// userRequested checks the series the user has requested, and returns it if it
// is supported, or if they used --force.
func userRequested(args CharmSeriesArgs, requestedSeries string) (string, error) {
	// TODO(sidecar): handle computed series
	series, err := charm.SeriesForCharm(requestedSeries, args.SupportedSeries)
	if args.Force {
		series = requestedSeries
	} else if err != nil {
		return "", err
	}

	// validate the series we get from the charm
	if err := validateSeries(args, series); err != nil {
		return "", err
	}

	// either it's a supported series or the user used --force, so just
	// give them what they asked for.
	if args.FromBundle {
		args.Logger.Infof("with the series %q defined by the bundle", series)
		return series, nil
	}
	args.Logger.Infof("with the user specified series %q", series)
	return series, nil
}

func validateSeries(args CharmSeriesArgs, seriesName string) error {
	// if we're forcing then we don't need the following validation checks.
	if len(args.SupportedJujuSeries) == 0 {
		// programming error
		return errors.Errorf("expected supported juju series to exist")
	}
	if args.Force {
		return nil
	}

	if !args.SupportedJujuSeries.Contains(seriesName) {
		return errors.NotSupportedf("series: %s", seriesName)
	}
	return nil
}
