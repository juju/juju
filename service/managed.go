// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils/fs"

	"github.com/juju/juju/service/initsystems"
)

const (
	// initDir is the name of the directory where per-service confDir
	// directories are stored. This will typically be relative to the
	// juju data dir.
	initDir = "init"
)

// serviceConfigs represents all the init system service configurations
// that juju manages. It is an implementation detail of the Services
// type.
type serviceConfigs struct {
	baseDir    string
	initSystem string
	prefixes   []string

	names []string
	fops  fs.Operations
}

// newConfigs builds a new serviceConfigs using the provided data.
// The caller must ensure that baseDir exists.
func newConfigs(baseDir, initSystem string, prefixes ...string) *serviceConfigs {
	if len(prefixes) == 0 {
		prefixes = jujuPrefixes
	}
	return &serviceConfigs{
		baseDir:    filepath.Join(baseDir, initDir),
		initSystem: initSystem,
		prefixes:   prefixes,
		fops:       newFileOps(),
	}
}

// refresh updates the list of managed service names based on the
// sub-directories of the "init" dir (see initDir).
func (sc *serviceConfigs) refresh() error {
	names, err := sc.list()
	if err != nil {
		return errors.Trace(err)
	}
	sc.names = names
	return nil
}

// list generates a fresh list of managed service names based on the
// sub-directories of the "init" dir (see initDir).
func (sc serviceConfigs) list() ([]string, error) {
	dirnames, err := fs.ListSubdirectoriesOp(sc.baseDir, sc.fops)
	if os.IsNotExist(errors.Cause(err)) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	var names []string
	for _, name := range dirnames {
		if !hasPrefix(name, sc.prefixes...) {
			continue
		}

		info := initsystems.NewConfDirInfo(name, sc.baseDir, sc.initSystem)
		if _, err := info.Read(sc.fops); err != nil {
			continue
		}

		names = append(names, name)
	}
	return names, nil
}

// lookup returns a ConfDirInfo for the given service name if it is
// already managed by juju. If not then nil is returned.
func (sc serviceConfigs) lookup(name string) *initsystems.ConfDirInfo {
	if !contains(sc.names, name) {
		return nil
	}
	info := initsystems.NewConfDirInfo(name, sc.baseDir, sc.initSystem)
	return &info
}

// add creates a new confDir for the given service name and populates
// the directory. The service gets added to the list of managed
// services. If the service is already managed then errors.AlreadyExists
// is returned.
func (sc *serviceConfigs) add(name string, conf Conf, handler initsystems.ConfHandler) error {
	if contains(sc.names, name) {
		return errors.AlreadyExistsf("service %q", name)
	}

	if err := conf.Validate(name); err != nil {
		return errors.Trace(err)
	}

	normalConf := conf.normalize()

	info := initsystems.NewConfDirInfo(name, sc.baseDir, sc.initSystem)
	confDir, err := info.Populate(normalConf, handler)
	if err != nil {
		return errors.Trace(err)
	}

	if err := confDir.Write(sc.fops); err != nil {
		return errors.Trace(err)
	}

	sc.names = append(sc.names, name)

	return nil
}

// remove deletes the conf directory for the given service name and
// removes the name from the list of juju-managed services. If the
// service isn't already managed then errors.NotFound is returned.
func (sc *serviceConfigs) remove(name string) error {
	info := sc.lookup(name)
	if info == nil {
		return errors.NotFoundf("service %q", name)
	}

	if err := info.Remove(sc.fops.RemoveAll); err != nil {
		return errors.Trace(err)
	}

	for i, managed := range sc.names {
		if name == managed {
			sc.names = append(sc.names[:i], sc.names[i+1:]...)
			break
		}
	}
	return nil
}
