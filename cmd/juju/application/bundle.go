// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package application

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/juju/bundlechanges"
	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable"
	csparams "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/charms"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/resource/resourceadapters"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/storage"
)

var watchAll = func(c *api.Client) (allWatcher, error) {
	return c.WatchAll()
}

type allWatcher interface {
	Next() ([]multiwatcher.Delta, error)
	Stop() error
}

// deploymentLogger is used to notify clients about the bundle deployment
// progress.
type deploymentLogger interface {
	// Infof formats and logs the given message.
	Infof(string, ...interface{})
}

// deployBundle deploys the given bundle data using the given API client and
// charm store client. The deployment is not transactional, and its progress is
// notified using the given deployment logger.
func deployBundle(
	bundleDir string,
	data *charm.BundleData,
	bundleConfigFile string,
	channel csparams.Channel,
	apiRoot DeployAPI,
	log deploymentLogger,
	bundleStorage map[string]map[string]storage.Constraints,
) (map[*charm.URL]*macaroon.Macaroon, error) {

	if err := processBundleConfig(data, bundleConfigFile); err != nil {
		return nil, err
	}
	verifyConstraints := func(s string) error {
		_, err := constraints.Parse(s)
		return err
	}
	verifyStorage := func(s string) error {
		_, err := storage.ParseConstraints(s)
		return err
	}
	var verifyError error
	if bundleDir == "" {
		verifyError = data.Verify(verifyConstraints, verifyStorage)
	} else {
		// Process includes in the bundle data.
		if err := processBundleIncludes(bundleDir, data); err != nil {
			return nil, errors.Annotate(err, "unable to process includes")
		}
		verifyError = data.VerifyLocal(bundleDir, verifyConstraints, verifyStorage)
	}
	if verifyError != nil {
		if verr, ok := verifyError.(*charm.VerificationError); ok {
			errs := make([]string, len(verr.Errors))
			for i, err := range verr.Errors {
				errs[i] = err.Error()
			}
			return nil, errors.New("the provided bundle has the following errors:\n" + strings.Join(errs, "\n"))
		}
		return nil, errors.Annotate(verifyError, "cannot deploy bundle")
	}

	// Retrieve bundle changes.
	changes := bundlechanges.FromData(data)
	numChanges := len(changes)

	// Initialize the unit status.
	status, err := apiRoot.Status(nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get model status")
	}
	unitStatus := make(map[string]string, numChanges)
	for _, serviceData := range status.Applications {
		for unit, unitData := range serviceData.Units {
			unitStatus[unit] = unitData.Machine
		}
	}

	// Instantiate a watcher used to follow the deployment progress.
	watcher, err := apiRoot.WatchAll()
	if err != nil {
		return nil, errors.Annotate(err, "cannot watch model")
	}
	defer watcher.Stop()

	// Instantiate the bundle handler.
	h := &bundleHandler{
		bundleDir:       bundleDir,
		changes:         changes,
		results:         make(map[string]string, numChanges),
		channel:         channel,
		api:             apiRoot,
		bundleStorage:   bundleStorage,
		log:             log,
		data:            data,
		unitStatus:      unitStatus,
		ignoredMachines: make(map[string]bool, len(data.Applications)),
		ignoredUnits:    make(map[string]bool, len(data.Applications)),
		watcher:         watcher,
	}

	// Deploy the bundle.
	csMacs := make(map[*charm.URL]*macaroon.Macaroon)
	channels := make(map[*charm.URL]csparams.Channel)
	for _, change := range changes {
		switch change := change.(type) {
		case *bundlechanges.AddCharmChange:
			cURL, channel, csMac, err2 := h.addCharm(change.Id(), change.Params)
			if err2 == nil {
				csMacs[cURL] = csMac
				channels[cURL] = channel
			}
			err = err2
		case *bundlechanges.AddMachineChange:
			err = h.addMachine(change.Id(), change.Params)
		case *bundlechanges.AddRelationChange:
			err = h.addRelation(change.Id(), change.Params)
		case *bundlechanges.AddApplicationChange:
			var cURL *charm.URL
			cURL, err = charm.ParseURL(resolve(change.Params.Charm, h.results))
			if err == nil {
				chID := charmstore.CharmID{
					URL:     cURL,
					Channel: channels[cURL],
				}
				csMac := csMacs[cURL]
				err = h.addService(apiRoot, change.Id(), change.Params, chID, csMac)
			}
		case *bundlechanges.AddUnitChange:
			err = h.addUnit(change.Id(), change.Params)
		case *bundlechanges.ExposeChange:
			err = h.exposeService(change.Id(), change.Params)
		case *bundlechanges.SetAnnotationsChange:
			err = h.setAnnotations(change.Id(), change.Params)
		default:
			return nil, errors.Errorf("unknown change type: %T", change)
		}
		if err != nil {
			return nil, errors.Annotate(err, "cannot deploy bundle")
		}
	}
	return csMacs, nil
}

