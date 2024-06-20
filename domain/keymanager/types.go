// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager

// PublicKey represents the domains understand of what a public key is and the
// indiviual parts the domain cares about.
type PublicKey struct {
	Comment     string
	Fingerprint string
	Key         string
}
