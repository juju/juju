// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

// Flatten takes as argument a nested array of elements T.
// Example: [["a", "b"], ["c", "d"]] -> ["a", "b", "c", "d"]
func Flatten[T any](nested [][]T) []T {
	var flattened []T
	for _, elem := range nested {
		flattened = append(flattened, elem...)
	}

	return flattened
}
