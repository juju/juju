// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
)

// NameMatchResult records the entities that should be retained after applying
// name-based status selectors.
type NameMatchResult struct {
	Applications map[string]struct{}
	Units        map[coreunit.Name]struct{}
	Machines     map[coremachine.Name]struct{}
}

// MatchStatusNames matches exact machine, application, and unit names from the
// supplied status snapshot and expands the result enough to keep status output
// coherent.
func MatchStatusNames(
	patterns []string,
	applications map[string]Application,
	units map[coreunit.Name]Unit,
	machines map[coremachine.Name]Machine,
) NameMatchResult {
	result := newNameMatchResult()
	if len(patterns) == 0 {
		for appName := range applications {
			result.addApplication(appName)
		}
		for unitName := range units {
			result.addUnit(unitName)
		}
		for machineName := range machines {
			result.addMachine(machineName)
		}
		return result
	}

	patternSet := make(map[string]struct{}, len(patterns))
	for _, pattern := range patterns {
		patternSet[pattern] = struct{}{}
	}

	directMachines := make(map[coremachine.Name]struct{})
	for appName, app := range applications {
		if _, ok := patternSet[appName]; !ok {
			continue
		}
		result.addApplication(appName)
		for unitName := range app.Units {
			result.addUnit(unitName)
		}
	}
	for unitName := range units {
		if _, ok := patternSet[unitName.String()]; ok {
			result.addUnit(unitName)
		}
	}
	for machineName := range machines {
		if _, ok := patternSet[machineName.String()]; ok {
			directMachines[machineName] = struct{}{}
		}
	}

	for machineName := range directMachines {
		result.addMachine(machineName)
		if machineName.IsContainer() {
			result.addMachine(machineName.Parent())
			continue
		}
		for candidate := range machines {
			if candidate == machineName || candidate.Parent() == machineName {
				result.addMachine(candidate)
			}
		}
	}

	for unitName := range units {
		machineName, ok := machineNameForUnit(unitName, units)
		if ok && result.hasMachine(machineName) {
			result.addUnit(unitName)
		}
	}

	result.expandUnitClosure(units)
	for unitName := range result.Units {
		machineName, ok := machineNameForUnit(unitName, units)
		if !ok {
			continue
		}
		result.addMachine(machineName)
		if machineName.IsContainer() {
			result.addMachine(machineName.Parent())
		}
	}

	return result
}

func newNameMatchResult() NameMatchResult {
	return NameMatchResult{
		Applications: make(map[string]struct{}),
		Units:        make(map[coreunit.Name]struct{}),
		Machines:     make(map[coremachine.Name]struct{}),
	}
}

func (r NameMatchResult) addApplication(name string) bool {
	if _, ok := r.Applications[name]; ok {
		return false
	}
	r.Applications[name] = struct{}{}
	return true
}

func (r NameMatchResult) addUnit(name coreunit.Name) bool {
	if _, ok := r.Units[name]; ok {
		return false
	}
	r.Units[name] = struct{}{}
	return true
}

func (r NameMatchResult) addMachine(name coremachine.Name) bool {
	if _, ok := r.Machines[name]; ok {
		return false
	}
	r.Machines[name] = struct{}{}
	return true
}

func (r NameMatchResult) hasMachine(name coremachine.Name) bool {
	_, ok := r.Machines[name]
	return ok
}

func (r NameMatchResult) expandUnitClosure(units map[coreunit.Name]Unit) {
	changed := true
	for changed {
		changed = false
		for unitName := range r.Units {
			unit, ok := units[unitName]
			if !ok {
				continue
			}
			if r.addApplication(unit.ApplicationName) {
				changed = true
			}
			if unit.Subordinate {
				if unit.PrincipalName != nil && r.addUnit(*unit.PrincipalName) {
					changed = true
				}
				continue
			}
			for _, subordinateName := range unit.SubordinateNames {
				if _, ok := units[subordinateName]; !ok {
					continue
				}
				if r.addUnit(subordinateName) {
					changed = true
				}
			}
		}
	}
}

func machineNameForUnit(unitName coreunit.Name, units map[coreunit.Name]Unit) (coremachine.Name, bool) {
	return machineNameForUnitWithVisited(unitName, units, make(map[coreunit.Name]struct{}))
}

func machineNameForUnitWithVisited(
	unitName coreunit.Name,
	units map[coreunit.Name]Unit,
	visited map[coreunit.Name]struct{},
) (coremachine.Name, bool) {
	if _, ok := visited[unitName]; ok {
		return "", false
	}
	visited[unitName] = struct{}{}

	unit, ok := units[unitName]
	if !ok {
		return "", false
	}
	if unit.MachineName != nil {
		return *unit.MachineName, true
	}
	if unit.PrincipalName == nil {
		return "", false
	}
	return machineNameForUnitWithVisited(*unit.PrincipalName, units, visited)
}
