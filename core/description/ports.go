// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

type versionedOpenedPorts struct {
	Version      int            `yaml:"version"`
	OpenedPorts_ []*openedPorts `yaml:"opened-ports"`
}

type openedPorts struct {
	SubnetID_    string      `yaml:"subnet-id"`
	OpenedPorts_ *portRanges `yaml:"opened-ports"`
}

// OpenedPortsArgs is an argument struct used to add a set of opened port ranges
// to a machine.
type OpenedPortsArgs struct {
	SubnetID    string
	OpenedPorts []PortRangeArgs
}

func newOpenedPorts(args OpenedPortsArgs) *openedPorts {
	result := &openedPorts{SubnetID_: args.SubnetID}
	result.setOpenedPorts(nil)
	for _, pargs := range args.OpenedPorts {
		result.OpenedPorts_.add(pargs)
	}
	return result
}

// SubnetID implements OpenedPorts.
func (p *openedPorts) SubnetID() string {
	return p.SubnetID_
}

// OpenPorts implements OpenedPorts.
func (p *openedPorts) OpenPorts() []PortRange {
	var result []PortRange
	for _, pr := range p.OpenedPorts_.OpenedPorts_ {
		result = append(result, pr)
	}
	return result
}

func (p *openedPorts) setOpenedPorts(ports []*portRange) {
	p.OpenedPorts_ = &portRanges{
		Version:      1,
		OpenedPorts_: ports,
	}
}

func importOpenedPorts(source map[string]interface{}) ([]*openedPorts, error) {
	checker := versionedChecker("opened-ports")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "opened-ports version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := openedPortsDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["opened-ports"].([]interface{})
	return importOpenedPortsList(sourceList, importFunc)
}

func importOpenedPortsList(sourceList []interface{}, importFunc openedPortsDeserializationFunc) ([]*openedPorts, error) {
	result := make([]*openedPorts, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for opened-ports %d, %T", i, value)
		}
		ports, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "opened-ports %d", i)
		}
		result = append(result, ports)
	}
	return result, nil
}

type openedPortsDeserializationFunc func(map[string]interface{}) (*openedPorts, error)

var openedPortsDeserializationFuncs = map[int]openedPortsDeserializationFunc{
	1: importOpenedPortsV1,
}

func importOpenedPortsV1(source map[string]interface{}) (*openedPorts, error) {
	fields := schema.Fields{
		"subnet-id":    schema.String(),
		"opened-ports": schema.StringMap(schema.Any()),
	}

	checker := schema.FieldMap(fields, nil) // no defaults

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "opened-ports v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	ports, err := importPortRanges(valid["opened-ports"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := &openedPorts{
		SubnetID_: valid["subnet-id"].(string),
	}
	result.setOpenedPorts(ports)
	return result, nil
}

type portRanges struct {
	Version      int          `yaml:"version"`
	OpenedPorts_ []*portRange `yaml:"opened-ports"`
}

type portRange struct {
	UnitName_ string `yaml:"unit-name"`
	FromPort_ int    `yaml:"from-port"`
	ToPort_   int    `yaml:"to-port"`
	Protocol_ string `yaml:"protocol"`
}

// PortRangeArgs is an argument struct used to create a PortRange. This is only
// done as part of creating OpenedPorts for a Machine.
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
	p.OpenedPorts_ = append(p.OpenedPorts_, newPortRange(args))
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
	checker := versionedChecker("opened-ports")
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
	sourceList := valid["opened-ports"].([]interface{})
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
