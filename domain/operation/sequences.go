// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import "github.com/juju/juju/domain/sequence"

const (
	// OperationSequenceNamespace is the namespace for operation and task sequences.
	// Both operation_id and task_id share the same sequence according to the DDL.
	OperationSequenceNamespace = sequence.StaticNamespace("operation")
)