// bundleHandler provides helpers and the state required to deploy a bundle.
type bundleHandler struct {
	// bundleDir is the path where the bundle file is located for local bundles.
	bundleDir string
	// changes holds the changes to be applied in order to deploy the bundle.
	changes []bundlechanges.Change

	// results collects data resulting from applying changes. Keys identify
	// changes, values result from interacting with the environment, and are
	// stored so that they can be potentially reused later, for instance for
	// resolving a dynamic placeholder included in a change. Specifically, the
	// following values are stored:
	// - when adding a charm, the fully resolved charm is stored;
	// - when deploying an application, the application name is stored;
	// - when adding a machine, the resulting machine id is stored;
	// - when adding a unit, either the id of the machine holding the unit or
	//   the unit name can be stored. The latter happens when a machine is
	//   implicitly created by adding a unit without a machine spec.
	results map[string]string

	// channel identifies the default channel to use for the bundle.
	channel csparams.Channel

	// api is used to interact with the environment.
	api DeployAPI

	// bundleStorage contains a mapping of application-specific storage
	// constraints. For each application, the storage constraints in the
	// map will replace or augment the storage constraints specified
	// in the bundle itself.
	bundleStorage map[string]map[string]storage.Constraints

	// log is used to output messages to the user, so that the user can keep
	// track of the bundle deployment progress.
	log deploymentLogger

	// data is the original bundle data that we want to deploy.
	data *charm.BundleData

	// unitStatus reflects the environment status and maps unit names to their
	// corresponding machine identifiers. This is kept updated by both change
	// handlers (addCharm, addService etc.) and by updateUnitStatus.
	unitStatus map[string]string

	// ignoredMachines and ignoredUnits map application names to whether a machine
	// or a unit creation has been skipped during the bundle deployment because
	// the current status of the environment does not require them to be added.
	ignoredMachines map[string]bool
	ignoredUnits    map[string]bool

	// watcher holds an environment mega-watcher used to keep the environment
	// status up to date.
	watcher allWatcher

	// warnedLXC indicates whether or not we have warned the user that the
	// bundle they're deploying uses lxc containers, which will be treated as
	// LXD.  This flag keeps us from writing the warning more than once per
	// bundle.
	warnedLXC bool
}

// addCharm adds a charm to the environment.
func (h *bundleHandler) addCharm(id string, p bundlechanges.AddCharmParams) (*charm.URL, csparams.Channel, *macaroon.Macaroon, error) {
	// First attempt to interpret as a local path.
	if strings.HasPrefix(p.Charm, ".") || filepath.IsAbs(p.Charm) {
		charmPath := p.Charm
		if !filepath.IsAbs(charmPath) {
			charmPath = filepath.Join(h.bundleDir, charmPath)
		}

		var noChannel csparams.Channel
		series := p.Series
		if series == "" {
			series = h.data.Series
		}
		ch, curl, err := charmrepo.NewCharmAtPath(charmPath, series)
		if err != nil && !os.IsNotExist(err) {
			return nil, noChannel, nil, errors.Annotatef(err, "cannot deploy local charm at %q", charmPath)
		}
		if err == nil {
			if curl, err = h.api.AddLocalCharm(curl, ch); err != nil {
				return nil, noChannel, nil, err
			}
			logger.Debugf("added charm %s", curl)
			h.results[id] = curl.String()
			return curl, noChannel, nil, nil
		}
	}

	// Not a local charm, so grab from the store.
	ch, err := charm.ParseURL(p.Charm)
	if err != nil {
		return nil, "", nil, errors.Trace(err)
	}
	modelCfg, err := getModelConfig(h.api)
	if err != nil {
		return nil, "", nil, errors.Trace(err)
	}

	url, channel, _, err := h.api.Resolve(modelCfg, ch)
	if err != nil {
		return nil, channel, nil, errors.Annotatef(err, "cannot resolve URL %q", p.Charm)
	}
	if url.Series == "bundle" {
		return nil, channel, nil, errors.Errorf("expected charm URL, got bundle URL %q", p.Charm)
	}
	var csMac *macaroon.Macaroon
	url, csMac, err = addCharmFromURL(h.api, url, channel)
	if err != nil {
		return nil, channel, nil, errors.Annotatef(err, "cannot add charm %q", p.Charm)
	}
	logger.Debugf("added charm %s", url)
	h.results[id] = url.String()
	return url, channel, csMac, nil
}

