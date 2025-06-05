// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/statushistory"
)

var (
	// ApplicationNamespace is the namespace for application status.
	ApplicationNamespace = statushistory.Namespace{Kind: status.KindApplication}

	// UnitAgentNamespace is the namespace for unit status.
	UnitAgentNamespace = statushistory.Namespace{Kind: status.KindUnitAgent}

	// UnitWorkloadNamespace is the namespace for unit workload status.
	UnitWorkloadNamespace = statushistory.Namespace{Kind: status.KindWorkload}

	// MachineNamespace is the namespace for machine status.
	MachineNamespace = statushistory.Namespace{Kind: status.KindMachine}

	// MachineInstanceNamespace is the namespace for instance status.
	MachineInstanceNamespace = statushistory.Namespace{Kind: status.KindMachineInstance}

	// FilesystemNamespace is the namespace for filesystem status.
	FilesystemNamespace = statushistory.Namespace{Kind: status.KindFilesystem}

	// VolumeNamespace is the namespace for volume status.
	VolumeNamespace = statushistory.Namespace{Kind: status.KindVolume}
)
