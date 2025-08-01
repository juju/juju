// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bundlechanges

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/juju/charm/v12"
	"github.com/juju/collections/set"
	"github.com/juju/names/v5"
	"github.com/kr/pretty"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/internal/naturalsort"
)

const kubernetes = "kubernetes"

// Model represents the existing deployment if any.
type Model struct {
	Applications map[string]*Application
	Machines     map[string]*Machine
	Relations    []Relation

	// ConstraintsEqual is a function that is able to determine if two
	// string values defining constraints are equal. This is to avoid a
	// hard dependency on the juju constraints package.
	ConstraintsEqual func(string, string) bool

	// Sequence holds a map of names to the next "number" that relates
	// to the unit or machine. The keys are "application-<name>", the string
	// "machine", or "machine-id/c" where n is a machine id, and c is a
	// container type.
	Sequence map[string]int

	// The Sequence map isn't touched during the processing of of bundle
	// changes, but we need to keep track, so a copy is made.
	sequence map[string]int

	// This is a mapping of existing machines to machines in the bundle.
	MachineMap map[string]string

	logger Logger
}

// Relation holds the information between two releations.
type Relation struct {
	App1      string
	Endpoint1 string
	App2      string
	Endpoint2 string
}

func (m *Model) pretty() string {
	// Hide the logger from the model output.
	another := *m
	another.logger = nil
	return pretty.Sprint(another)
}

func (m *Model) initializeSequence() {
	m.sequence = make(map[string]int)
	if m.Sequence != nil {
		for key, value := range m.Sequence {
			m.sequence[key] = value
		}
		// We assume that if the mapping was specified, a complete mapping was
		// specified.
		return
	}
	// Work to infer the mapping.

	for appName, app := range m.Applications {
		for _, unit := range app.Units {
			// This is pure paranoia, to avoid panics.
			if !names.IsValidUnit(unit.Name) {
				continue
			}
			u := names.NewUnitTag(unit.Name)
			unitNumber := u.Number()
			key := "application-" + appName
			if existing := m.sequence[key]; existing <= unitNumber {
				m.sequence[key] = unitNumber + 1
			}
		}
	}

	for machineID := range m.Machines {
		// Continued paranoia.
		if !names.IsValidMachine(machineID) {
			continue
		}
		tag := names.NewMachineTag(machineID)
		key := "machine"
		// We know that the child id is always a valid integer.
		n, _ := strconv.Atoi(tag.ChildId())
		if containerType := tag.ContainerType(); containerType != "" {
			key = "machine-" + tag.Parent().Id() + "/" + containerType
		}
		if existing := m.sequence[key]; existing <= n {
			m.sequence[key] = n + 1
		}
	}
}

func (m *Model) nextMachine() string {
	value := m.sequence["machine"]
	m.sequence["machine"] = value + 1
	return strconv.Itoa(value)
}

func (m *Model) nextContainer(parentID, containerType string) string {
	key := "machine-" + parentID + "/" + containerType
	value := m.sequence[key]
	m.sequence[key] = value + 1
	return fmt.Sprintf("%s/%s/%d", parentID, containerType, value)
}

func (m *Model) nextUnit(appName string) string {
	key := "application-" + appName
	value := m.sequence[key]
	m.sequence[key] = value + 1
	return fmt.Sprintf("%s/%d", appName, value)
}

// HasRelation checks to see if the model has a relation between two
// applications.
func (m *Model) HasRelation(app1, endpoint1, app2, endpoint2 string) bool {
	for _, rel := range m.Relations {
		oneWay := Relation{
			App1: app1, Endpoint1: endpoint1, App2: app2, Endpoint2: endpoint2,
		}
		other := Relation{
			App1: app2, Endpoint1: endpoint2, App2: app1, Endpoint2: endpoint1,
		}
		if rel == oneWay || rel == other {
			return true
		}
	}
	return false
}

func topLevelMachine(machineID string) string {
	if !names.IsContainerMachine(machineID) {
		return machineID
	}
	tag := names.NewMachineTag(machineID)
	return topLevelMachine(tag.Parent().Id())
}

