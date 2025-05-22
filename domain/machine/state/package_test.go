// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

func ptr[T any](v T) *T {
	return &v
}
