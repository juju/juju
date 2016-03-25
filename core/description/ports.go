// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

type versionedNetworkPorts struct {
	Version       int             `yaml:"version"`
	NetworkPorts_ []*networkPorts `yaml:"network-ports"`
}

type networkPorts struct {
	NetworkName_ string      `yaml:"network-name"`
	OpenPorts_   *portRanges `yaml:"open-ports"`
}

// NetworkPortsArgs is an argument struct used to add a set of opened port
// ranges to a machine.
type NetworkPortsArgs struct {
	NetworkName string
	OpenPorts   []PortRangeArgs
}

func newNetworkPorts(args NetworkPortsArgs) *networkPorts {
	result := &networkPorts{NetworkName_: args.NetworkName}
	result.setOpenPorts(nil)
	for _, pargs := range args.OpenPorts {
		result.OpenPorts_.add(pargs)
	}
	return result
}

// NetworkName implements NetworkPorts.
func (n *networkPorts) NetworkName() string {
	return n.NetworkName_
}

// OpenPorts implements NetworkPorts.
func (n *networkPorts) OpenPorts() []PortRange {
	var result []PortRange
	for _, pr := range n.OpenPorts_.OpenPorts_ {
		result = append(result, pr)
	}
	return result
}

func (n *networkPorts) setOpenPorts(ports []*portRange) {
	n.OpenPorts_ = &portRanges{
		Version:    1,
		OpenPorts_: ports,
	}
}

func importNetworkPorts(source map[string]interface{}) ([]*networkPorts, error) {
	checker := versionedChecker("network-ports")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "network-ports version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := networkPortsDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["network-ports"].([]interface{})
	return importNetworkPortsList(sourceList, importFunc)
}

func importNetworkPortsList(sourceList []interface{}, importFunc networkPortsDeserializationFunc) ([]*networkPorts, error) {
	result := make([]*networkPorts, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for network-ports %d, %T", i, value)
		}
		ports, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "network-ports %d", i)
		}
		result = append(result, ports)
	}
	return result, nil
}

type networkPortsDeserializationFunc func(map[string]interface{}) (*networkPorts, error)

var networkPortsDeserializationFuncs = map[int]networkPortsDeserializationFunc{
	1: importNetworkPortsV1,
}

func importNetworkPortsV1(source map[string]interface{}) (*networkPorts, error) {
	fields := schema.Fields{
		"network-name": schema.String(),
		"open-ports":   schema.StringMap(schema.Any()),
	}

	checker := schema.FieldMap(fields, nil) // no defaults

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "network-ports v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	ports, err := importPortRanges(valid["open-ports"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := &networkPorts{
		NetworkName_: valid["network-name"].(string),
	}
	result.setOpenPorts(ports)
	return result, nil
}

type portRanges struct {
	Version    int          `yaml:"version"`
	OpenPorts_ []*portRange `yaml:"open-ports"`
}

type portRange struct {
	UnitName_ string `yaml:"unit-name"`
	FromPort_ int    `yaml:"from-port"`
	ToPort_   int    `yaml:"to-port"`
	Protocol_ string `yaml:"protocol"`
}

// PortRangeArgs is an argument struct used to create a PortRange. This is
// only done as part of creating NetworkPorts for a Machine.
type PortRangeArgs struct {
	UnitName string
	FromPort int
	ToPort   int
	Protocol string
}

func newPortRange(args PortRangeArgs) *portRange {
	return &portRange{
		UnitName_: args.UnitName,
		FromPort_: args.FromPort,
		ToPort_:   args.ToPort,
		Protocol_: args.Protocol,
	}
}

func (p *portRanges) add(args PortRangeArgs) {
	p.OpenPorts_ = append(p.OpenPorts_, newPortRange(args))
}

// UnitName implements PortRange.
func (p *portRange) UnitName() string {
	return p.UnitName_
}

// FromPort implements PortRange.
func (p *portRange) FromPort() int {
	return p.FromPort_
}

// ToPort implements PortRange.
func (p *portRange) ToPort() int {
	return p.ToPort_
}

// Protocol implements PortRange.
func (p *portRange) Protocol() string {
	return p.Protocol_
}

func importPortRanges(source map[string]interface{}) ([]*portRange, error) {
	checker := versionedChecker("open-ports")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "port-range version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := portRangeDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["open-ports"].([]interface{})
	return importPortRangeList(sourceList, importFunc)
}

func importPortRangeList(sourceList []interface{}, importFunc portRangeDeserializationFunc) ([]*portRange, error) {
	result := make([]*portRange, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for port-range %d, %T", i, value)
		}
		ports, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "port-range %d", i)
		}
		result = append(result, ports)
	}
	return result, nil
}

type portRangeDeserializationFunc func(map[string]interface{}) (*portRange, error)

var portRangeDeserializationFuncs = map[int]portRangeDeserializationFunc{
	1: importPortRangeV1,
}

func importPortRangeV1(source map[string]interface{}) (*portRange, error) {
	fields := schema.Fields{
		"unit-name": schema.String(),
		"from-port": schema.Int(),
		"to-port":   schema.Int(),
		"protocol":  schema.String(),
	}

	checker := schema.FieldMap(fields, nil) // no defaults

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "port-range v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	return &portRange{
		UnitName_: valid["unit-name"].(string),
		FromPort_: int(valid["from-port"].(int64)),
		ToPort_:   int(valid["to-port"].(int64)),
		Protocol_: valid["protocol"].(string),
	}, nil
}