// InferMachineMap looks at all the machines defined in the bundle
// and infers their mapping to the existing machine.
// This method assumes that the units of an application are sorted
// in the natural sort order, meaning we start at unit zero and work
// our way up the unit numbers.
func (m *Model) InferMachineMap(data *charm.BundleData) {
	if m.MachineMap == nil {
		m.MachineMap = make(map[string]string)
	}
	e := newInference(m, data)
	e.processInitialPlacements()
	e.processBundleMachines()
}

// BundleMachine will return a the existing machine for the specified bundle
// machine ID. If there is not a mapping available, nil is returned.
func (m *Model) BundleMachine(id string) *Machine {
	if m.Machines == nil {
		return nil
	}
	// If the id isn't specified in the machine map, the empty string
	// is returned. If the no existing machine maps to the machine id,
	// a nil is returned from the Machines map.
	return m.Machines[m.MachineMap[id]]
}

func (m *Model) getUnitMachine(appName string, index int) string {
	if m.Applications == nil {
		return ""
	}
	app := m.Applications[appName]
	if app == nil {
		return ""
	}
	target := fmt.Sprintf("%s/%d", appName, index)
	for _, unit := range app.Units {
		if unit.Name == target {
			return unit.Machine
		}
	}
	return ""
}

// Application represents an existing charm deployed in the model.
type Application struct {
	Name             string
	Charm            string // The charm URL.
	Scale            int
	Options          map[string]interface{}
	Annotations      map[string]string
	Constraints      string // TODO: not updated yet.
	Exposed          bool
	ExposedEndpoints map[string]ExposedEndpoint
	SubordinateTo    []string
	Base             corebase.Base
	Channel          string
	Revision         int
	Placement        string
	Offers           []string
	// TODO: handle changes in:
	//   storage

	Units []Unit
}

// ExposedEndpoint encapsulates the expose-related parameters for a
// particular endpoint.
type ExposedEndpoint struct {
	ExposeToSpaces []string
	ExposeToCIDRs  []string
}

// Unit represents a unit in the model.
type Unit struct {
	Name    string
	Machine string
}

// Machine represents an existing machine in the model.
type Machine struct {
	ID          string
	Base        corebase.Base
	Annotations map[string]string
}

func (m *Model) hasCharm(charm string, revision int) bool {
	if len(m.Applications) == 0 {
		return false
	}
	for _, app := range m.Applications {
		if app.Charm == charm && ((revision >= 0 && revision == app.Revision) || revision < 0) {
			return true
		}
	}
	return false
}

func (m *Model) matchesCharmPermutation(charm, arch string, base corebase.Base, channel string, revision int, constraintGetter ConstraintGetter) bool {
	if arch == "" && base.Empty() && channel == "" {
		return m.hasCharm(charm, revision)
	}

	for _, app := range m.Applications {
		var appArch string
		if constraintGetter != nil {
			// If we can't solve the constraints, then we have to skip this
			// application.
			cons := constraintGetter(app.Constraints)
			var err error
			if appArch, err = cons.Arch(); err != nil {
				continue
			}
		}

		if app.Charm == charm &&
			appArch == arch &&
			app.Base == base &&
			app.Channel == channel &&
			((revision >= 0 && revision == app.Revision) || revision < 0) {
			return true
		}
	}
	return false
}

// GetApplication returns the application specified or nil
// if it doesn't have it.
func (m *Model) GetApplication(name string) *Application {
	return m.Applications[name]
}

func (m *Model) unitMachinesWithoutApp(sourceApp, targetApp, container string) []string {
	source := m.GetApplication(sourceApp)
	if source == nil {
		return []string{}
	}

	target := m.GetApplication(targetApp)
	machines := set.NewStrings()
	for _, unit := range source.Units {
		machines.Add(topLevelMachine(unit.Machine))
	}
	if target != nil {
		for _, unit := range target.Units {
			if container == "" {
				machines.Remove(unit.Machine)
			} else {
				machineTag := names.NewMachineTag(unit.Machine)
				if machineTag.ContainerType() == container {
					machines.Remove(topLevelMachine(unit.Machine))
				}
			}
		}
	}

	return naturalsort.Sort(machines.Values())
}

