// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	internalcharm "github.com/juju/juju/internal/charm"
)

// NotImplementedRelationError represents an error indicating that a specified
// relation is not implemented in a candidate charm to upgrade an application.
type NotImplementedRelationError struct {
	Relation internalcharm.Relation
}

// Error implements error built-in interface.
func (e NotImplementedRelationError) Error() string {
	return "relation not implemented"
}

// RelationQuotaLimitExceededError represents an error indicating that an
// existing relation would exceed its limit if an application is upgraded with
// a candidate charm.
type RelationQuotaLimitExceededError struct {
	Relation internalcharm.Relation
	Count    int
}

// Error implements error built-in interface.
func (e RelationQuotaLimitExceededError) Error() string {
	return "quota limit exceeded"
}
