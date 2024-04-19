// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package query provides generic functions designed to wrap variables,
// designating them as inputs or single/multi row outputs for SQLair.
// This package is small and dependency free, and exists so that these
// functions can be imported via the "." alias, making their usage as
// terse as possible.
package query

type processFunc = func(in, out, samples []any) ([]any, []any, []any)

// In returns a processing function suitable for handling
// the argument as an input to SQLair queries.
func In[T any](v T) processFunc {
	return func(in, out, samples []any) ([]any, []any, []any) {
		return append(in, v), out, append(samples, v)
	}
}

// Out returns a processing function suitable for handling the argument
// as an output column of SQLair queries returning a single row.
func Out[T any](v *T) processFunc {
	return func(in, out, samples []any) ([]any, []any, []any) {
		return in, append(out, v), append(samples, *v)
	}
}

// OutM returns a processing function suitable for handling the argument
// as the output column of SQLair queries returning multiple rows.
func OutM[T any](v *[]T) processFunc {
	return func(in, out, samples []any) ([]any, []any, []any) {
		var val T
		return in, append(out, v), append(samples, val)
	}
}
