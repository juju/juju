// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle

import (
	"reflect"
	"sort"
	"strings"

	"github.com/juju/charm/v10"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	corebase "github.com/juju/juju/core/base"
	bundlechanges "github.com/juju/juju/core/bundle/changes"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
)

// This file contains functionality required by both the application
// package and the application/deployer package.

// BuildModelRepresentation creates a buildchanges.Model, representing
// the existing deployment, to be used while deploying or diffing a bundle.
func BuildModelRepresentation(
	status *params.FullStatus,
	modelExtractor ModelExtractor,
	useExistingMachines bool,
	bundleMachines map[string]string,
) (*bundlechanges.Model, error) {
	var (
		annotationTags []string
		appNames       []string
		principalApps  []string
	)
	machineMap := make(map[string]string)
	machines := make(map[string]*bundlechanges.Machine)
	for id, machineStatus := range status.Machines {
		var (
			base corebase.Base
			err  error
		)
		if machineStatus.Base.Channel != "" {
			base, err = corebase.ParseBase(machineStatus.Base.Name, machineStatus.Base.Channel)
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		machines[id] = &bundlechanges.Machine{
			ID:   id,
			Base: base,
		}
		tag := names.NewMachineTag(id)
		annotationTags = append(annotationTags, tag.String())
		if useExistingMachines && tag.ContainerType() == "" {
			machineMap[id] = id
		}
	}

	offersByApplication := make(map[string][]string)
	for _, offer := range status.Offers {
		appOffers := offersByApplication[offer.ApplicationName]
		appOffers = append(appOffers, offer.OfferName)
		offersByApplication[offer.ApplicationName] = appOffers
	}

	// Now iterate over the bundleMachines that the user specified.
	for bundleMachine, modelMachine := range bundleMachines {
		machineMap[bundleMachine] = modelMachine
	}
	applications := make(map[string]*bundlechanges.Application)
	for name, appStatus := range status.Applications {
		curl, err := charm.ParseURL(appStatus.Charm)
		if err != nil {
			return nil, errors.Trace(err)
		}

		// CharmAlias is used to ensure that we use the name of the charm and
		// not the full path of the charm url, exposing the internal
		// representation of the charm URL.
		charmAlias := appStatus.Charm
		if charm.CharmHub.Matches(curl.Schema) {
			charmAlias = curl.Name
		}

		base, err := corebase.ParseBase(appStatus.Base.Name, appStatus.Base.Channel)
		if err != nil {
			return nil, errors.Trace(err)
		}
		app := &bundlechanges.Application{
			Name:          name,
			Charm:         charmAlias,
			Scale:         appStatus.Scale,
			Exposed:       appStatus.Exposed,
			Base:          base,
			Channel:       appStatus.CharmChannel,
			Revision:      curl.Revision,
			SubordinateTo: appStatus.SubordinateTo,
			Offers:        offersByApplication[name],
		}
		if len(appStatus.ExposedEndpoints) != 0 {
			app.ExposedEndpoints = make(map[string]bundlechanges.ExposedEndpoint)
			for endpoint, exposeDetails := range appStatus.ExposedEndpoints {
				app.ExposedEndpoints[endpoint] = bundlechanges.ExposedEndpoint{
					ExposeToSpaces: exposeDetails.ExposeToSpaces,
					ExposeToCIDRs:  exposeDetails.ExposeToCIDRs,
				}
			}
		}
		for unitName, unit := range appStatus.Units {
			app.Units = append(app.Units, bundlechanges.Unit{
				Name:    unitName,
				Machine: unit.Machine,
			})
		}
		applications[name] = app
		annotationTags = append(annotationTags, names.NewApplicationTag(name).String())
		appNames = append(appNames, name)
		if len(appStatus.Units) > 0 {
			// While this isn't entirely accurate, because an application
			// without any units is still a principal, it is less bad than
			// just using 'SubordinateTo' as a subordinate charm that isn't
			// related to anything has that empty too.
			principalApps = append(principalApps, name)
		}
	}
	mod := &bundlechanges.Model{
		Applications: applications,
		Machines:     machines,
		MachineMap:   machineMap,
	}
	for _, relation := range status.Relations {
		// All relations have two endpoints except peers.
		if len(relation.Endpoints) != 2 {
			continue
		}
		mod.Relations = append(mod.Relations, bundlechanges.Relation{
			App1:      relation.Endpoints[0].ApplicationName,
			Endpoint1: relation.Endpoints[0].Name,
			App2:      relation.Endpoints[1].ApplicationName,
			Endpoint2: relation.Endpoints[1].Name,
		})
	}
	// Get all the annotations.
	annotations, err := modelExtractor.GetAnnotations(annotationTags)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, result := range annotations {
		if result.Error.Error != nil {
			return nil, errors.Trace(result.Error.Error)
		}
		tag, err := names.ParseTag(result.EntityTag)
		if err != nil {
			return nil, errors.Trace(err) // This should never happen.
		}
		switch kind := tag.Kind(); kind {
		case names.ApplicationTagKind:
			mod.Applications[tag.Id()].Annotations = result.Annotations
		case names.MachineTagKind:
			mod.Machines[tag.Id()].Annotations = result.Annotations
		default:
			return nil, errors.Errorf("unexpected tag kind for annotations: %q", kind)
		}
	}
	// Add in the model sequences.
	sequences, err := modelExtractor.Sequences()
	if err == nil {
		mod.Sequence = sequences
	} else if !errors.IsNotSupported(err) {
		return nil, errors.Annotate(err, "getting model sequences")
	}

	// When dealing with bundles the current model generation is always used.
	sort.Strings(appNames)
	configValues, err := modelExtractor.GetConfig(model.GenerationMaster, appNames...)
	if err != nil {
		return nil, errors.Annotate(err, "getting application options")
	}
	for i, cfg := range configValues {
		options := make(map[string]interface{})
		// The config map has values that looks like this:
		//  map[string]interface {}{
		//        "value":       "",
		//        "source":     "user", // or "unset" or "default"
		//        "description": "Where to gather metrics from.\nExamples:\n  host1.maas:9090\n  host1.maas:9090, host2.maas:9090\n",
		//        "type":        "string",
		//    },
		// We want the value iff default is false.
		for key, valueMap := range cfg {
			value, err := applicationConfigValue(key, valueMap)
			if err != nil {
				return nil, errors.Annotatef(err, "bad application config for %q", appNames[i])
			}
			if value != nil {
				options[key] = value
			}
		}
		mod.Applications[appNames[i]].Options = options
	}
	// Lastly get all the application constraints.
	sort.Strings(principalApps)
	constraintValues, err := modelExtractor.GetConstraints(principalApps...)
	if err != nil {
		return nil, errors.Annotate(err, "getting application constraints")
	}
	for i, value := range constraintValues {
		mod.Applications[principalApps[i]].Constraints = value.String()
	}

	mod.ConstraintsEqual = func(a, b string) bool {
		// Since the constraints have already been validated, we don't
		// even bother checking the error response here.
		ac, _ := constraints.Parse(a)
		bc, _ := constraints.Parse(b)
		return reflect.DeepEqual(ac, bc)
	}

	return mod, nil
}

// applicationConfigValue returns the value if it is not a default value.
// If the value is a default value, nil is returned.
// If there was issue determining the type or value, an error is returned.
func applicationConfigValue(key string, valueMap interface{}) (interface{}, error) {
	vm, ok := valueMap.(map[string]interface{})
	if !ok {
		return nil, errors.Errorf("unexpected application config value type %T for key %q", valueMap, key)
	}
	source, found := vm["source"]
	if !found {
		return nil, errors.Errorf("missing application config value 'source' for key %q", key)
	}
	if source == "unset" {
		return nil, nil
	}
	value, found := vm["value"]
	if !found {
		return nil, errors.Errorf("missing application config value 'value'")
	}
	return value, nil
}

// ComposeAndVerifyBundle merges base and overlays then verifies the
// combined bundle data. Returns a slice of errors encountered while
// processing the bundle. They are for informational purposes and do
// not require failing the bundle deployment.
func ComposeAndVerifyBundle(base BundleDataSource, pathToOverlays []string) (*charm.BundleData, []error, error) {
	var dsList []charm.BundleDataSource
	unMarshallErrors := make([]error, 0)
	unMarshallErrors = append(unMarshallErrors, gatherErrors(base)...)

	dsList = append(dsList, base)
	for _, pathToOverlay := range pathToOverlays {
		ds, err := charm.LocalBundleDataSource(pathToOverlay)
		if err != nil {
			return nil, nil, errors.Annotatef(err, "unable to process overlays")
		}
		dsList = append(dsList, ds)
		unMarshallErrors = append(unMarshallErrors, gatherErrors(ds)...)
	}

	bundleData, err := charm.ReadAndMergeBundleData(dsList...)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if err = verifyBundle(bundleData, base.BasePath()); err != nil {
		return nil, nil, errors.Trace(err)
	}

	return bundleData, unMarshallErrors, nil
}

func gatherErrors(ds BundleDataSource) []error {
	returnErrors := make([]error, 0)
	for _, p := range ds.Parts() {
		if p.UnmarshallError == nil {
			continue
		}
		returnErrors = append(returnErrors, p.UnmarshallError)
	}
	return returnErrors
}

func verifyBundle(data *charm.BundleData, bundleDir string) error {
	verifyConstraints := func(s string) error {
		_, err := constraints.Parse(s)
		return err
	}
	verifyStorage := func(s string) error {
		_, err := storage.ParseConstraints(s)
		return err
	}
	verifyDevices := func(s string) error {
		_, err := devices.ParseConstraints(s)
		return err
	}
	var verifyError error
	if bundleDir == "" {
		verifyError = data.Verify(verifyConstraints, verifyStorage, verifyDevices)
	} else {
		verifyError = data.VerifyLocal(bundleDir, verifyConstraints, verifyStorage, verifyDevices)
	}

	if verr, ok := errors.Cause(verifyError).(*charm.VerificationError); ok {
		errs := make([]string, len(verr.Errors))
		for i, err := range verr.Errors {
			errs[i] = err.Error()
		}
		return errors.New("the provided bundle has the following errors:\n" + strings.Join(errs, "\n"))
	}
	return errors.Trace(verifyError)
}
