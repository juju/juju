// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constants

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/v2/agent"
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

	// JujuRunServerSocketPort is the port used by juju run callbacks.
	JujuRunServerSocketPort = 30666

	// TemplateFileNameAgentConf is the template agent.conf file name.
	TemplateFileNameAgentConf = "template-" + agent.AgentConfigFilename

	// CAASProviderType is the provider type for k8s.
	CAASProviderType = "kubernetes"

	// CAASImageRepoSecretName is the name of the secret for image pull.
	CAASImageRepoSecretName = "juju-image-pull-secret"
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
