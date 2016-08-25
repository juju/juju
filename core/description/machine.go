// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
)

type machines struct {
	Version   int        `yaml:"version"`
	Machines_ []*machine `yaml:"machines"`
}

type machine struct {
	Id_            string         `yaml:"id"`
	Nonce_         string         `yaml:"nonce"`
	PasswordHash_  string         `yaml:"password-hash"`
	Placement_     string         `yaml:"placement,omitempty"`
	Instance_      *cloudInstance `yaml:"instance,omitempty"`
	Series_        string         `yaml:"series"`
	ContainerType_ string         `yaml:"container-type,omitempty"`

	Status_        *status `yaml:"status"`
	StatusHistory_ `yaml:"status-history"`

	ProviderAddresses_ []*address `yaml:"provider-addresses,omitempty"`
	MachineAddresses_  []*address `yaml:"machine-addresses,omitempty"`

	PreferredPublicAddress_  *address `yaml:"preferred-public-address,omitempty"`
	PreferredPrivateAddress_ *address `yaml:"preferred-private-address,omitempty"`

	Tools_ *agentTools `yaml:"tools"`
	Jobs_  []string    `yaml:"jobs"`

	SupportedContainers_ *[]string `yaml:"supported-containers,omitempty"`

	Containers_ []*machine `yaml:"containers"`

	OpenedPorts_ *versionedOpenedPorts `yaml:"opened-ports,omitempty"`

	Annotations_ `yaml:"annotations,omitempty"`

	Constraints_ *constraints `yaml:"constraints,omitempty"`

	BlockDevices_ blockdevices `yaml:"block-devices,omitempty"`
}

// MachineArgs is an argument struct used to add a machine to the Model.
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
		StatusHistory_: newStatusHistory(),
	}
	if args.SupportedContainers != nil {
		supported := make([]string, len(*args.SupportedContainers))
		copy(supported, *args.SupportedContainers)
		m.SupportedContainers_ = &supported
	}
	m.setBlockDevices(nil)
	return m
}

// Id implements Machine.
func (m *machine) Id() string {
	return m.Id_
}

// Tag implements Machine.
func (m *machine) Tag() names.MachineTag {
	return names.NewMachineTag(m.Id_)
}

// Nonce implements Machine.
func (m *machine) Nonce() string {
	return m.Nonce_
}

// PasswordHash implements Machine.
func (m *machine) PasswordHash() string {
	return m.PasswordHash_
}

// Placement implements Machine.
func (m *machine) Placement() string {
	return m.Placement_
}

// Instance implements Machine.
func (m *machine) Instance() CloudInstance {
	// To avoid typed nils check nil here.
	if m.Instance_ == nil {
		return nil
	}
	return m.Instance_
}

// SetInstance implements Machine.
func (m *machine) SetInstance(args CloudInstanceArgs) {
	m.Instance_ = newCloudInstance(args)
}

// Series implements Machine.
func (m *machine) Series() string {
	return m.Series_
}

// ContainerType implements Machine.
func (m *machine) ContainerType() string {
	return m.ContainerType_
}

// Status implements Machine.
func (m *machine) Status() Status {
	// To avoid typed nils check nil here.
	if m.Status_ == nil {
		return nil
	}
	return m.Status_
}

// SetStatus implements Machine.
func (m *machine) SetStatus(args StatusArgs) {
	m.Status_ = newStatus(args)
}

// ProviderAddresses implements Machine.
func (m *machine) ProviderAddresses() []Address {
	var result []Address
	for _, addr := range m.ProviderAddresses_ {
		result = append(result, addr)
	}
	return result
}

// MachineAddresses implements Machine.
func (m *machine) MachineAddresses() []Address {
	var result []Address
	for _, addr := range m.MachineAddresses_ {
		result = append(result, addr)
	}
	return result
}

// SetAddresses implements Machine.
func (m *machine) SetAddresses(margs []AddressArgs, pargs []AddressArgs) {
	m.MachineAddresses_ = nil
	m.ProviderAddresses_ = nil
	for _, args := range margs {
		if args.Value != "" {
			m.MachineAddresses_ = append(m.MachineAddresses_, newAddress(args))
		}
	}
	for _, args := range pargs {
		if args.Value != "" {
			m.ProviderAddresses_ = append(m.ProviderAddresses_, newAddress(args))
		}
	}
}

