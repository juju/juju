// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"io"
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

// confFileOperations exposes the parts of fs.Operations used by confDir.
type confFileOperations interface {
	// Exists implements fs.Operations.
	Exists(string) (bool, error)

	// MkdirAll implements fs.Operations.
	MkdirAll(string, os.FileMode) error

	// ReadFile implements fs.Operations.
	ReadFile(string) ([]byte, error)

	// CreateFile implements fs.Operations.
	CreateFile(string) (io.WriteCloser, error)

	// Chmod implements fs.Operations.
	Chmod(string, os.FileMode) error

	// RemoveAll implements fs.Operations.
	RemoveAll(string) error
}

// confDir holds information about a service's conf directory. That
// directory will typically be found in the "init" subdirectory of the
// juju datadir (e.g. /var/lib/juju).
type confDir struct {
	// dirName is the absolute path to the service's conf directory.
	dirName string

	// initSystem identifies to which init system the confDir relates.
	initSystem string

	// fops is the set of file operations used by confDir.
	fops confFileOperations
}

// newConfDir builds a new confDir based on the provided info and
// returns it.
func newConfDir(name, initDir, initSystem string, fops confFileOperations) *confDir {
	if fops == nil {
		fops = newFileOps()
	}

	return &confDir{
		dirName:    filepath.Join(initDir, name),
		initSystem: initSystem,
		fops:       fops,
	}
}

// name returns the name of the service to which the confDir corresponds.
func (cd confDir) name() string {
	return filepath.Base(cd.dirName)
}

// confName returns the base name of the conf file. It is specific to
// to the confDir's init system.
func (cd confDir) confName() string {
	return fmt.Sprintf(filenameConf, cd.initSystem)
}

// filename returns the path to the conf file.
func (cd confDir) filename() string {
	return filepath.Join(cd.dirName, cd.confName())
}

// validate checks the confDir to ensure the directory has all the
// appropriate files.
func (cd confDir) validate() error {
	// The conf file must exist.
	confName := cd.confName()
	exists, err := cd.fops.Exists(filepath.Join(cd.dirName, confName))
	if !exists {
		return errors.NotValidf("%q missing conf file %q", cd.dirName, confName)
	}
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

// create creates the confDir's directory. If it already exists then it
// fails with errors.AlreadyExists.
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

// readFile reads the contents from the named file (relative to the
// confDir's directory) and returns it.
func (cd confDir) readFile(name string) ([]byte, error) {
	data, err := cd.fops.ReadFile(filepath.Join(cd.dirName, name))
	return data, errors.Trace(err)
}

// conf returns the contents of the confDir's conf file.
func (cd confDir) conf() ([]byte, error) {
	return cd.readFile(cd.confName())
}

// script returns the contents of the confDir's script file.
func (cd confDir) script() ([]byte, error) {
	return cd.readFile(filenameScript)
}

// writeFile writes the provided data to the named file (relative to the
// confDir's directory) and returns the full filename.
func (cd confDir) writeFile(name string, data []byte) (string, error) {
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

// writeConf writes the provided data to the confDir's conf file.
func (cd confDir) writeConf(data []byte) error {
	filename, err := cd.writeFile(cd.confName(), data)
	if err != nil {
		return errors.Trace(err)
	}

	if err := cd.fops.Chmod(filename, 0644); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// writeScript writes the provided data to the confDir's script file.
func (cd confDir) writeScript(script string) (string, error) {
	filename, err := cd.writeFile(filenameScript, []byte(script))
	if err != nil {
		return "", errors.Trace(err)
	}

	if err := cd.fops.Chmod(filename, 0755); err != nil {
		return "", errors.Trace(err)
	}

	return filename, nil
}

// normalizeConf turns the provided Conf into the more generic
// initsystems.Conf, making it fit for consumption by InitSystem
// implementations. This may include writing out the conf.Cmd (and
// conf.ExtraScript) to a script file.
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

// isSimpleScript checks the provided script to see if it is what
// confDir considers "simple". In the context of confDir, "simple" means
// it is a single line. A "simple" script will remain in Conf.Cmd, while
// a non-simple one will be written out to a script file and the path to
// that file stored in Conf.Cmd.
func (cd confDir) isSimpleScript(script string) bool {
	if strings.Contains(script, "\n") {
		return false
	}
	return true
}

// remove deletes the confDir's directory.
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
