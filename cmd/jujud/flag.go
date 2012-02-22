package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/state"
	"regexp"
	"strings"
)

// requiredError is useful when complaining about missing command-line options.
func requiredError(name string) error {
	return fmt.Errorf("--%s option must be set", name)
}

// stateInfoValue implements gnuflag.Value on a state.Info.
type stateInfoValue state.Info

var validAddr = regexp.MustCompile("^.*:[0-9]+$")

// Set splits value into zookeeper addresses.
func (v *stateInfoValue) Set(value string) error {
	v.Addrs = strings.Split(value, ",")
	for _, addr := range v.Addrs {
		if !validAddr.MatchString(addr) {
			return fmt.Errorf("%q is not a valid zookeeper address", addr)
		}
	}
	return nil
}

// String returns the original value passed to Set.
func (v *stateInfoValue) String() string {
	if v.Addrs != nil {
		return strings.Join(v.Addrs, ",")
	}
	return ""
}

// stateInfoVar sets up a gnuflag flag analagously to FlagSet.*Var methods.
func stateInfoVar(fs *gnuflag.FlagSet, target *state.Info, name string, value []string, usage string) {
	target.Addrs = value
	fs.Var((*stateInfoValue)(target), name, usage)
}
