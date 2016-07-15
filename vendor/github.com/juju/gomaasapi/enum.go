// Copyright 2012-2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

const (
	// NodeStatus* values represent the vocabulary of a Node‘s possible statuses.

	// The node has been created and has a system ID assigned to it.
	NodeStatusDeclared = "0"

	//Testing and other commissioning steps are taking place.
	NodeStatusCommissioning = "1"

	// Smoke or burn-in testing has a found a problem.
	NodeStatusFailedTests = "2"

	// The node can’t be contacted.
	NodeStatusMissing = "3"

	// The node is in the general pool ready to be deployed.
	NodeStatusReady = "4"

	// The node is ready for named deployment.
	NodeStatusReserved = "5"

	// The node is powering a service from a charm or is ready for use with a fresh Ubuntu install.
	NodeStatusDeployed = "6"

	// The node has been removed from service manually until an admin overrides the retirement.
	NodeStatusRetired = "7"

	// The node is broken: a step in the node lifecyle failed. More details
	// can be found in the node's event log.
	NodeStatusBroken = "8"

	// The node is being installed.
	NodeStatusDeploying = "9"

	// The node has been allocated to a user and is ready for deployment.
	NodeStatusAllocated = "10"

	// The deployment of the node failed.
	NodeStatusFailedDeployment = "11"

	// The node is powering down after a release request.
	NodeStatusReleasing = "12"

	// The releasing of the node failed.
	NodeStatusFailedReleasing = "13"

	// The node is erasing its disks.
	NodeStatusDiskErasing = "14"

	// The node failed to erase its disks.
	NodeStatusFailedDiskErasing = "15"
)
