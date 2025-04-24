// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelprovider

import (
	jujucloud "github.com/juju/juju/cloud"
)

// CloudCredentialInfo represents a credential.
type CloudCredentialInfo struct {
	AuthType   jujucloud.AuthType
	Attributes map[string]string
}
