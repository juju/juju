// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

const CurrentStreamsVersion = currentStreamsVersion

// SetSigningPublicKey sets a new public key for testing and returns the original key.
func SetSigningPublicKey(key string) string {
	oldKey := SimplestreamsImagesPublicKey
	SimplestreamsImagesPublicKey = key
	return oldKey
}
