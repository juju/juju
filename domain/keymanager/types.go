// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager

type PublicKey struct {
	Comment     string
	Fingerprint string
	Key         string
}
