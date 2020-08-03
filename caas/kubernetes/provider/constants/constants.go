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

	LabelOperator      = "juju-operator"
	LabelStorage       = "juju-storage"
	LabelVersion       = "juju-version"
	LabelApplication   = "juju-app"
	LabelModel         = "juju-model"
	LabelModelOperator = "juju-modeloperator"
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
