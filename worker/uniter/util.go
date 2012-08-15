package uniter

import (
	"errors"
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"os"
	"strconv"
	"strings"
)

// unitName converts a unit filesystem name to a unit name. See unitFsName.
func unitName(filename string) (string, bool) {
	i := strings.LastIndex(filename, "-")
	if i == -1 {
		return "", false
	}
	svcName := filename[:i]
	unitId := filename[i+1:]
	if _, err := strconv.Atoi(unitId); err != nil {
		return "", false
	}
	return svcName + "/" + unitId, true
}

// unitFsName returns a variation on the supplied unit name that can be used in
// a filesystem path. See unitName.
func unitFsName(unitName string) string {
	return strings.Replace(unitName, "/", "-", 1)
}

// atomicWrite marshals obj as yaml and then writes it to a file atomically
// by first writing a sibling with the suffix ".preparing", and then moving
// the sibling to the real path.
func atomicWrite(path string, obj interface{}) error {
	data, err := goyaml.Marshal(obj)
	if err != nil {
		return err
	}
	preparing := ".preparing"
	if err = ioutil.WriteFile(path+preparing, data, 0644); err != nil {
		return err
	}
	return os.Rename(path+preparing, path)
}

// ensureDir creates the directory at path if it doesn't already exist.
func ensureDir(path string) error {
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		return os.Mkdir(path, 0755)
	} else if !fi.IsDir() {
		return fmt.Errorf("%s must be a directory", path)
	}
	return nil
}

// errorContextf prefixes any error stored in err with text formatted
// according to the format specifier. If err does not contain an error,
// errorContextf does nothing.
func errorContextf(err *error, format string, args ...interface{}) {
	if *err != nil {
		*err = errors.New(fmt.Sprintf(format, args...) + ": " + (*err).Error())
	}
}
