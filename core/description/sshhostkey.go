// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

type sshHostKeys struct {
	Version      int           `yaml:"version"`
	SSHHostKeys_ []*sshHostKey `yaml:"ssh-host-keys"`
}

type sshHostKey struct {
	MachineID_ string   `yaml:"machine-id"`
	Keys_      []string `yaml:"keys"`
}

// MachineID implements SSHHostKey.
func (i *sshHostKey) MachineID() string {
	return i.MachineID_
}

// Keys implements SSHHostKey.
func (i *sshHostKey) Keys() []string {
	return i.Keys_
}

// SSHHostKeyArgs is an argument struct used to create a
// new internal sshHostKey type that supports the SSHHostKey interface.
type SSHHostKeyArgs struct {
	MachineID string
	Keys      []string
}

func newSSHHostKey(args SSHHostKeyArgs) *sshHostKey {
	return &sshHostKey{
		MachineID_: args.MachineID,
		Keys_:      args.Keys,
	}
}

func importSSHHostKeys(source map[string]interface{}) ([]*sshHostKey, error) {
	checker := versionedChecker("ssh-host-keys")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "ssh-host-keys version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := sshHostKeyDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["ssh-host-keys"].([]interface{})
	return importSSHHostKeyList(sourceList, importFunc)
}

func importSSHHostKeyList(sourceList []interface{}, importFunc sshHostKeyDeserializationFunc) ([]*sshHostKey, error) {
	result := make([]*sshHostKey, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for ssh-host-key %d, %T", i, value)
		}
		sshHostKey, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "ssh-host-key %d", i)
		}
		result = append(result, sshHostKey)
	}
	return result, nil
}

type sshHostKeyDeserializationFunc func(map[string]interface{}) (*sshHostKey, error)

var sshHostKeyDeserializationFuncs = map[int]sshHostKeyDeserializationFunc{
	1: importSSHHostKeyV1,
}

func importSSHHostKeyV1(source map[string]interface{}) (*sshHostKey, error) {
	fields := schema.Fields{
		"machine-id": schema.String(),
		"keys":       schema.List(schema.String()),
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
	return &sshHostKey{
		MachineID_: valid["machine-id"].(string),
		Keys_:      keys,
	}, nil
}
