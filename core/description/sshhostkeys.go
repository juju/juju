// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

type sshhostkeys struct {
	Version      int           `yaml:"version"`
	SSHHostKeys_ []*sshhostkey `yaml:"sshhostkeys"`
}

type sshhostkey struct {
	MachineID_ string   `yaml:"machineid"`
	Keys_      []string `yaml:"keys"`
}

// MachineID implements SSHHostKey.
func (i *sshhostkey) MachineID() string {
	return i.MachineID_
}

// Keys implements SSHHostKey.
func (i *sshhostkey) Keys() []string {
	return i.Keys_
}

// SSHHostKeyArgs is an argument struct used to create a
// new internal sshhostkey type that supports the SSHHostKey interface.
type SSHHostKeyArgs struct {
	MachineID string
	Keys      []string
}

func newSSHHostKey(args SSHHostKeyArgs) *sshhostkey {
	return &sshhostkey{
		MachineID_: args.MachineID,
		Keys_:      args.Keys,
	}
}

func importSSHHostKeys(source map[string]interface{}) ([]*sshhostkey, error) {
	checker := versionedChecker("sshhostkeys")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "sshhostkeys version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := sshhostkeyDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["sshhostkeys"].([]interface{})
	return importSSHHostKeyList(sourceList, importFunc)
}

func importSSHHostKeyList(sourceList []interface{}, importFunc sshhostkeyDeserializationFunc) ([]*sshhostkey, error) {
	result := make([]*sshhostkey, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for sshhostkey %d, %T", i, value)
		}
		sshhostkey, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "sshhostkey %d", i)
		}
		result = append(result, sshhostkey)
	}
	return result, nil
}

type sshhostkeyDeserializationFunc func(map[string]interface{}) (*sshhostkey, error)

var sshhostkeyDeserializationFuncs = map[int]sshhostkeyDeserializationFunc{
	1: importSSHHostKeyV1,
}

func importSSHHostKeyV1(source map[string]interface{}) (*sshhostkey, error) {
	fields := schema.Fields{
		"machineid": schema.String(),
		"keys":      schema.List(schema.String()),
	}
	// Some values don't have to be there.
	defaults := schema.Defaults{}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "sshhostkey v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	keysInterface := valid["keys"].([]interface{})
	keys := make([]string, len(keysInterface))
	for i, d := range keysInterface {
		keys[i] = d.(string)
	}
	return &sshhostkey{
		MachineID_: valid["machineid"].(string),
		Keys_:      keys,
	}, nil
}
