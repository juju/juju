// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams

// SetSigningKey sets a new signing key for testing and returns the original key.
func SetSigningKey(key string) string {
	oldKey := simpleStreamSigningKey
	simpleStreamSigningKey = key
	return oldKey
}
