// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/schema"

	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.state.migration")

type description struct {
	// Version conceptually encapsulates an understanding of which fields
	// exist and how they are populated. As extra fields and entities are
	// added, the version should be incremented and tests written to ensure
	// that newer versions of the code are still able to create Model
	// representations from versions.
	//
	// The version is all about the serialization of the structures from
	// the migration package. Each type will likely have a version.
	version int
	model   *model
	// TODO: extra binaries...
	// Tools
	// Charms
}

type ModelArgs struct {
	Owner              names.UserTag
	Config             map[string]interface{}
	LatestToolsVersion version.Number
}

func NewDescription(args ModelArgs) Description {
	return &description{
		version: 1,
		model: &model{
			Version:             1,
			Owner_:              args.Owner.Canonical(),
			Config_:             args.Config,
			LatestToolsVersion_: args.LatestToolsVersion,
			Users_: users{
				Version: 1,
			},
			Machines_: machines{
				Version: 1,
			},
		},
	}
}

func (d *description) Model() Model {
	return d.model
}

type model struct {
	Version int `yaml:"version"`

	Owner_  string                 `yaml:"owner"`
	Config_ map[string]interface{} `yaml:"config"`

	LatestToolsVersion_ version.Number `yaml:"latest-tools,omitempty"`

	Users_    users    `yaml:"users"`
	Machines_ machines `yaml:"machines"`

	// TODO: add extra entities, but initially focus on Machines.
	// Services, and through them, Units
	// Relations
	// Spaces
	// Storage

}

func (m *model) Tag() names.EnvironTag {
	// Here we make the assumption that the environment UUID is set
	// correctly in the Config.
	value := m.Config_["uuid"]
	// Explicitly ignore the 'ok' aspect of the cast. If we don't have it
	// and it is wrong, we panic. Here we fully expect it to exist, but
	// paranoia says 'never panic', so worst case is we have an empty string.
	uuid, _ := value.(string)
	return names.NewEnvironTag(uuid)
}

func (m *model) Owner() names.UserTag {
	return names.NewUserTag(m.Owner_)
}

func (m *model) Config() map[string]interface{} {
	// TODO: consider returning a deep copy.
	return m.Config_
}

func (m *model) LatestToolsVersion() version.Number {
	return m.LatestToolsVersion_
}

// Implement length-based sort with ByLen type.
type ByName []User

func (a ByName) Len() int           { return len(a) }
func (a ByName) Less(i, j int) bool { return a[i].Name().Canonical() < a[j].Name().Canonical() }
func (a ByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

func (m *model) Users() []User {
	var result []User
	for _, user := range m.Users_.Users_ {
		result = append(result, user)
	}
	sort.Sort(ByName(result))
	return result
}

func (m *model) AddUser(args UserArgs) {
	m.Users_.Users_ = append(m.Users_.Users_, newUser(args))
}

func (m *model) setUsers(userList []*user) {
	m.Users_ = users{
		Version: 1,
		Users_:  userList,
	}
}

func (m *model) Machines() []Machine {
	var result []Machine
	for _, machine := range m.Machines_.Machines_ {
		result = append(result, machine)
	}
	return result
}

func (m *model) AddMachine(args MachineArgs) Machine {
	machine := newMachine(args)
	m.Machines_.Machines_ = append(m.Machines_.Machines_, machine)
	return machine
}

func (m *model) setMachines(machineList []*machine) {
	m.Machines_ = machines{
		Version:   1,
		Machines_: machineList,
	}
}

func (m *model) Validate() error {
	for _, machine := range m.Machines_.Machines_ {
		if err := machine.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// importModel constructs a new Model from a map that in normal usage situations
// will be the result of interpreting a large YAML document.
//
// This method is a package internal serialisation method.
func importModel(source map[string]interface{}) (*model, error) {
	version, err := getVersion(source)
	if err != nil {
		return nil, errors.Trace(err)
	}

	importFunc, ok := modelDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}

	return importFunc(source)
}

type modelDeserializationFunc func(map[string]interface{}) (*model, error)

var modelDeserializationFuncs = map[int]modelDeserializationFunc{
	1: importModelV1,
}

func importModelV1(source map[string]interface{}) (*model, error) {
	result := &model{Version: 1}

	fields := schema.Fields{
		"owner":        schema.String(),
		"config":       schema.StringMap(schema.Any()),
		"latest-tools": schema.String(),
		"users":        schema.StringMap(schema.Any()),
		"machines":     schema.StringMap(schema.Any()),
	}
	// Some values don't have to be there.
	defaults := schema.Defaults{
		"latest-tools": schema.Omit,
	}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "model v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	result.Owner_ = valid["owner"].(string)
	result.Config_ = valid["config"].(map[string]interface{})

	if availableTools, ok := valid["latest-tools"]; ok {
		num, err := version.Parse(availableTools.(string))
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.LatestToolsVersion_ = num
	}

	userMap := valid["users"].(map[string]interface{})
	users, err := importUsers(userMap)
	if err != nil {
		return nil, errors.Annotatef(err, "users")
	}
	result.setUsers(users)

	machineMap := valid["machines"].(map[string]interface{})
	machines, err := importMachines(machineMap)
	if err != nil {
		return nil, errors.Annotatef(err, "machines")
	}
	result.setMachines(machines)

	return result, nil
}
