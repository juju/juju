// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/fs"

	"github.com/juju/juju/service/common"
)

// These are the filenames that may be in a conf directory.
const (
	filenameConf   = "%s.conf"
	filenameScript = "script.sh"
)

// confDir holds information about a service's conf directory. That
// directory will typically be found in the "init" subdirectory of the
// juju datadir (e.g. /var/lib/juju).
type confDir struct {
	// dirname is the absolute path to the service's conf directory.
	dirname    string
	initSystem string
	fops       fileOperations
}

func newConfDir(name, initDir, initSystem string) *confDir {
	return &confDir{
		dirname:    filepath.Join(initDir, name),
		initSystem: initSystem,
		fops:       &fileOps{},
	}
}

func (cd confDir) name() string {
	return filepath.Base(cd.dirname)
}

func (cd confDir) confname() string {
	return fmt.Sprintf(filenameConf, cd.initSystem)
}

func (cd confDir) filename() string {
	return filepath.Join(cd.dirname, cd.confname())
}

func (cd confDir) validate() error {
	// TODO(ericsnow) Loop through contents of dir?

	// The conf file must exist.
	confname := cd.confname()
	exists, err := cd.fops.exists(filepath.Join(cd.dirname, confname))
	if !exists {
		return errors.NotValidf("%q missing conf file %q", cd.dirname, confname)
	}
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (cd confDir) create() error {
	exists, err := cd.fops.exists(cd.dirname)
	if exists {
		// TODO(ericsnow) Allow if using a different init system?
		return errors.AlreadyExistsf("service conf dir %q", cd.dirname)
	}
	if err != nil {
		return errors.Trace(err)
	}
	// TODO(ericsnow) Are these the right permissions?
	if err := cd.fops.mkdirs(cd.dirname, 0777); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (cd confDir) readfile(name string) (string, error) {
	data, err := cd.fops.readfile(filepath.Join(cd.dirname, name))
	return string(data), errors.Trace(err)
}

func (cd confDir) conf() (string, error) {
	return cd.readfile(cd.confname())
}

func (cd confDir) script() (string, error) {
	return cd.readfile(filenameScript)
}

func (cd confDir) writeConf(conf *common.Conf, data []byte) error {
	// Handle any extraneous files.
	conf, err := cd.normalizeConf(conf)
	if err != nil {
		return errors.Trace(err)
	}

	// Write out the conf.
	file, err := cd.fops.create(cd.filename())
	if err != nil {
		return errors.Trace(err)
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (cd confDir) normalizeConf(conf *common.Conf) (*common.Conf, error) {
	// Make a copy so we don't mutate.
	copied := *conf
	conf = &copied

	// Write out the script if necessary.
	script, err := conf.Script()
	if err != nil {
		return nil, errors.Trace(err)
	}
	conf.Cmd = script
	conf.ExtraScript = ""
	if !cd.isSimple(script) {
		filename, err := cd.writeScript(script)
		if err != nil {
			return nil, errors.Trace(err)
		}
		conf.Cmd = filename
	}
	return conf, nil
}

func (cd confDir) isSimple(script string) bool {
	if strings.Contains(script, "\n") {
		return false
	}
	return true
}

func (cd confDir) writeScript(script string) (string, error) {
	filename := filepath.Join(cd.dirname, filenameScript)

	file, err := cd.fops.create(filename)
	if err != nil {
		return "", errors.Annotate(err, "while writing script")
	}
	defer file.Close()

	if _, err := file.Write([]byte(script)); err != nil {
		return "", errors.Trace(err)
	}

	// TODO(ericsnow) Set the proper permissions.

	return filename, nil
}

func (cd confDir) remove() error {
	err := cd.fops.remove(cd.dirname)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errors.Annotatef(err, "while removing conf dir for %q", cd.name())
	}
	return nil
}
