// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bundlechanges

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable"
)

// handleServices populates the change set with "addCharm"/"addApplication" records.
// This function also handles adding application annotations.
func handleApplications(add func(Change), services map[string]*charm.ApplicationSpec, defaultSeries string) map[string]string {
	charms := make(map[string]string, len(services))
	addedServices := make(map[string]string, len(services))
	// Iterate over the map using its sorted keys so that results are
	// deterministic and easier to test.
	names := make([]string, 0, len(services))
	for name, _ := range services {
		names = append(names, name)
	}
	sort.Strings(names)
	var change Change
	for _, name := range names {
		application := services[name]
		series := getSeries(application, defaultSeries)
		// Add the addCharm record if one hasn't been added yet.
		if charms[application.Charm] == "" {
			change = newAddCharmChange(AddCharmParams{
				Charm:  application.Charm,
				Series: series,
			})
			add(change)
			charms[application.Charm] = change.Id()
		}

		// Add the addApplication record for this application.
		change = newAddApplicationChange(AddApplicationParams{
			Charm:            "$" + charms[application.Charm],
			Series:           series,
			Application:      name,
			Options:          application.Options,
			Constraints:      application.Constraints,
			Storage:          application.Storage,
			EndpointBindings: application.EndpointBindings,
			Resources:        application.Resources,
		}, charms[application.Charm])
		add(change)
		id := change.Id()
		addedServices[name] = id

		// Expose the application if required.
		if application.Expose {
			add(newExposeChange(ExposeParams{
				Application: "$" + id,
			}, id))
		}

		// Add application annotations.
		if len(application.Annotations) > 0 {
			add(newSetAnnotationsChange(SetAnnotationsParams{
				EntityType:  ApplicationType,
				Id:          "$" + id,
				Annotations: application.Annotations,
			}, id))
		}
	}
	return addedServices
}

// handleMachines populates the change set with "addMachines" records.
// This function also handles adding machine annotations.
func handleMachines(add func(Change), machines map[string]*charm.MachineSpec, defaultSeries string) map[string]string {
	addedMachines := make(map[string]string, len(machines))
	// Iterate over the map using its sorted keys so that results are
	// deterministic and easier to test.
	names := make([]string, 0, len(machines))
	for name, _ := range machines {
		names = append(names, name)
	}
	sort.Strings(names)
	var change Change
	for _, name := range names {
		machine := machines[name]
		if machine == nil {
			machine = &charm.MachineSpec{}
		}
		series := machine.Series
		if series == "" {
			series = defaultSeries
		}
		// Add the addMachines record for this machine.
		change = newAddMachineChange(AddMachineParams{
			Series:      series,
			Constraints: machine.Constraints,
		})
		add(change)
		addedMachines[name] = change.Id()

		// Add machine annotations.
		if len(machine.Annotations) > 0 {
			add(newSetAnnotationsChange(SetAnnotationsParams{
				EntityType:  MachineType,
				Id:          "$" + change.Id(),
				Annotations: machine.Annotations,
			}, change.Id()))
		}
	}
	return addedMachines
}

// handleRelations populates the change set with "addRelation" records.
func handleRelations(add func(Change), relations [][]string, addedServices map[string]string) {
	for _, relation := range relations {
		// Add the addRelation record for this relation pair.
		args := make([]string, 2)
		requires := make([]string, 2)
		for i, endpoint := range relation {
			ep := parseEndpoint(endpoint)
			application := addedServices[ep.application]
			requires[i] = application
			ep.application = application
			args[i] = "$" + ep.String()
		}
		add(newAddRelationChange(AddRelationParams{
			Endpoint1: args[0],
			Endpoint2: args[1],
		}, requires...))
	}
}

