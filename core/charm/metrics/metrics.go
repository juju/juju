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

	//
	// Controller and Model, included in the RefreshRequest Metrics.
	//

	// UUID is the uuid of a model, either controller or model.
	UUID MetricKey = "uuid"
	// JujuVersion is the version of juju running in this model.
	JujuVersion MetricKey = "juju-version"

	//
	// Model metrics, included in the RefreshRequest Metrics.
	//

	// Provider matches the provider type defined in juju.
	Provider MetricKey = "provider"
	// Region is the region this model is operating in.
	Region MetricKey = "region"
	// Cloud is the name of the cloud this model is operating in.
	Cloud MetricKey = "cloud"
	// NumApplications is the number of applications in the model.
	NumApplications MetricKey = "applications"
	// NumMachines is the number of machines in the model.
	NumMachines MetricKey = "machines"
	// NumUnits is the number of units in the model.
	NumUnits MetricKey = "units"

	//
	// Charm metrics, included in the RefreshRequestContext Metrics.
	//

	// Relations is a common separated list of charms currently related
	// to an application.  (no spaces)
	Relations MetricKey = "relations"
)
