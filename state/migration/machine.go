// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/schema"

	"github.com/juju/juju/version"
)

type machines struct {
	Version   int        `yaml:"version"`
	Machines_ []*machine `yaml:"machines"`
}

type machine struct {
	Id_ string `yaml:"id"`

	Nonce_             string         `yaml:"nonce"`
	PasswordHash_      string         `yaml:"password-hash"`
	Placement_         string         `yaml:"placement,omitempty"`
	Instance_          *cloudInstance `yaml:"instance"`
	Series_            string         `yaml:"series"`
	ContainerType_     string         `yaml:"container-type,omitempty"`
	ProviderAddresses_ []*address     `yaml:"provider-addresses"`
	MachineAddresses_  []*address     `yaml:"machine-addresses"`

	PreferredPublicAddress_  *address `yaml:"preferred-public-address"`
	PreferredPrivateAddress_ *address `yaml:"preferred-private-address"`

	Tools_ AgentTools `yaml:"tools"`
	Jobs_  []string   `yaml:"jobs"`

	SupportedContainers_    []string `yaml:"supported-containers,omitempty"`
	SupportedContainersSet_ bool     `yaml:"supported-containers-set,omitempty"`

	Containers_ []*machine `yaml:"containers"`
}

// Keeping the agentTools with the machine code, because we hope
// that one day we will succeed in merging the unit agents with the
// machine agents.
type agentTools struct {
	Version_ version.Binary `yaml:"version"`
	URL_     string         `yaml:"url"`
	SHA256_  string         `yaml:"sha256"`
	Size_    int64          `yaml:"size"`
}

type cloudInstance struct {
	Version int `yaml:"version"`

	InstanceId_ string `yaml:"instance-id"`
	Status_     string `yaml:"status"`
	// For all the optional values, empty values make no sense, and
	// it would be better to have them not set rather than set with
	// a nonsense value.
	Architecture_     string   `yaml:"architecture,omitempty"`
	Memory_           uint64   `yaml:"memory,omitempty"`
	RootDisk_         uint64   `yaml:"root-disk,omitempty"`
	CpuCores_         uint64   `yaml:"cpu-cores,omitempty"`
	CpuPower_         uint64   `yaml:"cpu-power,omitempty"`
	Tags_             []string `yaml:"tags,omitempty"`
	AvailabilityZone_ string   `yaml:"availability-zone,omitempty"`
}

func (m *machine) Id() names.MachineTag {
	return names.NewMachineTag(m.Id_)
}

func (m *machine) Nonce() string {
	return m.Nonce_
}

func (m *machine) PasswordHash() string {
	return m.PasswordHash_
}

func (m *machine) Placement() string {
	return m.Placement_
}

func (m *machine) Instance() CloudInstance {
	return m.Instance_
}

func (m *machine) Series() string {
	return m.Series_
}

func (m *machine) ContainerType() string {
	return m.ContainerType_
}

func (m *machine) ProviderAddresses() []Address {
	var result []Address
	for _, addr := range m.ProviderAddresses_ {
		result = append(result, addr)
	}
	return result
}

func (m *machine) MachineAddresses() []Address {
	var result []Address
	for _, addr := range m.MachineAddresses_ {
		result = append(result, addr)
	}
	return result
}

func (m *machine) PreferredPublicAddress() Address {
	return m.PreferredPublicAddress_
}

func (m *machine) PreferredPrivateAddress() Address {
	return m.PreferredPrivateAddress_
}

func (m *machine) Tools() AgentTools {
	return m.Tools_
}

func (m *machine) Jobs() []string {
	return m.Jobs_
}

func (m *machine) SupportedContainers() ([]string, bool) {
	return m.SupportedContainers_, m.SupportedContainersSet_
}

func (m *machine) Containers() []Machine {
	var result []Machine
	for _, container := range m.Containers_ {
		result = append(result, container)
	}
	return result
}

func importMachines(source map[string]interface{}) ([]*machine, error) {
	checker := versionedChecker("machines")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "machines version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := machineDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["machines"].([]interface{})
	return importMachineList(sourceList, importFunc)
}

