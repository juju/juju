// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore
import (
	"io/ioutil"
	"fmt"
	"path/filepath"
	"os"

	"launchpad.net/goyaml"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/environs"
)

type diskStore struct {
	dir string
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
	return filepath.Join(d.dir, envName + ".yaml")
}

func (d *diskStore) EnvironInfo(envName string) (*environs.EnvironInfo, error) {
	path := d.envPath(envName)
	data, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.NotFoundf("environment %q", envName)
		}
		return nil, err
	}
	var info environs.EnvironInfo
	if err := goyaml.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("error unmarshalling %q: %v", path, err)
	}
	return &info, nil
}

func (d *diskStore) WriteEnvironInfo(envName string, info *environs.EnvironInfo) error {
	if err := os.MkdirAll(d.dir, 0700); err != nil {
		return err
	}
	data, err := goyaml.Marshal(info)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(d.envPath(envName), data, 0600); err != nil {
		return err
	}
	return nil
}