// addService deploys or update an application with no units. Service options are
// also set or updated.
func (h *bundleHandler) addService(
	api DeployAPI,
	id string,
	p bundlechanges.AddApplicationParams,
	chID charmstore.CharmID,
	csMac *macaroon.Macaroon,
) error {
	h.results[id] = p.Application
	ch := chID.URL.String()
	// Handle application configuration.
	configYAML := ""
	if len(p.Options) > 0 {
		config, err := yaml.Marshal(map[string]map[string]interface{}{p.Application: p.Options})
		if err != nil {
			return errors.Annotatef(err, "cannot marshal options for application %q", p.Application)
		}
		configYAML = string(config)
	}
	// Handle application constraints.
	cons, err := constraints.Parse(p.Constraints)
	if err != nil {
		// This should never happen, as the bundle is already verified.
		return errors.Annotate(err, "invalid constraints for application")
	}
	storageConstraints := h.bundleStorage[p.Application]
	if len(p.Storage) > 0 {
		if storageConstraints == nil {
			storageConstraints = make(map[string]storage.Constraints)
		}
		for k, v := range p.Storage {
			if _, ok := storageConstraints[k]; ok {
				// Storage constraints overridden
				// on the command line.
				continue
			}
			cons, err := storage.ParseConstraints(v)
			if err != nil {
				return errors.Annotate(err, "invalid storage constraints")
			}
			storageConstraints[k] = cons
		}
	}
	resources := make(map[string]string)
	for resName, path := range p.LocalResources {
		resources[resName] = path
	}
	for resName, revision := range p.Resources {
		resources[resName] = fmt.Sprint(revision)
	}
	charmInfo, err := h.api.CharmInfo(ch)
	if err != nil {
		return err
	}
	resNames2IDs, err := resourceadapters.DeployResources(
		p.Application,
		chID,
		csMac,
		resources,
		charmInfo.Meta.Resources,
		api,
	)
	if err != nil {
		return errors.Trace(err)
	}

	// Figure out what series we need to deploy with.
	conf, err := getModelConfig(h.api)
	if err != nil {
		return err
	}
	supportedSeries := charmInfo.Meta.Series
	if len(supportedSeries) == 0 && chID.URL.Series != "" {
		supportedSeries = []string{chID.URL.Series}
	}
	selector := seriesSelector{
		seriesFlag:      p.Series,
		charmURLSeries:  chID.URL.Series,
		supportedSeries: supportedSeries,
		conf:            conf,
		fromBundle:      true,
	}
	series, err := selector.charmSeries()
	if err != nil {
		return errors.Trace(err)
	}

	// Deploy the application.
	logger.Debugf("application %s is deploying (charm %s)", p.Application, ch)
	h.log.Infof("Deploying charm %q", ch)
	if err := api.Deploy(application.DeployArgs{
		CharmID:          chID,
		Cons:             cons,
		ApplicationName:  p.Application,
		Series:           series,
		ConfigYAML:       configYAML,
		Storage:          storageConstraints,
		Resources:        resNames2IDs,
		EndpointBindings: p.EndpointBindings,
	}); err == nil {
		for resName := range resNames2IDs {
			h.log.Infof("added resource %s", resName)
		}
		return nil
	} else if !isErrServiceExists(err) {
		return errors.Annotatef(err, "cannot deploy application %q", p.Application)
	}
	// The application is already deployed in the environment: check that its
	// charm is compatible with the one declared in the bundle. If it is,
	// reuse the existing application or upgrade to a specified revision.
	// Exit with an error otherwise.
	if err := h.upgradeCharm(api, p.Application, chID, csMac, resources); err != nil {
		return errors.Annotatef(err, "cannot upgrade application %q", p.Application)
	}
	// Update application configuration.
	if configYAML != "" {
		if err := h.api.Update(params.ApplicationUpdate{
			ApplicationName: p.Application,
			SettingsYAML:    configYAML,
		}); err != nil {
			// This should never happen as possible errors are already returned
			// by the application Deploy call above.
			return errors.Annotatef(err, "cannot update options for application %q", p.Application)
		}
		h.log.Infof("configuration updated for application %s", p.Application)
	}
	// Update application constraints.
	if p.Constraints != "" {
		if err := h.api.SetConstraints(p.Application, cons); err != nil {
			// This should never happen, as the bundle is already verified.
			return errors.Annotatef(err, "cannot update constraints for application %q", p.Application)
		}
		h.log.Infof("constraints applied for application %s", p.Application)
	}
	return nil
}

