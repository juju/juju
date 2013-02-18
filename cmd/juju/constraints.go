package main

import (
	"fmt"
	"launchpad.net/juju-core/state"
	"math"
	"strconv"
	"strings"
)

// constraintsValue implements gnuflag.Value for a state.Constraints.
type constraintsValue struct {
	c *state.Constraints
}

func (v *constraintsValue) String() string {
	return v.c.String()
}

func (v *constraintsValue) Set(raw string) error {
	args := strings.Split(strings.TrimSpace(raw), " ")
	for _, arg := range args {
		if arg == "" {
			continue
		}
		eq := strings.Index(arg, "=")
		if eq <= 0 {
			return fmt.Errorf("malformed constraint %q", arg)
		}
		name, str := arg[:eq], arg[eq+1:]
		var err error
		switch name {
		case "cpu-cores":
			err = v.setCpuCores(str)
		case "cpu-power":
			err = v.setCpuPower(str)
		case "mem":
			err = v.setMem(str)
		default:
			return fmt.Errorf("unknown constraint %q", name)
		}
		if err != nil {
			return fmt.Errorf("bad %q constraint: %v", name, err)
		}
	}
	return nil
}

func (v *constraintsValue) setCpuCores(str string) (err error) {
	if v.c.CpuCores != nil {
		return fmt.Errorf("already set")
	}
	v.c.CpuCores, err = parseUint64(str)
	return
}

func (v *constraintsValue) setCpuPower(str string) (err error) {
	if v.c.CpuPower != nil {
		return fmt.Errorf("already set")
	}
	v.c.CpuPower, err = parseUint64(str)
	return
}

func parseUint64(str string) (*uint64, error) {
	var value uint64
	if str != "" {
		if val, err := strconv.Atoi(str); err != nil || val < 0 {
			return nil, fmt.Errorf("must be a non-negative integer")
		} else {
			value = uint64(val)
		}
	}
	return &value, nil
}

func (v *constraintsValue) setMem(str string) error {
	if v.c.Mem != nil {
		return fmt.Errorf("already set")
	}
	var value uint64
	if str != "" {
		mult := 1.0
		if m, ok := mbSuffixes[str[len(str)-1:]]; ok {
			str = str[:len(str)-1]
			mult = m
		}
		val, err := strconv.ParseFloat(str, 64)
		if err != nil || val < 0 {
			return fmt.Errorf("must be a non-negative float with optional M/G/T/P suffix")
		}
		val *= mult
		value = uint64(math.Ceil(val))
	}
	v.c.Mem = &value
	return nil
}

var mbSuffixes = map[string]float64{
	"M": 1,
	"G": 1024,
	"T": 1024 * 1024,
	"P": 1024 * 1024 * 1024,
}
