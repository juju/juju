package state

import (
	"fmt"
	"strconv"
	"strings"
)

// Constraints describes a user's requirements of the hardware on which units
// of a service will run. Constraints are used to choose an existing machine
// onto which a unit will be deployed, or to provision a new machine if no
// existing one satisfies the requirements.
type Constraints struct {
	Cores *int
	Mem   *float64
}

// String expresses a Constraints in the language in which it was specified.
func (c Constraints) String() string {
	var strs []string
	if c.Cores != nil {
		strs = append(strs, fmt.Sprintf("cores=%d", *c.Cores))
	}
	if c.Mem != nil {
		s := strconv.FormatFloat(*c.Mem, 'f', -1, 64)
		strs = append(strs, fmt.Sprintf("mem=%sM", s))
	}
	return strings.Join(strs, " ")
}