// addMachine creates a new top-level machine or container in the environment.
func (h *bundleHandler) addMachine(id string, p bundlechanges.AddMachineParams) error {
	services := h.servicesForMachineChange(id)
	// Note that we always have at least one application that justifies the
	// creation of this machine.
	msg := services[0] + " unit"
	svcLen := len(services)
	if svcLen != 1 {
		msg = strings.Join(services[:svcLen-1], ", ") + " and " + services[svcLen-1] + " units"
	}
	// Check whether the desired number of units already exist in the
	// environment, in which case avoid adding other machines to host those
	// application units.
	machine := h.chooseMachine(services...)
	if machine != "" {
		h.results[id] = machine
		notify := make([]string, 0, svcLen)
		for _, application := range services {
			if !h.ignoredMachines[application] {
				h.ignoredMachines[application] = true
				notify = append(notify, application)
			}
		}
		svcLen = len(notify)
		switch svcLen {
		case 0:
			return nil
		case 1:
			msg = notify[0]
		default:
			msg = strings.Join(notify[:svcLen-1], ", ") + " and " + notify[svcLen-1]
		}
		h.log.Infof("avoid creating other machines to host %s units", msg)
		return nil
	}
	// Check if machine mapping was provided for services
	for _, application := range services {
		for _, toMach := range h.data.Applications[application].To {
			if targetMach, ok := machinesMap[toMach]; ok {
				if ! machinesProvided[targetMach] {
					h.results[id] = targetMach
					machinesProvided[targetMach] = true
					msg = application
					h.log.Infof("Assigned existing (pre-created) machine: %s to host: %s units", targetMach, msg)
					return nil
				}
			}
		}
	}
	cons, err := constraints.Parse(p.Constraints)
	if err != nil {
		// This should never happen, as the bundle is already verified.
		return errors.Annotate(err, "invalid constraints for machine")
	}
	machineParams := params.AddMachineParams{
		Constraints: cons,
		Series:      p.Series,
		Jobs:        []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
	}
	if ct := p.ContainerType; ct != "" {
		// for backwards compatibility with 1.x bundles, we treat lxc
		// placement directives as lxd.
		if ct == "lxc" {
			if !h.warnedLXC {
				h.log.Infof("Bundle has one or more containers specified as lxc. lxc containers are deprecated in Juju 2.0. lxd containers will be deployed instead.")
				h.warnedLXC = true
			}
			ct = string(instance.LXD)
		}
		containerType, err := instance.ParseContainerType(ct)
		if err != nil {
			return errors.Annotatef(err, "cannot create machine for holding %s", msg)
		}
		machineParams.ContainerType = containerType
		if p.ParentId != "" {
			machineParams.ParentId, err = h.resolveMachine(p.ParentId)
			if err != nil {
				return errors.Annotatef(err, "cannot retrieve parent placement for %s", msg)
			}
		}
	}
	r, err := h.api.AddMachines([]params.AddMachineParams{machineParams})
	if err != nil {
		return errors.Annotatef(err, "cannot create machine for holding %s", msg)
	}
	if r[0].Error != nil {
		return errors.Annotatef(r[0].Error, "cannot create machine for holding %s", msg)
	}
	machine = r[0].Machine
	if p.ContainerType == "" {
		logger.Debugf("created new machine %s for holding %s", machine, msg)
	} else if p.ParentId == "" {
		logger.Debugf("created %s container in new machine for holding %s", machine, msg)
	} else {
		logger.Debugf("created %s container in machine %s for holding %s", machine, machineParams.ParentId, msg)
	}
	h.results[id] = machine
	return nil
}

// addRelation creates a relationship between two services.
func (h *bundleHandler) addRelation(id string, p bundlechanges.AddRelationParams) error {
	ep1 := resolveRelation(p.Endpoint1, h.results)
	ep2 := resolveRelation(p.Endpoint2, h.results)
	// TODO(wallyworld) - CMR support in bundles
	_, err := h.api.AddRelation([]string{ep1, ep2}, nil)
	if err == nil {
		// A new relation has been established.
		h.log.Infof("Related %q and %q", ep1, ep2)
		return nil
	}
	if isErrRelationExists(err) {
		// The relation is already present in the environment.
		logger.Debugf("%s and %s are already related", ep1, ep2)
		return nil
	}
	return errors.Annotatef(err, "cannot add relation between %q and %q", ep1, ep2)
}

