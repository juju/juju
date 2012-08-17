package trivial

import (
	"errors"
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"os"
)

// AtomicWrite marshals obj as yaml and then writes it to a file atomically
// by first writing a sibling with the suffix ".preparing", and then moving
// the sibling to the real path.
func AtomicWrite(path string, obj interface{}) error {
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

// EnsureDir creates the directory at path if it doesn't already exist.
func EnsureDir(path string) error {
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		return os.MkdirAll(path, 0755)
	} else if !fi.IsDir() {
		return fmt.Errorf("%s must be a directory", path)
	}
	return nil
}

// ErrorContextf prefixes any error stored in err with text formatted
// according to the format specifier. If err does not contain an error,
// ErrorContextf does nothing.
func ErrorContextf(err *error, format string, args ...interface{}) {
	if *err != nil {
		*err = errors.New(fmt.Sprintf(format, args...) + ": " + (*err).Error())
	}
}
