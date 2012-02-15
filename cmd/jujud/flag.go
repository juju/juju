package main

import (
	"fmt"
	"regexp"
	"strings"
)

// requiredError is useful when complaining about missing command-line options.
func requiredError(name string) error {
	return fmt.Errorf("--%s option must be set", name)
}

// zkAddrsValue implements gnuflag.Value
type zkAddrsValue struct {
	addrs *[]string
}

var validAddr = regexp.MustCompile(".*:[0-9]+")

func (v *zkAddrsValue) Set(value string) error {
	*v.addrs = strings.Split(value, ",")
	for _, addr := range *v.addrs {
		if !validAddr.MatchString(addr) {
			return fmt.Errorf("%q is not a valid zookeeper address", addr)
		}
	}
	return nil
}

func (v *zkAddrsValue) String() string {
	return strings.Join(*v.addrs, ",")
}
