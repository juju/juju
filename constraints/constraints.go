// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/juju/errors"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/arch"
)

// The following constants list the supported constraint attribute names, as defined
// by the fields in the Value struct.
const (
	Arch         = "arch"
	Container    = "container"
	CpuCores     = "cpu-cores"
	CpuPower     = "cpu-power"
	Mem          = "mem"
	RootDisk     = "root-disk"
	Tags         = "tags"
	InstanceType = "instance-type"
)

// Value describes a user's requirements of the hardware on which units
// of a service will run. Constraints are used to choose an existing machine
// onto which a unit will be deployed, or to provision a new machine if no
// existing one satisfies the requirements.
type Value struct {

	// Arch, if not nil or empty, indicates that a machine must run the named
	// architecture.
	Arch *string `json:"arch,omitempty" yaml:"arch,omitempty"`

	// Container, if not nil, indicates that a machine must be the specified container type.
	Container *instance.ContainerType `json:"container,omitempty" yaml:"container,omitempty"`

	// CpuCores, if not nil, indicates that a machine must have at least that
	// number of effective cores available.
	CpuCores *uint64 `json:"cpu-cores,omitempty" yaml:"cpu-cores,omitempty"`

	// CpuPower, if not nil, indicates that a machine must have at least that
	// amount of CPU power available, where 100 CpuPower is considered to be
	// equivalent to 1 Amazon ECU (or, roughly, a single 2007-era Xeon).
	CpuPower *uint64 `json:"cpu-power,omitempty" yaml:"cpu-power,omitempty"`

	// Mem, if not nil, indicates that a machine must have at least that many
	// megabytes of RAM.
	Mem *uint64 `json:"mem,omitempty" yaml:"mem,omitempty"`

	// RootDisk, if not nil, indicates that a machine must have at least
	// that many megabytes of disk space available in the root disk. In
	// providers where the root disk is configurable at instance startup
	// time, an instance with the specified amount of disk space in the OS
	// disk might be requested.
	RootDisk *uint64 `json:"root-disk,omitempty" yaml:"root-disk,omitempty"`

	// Tags, if not nil, indicates tags that the machine must have applied to it.
	// An empty list is treated the same as a nil (unspecified) list, except an
	// empty list will override any default tags, where a nil list will not.
	Tags *[]string `json:"tags,omitempty" yaml:"tags,omitempty"`

	// InstanceType, if not nil, indicates that the specified cloud instance type
	// be used. Only valid for clouds which support instance types.
	InstanceType *string `json:"instance-type,omitempty" yaml:"instance-type,omitempty"`
}

// fieldNames records a mapping from the constraint tag to struct field name.
// eg "root-disk" maps to RootDisk.
var fieldNames map[string]string

