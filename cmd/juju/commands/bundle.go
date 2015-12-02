// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/juju/bundlechanges"
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/yaml.v1"

	"github.com/juju/juju/api"
	apiservice "github.com/juju/juju/api/service"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/storage"
)

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
	data *charm.BundleData, client *api.Client, serviceDeployer *serviceDeployer,
	csclient *csClient, repoPath string, conf *config.Config, log deploymentLogger,
	bundleStorage map[string]map[string]storage.Constraints,
) error {
	verifyConstraints := func(s string) error {
		_, err := constraints.Parse(s)
		return err
	}
	verifyStorage := func(s string) error {
		_, err := storage.ParseConstraints(s)
		return err
	}
	if err := data.Verify(verifyConstraints, verifyStorage); err != nil {
		return errors.Annotate(err, "cannot deploy bundle")
	}

	// Retrieve bundle changes.
	changes := bundlechanges.FromData(data)
	numChanges := len(changes)

	// Initialize the unit status.
	status, err := client.Status(nil)
	if err != nil {
		return errors.Annotate(err, "cannot get environment status")
	}
	unitStatus := make(map[string]string, numChanges)
	for _, serviceData := range status.Services {
		for unit, unitData := range serviceData.Units {
			unitStatus[unit] = unitData.Machine
		}
	}

	// Instantiate a watcher used to follow the deployment progress.
	watcher, err := client.WatchAll()
	if err != nil {
		return errors.Annotate(err, "cannot watch environment")
	}
	defer watcher.Stop()

	serviceClient, err := serviceDeployer.newServiceAPIClient()
	if err != nil {
		return errors.Annotate(err, "cannot get service client")
	}

	// Instantiate the bundle handler.
	h := &bundleHandler{
		changes:         changes,
		results:         make(map[string]string, numChanges),
		client:          client,
		serviceClient:   serviceClient,
		serviceDeployer: serviceDeployer,
		bundleStorage:   bundleStorage,
		csclient:        csclient,
		repoPath:        repoPath,
		conf:            conf,
		log:             log,
		data:            data,
		unitStatus:      unitStatus,
		ignoredMachines: make(map[string]bool, len(data.Services)),
		ignoredUnits:    make(map[string]bool, len(data.Services)),
		watcher:         watcher,
	}

	// Deploy the bundle.
	for _, change := range changes {
		switch change := change.(type) {
		case *bundlechanges.AddCharmChange:
			err = h.addCharm(change.Id(), change.Params)
		case *bundlechanges.AddMachineChange:
			err = h.addMachine(change.Id(), change.Params)
		case *bundlechanges.AddRelationChange:
			err = h.addRelation(change.Id(), change.Params)
		case *bundlechanges.AddServiceChange:
			err = h.addService(change.Id(), change.Params)
		case *bundlechanges.AddUnitChange:
			err = h.addUnit(change.Id(), change.Params)
		case *bundlechanges.ExposeChange:
			err = h.exposeService(change.Id(), change.Params)
		case *bundlechanges.SetAnnotationsChange:
			err = h.setAnnotations(change.Id(), change.Params)
		default:
			return errors.Errorf("unknown change type: %T", change)
		}
		if err != nil {
			return errors.Annotate(err, "cannot deploy bundle")
		}
	}
	return nil
}

// bundleHandler provides helpers and the state required to deploy a bundle.
type bundleHandler struct {
	// changes holds the changes to be applied in order to deploy the bundle.
	changes []bundlechanges.Change
	// results collects data resulting from applying changes. Keys identify
	// changes, values result from interacting with the environment, and are
	// stored so that they can be potentially reused later, for instance for
	// resolving a dynamic placeholder included in a change. Specifically, the
	// following values are stored:
	// - when adding a charm, the fully resolved charm is stored;
	// - when deploying a service, the service name is stored;
	// - when adding a machine, the resulting machine id is stored;
	// - when adding a unit, either the id of the machine holding the unit or
	//   the unit name can be stored. The latter happens when a machine is
	//   implicitly created by adding a unit without a machine spec.
	results map[string]string
	// client is used to interact with the environment.
	client *api.Client
	// serviceClient is used to interact with services.
	serviceClient *apiservice.Client
	// serviceDeployer is used to deploy services.
	serviceDeployer *serviceDeployer
	// bundleStorage contains a mapping of service-specific storage
	// constraints. For each service, the storage constraints in the
	// map will replace or augment the storage constraints specified
	// in the bundle itself.
	bundleStorage map[string]map[string]storage.Constraints
	// csclient is used to retrieve charms from the charm store.
	csclient *csClient
	// repoPath is used to retrieve charms from a local repository.
	repoPath string
	// conf holds the environment configuration.
	conf *config.Config
	// log is used to output messages to the user, so that the user can keep
	// track of the bundle deployment progress.
	log deploymentLogger
	// data is the original bundle data that we want to deploy.
	data *charm.BundleData
	// unitStatus reflects the environment status and maps unit names to their
	// corresponding machine identifiers. This is kept updated by both change
	// handlers (addCharm, addService etc.) and by updateUnitStatus.
	unitStatus map[string]string
	// ignoredMachines and ignoredUnits map service names to whether a machine
	// or a unit creation has been skipped during the bundle deployment because
	// the current status of the environment does not require them to be added.
	ignoredMachines map[string]bool
	ignoredUnits    map[string]bool
	// watcher holds an environment mega-watcher used to keep the environment
	// status up to date.
	watcher *api.AllWatcher
}

