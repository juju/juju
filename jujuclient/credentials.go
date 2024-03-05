// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import (
	"os"

	"github.com/juju/errors"
	"github.com/juju/utils/v4"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/juju/osenv"
)

// JujuCredentialsPath is the location where controllers information is
// expected to be found.
func JujuCredentialsPath() string {
	return osenv.JujuXDGDataHomePath("credentials.yaml")
}

// ReadCredentialsFile loads all credentials defined in a given file.
// If the file is not found, it is not an error.
func ReadCredentialsFile(file string) (*cloud.CredentialCollection, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return &cloud.CredentialCollection{}, nil
		}
		return nil, err
	}
	credentials, err := cloud.ParseCredentialCollection(data)
	if err != nil {
		return nil, err
	}
	return credentials, nil
}

// WriteCredentialsFile marshals to YAML details of the given credentials
// and writes it to the credentials file.
func WriteCredentialsFile(credentials *cloud.CredentialCollection) error {
	data, err := yaml.Marshal(credentials)
	if err != nil {
		return errors.Annotate(err, "cannot marshal yaml credentials")
	}
	return utils.AtomicWriteFile(JujuCredentialsPath(), data, os.FileMode(0600))
}
