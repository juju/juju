package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"regexp"
	"strings"
)

// requiredError is useful when complaining about missing command-line options.
func requiredError(name string) error {
	return fmt.Errorf("--%s option must be set", name)
}

// zkAddrsValue implements gnuflag.Value on a []string.
type zkAddrsValue []string

var validAddr = regexp.MustCompile("^.*:[0-9]+$")

// Set splits value into zookeeper addresses.
func (v *zkAddrsValue) Set(value string) error {
	*v = strings.Split(value, ",")
	for _, addr := range *v {
		if !validAddr.MatchString(addr) {
			return fmt.Errorf("%q is not a valid zookeeper address", addr)
		}
	}
	return nil
}

// String returns the original value passed to Set
func (v *zkAddrsValue) String() string {
	if *v != nil {
		return strings.Join(*v, ",")
	}
	return ""
}

// zkAddrsVar sets up a gnuflag flag analagously to FlagSet.*Var methods.
func zkAddrsVar(fs *gnuflag.FlagSet, target *[]string, name string, value []string, usage string) {
	*target = value
	fs.Var((*zkAddrsValue)(target), name, usage)
}
