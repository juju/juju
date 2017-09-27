// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package application

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/bundlechanges"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	"github.com/kr/pretty"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable"
	csparams "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/juju/charmstore.v4/config"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/application"
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
	ctx *cmd.Context,
	bundleStorage map[string]map[string]storage.Constraints,
	dryRun bool,
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
		// Process includes in the bundle data.
		if err := processBundleIncludes(ctx.Dir, data); err != nil {
			return nil, errors.Annotate(err, "unable to process includes")
		}
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

	// TODO: move bundle parsing and checking into the handler.
	h := makeBundleHandler(dryRun, bundleDir, channel, api, ctx, data, bundleStorage)
	if err := h.makeModel(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := h.getChanges(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := h.handleChanges(); err != nil {
		return nil, errors.Trace(err)
	}
	return h.csMacs, nil

}

// bundleHandler provides helpers and the state required to deploy a bundle.
type bundleHandler struct {
	dryRun bool

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

	// ctx is the command context, which is used to output messages to the
	// user, so that the user can keep track of the bundle deployment
	// progress.
	ctx *cmd.Context

	// data is the original bundle data that we want to deploy.
	data *charm.BundleData

	// unitStatus reflects the environment status and maps unit names to their
	// corresponding machine identifiers. This is kept updated by both change
	// handlers (addCharm, addService etc.) and by updateUnitStatus.
	unitStatus map[string]string

	modelConfig *config.Config

	model *bundlechanges.Model

	csMacs   map[*charm.URL]*macaroon.Macaroon
	channels map[*charm.URL]csparams.Channel

	// watcher holds an environment mega-watcher used to keep the environment
	// status up to date.
	watcher allWatcher

	// warnedLXC indicates whether or not we have warned the user that the
	// bundle they're deploying uses lxc containers, which will be treated as
	// LXD.  This flag keeps us from writing the warning more than once per
	// bundle.
	warnedLXC bool
}

func makeBundleHandler(
	dryRun bool,
	bundleDir string,
	channel string,
	api DeployAPI,
	ctx *cmd.Context,
	data *charm.BundleData,
	bundleStorage map[string]map[string]storage.Constraints,
) *bundleHandler {
	return &bundleHandler{
		dryRun:        dryRun,
		bundleDir:     bundleDir,
		results:       make(map[string]string, numChanges),
		channel:       channel,
		api:           apiRoot,
		bundleStorage: bundleStorage,
		ctx:           ctx,
		data:          data,
		unitStatus:    make(map[string]string), // XXX maybe kill
		csMacs:        make(map[*charm.URL]*macaroon.Macaroon),
		channels:      make(map[*charm.URL]csparams.Channel),
	}
}

func (h *bundleHandler) makeModel() error {

	// Initialize the unit status.
	status, err := h.api.Status(nil)
	if err != nil {
		return errors.Annotate(err, "cannot get model status")
	}

	h.model, err = buildModelRepresentation(status, h.api)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("model: %s", pretty.Sprint(model))

	// XXX: We should be able not need unitStatus if we use the model.
	for _, serviceData := range status.Applications {
		for unit, unitData := range serviceData.Units {
			h.unitStatus[unit] = unitData.Machine
		}
	}

	h.modelConfig, err = getModelConfig(h.api)
	if err != nil {
		return err
	}
	return nil
}

func (h *bundleHandler) getChanges() error {
	var err error
	h.changes, err = bundlechanges.FromData(h.data, h.model)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (h *bundleHandler) handleChanges() error {
	var err error
	// Instantiate a watcher used to follow the deployment progress.
	h.watcher, err = h.api.WatchAll()
	if err != nil {
		return nil, errors.Annotate(err, "cannot watch model")
	}
	defer h.watcher.Stop()

	// Deploy the bundle.
	for _, change := range changes {
		ctx.Infof(change.Description())
		switch change := change.(type) {
		case *bundlechanges.AddCharmChange:
			cURL, err := h.addCharm(change.Id(), change.Params)
		case *bundlechanges.AddMachineChange:
			err = h.addMachine(change.Id(), change.Params)
		case *bundlechanges.AddRelationChange:
			err = h.addRelation(change.Id(), change.Params)
		case *bundlechanges.AddApplicationChange:
			err = h.addApplication(change.Id(), change.Params)
		case *bundlechanges.AddUnitChange:
			err = h.addUnit(change.Id(), change.Params)
		case *bundlechanges.ExposeChange:
			err = h.exposeService(change.Id(), change.Params)
		case *bundlechanges.SetAnnotationsChange:
			err = h.setAnnotations(change.Id(), change.Params)
		case *bundlechanges.UpgradeCharmChange:
			err = h.upgradeCharm(change.Id(), change.Params)
		case *bundlechanges.SetOptionsChange:
			err = h.setOptions(change.Id(), change.Params)
		case *bundlechanges.SetConstraintsChange:
			err = h.setConstraints(change.Id(), change.Params)
		default:
			return nil, errors.Errorf("unknown change type: %T", change)
		}
		if err != nil {
			return nil, errors.Annotate(err, "cannot deploy bundle")
		}
	}
	return nil
}

// addCharm adds a charm to the environment.
func (h *bundleHandler) addCharm(id string, p bundlechanges.AddCharmParams) (*charm.URL, error) {
	var noChannel csparams.Channel
	if h.dryRun {
		return nil, nil
	}
	// First attempt to interpret as a local path.
	if strings.HasPrefix(p.Charm, ".") || filepath.IsAbs(p.Charm) {
		charmPath := p.Charm
		if !filepath.IsAbs(charmPath) {
			charmPath = filepath.Join(h.bundleDir, charmPath)
		}

		series := p.Series
		if series == "" {
			series = h.data.Series
		}
		ch, curl, err := charmrepo.NewCharmAtPath(charmPath, series)
		if err != nil && !os.IsNotExist(err) {
			return nil, errors.Annotatef(err, "cannot deploy local charm at %q", charmPath)
		}
		if err == nil {
			if curl, err = h.api.AddLocalCharm(curl, ch); err != nil {
				return nil, err
			}
			logger.Debugf("added charm %s", curl)
			h.results[id] = curl.String()
			return curl, nil
		}
	}

	// Not a local charm, so grab from the store.
	ch, err := charm.ParseURL(p.Charm)
	if err != nil {
		return nil, errors.Trace(err)
	}

	url, channel, _, err := h.api.Resolve(h.modelConfig, ch)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot resolve URL %q", p.Charm)
	}
	if url.Series == "bundle" {
		return nil, errors.Errorf("expected charm URL, got bundle URL %q", p.Charm)
	}
	var csMac *macaroon.Macaroon
	url, csMac, err = addCharmFromURL(h.api, url, channel)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot add charm %q", p.Charm)
	}
	logger.Debugf("added charm %s", url)
	h.results[id] = url.String()
	h.csMacs[url] = csMac
	h.channels[url] = csMac
	return url, nil
}

