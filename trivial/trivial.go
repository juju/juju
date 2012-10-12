package trivial

import (
	"errors"
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"os"
)

// WriteYaml marshals obj as yaml and then writes it to a file, atomically,
// by first writing a sibling with the suffix ".preparing" and then moving
// the sibling to the real path.
func WriteYaml(path string, obj interface{}) error {
	data, err := goyaml.Marshal(obj)
	if err != nil {
		return err
	}
	prep := path + ".preparing"
	f, err := os.OpenFile(prep, os.O_WRONLY|os.O_CREATE|os.O_SYNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	return os.Rename(prep, path)
}

// ReadYaml unmarshals the yaml contained in the file at path into obj. See
// goyaml.Unmarshal.
func ReadYaml(path string, obj interface{}) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	return goyaml.Unmarshal(data, obj)
}

// ErrorContextf prefixes any error stored in err with text formatted
// according to the format specifier. If err does not contain an error,
// ErrorContextf does nothing.
func ErrorContextf(err *error, format string, args ...interface{}) {
	if *err != nil {
		*err = errors.New(fmt.Sprintf(format, args...) + ": " + (*err).Error())
	}
}
