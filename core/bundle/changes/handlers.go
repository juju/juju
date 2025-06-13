// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bundlechanges

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/charm/v8"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/naturalsort"

	corecharm "github.com/juju/juju/core/charm"
)

type resolver struct {
	bundle           *charm.BundleData
	model            *Model
	bundleURL        string
	logger           Logger
	constraintGetter ConstraintGetter
	charmResolver    CharmResolver
	changes          *changeset
	force            bool
}

// handleApplications populates the change set with "addCharm"/"addApplication" records.
// This function also handles adding application annotations.
func (r *resolver) handleApplications() (map[string]string, error) {
	add := r.changes.add
	applications := r.bundle.Applications
	defaultSeries := r.bundle.Series
	existing := r.model

	charms := make(map[string]string, len(applications))
	addedApplications := make(map[string]string, len(applications))
	// Iterate over the map using its sorted keys so that results are
	// deterministic and easier to test.
	names := make([]string, 0, len(applications))
	for name := range applications {
		names = append(names, name)
	}
	naturalsort.Sort(names)
	var change Change
	for _, name := range names {
		application := applications[name]
		existingApp := existing.GetApplication(name)
		series, err := getSeries(application, defaultSeries)
		if err != nil {
			return nil, errors.Trace(err)
		}

		// Only parse the architecture constraint once and only if we give
		// a constraint getter function.
		var arch string
		if r.constraintGetter != nil {
			cons := r.constraintGetter(application.Constraints)
			arch, err = cons.Arch()
			if err != nil {
				return nil, errors.Trace(err)
			}
		}

		// For compatibility with pre juju 2.9 controllers, and to facilitate
		// matching existing charms in this method, assume a channel of stable
		// if not specified.  Required because another change ensures that the
		// cs applications have a channel provided an old controller.
		//
		// However, if we are deploying a bundle with local charms they
		// will always get an empty channel. For such cases, we should
		// never force a "stable" channel as deploying the bundle more
		// than once will fail (see LP1954933).
		channel := application.Channel
		if existingApp == nil || !strings.HasPrefix(existingApp.Charm, "local:") {
			if channel == "" {
				channel = "stable"
			}
		}

		revision := -1
		if application.Revision != nil {
			revision = *application.Revision
		} else {
			// The case of upgrade charmhub charm with by channel... need the correct revision,
			// or we will not have an addCharmChange corresponding to the upgradeCharmChange.
			if r.charmResolver != nil {
				_, rev, err := r.charmResolver(application.Charm, application.Series, channel, arch, revision)
				if err != nil {
					return nil, errors.Trace(err)
				}
				revision = rev
			}
		}

		// Add the addCharm record if one hasn't been added yet, this means
		// if the arch and series differ from an existing charm, then we create
		// a new charm.
		key := applicationKey(application.Charm, arch, series, channel, revision)
		if charms[key] == "" && !existing.matchesCharmPermutation(application.Charm, arch, series, channel, revision, r.constraintGetter) {
			change = newAddCharmChange(AddCharmParams{
				Charm:        application.Charm,
				Revision:     application.Revision,
				Series:       series,
				Channel:      application.Channel,
				Architecture: arch,
			})
			add(change)
			charms[key] = change.Id()
		}

		resources := make(map[string]int)
		localResources := make(map[string]string)
		for resName, res := range application.Resources {
			switch v := res.(type) {
			case int:
				resources[resName] = v
			case string:
				localResources[resName] = v
			}
		}
		if len(resources) == 0 {
			resources = nil
		}
		if len(localResources) == 0 {
			localResources = nil
		}
		var id string
		if existingApp == nil {

			var requires []string
			charmOrChange := application.Charm
			if charmChange := charms[key]; charmChange != "" {
				requires = append(requires, charmChange)
				charmOrChange = placeholder(charmChange)
			}

			// For Kubernetes bundles, we include the scale (num units).
			numUnits := 0
			if r.bundle.Type == kubernetes {
				numUnits = application.NumUnits
			}
			// Add the addApplication record for this application.
			change = newAddApplicationChange(AddApplicationParams{
				Charm:            charmOrChange,
				Series:           series,
				Application:      name,
				NumUnits:         numUnits,
				Options:          application.Options,
				Constraints:      application.Constraints,
				Storage:          application.Storage,
				Devices:          application.Devices,
				EndpointBindings: application.EndpointBindings,
				Resources:        resources,
				LocalResources:   localResources,
				charmURL:         application.Charm,
				Channel:          application.Channel,
			}, requires...)
			add(change)
			id = change.Id()
			addedApplications[name] = id

			// Expose the application if required.
			if application.Expose || len(application.ExposedEndpoints) != 0 {
				add(newExposeChange(ExposeParams{
					Application:      placeholder(id),
					ExposedEndpoints: mapExposedEndpointSpec(application.ExposedEndpoints),
					appName:          name,
				}, id))
			}
		} else {
			// Look for changes.
			if ok, err := r.allowCharmUpgrade(existingApp, application, arch); err != nil {
				return nil, errors.Trace(err)
			} else if ok {
				charmOrChange := application.Charm
				if charmChange := charms[key]; charmChange != "" {
					charmOrChange = placeholder(charmChange)
				}

				change = newUpgradeCharm(UpgradeCharmParams{
					Charm:          charmOrChange,
					Application:    name,
					Series:         series,
					Channel:        application.Channel,
					Resources:      resources,
					LocalResources: localResources,
					charmURL:       application.Charm,
				})
				add(change)
			}

			if changes := existingApp.changedOptions(application.Options); len(changes) > 0 {
				change = newSetOptionsChange(SetOptionsParams{
					Application: name,
					Options:     changes,
				})
				add(change)
			}

			if existing.ConstraintsEqual != nil && !existing.ConstraintsEqual(existingApp.Constraints, application.Constraints) {
				change = newSetConstraintsChange(SetConstraintsParams{
					Application: name,
					Constraints: application.Constraints,
				})
				add(change)
			}

			// We never do the negative. We will expose if necessary, but
			// never unexpose.
			if application.Expose || len(application.ExposedEndpoints) != 0 {
				// We emit a change if the app is not exposed
				// OR the app is already exposed but the
				// current expose endpoint params do not match
				// the incoming params.
				if !existingApp.Exposed || (existingApp.Exposed && !equalExposeParams(existingApp, application)) {
					add(newExposeChange(ExposeParams{
						Application:      name,
						ExposedEndpoints: mapExposedEndpointSpec(application.ExposedEndpoints),
						appName:          name,
						alreadyExposed:   existingApp.Exposed,
					}))
				}
			}

			if r.bundle.Type == kubernetes && existingApp.Scale != application.NumUnits {
				add(newScaleChange(ScaleParams{
					Application: name,
					appName:     name,
					Scale:       application.NumUnits,
				}))
			}
		}

		if r.bundleURL != "" {
			if application.Annotations == nil {
				application.Annotations = make(map[string]string)
			}
			// If a bundleURL already exists on a model and you're applying a new
			// bundle that overrides it, we will take the new bundleURL as you're now
			// making those existing applications to match the new bundle spec.
			application.Annotations["bundleURL"] = r.bundleURL
		}

		// Add application annotations.
		if annotations := existingApp.changedAnnotations(application.Annotations); len(annotations) > 0 {
			var (
				deps    []string
				paramID = name
			)
			if existingApp == nil {
				paramID = placeholder(id)
				deps = append(deps, id)
			}
			add(newSetAnnotationsChange(SetAnnotationsParams{
				EntityType:  ApplicationType,
				Id:          paramID,
				Annotations: annotations,
				target:      name,
			}, deps...))
		}
	}
	return addedApplications, nil
}