// addUnit adds a single unit to an application already present in the environment.
func (h *bundleHandler) addUnit(id string, p bundlechanges.AddUnitParams) error {
	applicationName := resolve(p.Application, h.results)
	// Check whether the desired number of units already exist in the
	// environment, in which case avoid adding other units.
	machine := h.chooseMachine(applicationName)
	if machine != "" {
		h.results[id] = machine
		if !h.ignoredUnits[applicationName] {
			h.ignoredUnits[applicationName] = true
			num := h.numUnitsForService(applicationName)
			var msg string
			if num == 1 {
				msg = "1 unit already present"
			} else {
				msg = fmt.Sprintf("%d units already present", num)
			}
			h.log.Infof("avoid adding new units to application %s: %s", applicationName, msg)
		}
		return nil
	}
	var machineSpec string
	var placementArg []*instance.Placement
	if p.To != "" {
		var err error
		if machineSpec, err = h.resolveMachine(p.To); err != nil {
			// Should never happen.
			return errors.Annotatef(err, "cannot retrieve placement for %q unit", applicationName)
		}
		placement, err := parsePlacement(machineSpec)
		if err != nil {
			return errors.Errorf("invalid --to parameter %q", machineSpec)
		}
		placementArg = append(placementArg, placement)
	}
	r, err := h.api.AddUnits(application.AddUnitsParams{
		ApplicationName: applicationName,
		NumUnits:        1,
		Placement:       placementArg,
	})
	if err != nil {
		return errors.Annotatef(err, "cannot add unit for application %q", applicationName)
	}
	unit := r[0]
	if machineSpec == "" {
		logger.Debugf("added %s unit to new machine", unit)
		// In this case, the unit name is stored in results instead of the
		// machine id, which is lazily evaluated later only if required.
		// This way we avoid waiting for watcher updates.
		h.results[id] = unit
	} else {
		logger.Debugf("added %s unit to new machine", unit)
		h.results[id] = machineSpec
	}
	// Note that the machineSpec can be empty for now, resulting in a partially
	// incomplete unit status. That's ok as the missing info is provided later
	// when it is required.
	h.unitStatus[unit] = machineSpec
	return nil
}

// exposeService exposes an application.
func (h *bundleHandler) exposeService(id string, p bundlechanges.ExposeParams) error {
	application := resolve(p.Application, h.results)
	if err := h.api.Expose(application); err != nil {
		return errors.Annotatef(err, "cannot expose application %s", application)
	}
	h.log.Infof("application %s exposed", application)
	return nil
}

// setAnnotations sets annotations for an application or a machine.
func (h *bundleHandler) setAnnotations(id string, p bundlechanges.SetAnnotationsParams) error {
	eid := resolve(p.Id, h.results)
	var tag string
	switch p.EntityType {
	case bundlechanges.MachineType:
		tag = names.NewMachineTag(eid).String()
	case bundlechanges.ApplicationType:
		tag = names.NewApplicationTag(eid).String()
	default:
		return errors.Errorf("unexpected annotation entity type %q", p.EntityType)
	}
	result, err := h.api.SetAnnotation(map[string]map[string]string{tag: p.Annotations})
	if err == nil && len(result) > 0 {
		err = result[0].Error
	}
	if err != nil {
		return errors.Annotatef(err, "cannot set annotations for %s %q", p.EntityType, eid)
	}
	logger.Debugf("annotations set for %s %s", p.EntityType, eid)
	return nil
}

// servicesForMachineChange returns the names of the services for which an
// "addMachine" change is required, as adding machines is required to place
// units, and units belong to services.
// Receive the id of the "addMachine" change.
func (h *bundleHandler) servicesForMachineChange(changeId string) []string {
	services := make(map[string]bool, len(h.data.Applications))
mainloop:
	for _, change := range h.changes {
		for _, required := range change.Requires() {
			if required != changeId {
				continue
			}
			switch change := change.(type) {
			case *bundlechanges.AddMachineChange:
				// The original machine is a container, and its parent is
				// another "addMachines" change. Search again using the
				// parent id.
				for _, application := range h.servicesForMachineChange(change.Id()) {
					services[application] = true
				}
				continue mainloop
			case *bundlechanges.AddUnitChange:
				// We have found the "addUnit" change, which refers to a
				// application: now resolve the application holding the unit.
				application := resolve(change.Params.Application, h.results)
				services[application] = true
				continue mainloop
			case *bundlechanges.SetAnnotationsChange:
				// A machine change is always required to set machine
				// annotations, but this isn't the interesting change here.
				continue mainloop
			default:
				// Should never happen.
				panic(fmt.Sprintf("unexpected change %T", change))
			}
		}
	}
	results := make([]string, 0, len(services))
	for application := range services {
		results = append(results, application)
	}
	sort.Strings(results)
	return results
}

