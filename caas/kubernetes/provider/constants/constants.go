// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constants

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentconstants "github.com/juju/juju/agent/constants"
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

	// ControllerServiceFQDNTemplate is the FQDN of the controller service using the cluster DNS.
	ControllerServiceFQDNTemplate = "controller-service.controller-%s.svc.cluster.local"
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
