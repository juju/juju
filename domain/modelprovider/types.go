// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelprovider

// CloudCredentialInfo represents a credential.
type CloudCredentialInfo struct {
	AuthType   string
	Attributes map[string]string
}