// addCharm adds a charm to the environment.
func (h *bundleHandler) addCharm(id string, p bundlechanges.AddCharmParams) error {
	url, repo, err := resolveCharmStoreEntityURL(p.Charm, h.csclient.params, h.repoPath, h.conf)
	if err != nil {
		return errors.Annotatef(err, "cannot resolve URL %q", p.Charm)
	}
	if url.Series == "bundle" {
		return errors.Errorf("expected charm URL, got bundle URL %q", p.Charm)
	}
	url, err = addCharmFromURL(h.client, url, repo, h.csclient)
	if err != nil {
		return errors.Annotatef(err, "cannot add charm %q", p.Charm)
	}
	h.log.Infof("added charm %s", url)
	h.results[id] = url.String()
	return nil
}

// addService deploys or update a service with no units. Service options are
// also set or updated.
func (h *bundleHandler) addService(id string, p bundlechanges.AddServiceParams) error {
	h.results[id] = p.Service
	ch := resolve(p.Charm, h.results)
	// Handle service configuration.
	configYAML := ""
	if len(p.Options) > 0 {
		config, err := yaml.Marshal(map[string]map[string]interface{}{p.Service: p.Options})
		if err != nil {
			return errors.Annotatef(err, "cannot marshal options for service %q", p.Service)
		}
		configYAML = string(config)
	}
	// Handle service constraints.
	cons, err := constraints.Parse(p.Constraints)
	if err != nil {
		// This should never happen, as the bundle is already verified.
		return errors.Annotate(err, "invalid constraints for service")
	}
	storageConstraints := h.bundleStorage[p.Service]
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
	// Deploy the service.
	if err := h.serviceDeployer.serviceDeploy(serviceDeployParams{
		charmURL:    ch,
		serviceName: p.Service,
		configYAML:  configYAML,
		constraints: cons,
		storage:     storageConstraints,
	}); err == nil {
		h.log.Infof("service %s deployed (charm: %s)", p.Service, ch)
		return nil
	} else if !isErrServiceExists(err) {
		return errors.Annotatef(err, "cannot deploy service %q", p.Service)
	}
	// The service is already deployed in the environment: check that its
	// charm is compatible with the one declared in the bundle. If it is,
	// reuse the existing service or upgrade to a specified revision.
	// Exit with an error otherwise.
	if err := upgradeCharm(h.serviceClient, h.log, p.Service, ch); err != nil {
		return errors.Annotatef(err, "cannot upgrade service %q", p.Service)
	}
	// Update service configuration.
	if configYAML != "" {
		if err := h.serviceClient.ServiceUpdate(params.ServiceUpdate{
			ServiceName:  p.Service,
			SettingsYAML: configYAML,
		}); err != nil {
			// This should never happen as possible errors are already returned
			// by the ServiceDeploy call above.
			return errors.Annotatef(err, "cannot update options for service %q", p.Service)
		}
		h.log.Infof("configuration updated for service %s", p.Service)
	}
	// Update service constraints.
	if p.Constraints != "" {
		if err := h.client.SetServiceConstraints(p.Service, cons); err != nil {
			// This should never happen, as the bundle is already verified.
			return errors.Annotatef(err, "cannot update constraints for service %q", p.Service)
		}
		h.log.Infof("constraints applied for service %s", p.Service)
	}
	return nil
}

