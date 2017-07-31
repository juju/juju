// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

// Status describes the status of a relation.
type Status string

const (
	Active  Status = "active"
	Revoked Status = "revoked"
)
