// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/version"
)

type device struct {
	controller *controller

	resourceURI string

	systemID string
	hostname string
	fqdn     string

	parent string
	owner  string

	ipAddresses  []string
	interfaceSet []*interface_
	zone         *zone
}

// SystemID implements Device.
func (d *device) SystemID() string {
	return d.systemID
}

// Hostname implements Device.
func (d *device) Hostname() string {
	return d.hostname
}

// FQDN implements Device.
func (d *device) FQDN() string {
	return d.fqdn
}

// Parent implements Device.
func (d *device) Parent() string {
	return d.parent
}

// Owner implements Device.
func (d *device) Owner() string {
	return d.owner
}

// IPAddresses implements Device.
func (d *device) IPAddresses() []string {
	return d.ipAddresses
}

// Zone implements Device.
func (d *device) Zone() Zone {
	if d.zone == nil {
		return nil
	}
	return d.zone
}

// InterfaceSet implements Device.
func (d *device) InterfaceSet() []Interface {
	result := make([]Interface, len(d.interfaceSet))
	for i, v := range d.interfaceSet {
		v.controller = d.controller
		result[i] = v
	}
	return result
}

// CreateInterfaceArgs is an argument struct for passing parameters to
// the Machine.CreateInterface method.
type CreateInterfaceArgs struct {
	// Name of the interface (required).
	Name string
	// MACAddress is the MAC address of the interface (required).
	MACAddress string
	// VLAN is the untagged VLAN the interface is connected to (required).
	VLAN VLAN
	// Tags to attach to the interface (optional).
	Tags []string
	// MTU - Maximum transmission unit. (optional)
	MTU int
	// AcceptRA - Accept router advertisements. (IPv6 only)
	AcceptRA bool
	// Autoconf - Perform stateless autoconfiguration. (IPv6 only)
	Autoconf bool
}

// Validate checks the required fields are set for the arg structure.
func (a *CreateInterfaceArgs) Validate() error {
	if a.Name == "" {
		return errors.NotValidf("missing Name")
	}
	if a.MACAddress == "" {
		return errors.NotValidf("missing MACAddress")
	}
	if a.VLAN == nil {
		return errors.NotValidf("missing VLAN")
	}
	return nil
}

// interfacesURI used to add interfaces for this device. The operations
// are on the nodes endpoint, not devices.
func (d *device) interfacesURI() string {
	return strings.Replace(d.resourceURI, "devices", "nodes", 1) + "interfaces/"
}

// CreateInterface implements Device.
func (d *device) CreateInterface(args CreateInterfaceArgs) (Interface, error) {
	if err := args.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	params := NewURLParams()
	params.Values.Add("name", args.Name)
	params.Values.Add("mac_address", args.MACAddress)
	params.Values.Add("vlan", fmt.Sprint(args.VLAN.ID()))
	params.MaybeAdd("tags", strings.Join(args.Tags, ","))
	params.MaybeAddInt("mtu", args.MTU)
	params.MaybeAddBool("accept_ra", args.AcceptRA)
	params.MaybeAddBool("autoconf", args.Autoconf)
	result, err := d.controller.post(d.interfacesURI(), "create_physical", params.Values)
	if err != nil {
		if svrErr, ok := errors.Cause(err).(ServerError); ok {
			switch svrErr.StatusCode {
			case http.StatusNotFound, http.StatusConflict:
				return nil, errors.Wrap(err, NewBadRequestError(svrErr.BodyMessage))
			case http.StatusForbidden:
				return nil, errors.Wrap(err, NewPermissionError(svrErr.BodyMessage))
			case http.StatusServiceUnavailable:
				return nil, errors.Wrap(err, NewCannotCompleteError(svrErr.BodyMessage))
			}
		}
		return nil, NewUnexpectedError(err)
	}

	iface, err := readInterface(d.controller.apiVersion, result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	iface.controller = d.controller

	// TODO: add to the interfaces for the device when the interfaces are returned.
	// lp:bug 1567213.
	return iface, nil
}

// Delete implements Device.
func (d *device) Delete() error {
	err := d.controller.delete(d.resourceURI)
	if err != nil {
		if svrErr, ok := errors.Cause(err).(ServerError); ok {
			switch svrErr.StatusCode {
			case http.StatusNotFound:
				return errors.Wrap(err, NewNoMatchError(svrErr.BodyMessage))
			case http.StatusForbidden:
				return errors.Wrap(err, NewPermissionError(svrErr.BodyMessage))
			}
		}
		return NewUnexpectedError(err)
	}
	return nil
}

func readDevice(controllerVersion version.Number, source interface{}) (*device, error) {
	readFunc, err := getDeviceDeserializationFunc(controllerVersion)
	if err != nil {
		return nil, errors.Trace(err)
	}

	checker := schema.StringMap(schema.Any())
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, WrapWithDeserializationError(err, "device base schema check failed")
	}
	valid := coerced.(map[string]interface{})
	return readFunc(valid)
}

