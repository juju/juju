// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package utils

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"

	"gopkg.in/yaml.v2"
)

// WriteYaml marshals obj as yaml to a temporary file in the same directory
// as path, than atomically replaces path with the temporary file.
func WriteYaml(path string, obj interface{}) error {
	data, err := yaml.Marshal(obj)
	if err != nil {
		return errors.Trace(err)
	}
	dir := filepath.Dir(path)
	f, err := ioutil.TempFile(dir, "juju")
	if err != nil {
		return errors.Trace(err)
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()      // don't leak file handle
		os.Remove(tmp) // don't leak half written files on disk
		return errors.Trace(err)
	}
	// Explicitly close the file before moving it. This is needed on Windows
	// where the OS will not allow us to move a file that still has an open
	// file handle. Must check the error on close because filesystems can delay
	// reporting errors until the file is closed.
	if err := f.Close(); err != nil {
		os.Remove(tmp) // don't leak half written files on disk
		return errors.Trace(err)
	}

	// ioutils.TempFile creates files 0600, but this function has a contract
	// that files will be world readable, 0644 after replacement.
	if err := os.Chmod(tmp, 0644); err != nil {
		os.Remove(tmp) // remove file with incorrect permissions.
		return errors.Trace(err)
	}

	return ReplaceFile(tmp, path)
}

// ReadYaml unmarshals the yaml contained in the file at path into obj. See
// goyaml.Unmarshal. If path is not found, the error returned will be compatible
// with os.IsNotExist.
func ReadYaml(path string, obj interface{}) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err // cannot wrap here because callers check for NotFound.
	}
	return yaml.Unmarshal(data, obj)
}

// ConformYAML ensures all keys of any nested maps are strings.  This is
// necessary because YAML unmarshals map[interface{}]interface{} in nested
// maps, which cannot be serialized by json or bson. Also, handle
// []interface{}. cf. gopkg.in/juju/charm.v4/actions.go cleanse
func ConformYAML(input interface{}) (interface{}, error) {
	switch typedInput := input.(type) {

	case map[string]interface{}:
		newMap := make(map[string]interface{})
		for key, value := range typedInput {
			newValue, err := ConformYAML(value)
			if err != nil {
				return nil, err
			}
			newMap[key] = newValue
		}
		return newMap, nil

	case map[interface{}]interface{}:
		newMap := make(map[string]interface{})
		for key, value := range typedInput {
			typedKey, ok := key.(string)
			if !ok {
				return nil, errors.New("map keyed with non-string value")
			}
			newMap[typedKey] = value
		}
		return ConformYAML(newMap)

	case []interface{}:
		newSlice := make([]interface{}, len(typedInput))
		for i, sliceValue := range typedInput {
			newSliceValue, err := ConformYAML(sliceValue)
			if err != nil {
				return nil, errors.New("map keyed with non-string value")
			}
			newSlice[i] = newSliceValue
		}
		return newSlice, nil

	default:
		return input, nil
	}
}
