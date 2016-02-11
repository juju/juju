// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/schema"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.state.migration")

// ModelArgs represent the bare minimum information that is needed
// to represent a model.
type ModelArgs struct {
	Owner              names.UserTag
	Config             map[string]interface{}
	LatestToolsVersion version.Number
}

// NewModel returns a Model based on the args specified.
func NewModel(args ModelArgs) Model {
	m := &model{
		Version:             1,
		Owner_:              args.Owner.Canonical(),
		Config_:             args.Config,
		LatestToolsVersion_: args.LatestToolsVersion,
	}
	m.setUsers(nil)
	m.setMachines(nil)
	m.setServices(nil)
	return m
}

// DeserializeModel constructs a Model from a serialized
// YAML byte stream. The normal use for this is to construct
// the Model representation after getting the byte stream from
// an API connection or read from a file.
func DeserializeModel(bytes []byte) (Model, error) {
	var source map[string]interface{}
	err := yaml.Unmarshal(bytes, &source)
	if err != nil {
		return nil, errors.Trace(err)
	}

	model, err := importModel(source)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return model, nil
}

type model struct {
	Version int `yaml:"version"`

	Owner_  string                 `yaml:"owner"`
	Config_ map[string]interface{} `yaml:"config"`

	LatestToolsVersion_ version.Number `yaml:"latest-tools,omitempty"`

	Users_    users    `yaml:"users"`
	Machines_ machines `yaml:"machines"`
	Services_ services `yaml:"services"`

	// TODO: add extra entities, but initially focus on Machines.
	// Services, and through them, Units
	// Relations
	// Spaces
	// Storage

}

func (m *model) Tag() names.ModelTag {
	// Here we make the assumption that the environment UUID is set
	// correctly in the Config.
	value := m.Config_["uuid"]
	// Explicitly ignore the 'ok' aspect of the cast. If we don't have it
	// and it is wrong, we panic. Here we fully expect it to exist, but
	// paranoia says 'never panic', so worst case is we have an empty string.
	uuid, _ := value.(string)
	return names.NewModelTag(uuid)
}

// Owner implements Model.
func (m *model) Owner() names.UserTag {
	return names.NewUserTag(m.Owner_)
}

// Config implements Model.
func (m *model) Config() map[string]interface{} {
	// TODO: consider returning a deep copy.
	return m.Config_
}

// LatestToolsVersion implements Model.
func (m *model) LatestToolsVersion() version.Number {
	return m.LatestToolsVersion_
}

// Implement length-based sort with ByLen type.
type ByName []User

func (a ByName) Len() int           { return len(a) }
func (a ByName) Less(i, j int) bool { return a[i].Name().Canonical() < a[j].Name().Canonical() }
func (a ByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

// Users implements Model.
func (m *model) Users() []User {
	var result []User
	for _, user := range m.Users_.Users_ {
		result = append(result, user)
	}
	sort.Sort(ByName(result))
	return result
}

// AddUser implements Model.
func (m *model) AddUser(args UserArgs) {
	m.Users_.Users_ = append(m.Users_.Users_, newUser(args))
}

func (m *model) setUsers(userList []*user) {
	m.Users_ = users{
		Version: 1,
		Users_:  userList,
	}
}

// Machines implements Model.
func (m *model) Machines() []Machine {
	var result []Machine
	for _, machine := range m.Machines_.Machines_ {
		result = append(result, machine)
	}
	return result
}

// AddMachine implements Model.
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

// Services implements Model.
func (m *model) Services() []Service {
	var result []Service
	for _, service := range m.Services_.Services_ {
		result = append(result, service)
	}
	return result
}

// AddService implements Model.
func (m *model) AddService(args ServiceArgs) Service {
	service := newService(args)
	m.Services_.Services_ = append(m.Services_.Services_, service)
	return service
}

func (m *model) setServices(serviceList []*service) {
	m.Services_ = services{
		Version:   1,
		Services_: serviceList,
	}
}

// Validate implements Model.
func (m *model) Validate() error {
	for _, machine := range m.Machines_.Machines_ {
		if err := machine.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	for _, service := range m.Services_.Services_ {
		if err := service.Validate(); err != nil {
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
		"services":     schema.StringMap(schema.Any()),
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

	serviceMap := valid["services"].(map[string]interface{})
	services, err := importServices(serviceMap)
	if err != nil {
		return nil, errors.Annotatef(err, "services")
	}
	result.setServices(services)

	return result, nil
}
