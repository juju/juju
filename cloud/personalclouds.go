// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package cloud provides functionality to parse information
// describing clouds, including regions, supported auth types etc.

package cloud

import (
	"io/ioutil"
	"os"

	"github.com/juju/errors"

	"github.com/juju/juju/juju/osenv"
)

// JujuPersonalCloudsPath is the location where personal cloud information is
// expected to be found. Requires JUJU_HOME to be set.
func JujuPersonalCloudsPath() string {
	return osenv.JujuXDGDataHomePath("clouds.yaml")
}

// PersonalCloudMetadata loads any personal cloud metadata defined
// in the Juju Home directory. If not cloud metadata is found,
// that is not an error; nil is returned.
func PersonalCloudMetadata() (map[string]Cloud, error) {
	clouds, err := ParseCloudMetadataFile(JujuPersonalCloudsPath())
	if err != nil && os.IsNotExist(err) {
		return nil, nil
	}
	return clouds, err
}

// ParseCloudMetadataFile loads any cloud metadata defined
// in the specified file.
func ParseCloudMetadataFile(file string) (map[string]Cloud, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	clouds, err := ParseCloudMetadata(data)
	if err != nil {
		return nil, err
	}
	return clouds, err
}

// WritePersonalCloudMetadata marshals to YAMl and writes the cloud metadata
// to the personal cloud file.
func WritePersonalCloudMetadata(cloudsMap map[string]Cloud) error {
	data, err := marshalCloudMetadata(cloudsMap)
	if err != nil {
		return errors.Trace(err)
	}
	return ioutil.WriteFile(JujuPersonalCloudsPath(), data, os.FileMode(0600))
}
