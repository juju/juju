// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v2

import (
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/export/types/latest"
)

// ImportOperations registers the model-DB import operations for the new export
// format with the given coordinator. It is the new-format counterpart of the
// legacy migration.ImportOperations; operations consume the transformed,
// target-version model-DB payload ([latest.ModelExport]) directly — no
// description.Model, and no per-operation type assertion.
//
// No per-domain operations are registered yet: the core domains are added by
// Task 8 and the CMR/offer/secret and remaining domains by Task 9, each via a
// RegisterImport(coordinator, ...) call here (from the domain's
// .../modelmigration/v2 package) in FK-respecting order. Wiring this empty
// registry now exercises the coordinator route end to end on every v8 import,
// so those tasks only add their operations.
func ImportOperations(coordinator coremodelmigration.OperationAdder[*latest.ModelExport]) {
	// Order is important! Mirror the FK order used by the legacy import when
	// adding per-domain RegisterImport(coordinator, ...) calls.
}
