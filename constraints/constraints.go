// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/errgo/errgo"

	"launchpad.net/juju-core/instance"
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
}

// IsEmpty returns if the given constraints value has no constraints set
func IsEmpty(v *Value) bool {
	return v == nil ||
		v.Arch == nil &&
			v.Container == nil &&
			v.CpuCores == nil &&
			v.CpuPower == nil &&
			v.Mem == nil &&
			v.RootDisk == nil &&
			v.Tags == nil
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

// WithFallbacks returns a copy of v with nil values taken from v0.
func (v Value) WithFallbacks(v0 Value) Value {
	v1 := v0
	if v.Arch != nil {
		v1.Arch = v.Arch
	}
	if v.Container != nil {
		v1.Container = v.Container
	}
	if v.CpuCores != nil {
		v1.CpuCores = v.CpuCores
	}
	if v.CpuPower != nil {
		v1.CpuPower = v.CpuPower
	}
	if v.Mem != nil {
		v1.Mem = v.Mem
	}
	if v.RootDisk != nil {
		v1.RootDisk = v.RootDisk
	}
	if v.Tags != nil {
		v1.Tags = v.Tags
	}
	return v1
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

// setRaw interprets a name=value string and sets the supplied value.
func (v *Value) setRaw(raw string) error {
	eq := strings.Index(raw, "=")
	if eq <= 0 {
		return fmt.Errorf("malformed constraint %q", raw)
	}
	name, str := raw[:eq], raw[eq+1:]
	var err error
	switch name {
	case "arch":
		err = v.setArch(str)
	case "container":
		err = v.setContainer(str)
	case "cpu-cores":
		err = v.setCpuCores(str)
	case "cpu-power":
		err = v.setCpuPower(str)
	case "mem":
		err = v.setMem(str)
	case "root-disk":
		err = v.setRootDisk(str)
	case "tags":
		err = v.setTags(str)
	default:
		return fmt.Errorf("unknown constraint %q", name)
	}
	if err != nil {
		return errgo.Annotatef(err, "bad %q constraint", name)
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
		case "arch":
			v.Arch = &vstr
		case "container":
			ctype := instance.ContainerType(vstr)
			v.Container = &ctype
		case "cpu-cores":
			v.CpuCores, err = parseUint64(vstr)
		case "cpu-power":
			v.CpuPower, err = parseUint64(vstr)
		case "mem":
			v.Mem, err = parseUint64(vstr)
		case "root-disk":
			v.RootDisk, err = parseUint64(vstr)
		case "tags":
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
	switch str {
	case "":
	case "amd64", "i386", "arm", "arm64", "ppc64":
	default:
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
