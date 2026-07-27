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
			MachineName:     new(coremachine.Name("1")),
		},
		"wordpress/0": {
			ApplicationName: "wordpress",
			MachineName:     new(coremachine.Name("2")),
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

	result := MatchStatusNames([]string{"mysql"}, applications, units, machines, nil)

	c.Check(sortedAppNames(result), tc.DeepEquals, []string{"mysql"})
	c.Check(sortedUnitNames(result), tc.DeepEquals, []string{"mysql/0"})
	c.Check(sortedMachineNames(result), tc.DeepEquals, []string{"1"})
}

func (s *filterNamesSuite) TestMatchStatusNamesPrincipalUnitIncludesSubordinate(c *tc.C) {
	units := map[coreunit.Name]Unit{
		"mysql/0": {
			ApplicationName:  "mysql",
			MachineName:      new(coremachine.Name("1")),
			SubordinateNames: []coreunit.Name{"logging/0"},
		},
		"logging/0": {
			ApplicationName: "logging",
			Subordinate:     true,
			PrincipalName:   new(coreunit.Name("mysql/0")),
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

	result := MatchStatusNames([]string{"mysql/0"}, applications, units, machines, nil)

	c.Check(sortedAppNames(result), tc.DeepEquals, []string{"logging", "mysql"})
	c.Check(sortedUnitNames(result), tc.DeepEquals, []string{"logging/0", "mysql/0"})
	c.Check(sortedMachineNames(result), tc.DeepEquals, []string{"1"})
}

func (s *filterNamesSuite) TestMatchStatusNamesSubordinateIncludesPrincipal(c *tc.C) {
	units := map[coreunit.Name]Unit{
		"mysql/0": {
			ApplicationName:  "mysql",
			MachineName:      new(coremachine.Name("1")),
			SubordinateNames: []coreunit.Name{"logging/0"},
		},
		"logging/0": {
			ApplicationName: "logging",
			Subordinate:     true,
			PrincipalName:   new(coreunit.Name("mysql/0")),
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

	result := MatchStatusNames([]string{"logging/0"}, applications, units, machines, nil)

	c.Check(sortedAppNames(result), tc.DeepEquals, []string{"logging", "mysql"})
	c.Check(sortedUnitNames(result), tc.DeepEquals, []string{"logging/0", "mysql/0"})
	c.Check(sortedMachineNames(result), tc.DeepEquals, []string{"1"})
}

func (s *filterNamesSuite) TestMatchStatusNamesMachineIncludesHostedUnitsAndContainers(c *tc.C) {
	units := map[coreunit.Name]Unit{
		"mysql/0": {
			ApplicationName: "mysql",
			MachineName:     new(coremachine.Name("0")),
		},
		"wordpress/0": {
			ApplicationName: "wordpress",
			MachineName:     new(coremachine.Name("0/lxd/0")),
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

	result := MatchStatusNames([]string{"0"}, applications, units, machines, nil)

	c.Check(sortedAppNames(result), tc.DeepEquals, []string{"mysql", "wordpress"})
	c.Check(sortedUnitNames(result), tc.DeepEquals, []string{"mysql/0", "wordpress/0"})
	c.Check(sortedMachineNames(result), tc.DeepEquals, []string{"0", "0/lxd/0"})
}

func (s *filterNamesSuite) TestMatchStatusNamesLeaderPatternResolvesToLeaderUnit(c *tc.C) {
	units := map[coreunit.Name]Unit{
		"mysql/0": {
			ApplicationName: "mysql",
			MachineName:     new(coremachine.Name("1")),
		},
		"mysql/1": {
			ApplicationName: "mysql",
			MachineName:     new(coremachine.Name("2")),
		},
	}
	applications := map[string]Application{
		"mysql": {
			Units: map[coreunit.Name]Unit{
				"mysql/0": units["mysql/0"],
				"mysql/1": units["mysql/1"],
			},
		},
	}
	machines := map[coremachine.Name]Machine{
		"1": {Name: "1"},
		"2": {Name: "2"},
	}
	leaders := map[string]string{"mysql": "mysql/1"}

	result := MatchStatusNames([]string{"mysql/leader"}, applications, units, machines, leaders)
	c.Check(sortedAppNames(result), tc.DeepEquals, []string{"mysql"})
	c.Check(sortedUnitNames(result), tc.DeepEquals, []string{"mysql/1"})
	c.Check(sortedMachineNames(result), tc.DeepEquals, []string{"2"})
}

func (s *filterNamesSuite) TestMatchStatusNamesLeaderPatternUnresolvedMatchesNothing(c *tc.C) {
	units := map[coreunit.Name]Unit{
		"mysql/0": {
			ApplicationName: "mysql",
			MachineName:     new(coremachine.Name("1")),
		},
	}
	applications := map[string]Application{
		"mysql": {
			Units: map[coreunit.Name]Unit{
				"mysql/0": units["mysql/0"],
			},
		},
	}
	machines := map[coremachine.Name]Machine{
		"1": {Name: "1"},
	}

	// No leader known — pattern is left unchanged and matches nothing.
	result := MatchStatusNames([]string{"mysql/leader"}, applications, units, machines, nil)
	c.Check(sortedAppNames(result), tc.HasLen, 0)
	c.Check(sortedUnitNames(result), tc.HasLen, 0)
	c.Check(sortedMachineNames(result), tc.HasLen, 0)
}

func (s *filterNamesSuite) TestMatchStatusNamesLeaderPatternMixedWithOtherPatterns(c *tc.C) {
	units := map[coreunit.Name]Unit{
		"mysql/0": {
			ApplicationName: "mysql",
			MachineName:     new(coremachine.Name("1")),
		},
		"mysql/1": {
			ApplicationName: "mysql",
			MachineName:     new(coremachine.Name("2")),
		},
		"wordpress/0": {
			ApplicationName: "wordpress",
			MachineName:     new(coremachine.Name("3")),
		},
	}
	applications := map[string]Application{
		"mysql": {
			Units: map[coreunit.Name]Unit{
				"mysql/0": units["mysql/0"],
				"mysql/1": units["mysql/1"],
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
		"3": {Name: "3"},
	}
	leaders := map[string]string{"mysql": "mysql/1"}

	// mysql/leader resolves to mysql/1; wordpress/leader has no leader and is
	// dropped; mysql/0 is a direct unit pattern.
	result := MatchStatusNames(
		[]string{"mysql/leader", "wordpress/leader", "mysql/0"},
		applications, units, machines, leaders,
	)
	c.Check(sortedAppNames(result), tc.DeepEquals, []string{"mysql"})
	c.Check(sortedUnitNames(result), tc.DeepEquals, []string{"mysql/0", "mysql/1"})
	c.Check(sortedMachineNames(result), tc.DeepEquals, []string{"1", "2"})
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
