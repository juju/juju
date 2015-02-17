// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/fs"

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
	// dirName is the absolute path to the service's conf directory.
	dirName    string
	initSystem string
	fops       fs.Operations
}

func newConfDir(name, initDir, initSystem string, fops fs.Operations) *confDir {
	if fops == nil {
		fops = newFileOps()
	}

	return &confDir{
		dirName:    filepath.Join(initDir, name),
		initSystem: initSystem,
		fops:       fops,
	}
}

var newFileOps = func() fs.Operations {
	return &fs.Ops{}
}

func (cd confDir) name() string {
	return filepath.Base(cd.dirName)
}

func (cd confDir) confname() string {
	return fmt.Sprintf(filenameConf, cd.initSystem)
}

func (cd confDir) filename() string {
	return filepath.Join(cd.dirName, cd.confname())
}

func (cd confDir) validate() error {
	// The conf file must exist.
	confname := cd.confname()
	exists, err := cd.fops.Exists(filepath.Join(cd.dirName, confname))
	if !exists {
		return errors.NotValidf("%q missing conf file %q", cd.dirName, confname)
	}
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (cd confDir) create() error {
	exists, err := cd.fops.Exists(cd.dirName)
	if exists {
		return errors.AlreadyExistsf("service conf dir %q", cd.dirName)
	}
	if err != nil {
		return errors.Trace(err)
	}
	if err := cd.fops.MkdirAll(cd.dirName, 0755); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (cd confDir) readfile(name string) ([]byte, error) {
	data, err := cd.fops.ReadFile(filepath.Join(cd.dirName, name))
	return data, errors.Trace(err)
}

func (cd confDir) conf() ([]byte, error) {
	return cd.readfile(cd.confname())
}

func (cd confDir) script() ([]byte, error) {
	return cd.readfile(filenameScript)
}

func (cd confDir) writefile(name string, data []byte) (string, error) {
	filename := filepath.Join(cd.dirName, name)

	file, err := cd.fops.CreateFile(filename)
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

	if err := cd.fops.Chmod(filename, 0644); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (cd confDir) writeScript(script string) (string, error) {
	filename, err := cd.writefile(filenameScript, []byte(script))
	if err != nil {
		return "", errors.Trace(err)
	}

	if err := cd.fops.Chmod(filename, 0755); err != nil {
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
	err := cd.fops.RemoveAll(cd.dirName)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errors.Annotatef(err, "while removing conf dir for %q", cd.name())
	}
	return nil
}