// chooseMachine returns the id of a machine that will be used to host a unit
// of all the given services. If one of the services still requires units to be
// added, an empty string is returned, meaning that a new machine must be
// created for holding the unit. If instead all units are already placed,
// return the id of the machine which already holds units of the given services
// and which hosts the least number of units.
func (h *bundleHandler) chooseMachine(services ...string) string {
	candidateMachines := make(map[string]bool, len(h.unitStatus))
	numUnitsPerMachine := make(map[string]int, len(h.unitStatus))
	numUnitsPerService := make(map[string]int, len(h.data.Applications))
	// Collect the number of units and the corresponding machines for all
	// involved services.
	for unit, machine := range h.unitStatus {
		// Retrieve the top level machine.
		machine = strings.Split(machine, "/")[0]
		numUnitsPerMachine[machine]++
		svc, err := names.UnitApplication(unit)
		if err != nil {
			// Should never happen because the bundle logic has already checked
			// that unit names are well formed.
			panic(err)
		}
		for _, application := range services {
			if application != svc {
				continue
			}
			numUnitsPerService[application]++
			candidateMachines[machine] = true
		}
	}
	// If at least one application still requires units to be added, return an
	// empty machine in order to force new machine creation.
	for _, application := range services {
		if numUnitsPerService[application] < h.data.Applications[application].NumUnits {
			return ""
		}
	}
	// Return the least used machine.
	var result string
	var min int
	for machine, num := range numUnitsPerMachine {
		if candidateMachines[machine] && (result == "" || num < min) {
			result, min = machine, num
		}
	}
	return result
}

// updateUnitStatusPeriod is the time duration used to wait for a mega-watcher
// change to be available.
var updateUnitStatusPeriod = watcher.Period + 5*time.Second

// updateUnitStatus uses the mega-watcher to update units and machines info
// (h.unitStatus) so that it reflects the current environment status.
// This function must be called assuming new delta changes are available or
// will be available within the watcher time period. Otherwise, the function
// unblocks and an error is returned.
func (h *bundleHandler) updateUnitStatus() error {
	var delta []multiwatcher.Delta
	var err error
	ch := make(chan struct{})
	go func() {
		delta, err = h.watcher.Next()
		close(ch)
	}()
	select {
	case <-ch:
		if err != nil {
			return errors.Annotate(err, "cannot update model status")
		}
		for _, d := range delta {
			switch entityInfo := d.Entity.(type) {
			case *multiwatcher.UnitInfo:
				h.unitStatus[entityInfo.Name] = entityInfo.MachineId
			}
		}
	case <-time.After(updateUnitStatusPeriod):
		// TODO(fwereade): 2016-03-17 lp:1558657
		return errors.New("timeout while trying to get new changes from the watcher")
	}
	return nil
}

// numUnitsForService return the number of units belonging to the given application
// currently in the environment.
func (h *bundleHandler) numUnitsForService(application string) (num int) {
	for unit := range h.unitStatus {
		svc, err := names.UnitApplication(unit)
		if err != nil {
			// Should never happen.
			panic(err)
		}
		if svc == application {
			num++
		}
	}
	return num
}

// resolveMachine returns the machine id resolving the given unit or machine
// placeholder.
func (h *bundleHandler) resolveMachine(placeholder string) (string, error) {
	machineOrUnit := resolve(placeholder, h.results)
	if !names.IsValidUnit(machineOrUnit) {
		return machineOrUnit, nil
	}
	for h.unitStatus[machineOrUnit] == "" {
		if err := h.updateUnitStatus(); err != nil {
			return "", errors.Annotate(err, "cannot resolve machine")
		}
	}
	return h.unitStatus[machineOrUnit], nil
}

// resolveRelation returns the relation name resolving the included application
// placeholder.
func resolveRelation(e string, results map[string]string) string {
	parts := strings.SplitN(e, ":", 2)
	application := resolve(parts[0], results)
	if len(parts) == 1 {
		return application
	}
	return fmt.Sprintf("%s:%s", application, parts[1])
}

// resolve returns the real entity name for the bundle entity (for instance a
// application or a machine) with the given placeholder id.
// A placeholder id is a string like "$deploy-42" or "$addCharm-2", indicating
// the results of a previously applied change. It always starts with a dollar
// sign, followed by the identifier of the referred change. A change id is a
// string indicating the action type ("deploy", "addRelation" etc.), followed
// by a unique incremental number.
func resolve(placeholder string, results map[string]string) string {
	if !strings.HasPrefix(placeholder, "$") {
		panic(`placeholder does not start with "$"`)
	}
	id := placeholder[1:]
	return results[id]
}