// handleUnits populates the change set with "addUnit" records.
// It also handles adding machine containers where to place units if required.
func handleUnits(add func(Change), services map[string]*charm.ApplicationSpec, addedServices, addedMachines map[string]string, defaultSeries string) {
	records := make(map[string]*AddUnitChange)
	// Iterate over the map using its sorted keys so that results are
	// deterministic and easier to test.
	names := make([]string, 0, len(services))
	for name, _ := range services {
		names = append(names, name)
	}
	sort.Strings(names)
	// Collect and add all unit changes. These records are likely to be
	// modified later in order to handle unit placement.
	for _, name := range names {
		application := services[name]
		for i := 0; i < application.NumUnits; i++ {
			addedApplication := addedServices[name]
			change := newAddUnitChange(AddUnitParams{
				Application: "$" + addedApplication,
			}, addedApplication)
			add(change)
			records[fmt.Sprintf("%s/%d", name, i)] = change
		}
	}
	// Now handle unit placement for each added application unit.
	for _, name := range names {
		application := services[name]
		numPlaced := len(application.To)
		if numPlaced == 0 {
			// If there are no placement directives it means that either the
			// application has no units (in which case there is no need to
			// proceed), or the units are not placed (in which case there is no
			// need to modify the change already added above).
			continue
		}
		// servicePlacedUnits holds, for each application, the number of units of
		// the current application already placed to that application.
		servicePlacedUnits := make(map[string]int)
		// At this point we know that we have at least one placement directive.
		// Fill the other ones if required.
		lastPlacement := application.To[numPlaced-1]
		for i := 0; i < application.NumUnits; i++ {
			p := lastPlacement
			if i < numPlaced {
				p = application.To[i]
			}
			// Generate the changes required in order to place this unit, and
			// retrieve the identifier of the parent change.
			parentId := unitParent(add, p, records, addedMachines, servicePlacedUnits, getSeries(application, defaultSeries))
			// Retrieve and modify the original "addUnit" change to add the
			// new parent requirement and placement target.
			change := records[fmt.Sprintf("%s/%d", name, i)]
			change.requires = append(change.requires, parentId)
			change.Params.To = "$" + parentId
		}
	}
}

func unitParent(add func(Change), p string, records map[string]*AddUnitChange, addedMachines map[string]string, servicePlacedUnits map[string]int, series string) (parentId string) {
	placement, err := charm.ParsePlacement(p)
	if err != nil {
		// Since the bundle is already verified, this should never happen.
		panic(err)
	}
	if placement.Machine == "new" {
		// The unit is placed to a new machine.
		change := newAddMachineChange(AddMachineParams{
			ContainerType: placement.ContainerType,
			Series:        series,
		})
		add(change)
		return change.Id()
	}
	if placement.Machine != "" {
		// The unit is placed to a machine declared in the bundle.
		parentId = addedMachines[placement.Machine]
		if placement.ContainerType != "" {
			parentId = addContainer(add, placement.ContainerType, parentId, series)
		}
		return parentId
	}
	// The unit is placed to another unit or to an application.
	number := placement.Unit
	if number == -1 {
		// The unit is placed to an application. Calculate the unit number to be
		// used for unit co-location.
		if n, ok := servicePlacedUnits[placement.Application]; ok {
			number = n + 1
		} else {
			number = 0
		}
		servicePlacedUnits[placement.Application] = number
	}
	otherUnit := fmt.Sprintf("%s/%d", placement.Application, number)
	parentId = records[otherUnit].Id()
	if placement.ContainerType != "" {
		parentId = addContainer(add, placement.ContainerType, parentId, series)
	}
	return parentId
}

func addContainer(add func(Change), containerType, parentId string, series string) string {
	change := newAddMachineChange(AddMachineParams{
		ContainerType: containerType,
		ParentId:      "$" + parentId,
		Series:        series,
	}, parentId)
	add(change)
	return change.Id()
}

// getSeries retrieves the series of a application from the ApplicationSpec or from the
// charm path or URL if provided, otherwise falling back on a default series.
func getSeries(application *charm.ApplicationSpec, defaultSeries string) string {
	if application.Series != "" {
		return application.Series
	}
	// We may have a local charm path.
	_, curl, err := charmrepo.NewCharmAtPath(application.Charm, "")
	if charm.IsMissingSeriesError(err) {
		// local charm path is valid but the charm doesn't declare a default series.
		return defaultSeries
	}
	if err == nil {
		// Return the default series from the local charm.
		return curl.Series
	}
	// The following is safe because the bundle data is assumed to be already
	// verified, and therefore this must be a valid charm URL.
	series := charm.MustParseURL(application.Charm).Series
	if series != "" {
		return series
	}
	return defaultSeries
}

// parseEndpoint creates an endpoint from its string representation.
func parseEndpoint(e string) *endpoint {
	parts := strings.SplitN(e, ":", 2)
	ep := &endpoint{
		application: parts[0],
	}
	if len(parts) == 2 {
		ep.relation = parts[1]
	}
	return ep
}

// endpoint holds a relation endpoint.
type endpoint struct {
	application string
	relation    string
}

// String returns the string representation of an endpoint.
func (ep endpoint) String() string {
	if ep.relation == "" {
		return ep.application
	}
	return fmt.Sprintf("%s:%s", ep.application, ep.relation)
}
