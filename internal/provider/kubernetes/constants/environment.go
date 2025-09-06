// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constants

const (
	// OperatorPodIPEnvName is the environment name for operator pod IP.
	OperatorPodIPEnvName = "JUJU_OPERATOR_POD_IP"

	// OperatorServiceIPEnvName is the environment name for operator service IP.
	OperatorServiceIPEnvName = "JUJU_OPERATOR_SERVICE_IP"

	// OperatorNamespaceEnvName is the environment name for k8s namespace the operator is in.
	OperatorNamespaceEnvName = "JUJU_OPERATOR_NAMESPACE"
)
