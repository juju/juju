// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constants

const (
	EnvAgentHTTPProbePort  = "HTTP_PROBE_PORT"
	EnvJujuContainerNames  = "JUJU_CONTAINER_NAMES"
	EnvJujuK8sPodName      = "JUJU_K8S_POD_NAME"
	EnvJujuK8sPodUUID      = "JUJU_K8S_POD_UUID"
	EnvJujuK8sUnitPassword = "JUJU_K8S_UNIT_PASSWORD"

	// ApplicationInitContainer is the init container which sets up the charm agent config.
	ApplicationInitContainer = "charm-init"
	// ApplicationCharmContainer is the container which runs the unit agent.
	ApplicationCharmContainer = "charm"
)