func (r *resolver) allowCharmUpgrade(existingApp *Application, bundleApp *charm.ApplicationSpec, bundleArch string) (bool, error) {
	// This covers most of v1 charm URL changes, everything else below is to
	// support channels. Charmstore charms allow channels, but bundles were not
	// aware of them, with the introduction of Charmhub charms, then we do need
	// to factor in channels.
	if existingApp.Charm != bundleApp.Charm {
		return true, nil
	}
	// No existing revision found, so assume no upgrades are available.
	if existingApp.Revision == -1 {
		return false, nil
	}
	// This handles the case that the existing application doesn't have a
	// channel, so we're talking to an older controller.
	if existingApp.Channel == "" {
		// No upgrade required.
		if bundleApp.Channel == "" {
			return false, nil
		}
		// If the bundle channel is not empty and we're not using force, then
		// raise an error asking the user to supply force.
		if !r.force && bundleApp.Channel != "" {
			return false, errors.Errorf("upgrades not supported when the channel for %q is unknown; use --force to override", existingApp.Name)
		}
		return true, nil
	}

	var (
		resolvedChan = bundleApp.Channel
		resolvedRev  = -1
	)
	if r.charmResolver != nil {
		rev := -1
		if bundleApp.Revision != nil {
			rev = *bundleApp.Revision
		}
		var err error
		resolvedChan, resolvedRev, err = r.charmResolver(bundleApp.Charm, bundleApp.Series, bundleApp.Channel, bundleArch, rev)
		if err != nil {
			return false, errors.Trace(err)
		}
	}
	if !r.force && existingApp.Channel != resolvedChan {
		verb := "requested"
		if bundleApp.Channel == "" {
			verb = "resolved"
		}
		return false, errors.Errorf("application %q: upgrades not supported across channels (existing: %q, %s: %q); use --force to override", existingApp.Name, existingApp.Channel, verb, resolvedChan)
	}

	// The revision number is in the origin for a charmhub charm and in the url for a charmstore charm.
	if resolvedRev > existingApp.Revision {
		return true, nil
	}
	if resolvedRev != -1 && resolvedRev < existingApp.Revision {
		// For charmhub charms, we currently don't support downgrades.
		return false, errors.Errorf("application %q: downgrades are not currently supported: deployed revision %v is newer than requested revision %v", existingApp.Name, existingApp.Revision, resolvedRev)
	}
	// Same revision, no upgrade required.
	return false, nil
}

