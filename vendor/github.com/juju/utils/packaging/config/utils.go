// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package config

// flipMap is a helper function which flips a strings map, making the
// keys of the initial one the values and vice-versa.
func flipMap(m map[string]string) map[string]string {
	res := make(map[string]string)
	for k, v := range m {
		res[v] = k
	}
	return res
}
