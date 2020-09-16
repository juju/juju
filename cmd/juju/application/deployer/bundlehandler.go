// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/juju/bundlechanges/v3"
	"github.com/juju/charm/v8"
	"github.com/juju/charm/v8/resource"
	"github.com/juju/charmrepo/v6"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	jujuclock "github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/kr/pretty"
	"gopkg.in/macaroon.v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/application"
	commoncharm "github.com/juju/juju/api/common/charm"
	app "github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	appbundle "github.com/juju/juju/cmd/juju/application/bundle"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/modelcmd"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/resource/resourceadapters"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/storage"
)

// deploymentLogger is used to notify clients about the bundle deployment
// progress.
type deploymentLogger interface {
	// Infof formats and logs the given message.
	Infof(string, ...interface{})
}

type bundleDeploySpec struct {
	ctx        *cmd.Context
	filesystem modelcmd.Filesystem

	dryRun bool
	force  bool
	trust  bool

	bundleDataSource  charm.BundleDataSource
	bundleDir         string
	bundleURL         *charm.URL
	bundleOverlayFile []string
	origin            commoncharm.Origin

	deployAPI            DeployerAPI
	bundleResolver       Resolver
	authorizer           store.MacaroonGetter
	getConsumeDetailsAPI func(*charm.OfferURL) (ConsumeDetails, error)
	deployResources      resourceadapters.DeployResourcesFunc

	useExistingMachines bool
	bundleMachines      map[string]string
	bundleStorage       map[string]map[string]storage.Constraints
	bundleDevices       map[string]map[string]devices.Constraints

	targetModelName string
	targetModelUUID string
	controllerName  string
	accountUser     string
}

