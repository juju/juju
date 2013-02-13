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
	CpuCores *int
	CpuPower *float64
	Mem      *float64
}

// String expresses a Constraints in the language in which it was specified.
func (c Constraints) String() string {
	var strs []string
	if c.CpuCores != nil {
		strs = append(strs, "cpu-cores="+intStr(*c.CpuCores))
	}
	if c.CpuPower != nil {
		strs = append(strs, "cpu-power="+floatStr(*c.CpuPower))
	}
	if c.Mem != nil {
		s := floatStr(*c.Mem)
		if s != "" {
			s += "M"
		}
		strs = append(strs, "mem="+s)
	}
	return strings.Join(strs, " ")
}

func intStr(i int) string {
	if i == 0 {
		return ""
	}
	return fmt.Sprintf("%d", i)
}

func floatStr(f float64) string {
	if f == 0 {
		return ""
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}