func init() {
	// Create the fieldNames map by inspecting the json tags for each of
	// the Value struct fields.
	fieldNames = make(map[string]string)
	typ := reflect.TypeOf(Value{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if tag := field.Tag.Get("json"); tag != "" {
			if i := strings.Index(tag, ","); i >= 0 {
				tag = tag[0:i]
			}
			if tag == "-" {
				continue
			}
			if tag != "" {
				fieldNames[tag] = field.Name
			}
		}
	}
}

// IsEmpty returns if the given constraints value has no constraints set
func IsEmpty(v *Value) bool {
	return v.String() == ""
}

// HasInstanceType returns true if the constraints.Value specifies an instance type.
func (v *Value) HasInstanceType() bool {
	return v.InstanceType != nil && *v.InstanceType != ""
}

// String expresses a constraints.Value in the language in which it was specified.
func (v Value) String() string {
	var strs []string
	if v.Arch != nil {
		strs = append(strs, "arch="+*v.Arch)
	}
	if v.Container != nil {
		strs = append(strs, "container="+string(*v.Container))
	}
	if v.CpuCores != nil {
		strs = append(strs, "cpu-cores="+uintStr(*v.CpuCores))
	}
	if v.CpuPower != nil {
		strs = append(strs, "cpu-power="+uintStr(*v.CpuPower))
	}
	if v.InstanceType != nil {
		strs = append(strs, "instance-type="+string(*v.InstanceType))
	}
	if v.Mem != nil {
		s := uintStr(*v.Mem)
		if s != "" {
			s += "M"
		}
		strs = append(strs, "mem="+s)
	}
	if v.RootDisk != nil {
		s := uintStr(*v.RootDisk)
		if s != "" {
			s += "M"
		}
		strs = append(strs, "root-disk="+s)
	}
	if v.Tags != nil {
		s := strings.Join(*v.Tags, ",")
		strs = append(strs, "tags="+s)
	}
	return strings.Join(strs, " ")
}

func uintStr(i uint64) string {
	if i == 0 {
		return ""
	}
	return fmt.Sprintf("%d", i)
}

// Parse constructs a constraints.Value from the supplied arguments,
// each of which must contain only spaces and name=value pairs. If any
// name is specified more than once, an error is returned.
func Parse(args ...string) (Value, error) {
	cons := Value{}
	for _, arg := range args {
		raws := strings.Split(strings.TrimSpace(arg), " ")
		for _, raw := range raws {
			if raw == "" {
				continue
			}
			if err := cons.setRaw(raw); err != nil {
				return Value{}, err
			}
		}
	}
	return cons, nil
}

// MustParse constructs a constraints.Value from the supplied arguments,
// as Parse, but panics on failure.
func MustParse(args ...string) Value {
	v, err := Parse(args...)
	if err != nil {
		panic(err)
	}
	return v
}

// Constraints implements gnuflag.Value for a Constraints.
type ConstraintsValue struct {
	Target *Value
}

func (v ConstraintsValue) Set(s string) error {
	cons, err := Parse(s)
	if err != nil {
		return err
	}
	*v.Target = cons
	return nil
}

func (v ConstraintsValue) String() string {
	return v.Target.String()
}

func (v *Value) fieldFromTag(tagName string) (reflect.Value, bool) {
	fieldName := fieldNames[tagName]
	val := reflect.ValueOf(v).Elem().FieldByName(fieldName)
	return val, val.IsValid()
}

// attributesWithValues returns the non-zero attribute tags and their values from the constraint.
func (v *Value) attributesWithValues() (result map[string]interface{}) {
	result = make(map[string]interface{})
	for fieldTag, fieldName := range fieldNames {
		val := reflect.ValueOf(v).Elem().FieldByName(fieldName)
		if !val.IsNil() {
			result[fieldTag] = val.Elem().Interface()
		}
	}
	return result
}

// hasAny returns any attrTags for which the constraint has a non-nil value.
func (v *Value) hasAny(attrTags ...string) []string {
	attrValues := v.attributesWithValues()
	var result []string = []string{}
	for _, tag := range attrTags {
		_, ok := attrValues[tag]
		if ok {
			result = append(result, tag)
		}
	}
	return result
}

// without returns a copy of the constraint without values for
// the specified attributes.
func (v *Value) without(attrTags ...string) (Value, error) {
	result := *v
	for _, tag := range attrTags {
		val, ok := result.fieldFromTag(tag)
		if !ok {
			return Value{}, fmt.Errorf("unknown constraint %q", tag)
		}
		val.Set(reflect.Zero(val.Type()))
	}
	return result, nil
}

// setRaw interprets a name=value string and sets the supplied value.
func (v *Value) setRaw(raw string) error {
	eq := strings.Index(raw, "=")
	if eq <= 0 {
		return fmt.Errorf("malformed constraint %q", raw)
	}
	name, str := raw[:eq], raw[eq+1:]
	var err error
	switch name {
	case Arch:
		err = v.setArch(str)
	case Container:
		err = v.setContainer(str)
	case CpuCores:
		err = v.setCpuCores(str)
	case CpuPower:
		err = v.setCpuPower(str)
	case Mem:
		err = v.setMem(str)
	case RootDisk:
		err = v.setRootDisk(str)
	case Tags:
		err = v.setTags(str)
	case InstanceType:
		err = v.setInstanceType(str)
	default:
		return fmt.Errorf("unknown constraint %q", name)
	}
	if err != nil {
		return errors.Annotatef(err, "bad %q constraint", name)
	}
	return nil
}

// SetYAML is required to unmarshall a constraints.Value object
// to ensure the container attribute is correctly handled when it is empty.
// Because ContainerType is an alias for string, Go's reflect logic used in the
// YAML decode determines that *string and *ContainerType are not assignable so
// the container value of "" in the YAML is ignored.
func (v *Value) SetYAML(tag string, value interface{}) bool {
	values, ok := value.(map[interface{}]interface{})
	if !ok {
		return false
	}
	for k, val := range values {
		vstr := fmt.Sprintf("%v", val)
		var err error
		switch k {
		case Arch:
			v.Arch = &vstr
		case Container:
			ctype := instance.ContainerType(vstr)
			v.Container = &ctype
		case InstanceType:
			v.InstanceType = &vstr
		case CpuCores:
			v.CpuCores, err = parseUint64(vstr)
		case CpuPower:
			v.CpuPower, err = parseUint64(vstr)
		case Mem:
			v.Mem, err = parseUint64(vstr)
		case RootDisk:
			v.RootDisk, err = parseUint64(vstr)
		case Tags:
			v.Tags, err = parseYamlTags(val)
		default:
			return false
		}
		if err != nil {
			return false
		}
	}
	return true
}

func (v *Value) setContainer(str string) error {
	if v.Container != nil {
		return fmt.Errorf("already set")
	}
	if str == "" {
		ctype := instance.ContainerType("")
		v.Container = &ctype
	} else {
		ctype, err := instance.ParseContainerTypeOrNone(str)
		if err != nil {
			return err
		}
		v.Container = &ctype
	}
	return nil
}

// HasContainer returns true if the constraints.Value specifies a container.
func (v *Value) HasContainer() bool {
	return v.Container != nil && *v.Container != "" && *v.Container != instance.NONE
}

func (v *Value) setArch(str string) error {
	if v.Arch != nil {
		return fmt.Errorf("already set")
	}
	if str != "" && !arch.IsSupportedArch(str) {
		return fmt.Errorf("%q not recognized", str)
	}
	v.Arch = &str
	return nil
}

func (v *Value) setCpuCores(str string) (err error) {
	if v.CpuCores != nil {
		return fmt.Errorf("already set")
	}
	v.CpuCores, err = parseUint64(str)
	return
}

func (v *Value) setCpuPower(str string) (err error) {
	if v.CpuPower != nil {
		return fmt.Errorf("already set")
	}
	v.CpuPower, err = parseUint64(str)
	return
}

func (v *Value) setInstanceType(str string) error {
	if v.InstanceType != nil {
		return fmt.Errorf("already set")
	}
	v.InstanceType = &str
	return nil
}

func (v *Value) setMem(str string) (err error) {
	if v.Mem != nil {
		return fmt.Errorf("already set")
	}
	v.Mem, err = parseSize(str)
	return
}

func (v *Value) setRootDisk(str string) (err error) {
	if v.RootDisk != nil {
		return fmt.Errorf("already set")
	}
	v.RootDisk, err = parseSize(str)
	return
}

func (v *Value) setTags(str string) error {
	if v.Tags != nil {
		return fmt.Errorf("already set")
	}
	v.Tags = parseTags(str)
	return nil
}

func parseUint64(str string) (*uint64, error) {
	var value uint64
	if str != "" {
		if val, err := strconv.ParseUint(str, 10, 64); err != nil {
			return nil, fmt.Errorf("must be a non-negative integer")
		} else {
			value = uint64(val)
		}
	}
	return &value, nil
}

func parseSize(str string) (*uint64, error) {
	var value uint64
	if str != "" {
		mult := 1.0
		if m, ok := mbSuffixes[str[len(str)-1:]]; ok {
			str = str[:len(str)-1]
			mult = m
		}
		val, err := strconv.ParseFloat(str, 64)
		if err != nil || val < 0 {
			return nil, fmt.Errorf("must be a non-negative float with optional M/G/T/P suffix")
		}
		val *= mult
		value = uint64(math.Ceil(val))
	}
	return &value, nil
}

// parseTags returns the tags in the value s.  We expect the tags to be comma delimited strings.
func parseTags(s string) *[]string {
	if s == "" {
		return &[]string{}
	}
	t := strings.Split(s, ",")
	return &t
}

func parseYamlTags(val interface{}) (*[]string, error) {
	ifcs, ok := val.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected type passed to tags: %T", val)
	}
	tags := make([]string, len(ifcs))
	for n, ifc := range ifcs {
		s, ok := ifc.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected type passed as a tag: %T", ifc)
		}
		tags[n] = s
	}
	return &tags, nil
}

var mbSuffixes = map[string]float64{
	"M": 1,
	"G": 1024,
	"T": 1024 * 1024,
	"P": 1024 * 1024 * 1024,
}