func mapExposedEndpointSpec(specs map[string]charm.ExposedEndpointSpec) map[string]*ExposedEndpointParams {
	if len(specs) == 0 {
		return nil
	}

	out := make(map[string]*ExposedEndpointParams)
	for epName, spec := range specs {
		out[epName] = &ExposedEndpointParams{
			ExposeToSpaces: spec.ExposeToSpaces,
			ExposeToCIDRs:  spec.ExposeToCIDRs,
		}
	}
	return out
}

func equalExposeParams(cur *Application, incoming *charm.ApplicationSpec) bool {
	if len(cur.ExposedEndpoints) != len(incoming.ExposedEndpoints) {
		return false
	}

	for epName, expDetails := range cur.ExposedEndpoints {
		incDetails, found := incoming.ExposedEndpoints[epName]
		if !found {
			return false
		}

		if !equalStringSlice(expDetails.ExposeToSpaces, incDetails.ExposeToSpaces) ||
			!equalStringSlice(expDetails.ExposeToCIDRs, incDetails.ExposeToCIDRs) {
			return false
		}
	}

	return true
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	sort.Strings(a)
	sort.Strings(b)

	for i, aEntry := range a {
		if aEntry != b[i] {
			return false
		}
	}

	return true
}

// handleMachines populates the change set with "addMachines" records.
// This function also handles adding machine annotations.
func (r *resolver) handleMachines() map[string]*AddMachineChange {
	add := r.changes.add
	machines := r.bundle.Machines
	defaultSeries := r.bundle.Series
	existing := r.model

	addedMachines := make(map[string]*AddMachineChange, len(machines))
	// Iterate over the map using its sorted keys so that results are
	// deterministic and easier to test.
	names := make([]string, 0, len(machines))
	for name := range machines {
		names = append(names, name)
	}
	naturalsort.Sort(names)
	for _, name := range names {
		machine := machines[name]
		if machine == nil {
			machine = &charm.MachineSpec{}
		}
		series := machine.Series
		if series == "" {
			series = defaultSeries
		}

		var id string
		var target string
		var requires []string

		existingMachine := existing.BundleMachine(name)
		if existingMachine == nil {
			// Add the addMachines record for this machine.
			machineID := existing.nextMachine()
			change := newAddMachineChange(AddMachineParams{
				Series:          series,
				Constraints:     machine.Constraints,
				machineID:       machineID,
				bundleMachineID: name,
			})
			add(change)
			addedMachines[name] = change
			id = placeholder(change.Id())
			target = "new machine " + machineID
			requires = append(requires, change.Id())
		} else {
			id = existingMachine.ID
			target = "existing machine " + existingMachine.ID
		}
		// Worth noting that changedAnnotations is perfectly fine being
		// called on a nil machine.
		changed := existingMachine.changedAnnotations(machine.Annotations)
		// Add machine annotations.
		if len(changed) > 0 {
			add(newSetAnnotationsChange(SetAnnotationsParams{
				EntityType:  MachineType,
				Id:          id,
				Annotations: changed,
				target:      target,
			}, requires...))
		}
	}
	return addedMachines
}

// handleRelations populates the change set with "addRelation" records.
func (r *resolver) handleRelations(addedApplications map[string]string) {
	add := r.changes.add
	relations := r.bundle.Relations
	existing := r.model

	for _, relation := range relations {
		// Add the addRelation record for this relation pair.
		var requires []string
		// For every relation we have three possible situations:
		// 1) The relation is for an application we haven't yet deployed, so it won't
		// yet exist, and one or more of the endpoints are placeholders.
		// 2) The applications exist but the relation doesn't. So both of the applications
		// refer to existing applications.
		// 3) The relation already exists, so nothing to change.

		ep1 := parseEndpoint(relation[0])
		ep2 := parseEndpoint(relation[1])
		if existing.HasRelation(ep1.application, ep1.relation, ep2.application, ep2.relation) {
			continue
		}

		getEndpointNames := func(ep *endpoint) (string, string) {
			// If the application exists, then we don't require it, and the param
			// is the endpoint string not a placeholder.
			nice := ep.String()
			if app := existing.GetApplication(ep.application); app != nil {
				return nice, nice
			}
			pendingApp := addedApplications[ep.application]
			ep.application = pendingApp
			requires = append(requires, pendingApp)
			return placeholder(ep.String()), nice
		}

		// We need to get the args first as they mutate the requires slice.
		arg0, nice0 := getEndpointNames(ep1)
		arg1, nice1 := getEndpointNames(ep2)

		add(newAddRelationChange(AddRelationParams{
			Endpoint1:            arg0,
			Endpoint2:            arg1,
			applicationEndpoint1: nice0,
			applicationEndpoint2: nice1,
		}, requires...))
	}
}

