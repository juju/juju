// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/service/initsystems"
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

func newConfDir(name, initDir, initSystem string, fops fileOperations) *confDir {
	if fops == nil {
		fops = newFileOps()
	}

	return &confDir{
		dirname:    filepath.Join(initDir, name),
		initSystem: initSystem,
		fops:       fops,
	}
}

var newFileOps = func() fileOperations {
	return &fileOps{}
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
	if err := cd.fops.mkdirAll(cd.dirname, 0755); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (cd confDir) readfile(name string) ([]byte, error) {
	data, err := cd.fops.readFile(filepath.Join(cd.dirname, name))
	return data, errors.Trace(err)
}

func (cd confDir) conf() ([]byte, error) {
	return cd.readfile(cd.confname())
}

func (cd confDir) script() ([]byte, error) {
	return cd.readfile(filenameScript)
}

func (cd confDir) writefile(name string, data []byte) (string, error) {
	// TODO(ericsnow) Fail if the file already exists?
	// TODO(ericsnow) Create with desired permissions?

	filename := filepath.Join(cd.dirname, name)

	file, err := cd.fops.createFile(filename)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		return "", errors.Trace(err)
	}

	return filename, nil
}

func (cd confDir) writeConf(data []byte) error {
	filename, err := cd.writefile(cd.confname(), data)
	if err != nil {
		return errors.Trace(err)
	}

	// TODO(ericsnow) Are these the right permissions?
	if err := cd.fops.chmod(filename, 0644); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (cd confDir) writeScript(script string) (string, error) {
	filename, err := cd.writefile(filenameScript, []byte(script))
	if err != nil {
		return "", errors.Trace(err)
	}

	// TODO(ericsnow) Are these the right permissions?
	if err := cd.fops.chmod(filename, 0755); err != nil {
		return "", errors.Trace(err)
	}

	return filename, nil
}

func (cd confDir) normalizeConf(conf Conf) (*initsystems.Conf, error) {
	// Write out the script if necessary.
	script, err := conf.Script()
	if err != nil {
		return nil, errors.Trace(err)
	}
	conf.Cmd = script
	conf.ExtraScript = ""
	if !cd.isSimpleScript(script) {
		filename, err := cd.writeScript(script)
		if err != nil {
			return nil, errors.Trace(err)
		}
		conf.Cmd = filename
	}

	normalConf, err := conf.normalize()
	return normalConf, errors.Trace(err)
}

func (cd confDir) isSimpleScript(script string) bool {
	if strings.Contains(script, "\n") {
		return false
	}
	return true
}

func (cd confDir) remove() error {
	err := cd.fops.removeAll(cd.dirname)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errors.Annotatef(err, "while removing conf dir for %q", cd.name())
	}
	return nil
}
