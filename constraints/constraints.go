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
	"github.com/juju/names"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
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
	Networks     = "networks"
	Spaces       = "spaces"
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

	// Spaces, if not nil, holds a list of juju network spaces that
	// should be available (or not) on the machine. Positive and
	// negative values are accepted, and the difference is the latter
	// have a "^" prefix to the name.
	Spaces *[]string `json:"spaces,omitempty" yaml:"spaces,omitempty"`

	// Networks, if not nil, holds a list of juju networks that
	// should be available (or not) on the machine. Positive and
	// negative values are accepted, and the difference is the latter
	// have a "^" prefix to the name.
	//
	// TODO(dimitern): Drop this as soon as spaces can be used for
	// deployments instead.
	Networks *[]string `json:"networks,omitempty" yaml:"networks,omitempty"`
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

// extractItems returns the list of entries in the given field which
// are either positive (included) or negative (!included; with prefix
// "^").
func (v *Value) extractItems(field []string, included bool) []string {
	var items []string
	for _, name := range field {
		prefixed := strings.HasPrefix(name, "^")
		if prefixed && !included {
			// has prefix and we want negatives.
			items = append(items, strings.TrimPrefix(name, "^"))
		} else if !prefixed && included {
			// no prefix and we want positives.
			items = append(items, name)
		}
	}
	return items
}

// IncludeSpaces returns a list of spaces to include when starting a
// machine, if specified.
func (v *Value) IncludeSpaces() []string {
	if v.Spaces == nil {
		return nil
	}
	return v.extractItems(*v.Spaces, true)
}

// ExcludeSpaces returns a list of spaces to exclude when starting a
// machine, if specified. They are given in the spaces constraint with
// a "^" prefix to the name, which is stripped before returning.
func (v *Value) ExcludeSpaces() []string {
	if v.Spaces == nil {
		return nil
	}
	return v.extractItems(*v.Spaces, false)
}

// HaveSpaces returns whether any spaces constraints were specified.
func (v *Value) HaveSpaces() bool {
	return v.Spaces != nil && len(*v.Spaces) > 0
}

// TODO(dimitern): Drop the following 3 methods once spaces can be
// used as deployment constraints.

// IncludeNetworks returns a list of networks to include when starting
// a machine, if specified.
func (v *Value) IncludeNetworks() []string {
	if v.Networks == nil {
		return nil
	}
	return v.extractItems(*v.Networks, true)
}

// ExcludeNetworks returns a list of networks to exclude when starting
// a machine, if specified. They are given in the networks constraint
// with a "^" prefix to the name, which is stripped before returning.
func (v *Value) ExcludeNetworks() []string {
	if v.Networks == nil {
		return nil
	}
	return v.extractItems(*v.Networks, false)
}

// HaveNetworks returns whether any network constraints were specified.
func (v *Value) HaveNetworks() bool {
	return v.Networks != nil && len(*v.Networks) > 0
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
	if v.Spaces != nil {
		s := strings.Join(*v.Spaces, ",")
		strs = append(strs, "spaces="+s)
	}
	if v.Networks != nil {
		s := strings.Join(*v.Networks, ",")
		strs = append(strs, "networks="+s)
	}
	return strings.Join(strs, " ")
}

// GoString allows printing a constraints.Value nicely with the fmt
// package, especially when nested inside other types.
func (v Value) GoString() string {
	var values []string
	if v.Arch != nil {
		values = append(values, fmt.Sprintf("Arch: %q", *v.Arch))
	}
	if v.CpuCores != nil {
		values = append(values, fmt.Sprintf("CpuCores: %v", *v.CpuCores))
	}
	if v.CpuPower != nil {
		values = append(values, fmt.Sprintf("CpuPower: %v", *v.CpuPower))
	}
	if v.Mem != nil {
		values = append(values, fmt.Sprintf("Mem: %v", *v.Mem))
	}
	if v.RootDisk != nil {
		values = append(values, fmt.Sprintf("RootDisk: %v", *v.RootDisk))
	}
	if v.InstanceType != nil {
		values = append(values, fmt.Sprintf("InstanceType: %q", *v.InstanceType))
	}
	if v.Container != nil {
		values = append(values, fmt.Sprintf("Container: %q", *v.Container))
	}
	if v.Tags != nil && *v.Tags != nil {
		values = append(values, fmt.Sprintf("Tags: %q", *v.Tags))
	} else if v.Tags != nil {
		values = append(values, "Tags: (*[]string)(nil)")
	}
	if v.Spaces != nil && *v.Spaces != nil {
		values = append(values, fmt.Sprintf("Spaces: %q", *v.Spaces))
	} else if v.Spaces != nil {
		values = append(values, "Spaces: (*[]string)(nil)")
	}
	if v.Networks != nil && *v.Networks != nil {
		values = append(values, fmt.Sprintf("Networks: %q", *v.Networks))
	} else if v.Networks != nil {
		values = append(values, "Networks: (*[]string)(nil)")
	}
	return fmt.Sprintf("{%s}", strings.Join(values, ", "))
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

// Merge returns the effective constraints after merging any given
// existing values.
func Merge(values ...Value) (Value, error) {
	var args []string
	for _, value := range values {
		args = append(args, value.String())
	}
	return Parse(args...)
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
			return Value{}, errors.Errorf("unknown constraint %q", tag)
		}
		val.Set(reflect.Zero(val.Type()))
	}
	return result, nil
}

