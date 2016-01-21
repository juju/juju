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
	Id_                string         `yaml:"id"`
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

	Tools_ *agentTools `yaml:"tools"`
	Jobs_  []string    `yaml:"jobs"`

	SupportedContainers_ *[]string `yaml:"supported-containers,omitempty"`

	Containers_ []*machine `yaml:"containers"`
}

type MachineArgs struct {
	Id            names.MachineTag
	Nonce         string
	PasswordHash  string
	Placement     string
	Series        string
	ContainerType string
	Jobs          []string
	// A null value means that we don't yet know which containers
	// are supported. An empty slice means 'no containers are supported'.
	SupportedContainers *[]string
}

func newMachine(args MachineArgs) *machine {
	var jobs []string
	if count := len(args.Jobs); count > 0 {
		jobs = make([]string, count)
		copy(jobs, args.Jobs)
	}
	m := &machine{
		Id_:            args.Id.Id(),
		Nonce_:         args.Nonce,
		PasswordHash_:  args.PasswordHash,
		Placement_:     args.Placement,
		Series_:        args.Series,
		ContainerType_: args.ContainerType,
		Jobs_:          jobs,
	}
	if args.SupportedContainers != nil {
		supported := make([]string, len(*args.SupportedContainers))
		copy(supported, *args.SupportedContainers)
		m.SupportedContainers_ = &supported
	}
	return m
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

func (m *machine) SetInstance(args CloudInstanceArgs) {
	m.Instance_ = newCloudInstance(args)
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
	if m.SupportedContainers_ == nil {
		return nil, false
	}
	return *m.SupportedContainers_, true
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
		"id":             schema.String(),
		"nonce":          schema.String(),
		"password-hash":  schema.String(),
		"placement":      schema.String(),
		"instance":       schema.StringMap(schema.Any()),
		"series":         schema.String(),
		"container-type": schema.String(),
		"tools":          schema.StringMap(schema.Any()),
		"containers":     schema.List(schema.StringMap(schema.Any())),
	}
	defaults := schema.Defaults{
		"placement":      "",
		"container-type": "",
	}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "machine v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	result.Id_ = valid["id"].(string)
	result.Nonce_ = valid["nonce"].(string)
	result.PasswordHash_ = valid["password-hash"].(string)
	result.Placement_ = valid["placement"].(string)
	result.Series_ = valid["series"].(string)
	result.ContainerType_ = valid["container-type"].(string)

	instance, err := importCloudInstance(valid["instance"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Trace(err)
	}
	result.Instance_ = instance

	tools, err := importAgentTools(valid["tools"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Trace(err)
	}
	result.Tools_ = tools

	machineList := valid["containers"].([]interface{})
	machines, err := importMachineList(machineList, importMachineV1)
	if err != nil {
		return nil, errors.Annotatef(err, "containers")
	}
	result.Containers_ = machines

	return result, nil

}

type CloudInstanceArgs struct {
	InstanceId       string
	Status           string
	Architecture     string
	Memory           uint64
	RootDisk         uint64
	CpuCores         uint64
	CpuPower         uint64
	Tags             []string
	AvailabilityZone string
}

func newCloudInstance(args CloudInstanceArgs) *cloudInstance {
	tags := make([]string, len(args.Tags))
	copy(tags, args.Tags)
	return &cloudInstance{
		Version:           1,
		InstanceId_:       args.InstanceId,
		Status_:           args.Status,
		Architecture_:     args.Architecture,
		Memory_:           args.Memory,
		RootDisk_:         args.RootDisk,
		CpuCores_:         args.CpuCores,
		CpuPower_:         args.CpuPower,
		Tags_:             tags,
		AvailabilityZone_: args.AvailabilityZone,
	}
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
	tags := make([]string, len(c.Tags_))
	copy(tags, c.Tags_)
	return tags
}

func (c *cloudInstance) AvailabilityZone() string {
	return c.AvailabilityZone_
}

// importCloudInstance constructs a new cloudInstance from a map that in
// normal usage situations will be the result of interpreting a large YAML
// document.
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

// Keeping the agentTools with the machine code, because we hope
// that one day we will succeed in merging the unit agents with the
// machine agents.
type agentTools struct {
	Version_      int            `yaml:"version"`
	ToolsVersion_ version.Binary `yaml:"tools-version"`
	URL_          string         `yaml:"url"`
	SHA256_       string         `yaml:"sha256"`
	Size_         int64          `yaml:"size"`
}

func (a *agentTools) Version() version.Binary {
	return a.ToolsVersion_
}
func (a *agentTools) URL() string {
	return a.URL_
}

func (a *agentTools) SHA256() string {
	return a.SHA256_
}

func (a *agentTools) Size() int64 {
	return a.Size_
}

// importAgentTools constructs a new agentTools instance from a map that in
// normal usage situations will be the result of interpreting a large YAML
// document.
//
// This method is a package internal serialisation method.
func importAgentTools(source map[string]interface{}) (*agentTools, error) {
	version, err := getVersion(source)
	if err != nil {
		return nil, errors.Annotate(err, "agentTools version schema check failed")
	}

	importFunc, ok := agentToolsDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}

	return importFunc(source)
}

type agentToolsDeserializationFunc func(map[string]interface{}) (*agentTools, error)

var agentToolsDeserializationFuncs = map[int]agentToolsDeserializationFunc{
	1: importAgentToolsV1,
}

func importAgentToolsV1(source map[string]interface{}) (*agentTools, error) {
	fields := schema.Fields{
		"tools-version": schema.String(),
		"url":           schema.String(),
		"sha256":        schema.String(),
		"size":          schema.Int(),
	}
	checker := schema.FieldMap(fields, nil) // no defaults

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "agentTools v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	verString := valid["tools-version"].(string)
	toolsVersion, err := version.ParseBinary(verString)
	if err != nil {
		return nil, errors.Annotatef(err, "agentTools tools-version")
	}

	return &agentTools{
		Version_:      1,
		ToolsVersion_: toolsVersion,
		URL_:          valid["url"].(string),
		SHA256_:       valid["sha256"].(string),
		Size_:         valid["size"].(int64),
	}, nil
}
