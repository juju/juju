// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

// SetSigningPublicKey sets a new public key for testing and returns the original key.
func SetSigningPublicKey(key string) string {
	oldKey := simplestreamsImagesPublicKey
	simplestreamsImagesPublicKey = key
	return oldKey
}