func importMachineList(sourceList []interface{}, importFunc machineDeserializationFunc) ([]*machine, error) {
	result := make([]*machine, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for machine %d, %T", i, value)
		}
		machine, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "machine %d", i)
		}
		result = append(result, machine)
	}
	return result, nil
}

type machineDeserializationFunc func(map[string]interface{}) (*machine, error)

var machineDeserializationFuncs = map[int]machineDeserializationFunc{
	1: importMachineV1,
}

func importMachineV1(source map[string]interface{}) (*machine, error) {
	result := &machine{}

	fields := schema.Fields{
		"id":         schema.String(),
		"instance":   schema.StringMap(schema.Any()),
		"containers": schema.List(schema.StringMap(schema.Any())),
	}
	checker := schema.FieldMap(fields, nil) // no defaults

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "machine v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	result.Id_ = valid["id"].(string)

	instance, err := importCloudInstance(valid["instance"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Trace(err)
	}
	result.Instance_ = instance

	machineList := valid["containers"].([]interface{})
	machines, err := importMachineList(machineList, importMachineV1)
	if err != nil {
		return nil, errors.Annotatef(err, "containers")
	}
	result.Containers_ = machines

	return result, nil

}

func (c *cloudInstance) InstanceId() string {
	return c.InstanceId_
}

func (c *cloudInstance) Status() string {
	return c.Status_
}

func (c *cloudInstance) Architecture() string {
	return c.Architecture_
}

func (c *cloudInstance) Memory() uint64 {
	return c.Memory_
}

func (c *cloudInstance) RootDisk() uint64 {
	return c.RootDisk_
}

func (c *cloudInstance) CpuCores() uint64 {
	return c.CpuCores_
}

func (c *cloudInstance) CpuPower() uint64 {
	return c.CpuPower_
}

func (c *cloudInstance) Tags() []string {
	return c.Tags_
}

func (c *cloudInstance) AvailabilityZone() string {
	return c.AvailabilityZone_
}

// importAddress constructs a new Address from a map that in normal usage situations
// will be the result of interpreting a large YAML document.
//
// This method is a package internal serialisation method.
func importCloudInstance(source map[string]interface{}) (*cloudInstance, error) {
	version, err := getVersion(source)
	if err != nil {
		return nil, errors.Annotate(err, "cloudInstance version schema check failed")
	}

	importFunc, ok := cloudInstanceDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}

	return importFunc(source)
}

type cloudInstanceDeserializationFunc func(map[string]interface{}) (*cloudInstance, error)

var cloudInstanceDeserializationFuncs = map[int]cloudInstanceDeserializationFunc{
	1: importCloudInstanceV1,
}

func importCloudInstanceV1(source map[string]interface{}) (*cloudInstance, error) {
	fields := schema.Fields{
		"instance-id":       schema.String(),
		"status":            schema.String(),
		"architecture":      schema.String(),
		"memory":            schema.Uint(),
		"root-disk":         schema.Uint(),
		"cpu-cores":         schema.Uint(),
		"cpu-power":         schema.Uint(),
		"tags":              schema.List(schema.String()),
		"availability-zone": schema.String(),
	}
	// Some values don't have to be there.
	defaults := schema.Defaults{
		"architecture":      "",
		"memory":            uint64(0),
		"root-disk":         uint64(0),
		"cpu-cores":         uint64(0),
		"cpu-power":         uint64(0),
		"tags":              schema.Omit,
		"availability-zone": "",
	}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "cloudInstance v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	var tags []string
	if vtags, ok := valid["tags"]; ok {
		tags = vtags.([]string)
	}

	return &cloudInstance{
		Version:           1,
		InstanceId_:       valid["instance-id"].(string),
		Status_:           valid["status"].(string),
		Architecture_:     valid["architecture"].(string),
		Memory_:           valid["memory"].(uint64),
		RootDisk_:         valid["root-disk"].(uint64),
		CpuCores_:         valid["cpu-cores"].(uint64),
		CpuPower_:         valid["cpu-power"].(uint64),
		Tags_:             tags,
		AvailabilityZone_: valid["availability-zone"].(string),
	}, nil
}