// addMachine creates a new top-level machine or container in the environment.
func (h *bundleHandler) addMachine(id string, p bundlechanges.AddMachineParams) error {
	services := h.servicesForMachineChange(id)
	// Note that we always have at least one service that justifies the
	// creation of this machine.
	msg := services[0] + " unit"
	svcLen := len(services)
	if svcLen != 1 {
		msg = strings.Join(services[:svcLen-1], ", ") + " and " + services[svcLen-1] + " units"
	}
	// Check whether the desired number of units already exist in the
	// environment, in which case avoid adding other machines to host those
	// service units.
	machine := h.chooseMachine(services...)
	if machine != "" {
		h.results[id] = machine
		notify := make([]string, 0, svcLen)
		for _, service := range services {
			if !h.ignoredMachines[service] {
				h.ignoredMachines[service] = true
				notify = append(notify, service)
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
	if p.ContainerType != "" {
		containerType, err := instance.ParseContainerType(p.ContainerType)
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
	r, err := h.client.AddMachines([]params.AddMachineParams{machineParams})
	if err != nil {
		return errors.Annotatef(err, "cannot create machine for holding %s", msg)
	}
	if r[0].Error != nil {
		return errors.Annotatef(r[0].Error, "cannot create machine for holding %s", msg)
	}
	machine = r[0].Machine
	if p.ContainerType == "" {
		h.log.Infof("created new machine %s for holding %s", machine, msg)
	} else if p.ParentId == "" {
		h.log.Infof("created %s container in new machine for holding %s", machine, msg)
	} else {
		h.log.Infof("created %s container in machine %s for holding %s", machine, machineParams.ParentId, msg)
	}
	h.results[id] = machine
	return nil
}

// addRelation creates a relationship between two services.
func (h *bundleHandler) addRelation(id string, p bundlechanges.AddRelationParams) error {
	ep1 := resolveRelation(p.Endpoint1, h.results)
	ep2 := resolveRelation(p.Endpoint2, h.results)
	_, err := h.client.AddRelation(ep1, ep2)
	if err == nil {
		// A new relation has been established.
		h.log.Infof("related %s and %s", ep1, ep2)
		return nil
	}
	if isErrRelationExists(err) {
		// The relation is already present in the environment.
		h.log.Infof("%s and %s are already related", ep1, ep2)
		return nil
	}
	return errors.Annotatef(err, "cannot add relation between %q and %q", ep1, ep2)
}

// addUnit adds a single unit to a service already present in the environment.
func (h *bundleHandler) addUnit(id string, p bundlechanges.AddUnitParams) error {
	service := resolve(p.Service, h.results)
	// Check whether the desired number of units already exist in the
	// environment, in which case avoid adding other units.
	machine := h.chooseMachine(service)
	if machine != "" {
		h.results[id] = machine
		if !h.ignoredUnits[service] {
			h.ignoredUnits[service] = true
			num := h.numUnitsForService(service)
			var msg string
			if num == 1 {
				msg = "1 unit already present"
			} else {
				msg = fmt.Sprintf("%d units already present", num)
			}
			h.log.Infof("avoid adding new units to service %s: %s", service, msg)
		}
		return nil
	}
	var machineSpec string
	if p.To != "" {
		var err error
		if machineSpec, err = h.resolveMachine(p.To); err != nil {
			// Should never happen.
			return errors.Annotatef(err, "cannot retrieve placement for %q unit", service)
		}
	}
	r, err := h.client.AddServiceUnits(service, 1, machineSpec)
	if err != nil {
		return errors.Annotatef(err, "cannot add unit for service %q", service)
	}
	unit := r[0]
	if machineSpec == "" {
		h.log.Infof("added %s unit to new machine", unit)
		// In this case, the unit name is stored in results instead of the
		// machine id, which is lazily evaluated later only if required.
		// This way we avoid waiting for watcher updates.
		h.results[id] = unit
	} else {
		h.log.Infof("added %s unit to machine %s", unit, machineSpec)
		h.results[id] = machineSpec
	}
	// Note that the machineSpec can be empty for now, resulting in a partially
	// incomplete unit status. That's ok as the missing info is provided later
	// when it is required.
	h.unitStatus[unit] = machineSpec
	return nil
}

// exposeService exposes a service.
func (h *bundleHandler) exposeService(id string, p bundlechanges.ExposeParams) error {
	service := resolve(p.Service, h.results)
	if err := h.client.ServiceExpose(service); err != nil {
		return errors.Annotatef(err, "cannot expose service %s", service)
	}
	h.log.Infof("service %s exposed", service)
	return nil
}

// setAnnotations sets annotations for a service or a machine.
func (h *bundleHandler) setAnnotations(id string, p bundlechanges.SetAnnotationsParams) error {
	eid := resolve(p.Id, h.results)
	var tag string
	switch p.EntityType {
	case bundlechanges.MachineType:
		tag = names.NewMachineTag(eid).String()
	case bundlechanges.ServiceType:
		tag = names.NewServiceTag(eid).String()
	default:
		return errors.Errorf("unexpected annotation entity type %q", p.EntityType)
	}
	if err := h.client.SetAnnotations(tag, p.Annotations); err != nil {
		return errors.Annotatef(err, "cannot set annotations for %s %q", p.EntityType, eid)
	}
	h.log.Infof("annotations set for %s %s", p.EntityType, eid)
	return nil
}

// servicesForMachineChange returns the names of the services for which an
// "addMachine" change is required, as adding machines is required to place
// units, and units belong to services.
// Receive the id of the "addMachine" change.
func (h *bundleHandler) servicesForMachineChange(changeId string) []string {
	services := make(map[string]bool, len(h.data.Services))
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
				for _, service := range h.servicesForMachineChange(change.Id()) {
					services[service] = true
				}
				continue mainloop
			case *bundlechanges.AddUnitChange:
				// We have found the "addUnit" change, which refers to a
				// service: now resolve the service holding the unit.
				service := resolve(change.Params.Service, h.results)
				services[service] = true
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
	for service := range services {
		results = append(results, service)
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
	numUnitsPerService := make(map[string]int, len(h.data.Services))
	// Collect the number of units and the corresponding machines for all
	// involved services.
	for unit, machine := range h.unitStatus {
		// Retrieve the top level machine.
		machine = strings.Split(machine, "/")[0]
		numUnitsPerMachine[machine]++
		svc, err := names.UnitService(unit)
		if err != nil {
			// Should never happen because the bundle logic has already checked
			// that unit names are well formed.
			panic(err)
		}
		for _, service := range services {
			if service != svc {
				continue
			}
			numUnitsPerService[service]++
			candidateMachines[machine] = true
		}
	}
	// If at least one service still requires units to be added, return an
	// empty machine in order to force new machine creation.
	for _, service := range services {
		if numUnitsPerService[service] < h.data.Services[service].NumUnits {
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
			return errors.Annotate(err, "cannot update environment status")
		}
		for _, d := range delta {
			switch entityInfo := d.Entity.(type) {
			case *multiwatcher.UnitInfo:
				h.unitStatus[entityInfo.Name] = entityInfo.MachineId
			}
		}
	case <-time.After(updateUnitStatusPeriod):
		return errors.New("timeout while trying to get new changes from the watcher")
	}
	return nil
}

// numUnitsForService return the number of units belonging to the given service
// currently in the environment.
func (h *bundleHandler) numUnitsForService(service string) (num int) {
	for unit := range h.unitStatus {
		svc, err := names.UnitService(unit)
		if err != nil {
			// Should never happen.
			panic(err)
		}
		if svc == service {
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

// resolveRelation returns the relation name resolving the included service
// placeholder.
func resolveRelation(e string, results map[string]string) string {
	parts := strings.SplitN(e, ":", 2)
	service := resolve(parts[0], results)
	if len(parts) == 1 {
		return service
	}
	return fmt.Sprintf("%s:%s", service, parts[1])
}

// resolve returns the real entity name for the bundle entity (for instance a
// service or a machine) with the given placeholder id.
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

// upgradeCharm upgrades the charm for the given service to the given charm id.
// If the service is already deployed using the given charm id, do nothing.
// This function returns an error if the existing charm and the target one are
// incompatible, meaning an upgrade from one to the other is not allowed.
func upgradeCharm(client *apiservice.Client, log deploymentLogger, service, id string) error {
	existing, err := client.ServiceGetCharmURL(service)
	if err != nil {
		return errors.Annotatef(err, "cannot retrieve info for service %q", service)
	}
	if existing.String() == id {
		log.Infof("reusing service %s (charm: %s)", service, id)
		return nil
	}
	url, err := charm.ParseURL(id)
	if err != nil {
		return errors.Annotatef(err, "cannot parse charm URL %q", id)
	}
	if url.WithRevision(-1).Path() != existing.WithRevision(-1).Path() {
		return errors.Errorf("bundle charm %q is incompatible with existing charm %q", id, existing)
	}
	if err := client.ServiceSetCharm(service, id, false); err != nil {
		return errors.Annotatef(err, "cannot upgrade charm to %q", id)
	}
	log.Infof("upgraded charm for existing service %s (from %s to %s)", service, existing, id)
	return nil
}

// isErrServiceExists reports whether the given error has been generated
// from trying to deploy a service that already exists.
func isErrServiceExists(err error) bool {
	// TODO frankban (bug 1495952): do this check using the cause rather than
	// the string when a specific cause is available.
	return strings.HasSuffix(err.Error(), "service already exists")
}

// isErrRelationExists reports whether the given error has been generated
// from trying to create an already established relation.
func isErrRelationExists(err error) bool {
	// TODO frankban (bug 1495952): do this check using the cause rather than
	// the string when a specific cause is available.
	return strings.HasSuffix(err.Error(), "relation already exists")
}