// PreferredPublicAddress implements Machine.
func (m *machine) PreferredPublicAddress() Address {
	// To avoid typed nils check nil here.
	if m.PreferredPublicAddress_ == nil {
		return nil
	}
	return m.PreferredPublicAddress_
}

// PreferredPrivateAddress implements Machine.
func (m *machine) PreferredPrivateAddress() Address {
	// To avoid typed nils check nil here.
	if m.PreferredPrivateAddress_ == nil {
		return nil
	}
	return m.PreferredPrivateAddress_
}

// SetPreferredAddresses implements Machine.
func (m *machine) SetPreferredAddresses(public AddressArgs, private AddressArgs) {
	if public.Value != "" {
		m.PreferredPublicAddress_ = newAddress(public)
	}
	if private.Value != "" {
		m.PreferredPrivateAddress_ = newAddress(private)
	}
}

// Tools implements Machine.
func (m *machine) Tools() AgentTools {
	// To avoid a typed nil, check before returning.
	if m.Tools_ == nil {
		return nil
	}
	return m.Tools_
}

// SetTools implements Machine.
func (m *machine) SetTools(args AgentToolsArgs) {
	m.Tools_ = newAgentTools(args)
}

// Jobs implements Machine.
func (m *machine) Jobs() []string {
	return m.Jobs_
}

// SupportedContainers implements Machine.
func (m *machine) SupportedContainers() ([]string, bool) {
	if m.SupportedContainers_ == nil {
		return nil, false
	}
	return *m.SupportedContainers_, true
}

// Containers implements Machine.
func (m *machine) Containers() []Machine {
	var result []Machine
	for _, container := range m.Containers_ {
		result = append(result, container)
	}
	return result
}

// BlockDevices implements Machine.
func (m *machine) BlockDevices() []BlockDevice {
	var result []BlockDevice
	for _, device := range m.BlockDevices_.BlockDevices_ {
		result = append(result, device)
	}
	return result
}

// AddBlockDevice implements Machine.
func (m *machine) AddBlockDevice(args BlockDeviceArgs) BlockDevice {
	return m.BlockDevices_.add(args)
}

func (m *machine) setBlockDevices(devices []*blockdevice) {
	m.BlockDevices_ = blockdevices{
		Version:       1,
		BlockDevices_: devices,
	}
}

// AddContainer implements Machine.
func (m *machine) AddContainer(args MachineArgs) Machine {
	container := newMachine(args)
	m.Containers_ = append(m.Containers_, container)
	return container
}

// OpenedPorts implements Machine.
func (m *machine) OpenedPorts() []OpenedPorts {
	if m.OpenedPorts_ == nil {
		return nil
	}
	var result []OpenedPorts
	for _, ports := range m.OpenedPorts_.OpenedPorts_ {
		result = append(result, ports)
	}
	return result
}

// AddOpenedPorts implements Machine.
func (m *machine) AddOpenedPorts(args OpenedPortsArgs) OpenedPorts {
	if m.OpenedPorts_ == nil {
		m.OpenedPorts_ = &versionedOpenedPorts{Version: 1}
	}
	ports := newOpenedPorts(args)
	m.OpenedPorts_.OpenedPorts_ = append(m.OpenedPorts_.OpenedPorts_, ports)
	return ports
}

func (m *machine) setOpenedPorts(portsList []*openedPorts) {
	m.OpenedPorts_ = &versionedOpenedPorts{
		Version:      1,
		OpenedPorts_: portsList,
	}
}

// Constraints implements HasConstraints.
func (m *machine) Constraints() Constraints {
	if m.Constraints_ == nil {
		return nil
	}
	return m.Constraints_
}

// SetConstraints implements HasConstraints.
func (m *machine) SetConstraints(args ConstraintsArgs) {
	m.Constraints_ = newConstraints(args)
}

