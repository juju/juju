// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/version"
)

type machine struct {
	controller *controller

	resourceURI string

	systemID string
	hostname string
	fqdn     string
	tags     []string

	operatingSystem string
	distroSeries    string
	architecture    string
	memory          int
	cpuCount        int

	ipAddresses []string
	powerState  string

	// NOTE: consider some form of status struct
	statusName    string
	statusMessage string

	bootInterface *interface_
	interfaceSet  []*interface_
	zone          *zone
	// Don't really know the difference between these two lists:
	physicalBlockDevices []*blockdevice
	blockDevices         []*blockdevice
}

func (m *machine) updateFrom(other *machine) {
	m.resourceURI = other.resourceURI
	m.systemID = other.systemID
	m.hostname = other.hostname
	m.fqdn = other.fqdn
	m.operatingSystem = other.operatingSystem
	m.distroSeries = other.distroSeries
	m.architecture = other.architecture
	m.memory = other.memory
	m.cpuCount = other.cpuCount
	m.ipAddresses = other.ipAddresses
	m.powerState = other.powerState
	m.statusName = other.statusName
	m.statusMessage = other.statusMessage
	m.zone = other.zone
}

// SystemID implements Machine.
func (m *machine) SystemID() string {
	return m.systemID
}

// Hostname implements Machine.
func (m *machine) Hostname() string {
	return m.hostname
}

// FQDN implements Machine.
func (m *machine) FQDN() string {
	return m.fqdn
}

// Tags implements Machine.
func (m *machine) Tags() []string {
	return m.tags
}

// IPAddresses implements Machine.
func (m *machine) IPAddresses() []string {
	return m.ipAddresses
}

// Memory implements Machine.
func (m *machine) Memory() int {
	return m.memory
}

// CPUCount implements Machine.
func (m *machine) CPUCount() int {
	return m.cpuCount
}

// PowerState implements Machine.
func (m *machine) PowerState() string {
	return m.powerState
}

// Zone implements Machine.
func (m *machine) Zone() Zone {
	if m.zone == nil {
		return nil
	}
	return m.zone
}

// BootInterface implements Machine.
func (m *machine) BootInterface() Interface {
	if m.bootInterface == nil {
		return nil
	}
	m.bootInterface.controller = m.controller
	return m.bootInterface
}

// InterfaceSet implements Machine.
func (m *machine) InterfaceSet() []Interface {
	result := make([]Interface, len(m.interfaceSet))
	for i, v := range m.interfaceSet {
		v.controller = m.controller
		result[i] = v
	}
	return result
}

// Interface implements Machine.
func (m *machine) Interface(id int) Interface {
	for _, iface := range m.interfaceSet {
		if iface.ID() == id {
			iface.controller = m.controller
			return iface
		}
	}
	return nil
}

// OperatingSystem implements Machine.
func (m *machine) OperatingSystem() string {
	return m.operatingSystem
}

// DistroSeries implements Machine.
func (m *machine) DistroSeries() string {
	return m.distroSeries
}

// Architecture implements Machine.
func (m *machine) Architecture() string {
	return m.architecture
}

// StatusName implements Machine.
func (m *machine) StatusName() string {
	return m.statusName
}

// StatusMessage implements Machine.
func (m *machine) StatusMessage() string {
	return m.statusMessage
}

// PhysicalBlockDevices implements Machine.
func (m *machine) PhysicalBlockDevices() []BlockDevice {
	result := make([]BlockDevice, len(m.physicalBlockDevices))
	for i, v := range m.physicalBlockDevices {
		result[i] = v
	}
	return result
}

// PhysicalBlockDevice implements Machine.
func (m *machine) PhysicalBlockDevice(id int) BlockDevice {
	for _, blockDevice := range m.physicalBlockDevices {
		if blockDevice.ID() == id {
			return blockDevice
		}
	}
	return nil
}

// BlockDevices implements Machine.
func (m *machine) BlockDevices() []BlockDevice {
	result := make([]BlockDevice, len(m.blockDevices))
	for i, v := range m.blockDevices {
		result[i] = v
	}
	return result
}

