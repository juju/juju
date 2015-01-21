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
	_, err := stat(filepath.Join(cd.dirname, confname))
	if os.IsNotExist(err) {
		return errors.NotValidf("%q missing conf file %q", cd.dirname, confname)
	}
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

var stat = os.Stat

func (cd confDir) readfile(name string) (string, error) {
	data, err := readfile(filepath.Join(cd.dirname, name))
	return string(data), errors.Trace(err)
}

var readfile = ioutil.ReadFile

func (cd confDir) conf() (string, error) {
	return cd.readfile(cd.confname())
}

func (cd confDir) script() (string, error) {
	return cd.readfile(filenameScript)
}

func (cd confDir) write(conf *common.Conf, init common.InitSystem) error {
	_, err := stat(cd.dirname)
	if err == nil {
		return errors.AlreadyExistsf("service conf dir %q", cd.dirname)
	}
	if !os.IsNotExist(err) {
		return errors.Trace(err)
	}

	// Make a copy so we don't mutate.
	copied := *conf
	conf = &copied

	// Write out the script if necessary.
	script, err := conf.Script()
	if err != nil {
		return errors.Trace(err)
	}
	conf.Cmd = script
	conf.ExtraScript = ""
	if !cd.isSimple(script) {
		filename, err := cd.writeScript(script)
		if err != nil {
			return errors.Trace(err)
		}
		conf.Cmd = filename
	}

	// Write out the conf.
	data, err := init.Serialize(conf)
	if err != nil {
		return errors.Trace(err)
	}
	file, err := create(cd.filename())
	if err != nil {
		return errors.Trace(err)
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (cd confDir) isSimple(script string) bool {
	if strings.Contains(script, "\n") {
		return false
	}
	return true
}

func (cd confDir) writeScript(script string) (string, error) {
	filename := filepath.Join(cd.dirname, filenameScript)

	file, err := create(filename)
	if err != nil {
		return "", errors.Annotate(err, "while writing script")
	}
	defer file.Close()

	if _, err := file.Write([]byte(script)); err != nil {
		return "", errors.Trace(err)
	}

	return filename, nil
}

var create = func(filename string) (io.WriteCloser, error) {
	return os.Create(filename)
}

func (cd confDir) remove() error {
	if err := remove(cd.dirname); err != nil {
		return errors.Annotatef(err, "while removing conf dir for %q", cd.name())
	}
	return nil
}

var remove = os.RemoveAll
