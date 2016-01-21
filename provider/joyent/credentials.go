// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
)

type environProviderCredentials struct{}

func (environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		// TODO(axw) Need to set private-key from private-key-file if
		//           set. This is different to "jsonfile", because we're
		//           setting just one attribute from a file instead of
		//           all of the credentials.
		//
		//           The schema probably needs an attribute field that
		//           specifies the name of an attribute that will be
		//           used to specify the path of the file that will be
		//           read. In the GUI, we could use that as an indication
		//           to offer a file-upload dialog for that attribute;
		//           when adding credentials interactively in the CLI,
		//           we would first ask for "private-key", and if nothing
		//           is entered, prompt for a file location (with
		//           directions to that effect).
		//
		// TODO(axw) we need a more appropriate name for this authentication
		//           type. ssh?
		cloud.UserPassAuthType: {
			sdcUser: {
				Description: "SmartDataCenter user ID",
			},
			sdcKeyId: {
				Description: "SmartDataCenter key ID",
			},
			mantaUser: {
				Description: "Manta user ID",
			},
			mantaKeyId: {
				Description: "Manta key ID",
			},
			privateKey: {
				Description: "Private key used to sign requests",
				Hidden:      true,
			},
			algorithm: {
				Description: "Algorithm used to generate the private key",
			},
		},
	}
}

func (environProviderCredentials) DetectCredentials() ([]cloud.Credential, error) {
	return nil, errors.NotFoundf("credentials")
}
