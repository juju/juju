// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"os"
	"path/filepath"

	"github.com/juju/errors"

	"github.com/juju/juju/service/initsystems"
)

const (
	initDir = "init"
)

type serviceConfigs struct {
	baseDir    string
	initSystem string
	prefixes   []string

	names []string
	fops  fileOperations
}

func newConfigs(baseDir, initSystem string, prefixes ...string) *serviceConfigs {
	if len(prefixes) == 0 {
		prefixes = jujuPrefixes
	}
	// TODO(ericsnow) Fail if the provided baseDir does not exist?
	return &serviceConfigs{
		baseDir:    filepath.Join(baseDir, initDir),
		initSystem: initSystem,
		prefixes:   prefixes,
		fops:       newFileOps(),
	}
}

func (sc serviceConfigs) newDir(name string) *confDir {
	confDir := newConfDir(name, sc.baseDir, sc.initSystem, sc.fops)
	return confDir
}

func (sc *serviceConfigs) refresh() error {
	names, err := sc.list()
	if err != nil {
		return errors.Trace(err)
	}
	sc.names = names
	return nil
}

func (sc serviceConfigs) list() ([]string, error) {
	dirnames, err := listSubdirectories(sc.baseDir, sc.fops)
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

		dir := sc.newDir(name)
		if err := dir.validate(); err == nil {
			names = append(names, name)
		}
	}
	return names, nil
}

func (sc serviceConfigs) lookup(name string) *confDir {
	if !contains(sc.names, name) {
		return nil
	}
	return sc.newDir(name)
}

type serializer interface {
	Serialize(name string, conf initsystems.Conf) ([]byte, error)
}

func (sc *serviceConfigs) add(name string, conf Conf, serializer serializer) error {
	if contains(sc.names, name) {
		return errors.AlreadyExistsf("service %q", name)
	}

	confDir := sc.newDir(name)
	if err := confDir.create(); err != nil {
		return errors.Trace(err)
	}

	normalConf, err := confDir.normalizeConf(conf)
	if err != nil {
		return errors.Trace(err)
	}

	var data []byte
	for {
		data, err = serializer.Serialize(name, *normalConf)
		if err == nil {
			break
		}

		if err := normalConf.Repair(err); err != nil {
			return errors.Trace(err)
		}
	}

	if err := confDir.writeConf(data); err != nil {
		return errors.Trace(err)
	}

	sc.names = append(sc.names, name)

	return nil
}

func (sc *serviceConfigs) remove(name string) error {
	confDir := sc.lookup(name)
	if confDir == nil {
		return errors.NotFoundf("service %q", name)
	}

	if err := confDir.remove(); err != nil {
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