func (h *bundleHandler) makeResourceMap(storeResources map[string]int, localResources map[string]string) map[string]string {
	resources := make(map[string]string)
	for resName, path := range localResources {
		if !filepath.IsAbs(path) {
			path = filepath.Clean(filepath.Join(h.bundleDir, path))
		}
		resources[resName] = path
	}
	for resName, revision := range storeResources {
		resources[resName] = fmt.Sprint(revision)
	}
	return resources
}

// addApplication deploys an application with no units.
func (h *bundleHandler) addApplication(id string, p bundlechanges.AddApplicationParams) error {
	// TODO: add verbose output for details
	if h.dryRun {
		return nil
	}
	cURL, err := charm.ParseURL(resolve(p.Charm, h.results))
	if err != nil {
		return errors.Trace(err)
	}

	chID := charmstore.CharmID{
		URL:     cURL,
		Channel: h.channels[cURL],
	}
	csMac := csMacs[cURL]

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
	resources := h.makeResourceMap(p.Resources, p.LocalResources)
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
		h.api,
	)
	if err != nil {
		return errors.Trace(err)
	}

	// Figure out what series we need to deploy with.
	supportedSeries := charmInfo.Meta.Series
	if len(supportedSeries) == 0 && chID.URL.Series != "" {
		supportedSeries = []string{chID.URL.Series}
	}
	selector := seriesSelector{
		seriesFlag:      p.Series,
		charmURLSeries:  chID.URL.Series,
		supportedSeries: supportedSeries,
		conf:            h.modelConfig,
		fromBundle:      true,
	}
	series, err := selector.charmSeries()
	if err != nil {
		return errors.Trace(err)
	}

	// Deploy the application.
	if err := api.Deploy(application.DeployArgs{
		CharmID:          chID,
		Cons:             cons,
		ApplicationName:  p.Application,
		Series:           series,
		ConfigYAML:       configYAML,
		Storage:          storageConstraints,
		Resources:        resNames2IDs,
		EndpointBindings: p.EndpointBindings,
	}); err != nil {
		return errors.Annotatef(err, "cannot deploy application %q", p.Application)
	}

	for resName := range resNames2IDs {
		h.ctx.Verbosef("  added resource %s", resName)
	}
	return nil
}

