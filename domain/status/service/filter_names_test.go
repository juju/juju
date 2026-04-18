// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"slices"
	"testing"

	"github.com/juju/tc"

	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
)

type filterNamesSuite struct{}

func TestFilterNamesSuite(t *testing.T) {
	tc.Run(t, &filterNamesSuite{})
}

func (s *filterNamesSuite) TestMatchStatusNamesApplicationIncludesUnitsAndMachines(c *tc.C) {
	units := map[coreunit.Name]Unit{
		"mysql/0": {
			ApplicationName: "mysql",
			MachineName:     ptrMachineName("1"),
		},
		"wordpress/0": {
			ApplicationName: "wordpress",
			MachineName:     ptrMachineName("2"),
		},
	}
	applications := map[string]Application{
		"mysql": {
			Units: map[coreunit.Name]Unit{
				"mysql/0": units["mysql/0"],
			},
		},
		"wordpress": {
			Units: map[coreunit.Name]Unit{
				"wordpress/0": units["wordpress/0"],
			},
		},
	}
	machines := map[coremachine.Name]Machine{
		"1": {Name: "1"},
		"2": {Name: "2"},
	}

	result := MatchStatusNames([]string{"mysql"}, applications, units, machines)

	c.Check(sortedAppNames(result), tc.DeepEquals, []string{"mysql"})
	c.Check(sortedUnitNames(result), tc.DeepEquals, []string{"mysql/0"})
	c.Check(sortedMachineNames(result), tc.DeepEquals, []string{"1"})
}

func (s *filterNamesSuite) TestMatchStatusNamesPrincipalUnitIncludesSubordinate(c *tc.C) {
	units := map[coreunit.Name]Unit{
		"mysql/0": {
			ApplicationName:  "mysql",
			MachineName:      ptrMachineName("1"),
			SubordinateNames: []coreunit.Name{"logging/0"},
		},
		"logging/0": {
			ApplicationName: "logging",
			Subordinate:     true,
			PrincipalName:   ptrUnitName("mysql/0"),
		},
	}
	applications := map[string]Application{
		"mysql": {
			Units: map[coreunit.Name]Unit{
				"mysql/0": units["mysql/0"],
			},
		},
		"logging": {
			Units: map[coreunit.Name]Unit{
				"logging/0": units["logging/0"],
			},
		},
	}
	machines := map[coremachine.Name]Machine{
		"1": {Name: "1"},
	}

	result := MatchStatusNames([]string{"mysql/0"}, applications, units, machines)

	c.Check(sortedAppNames(result), tc.DeepEquals, []string{"logging", "mysql"})
	c.Check(sortedUnitNames(result), tc.DeepEquals, []string{"logging/0", "mysql/0"})
	c.Check(sortedMachineNames(result), tc.DeepEquals, []string{"1"})
}

func (s *filterNamesSuite) TestMatchStatusNamesSubordinateIncludesPrincipal(c *tc.C) {
	units := map[coreunit.Name]Unit{
		"mysql/0": {
			ApplicationName:  "mysql",
			MachineName:      ptrMachineName("1"),
			SubordinateNames: []coreunit.Name{"logging/0"},
		},
		"logging/0": {
			ApplicationName: "logging",
			Subordinate:     true,
			PrincipalName:   ptrUnitName("mysql/0"),
		},
	}
	applications := map[string]Application{
		"mysql": {
			Units: map[coreunit.Name]Unit{
				"mysql/0": units["mysql/0"],
			},
		},
		"logging": {
			Units: map[coreunit.Name]Unit{
				"logging/0": units["logging/0"],
			},
		},
	}
	machines := map[coremachine.Name]Machine{
		"1": {Name: "1"},
	}

	result := MatchStatusNames([]string{"logging/0"}, applications, units, machines)

	c.Check(sortedAppNames(result), tc.DeepEquals, []string{"logging", "mysql"})
	c.Check(sortedUnitNames(result), tc.DeepEquals, []string{"logging/0", "mysql/0"})
	c.Check(sortedMachineNames(result), tc.DeepEquals, []string{"1"})
}

func (s *filterNamesSuite) TestMatchStatusNamesMachineIncludesHostedUnitsAndContainers(c *tc.C) {
	units := map[coreunit.Name]Unit{
		"mysql/0": {
			ApplicationName: "mysql",
			MachineName:     ptrMachineName("0"),
		},
		"wordpress/0": {
			ApplicationName: "wordpress",
			MachineName:     ptrMachineName("0/lxd/0"),
		},
	}
	applications := map[string]Application{
		"mysql": {
			Units: map[coreunit.Name]Unit{
				"mysql/0": units["mysql/0"],
			},
		},
		"wordpress": {
			Units: map[coreunit.Name]Unit{
				"wordpress/0": units["wordpress/0"],
			},
		},
	}
	machines := map[coremachine.Name]Machine{
		"0":       {Name: "0"},
		"0/lxd/0": {Name: "0/lxd/0"},
		"1":       {Name: "1"},
	}

	result := MatchStatusNames([]string{"0"}, applications, units, machines)

	c.Check(sortedAppNames(result), tc.DeepEquals, []string{"mysql", "wordpress"})
	c.Check(sortedUnitNames(result), tc.DeepEquals, []string{"mysql/0", "wordpress/0"})
	c.Check(sortedMachineNames(result), tc.DeepEquals, []string{"0", "0/lxd/0"})
}

func ptrMachineName(name coremachine.Name) *coremachine.Name {
	return &name
}

func ptrUnitName(name coreunit.Name) *coreunit.Name {
	return &name
}

func sortedAppNames(result NameMatchResult) []string {
	out := make([]string, 0, len(result.Applications))
	for name := range result.Applications {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

func sortedUnitNames(result NameMatchResult) []string {
	out := make([]string, 0, len(result.Units))
	for name := range result.Units {
		out = append(out, name.String())
	}
	slices.Sort(out)
	return out
}

func sortedMachineNames(result NameMatchResult) []string {
	out := make([]string, 0, len(result.Machines))
	for name := range result.Machines {
		out = append(out, name.String())
	}
	slices.Sort(out)
	return out
}
