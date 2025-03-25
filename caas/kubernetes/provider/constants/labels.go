// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constants

// LabelVersion represents which version of labelling a model
// uses.
type LabelVersion int

const (
	LegacyLabelVersion LabelVersion = iota
	// LabelVersion1 introduces domain based labelling
	// scheme for juju resources in kubernetes.
	LabelVersion1
	// LabelVersion2 introduces model and controller uuid
	// labelling to disambiguate cluster scoped resources.
	LabelVersion2

	labelVersionMax
)
const (
	// FirstLabelVersion is the first supported label version.
	FirstLabelVersion LabelVersion = LegacyLabelVersion
	// LastLabelVersion is the last supported label version.
	LastLabelVersion = labelVersionMax - 1
)

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
	// Since LabelVersion1.
	LabelJujuModelOperatorDisableWebhook = "model.juju.is/disable-webhook"

	// LabelJujuModelName is the juju label applied for juju models.
	// Since LabelVersion1.
	LabelJujuModelName = "model.juju.is/name"

	// LabelJujuModelUUID is the juju label applied for juju models.
	// Since LabelVersion2.
	LabelJujuModelUUID = "model.juju.is/id"

	// LabelJujuControllerUUID is the juju label applied for juju controllers.
	// Since LabelVersion2.
	LabelJujuControllerUUID = "controller.juju.is/id"

	// LabelJujuOperatorName is the juju label applied to Juju operators to
	// identify their name. Operator names are generally named after the thing
	// the operator is controlling. i.e an operator name for a model test would be
	// "test".
	// Since LabelVersion1.
	LabelJujuOperatorName = "operator.juju.is/name"

	// LabelJujuOperatorTarget is the juju label applied to Juju operators to
	// describe the modeling paradigm they target. For example model,
	// application.
	// Since LabelVersion1.
	LabelJujuOperatorTarget = "operator.juju.is/target"

	// LabelJujuStorageName is the juju label applied to Juju storage objects to
	// describe their name.
	// Since LabelVersion1.
	LabelJujuStorageName = "storage.juju.is/name"

	// LegacyLabelKubernetesAppName is the legacy label key used for juju app
	// identification. This purely exists to maintain backwards functionality.
	// See https://bugs.launchpad.net/juju/+bug/1888513
	// For LegacyLabelVersion.
	LegacyLabelKubernetesAppName = "juju-app"

	// LegacyLabelModelName is the legacy label key used for juju models. This
	// purely exists to maintain backwards functionality.
	// See https://bugs.launchpad.net/juju/+bug/1888513
	// For LegacyLabelVersion.
	LegacyLabelModelName = "juju-model"

	// LegacyLabelModelOperator is the legacy label key used for juju model
	// operators. This purely exists to maintain backwards functionality.
	// See https://bugs.launchpad.net/juju/+bug/1888513
	// For LegacyLabelVersion.
	LegacyLabelModelOperator = "juju-modeloperator"

	// LegacyLabelKubernetesOperatorName is the legacy label key used for juju
	// operators. This purely exists to maintain backwards functionality.
	// See https://bugs.launchpad.net/juju/+bug/1888513
	// For LegacyLabelVersion.
	LegacyLabelKubernetesOperatorName = "juju-operator"

	// LegacyLabelJujuStorageName is the legacy label key used for juju storage
	// pvc. This purely exists to maintain backwards functionality.
	// See https://bugs.launchpad.net/juju/+bug/1888513
	// For LegacyLabelVersion.
	LegacyLabelStorageName = "juju-storage"
)
