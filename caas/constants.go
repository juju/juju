// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

const (
	// CharmContainerName is the name of the charm container in a charm application pod.
	CharmContainerName = "charm"
)

const (
	// CharmMemRequestMi is the charm container's memory request constraint in MiB.
	CharmMemRequestMi = 64
	// CharmMemLimitMi is the charm container's memory limit constraint in MiB.
	CharmMemLimitMi = 256
)
