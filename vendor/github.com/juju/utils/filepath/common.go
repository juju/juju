// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package filepath

func splitSuffix(path string) (string, string) {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' && i > 0 {
			return path[:i], path[i:]
		}
	}
	return path, ""
}