// deployBundle deploys the given bundle data using the given API client and
// charm store client. The deployment is not transactional, and its progress is
// notified using the given deployment logger.
//
// Note: deployBundle expects that spec.BundleData points to a verified bundle
// that has all required external overlays applied.
func bundleDeploy(bundleData *charm.BundleData, spec bundleDeploySpec) (map[*charm.URL]*macaroon.Macaroon, error) {
	// TODO: move bundle parsing and checking into the handler.
	h := makeBundleHandler(bundleData, spec)
	if err := h.makeModel(spec.useExistingMachines, spec.bundleMachines); err != nil {
		return nil, errors.Trace(err)
	}
	if err := h.resolveCharmsAndEndpoints(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := h.getChanges(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := h.handleChanges(); err != nil {
		return nil, errors.Trace(err)
	}
	return h.macaroons, nil
}

// bundleHandler provides helpers and the state required to deploy a bundle.
type bundleHandler struct {
	dryRun bool
	force  bool
	trust  bool

	clock jujuclock.Clock

	// bundleDir is the path where the bundle file is located for local bundles.
	bundleDir string
	// changes holds the changes to be applied in order to deploy the bundle.
	changes []bundlechanges.Change

	// applications are all the applications defined in the bundle.
	// Used primarily for iterating over sorted values.
	applications set.Strings

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

	// origin identifies the default channel to use for the bundle.
	origin commoncharm.Origin

	// deployAPI is used to interact with the environment.
	deployAPI            DeployerAPI
	bundleResolver       Resolver
	authorizer           store.MacaroonGetter
	getConsumeDetailsAPI func(*charm.OfferURL) (ConsumeDetails, error)
	deployResources      resourceadapters.DeployResourcesFunc

	// bundleStorage contains a mapping of application-specific storage
	// constraints. For each application, the storage constraints in the
	// map will replace or augment the storage constraints specified
	// in the bundle itself.
	bundleStorage map[string]map[string]storage.Constraints

	// bundleDevices contains a mapping of application-specific device
	// constraints. For each application, the device constraints in the
	// map will replace or augment the device constraints specified
	// in the bundle itself.
	bundleDevices map[string]map[string]devices.Constraints

	// ctx is the command context, which is used to output messages to the
	// user, so that the user can keep track of the bundle deployment
	// progress.
	ctx *cmd.Context

	// filesystem provides access to the filesystem.
	filesystem modelcmd.Filesystem

	// data is the original bundle data that we want to deploy.
	data *charm.BundleData

	// bundleURL is the URL of the bundle when deploying a bundle from the
	// charmstore, nil otherwise.
	bundleURL *charm.URL

	// unitStatus reflects the environment status and maps unit names to their
	// corresponding machine identifiers. This is kept updated by both change
	// handlers (addCharm, addApplication etc.) and by updateUnitStatus.
	unitStatus map[string]string

	modelConfig *config.Config

	model *bundlechanges.Model

	macaroons map[*charm.URL]*macaroon.Macaroon
	origins   map[*charm.URL]commoncharm.Origin

	// watcher holds an environment mega-watcher used to keep the environment
	// status up to date.
	watcher api.AllWatch

	// warnedLXC indicates whether or not we have warned the user that the
	// bundle they're deploying uses lxc containers, which will be treated as
	// LXD.  This flag keeps us from writing the warning more than once per
	// bundle.
	warnedLXC bool

	// The name and UUID of the model where the bundle is about to be deployed.
	targetModelName string
	targetModelUUID string

	// Controller name required for consuming a offer when deploying a bundle.
	controllerName string

	// accountUser holds the user of the account associated with the
	// current controller.
	accountUser string
}

func makeBundleHandler(bundleData *charm.BundleData, spec bundleDeploySpec) *bundleHandler {
	applications := set.NewStrings()
	for name := range bundleData.Applications {
		applications.Add(name)
	}
	return &bundleHandler{
		// TODO (stickupkid): pass this through from the constructor.
		clock: jujuclock.WallClock,

		dryRun:               spec.dryRun,
		force:                spec.force,
		trust:                spec.trust,
		bundleDir:            spec.bundleDir,
		applications:         applications,
		results:              make(map[string]string),
		origin:               spec.origin,
		deployAPI:            spec.deployAPI,
		bundleResolver:       spec.bundleResolver,
		authorizer:           spec.authorizer,
		getConsumeDetailsAPI: spec.getConsumeDetailsAPI,
		deployResources:      spec.deployResources,
		bundleStorage:        spec.bundleStorage,
		bundleDevices:        spec.bundleDevices,
		ctx:                  spec.ctx,
		filesystem:           spec.filesystem,
		data:                 bundleData,
		bundleURL:            spec.bundleURL,
		unitStatus:           make(map[string]string),
		macaroons:            make(map[*charm.URL]*macaroon.Macaroon),
		origins:              make(map[*charm.URL]commoncharm.Origin),

		targetModelName: spec.targetModelName,
		targetModelUUID: spec.targetModelUUID,
		controllerName:  spec.controllerName,
		accountUser:     spec.accountUser,
	}
}

func (h *bundleHandler) makeModel(
	useExistingMachines bool,
	bundleMachines map[string]string,
) error {
	// Initialize the unit status.
	status, err := h.deployAPI.Status(nil)
	if err != nil {
		return errors.Annotate(err, "cannot get model status")
	}

	h.model, err = appbundle.BuildModelRepresentation(status, h.deployAPI, useExistingMachines, bundleMachines)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("model: %s", pretty.Sprint(h.model))

	for _, appData := range status.Applications {
		for unit, unitData := range appData.Units {
			h.unitStatus[unit] = unitData.Machine
		}
	}

	h.modelConfig, err = getModelConfig(h.deployAPI)
	if err != nil {
		return err
	}
	return nil
}

// resolveCharmsAndEndpoints will go through the bundle and
// resolve the charm URLs. From the model the charm names are
// fully qualified, meaning they have a source and revision id.
// Effectively the logic this method follows is:
//   * if the bundle specifies a local charm, and the application
//     exists already, then override the charm URL in the bundle
//     spec to match the charm name from the model. We don't
//     upgrade local charms as part of a bundle deploy.
//   * the charm URL is resolved and the bundle spec is replaced
//     with the fully resolved charm URL - i.e.: with rev id.
//   * check all endpoints, and if any of them have implicit endpoints,
//     and if they do, resolve the implicitness in order to compare
//     with relations in the model.
func (h *bundleHandler) resolveCharmsAndEndpoints() error {
	deployedApps := set.NewStrings()

	for _, name := range h.applications.SortedValues() {
		spec := h.data.Applications[name]
		app := h.model.GetApplication(name)
		if app != nil {
			deployedApps.Add(name)

			if h.isLocalCharm(spec.Charm) {
				logger.Debugf("%s exists in model uses a local charm, replacing with %q", name, app.Charm)
				// Replace with charm from model
				spec.Charm = app.Charm
				continue
			}
			// If the charm matches, don't bother resolving.
			if spec.Charm == app.Charm {
				continue
			}
		}

		if h.isLocalCharm(spec.Charm) {
			continue
		}

		ch, err := resolveCharmURL(spec.Charm)
		if err != nil {
			return errors.Trace(err)
		}

		var via string
		switch {
		case charm.CharmHub.Matches(ch.Schema):
			via = "via charmhub: "
		case charm.CharmStore.Matches(ch.Schema):
			via = "via charmstore: "
		case charm.Local.Matches(ch.Schema):
			via = "via local filesystem: "
		default:
			via = ": "
		}

		var fromChannel string
		var channel corecharm.Channel
		if spec.Channel != "" {
			fromChannel = fmt.Sprintf(" from channel: %s", spec.Channel)
			channel, err = corecharm.ParseChannel(spec.Channel)
			if err != nil {
				return errors.Trace(err)
			}
		}
		h.ctx.Infof("Resolving charm %s%s%s", via, ch.FullPath(), fromChannel)
		origin, err := utils.DeduceOrigin(ch, channel)
		if err != nil {
			return errors.Trace(err)
		}
		url, _, _, err := h.bundleResolver.ResolveCharm(ch, origin)
		if err != nil {
			return errors.Annotatef(err, "cannot resolve URL %q", spec.Charm)
		}
		if url.Series == "bundle" {
			return errors.Errorf("expected charm URL, got bundle URL %q", spec.Charm)
		}

		spec.Charm = url.String()
	}

	// TODO(thumper): the InferEndpoints code is deeply wedged in the
	// persistence layer and needs to be extracted. This is a multi-day
	// effort, so for now the bundle handling is doing no implicit endpoint
	// handling.
	return nil
}

func (h *bundleHandler) getChanges() error {
	bundleURL := ""
	if h.bundleURL != nil {
		bundleURL = h.bundleURL.String()
	}
	// TODO(stickupkid): The following should use the new
	// Bundle.getChangesMapArgs, with the fallback to Bundle.getChanges and
	// with controllers without a Bundle facade should use the bundlechanges
	// library.
	// The real sticking point is that all the code below uses the bundlechanges
	// library directly and that's just not cricket. Instead there should be
	// some normalisation for all entry points, either that be a API or a
	// library. Then walk over that normalised AST of changes to execute the
	// changes required. That unfortunately is some re-factoring and would take
	// some time to do that, hence why we're still using the library directly
	// unlike other clients.
	cfg := bundlechanges.ChangesConfig{
		Bundle:    h.data,
		BundleURL: bundleURL,
		Model:     h.model,
		Logger:    logger,
	}
	logger.Tracef("bundlechanges.ChangesConfig.Bundle %s", pretty.Sprint(cfg.Bundle))
	logger.Tracef("bundlechanges.ChangesConfig.BundleURL %s", pretty.Sprint(cfg.BundleURL))
	logger.Tracef("bundlechanges.ChangesConfig.Model %s", pretty.Sprint(cfg.Model))
	changes, err := bundlechanges.FromData(cfg)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Tracef("changes %s", pretty.Sprint(changes))
	h.changes = changes
	return nil
}

func (h *bundleHandler) handleChanges() error {
	var err error
	// Instantiate a watcher used to follow the deployment progress.
	h.watcher, err = h.deployAPI.WatchAll()
	if err != nil {
		return errors.Annotate(err, "cannot watch model")
	}
	defer func() { _ = h.watcher.Stop() }()

	if len(h.changes) == 0 {
		h.ctx.Infof("No changes to apply.")
		return nil
	}

	if h.dryRun {
		fmt.Fprintf(h.ctx.Stdout, "Changes to deploy bundle:\n")
	} else {
		fmt.Fprintf(h.ctx.Stdout, "Executing changes:\n")
	}

	// Deploy the bundle.
	for i, change := range h.changes {
		fmt.Fprint(h.ctx.Stdout, fmtChange(change))
		logger.Tracef("%d: change %s", i, pretty.Sprint(change))
		switch change := change.(type) {
		case *bundlechanges.AddCharmChange:
			err = h.addCharm(change)
		case *bundlechanges.AddMachineChange:
			err = h.addMachine(change)
		case *bundlechanges.AddRelationChange:
			err = h.addRelation(change)
		case *bundlechanges.AddApplicationChange:
			err = h.addApplication(change)
		case *bundlechanges.ScaleChange:
			err = h.scaleApplication(change)
		case *bundlechanges.AddUnitChange:
			err = h.addUnit(change)
		case *bundlechanges.ExposeChange:
			err = h.exposeApplication(change)
		case *bundlechanges.SetAnnotationsChange:
			err = h.setAnnotations(change)
		case *bundlechanges.UpgradeCharmChange:
			err = h.upgradeCharm(change)
		case *bundlechanges.SetOptionsChange:
			err = h.setOptions(change)
		case *bundlechanges.SetConstraintsChange:
			err = h.setConstraints(change)
		case *bundlechanges.CreateOfferChange:
			err = h.createOffer(change)
		case *bundlechanges.ConsumeOfferChange:
			err = h.consumeOffer(change)
		case *bundlechanges.GrantOfferAccessChange:
			err = h.grantOfferAccess(change)
		default:
			return errors.Errorf("unknown change type: %T", change)
		}
		if err != nil {
			return errors.Trace(err)
		}
	}

	if !h.dryRun {
		h.ctx.Infof("Deploy of bundle completed.")
	}

	return nil
}

func fmtChange(ch bundlechanges.Change) string {
	var buf bytes.Buffer
	for _, desc := range ch.Description() {
		fmt.Fprintf(&buf, "- %s\n", desc)
	}
	return buf.String()
}

func (h *bundleHandler) isLocalCharm(name string) bool {
	return strings.HasPrefix(name, ".") || filepath.IsAbs(name)
}

// addCharm adds a charm to the environment.
func (h *bundleHandler) addCharm(change *bundlechanges.AddCharmChange) error {
	if h.dryRun {
		return nil
	}
	id := change.Id()
	chParms := change.Params
	// First attempt to interpret as a local path.
	if h.isLocalCharm(chParms.Charm) {
		charmPath := chParms.Charm
		if !filepath.IsAbs(charmPath) {
			charmPath = filepath.Join(h.bundleDir, charmPath)
		}

		series := chParms.Series
		if series == "" {
			series = h.data.Series
		}
		ch, curl, err := charmrepo.NewCharmAtPathForceSeries(charmPath, series, h.force)
		if err != nil && !os.IsNotExist(err) {
			return errors.Annotatef(err, "cannot deploy local charm at %q", charmPath)
		}
		if err == nil {
			if err := lxdprofile.ValidateLXDProfile(lxdCharmProfiler{
				Charm: ch,
			}); err != nil && !h.force {
				return errors.Annotatef(err, "cannot deploy local charm at %q", charmPath)
			}
			if curl, err = h.deployAPI.AddLocalCharm(curl, ch, h.force); err != nil {
				return err
			}
			logger.Debugf("added charm %s", curl)
			h.results[id] = curl.String()
			return nil
		}
	}

	// Not a local charm, so grab from the store.
	ch, err := resolveCharmURL(chParms.Charm)
	if err != nil {
		return errors.Trace(err)
	}
	// A channel is needed whether the risk is valid or not.
	channel, _ := corecharm.ParseChannel(chParms.Channel)
	origin, err := utils.DeduceOrigin(ch, channel)
	if err != nil {
		return errors.Trace(err)
	}

	url, origin, _, err := h.bundleResolver.ResolveCharm(ch, origin)
	if err != nil {
		return errors.Annotatef(err, "cannot resolve URL %q", chParms.Charm)
	}
	if url.Series == "bundle" {
		return errors.Errorf("expected charm URL, got bundle URL %q", chParms.Charm)
	}

	var macaroon *macaroon.Macaroon
	var charmOrigin commoncharm.Origin
	url, macaroon, charmOrigin, err = store.AddCharmFromURL(h.deployAPI, h.authorizer, url, origin, h.force)
	if err != nil {
		return errors.Annotatef(err, "cannot add charm %q", chParms.Charm)
	}
	logger.Debugf("added charm %s", url)
	h.results[id] = url.String()
	h.macaroons[url] = macaroon
	h.origins[url] = charmOrigin
	return nil
}

func (h *bundleHandler) makeResourceMap(meta map[string]resource.Meta, storeResources map[string]int, localResources map[string]string) map[string]string {
	resources := make(map[string]string)
	for resName, path := range localResources {
		// The resource may be a relative path, convert to absolute path.
		// NB for OCI image resources, path could be a yaml file but it may
		// also just be a docker registry path.
		maybePath := path
		if !filepath.IsAbs(path) {
			maybePath = filepath.Clean(filepath.Join(h.bundleDir, path))
		}
		_, err := h.filesystem.Stat(maybePath)
		if err == nil || meta[resName].Type == resource.TypeFile {
			path = maybePath
		}
		resources[resName] = path
	}
	for resName, revision := range storeResources {
		resources[resName] = fmt.Sprint(revision)
	}
	return resources
}

// addApplication deploys an application with no units.
func (h *bundleHandler) addApplication(change *bundlechanges.AddApplicationChange) error {
	// TODO: add verbose output for details
	if h.dryRun {
		return nil
	}

	p := change.Params
	cURL, err := resolveCharmURL(resolve(p.Charm, h.results))
	if err != nil {
		return errors.Trace(err)
	}

	macaroon := h.macaroons[cURL]

	h.results[change.Id()] = p.Application

	// If this application requires trust and the operator consented to
	// granting it, set the "trust" application option to true. This is
	// equivalent to running 'juju trust $app'.
	if h.trust && applicationRequiresTrust(h.data.Applications[p.Application]) {
		if p.Options == nil {
			p.Options = make(map[string]interface{})
		}

		p.Options[app.TrustConfigOptionName] = strconv.FormatBool(h.trust)
	}

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
				// storage constraints overridden
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
	deviceConstraints := h.bundleDevices[p.Application]
	if len(p.Devices) > 0 {
		if deviceConstraints == nil {
			deviceConstraints = make(map[string]devices.Constraints)
		}
		for k, v := range p.Devices {
			if _, ok := deviceConstraints[k]; ok {
				// Device constraints overridden
				// on the command line.
				continue
			}
			cons, err := devices.ParseConstraints(v)
			if err != nil {
				return errors.Annotate(err, "invalid device constraints")
			}
			deviceConstraints[k] = cons
		}
	}
	charmInfo, err := h.deployAPI.CharmInfo(cURL.String())
	if err != nil {
		return errors.Trace(err)
	}

	if err := lxdprofile.ValidateLXDProfile(lxdCharmInfoProfiler{
		CharmInfo: charmInfo,
	}); err != nil && !h.force {
		return errors.Trace(err)
	}

	chID := charmstore.CharmID{
		URL:     cURL,
		Channel: csparams.Channel(h.origins[cURL].Risk),
	}

	var resNames2IDs map[string]string

	// Handle the fact that charmhub doesn't support resources, so we skip that
	// request until they are.
	if !charm.CharmHub.Matches(cURL.Schema) {
		resources := h.makeResourceMap(charmInfo.Meta.Resources, p.Resources, p.LocalResources)
		resNames2IDs, err = h.deployResources(
			p.Application,
			chID,
			macaroon,
			resources,
			charmInfo.Meta.Resources,
			h.deployAPI,
			h.filesystem,
		)
		if err != nil {
			return errors.Trace(err)
		}
	}

	// Figure out what series we need to deploy with.
	// Note: with the advent of charmhub urls, the URL series will always be
	// empty.
	supportedSeries := charmInfo.Meta.Series
	if len(supportedSeries) == 0 && chID.URL.Series != "" {
		supportedSeries = []string{chID.URL.Series}
	}

	workloadSeries, err := supportedJujuSeries(h.clock.Now(), p.Series, h.modelConfig.ImageStream())
	if err != nil {
		return errors.Trace(err)
	}

	selector := seriesSelector{
		seriesFlag:          p.Series,
		charmURLSeries:      chID.URL.Series,
		supportedSeries:     supportedSeries,
		supportedJujuSeries: workloadSeries,
		conf:                h.modelConfig,
		force:               h.force,
		fromBundle:          true,
	}
	series, err := selector.charmSeries()
	if err = charmValidationError(series, cURL.Name, errors.Trace(err)); err != nil {
		return errors.Trace(err)
	}

	// Only Kubernetes bundles send the unit count and placement with the deploy API call.
	numUnits := 0
	var placement []*instance.Placement
	if h.data.Type == "kubernetes" {
		numUnits = p.NumUnits
	}
	var track string
	if h.origin.Track != nil {
		track = *h.origin.Track
	}
	// A channel is needed whether the risk is valid or not.
	channel, _ := corecharm.MakeChannel(track, h.origin.Risk, "")
	origin, err := utils.DeduceOrigin(chID.URL, channel)
	if err != nil {
		return errors.Trace(err)
	}

	// Deploy the application.
	if err := h.deployAPI.Deploy(application.DeployArgs{
		CharmID:          chID,
		CharmOrigin:      origin,
		Cons:             cons,
		ApplicationName:  p.Application,
		Series:           series,
		NumUnits:         numUnits,
		Placement:        placement,
		ConfigYAML:       configYAML,
		Storage:          storageConstraints,
		Devices:          deviceConstraints,
		Resources:        resNames2IDs,
		EndpointBindings: p.EndpointBindings,
	}); err != nil {
		return errors.Annotatef(err, "cannot deploy application %q", p.Application)
	}
	h.writeAddedResources(resNames2IDs)

	return nil
}

func (h *bundleHandler) writeAddedResources(resNames2IDs map[string]string) {
	// Make sure the resources are output in a defined order.
	names := set.NewStrings()
	for resName := range resNames2IDs {
		names.Add(resName)
	}
	for _, name := range names.SortedValues() {
		h.ctx.Infof("  added resource %s", name)
	}
}

// scaleApplication updates the number of units for an application.
func (h *bundleHandler) scaleApplication(change *bundlechanges.ScaleChange) error {
	if h.dryRun {
		return nil
	}

	p := change.Params

	result, err := h.deployAPI.ScaleApplication(application.ScaleApplicationParams{
		ApplicationName: p.Application,
		Scale:           p.Scale,
	})
	if err == nil && result.Error != nil {
		err = result.Error
	}
	if err != nil {
		return errors.Annotatef(err, "cannot scale application %q", p.Application)
	}
	return nil
}

// addMachine creates a new top-level machine or container in the environment.
func (h *bundleHandler) addMachine(change *bundlechanges.AddMachineChange) error {
	p := change.Params
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

	deployedApps := func() string {
		apps := h.applicationsForMachineChange(change.Id())
		// Note that we *should* always have at least one application
		// that justifies the creation of this machine. But just in
		// case, check (see https://pad.lv/1773357).
		if len(apps) == 0 {
			h.ctx.Warningf("no applications found for machine change %q", change.Id())
			return "nothing"
		}
		msg := apps[0] + " unit"

		if count := len(apps); count != 1 {
			msg = strings.Join(apps[:count-1], ", ") + " and " + apps[count-1] + " units"
		}
		return msg
	}

	cons, err := constraints.Parse(p.Constraints)
	if err != nil {
		// This should never happen, as the bundle is already verified.
		return errors.Annotate(err, "invalid constraints for machine")
	}
	machineParams := params.AddMachineParams{
		Constraints: cons,
		Series:      p.Series,
		Jobs:        []model.MachineJob{model.JobHostUnits},
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
			return errors.Annotatef(err, "cannot create machine for holding %s", deployedApps())
		}
		machineParams.ContainerType = containerType
		if p.ParentId != "" {
			logger.Debugf("p.ParentId: %q", p.ParentId)
			id, err := h.resolveMachine(p.ParentId)
			if err != nil {
				return errors.Annotatef(err, "cannot retrieve parent placement for %s", deployedApps())
			}
			// Never create nested containers for deployment.
			machineParams.ParentId = h.topLevelMachine(id)
		}
	}
	logger.Debugf("machineParams: %s", pretty.Sprint(machineParams))
	r, err := h.deployAPI.AddMachines([]params.AddMachineParams{machineParams})
	if err != nil {
		return errors.Annotatef(err, "cannot create machine for holding %s", deployedApps())
	}
	if r[0].Error != nil {
		return errors.Annotatef(r[0].Error, "cannot create machine for holding %s", deployedApps())
	}
	machine := r[0].Machine
	if p.ContainerType == "" {
		logger.Debugf("created new machine %s for holding %s", machine, deployedApps())
	} else if p.ParentId == "" {
		logger.Debugf("created %s container in new machine for holding %s", machine, deployedApps())
	} else {
		logger.Debugf("created %s container in machine %s for holding %s", machine, machineParams.ParentId, deployedApps())
	}
	h.results[change.Id()] = machine
	return nil
}

// addRelation creates a relationship between two applications.
func (h *bundleHandler) addRelation(change *bundlechanges.AddRelationChange) error {
	if h.dryRun {
		return nil
	}
	p := change.Params
	ep1 := resolveRelation(p.Endpoint1, h.results)
	ep2 := resolveRelation(p.Endpoint2, h.results)
	// TODO(wallyworld) - CMR support in bundles
	_, err := h.deployAPI.AddRelation([]string{ep1, ep2}, nil)
	if err != nil {
		// TODO(thumper): remove this error check when we add resolving
		// implicit relations.
		if params.IsCodeAlreadyExists(err) {
			return nil
		}
		return errors.Annotatef(err, "cannot add relation between %q and %q", ep1, ep2)

	}
	return nil
}

// addUnit adds a single unit to an application already present in the environment.
func (h *bundleHandler) addUnit(change *bundlechanges.AddUnitChange) error {
	if h.dryRun {
		return nil
	}

	p := change.Params
	applicationName := resolve(p.Application, h.results)
	var err error
	var placementArg []*instance.Placement
	targetMachine := p.To
	if targetMachine != "" {
		logger.Debugf("addUnit: placement %q", targetMachine)
		// The placement maybe "container:machine"
		container := ""
		if parts := strings.Split(targetMachine, ":"); len(parts) > 1 {
			container = parts[0]
			targetMachine = parts[1]
		}
		targetMachine, err = h.resolveMachine(targetMachine)
		if err != nil {
			// Should never happen.
			return errors.Annotatef(err, "cannot retrieve placement for %q unit", applicationName)
		}
		directive := targetMachine
		if container != "" {
			directive = container + ":" + directive
		}
		placement, err := utils.ParsePlacement(directive)
		if err != nil {
			return errors.Errorf("invalid --to parameter %q", directive)
		}
		logger.Debugf("  resolved: placement %q", directive)
		placementArg = append(placementArg, placement)
	}
	r, err := h.deployAPI.AddUnits(application.AddUnitsParams{
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
		h.results[change.Id()] = unit
	} else {
		logger.Debugf("added %s unit to new machine", unit)
		h.results[change.Id()] = targetMachine
	}

	// Note that the targetMachine can be empty for now, resulting in a partially
	// incomplete unit status. That's ok as the missing info is provided later
	// when it is required.
	h.unitStatus[unit] = targetMachine
	return nil
}

// upgradeCharm will get the application to use the new charm.
func (h *bundleHandler) upgradeCharm(change *bundlechanges.UpgradeCharmChange) error {
	if h.dryRun {
		return nil
	}

	p := change.Params
	cURL, err := resolveCharmURL(resolve(p.Charm, h.results))
	if err != nil {
		return errors.Trace(err)
	}

	chID := application.CharmID{
		URL:    cURL,
		Origin: h.origins[cURL],
	}
	macaroon := h.macaroons[cURL]

	meta, err := utils.GetMetaResources(cURL, h.deployAPI)
	if err != nil {
		return errors.Trace(err)
	}
	resources := h.makeResourceMap(meta, p.Resources, p.LocalResources)

	resourceLister, err := resourceadapters.NewAPIClient(h.deployAPI)
	if err != nil {
		return errors.Trace(err)
	}
	filtered, err := utils.GetUpgradeResources(resourceLister, p.Application, resources, meta)
	if err != nil {
		return errors.Trace(err)
	}
	var resNames2IDs map[string]string

	// Handle the fact that charmhub doesn't support resources, so we skip that
	// request until they are.
	if !charm.CharmHub.Matches(cURL.Schema) && len(filtered) != 0 {
		resNames2IDs, err = h.deployResources(
			p.Application,
			charmstore.CharmID{
				URL:     chID.URL,
				Channel: csparams.Channel(chID.Origin.Risk),
			},
			macaroon,
			resources,
			filtered,
			h.deployAPI,
			h.filesystem,
		)
		if err != nil {
			return errors.Trace(err)
		}
	}

	cfg := application.SetCharmConfig{
		ApplicationName: p.Application,
		CharmID:         chID,
		ResourceIDs:     resNames2IDs,
		Force:           h.force,
	}
	// Bundles only ever deal with the current generation.
	if err := h.deployAPI.SetCharm(model.GenerationMaster, cfg); err != nil {
		return errors.Trace(err)
	}
	h.writeAddedResources(resNames2IDs)

	return nil
}

// setOptions updates application configuration settings.
func (h *bundleHandler) setOptions(change *bundlechanges.SetOptionsChange) error {
	p := change.Params
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
	cfg, err := yaml.Marshal(map[string]map[string]interface{}{p.Application: p.Options})
	if err != nil {
		return errors.Annotatef(err, "cannot marshal options for application %q", p.Application)
	}

	if err := h.deployAPI.Update(params.ApplicationUpdate{
		ApplicationName: p.Application,
		SettingsYAML:    string(cfg),
		Generation:      model.GenerationMaster,
	}); err != nil {
		return errors.Annotatef(err, "cannot update options for application %q", p.Application)
	}

	return nil
}

// setConstraints updates application constraints.
func (h *bundleHandler) setConstraints(change *bundlechanges.SetConstraintsChange) error {
	if h.dryRun {
		return nil
	}
	p := change.Params
	// We know that p.constraints is a valid constraints type due to the validation.
	cons, _ := constraints.Parse(p.Constraints)
	if err := h.deployAPI.SetConstraints(p.Application, cons); err != nil {
		// This should never happen, as the bundle is already verified.
		return errors.Annotatef(err, "cannot update constraints for application %q", p.Application)
	}

	return nil
}

// exposeApplication exposes an application.
func (h *bundleHandler) exposeApplication(change *bundlechanges.ExposeChange) error {
	if h.dryRun {
		return nil
	}

	application := resolve(change.Params.Application, h.results)
	exposedEndpoints := make(map[string]params.ExposedEndpoint)
	for endpointName, exposeDetails := range change.Params.ExposedEndpoints {
		exposedEndpoints[endpointName] = params.ExposedEndpoint{
			ExposeToSpaces: exposeDetails.ExposeToSpaces,
			ExposeToCIDRs:  exposeDetails.ExposeToCIDRs,
		}
	}

	if err := h.deployAPI.Expose(application, exposedEndpoints); err != nil {
		return errors.Annotatef(err, "cannot expose application %s", application)
	}
	return nil
}

// setAnnotations sets annotations for an application or a machine.
func (h *bundleHandler) setAnnotations(change *bundlechanges.SetAnnotationsChange) error {
	p := change.Params
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
	result, err := h.deployAPI.SetAnnotation(map[string]map[string]string{tag: p.Annotations})
	if err == nil && len(result) > 0 {
		err = result[0].Error
	}
	if err != nil {
		return errors.Annotatef(err, "cannot set annotations for %s %q", p.EntityType, eid)
	}
	return nil
}

// createOffer creates an offer targeting one or more application endpoints.
func (h *bundleHandler) createOffer(change *bundlechanges.CreateOfferChange) error {
	if h.dryRun {
		return nil
	}

	p := change.Params
	result, err := h.deployAPI.Offer(h.targetModelUUID, p.Application, p.Endpoints, p.OfferName, "")
	if err == nil && len(result) > 0 && result[0].Error != nil {
		err = result[0].Error
	}
	if err != nil {
		return errors.Annotatef(err, "cannot create offer %s", p.OfferName)
	}
	return nil
}

// consumeOffer consumes an existing offer
func (h *bundleHandler) consumeOffer(change *bundlechanges.ConsumeOfferChange) error {
	if h.dryRun {
		return nil
	}

	p := change.Params
	url, err := charm.ParseOfferURL(p.URL)
	if err != nil {
		return errors.Trace(err)
	}
	if url.HasEndpoint() {
		return errors.Errorf("remote offer %q shouldn't include endpoint", p.URL)
	}
	if url.User == "" {
		url.User = h.accountUser
	}
	if url.Source == "" {
		url.Source = h.controllerName
	}
	// Get the consume details from the offer deployAPI. We don't use the generic
	// DeployerAPI as we may have to contact another controller to gain access
	// to that information.
	controllerOfferAPI, err := h.getConsumeDetailsAPI(url)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = controllerOfferAPI.Close() }()

	// Ensure we use the Local url here as we have to ignore the source (read as
	// target) controller, as the names of controllers might not match and we
	// end up with an error stating that the controller doesn't exist, even
	// though it's correct.
	consumeDetails, err := controllerOfferAPI.GetConsumeDetails(url.AsLocal().String())
	if err != nil {
		return errors.Trace(err)
	}
	// Parse the offer details URL and add the source controller so
	// things like status can show the original source of the offer.
	offerURL, err := charm.ParseOfferURL(consumeDetails.Offer.OfferURL)
	if err != nil {
		return errors.Trace(err)
	}
	offerURL.Source = url.Source
	consumeDetails.Offer.OfferURL = offerURL.String()

	// construct the cosume application arguments
	arg := crossmodel.ConsumeApplicationArgs{
		Offer:            *consumeDetails.Offer,
		ApplicationAlias: p.ApplicationName,
		Macaroon:         consumeDetails.Macaroon,
	}
	if consumeDetails.ControllerInfo != nil {
		controllerTag, err := names.ParseControllerTag(consumeDetails.ControllerInfo.ControllerTag)
		if err != nil {
			return errors.Trace(err)
		}
		arg.ControllerInfo = &crossmodel.ControllerInfo{
			ControllerTag: controllerTag,
			Alias:         consumeDetails.ControllerInfo.Alias,
			Addrs:         consumeDetails.ControllerInfo.Addrs,
			CACert:        consumeDetails.ControllerInfo.CACert,
		}
	}
	localName, err := h.deployAPI.Consume(arg)
	if err != nil {
		return errors.Trace(err)
	}
	h.results[change.Id()] = localName
	h.ctx.Infof("Added %s as %s", url.Path(), localName)
	return nil
}

// grantOfferAccess grants access to an offer.
func (h *bundleHandler) grantOfferAccess(change *bundlechanges.GrantOfferAccessChange) error {
	if h.dryRun {
		return nil
	}

	p := change.Params

	offerURL := fmt.Sprintf("%s.%s", h.targetModelName, p.Offer)
	if err := h.deployAPI.GrantOffer(p.User, p.Access, offerURL); err != nil && !isUserAlreadyHasAccessErr(err) {

		return errors.Annotatef(err, "cannot grant %s access to user %s on offer %s", p.Access, p.User, offerURL)
	}
	return nil
}

// applicationsForMachineChange returns the names of the applications for which an
// "addMachine" change is required, as adding machines is required to place
// units, and units belong to applications.
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
	var delta []params.Delta
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
			case *params.UnitInfo:
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
	logger.Debugf("resolveMachine(%q)", placeholder)
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

func (h *bundleHandler) topLevelMachine(id string) string {
	if !names.IsContainerMachine(id) {
		return id
	}
	tag := names.NewMachineTag(id)
	return tag.Parent().Id()
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
// entity already existed in the model, the placeholder value is the actual
// entity from the model, and in these situations the placeholder value doesn't
// start with the '$'.
func resolve(placeholder string, results map[string]string) string {
	logger.Debugf("resolve %q from %s", placeholder, pretty.Sprint(results))
	if !strings.HasPrefix(placeholder, "$") {
		return placeholder
	}
	id := placeholder[1:]
	return results[id]
}

// applicationRequiresTrust returns true if this app requires the operator to
// explicitly trust it. trust requirements may be either specified as an option
// or via the "trust" field at the application spec level
func applicationRequiresTrust(appSpec *charm.ApplicationSpec) bool {
	optRequiresTrust := appSpec.Options != nil && appSpec.Options["trust"] == true
	return appSpec.RequiresTrust || optRequiresTrust
}

// isUserAlreadyHasAccessErr returns true if err indicates that the user
// already has access to an offer. Unfortunately, the server does not set a
// status code for this error so we need to fall back to a hacky string
// comparison to be compatible with older controllers.
func isUserAlreadyHasAccessErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "user already has")
}