// upgradeCharm upgrades the charm for the given application to the given charm id.
// If the application is already deployed using the given charm id, do nothing.
// This function returns an error if the existing charm and the target one are
// incompatible, meaning an upgrade from one to the other is not allowed.
func (h *bundleHandler) upgradeCharm(
	api DeployAPI,
	applicationName string,
	chID charmstore.CharmID,
	csMac *macaroon.Macaroon,
	resources map[string]string,
) error {
	id := chID.URL.String()
	existing, err := h.api.GetCharmURL(applicationName)
	if err != nil {
		return errors.Annotatef(err, "cannot retrieve info for application %q", applicationName)
	}
	if existing.String() == id {
		h.log.Infof("reusing application %s (charm: %s)", applicationName, id)
		return nil
	}
	url, err := charm.ParseURL(id)
	if err != nil {
		return errors.Annotatef(err, "cannot parse charm URL %q", id)
	}
	chID.URL = url
	if url.WithRevision(-1).Path() != existing.WithRevision(-1).Path() {
		return errors.Errorf("bundle charm %q is incompatible with existing charm %q", id, existing)
	}
	charmsClient := charms.NewClient(api)
	resourceLister, err := resourceadapters.NewAPIClient(api)
	if err != nil {
		return errors.Trace(err)
	}
	filtered, err := getUpgradeResources(charmsClient, resourceLister, applicationName, url, resources)
	if err != nil {
		return errors.Trace(err)
	}
	var resNames2IDs map[string]string
	if len(filtered) != 0 {
		resNames2IDs, err = resourceadapters.DeployResources(
			applicationName,
			chID,
			csMac,
			resources,
			filtered,
			api,
		)
		if err != nil {
			return errors.Trace(err)
		}
	}
	cfg := application.SetCharmConfig{
		ApplicationName: applicationName,
		CharmID:         chID,
		ResourceIDs:     resNames2IDs,
	}
	if err := h.api.SetCharm(cfg); err != nil {
		return errors.Annotatef(err, "cannot upgrade charm to %q", id)
	}
	h.log.Infof("upgraded charm for existing application %s (from %s to %s)", applicationName, existing, id)
	for resName := range resNames2IDs {
		h.log.Infof("added resource %s", resName)
	}
	return nil
}

// isErrServiceExists reports whether the given error has been generated
// from trying to deploy an application that already exists.
func isErrServiceExists(err error) bool {
	// TODO frankban (bug 1495952): do this check using the cause rather than
	// the string when a specific cause is available.
	return params.IsCodeAlreadyExists(err) || strings.HasSuffix(err.Error(), "application already exists")
}

// isErrRelationExists reports whether the given error has been generated
// from trying to create an already established relation.
func isErrRelationExists(err error) bool {
	// TODO frankban (bug 1495952): do this check using the cause rather than
	// the string when a specific cause is available.
	return params.IsCodeAlreadyExists(err) || strings.HasSuffix(err.Error(), "relation already exists")
}

func processBundleIncludes(baseDir string, data *charm.BundleData) error {

	for app, appData := range data.Applications {
		for key, value := range appData.Options {
			result, processed, err := processValue(baseDir, value)
			if err != nil {
				return errors.Annotatef(err, "processing options value %s for application %s", key, app)
			}
			if processed {
				appData.Options[key] = result
			}
		}
		for key, value := range appData.Annotations {
			result, processed, err := processValue(baseDir, value)
			if err != nil {
				return errors.Annotatef(err, "processing annotation value %s for application %s", key, app)
			}
			if processed {
				appData.Annotations[key] = result.(string)
			}
		}
	}

	for machine, machineData := range data.Machines {
		for key, value := range machineData.Annotations {
			result, processed, err := processValue(baseDir, value)
			if err != nil {
				return errors.Annotatef(err, "processing annotation value %s for machine %s", key, machine)
			}
			if processed {
				machineData.Annotations[key] = result.(string)
			}
		}
	}
	return nil
}

func processValue(baseDir string, v interface{}) (interface{}, bool, error) {

	const (
		includeFile   = "include-file://"
		includeBase64 = "include-base64://"
	)

	value, ok := v.(string)
	if !ok {
		// Not a string, just return it unchanged.
		return v, false, nil
	}

	encode := false
	readFile := false
	filename := ""

	if strings.HasPrefix(value, includeFile) {
		readFile = true
		filename = value[len(includeFile):]
	} else if strings.HasPrefix(value, includeBase64) {
		encode = true
		readFile = true
		filename = value[len(includeBase64):]
	}

	if !readFile {
		// Unchanged, just return it.
		return v, false, nil
	}

	if !filepath.IsAbs(filename) {
		filename = filepath.Clean(filepath.Join(baseDir, filename))
	}

	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, false, errors.Annotate(err, "unable to read file")
	}

	var result string
	if encode {
		result = base64.StdEncoding.EncodeToString(bytes)
	} else {
		result = string(bytes)
	}

	return result, true, nil
}

