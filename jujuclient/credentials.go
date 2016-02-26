// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import (
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	"github.com/juju/utils"
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
func ReadCredentialsFile(file string) (map[string]cloud.CloudCredential, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	credentials, err := cloud.ParseCredentials(data)
	if err != nil {
		return nil, err
	}
	return credentials, nil
}

// WriteCredentialsFile marshals to YAML details of the given credentials
// and writes it to the credentials file.
func WriteCredentialsFile(credentials map[string]cloud.CloudCredential) error {
	data, err := yaml.Marshal(credentialsCollection{credentials})
	if err != nil {
		return errors.Annotate(err, "cannot marshal yaml credentials")
	}
	return utils.AtomicWriteFile(JujuCredentialsPath(), data, os.FileMode(0600))
}

// credentialsCollection is a struct containing cloud credential information,
// used marshalling and unmarshalling.
type credentialsCollection struct {
	// Credentials is a map of cloud credentials, keyed on cloud name.
	Credentials map[string]cloud.CloudCredential `yaml:"credentials"`
}