// Devices implements Machine.
func (m *machine) Devices(args DevicesArgs) ([]Device, error) {
	// Perhaps in the future, MAAS will give us a way to query just for the
	// devices for a particular parent.
	devices, err := m.controller.Devices(args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var result []Device
	for _, device := range devices {
		if device.Parent() == m.SystemID() {
			result = append(result, device)
		}
	}
	return result, nil
}

// StartArgs is an argument struct for passing parameters to the Machine.Start
// method.
type StartArgs struct {
	// UserData needs to be Base64 encoded user data for cloud-init.
	UserData     string
	DistroSeries string
	Kernel       string
	Comment      string
}

// Start implements Machine.
func (m *machine) Start(args StartArgs) error {
	params := NewURLParams()
	params.MaybeAdd("user_data", args.UserData)
	params.MaybeAdd("distro_series", args.DistroSeries)
	params.MaybeAdd("hwe_kernel", args.Kernel)
	params.MaybeAdd("comment", args.Comment)
	result, err := m.controller.post(m.resourceURI, "deploy", params.Values)
	if err != nil {
		if svrErr, ok := errors.Cause(err).(ServerError); ok {
			switch svrErr.StatusCode {
			case http.StatusNotFound, http.StatusConflict:
				return errors.Wrap(err, NewBadRequestError(svrErr.BodyMessage))
			case http.StatusForbidden:
				return errors.Wrap(err, NewPermissionError(svrErr.BodyMessage))
			case http.StatusServiceUnavailable:
				return errors.Wrap(err, NewCannotCompleteError(svrErr.BodyMessage))
			}
		}
		return NewUnexpectedError(err)
	}

	machine, err := readMachine(m.controller.apiVersion, result)
	if err != nil {
		return errors.Trace(err)
	}
	m.updateFrom(machine)
	return nil
}

// CreateMachineDeviceArgs is an argument structure for Machine.CreateDevice.
// All fields except Hostname are required.
type CreateMachineDeviceArgs struct {
	Hostname      string
	InterfaceName string
	MACAddress    string
	Subnet        Subnet
}

// Validate ensures that all required values are non-emtpy.
func (a *CreateMachineDeviceArgs) Validate() error {
	if a.InterfaceName == "" {
		return errors.NotValidf("missing InterfaceName")
	}
	if a.MACAddress == "" {
		return errors.NotValidf("missing MACAddress")
	}
	if a.Subnet == nil {
		return errors.NotValidf("missing Subnet")
	}
	return nil
}

// CreateDevice implements Machine
func (m *machine) CreateDevice(args CreateMachineDeviceArgs) (_ Device, err error) {
	if err := args.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	device, err := m.controller.CreateDevice(CreateDeviceArgs{
		Hostname:     args.Hostname,
		MACAddresses: []string{args.MACAddress},
		Parent:       m.SystemID(),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	defer func() {
		// If there is an error return, at least try to delete the device we just created.
		if err != nil {
			if innerErr := device.Delete(); innerErr != nil {
				logger.Warningf("could not delete device %q", device.SystemID())
			}
		}
	}()

	// There should be one interface created for each MAC Address, and since
	// we only specified one, there should just be one response.
	interfaces := device.InterfaceSet()
	if count := len(interfaces); count != 1 {
		err := errors.Errorf("unexpected interface count for device: %d", count)
		return nil, NewUnexpectedError(err)
	}
	iface := interfaces[0]

	// Now update the name and vlan of interface that was created…
	updateArgs := UpdateInterfaceArgs{}
	if iface.Name() != args.InterfaceName {
		updateArgs.Name = args.InterfaceName
	}
	if iface.VLAN().ID() != args.Subnet.VLAN().ID() {
		updateArgs.VLAN = args.Subnet.VLAN()
	}
	err = iface.Update(updateArgs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	err = iface.LinkSubnet(LinkSubnetArgs{
		Mode:   LinkModeStatic,
		Subnet: args.Subnet,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	return device, nil
}

func readMachine(controllerVersion version.Number, source interface{}) (*machine, error) {
	readFunc, err := getMachineDeserializationFunc(controllerVersion)
	if err != nil {
		return nil, errors.Trace(err)
	}

	checker := schema.StringMap(schema.Any())
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, WrapWithDeserializationError(err, "machine base schema check failed")
	}
	valid := coerced.(map[string]interface{})
	return readFunc(valid)
}

func readMachines(controllerVersion version.Number, source interface{}) ([]*machine, error) {
	readFunc, err := getMachineDeserializationFunc(controllerVersion)
	if err != nil {
		return nil, errors.Trace(err)
	}

	checker := schema.List(schema.StringMap(schema.Any()))
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, WrapWithDeserializationError(err, "machine base schema check failed")
	}
	valid := coerced.([]interface{})
	return readMachineList(valid, readFunc)
}

func getMachineDeserializationFunc(controllerVersion version.Number) (machineDeserializationFunc, error) {
	var deserialisationVersion version.Number
	for v := range machineDeserializationFuncs {
		if v.Compare(deserialisationVersion) > 0 && v.Compare(controllerVersion) <= 0 {
			deserialisationVersion = v
		}
	}
	if deserialisationVersion == version.Zero {
		return nil, NewUnsupportedVersionError("no machine read func for version %s", controllerVersion)
	}
	return machineDeserializationFuncs[deserialisationVersion], nil
}

func readMachineList(sourceList []interface{}, readFunc machineDeserializationFunc) ([]*machine, error) {
	result := make([]*machine, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, NewDeserializationError("unexpected value for machine %d, %T", i, value)
		}
		machine, err := readFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "machine %d", i)
		}
		result = append(result, machine)
	}
	return result, nil
}

type machineDeserializationFunc func(map[string]interface{}) (*machine, error)

var machineDeserializationFuncs = map[version.Number]machineDeserializationFunc{
	twoDotOh: machine_2_0,
}

func machine_2_0(source map[string]interface{}) (*machine, error) {
	fields := schema.Fields{
		"resource_uri": schema.String(),

		"system_id": schema.String(),
		"hostname":  schema.String(),
		"fqdn":      schema.String(),
		"tag_names": schema.List(schema.String()),

		"osystem":       schema.String(),
		"distro_series": schema.String(),
		"architecture":  schema.OneOf(schema.Nil(""), schema.String()),
		"memory":        schema.ForceInt(),
		"cpu_count":     schema.ForceInt(),

		"ip_addresses":   schema.List(schema.String()),
		"power_state":    schema.String(),
		"status_name":    schema.String(),
		"status_message": schema.OneOf(schema.Nil(""), schema.String()),

		"boot_interface": schema.OneOf(schema.Nil(""), schema.StringMap(schema.Any())),
		"interface_set":  schema.List(schema.StringMap(schema.Any())),
		"zone":           schema.StringMap(schema.Any()),

		"physicalblockdevice_set": schema.List(schema.StringMap(schema.Any())),
		"blockdevice_set":         schema.List(schema.StringMap(schema.Any())),
	}
	defaults := schema.Defaults{
		"architecture": "",
	}
	checker := schema.FieldMap(fields, defaults)
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, WrapWithDeserializationError(err, "machine 2.0 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	var bootInterface *interface_
	if ifaceMap, ok := valid["boot_interface"].(map[string]interface{}); ok {
		bootInterface, err = interface_2_0(ifaceMap)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	interfaceSet, err := readInterfaceList(valid["interface_set"].([]interface{}), interface_2_0)
	if err != nil {
		return nil, errors.Trace(err)
	}
	zone, err := zone_2_0(valid["zone"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Trace(err)
	}
	physicalBlockDevices, err := readBlockDeviceList(valid["physicalblockdevice_set"].([]interface{}), blockdevice_2_0)
	if err != nil {
		return nil, errors.Trace(err)
	}
	blockDevices, err := readBlockDeviceList(valid["blockdevice_set"].([]interface{}), blockdevice_2_0)
	if err != nil {
		return nil, errors.Trace(err)
	}
	architecture, _ := valid["architecture"].(string)
	statusMessage, _ := valid["status_message"].(string)
	result := &machine{
		resourceURI: valid["resource_uri"].(string),

		systemID: valid["system_id"].(string),
		hostname: valid["hostname"].(string),
		fqdn:     valid["fqdn"].(string),
		tags:     convertToStringSlice(valid["tag_names"]),

		operatingSystem: valid["osystem"].(string),
		distroSeries:    valid["distro_series"].(string),
		architecture:    architecture,
		memory:          valid["memory"].(int),
		cpuCount:        valid["cpu_count"].(int),

		ipAddresses:   convertToStringSlice(valid["ip_addresses"]),
		powerState:    valid["power_state"].(string),
		statusName:    valid["status_name"].(string),
		statusMessage: statusMessage,

		bootInterface:        bootInterface,
		interfaceSet:         interfaceSet,
		zone:                 zone,
		physicalBlockDevices: physicalBlockDevices,
		blockDevices:         blockDevices,
	}

	return result, nil
}

func convertToStringSlice(field interface{}) []string {
	if field == nil {
		return nil
	}
	fieldSlice := field.([]interface{})
	result := make([]string, len(fieldSlice))
	for i, value := range fieldSlice {
		result[i] = value.(string)
	}
	return result
}
