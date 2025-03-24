// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constants

const (
	// LabelJujuAppCreatedBy is a Juju application label to apply to objects
	// created by applications managed by Juju. Think istio, kubeflow etc
	// See https://bugs.launchpad.net/juju/+bug/1892285
	LabelJujuAppCreatedBy = "app.juju.is/created-by"

	// LabelKubernetesAppName is the common meta key for kubernetes app names.
	// See https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/#labels
	LabelKubernetesAppName = "app.kubernetes.io/name"

	// LabelKubernetesAppManaged is the common meta key for kubernetes apps
	// that are managed by a non k8s process (such as Juju).
	// See https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/#labels
	LabelKubernetesAppManaged = "app.kubernetes.io/managed-by"

	// LabelJujuModelOperatorDisableWebhook is the label used for bypassing
	// model admission validation and mutation on objects.
	LabelJujuModelOperatorDisableWebhook = "model.juju.is/disable-webhook"

	// LabelJujuModelName is the juju label applied for juju models.
	LabelJujuModelName = "model.juju.is/name"

	// LabelJujuOperatorName is the juju label applied to Juju operators to
	// identify their name. Operator names are generally named after the thing
	// the operator is controlling. i.e an operator name for a model test would be
	// "test"
	LabelJujuOperatorName = "operator.juju.is/name"

	// LabelJujuOperatorTarget is the juju label applied to Juju operators to
	// describe the modeling paradigm they target. For example model,
	// application
	LabelJujuOperatorTarget = "operator.juju.is/target"

	// LabelJujuStorageName is the juju label applied to Juju storage objects to
	// describe their name.
	LabelJujuStorageName = "storage.juju.is/name"

	// LegacyLabelKubernetesAppName is the legacy label key used for juju app
	// identification. This purely exists to maintain backwards functionality.
	// See https://bugs.launchpad.net/juju/+bug/1888513
	LegacyLabelKubernetesAppName = "juju-app"

	// LegacyLabelModelName is the legacy label key used for juju models. This
	// purely exists to maintain backwards functionality.
	// See https://bugs.launchpad.net/juju/+bug/1888513
	LegacyLabelModelName = "juju-model"

	// LegacyLabelModelOperator is the legacy label key used for juju model
	// operators. This purely exists to maintain backwards functionality.
	// See https://bugs.launchpad.net/juju/+bug/1888513
	LegacyLabelModelOperator = "juju-modeloperator"

	// LegacyLabelKubernetesOperatorName is the legacy label key used for juju
	// operators. This purely exists to maintain backwards functionality.
	// See https://bugs.launchpad.net/juju/+bug/1888513
	LegacyLabelKubernetesOperatorName = "juju-operator"

	// LegacyLabelStorageName is the legacy label key used for juju storage
	// pvc. This purely exists to maintain backwards functionality.
	// See https://bugs.launchpad.net/juju/+bug/1888513
	LegacyLabelStorageName = "juju-storage"
)
