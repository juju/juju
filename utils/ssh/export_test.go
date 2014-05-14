// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

var (
	ReadAuthorisedKeys  = readAuthorisedKeys
	WriteAuthorisedKeys = writeAuthorisedKeys
	InitDefaultClient   = initDefaultClient
	DefaultIdentities   = &defaultIdentities
	SSHDial             = &sshDial
	RSAGenerateKey      = &rsaGenerateKey
)
