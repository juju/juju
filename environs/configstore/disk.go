// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"launchpad.net/goyaml"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/errors"
)

type diskStore struct {
	dir string
}

type environInfo struct {
	Creds    environs.APICredentials
	Endpoint environs.APIEndpoint
}

// NewDisk returns a ConfigStorage implementation that
// stores configuration in the given directory.
// The parent of the directory must already exist;
// the directory itself is created on demand.
func NewDisk(dir string) (environs.ConfigStorage, error) {
	parent, _ := filepath.Split(dir)
	if _, err := os.Stat(parent); err != nil {
		return nil, err
	}
	return &diskStore{dir}, nil
}

func (d *diskStore) envPath(envName string) string {
	return filepath.Join(d.dir, envName+".yaml")
}

// EnvironInfo implements environs.ConfigStorage.EnvironInfo.
func (d *diskStore) ReadInfo(envName string) (environs.EnvironInfo, error) {
	path := d.envPath(envName)
	data, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.NotFoundf("environment %q", envName)
		}
		return nil, err
	}
	var info environInfo
	if err := goyaml.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("error unmarshalling %q: %v", path, err)
	}
	return &info, nil
}

func (info *environInfo) APICredentials() environs.APICredentials {
	return info.Creds
}

func (info *environInfo) APIEndpoint() environs.APIEndpoint {
	return info.Endpoint
}