// handleOffers populates the change set with "CreateOffer" records.
// ensure we pass back handle offers so that if the map resizes we don't lose
// applications
func (r *resolver) handleOffers(addedApplications map[string]string, deployedBundleApplications set.Strings, existingModelOffers map[string]set.Strings) (map[string]string, error) {
	// the bundle should have been verified before calling handling of types
	// as saas applications will stamp on existing applications with the same
	// name.
	for name, saasSpec := range r.bundle.Saas {
		change := newConsumeOfferChange(ConsumeOfferParams{
			URL:             saasSpec.URL,
			ApplicationName: name,
		})
		r.changes.add(change)
		addedApplications[name] = change.Id()
	}

	for appName, appSpec := range r.bundle.Applications {
		for offerName, offerSpec := range appSpec.Offers {
			// To create an offer, the application must be deployed
			// as part of this changeset or (if we are abusing
			// bundle deployments to do upgrades) already deployed
			// to the controller. In the latter case, we insist
			// that the application MUST be present both in the
			// bundle and the controller.
			//
			// NOTE: Bundle validation rules prevent you from
			// creating offers without a fully valid application
			// block in the bundle but let's be extra paranoid just
			// in case these rules are relaxed in the future.
			var reqs []string
			if changeID := addedApplications[appName]; changeID != "" {
				reqs = append(reqs, changeID)
			} else if !deployedBundleApplications.Contains(appName) {
				return nil, errors.NotFoundf("cannot create offer %s: application %s", offerName, appName)
			}

			// If we are updating the offer details and the offered
			// application is already deployed, we don't have any
			// requirements.
			updateOffer := existingModelOffers[appName] != nil && existingModelOffers[appName].Contains(offerName)
			if updateOffer {
				reqs = nil
			}

			change := newCreateOfferChange(CreateOfferParams{
				Application: appName,
				Endpoints:   offerSpec.Endpoints,
				OfferName:   offerName,
				Update:      updateOffer,
			}, reqs...)
			r.changes.add(change)

			for user, access := range offerSpec.ACL {
				r.changes.add(newGrantOfferAccessChange(GrantOfferAccessParams{
					User:   user,
					Access: access,
					Offer:  offerName,
				}, change.Id()))
			}
		}
	}
	return addedApplications, nil
}

type unitProcessor struct {
	add           func(Change)
	existing      *Model
	bundle        *charm.BundleData
	defaultSeries string
	logger        Logger

	// The added applications and machines are maps from names to
	// change IDs.
	addedApplications map[string]string
	addedMachines     map[string]*AddMachineChange

	// Sorted keys from the applications map.
	appNames []string

	// addUnitChanges maps the given placeholder unit name for the change that
	// was created to add the unit. This mapping added during the first phase
	// of the units where all new units are added, and used in the placement
	// phase to get the underlying change to annotate with the placement
	// details. The are also used in determining the underlying base machine
	// for other units where the placement directive mentions a unit or
	// application.
	addUnitChanges map[string]*AddUnitChange

	// appChanges holds all the new unit changes for a given application name.
	// These are used during placement when the placement specifies another
	// application rather than a unit of the application.
	appChanges map[string][]*AddUnitChange

	// existingMachinesWithoutApp is populated as needed by data from the
	// existing Model. The key is a pair of application names, and the value
	// is a list of machine IDs where the first application is on the machine
	// and the second application isn't.
	existingMachinesWithoutApp map[string][]string

	// newUnitsWithoutApp is populated as needed during the processing of
	// placing units next to an application. The key is the same as for
	// existingMachinesWithoutApp and the map is used for the same purpose.
	// When first added, the value is the new units for the second
	// application. The values are consumed from the slice as placements are
	// processed.
	newUnitsWithoutApp map[string][]*AddUnitChange
}

func (p *unitProcessor) unitPlaceholder(appName string, n int) string {
	return fmt.Sprintf("%s/%s", appName, placeholder(fmt.Sprint(n)))
}

func (p *unitProcessor) addAllNeededUnits() {
	// Collect and add all unit changes. These records are likely to be
	// modified later in order to handle unit placement.
	for _, name := range p.appNames {
		application := p.bundle.Applications[name]
		existingApp := p.existing.GetApplication(name)
		for i := existingApp.unitCount(); i < application.NumUnits; i++ {
			var requires []string
			changeApplication := name
			if existingApp == nil {
				appChangeID := p.addedApplications[name]
				requires = append(requires, appChangeID)
				changeApplication = placeholder(appChangeID)
			}
			unitName := p.existing.nextUnit(name)
			change := newAddUnitChange(AddUnitParams{
				Application: changeApplication,
				unitName:    unitName,
			}, requires...)
			p.add(change)
			p.addUnitChanges[p.unitPlaceholder(name, i)] = change
			p.appChanges[name] = append(p.appChanges[name], change)
		}
	}
}

func (p *unitProcessor) placementDependencies(app *charm.ApplicationSpec) set.Strings {
	result := set.NewStrings()
	for _, value := range app.To {
		placement, _ := charm.ParsePlacement(value)
		result.Add(placement.Application)
	}
	// Simplify the above loop by not caring if the application isn't set, and
	// just remove it at the end.
	result.Remove("")
	return result
}

func (p *unitProcessor) processUnitPlacement() error {
	processed := set.NewStrings()
	toDo := set.NewStrings(p.appNames...)

	// The processing of units is none using successive passes where all
	// applications mentioned in the current application's placement
	// directives must have already been done. If for any given cycle through
	// the loop done is zero, then there must be cycles in the remaining
	// placement directives and an error is returned.
	for !toDo.IsEmpty() {
		done := 0
		sortedNames := toDo.SortedValues()

		// Now handle unit placement for each added application unit.
		for _, name := range sortedNames {
			application := p.bundle.Applications[name]
			deps := p.placementDependencies(application)
			if notDoneYet := deps.Difference(processed); !notDoneYet.IsEmpty() {
				// This application depends on something we haven't yet processed
				// so try again next time through the outer loop.
				continue
			}
			err := p.placeUnitsForApplication(name, application)
			if err != nil {
				return err
			}
			processed.Add(name)
			toDo.Remove(name)
			done++
		}

		// If we haven't done any then we have a cycle
		if done == 0 {
			return errors.Errorf("cycle in placement directives for: %s", strings.Join(toDo.SortedValues(), ", "))
		}
	}
	return nil
}

