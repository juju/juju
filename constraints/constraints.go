// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints

import (
	"fmt"
	"math"
	"strconv"
	"strings"

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

	// OsDisk, if not nil, indicates that a machine must have at least that
	// many megabytes of disk space available in the OS disk, aka root
	// partition. In providers where the OS disk is configurable at
	// instance startup time, an instance with the specified amount of disk
	// space in the OS disk might be requested.
	OsDisk *uint64 `json:"os-disk,omitempty" yaml:"os-disk,omitempty"`
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
	if v.OsDisk != nil {
		s := uintStr(*v.OsDisk)
		if s != "" {
			s += "M"
		}
		strs = append(strs, "os-disk="+s)
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
	if v.OsDisk != nil {
		v1.OsDisk = v.OsDisk
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
	case "os-disk":
		err = v.setOsDisk(str)
	default:
		return fmt.Errorf("unknown constraint %q", name)
	}
	if err != nil {
		return fmt.Errorf("bad %q constraint: %v", name, err)
	}
	return nil
}

// SetYAML is required to unmarshall a constraints.Value object
// to ensure the container attribute is correctly handled when it is empty.
// Because ContainerType is an alias for string, Go's reflect logic used in the
// YAML decode determines that *string and *ContainerType are not assignable so
// the container value of "" in the YAML is ignored.
func (v *Value) SetYAML(tag string, value interface{}) bool {
	values := value.(map[interface{}]interface{})
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
		case "os-disk":
			v.OsDisk, err = parseUint64(vstr)
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
		ctype, err := instance.ParseSupportedContainerTypeOrNone(str)
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
	case "amd64", "i386", "arm":
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

func (v *Value) setOsDisk(str string) (err error) {
	if v.OsDisk != nil {
		return fmt.Errorf("already set")
	}
	v.OsDisk, err = parseSize(str)
	return
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

var mbSuffixes = map[string]float64{
	"M": 1,
	"G": 1024,
	"T": 1024 * 1024,
	"P": 1024 * 1024 * 1024,
}