// setRaw interprets a name=value string and sets the supplied value.
func (v *Value) setRaw(raw string) error {
	eq := strings.Index(raw, "=")
	if eq <= 0 {
		return errors.Errorf("malformed constraint %q", raw)
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
	case Spaces:
		err = v.setSpaces(str)
	case Networks:
		err = v.setNetworks(str)
	default:
		return errors.Errorf("unknown constraint %q", name)
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
			v.Tags, err = parseYamlStrings("tags", val)
		case Spaces:
			var spaces *[]string
			spaces, err = parseYamlStrings("spaces", val)
			if err != nil {
				return false
			}
			err = v.validateSpaces(spaces)
			if err == nil {
				v.Spaces = spaces
			}
		case Networks:
			var networks *[]string
			networks, err = parseYamlStrings("networks", val)
			if err != nil {
				return false
			}
			err = v.validateNetworks(networks)
			if err == nil {
				v.Networks = networks
			}
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
		return errors.Errorf("already set")
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
		return errors.Errorf("already set")
	}
	if str != "" && !arch.IsSupportedArch(str) {
		return errors.Errorf("%q not recognized", str)
	}
	v.Arch = &str
	return nil
}

func (v *Value) setCpuCores(str string) (err error) {
	if v.CpuCores != nil {
		return errors.Errorf("already set")
	}
	v.CpuCores, err = parseUint64(str)
	return
}

func (v *Value) setCpuPower(str string) (err error) {
	if v.CpuPower != nil {
		return errors.Errorf("already set")
	}
	v.CpuPower, err = parseUint64(str)
	return
}

func (v *Value) setInstanceType(str string) error {
	if v.InstanceType != nil {
		return errors.Errorf("already set")
	}
	v.InstanceType = &str
	return nil
}

func (v *Value) setMem(str string) (err error) {
	if v.Mem != nil {
		return errors.Errorf("already set")
	}
	v.Mem, err = parseSize(str)
	return
}

func (v *Value) setRootDisk(str string) (err error) {
	if v.RootDisk != nil {
		return errors.Errorf("already set")
	}
	v.RootDisk, err = parseSize(str)
	return
}

func (v *Value) setTags(str string) error {
	if v.Tags != nil {
		return errors.Errorf("already set")
	}
	v.Tags = parseCommaDelimited(str)
	return nil
}

func (v *Value) setSpaces(str string) error {
	if v.Spaces != nil {
		return errors.Errorf("already set")
	}
	spaces := parseCommaDelimited(str)
	if err := v.validateSpaces(spaces); err != nil {
		return err
	}
	v.Spaces = spaces
	return nil
}

func (v *Value) validateSpaces(spaces *[]string) error {
	if spaces == nil {
		return nil
	}
	for _, name := range *spaces {
		space := strings.TrimPrefix(name, "^")
		if !names.IsValidSpace(space) {
			return errors.Errorf("%q is not a valid space name", space)
		}
	}
	return nil
}

func (v *Value) setNetworks(str string) error {
	if v.Networks != nil {
		return errors.Errorf("already set")
	}
	networks := parseCommaDelimited(str)
	if err := v.validateNetworks(networks); err != nil {
		return err
	}
	v.Networks = networks
	return nil
}

func (v *Value) validateNetworks(networks *[]string) error {
	if networks == nil {
		return nil
	}
	for _, name := range *networks {
		netName := strings.TrimPrefix(name, "^")
		if !names.IsValidNetwork(netName) {
			return errors.Errorf("%q is not a valid network name", netName)
		}
	}
	return nil
}

func parseUint64(str string) (*uint64, error) {
	var value uint64
	if str != "" {
		if val, err := strconv.ParseUint(str, 10, 64); err != nil {
			return nil, errors.Errorf("must be a non-negative integer")
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
			return nil, errors.Errorf("must be a non-negative float with optional M/G/T/P suffix")
		}
		val *= mult
		value = uint64(math.Ceil(val))
	}
	return &value, nil
}

// parseCommaDelimited returns the items in the value s. We expect the
// items to be comma delimited strings.
func parseCommaDelimited(s string) *[]string {
	if s == "" {
		return &[]string{}
	}
	t := strings.Split(s, ",")
	return &t
}

func parseYamlStrings(entityName string, val interface{}) (*[]string, error) {
	ifcs, ok := val.([]interface{})
	if !ok {
		return nil, errors.Errorf("unexpected type passed to %s: %T", entityName, val)
	}
	items := make([]string, len(ifcs))
	for n, ifc := range ifcs {
		s, ok := ifc.(string)
		if !ok {
			return nil, errors.Errorf("unexpected type passed as in %s: %T", entityName, ifc)
		}
		items[n] = s
	}
	return &items, nil
}

var mbSuffixes = map[string]float64{
	"M": 1,
	"G": 1024,
	"T": 1024 * 1024,
	"P": 1024 * 1024 * 1024,
}