func (p *unitProcessor) placeUnitsForApplication(name string, application *charm.ApplicationSpec) error {
	existingApp := p.existing.GetApplication(name)

	lastPlacement := ""
	numPlaced := len(application.To)
	if numPlaced > 0 {
		// At this point we know that we have at least one placement directive.
		// Fill the other ones if required.
		lastPlacement = application.To[numPlaced-1]
		// Only use the last placement if it specifies an application
		// (not a unit), or "new" for the machine.
		placement, _ := charm.ParsePlacement(lastPlacement)
		switch {
		case placement.Machine == "new":
			// This is fine.
		case placement.Application != "" && placement.Unit == -1:
			// This is also fine.
		default:
			// Default to empty placement, because targeting a
			// specific machine or specific unit for multiple placed
			// units doesn't really make sense.
			lastPlacement = ""
		}
	}

	p.logger.Tracef("model: %s", p.existing.pretty())
	p.logger.Tracef("placements: %v", application.To)
	unsatisfied := p.existing.unsatisfiedMachineAndUnitPlacements(name, application.To)
	p.logger.Tracef("unsatisfied: %v", unsatisfied)

	var lastChangeID string
	// unitCount on a nil existingApp returns zero.
	for i := existingApp.unitCount(); i < application.NumUnits; i++ {
		directive := lastPlacement
		if len(unsatisfied) > 0 {
			directive, unsatisfied = unsatisfied[0], unsatisfied[1:]
		}
		_ = unsatisfied

		p.logger.Tracef("directive: %q", directive)
		placement, err := p.getPlacementForNewUnit(name, application, directive)
		if err != nil {
			return err
		}
		// Retrieve and modify the original "addUnit" change to add the
		// new parent requirement and placement target.
		change := p.addUnitChanges[p.unitPlaceholder(name, i)]
		change.Params.placementDescription = placement.placementDescription
		change.Params.baseMachine = placement.baseMachine
		change.Params.To = placement.target
		change.Params.directive = placement.directive
		change.requires = append(change.requires, placement.requires...)

		if lastChangeID != "" {
			change.requires = append(change.requires, lastChangeID)
		}
		lastChangeID = change.id
	}
	return nil
}

// existingMachinePlacement generates the standard unitPlacement for a machine
// that already exists in the model. If container is not empty, then this
// indicates that the placement is for a container on the machine.
func (p *unitProcessor) existingMachinePlacement(machineID, container string) unitPlacement {
	toMachine := machineID
	description := "existing machine " + machineID
	if container != "" {
		toMachine = container + ":" + toMachine
		description = p.existing.nextContainer(machineID, container)
	}

	return unitPlacement{
		target:               toMachine,
		placementDescription: description,
		baseMachine:          machineID,
	}
}

// newMachineForUnit handles the placement directives "new" and
// "container:new", where container is a supported container type. Most often
// "lxd" or "kvm".
func (p *unitProcessor) newMachineForUnit(application *charm.ApplicationSpec, placement *charm.UnitPlacement) (unitPlacement, error) {
	return p.addNewMachine(application, placement.ContainerType)
}

// definedMachineForUnit handles the placement directives where an actual
// machine number is specified, perhaps with a container. The machine numbers
// mentioned must be in the bundles machines specification. Examples would be:
// "2", "lxd:1".
func (p *unitProcessor) definedMachineForUnit(application *charm.ApplicationSpec, placement *charm.UnitPlacement) (unitPlacement, error) {
	// See if we have the mapped machine in the existing model.
	machine := p.existing.BundleMachine(placement.Machine)
	var err error
	if machine == nil {
		// The unit is placed to a machine declared in the bundle.
		change := p.addedMachines[placement.Machine]
		result := unitPlacement{
			target:               placeholder(change.Id()),
			requires:             []string{change.Id()},
			placementDescription: "new machine " + change.Params.machineID,
			baseMachine:          change.Params.machineID,
		}
		if placement.ContainerType != "" {
			result, err = p.addContainer(result, application, placement.ContainerType)
		}
		return result, err
	}
	// Placement is the machine, or a container on that machine.
	return p.existingMachinePlacement(machine.ID, placement.ContainerType), err
}

