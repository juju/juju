// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/instance"
)

// The following constants list the supported constraint attribute names, as defined
// by the fields in the Value struct.
const (
	Arch      = "arch"
	Container = "container"
	// cpuCores is an alias for Cores.
	cpuCores         = "cpu-cores"
	Cores            = "cores"
	CpuPower         = "cpu-power"
	Mem              = "mem"
	RootDisk         = "root-disk"
	RootDiskSource   = "root-disk-source"
	Tags             = "tags"
	InstanceRole     = "instance-role"
	InstanceType     = "instance-type"
	Spaces           = "spaces"
	VirtType         = "virt-type"
	Zones            = "zones"
	AllocatePublicIP = "allocate-public-ip"
	ImageID          = "image-id"
)

// Value describes a user's requirements of the hardware on which units
// of an application will run. Constraints are used to choose an existing machine
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
	CpuCores *uint64 `json:"cores,omitempty" yaml:"cores,omitempty"`

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

	// RootDiskSource, if specified, determines what storage the root
	// disk should be allocated from. This will be provider specific -
	// in the case of vSphere it identifies the datastore the root
	// disk file should be created in.
	RootDiskSource *string `json:"root-disk-source,omitempty" yaml:"root-disk-source,omitempty"`

	// Tags, if not nil, indicates tags that the machine must have applied to it.
	// An empty list is treated the same as a nil (unspecified) list, except an
	// empty list will override any default tags, where a nil list will not.
	Tags *[]string `json:"tags,omitempty" yaml:"tags,omitempty"`

	// InstanceRole, if not nil, indicates that the specified role/profile for
	// the given cloud should be used. Only valid for clouds which support
	// instance roles. Currently only for AWS with instance-profiles
	InstanceRole *string `json:"instance-role,omitempty" yaml:"instance-role,omitempty"`

	// InstanceType, if not nil, indicates that the specified cloud instance type
	// be used. Only valid for clouds which support instance types.
	InstanceType *string `json:"instance-type,omitempty" yaml:"instance-type,omitempty"`

	// Spaces, if not nil, holds a list of juju network spaces that
	// should be available (or not) on the machine. Positive and
	// negative values are accepted, and the difference is the latter
	// have a "^" prefix to the name.
	Spaces *[]string `json:"spaces,omitempty" yaml:"spaces,omitempty"`

	// VirtType, if not nil or empty, indicates that a machine must run the named
	// virtual type. Only valid for clouds with multi-hypervisor support.
	VirtType *string `json:"virt-type,omitempty" yaml:"virt-type,omitempty"`

	// Zones, if not nil, holds a list of availability zones limiting where
	// the machine can be located.
	Zones *[]string `json:"zones,omitempty" yaml:"zones,omitempty"`

	// AllocatePublicIP, if nil or true, signals that machines should be
	// created with a public IP address instead of a cloud local one.
	// The default behaviour if the value is not specified is to allocate
	// a public IP so that public cloud behaviour works out of the box.
	AllocatePublicIP *bool `json:"allocate-public-ip,omitempty" yaml:"allocate-public-ip,omitempty"`

	// ImageID, if not nil, indicates that a machine must use the specified
	// image. This is provider specific, and for the moment is only
	// implemented on MAAS clouds.
	ImageID *string `json:"image-id,omitempty" yaml:"image-id,omitempty"`
}

var rawAliases = map[string]string{
	cpuCores: Cores,
}

// resolveAlias returns the canonical representation of the given key, if it'a
// an alias listed in aliases, otherwise it returns the original key.
func resolveAlias(key string) string {
	if canonical, ok := rawAliases[key]; ok {
		return canonical
	}
	return key
}

// IsEmpty returns if the given constraints value has no constraints set
func IsEmpty(v *Value) bool {
	return v.String() == ""
}

// HasArch returns true if the constraints.Value specifies an architecture.
func (v *Value) HasArch() bool {
	return v.Arch != nil && *v.Arch != ""
}

// HasMem returns true if the constraints.Value specifies a minimum amount
// of memory.
func (v *Value) HasMem() bool {
	return v.Mem != nil && *v.Mem > 0
}

// HasCpuPower returns true if the constraints.Value specifies a minimum amount
// of CPU power.
func (v *Value) HasCpuPower() bool {
	return v.CpuPower != nil && *v.CpuPower > 0
}

