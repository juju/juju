// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constants

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/agent"
)

const (
	// OperatorPodIPEnvName is the environment name for operator pod IP.
	OperatorPodIPEnvName = "JUJU_OPERATOR_POD_IP"

	// OperatorServiceIPEnvName is the environment name for operator service IP.
	OperatorServiceIPEnvName = "JUJU_OPERATOR_SERVICE_IP"

	// OperatorNamespaceEnvName is the environment name for k8s namespace the operator is in.
	OperatorNamespaceEnvName = "JUJU_OPERATOR_NAMESPACE"

	// JujuRunServerSocketPort is the port used by juju run callbacks.
	JujuRunServerSocketPort = 30666

	// TemplateFileNameAgentConf is the template agent.conf file name.
	TemplateFileNameAgentConf = "template-" + agent.AgentConfigFilename

	// AnnotationPrefix of juju annotations
	AnnotationPrefix = "juju.io"

	// LabelJujuAppCreatedBy is a Juju application label to apply to objects
	// created by applications managed by Juju. Think istio, kubeflow etc
	// See https://bugs.launchpad.net/juju/+bug/1892285
	LabelJujuAppCreatedBy = "app.juju.is/created-by"

	// AnnotationJujuStorageName is the Juju annotation that represents a
	// storage objects associated Juju name.
	AnnotationJujuStorageName = "storage.juju.is/name"

	// AnnotationJujuVersion is the version annotation used on operator
	// deployments.
	AnnotationJujuVersion = "juju.is/version"

	// LabelKubernetesAppName is the common meta key for kubernetes app names.
	// See https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/#labels
	LabelKubernetesAppName = "app.kubernetes.io/name"

	// LabelKubernetesAppManaged is the common meta key for kubernetes apps
	// that are managed by a non k8s process (such as Juju).
	// See https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/#labels
	LabelKubernetesAppManaged = "app.kubernetes.io/managed-by"

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

	// LegacyAnnotationStorageName is the legacy annotation used by Juju for
	// dictating storage name on k8s storage objects.
	LegacyAnnotationStorageName = "juju-storage"

	// LegacyAnnotationVersion is the legacy annotation used by Juju for
	// dictating juju agent version on operators.
	LegacyAnnotationVersion = "juju-version"

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

	// LegacyLabelJujuStorageName is the legacy label key used for juju storage
	// pvc. This purely exists to maintain backwards functionality.
	// See https://bugs.launchpad.net/juju/+bug/1888513
	LegacyLabelStorageName = "juju-storage"
)

func AnnotationKey(name string) string {
	return AnnotationPrefix + "/" + name
}

var (
	DefaultPropagationPolicy = metav1.DeletePropagationForeground

	AnnotationModelUUIDKey              = AnnotationKey("model")
	AnnotationControllerUUIDKey         = AnnotationKey("controller")
	AnnotationControllerIsControllerKey = AnnotationKey("is-controller")
	AnnotationUnit                      = AnnotationKey("unit")
	AnnotationCharmModifiedVersionKey   = AnnotationKey("charm-modified-version")
)
