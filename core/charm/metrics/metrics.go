// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metrics

// MetricKey represents metrics keys collected and sent to charmhub.
type MetricKey string

func (c MetricKey) String() string {
	return string(c)
}

const (
	// Controller is used in RequestMetrics
	Controller MetricKey = "controller"

	// Model is used in RequestMetrics
	Model MetricKey = "model"
)

// MetricValueKey represents metrics value keys collected and sent to charmhub.
type MetricValueKey string

func (c MetricValueKey) String() string {
	return string(c)
}

const (
	//
	// Controller and Model, included in the RefreshRequest Metrics.
	//

	// UUID is the uuid of a model, either controller or model.
	UUID MetricValueKey = "uuid"
	// JujuVersion is the version of juju running in this model.
	JujuVersion MetricValueKey = "juju-version"

	//
	// Model metrics, included in the RefreshRequest Metrics.
	//

	// Provider matches the provider type defined in juju.
	Provider MetricValueKey = "provider"
	// Region is the region this model is operating in.
	Region MetricValueKey = "region"
	// Cloud is the name of the cloud this model is operating in.
	Cloud MetricValueKey = "cloud"
	// NumApplications is the number of applications in the model.
	NumApplications MetricValueKey = "applications"
	// NumMachines is the number of machines in the model.
	NumMachines MetricValueKey = "machines"
	// NumUnits is the number of units in the model.
	NumUnits MetricValueKey = "units"

	//
	// Charm metrics, included in the RefreshRequestContext Metrics.
	//

	// Relations is a common separated list of charms currently related
	// to an application.  (no spaces)
	Relations MetricValueKey = "relations"
)
