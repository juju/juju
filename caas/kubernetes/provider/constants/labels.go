// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constants

const (
	// Below are keys for juju specific labels.
	LabelOperator      = "juju-operator"
	LabelStorage       = "juju-storage"
	LabelVersion       = "juju-version"
	LabelApplication   = "juju-app"
	LabelModel         = "juju-model"
	LabelModelOperator = "juju-modeloperator"

	// LabelJujuAppCreatedBy is a Juju application label to apply to objects
	// created by applications managed by Juju. Think istio, kubeflow etc
	// See https://bugs.launchpad.net/juju/+bug/1892285
	LabelJujuAppCreatedBy = "app.juju.is/created-by"
)