type bundleConfig struct {
	Applications map[string]*charm.ApplicationSpec `yaml:"applications"`
	// TODO soon, add machine mapping and space mapping.
}

type bundleConfigValueExists struct {
	Applications map[string]map[string]interface{} `yaml:"applications"`
}

func processBundleConfig(data *charm.BundleData, bundleConfigFile string) error {
	if bundleConfigFile == "" {
		// Nothing to do here.
		return nil
	}

	bundleConfigFile, err := utils.NormalizePath(bundleConfigFile)
	if err != nil {
		return errors.Annotate(err, "unable to normalise bundle-config file")
	}
	// Make sure the filename is absolute.
	if !filepath.IsAbs(bundleConfigFile) {
		cwd, err := os.Getwd()
		if err != nil {
			return errors.Trace(err)
		}
		bundleConfigFile = filepath.Clean(filepath.Join(cwd, bundleConfigFile))
	}
	content, err := ioutil.ReadFile(bundleConfigFile)
	if err != nil {
		return errors.Annotate(err, "unable to open bundle-config file")
	}
	baseDir := filepath.Dir(bundleConfigFile)

	// Now that we have the content, attempt to deserialize into the bundleConfig.
	var config bundleConfig
	if err := yaml.Unmarshal(content, &config); err != nil {
		return errors.Annotate(err, "unable to deserialize config structure")
	}
	// If this works, then this deserialisation should certainly succeed.
	// Since we are only looking to overwrite the values in the underlying bundle
	// for config values that are set, we need to know if they were actually set,
	// and not just zero. The configCheck structure is a map that allows us to check
	// if the fields were actually in the underlying YAML.
	var configCheck bundleConfigValueExists
	if err := yaml.Unmarshal(content, &configCheck); err != nil {
		return errors.Annotate(err, "unable to deserialize config structure")
	}
	// Additional checks to make sure that only things that we know about
	// are passed in as config.
	var checkTopLevel map[string]interface{}
	if err := yaml.Unmarshal(content, &checkTopLevel); err != nil {
		return errors.Annotate(err, "unable to deserialize config structure")
	}
	for key := range checkTopLevel {
		switch key {
		case "applications":
			// no-op, all good
		default:
			return errors.Errorf("unexpected key %q in config", key)
		}
	}

	// We want to confirm that all the applications mentioned in the config
	// actually exist in the bundle data.
	for appName, bc := range config.Applications {
		app, found := data.Applications[appName]
		if !found {
			return errors.Errorf("application %q from config not found in bundle", appName)
		}

		fieldCheck := configCheck.Applications[appName]

		if _, set := fieldCheck["charm"]; set {
			app.Charm = bc.Charm
		}
		if _, set := fieldCheck["series"]; set {
			app.Series = bc.Series
		}
		if _, set := fieldCheck["resources"]; set {
			if app.Resources == nil {
				app.Resources = make(map[string]interface{})
			}
			for key, value := range bc.Resources {
				app.Resources[key] = value
			}
		}
		if _, set := fieldCheck["num_units"]; set {
			app.NumUnits = bc.NumUnits
		}
		if _, set := fieldCheck["to"]; set {
			app.To = bc.To
		}
		if _, set := fieldCheck["expose"]; set {
			app.Expose = bc.Expose
		}
		if _, set := fieldCheck["options"]; set {
			if app.Options == nil {
				app.Options = make(map[string]interface{})
			}
			for key, value := range bc.Options {
				result, _, err := processValue(baseDir, value)
				if err != nil {
					return errors.Annotatef(err, "processing config options value %s for application %s", key, appName)
				}
				app.Options[key] = result
			}
		}
		if _, set := fieldCheck["annotations"]; set {
			if app.Annotations == nil {
				app.Annotations = make(map[string]string)
			}
			for key, value := range bc.Annotations {
				result, _, err := processValue(baseDir, value)
				if err != nil {
					return errors.Annotatef(err, "processing config annotations value %s for application %s", key, appName)
				}
				app.Annotations[key] = result.(string)
			}
		}
		if _, set := fieldCheck["constraints"]; set {
			app.Constraints = bc.Constraints
		}
		if _, set := fieldCheck["storage"]; set {
			if app.Storage == nil {
				app.Storage = make(map[string]string)
			}
			for key, value := range bc.Storage {
				app.Storage[key] = value
			}
		}
		if _, set := fieldCheck["bindings"]; set {
			if app.EndpointBindings == nil {
				app.EndpointBindings = make(map[string]string)
			}
			for key, value := range bc.EndpointBindings {
				app.EndpointBindings[key] = value
			}
		}
	}
	return nil
}