// HasCpuCores returns true if the constraints.Value specifies a minimum number
// of CPU cores.
func (v *Value) HasCpuCores() bool {
	return v.CpuCores != nil && *v.CpuCores > 0
}

// HasRootDisk returns true if the constraints.Value specifies a RootDisk size.
func (v *Value) HasRootDisk() bool {
	return v.RootDisk != nil && *v.RootDisk > 0
}

// HasRootDiskSource returns true if the constraints.Value specifies a
// source for its root disk.
func (v *Value) HasRootDiskSource() bool {
	return v.RootDiskSource != nil && *v.RootDiskSource != ""
}

// HasInstanceRole returns true if the constraints.Value specifies an instance
// role.
func (v *Value) HasInstanceRole() bool {
	return v.InstanceRole != nil && *v.InstanceRole != ""
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

// IncludeSpaces returns a list of space IDs to include when starting a
// machine, if specified.
func (v *Value) IncludeSpaces() []string {
	if v.Spaces == nil {
		return nil
	}
	return v.extractItems(*v.Spaces, true)
}

// ExcludeSpaces returns a list of space IDs to exclude when starting a
// machine, if specified. They are given in the spaces constraint with
// a "^" prefix to the id, which is stripped before returning.
func (v *Value) ExcludeSpaces() []string {
	if v.Spaces == nil {
		return nil
	}
	return v.extractItems(*v.Spaces, false)
}

// HasSpaces returns whether any spaces constraints were specified.
func (v *Value) HasSpaces() bool {
	return v.Spaces != nil && len(*v.Spaces) > 0
}

// HasVirtType returns true if the constraints.Value specifies an virtual type.
func (v *Value) HasVirtType() bool {
	return v.VirtType != nil && *v.VirtType != ""
}

// HasZones returns whether any zone constraints were specified.
func (v *Value) HasZones() bool {
	return v.Zones != nil && len(*v.Zones) > 0
}

// HasAllocatePublicIP returns whether the allocate-public-ip constraint was specified.
func (v *Value) HasAllocatePublicIP() bool {
	return v.AllocatePublicIP != nil
}

// HasImageID returns true if the constraints.Value specifies an image-id.
func (v *Value) HasImageID() bool {
	return v.ImageID != nil && *v.ImageID != ""
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
		strs = append(strs, "cores="+uintStr(*v.CpuCores))
	}
	if v.CpuPower != nil {
		strs = append(strs, "cpu-power="+uintStr(*v.CpuPower))
	}
	if v.InstanceRole != nil {
		strs = append(strs, "instance-role="+(*v.InstanceRole))
	}
	if v.InstanceType != nil {
		strs = append(strs, "instance-type="+(*v.InstanceType))
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
	if v.RootDiskSource != nil {
		s := *v.RootDiskSource
		strs = append(strs, "root-disk-source="+s)
	}
	if v.Tags != nil {
		s := strings.Join(*v.Tags, ",")
		strs = append(strs, "tags="+s)
	}
	if v.Spaces != nil {
		s := strings.Join(*v.Spaces, ",")
		strs = append(strs, "spaces="+s)
	}
	if v.VirtType != nil {
		strs = append(strs, "virt-type="+(*v.VirtType))
	}
	if v.Zones != nil {
		s := strings.Join(*v.Zones, ",")
		strs = append(strs, "zones="+s)
	}
	if v.AllocatePublicIP != nil {
		strs = append(strs, "allocate-public-ip="+boolStr(*v.AllocatePublicIP))
	}
	if v.ImageID != nil {
		strs = append(strs, "image-id="+(*v.ImageID))
	}

	// Ensure constraint values with spaces are properly escaped
	for i := 0; i < len(strs); i++ {
		strs[i] = strings.Replace(strs[i], " ", `\ `, -1)
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
		values = append(values, fmt.Sprintf("Cores: %v", *v.CpuCores))
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
	if v.InstanceRole != nil {
		values = append(values, fmt.Sprintf("InstanceRole: %q", *v.InstanceRole))
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
	if v.VirtType != nil {
		values = append(values, fmt.Sprintf("VirtType: %q", *v.VirtType))
	}
	if v.Zones != nil && *v.Zones != nil {
		values = append(values, fmt.Sprintf("Zones: %q", *v.Zones))
	} else if v.Zones != nil {
		values = append(values, "Zones: (*[]string)(nil)")
	}
	if v.AllocatePublicIP != nil {
		values = append(values, fmt.Sprintf("AllocatePublicIP: %v", *v.AllocatePublicIP))
	}
	if v.ImageID != nil {
		values = append(values, fmt.Sprintf("ImageID: %q", *v.ImageID))
	}
	return fmt.Sprintf("{%s}", strings.Join(values, ", "))
}

func uintStr(i uint64) string {
	if i == 0 {
		return ""
	}
	return fmt.Sprintf("%d", i)
}

func boolStr(b bool) string {
	return fmt.Sprintf("%v", b)
}

// Parse constructs a constraints.Value from the supplied arguments,
// each of which must contain only spaces and name=value pairs. If any
// name is specified more than once, an error is returned.
func Parse(args ...string) (Value, error) {
	v, _, err := ParseWithAliases(args...)
	return v, err
}

// ParseWithAliases constructs a constraints.Value from the supplied arguments, each
// of which must contain only spaces and name=value pairs. If any name is
// specified more than once, an error is returned.  The aliases map returned
// contains a map of aliases used, and their canonical values.
func ParseWithAliases(args ...string) (cons Value, aliases map[string]string, err error) {
	aliases = make(map[string]string)
	for _, arg := range args {
		// Replace slash-escaped spaces with a null byte so we can
		// safely split on remaining whitespace.
		arg = strings.Replace(arg, `\ `, "\x00", -1)
		raws := strings.Split(strings.TrimSpace(arg), " ")
		for _, raw := range raws {
			// Replace null bytes back to spaces
			raw = strings.TrimSpace(strings.Replace(raw, "\x00", " ", -1))
			if raw == "" {
				continue
			}
			name, val, err := splitRaw(raw)
			if err != nil {
				return Value{}, nil, errors.Trace(err)
			}
			if canonical, ok := rawAliases[name]; ok {
				aliases[name] = canonical
				name = canonical
			}
			if err := cons.setRaw(name, val); err != nil {
				return Value{}, aliases, errors.Trace(err)
			}
		}
	}
	return cons, aliases, nil
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

// attributesWithValues returns the non-zero attribute tags and their values from the constraint.
func (v *Value) attributesWithValues() map[string]interface{} {
	// These can never fail, so we ignore the error for the sake of keeping our
	// API clean.  I'm sorry (but not that sorry).
	b, _ := json.Marshal(v)
	result := map[string]interface{}{}
	_ = json.Unmarshal(b, &result)
	return result
}

func fromAttributes(attr map[string]interface{}) Value {
	b, _ := json.Marshal(attr)
	var result Value
	_ = json.Unmarshal(b, &result)
	return result
}

// hasAny returns any attrTags for which the constraint has a non-nil value.
func (v *Value) hasAny(attrTags ...string) []string {
	attributes := v.attributesWithValues()
	var result []string
	for _, tag := range attrTags {
		_, ok := attributes[resolveAlias(tag)]
		if ok {
			result = append(result, tag)
		}
	}
	return result
}

// without returns a copy of the constraint without values for
// the specified attributes.
func (v *Value) without(attrTags ...string) Value {
	attributes := v.attributesWithValues()
	for _, tag := range attrTags {
		delete(attributes, resolveAlias(tag))
	}
	return fromAttributes(attributes)
}

func splitRaw(s string) (name, val string, err error) {
	eq := strings.Index(s, "=")
	if eq <= 0 {
		return "", "", errors.Errorf("malformed constraint %q", s)
	}
	return s[:eq], s[eq+1:], nil
}

// setRaw interprets a name=value string and sets the supplied value.
func (v *Value) setRaw(name, str string) error {
	var err error
	switch resolveAlias(name) {
	case Arch:
		err = v.setArch(str)
	case Container:
		err = v.setContainer(str)
	case Cores:
		err = v.setCpuCores(str)
	case CpuPower:
		err = v.setCpuPower(str)
	case Mem:
		err = v.setMem(str)
	case RootDisk:
		err = v.setRootDisk(str)
	case RootDiskSource:
		err = v.setRootDiskSource(str)
	case Tags:
		err = v.setTags(str)
	case InstanceRole:
		err = v.setInstanceRole(str)
	case InstanceType:
		err = v.setInstanceType(str)
	case Spaces:
		err = v.setSpaces(str)
	case VirtType:
		err = v.setVirtType(str)
	case Zones:
		err = v.setZones(str)
	case AllocatePublicIP:
		err = v.setAllocatePublicIP(str)
	case ImageID:
		err = v.setImageID(str)
	default:
		return errors.Errorf("unknown constraint %q", name)
	}
	if err != nil {
		return errors.Annotatef(err, "bad %q constraint", name)
	}
	return nil
}

// UnmarshalYAML is required to unmarshal a constraints.Value object
// to ensure the container attribute is correctly handled when it is empty.
// Because ContainerType is an alias for string, Go's reflect logic used in the
// YAML decode determines that *string and *ContainerType are not assignable so
// the container value of "" in the YAML is ignored.
func (v *Value) UnmarshalYAML(unmarshal func(interface{}) error) error {
	values := map[interface{}]interface{}{}
	err := unmarshal(&values)
	if err != nil {
		return errors.Trace(err)
	}
	canonicals := map[string]string{}
	for k, val := range values {
		vstr := fmt.Sprintf("%v", val)
		key, ok := k.(string)
		if !ok {
			return errors.Errorf("unexpected non-string key: %#v", k)
		}
		canonical := resolveAlias(key)
		if v, ok := canonicals[canonical]; ok {
			// duplicate entry
			return errors.Errorf("constraint %q duplicates constraint %q", key, v)
		}
		canonicals[canonical] = key
		switch canonical {
		case Arch:
			v.Arch = &vstr
		case Container:
			ctype := instance.ContainerType(vstr)
			v.Container = &ctype
		case InstanceRole:
			v.InstanceRole = &vstr
		case InstanceType:
			v.InstanceType = &vstr
		case Cores:
			v.CpuCores, err = parseUint64(vstr)
		case CpuPower:
			v.CpuPower, err = parseUint64(vstr)
		case Mem:
			v.Mem, err = parseUint64(vstr)
		case RootDisk:
			v.RootDisk, err = parseUint64(vstr)
		case RootDiskSource:
			v.RootDiskSource = &vstr
		case Tags:
			v.Tags, err = parseYamlStrings("tags", val)
		case Spaces:
			var spaces *[]string
			spaces, err = parseYamlStrings("spaces", val)
			if err != nil {
				return errors.Trace(err)
			}
			err = v.validateSpaces(spaces)
			if err == nil {
				v.Spaces = spaces
			}
		case VirtType:
			v.VirtType = &vstr
		case Zones:
			v.Zones, err = parseYamlStrings("zones", val)
		case AllocatePublicIP:
			v.AllocatePublicIP, err = parseBool(vstr)
		case ImageID:
			v.ImageID = &vstr
		default:
			return errors.Errorf("unknown constraint value: %v", k)
		}
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
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

func (v *Value) setInstanceRole(str string) error {
	if v.InstanceRole != nil {
		return errors.Errorf("already set")
	}
	v.InstanceRole = &str
	return nil
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

func (v *Value) setRootDiskSource(str string) error {
	if v.RootDiskSource != nil {
		return errors.Errorf("already set")
	}
	v.RootDiskSource = &str
	return nil
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

func (v *Value) setVirtType(str string) error {
	if v.VirtType != nil {
		return errors.Errorf("already set")
	}
	v.VirtType = &str
	return nil
}

func (v *Value) setZones(str string) error {
	if v.Zones != nil {
		return errors.Errorf("already set")
	}
	v.Zones = parseCommaDelimited(str)
	return nil
}

func (v *Value) setAllocatePublicIP(str string) (err error) {
	if str == "" {
		return nil
	}
	if v.AllocatePublicIP != nil {
		return errors.Errorf("already set")
	}
	v.AllocatePublicIP, err = parseBool(str)
	return
}

func (v *Value) setImageID(str string) (err error) {
	if v.ImageID != nil {
		return errors.Errorf("already set")
	}
	v.ImageID = &str
	return
}

func parseBool(str string) (*bool, error) {
	var value bool
	if str != "" {
		val, err := strconv.ParseBool(str)
		if err != nil {
			return nil, errors.Errorf("must be 'true' or 'false'")
		}
		value = val
	}
	return &value, nil
}

func parseUint64(str string) (*uint64, error) {
	var value uint64
	if str != "" {
		val, err := strconv.ParseUint(str, 10, 64)
		if err != nil {
			return nil, errors.Errorf("must be a non-negative integer")
		}
		value = val
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
