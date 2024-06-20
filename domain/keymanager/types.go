// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager

// FingerprintHashAlgorithm represents the hasing algorithm to computer the
// fingerprint of a public key.
type FingerprintHashAlgorithm string

// PublicKey repreents an exploded view of an ssh public key. Specifically the
// components of a public key that the keymanager cares about in order to offer
// the functionality of the domain.
type PublicKey struct {
	// Comment represents the comment string of the public key. May be empty if
	// the public key does not contain a comment.
	Comment string

	// FingerprintHash is the hasing algorithm used to computer the
	// [PublicKey.Fingerprint]
	FingerprintHash FingerprintHashAlgorithm

	// Fingerprint is the computed hash of an ssh public key.
	Fingerprint string

	// Key is the raw ssh public key.
	Key string
}

const (
	// FingerprintHashAlgorithmMD5
	FingerprintHashAlgorithmMD5 = FingerprintHashAlgorithm("md5")

	// FingerprintHashAlgorithmSHA256
	FingerprintHashAlgorithmSHA256 = FingerprintHashAlgorithm("sha256")
)

func (f FingerprintHashAlgorithm) String() string {
	return string(f)
}
