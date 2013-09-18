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
	path string
	User string
	Password string
	StateServers []string	`yaml:"state-servers"`
	CACert string	`yaml:"ca-cert"`
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

// CreateInfo implements environs.ConfigStorage.CreateInfo.
func (d *diskStore) CreateInfo(envName string) (environs.EnvironInfo, error) {
	// We create an empty file so that any subsequent CreateInfos
	// will fail.
	path := d.envPath(envName)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		return nil, environs.ErrEnvironInfoAlreadyExists
	}
	if err != nil {
		return nil, err
	}
	file.Close()
	return &environInfo{
		path: path,
	}, nil
}

// ReadInfo implements environs.ConfigStorage.ReadInfo.
func (d *diskStore) ReadInfo(envName string) (environs.EnvironInfo, error) {
	path := d.envPath(envName)
	data, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.NotFoundf("environment %q", envName)
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("environment in progress XXX better error message needed")
	}
	var info environInfo
	if err := goyaml.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("error unmarshalling %q: %v", path, err)
	}
	info.path = path
	return &info, nil
}

// APICredentials implements environs.EnvironInfo.APICredentials.
func (info *environInfo) APICredentials() environs.APICredentials {
	return environs.APICredentials{
		User: info.User,
		Password: info.Password,
	}
}

// APIEndpoint implements environs.EnvironInfo.APIEndpoint.
func (info *environInfo) APIEndpoint() environs.APIEndpoint {
	return environs.APIEndpoint{
		Addresses: info.StateServers,
		CACert: info.CACert,
	}
}

func (info *environInfo) SetAPIEndpoint(endpoint environs.APIEndpoint) {
	info.StateServers = endpoint.Addresses
	info.CACert = endpoint.CACert
}

func (info *environInfo) SetAPICredentials(creds environs.APICredentials) {
	info.User = creds.User
	info.Password = creds.Password
}

func (info *environInfo) Write() error {
	data, err := goyaml.Marshal(info)
	if err != nil {
		return fmt.Errorf("cannot marshal environment info: %v", err)
	}
	// Create a temporary file and rename it, so that the data
	// changes atomically.
	parent, _ := filepath.Split(info.path)
	tmpFile, err := ioutil.TempFile(parent, "")
	if err != nil {
		return fmt.Errorf("cannot create temporary file: %v", err)
	}
	defer tmpFile.Close()
	_, err = tmpFile.Write(data)
	if err != nil {
		return fmt.Errorf("cannot write temporary file: %v", err)
	}
	if err := os.Rename(tmpFile.Name(), info.path); err != nil {
		os.Remove(tmpFile.Name())
		return fmt.Errorf("cannot rename new environment info file: %v", err)
	}
	return nil
}

func (info *environInfo) Destroy() error {
	return os.Remove(info.path)
}