// Validate implements Machine.
func (m *machine) Validate() error {
	if m.Id_ == "" {
		return errors.NotValidf("machine missing id")
	}
	if m.Status_ == nil {
		return errors.NotValidf("machine %q missing status", m.Id_)
	}
	// Since all exports should be done when machines are stable,
	// there should always be tools and cloud instance.
	if m.Tools_ == nil {
		return errors.NotValidf("machine %q missing tools", m.Id_)
	}
	if m.Instance_ == nil {
		return errors.NotValidf("machine %q missing instance", m.Id_)
	}
	for _, container := range m.Containers_ {
		if err := container.Validate(); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
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
	fields := schema.Fields{
		"id":                   schema.String(),
		"nonce":                schema.String(),
		"password-hash":        schema.String(),
		"placement":            schema.String(),
		"instance":             schema.StringMap(schema.Any()),
		"series":               schema.String(),
		"container-type":       schema.String(),
		"jobs":                 schema.List(schema.String()),
		"status":               schema.StringMap(schema.Any()),
		"supported-containers": schema.List(schema.String()),
		"tools":                schema.StringMap(schema.Any()),
		"containers":           schema.List(schema.StringMap(schema.Any())),
		"opened-ports":         schema.StringMap(schema.Any()),

		"provider-addresses":        schema.List(schema.StringMap(schema.Any())),
		"machine-addresses":         schema.List(schema.StringMap(schema.Any())),
		"preferred-public-address":  schema.StringMap(schema.Any()),
		"preferred-private-address": schema.StringMap(schema.Any()),

		"block-devices": schema.StringMap(schema.Any()),
	}

	defaults := schema.Defaults{
		"placement":      "",
		"container-type": "",
		// Even though we are expecting instance data for every machine,
		// it isn't strictly necessary, so we allow it to not exist here.
		"instance":                  schema.Omit,
		"supported-containers":      schema.Omit,
		"opened-ports":              schema.Omit,
		"block-devices":             schema.Omit,
		"provider-addresses":        schema.Omit,
		"machine-addresses":         schema.Omit,
		"preferred-public-address":  schema.Omit,
		"preferred-private-address": schema.Omit,
	}
	addAnnotationSchema(fields, defaults)
	addConstraintsSchema(fields, defaults)
	addStatusHistorySchema(fields)
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "machine v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	result := &machine{
		Id_:            valid["id"].(string),
		Nonce_:         valid["nonce"].(string),
		PasswordHash_:  valid["password-hash"].(string),
		Placement_:     valid["placement"].(string),
		Series_:        valid["series"].(string),
		ContainerType_: valid["container-type"].(string),
		StatusHistory_: newStatusHistory(),
		Jobs_:          convertToStringSlice(valid["jobs"]),
	}
	result.importAnnotations(valid)
	if err := result.importStatusHistory(valid); err != nil {
		return nil, errors.Trace(err)
	}

	if constraintsMap, ok := valid["constraints"]; ok {
		constraints, err := importConstraints(constraintsMap.(map[string]interface{}))
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.Constraints_ = constraints
	}

	if supported, ok := valid["supported-containers"]; ok {
		supportedList := supported.([]interface{})
		s := make([]string, len(supportedList))
		for i, containerType := range supportedList {
			s[i] = containerType.(string)
		}
		result.SupportedContainers_ = &s
	}

	if instanceMap, ok := valid["instance"]; ok {
		instance, err := importCloudInstance(instanceMap.(map[string]interface{}))
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.Instance_ = instance
	}

	if blockDeviceMap, ok := valid["block-devices"]; ok {
		devices, err := importBlockDevices(blockDeviceMap.(map[string]interface{}))
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.setBlockDevices(devices)
	} else {
		result.setBlockDevices(nil)
	}

	// Tools and status are required, so we expect them to be there.
	tools, err := importAgentTools(valid["tools"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Trace(err)
	}
	result.Tools_ = tools

	status, err := importStatus(valid["status"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Trace(err)
	}
	result.Status_ = status

	if addresses, ok := valid["provider-addresses"]; ok {
		providerAddresses, err := importAddresses(addresses.([]interface{}))
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.ProviderAddresses_ = providerAddresses
	}

	if addresses, ok := valid["machine-addresses"]; ok {
		machineAddresses, err := importAddresses(addresses.([]interface{}))
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.MachineAddresses_ = machineAddresses
	}

	if address, ok := valid["preferred-public-address"]; ok {
		publicAddress, err := importAddress(address.(map[string]interface{}))
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.PreferredPublicAddress_ = publicAddress
	}

	if address, ok := valid["preferred-private-address"]; ok {
		privateAddress, err := importAddress(address.(map[string]interface{}))
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.PreferredPrivateAddress_ = privateAddress
	}

	machineList := valid["containers"].([]interface{})
	machines, err := importMachineList(machineList, importMachineV1)
	if err != nil {
		return nil, errors.Annotatef(err, "containers")
	}
	result.Containers_ = machines

	if portsMap, ok := valid["opened-ports"]; ok {
		portsList, err := importOpenedPorts(portsMap.(map[string]interface{}))
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.setOpenedPorts(portsList)
	}

	return result, nil

}

// CloudInstanceArgs is an argument struct used to add information about the
// cloud instance to a Machine.
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

// InstanceId implements CloudInstance.
func (c *cloudInstance) InstanceId() string {
	return c.InstanceId_
}

// Status implements CloudInstance.
func (c *cloudInstance) Status() string {
	return c.Status_
}

// Architecture implements CloudInstance.
func (c *cloudInstance) Architecture() string {
	return c.Architecture_
}

// Memory implements CloudInstance.
func (c *cloudInstance) Memory() uint64 {
	return c.Memory_
}

// RootDisk implements CloudInstance.
func (c *cloudInstance) RootDisk() uint64 {
	return c.RootDisk_
}

// CpuCores implements CloudInstance.
func (c *cloudInstance) CpuCores() uint64 {
	return c.CpuCores_
}

// CpuPower implements CloudInstance.
func (c *cloudInstance) CpuPower() uint64 {
	return c.CpuPower_
}

// Tags implements CloudInstance.
func (c *cloudInstance) Tags() []string {
	tags := make([]string, len(c.Tags_))
	copy(tags, c.Tags_)
	return tags
}

// AvailabilityZone implements CloudInstance.
func (c *cloudInstance) AvailabilityZone() string {
	return c.AvailabilityZone_
}

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
		"memory":            schema.ForceUint(),
		"root-disk":         schema.ForceUint(),
		"cpu-cores":         schema.ForceUint(),
		"cpu-power":         schema.ForceUint(),
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

	return &cloudInstance{
		Version:           1,
		InstanceId_:       valid["instance-id"].(string),
		Status_:           valid["status"].(string),
		Architecture_:     valid["architecture"].(string),
		Memory_:           valid["memory"].(uint64),
		RootDisk_:         valid["root-disk"].(uint64),
		CpuCores_:         valid["cpu-cores"].(uint64),
		CpuPower_:         valid["cpu-power"].(uint64),
		Tags_:             convertToStringSlice(valid["tags"]),
		AvailabilityZone_: valid["availability-zone"].(string),
	}, nil
}

// AgentToolsArgs is an argument struct used to add information about the
// tools the agent is using to a Machine.
type AgentToolsArgs struct {
	Version version.Binary
	URL     string
	SHA256  string
	Size    int64
}

func newAgentTools(args AgentToolsArgs) *agentTools {
	return &agentTools{
		Version_:      1,
		ToolsVersion_: args.Version,
		URL_:          args.URL,
		SHA256_:       args.SHA256,
		Size_:         args.Size,
	}
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

// Version implements AgentTools.
func (a *agentTools) Version() version.Binary {
	return a.ToolsVersion_
}

// URL implements AgentTools.
func (a *agentTools) URL() string {
	return a.URL_
}

// SHA256 implements AgentTools.
func (a *agentTools) SHA256() string {
	return a.SHA256_
}

// Size implements AgentTools.
func (a *agentTools) Size() int64 {
	return a.Size_
}

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
