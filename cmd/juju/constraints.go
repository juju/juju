package main

import (
	"fmt"
	"launchpad.net/juju-core/state"
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
	args := strings.Split(raw, " ")
	for _, arg := range args {
		eq := strings.Index(arg, "=")
		if eq <= 0 {
			return fmt.Errorf("malformed constraint %q", arg)
		}
		name, str := arg[:eq], arg[eq+1:]
		var err error
		switch name {
		case "cores":
			err = v.setCores(str)
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

func (v *constraintsValue) setCores(str string) error {
	if v.c.Cores != nil {
		return fmt.Errorf("already set")
	}
	var val int
	if str != "" {
		var err error
		if val, err = strconv.Atoi(str); err != nil || val < 0 {
			return fmt.Errorf("must be a non-negative integer")
		}
	}
	v.c.Cores = &val
	return nil
}

func (v *constraintsValue) setMem(str string) error {
	if v.c.Mem != nil {
		return fmt.Errorf("already set")
	}
	var val float64
	if str != "" {
		mult := 1.0
		if m, ok := mbSuffixes[str[len(str)-1:]]; ok {
			str = str[:len(str)-1]
			mult = m
		}
		var err error
		if val, err = strconv.ParseFloat(str, 64); err != nil || val < 0 {
			return fmt.Errorf("must be a non-negative float with optional M/G/T/P suffix")
		}
		val *= mult
	}
	v.c.Mem = &val
	return nil
}

var mbSuffixes = map[string]float64{
	"M": 1,
	"G": 1024,
	"T": 1024 * 1024,
	"P": 1024 * 1024 * 1024,
}