func readDevices(controllerVersion version.Number, source interface{}) ([]*device, error) {
	readFunc, err := getDeviceDeserializationFunc(controllerVersion)
	if err != nil {
		return nil, errors.Trace(err)
	}

	checker := schema.List(schema.StringMap(schema.Any()))
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, WrapWithDeserializationError(err, "device base schema check failed")
	}
	valid := coerced.([]interface{})
	return readDeviceList(valid, readFunc)
}

func getDeviceDeserializationFunc(controllerVersion version.Number) (deviceDeserializationFunc, error) {
	var deserialisationVersion version.Number
	for v := range deviceDeserializationFuncs {
		if v.Compare(deserialisationVersion) > 0 && v.Compare(controllerVersion) <= 0 {
			deserialisationVersion = v
		}
	}
	if deserialisationVersion == version.Zero {
		return nil, NewUnsupportedVersionError("no device read func for version %s", controllerVersion)
	}
	return deviceDeserializationFuncs[deserialisationVersion], nil
}

// readDeviceList expects the values of the sourceList to be string maps.
func readDeviceList(sourceList []interface{}, readFunc deviceDeserializationFunc) ([]*device, error) {
	result := make([]*device, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, NewDeserializationError("unexpected value for device %d, %T", i, value)
		}
		device, err := readFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "device %d", i)
		}
		result = append(result, device)
	}
	return result, nil
}

type deviceDeserializationFunc func(map[string]interface{}) (*device, error)

var deviceDeserializationFuncs = map[version.Number]deviceDeserializationFunc{
	twoDotOh: device_2_0,
}

func device_2_0(source map[string]interface{}) (*device, error) {
	fields := schema.Fields{
		"resource_uri": schema.String(),

		"system_id": schema.String(),
		"hostname":  schema.String(),
		"fqdn":      schema.String(),
		"parent":    schema.OneOf(schema.Nil(""), schema.String()),
		"owner":     schema.OneOf(schema.Nil(""), schema.String()),

		"ip_addresses":  schema.List(schema.String()),
		"interface_set": schema.List(schema.StringMap(schema.Any())),
		"zone":          schema.StringMap(schema.Any()),
	}
	defaults := schema.Defaults{
		"owner":  "",
		"parent": "",
	}
	checker := schema.FieldMap(fields, defaults)
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, WrapWithDeserializationError(err, "device 2.0 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	interfaceSet, err := readInterfaceList(valid["interface_set"].([]interface{}), interface_2_0)
	if err != nil {
		return nil, errors.Trace(err)
	}
	zone, err := zone_2_0(valid["zone"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Trace(err)
	}
	owner, _ := valid["owner"].(string)
	parent, _ := valid["parent"].(string)
	result := &device{
		resourceURI: valid["resource_uri"].(string),

		systemID: valid["system_id"].(string),
		hostname: valid["hostname"].(string),
		fqdn:     valid["fqdn"].(string),
		parent:   parent,
		owner:    owner,

		ipAddresses:  convertToStringSlice(valid["ip_addresses"]),
		interfaceSet: interfaceSet,
		zone:         zone,
	}
	return result, nil
}