// definedUnitForUnit handles the placement directive where a unit is to be
// co-located with another unit of a different application. Examples would be
// "foo/3" or "lxd:foo/2". If the placement specifies a container then the
// container is placed on the same base machine as the other unit. This means
// that if the target unit is also in a container, the containers become
// siblings, not nested.
func (p *unitProcessor) definedUnitForUnit(application *charm.ApplicationSpec, placement *charm.UnitPlacement, directive string) (unitPlacement, error) {
	// If the placement refers to a Unit, see if there is a unit for the app
	// in the existing model that exists.
	setDirective := func(result unitPlacement) unitPlacement {
		result.directive = directive
		return result
	}

	machineID := p.existing.getUnitMachine(placement.Application, placement.Unit)
	if machineID != "" {
		// Placement is the machine, or a container on that machine.
		return setDirective(p.existingMachinePlacement(machineID, placement.ContainerType)), nil
	}

	// The specified unit number doesn't relate to a known existing unit, so see if
	// it matches a unit we are adding.
	otherUnit := p.unitPlaceholder(placement.Application, placement.Unit)
	otherChange := p.addUnitChanges[otherUnit]
	if otherChange == nil {
		// There is clearly a wierdness in the to declarations, so fall back to a new machine.
		return p.newMachineForUnit(application, placement)
	}

	result, err := p.newUnitPlacementForChange(otherChange, application, placement.ContainerType)
	return setDirective(result), err
}

func (p *unitProcessor) nextMachineForExistingAppUnits(appName string, placement *charm.UnitPlacement) string {
	key := appName + "/" + placement.Application
	machines := p.existingMachinesWithoutApp[key]
	if machines == nil {
		// We only get this once per key as once it is non-nil, we don't ask again.
		machines = p.existing.unitMachinesWithoutApp(placement.Application, appName, placement.ContainerType)
		p.existingMachinesWithoutApp[key] = machines
	}
	if len(machines) == 0 {
		return ""
	}
	result, machines := machines[0], machines[1:]
	p.existingMachinesWithoutApp[key] = machines
	return result
}

func (p *unitProcessor) nextUnitChangeForApp(appName string, placement *charm.UnitPlacement) *AddUnitChange {
	key := appName + "/" + placement.Application
	changes := p.newUnitsWithoutApp[key]
	if changes == nil {
		newUnits := p.appChanges[placement.Application]
		if newUnits == nil {
			changes = []*AddUnitChange{}
		} else {
			// Copy the slice for our purposes as we are going to consume the
			// resulting slice.
			changes = newUnits[:]
		}
		p.newUnitsWithoutApp[key] = changes
	}
	if len(changes) == 0 {
		return nil
	}
	result, changes := changes[0], changes[1:]
	p.newUnitsWithoutApp[key] = changes
	return result
}

func (p *unitProcessor) newUnitPlacementForChange(change *AddUnitChange, application *charm.ApplicationSpec, containerType string) (unitPlacement, error) {
	var err error
	baseMachine := change.Params.baseMachine
	// Here we need to do some magic. If the new unit is being placed into a container
	// then the container should be a sibling to the change, otherwise we need it
	// to be placed in the same machine as the change.
	result := unitPlacement{
		target:               placeholder(change.Id()),
		baseMachine:          baseMachine,
		placementDescription: change.Params.placementDescription,
		requires:             []string{change.Id()},
	}

	// It would be nice if we could be smarter with the creation of containers.
	// Need to check with the GUI folks about removing container additions, and
	// instead just handling it in unit placement.
	if containerType != "" {
		result, err = p.addContainer(result, application, containerType)
	}

	return result, err
}

func (p *unitProcessor) definedApplicationForUnit(appName string, application *charm.ApplicationSpec, placement *charm.UnitPlacement, directive string) (unitPlacement, error) {
	setDirective := func(result unitPlacement) unitPlacement {
		result.directive = directive
		return result
	}
	// First see if there is a unit of the placement application that doesn't
	// have a unit of the application we are trying to place next to it (or in
	// a container as defined by the placement).
	existingMachine := p.nextMachineForExistingAppUnits(appName, placement)
	if existingMachine != "" {
		return setDirective(p.existingMachinePlacement(existingMachine, placement.ContainerType)), nil
	}
	// If there are none in the model, look for units of appName that have been placed.
	change := p.nextUnitChangeForApp(appName, placement)
	if change != nil {
		result, err := p.newUnitPlacementForChange(change, application, placement.ContainerType)
		return setDirective(result), err
	}

	return unitPlacement{
		baseMachine: p.existing.nextMachine(),
	}, nil
}

type unitPlacement struct {
	// The target is the placement directive for the unit to be deployed.
	// The difference here is that the machine number may instead refer to
	// the change id for the add machine change that creates a machines.
	// Examples would be: "4", "lxd:4", "", "lxd:", "$addMachine-14".
	target string

	// baseMachine refers to the top level machine for this unit. This is used
	// for the placement description of other units when they are co-located
	// with this new unit. The baseMachine is used to generate the container
	// identifier for new containers.
	baseMachine string

	// requires additional changes to be applied prior to this unit change.
	requires []string

	// This is the description shown for the add unit change.
	placementDescription string
	// If directive is specified, it is added to the placement description
	// to explain why the unit is being placed there.
	directive string
}

