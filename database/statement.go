// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"strings"

	"github.com/juju/collections/transform"
)

// SliceToPlaceholder returns a string that can be used in a SQL/DML
// statement as a parameter list for a [NOT] IN clause.
// For example, passing []int{1, 2, 3} would return "?,?,?".
func SliceToPlaceholder[T any](in []T) string {
	return strings.Join(transform.Slice(in, func(item T) string { return "?" }), ",")
}
