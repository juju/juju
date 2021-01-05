// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/charm/v9"
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

var caasOS = set.NewStrings(os.Kubernetes.String())

// ValidateSeries ensures the charm series is valid for the model type.
func ValidateSeries(modelType ModelType, charmSeries string, charmFormat charm.Format) error {
	if charmFormat >= charm.FormatV2 {
		system, err := systems.ParseSystemFromSeries(charmSeries)
		if err != nil {
			return errors.Trace(err)
		}
		if system.Resource != "" {
			switch modelType {
			case CAAS:
				// CAAS models support using a resource as the system.
				return nil
			case IAAS:
				return errors.NotValidf("IAAS models don't support systems referencing a resource")
			}
		}
	} else {
		os, err := series.GetOSFromSeries(charmSeries)
		if err != nil {
			return errors.Trace(err)
		}
		switch modelType {
		case CAAS:
			if !caasOS.Contains(os.String()) {
				return errors.NotValidf("series %q in a kubernetes model", charmSeries)
			}
		case IAAS:
			if caasOS.Contains(os.String()) {
				return errors.NotValidf("series %q in a non container model", charmSeries)
			}
		}
	}
	return nil
}