func (p *unitProcessor) getPlacementForNewUnit(appName string, application *charm.ApplicationSpec, directive string) (unitPlacement, error) {
	if directive == "" {
		// There is no specified directive for this unit, so it gets a new machine.
		return unitPlacement{
			baseMachine: p.existing.nextMachine(),
		}, nil
	}

	placement, err := charm.ParsePlacement(directive)
	if err != nil {
		// Since the bundle is already verified, this should never happen.
		return unitPlacement{}, nil
	}

	if placement.Machine == "new" {
		return p.newMachineForUnit(application, placement)
	}

	if placement.Machine != "" {
		return p.definedMachineForUnit(application, placement)
	}

	if placement.Unit >= 0 {
		return p.definedUnitForUnit(application, placement, directive)
	}

	return p.definedApplicationForUnit(appName, application, placement, directive)
}

func (p *unitProcessor) addNewMachine(application *charm.ApplicationSpec, containerType string) (unitPlacement, error) {
	machineID := p.existing.nextMachine()
	description := "new machine " + machineID
	placeholderContainer := ""
	if containerType != "" {
		placeholderContainer = p.existing.nextContainer(machineID, containerType)
		description = placeholderContainer
	}
	constraints, err := fixupConstraintsWithBindings(application.Constraints, application.EndpointBindings)
	if err != nil {
		return unitPlacement{}, err
	}
	series, err := getSeries(application, p.defaultSeries)
	if err != nil {
		return unitPlacement{}, err
	}
	change := newAddMachineChange(AddMachineParams{
		ContainerType:      containerType,
		Series:             series,
		Constraints:        constraints,
		machineID:          machineID,
		containerMachineID: placeholderContainer,
	})
	p.add(change)
	return unitPlacement{
		target:               placeholder(change.Id()),
		requires:             []string{change.Id()},
		baseMachine:          machineID,
		placementDescription: description,
	}, nil
}

// fixupConstraintsWithBindings returns constraints with
// added spaces constraints for bound endpoints.
func fixupConstraintsWithBindings(inputConstraints string, endpointBindings map[string]string) (string, error) {
	posSpaces := make(map[string]bool)
	negSpaces := make(map[string]bool)
	for _, space := range endpointBindings {
		posSpaces[space] = true
	}

	if len(posSpaces) == 0 {
		return inputConstraints, nil
	}

	constraintsMap := make(map[string]string)
	var constraintsKeyList []string
	if len(inputConstraints) > 0 {
		constraints := strings.Split(inputConstraints, " ")
		for _, constraint := range constraints {
			split := strings.SplitN(constraint, "=", 2)
			if len(split) != 2 {
				return "", fmt.Errorf("invalid constraint: %q %q %d", constraint, inputConstraints, len(constraints))
			}
			key, value := split[0], split[1]
			constraintsMap[key] = value
			if key != "spaces" {
				constraintsKeyList = append(constraintsKeyList, key)
			}
		}
	}

	var spaces []string
	if spacesToSplit := constraintsMap["spaces"]; len(spacesToSplit) > 0 {
		spaces = strings.Split(spacesToSplit, ",")
	}

	for _, space := range spaces {
		if strings.HasPrefix(space, "^") {
			negSpaces[space[1:]] = true
			if posSpaces[space[1:]] {
				return "", fmt.Errorf("space %q is required but it's forbidden by constraint", space[1:])
			}
		} else {
			posSpaces[space] = true
		}
	}
	var outputSpaces []string
	for k := range posSpaces {
		outputSpaces = append(outputSpaces, k)
	}
	for k := range negSpaces {
		outputSpaces = append(outputSpaces, "^"+k)
	}
	// To make test tests stable.
	naturalsort.Sort(outputSpaces)
	naturalsort.Sort(constraintsKeyList)
	output := "spaces=" + strings.Join(outputSpaces, ",")
	for _, constraint := range constraintsKeyList {
		output += " " + constraint + "=" + constraintsMap[constraint]
	}
	return output, nil
}

func (p *unitProcessor) addContainer(up unitPlacement, application *charm.ApplicationSpec, containerType string) (unitPlacement, error) {
	placeholderContainer := p.existing.nextContainer(up.baseMachine, containerType)
	_, existing := p.existing.Machines[up.baseMachine]
	description := placeholderContainer

	constraints, err := fixupConstraintsWithBindings(application.Constraints, application.EndpointBindings)
	if err != nil {
		return unitPlacement{}, err
	}
	series, err := getSeries(application, p.defaultSeries)
	if err != nil {
		return unitPlacement{}, err
	}
	params := AddMachineParams{
		ContainerType:      containerType,
		ParentId:           up.target,
		Series:             series,
		Constraints:        constraints,
		existing:           existing,
		machineID:          up.baseMachine,
		containerMachineID: placeholderContainer,
	}
	change := newAddMachineChange(params, up.requires...)
	p.add(change)
	return unitPlacement{
		target:               placeholder(change.Id()),
		requires:             []string{change.Id()},
		placementDescription: description,
		baseMachine:          up.baseMachine, // The underlying base machine stays the same.
	}, nil
}