func (m *Model) unsatisfiedMachineAndUnitPlacements(sourceApp string, placements []string) []string {
	// Cases we care about here are machine or unit placement.
	source := m.GetApplication(sourceApp)
	if source == nil {
		// Return a copy of the slice.
		return append([]string(nil), placements...)
	}

	var result []string

	for _, value := range placements {
		p, _ := charm.ParsePlacement(value)
		switch {
		case p.Machine == "new":
			result = append(result, value)
		case p.Machine != "":
			if !m.machineHasApp(p.Machine, sourceApp, p.ContainerType) {
				result = append(result, value)
			}
		case p.Application != "" && p.Unit < 0:
			result = append(result, value)
		case p.Application != "":
			machine := m.getUnitMachine(p.Application, p.Unit)
			if machine == "" {
				// This is unsatisfied because we don't have that unit.
				result = append(result, value)
			} else if !m.machineHasApp(machine, sourceApp, p.ContainerType) {
				result = append(result, value)
			}
		}
	}
	return result
}

func (m *Model) machineHasApp(machine, appName, containerType string) bool {
	return m.getAppUnitOnMachine(machine, appName, containerType) != ""
}

func (m *Model) getAppUnitOnMachine(machine, appName, containerType string) string {
	if mappedMachine, ok := m.MachineMap[machine]; ok {
		machine = mappedMachine
	}
	app := m.GetApplication(appName)
	if app == nil {
		return ""
	}
	for _, u := range app.Units {
		machineTag := names.NewMachineTag(u.Machine)
		if containerType == "" {
			if machineTag.ContainerType() == "" && machineTag.Id() == machine {
				return u.Name
			}
		} else {
			if machineTag.ContainerType() == containerType &&
				machineTag.Parent().Id() == machine {
				return u.Name
			}
		}
	}
	return ""
}

func (a *Application) unitCount() int {
	if a == nil {
		return 0
	}
	return len(a.Units)
}

func (a *Application) changedAnnotations(annotations map[string]string) map[string]string {
	if a == nil || len(a.Annotations) == 0 {
		return annotations
	}
	changes := make(map[string]string)
	for key, value := range annotations {
		current, found := a.Annotations[key]
		if !found || current != value {
			changes[key] = value
		}
	}
	return changes
}

func (a *Application) changedOptions(options map[string]interface{}) map[string]interface{} {
	if a == nil || len(a.Options) == 0 {
		return options
	}
	changes := make(map[string]interface{})
	for key, value := range options {
		current, found := a.Options[key]
		// options should have been validated by now to only contain comparable
		// types. Here we assume that the options have the correct type, and the
		// existing options have possibly been passed through JSON serialization
		// which converts int values to floats.
		switch value.(type) {
		case int:
			// If the validation code has done its job, the option from the
			// model should be a number too.
			switch cv := current.(type) {
			case float64: // JSON encoding converts ints to floats.
				current = int(cv)
			}
		}
		if !found || current != value {
			changes[key] = value
		}
	}
	return changes
}

func (m *Machine) changedAnnotations(annotations map[string]string) map[string]string {
	if m == nil || len(m.Annotations) == 0 {
		return annotations
	}
	changes := make(map[string]string)
	for key, value := range annotations {
		current, found := m.Annotations[key]
		if !found || current != value {
			changes[key] = value
		}
	}
	return changes
}

type inferenceEngine struct {
	model  *Model
	bundle *charm.BundleData

	appUnits      map[string][]Unit
	appPlacements map[string][]string

	initialMachines set.Strings
	logger          Logger
}