// addMachine creates a new top-level machine or container in the environment.
func (h *bundleHandler) addMachine(id string, p bundlechanges.AddMachineParams) error {
	var verbose []string
	if p.Series != "" {
		verbose = append(verbose, fmt.Sprintf("with series %q", p.Series))
	}
	if p.Constraints != "" {
		verbose = append(verbose, fmt.Sprintf("with constraints %q", p.Constraints))
	}
	if output := strings.Join(verbose, ", "); output != "" {
		h.ctx.Verbosef("  %s", output)
	}
	if h.dryRun {
		return nil
	}

	apps := h.applicationsForMachineChange(id)
	// Note that we always have at least one application that justifies the
	// creation of this machine.
	msg := apps[0] + " unit"

	if count := len(apps); count != 1 {
		msg = strings.Join(apps[:count-1], ", ") + " and " + apps[count-1] + " units"
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
		// TODO(thumper): move the warning and translation into the bundle reading code.

		// for backwards compatibility with 1.x bundles, we treat lxc
		// placement directives as lxd.
		if ct == "lxc" {
			if !h.warnedLXC {
				h.ctx.Infof("Bundle has one or more containers specified as lxc. lxc containers are deprecated in Juju 2.0. lxd containers will be deployed instead.")
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
	if h.dryRun {
		return nil
	}
	ep1 := resolveRelation(p.Endpoint1, h.results)
	ep2 := resolveRelation(p.Endpoint2, h.results)
	// TODO(wallyworld) - CMR support in bundles
	_, err := h.api.AddRelation([]string{ep1, ep2}, nil)
	if err != nil {
		return errors.Annotatef(err, "cannot add relation between %q and %q", ep1, ep2)

	}
	return nil
}

// addUnit adds a single unit to an application already present in the environment.
func (h *bundleHandler) addUnit(id string, p bundlechanges.AddUnitParams) error {
	if h.dryRun {
		return nil
	}
	applicationName := resolve(p.Application, h.results)
	var placementArg []*instance.Placement
	targetMachine := p.To
	if targetMachine != "" {
		// The placement maybe "container:machine"
		container := ""
		if parts := strings.Split(targetMachine, ":"); len(parts) > 1 {
			container = parts[0]
			targetMachine = parts[1]
		}
		targetMachine, err := h.resolveMachine(targetMachine)
		if err != nil {
			// Should never happen.
			return errors.Annotatef(err, "cannot retrieve placement for %q unit", applicationName)
		}
		directive := targetMachine
		if container != "" {
			directive = container + ":" + directive
		}
		placement, err := parsePlacement(directive)
		if err != nil {
			return errors.Errorf("invalid --to parameter %q", directive)
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
	if targetMachine == "" {
		logger.Debugf("added %s unit to new machine", unit)
		// In this case, the unit name is stored in results instead of the
		// machine id, which is lazily evaluated later only if required.
		// This way we avoid waiting for watcher updates.
		h.results[id] = unit
	} else {
		logger.Debugf("added %s unit to new machine", unit)
		h.results[id] = targetMachine
	}

	// Note that the targetMachine can be empty for now, resulting in a partially
	// incomplete unit status. That's ok as the missing info is provided later
	// when it is required.
	h.unitStatus[unit] = targetMachine
	return nil
}

// upgradeCharm will get the application to use the new charm.
func (h *bundleHandler) upgradeCharm(id string, p bundlechanges.UpgradeCharmParams) error {
	if h.dryRun {
		return nil
	}

	cURL, err := charm.ParseURL(resolve(p.Charm, h.results))
	if err != nil {
		return errors.Trace(err)
	}

	chID := charmstore.CharmID{
		URL:     cURL,
		Channel: h.channels[cURL],
	}
	csMac := csMacs[cURL]

	resources := h.makeResourceMap(p.Resources, p.LocalResources)

	// charmsClient := charms.NewClient(api)
	resourceLister, err := resourceadapters.NewAPIClient(h.api)
	if err != nil {
		return errors.Trace(err)
	}
	filtered, err := getUpgradeResources(h.api, resourceLister, p.Application, cURL, resources)
	if err != nil {
		return errors.Trace(err)
	}
	var resNames2IDs map[string]string
	if len(filtered) != 0 {
		resNames2IDs, err = resourceadapters.DeployResources(
			p.Application,
			chID,
			csMac,
			resources,
			filtered,
			h.api,
		)
		if err != nil {
			return errors.Trace(err)
		}
	}
	cfg := application.SetCharmConfig{
		ApplicationName: p.Application,
		CharmID:         chID,
		ResourceIDs:     resNames2IDs,
	}
	if err := h.api.SetCharm(cfg); err != nil {
		return errors.Annotatef(err, "cannot upgrade charm to %q", id)
	}
	for resName := range resNames2IDs {
		h.ctx.Verbosef("  added resource %s", resName)
	}

	return nil
}

// setOptions updates application configuration settings.
func (h *bundleHandler) setOptions(id string, p bundlechanges.SetOptionsParams) error {
	h.ctx.Verbosef("  setting options:")
	for key, value := range p.Options {
		switch value.(type) {
		case string:
			h.ctx.Verbosef("    %s: %q", key, value)
		default:
			h.ctx.Verbosef("    %s: %v", key, value)
		}
	}
	if h.dryRun {
		return nil
	}

	// We know that there wouldn't be any setOptions if there were no options.
	config, err := yaml.Marshal(map[string]map[string]interface{}{p.Application: p.Options})
	if err != nil {
		return errors.Annotatef(err, "cannot marshal options for application %q", p.Application)
	}

	if err := h.api.Update(params.ApplicationUpdate{
		ApplicationName: p.Application,
		SettingsYAML:    string(config),
	}); err != nil {
		return errors.Annotatef(err, "cannot update options for application %q", p.Application)
	}

	return nil
}

// setConstraints updates application constraints.
func (h *bundleHandler) setConstraints(id string, p bundlechanges.SetConstraintsParams) error {
	if h.dryRun {
		return nil
	}

	if err := h.api.SetConstraints(p.Application, p.Constraints); err != nil {
		// This should never happen, as the bundle is already verified.
		return errors.Annotatef(err, "cannot update constraints for application %q", p.Application)
	}

	return nil
}

// exposeService exposes an application.
func (h *bundleHandler) exposeService(id string, p bundlechanges.ExposeParams) error {
	if h.dryRun {
		return nil
	}
	application := resolve(p.Application, h.results)
	if err := h.api.Expose(application); err != nil {
		return errors.Annotatef(err, "cannot expose application %s", application)
	}
	return nil
}

// setAnnotations sets annotations for an application or a machine.
func (h *bundleHandler) setAnnotations(id string, p bundlechanges.SetAnnotationsParams) error {
	h.ctx.Verbosef("  setting annotations:")
	for key, value := range p.Annotations {
		h.ctx.Verbosef("    %s: %q", key, value)
	}
	if h.dryRun {
		return nil
	}
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
	return nil
}

// applicationsForMachineChange returns the names of the applications for which an
// "addMachine" change is required, as adding machines is required to place
// units, and units belong to services.
// Receive the id of the "addMachine" change.
func (h *bundleHandler) applicationsForMachineChange(changeId string) []string {
	applications := set.NewStrings()
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
				for _, application := range h.applicationsForMachineChange(change.Id()) {
					applications.Add(application)
				}
				continue mainloop
			case *bundlechanges.AddUnitChange:
				// We have found the "addUnit" change, which refers to a
				// application: now resolve the application holding the unit.
				application := resolve(change.Params.Application, h.results)
				applications.Add(application)
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
	return applications.SortedValues()
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
//
// Now that the bundlechanges library understands the existing model, if the
// entity already existed in the model, the placehodler value is the actual
// entity from the model, and in these situations the placeholder value doesn't
// start with the '$'.
func resolve(placeholder string, results map[string]string) string {
	if !strings.HasPrefix(placeholder, "$") {
		return placeholder
	}
	id := placeholder[1:]
	return results[id]
}

func processBundleIncludes(baseDir string, data *charm.BundleData) error {
	for app, appData := range data.Applications {
		// A bundle isn't valid if there are no applications, and applications must
		// specify a charm at least, so we know appData must be non-nil.
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
		if machineData == nil {
			continue
		}
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

func buildModelRepresentation(status *params.FullStatus, apiRoot DeployAPI) (*bundlechanges.Model, error) {
	var (
		annotationTags []string
		appNames       []string
	)

	machines := make(map[string]*bundlechanges.Machine)
	for id := range status.Machines {
		machines[id] = &bundlechanges.Machine{ID: id}
		annotationTags = append(annotationTags, names.NewMachineTag(id).String())
	}
	applications := make(map[string]*bundlechanges.Application)
	for name, appStatus := range status.Applications {
		application := &bundlechanges.Application{
			Name:    name,
			Charm:   appStatus.Charm,
			Exposed: appStatus.Exposed,
		}
		for unitName, unit := range appStatus.Units {
			application.Units = append(application.Units, bundlechanges.Unit{
				Name:    unitName,
				Machine: unit.Machine,
			})
		}
		applications[name] = application
		annotationTags = append(annotationTags, names.NewApplicationTag(name).String())
		appNames = append(appNames, name)
	}
	model := &bundlechanges.Model{
		Applications: applications,
		Machines:     machines,
	}
	for _, relation := range status.Relations {
		if relation.Scope == "peer" {
			continue
		}
		if count := len(relation.Endpoints); count != 2 {
			return nil, errors.Errorf("unexpected size of relations, wanted 2 got %d", count)
		}
		model.Relations = append(model.Relations, bundlechanges.Relation{
			App1:      relation.Endpoints[0].ApplicationName,
			Endpoint1: relation.Endpoints[0].Name,
			App2:      relation.Endpoints[1].ApplicationName,
			Endpoint2: relation.Endpoints[1].Name,
		})
	}
	// Get all the annotations.
	annotations, err := apiRoot.GetAnnotations(annotationTags)
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
			model.Applications[tag.Id()].Annotations = result.Annotations
		case names.MachineTagKind:
			model.Machines[tag.Id()].Annotations = result.Annotations
		default:
			return nil, errors.Errorf("unexpected tag kind for annotations: %q", kind)
		}
	}
	// Now get all the application config.
	configValues, err := apiRoot.GetConfig(appNames...)
	if err != nil {
		return nil, errors.Annotate(err, "getting application options")
	}
	for i, config := range configValues {
		model.Applications[appNames[i]].Options = config
	}
	// Lastly get all the application constraints.
	constraintValues, err := apiRoot.GetConstraints(appNames...)
	if err != nil {
		return nil, errors.Annotate(err, "getting application constraints")
	}
	for i, value := range constraintValues {
		model.Applications[appNames[i]].Constraints = value.String()
	}

	model.ConstraintsEqual = func(a, b string) bool {
		// Since the constraints have already been validated, we don't
		// even bother checking the error response here.
		ac, _ := constraints.Parse(a)
		bc, _ := constraints.Parse(b)
		return ac == bc
	}

	return model, nil
}