// handleUnits populates the change set with "addUnit" records.
// It also handles adding machine containers where to place units if required.
func (r *resolver) handleUnits(addedApplications map[string]string, addedMachines map[string]*AddMachineChange) error {

	// Iterate over the map using its sorted keys so that results are
	// deterministic and easier to test.
	names := make([]string, 0, len(r.bundle.Applications))
	for name := range r.bundle.Applications {
		names = append(names, name)
	}
	naturalsort.Sort(names)

	processor := &unitProcessor{
		add:                        r.changes.add,
		existing:                   r.model,
		bundle:                     r.bundle,
		defaultSeries:              r.bundle.Series,
		logger:                     r.logger,
		addedApplications:          addedApplications,
		addedMachines:              addedMachines,
		appNames:                   names,
		addUnitChanges:             make(map[string]*AddUnitChange),
		appChanges:                 make(map[string][]*AddUnitChange),
		existingMachinesWithoutApp: make(map[string][]string),
		newUnitsWithoutApp:         make(map[string][]*AddUnitChange),
	}

	processor.addAllNeededUnits()
	err := processor.processUnitPlacement()
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(r.checkAllMachinesUsed(addedMachines))
}

// checkAllMachinesUsed ensures that we haven't added any machines
// that don't host any units or containers - this indicates an
// inconsistency between the bundle and the machine map to the
// existing model.
func (r *resolver) checkAllMachinesUsed(addedMachines map[string]*AddMachineChange) error {
	dependents := r.changes.dependents()
	for _, change := range addedMachines {
		if len(dependents[change.Id()]) == 0 {
			return r.newMachineMapError(change)
		}
	}
	return nil
}

func (r *resolver) newMachineMapError(machineChange *AddMachineChange) *InconsistentMachineMapError {
	// The machine map needs some extra entry for this machine, the
	// possible target machines are the ones in the model that aren't
	// already referred to from the bundle (via the machine map).
	modelMachines := set.NewStrings()
	for machineID := range r.model.Machines {
		modelMachines.Add(machineID)
	}
	for bundleMachineID := range r.bundle.Machines {
		modelMachineID, found := r.model.MachineMap[bundleMachineID]
		if !found {
			continue
		}
		modelMachines.Remove(modelMachineID)
	}
	targetIDs := naturalsort.Sort(modelMachines.Values())

	var applications []string
	for name, application := range r.bundle.Applications {
		for _, to := range application.To {
			if to == machineChange.Params.bundleMachineID {
				applications = append(applications, name)
			}
		}
	}

	return &InconsistentMachineMapError{
		BundleMachine:     machineChange.Params.bundleMachineID,
		PossibleTargetIDs: targetIDs,
		Applications:      naturalsort.Sort(applications),
	}
}

// InconsistentMachineMapError indicates that there is a mismatch
// between the bundle machines and the machine map which should be
// resolved with explicit entries.
type InconsistentMachineMapError struct {
	BundleMachine     string
	PossibleTargetIDs []string
	Applications      []string
}

func (err *InconsistentMachineMapError) Error() string {
	quotedIDs := make([]string, len(err.PossibleTargetIDs))
	for i, id := range err.PossibleTargetIDs {
		quotedIDs[i] = fmt.Sprintf("%q", id)
	}
	var modelMachines string
	if len(quotedIDs) == 1 {
		modelMachines = fmt.Sprintf(", perhaps to unreferenced model machine %s", quotedIDs[0])
	} else if len(quotedIDs) > 1 {
		modelMachines = fmt.Sprintf(", perhaps to one of model machines [%s]", strings.Join(quotedIDs, ", "))
	}
	var applications string
	if len(err.Applications) != 0 {
		applications = fmt.Sprintf(" - the target should host [%s]",
			strings.Join(err.Applications, ", "))
	}
	return fmt.Sprintf(
		`bundle and machine mapping are inconsistent: need an explicit entry mapping bundle machine %q%s%s`,
		err.BundleMachine,
		modelMachines,
		applications,
	)
}

func placeholder(changeID string) string {
	return "$" + changeID
}

func applicationKey(charm, arch, series, channel string, revision int) string {
	return fmt.Sprintf("%s:%s:%s:%s:%d", charm, arch, series, channel, revision)
}

// getSeries retrieves the series of a application from the ApplicationSpec or from the
// charm path or URL if provided, otherwise falling back on a default series.
func getSeries(application *charm.ApplicationSpec, defaultSeries string) (string, error) {
	if application.Series != "" {
		return application.Series, nil
	}

	// Handle local charm paths.
	if charm.IsValidLocalCharmOrBundlePath(application.Charm) {
		_, charmURL, err := corecharm.NewCharmAtPath(application.Charm, defaultSeries)
		if charm.IsMissingSeriesError(err) {
			// local charm path is valid but the charm doesn't declare a default series.
			return defaultSeries, nil
		} else if charm.IsUnsupportedSeriesError(err) {
			// The bundle's default series is not supported by the charm, but we'll
			// use it anyway. This is no different to the case above where application.Series
			// is used without checking for potential charm incompatibility.
			return defaultSeries, nil
		} else if err != nil {
			return "", errors.Trace(err)
		}
		// Return the default series from the local charm.
		return charmURL.Series, nil
	}

	// The following is safe because the bundle data is assumed to be already
	// verified, and therefore this must be a valid charm URL.
	charmURL, err := charm.ParseURL(application.Charm)
	if err != nil {
		return "", errors.Trace(err)
	}
	series := charmURL.Series
	if series != "" {
		return series, nil
	}
	return defaultSeries, nil
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
