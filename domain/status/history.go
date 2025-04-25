// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import "github.com/juju/juju/internal/statushistory"

var (
	// ApplicationNamespace is the namespace for application status.
	ApplicationNamespace = statushistory.Namespace{Name: "application"}

	// UnitNamespace is the namespace for unit status.
	UnitAgentNamespace = statushistory.Namespace{Name: "unit-agent"}

	// UnitWorkloadNamespace is the namespace for unit workload status.
	UnitWorkloadNamespace = statushistory.Namespace{Name: "unit-workload"}
)
