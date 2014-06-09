// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"launchpad.net/goyaml"

	"github.com/juju/juju/juju/osenv"
)

var logger = loggo.GetLogger("juju.environs.configstore")

// Default returns disk-based environment config storage
// rooted at JujuHome.
func Default() (Storage, error) {
	return NewDisk(osenv.JujuHome())
}

type diskStore struct {
	dir string
}

type EnvironInfoData struct {
	User         string
	Password     string
	StateServers []string               `json:"state-servers" yaml:"state-servers"`
	CACert       string                 `json:"ca-cert" yaml:"ca-cert"`
	Config       map[string]interface{} `json:"bootstrap-config,omitempty" yaml:"bootstrap-config,omitempty"`
}

type environInfo struct {
	path string
	// initialized signifies whether the info has been written.
	initialized bool

	// created signifies whether the info was returned from
	// a CreateInfo call.
	created bool

	EnvInfo EnvironInfoData
}

// NewDisk returns a ConfigStorage implementation that stores
// configuration in the given directory. The parent of the directory
// must already exist; the directory itself is created on demand.
func NewDisk(dir string) (Storage, error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, err
	}
	return &diskStore{dir}, nil
}

func (d *diskStore) envPath(envName string) string {
	return filepath.Join(d.dir, "environments", envName+".jenv")
}

func (d *diskStore) mkEnvironmentsDir() error {
	path := filepath.Join(d.dir, "environments")
	logger.Debugf("Making %v", path)
	err := os.Mkdir(path, 0700)
	if os.IsExist(err) {
		return nil
	}
	return err
}

// CreateInfo implements Storage.CreateInfo.
func (d *diskStore) CreateInfo(envName string) (EnvironInfo, error) {
	if err := d.mkEnvironmentsDir(); err != nil {
		return nil, err
	}
	// We create an empty file so that any subsequent CreateInfos
	// will fail.
	path := d.envPath(envName)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		return nil, ErrEnvironInfoAlreadyExists
	}
	if err != nil {
		return nil, err
	}
	file.Close()
	return &environInfo{
		created: true,
		path:    path,
	}, nil
}

// ReadInfo implements Storage.ReadInfo.
func (d *diskStore) ReadInfo(envName string) (EnvironInfo, error) {
	path := d.envPath(envName)
	data, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.NotFoundf("environment %q", envName)
		}
		return nil, err
	}
	var info environInfo
	info.path = path
	if len(data) == 0 {
		return &info, nil
	}
	if err := goyaml.Unmarshal(data, &info.EnvInfo); err != nil {
		return nil, fmt.Errorf("error unmarshalling %q: %v", path, err)
	}
	info.initialized = true
	return &info, nil
}

// Initialized implements EnvironInfo.Initialized.
func (info *environInfo) Initialized() bool {
	return info.initialized
}

// BootstrapConfig implements EnvironInfo.BootstrapConfig.
func (info *environInfo) BootstrapConfig() map[string]interface{} {
	return info.EnvInfo.Config
}

// APICredentials implements EnvironInfo.APICredentials.
func (info *environInfo) APICredentials() APICredentials {
	return APICredentials{
		User:     info.EnvInfo.User,
		Password: info.EnvInfo.Password,
	}
}

// APIEndpoint implements EnvironInfo.APIEndpoint.
func (info *environInfo) APIEndpoint() APIEndpoint {
	return APIEndpoint{
		Addresses: info.EnvInfo.StateServers,
		CACert:    info.EnvInfo.CACert,
	}
}

// SetBootstrapConfig implements EnvironInfo.SetBootstrapConfig.
func (info *environInfo) SetBootstrapConfig(attrs map[string]interface{}) {
	if !info.created {
		panic("bootstrap config set on environment info that has not just been created")
	}
	info.EnvInfo.Config = attrs
}

// SetAPIEndpoint implements EnvironInfo.SetAPIEndpoint.
func (info *environInfo) SetAPIEndpoint(endpoint APIEndpoint) {
	info.EnvInfo.StateServers = endpoint.Addresses
	info.EnvInfo.CACert = endpoint.CACert
}

// SetAPICredentials implements EnvironInfo.SetAPICredentials.
func (info *environInfo) SetAPICredentials(creds APICredentials) {
	info.EnvInfo.User = creds.User
	info.EnvInfo.Password = creds.Password
}

// Location returns the location of the environInfo in human readable format.
func (info *environInfo) Location() string {
	return fmt.Sprintf("file %q", info.path)
}

// Write implements EnvironInfo.Write.
func (info *environInfo) Write() error {
	data, err := goyaml.Marshal(info.EnvInfo)
	if err != nil {
		return errors.Annotate(err, "cannot marshal environment info")
	}
	// Create a temporary file and rename it, so that the data
	// changes atomically.
	parent, _ := filepath.Split(info.path)
	tmpFile, err := ioutil.TempFile(parent, "")
	if err != nil {
		return errors.Annotate(err, "cannot create temporary file")
	}
	_, err = tmpFile.Write(data)
	// N.B. We need to close the file before renaming it
	// otherwise it will fail under Windows with a file-in-use
	// error.
	tmpFile.Close()
	if err != nil {
		return errors.Annotate(err, "cannot write temporary file")
	}
	if err := utils.ReplaceFile(tmpFile.Name(), info.path); err != nil {
		os.Remove(tmpFile.Name())
		return errors.Annotate(err, "cannot rename new environment info file")
	}
	info.initialized = true
	return nil
}

// Destroy implements EnvironInfo.Destroy.
func (info *environInfo) Destroy() error {
	err := os.Remove(info.path)
	if os.IsNotExist(err) {
		return fmt.Errorf("environment info has already been removed")
	}
	return err
}
