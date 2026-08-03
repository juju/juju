// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constants

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentconstants "github.com/juju/juju/agent/constants"
	corepebble "github.com/juju/juju/core/pebble"
)

const (
	// Domain is the primary TLD for juju when giving resource domains to
	// Kubernetes
	Domain = "juju.is"

	// LegacyDomain is the legacy primary TLD for juju when giving resource domains to
	// Kubernetes
	LegacyDomain = "juju.io"

	// AgentHTTPProbePort is the default port used by the HTTP server responding
	// to caas probes
	AgentHTTPProbePort = "3856"

	// AgentHTTPPathLiveness is the path used for liveness probes on the agent
	AgentHTTPPathLiveness = "/liveness"

	// AgentHTTPPathReadiness is the path used for readiness probes on the agent
	AgentHTTPPathReadiness = "/readiness"

	// AgentHTTPPathStartup is the path used for startup probes on the agent
	AgentHTTPPathStartup = "/startup"

	// DefaultPebbleDir is the default directory Pebble considers when starting
	// up. It re-exports the canonical value from core/pebble so existing
	// callers within the k8s provider can continue to use this package.
	DefaultPebbleDir = corepebble.DefaultPebbleDir

	// JujuExecServerSocketPort is the port used by juju run callbacks.
	JujuExecServerSocketPort = 30666

	// TemplateFileNameAgentConf is the template agent.conf file name.
	TemplateFileNameAgentConf = "template-" + agentconstants.AgentConfigFilename

	// ControllerAgentConfigFilename is the agent conf filename
	// for the controller agent for the api server.
	ControllerAgentConfigFilename = "controller-agent.conf"

	// ControllerUnitAgentConfigFilename is the agent conf filename
	// for the controller unit agent which runs the charm.
	ControllerUnitAgentConfigFilename = "controller-unit-agent.conf"

	// CAASProviderType is the provider type for k8s.
	CAASProviderType = "kubernetes"

	// CAASImageRepoSecretName is the name of the secret for image pull.
	CAASImageRepoSecretName = "juju-image-pull-secret"

	// JujuControllerStackName is the juju CAAS controller stack name.
	JujuControllerStackName = "controller"

	// JujuControllerModelName is the name of the juju controller model.
	JujuControllerModelName = "controller"

	// ControllerServiceFQDNTemplate is the FQDN of the controller service using the cluster DNS.
	ControllerServiceFQDNTemplate = "controller-service.controller-%s.svc.cluster.local"

	// ControllerServiceEndpointsName is the name of the headless service that
	// governs the controller StatefulSet. It gives each controller pod a
	// stable per-ordinal DNS name for Dqlite peering. It must match the name
	// derived for the headless service during bootstrap
	// (getBootstrapResourceName(JujuControllerStackName, "service-endpoints")).
	ControllerServiceEndpointsName = JujuControllerStackName + "-service-endpoints"

	// ClusterLocalDomain is the default cluster-internal DNS domain suffix used
	// by Kubernetes for service and pod FQDNs. Non-default cluster DNS domains
	// are not currently supported.
	ClusterLocalDomain = "svc.cluster.local"

	// CharmVolumeName is the name of the k8s volume where shared charm data is stored.
	CharmVolumeName = "charm-data"

	// JujuUserID is the juju user id for rootless juju agents.
	// NOTE: 170 uid/gid must be updated here and in caas/Dockerfile and caas/scripts.go
	JujuUserID int64 = 170
	// JujuGroupID is the juju group id for rootless juju agents.
	JujuGroupID int64 = 170
	// JujuSudoUserID is the juju user id for rootless juju agents with sudo.
	// NOTE: 171 uid/gid must be updated here and in caas/Dockerfile
	JujuSudoUserID int64 = 171
	// JujuSudoGroupID is the juju group id for rootless juju agents with sudo.
	JujuSudoGroupID int64 = 171
	// JujuFSGroupID is the group id for all fs entries written to k8s volumes.
	JujuFSGroupID int64 = 170
)

const (
	// CharmMemRequestMi is the charm container's memory request value in Mi.
	CharmMemRequestMi = "64Mi"
	// CharmMemLimitMi is the charm container's memory limit value in Mi.
	CharmMemLimitMi = "1024Mi"
)

const (
	// ModelOperatorTargetValue is the value of the operator target for
	// model operators.
	ModelOperatorTargetValue = "model"

	// ModelOperatorName is the model operator stack name used for deployment,
	// service, RBAC resources.
	ModelOperatorName = "modeloperator"
)

// DefaultPropagationPolicy returns the default propagation policy.
func DefaultPropagationPolicy() *metav1.DeletionPropagation {
	v := metav1.DeletePropagationForeground
	return &v
}

// DeletePropagationBackground returns the background propagation policy.
func DeletePropagationBackground() *metav1.DeletionPropagation {
	v := metav1.DeletePropagationBackground
	return &v
}

// DeletePropagationOrphan returns the orphan propagation policy.
func DeletePropagationOrphan() *metav1.DeletionPropagation {
	v := metav1.DeletePropagationOrphan
	return &v
}
