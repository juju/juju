// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/charm/v8"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/os/v2"
	"github.com/juju/os/v2/series"
	"github.com/juju/systems"
)

// ModelType indicates a model type.
type ModelType string

const (
	// IAAS is the type for IAAS models.
	IAAS ModelType = "iaas"

	// CAAS is the type for CAAS models.
	CAAS ModelType = "caas"
)

// String returns m as a string.
func (m ModelType) String() string {
	return string(m)
}

// Model represents the state of a model.
type Model struct {
	// Name returns the human friendly name of the model.
	Name string

	// UUID is the universally unique identifier of the model.
	UUID string

	// ModelType is the type of model.
	ModelType ModelType
}

// ValidateSeriesArgs holds the arguments for the ValidateSeries method
type ValidateSeriesArgs struct {
	ModelType ModelType
	Name      string
	Series    string
	Format    charm.Format
}

var caasOS = set.NewStrings(os.Kubernetes.String())

// ValidateSeries ensures the charm series is valid for the model type.
func ValidateSeries(args ValidateSeriesArgs) error {
	if args.Format >= charm.FormatV2 {
		system, err := systems.ParseSystemFromSeries(args.Series)
		if err != nil {
			return errors.Trace(err)
		}

		if system.Resource == "" {
			return nil
		}

		switch args.ModelType {
		// CAAS models support using a resource as the system.
		case CAAS:
			return nil
		case IAAS:
			return errors.NotValidf("IAAS models don't support systems referencing a resource")
		}

		return nil
	}

	os, err := series.GetOSFromSeries(args.Series)
	if err != nil {
		return errors.Trace(err)
	}
	switch args.ModelType {
	case CAAS:
		if !caasOS.Contains(os.String()) {
			return errors.NotValidf(
				"%q is not a container charm",
				args.Name,
			)
		}
	case IAAS:
		if caasOS.Contains(os.String()) {
			return errors.NotValidf(
				"%q is not an IAAS charm",
				args.Name,
			)
		}
	}

	return nil
}