func newInference(m *Model, data *charm.BundleData) *inferenceEngine {
	appUnits := make(map[string][]Unit)
	// The initialMachines starts by including all the targets defined
	// by the user for the machine map.
	initialMachines := set.NewStrings()
	for _, target := range m.MachineMap {
		initialMachines.Add(target)
	}
	for appName, app := range m.Applications {
		var units []Unit
		// If the unit is on a machine we have already been told the mapping for,
		// skip it in the inference.
		for _, unit := range app.Units {
			machine := topLevelMachine(unit.Machine)
			if !initialMachines.Contains(machine) {
				units = append(units, unit)
			}
		}
		appUnits[appName] = units
	}
	return &inferenceEngine{
		model:           m,
		bundle:          data,
		appUnits:        appUnits,
		appPlacements:   make(map[string][]string),
		initialMachines: initialMachines,
		logger:          m.logger,
	}
}

// processInitialPlacements goes through all the application placement directives
// and looks to see if there is a machine in the model that has the placement
// satisfied.
func (e *inferenceEngine) processInitialPlacements() {
	for appName, app := range e.bundle.Applications {
		var unused []string
		for _, to := range app.To {
			unused = append(unused, to)
			// Here we explicitly ignore the error return of the parse placement
			// as the bundle should have been fully validated by now, which does
			// check the placement. However we do check to make sure the placement
			// is not nil (which it would be in an error case), because we don't
			// want to panic if, for some weird reason, it does error.
			placement, _ := charm.ParsePlacement(to)
			if placement == nil || placement.Machine == "" {
				continue
			}
			// If this machine is mapped already, skip this one.
			machine := placement.Machine
			if _, ok := e.model.MachineMap[machine]; ok {
				continue
			}
			if uName := e.model.getAppUnitOnMachine(machine, appName, placement.ContainerType); uName != "" {
				e.model.MachineMap[machine] = machine
				e.initialMachines.Add(machine)
				e.markUnitUsed(appName, uName)
				e.logger.Tracef("unit %q satisfies %q", uName, to)
				e.logger.Tracef("units left: %v", e.appUnits[appName])
				// If we did use it, take it off the end.
				unused = unused[:len(unused)-1]
			}
		}
		sort.Strings(unused)
		e.appPlacements[appName] = unused
		e.logger.Tracef("unused placements: %#v", unused)
	}
}

func (e *inferenceEngine) markUnitUsed(appName, uName string) {
	units := e.appUnits[appName]
	for idx, unit := range units {
		if unit.Name == uName {
			e.appUnits[appName] = append(units[:idx], units[idx+1:]...)
			return
		}
	}
}

// processBundleMachines is the second pass inference where we check each of the machines
// defined in the bundle, and look to see if there are placement directives that target those
// machines.
func (e *inferenceEngine) processBundleMachines() {
	var ids []string
	for id := range e.bundle.Machines {
		ids = append(ids, id)
	}
	naturalsort.Sort(ids)

	applications := make([]string, 0, len(e.bundle.Applications))
	for appName := range e.bundle.Applications {
		applications = append(applications, appName)
	}
	sort.Strings(applications)

mainloop:
	for _, id := range ids {
		// The simplest case is where the user has specified a mapping
		// for us.
		if _, found := e.model.MachineMap[id]; found {
			continue
		}
		e.logger.Tracef("machine: %s", id)
		// Look for a unit placement directive that specifies the machine.
		for _, appName := range applications {
			e.logger.Tracef("app: %s", appName)
			for _, to := range e.appPlacements[appName] {
				// Here we explicitly ignore the error return of the parse placement
				// as the bundle should have been fully validated by now, which does
				// check the placement. However we do check to make sure the placement
				// is not nil (which it would be in an error case), because we don't
				// want to panic if, for some weird reason, it does error.
				e.logger.Tracef("to: %s", to)
				placement, _ := charm.ParsePlacement(to)
				if placement == nil || placement.Machine != id {
					continue
				}

				deployed := e.appUnits[appName]
				// See if we have deployed this unit yet.
				if len(deployed) == 0 {
					continue
				}
				// Find the first unit that we haven't already used.
				unit := deployed[0]

				e.logger.Tracef("unit: %#v", unit)
				machine := topLevelMachine(unit.Machine)
				if e.initialMachines.Contains(machine) {
					// Can't match the same machine twice.
					continue
				}
				e.model.MachineMap[id] = machine
				e.appUnits[appName] = deployed[1:]
				continue mainloop
			}
		}
	}
}
