// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	jujuclock "github.com/juju/clock"
	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/kr/pretty"
	"gopkg.in/macaroon.v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/charms"
	"github.com/juju/juju/api/client/resources"
	commoncharm "github.com/juju/juju/api/common/charm"
	app "github.com/juju/juju/apiserver/facades/client/application"
	appbundle "github.com/juju/juju/cmd/juju/application/bundle"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/arch"
	bundlechanges "github.com/juju/juju/core/bundle/changes"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/storage"
)

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
	modelConstraints  constraints.Value

	deployAPI            DeployerAPI
	bundleResolver       Resolver
	authorizer           store.MacaroonGetter
	getConsumeDetailsAPI func(*charm.OfferURL) (ConsumeDetails, error)
	deployResources      DeployResourcesFunc

	useExistingMachines bool
	bundleMachines      map[string]string
	bundleStorage       map[string]map[string]storage.Constraints
	bundleDevices       map[string]map[string]devices.Constraints

	targetModelName string
	targetModelUUID string
	controllerName  string
	accountUser     string

	knownSpaceNames set.Strings
}

// deployBundle deploys the given bundle data using the given API client and
// charm store client. The deployment is not transactional, and its progress is
// notified using the given deployment logger.
//
// Note: deployBundle expects that spec.BundleData points to a verified bundle
// that has all required external overlays applied.
func bundleDeploy(defaultCharmSchema charm.Schema, bundleData *charm.BundleData, spec bundleDeploySpec) (map[charm.URL]*macaroon.Macaroon, error) {
	// TODO: move bundle parsing and checking into the handler.
	h := makeBundleHandler(defaultCharmSchema, bundleData, spec)
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
	origin           commoncharm.Origin
	modelConstraints constraints.Value

	// deployAPI is used to interact with the environment.
	deployAPI            DeployerAPI
	bundleResolver       Resolver
	authorizer           store.MacaroonGetter
	getConsumeDetailsAPI func(*charm.OfferURL) (ConsumeDetails, error)
	deployResources      DeployResourcesFunc

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

	macaroons map[charm.URL]*macaroon.Macaroon

	// origins holds a different origins based on the charm URL and channels for
	// each origin.
	origins map[charm.URL]map[string]commoncharm.Origin

	// knownSpaceNames is a set of the names of existing spaces an application
	// can bind to
	knownSpaceNames set.Strings

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

	// The default schema to use for charm URLs that don't specify one.
	defaultCharmSchema charm.Schema
}

func makeBundleHandler(defaultCharmSchema charm.Schema, bundleData *charm.BundleData, spec bundleDeploySpec) *bundleHandler {
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
		modelConstraints:     spec.modelConstraints,
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
		macaroons:            make(map[charm.URL]*macaroon.Macaroon),
		origins:              make(map[charm.URL]map[string]commoncharm.Origin),
		knownSpaceNames:      spec.knownSpaceNames,

		targetModelName: spec.targetModelName,
		targetModelUUID: spec.targetModelUUID,
		controllerName:  spec.controllerName,
		accountUser:     spec.accountUser,

		defaultCharmSchema: defaultCharmSchema,
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
	status, err = h.updateChannelsModelStatus(status)
	if err != nil {
		return errors.Annotate(err, "updating current application channels")
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

// updateChannelsModelStatus gets the application's channel from a different
// source when the default charm schema is charmstore.  Required for compatibility
// between pre 2.9 controllers and newer clients.  The controller has the data,
// status output does not.
func (h *bundleHandler) updateChannelsModelStatus(status *params.FullStatus) (*params.FullStatus, error) {
	if !h.defaultCharmSchema.Matches(charm.CharmStore.String()) || len(status.Applications) <= 0 {
		return status, nil
	}
	var tags []names.ApplicationTag
	for k := range status.Applications {
		tags = append(tags, names.NewApplicationTag(k))
	}
	infoResults, err := h.deployAPI.ApplicationsInfo(tags)
	if err != nil {
		return nil, err
	}

	for i, result := range infoResults {
		name := tags[i].Id()
		if result.Error != nil {
			return nil, errors.Annotatef(err, "%s", name)
		}
		appStatus, ok := status.Applications[name]
		if !ok {
			return nil, errors.NotFoundf("programming error: %q application info", name)
		}
		appStatus.CharmChannel = result.Result.Channel
		status.Applications[name] = appStatus
	}
	return status, nil
}

// resolveCharmsAndEndpoints will go through the bundle and
// resolve the charm URLs. From the model the charm names are
// fully qualified, meaning they have a source and revision id.
// Effectively the logic this method follows is:
//   - if the bundle specifies a local charm, and the application
//     exists already, then override the charm URL in the bundle
//     spec to match the charm name from the model. We don't
//     upgrade local charms as part of a bundle deploy.
//   - the charm URL is resolved and the bundle spec is replaced
//     with the fully resolved charm URL - i.e.: with rev id.
//   - check all endpoints, and if any of them have implicit endpoints,
//     and if they do, resolve the implicitness in order to compare
//     with relations in the model.
func (h *bundleHandler) resolveCharmsAndEndpoints() error {
	deployedApps := set.NewStrings()

	for _, name := range h.applications.SortedValues() {
		spec := h.data.Applications[name]
		app := h.model.GetApplication(name)

		var cons constraints.Value
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

			var err error
			cons, err = constraints.Parse(app.Constraints)
			if err != nil {
				return errors.Trace(err)
			}
		}

		if h.isLocalCharm(spec.Charm) {
			continue
		}

		ch, err := resolveCharmURL(spec.Charm, h.defaultCharmSchema)
		if err != nil {
			return errors.Trace(err)
		}

		// To deploy by revision, the revision number must be in the origin for a
		// charmhub charm and in the url for a charmstore charm.
		if charm.CharmHub.Matches(ch.Schema) {
			if ch.Revision != -1 {
				return errors.Errorf("cannot specify revision in %q, please use revision", ch)
			}
			var channel charm.Channel
			if spec.Channel != "" {
				channel, err = charm.ParseChannelNormalize(spec.Channel)
				if err != nil {
					return errors.Annotatef(err, "for application %q", spec.Charm)
				}
			}
			if spec.Revision != nil && *spec.Revision != -1 && channel.Empty() {
				return errors.Errorf("application %q with a revision requires a channel for future upgrades, please use channel", name)
			}
		} else if charm.CharmStore.Matches(ch.Schema) {
			if ch.Revision != -1 && spec.Revision != nil && *spec.Revision != -1 && ch.Revision != *spec.Revision {
				return errors.Errorf("two different revisions to deploy %q: specified %d and %d, please choose one.", name, ch.Revision, *spec.Revision)
			}
			if ch.Revision == -1 && spec.Revision != nil && *spec.Revision != -1 {
				ch = ch.WithRevision(*spec.Revision)
			}
		}

		channel, origin, err := h.constructChannelAndOrigin(ch, spec.Series, spec.Channel, cons)
		if err != nil {
			return errors.Trace(err)
		}
		url, origin, _, err := h.bundleResolver.ResolveCharm(ch, origin, false) // no --switch possible.
		if err != nil {
			return errors.Annotatef(err, "cannot resolve charm or bundle %q", ch.Name)
		}
		if charm.CharmHub.Matches(url.Schema) {
			// Although we've resolved the charm URL, we actually don't want the
			// whole URL (architecture, channel and revision), only the name is
			// verified that it exists.
			url = &charm.URL{
				Schema:   charm.CharmHub.String(),
				Name:     url.Name,
				Revision: -1,
			}
			origin = origin.WithSeries("")
		}

		h.ctx.Infof("%s", formatLocatedText(ch, origin))
		if url.Series == "bundle" || origin.Type == "bundle" {
			return errors.Errorf("expected charm, got bundle %q", ch.Name)
		}

		spec.Charm = url.String()

		// Ensure we set the origin for a resolved charm. When an upgrade with a
		// bundle happens, we need to ensure that we have all the existing
		// charm origins as well as any potential new ones.
		// Specifically this happens when a bundle is re-using a charm from
		// another application, but giving it a new name.
		h.addOrigin(*url, channel, origin)
	}

	// TODO(thumper): the InferEndpoints code is deeply wedged in the
	// persistence layer and needs to be extracted. This is a multi-day
	// effort, so for now the bundle handling is doing no implicit endpoint
	// handling.
	return nil
}

func (h *bundleHandler) resolveCharmChannelAndRevision(charmURL, charmSeries, charmChannel, arch string, revision int) (string, int, error) {
	if h.isLocalCharm(charmURL) {
		return charmChannel, -1, nil
	}
	// If the charm URL already contains a revision, return that before
	// attempting to resolve a revision from any charm store. We can ignore the
	// error here, as we want to just parse out the charm URL.
	// Resolution and validation of the charm URL happens further down.
	if curl, err := charm.ParseURL(charmURL); err == nil {
		if charm.Local.Matches(curl.Schema) {
			return charmChannel, -1, nil
		} else if curl.Revision >= 0 && charmChannel != "" {
			return charmChannel, curl.Revision, nil
		}
	}

	// Resolve and validate a charm URL based on passed in charm.
	ch, err := resolveCharmURL(charmURL, h.defaultCharmSchema)
	if err != nil {
		return "", -1, errors.Trace(err)
	}

	var cons constraints.Value
	if arch != "" {
		cons = constraints.Value{Arch: &arch}
	}

	// Charmhub charms require the revision in the origin, but not the charm url used.
	// Add it here temporarily to construct the origin.
	if revision != -1 {
		ch = ch.WithRevision(revision)
	}

	_, origin, err := h.constructChannelAndOrigin(ch, charmSeries, charmChannel, cons)
	if err != nil {
		return "", -1, errors.Trace(err)
	}
	_, origin, _, err = h.bundleResolver.ResolveCharm(ch, origin, false) // no --switch possible.
	if err != nil {
		return "", -1, errors.Annotatef(err, "cannot resolve charm or bundle %q", ch.Name)
	}
	resolvedChan := origin.CharmChannel().Normalize().String()
	rev := origin.Revision
	if rev == nil {
		return resolvedChan, -1, nil
	}
	return resolvedChan, *rev, nil
}

// constructChannelAndOrigin attempts to construct a fully qualified channel
// along with an origin that matches the hardware constraints and the charm url
// source.
func (h *bundleHandler) constructChannelAndOrigin(curl *charm.URL, charmSeries, charmChannel string, cons constraints.Value) (charm.Channel, commoncharm.Origin, error) {
	var channel charm.Channel
	if charmChannel != "" {
		var err error
		if channel, err = charm.ParseChannelNormalize(charmChannel); err != nil {
			return charm.Channel{}, commoncharm.Origin{}, errors.Trace(err)
		}
	}

	platform, err := utils.DeducePlatform(cons, charmSeries, h.modelConstraints)
	if err != nil {
		return charm.Channel{}, commoncharm.Origin{}, errors.Trace(err)
	}

	origin, err := utils.DeduceOrigin(curl, channel, platform)
	if err != nil {
		return charm.Channel{}, commoncharm.Origin{}, errors.Trace(err)
	}
	return channel, origin, nil
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
		Bundle:           h.data,
		BundleURL:        bundleURL,
		Model:            h.model,
		ConstraintGetter: addCharmConstraintsParser(h.modelConstraints),
		CharmResolver:    h.resolveCharmChannelAndRevision,
		Logger:           logger,
		Force:            h.force,
	}
	if logger.IsTraceEnabled() {
		logger.Tracef("bundlechanges.ChangesConfig.Bundle %s", pretty.Sprint(cfg.Bundle))
		logger.Tracef("bundlechanges.ChangesConfig.BundleURL %s", pretty.Sprint(cfg.BundleURL))
		logger.Tracef("bundlechanges.ChangesConfig.Model %s", pretty.Sprint(cfg.Model))
	}
	changes, err := bundlechanges.FromData(cfg)
	if err != nil {
		return errors.Trace(err)
	}
	if logger.IsTraceEnabled() {
		logger.Tracef("changes %s", pretty.Sprint(changes))
	}
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
		if logger.IsTraceEnabled() {
			logger.Tracef("%d: change %s", i, pretty.Sprint(change))
		}
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
	return strings.HasPrefix(name, ".") || filepath.IsAbs(name) || strings.HasPrefix(name, "local:")
}

// addCharm adds a charm to the environment.
func (h *bundleHandler) addCharm(change *bundlechanges.AddCharmChange) error {
	if h.dryRun {
		return nil
	}
	id := change.Id()
	chParams := change.Params

	// Use the chSeries specified for this charm in the bundle,
	// fallback to the chSeries specified for the bundle.
	chSeries := chParams.Series
	if chSeries == "" {
		chSeries = h.data.Series
	}

	// First attempt to interpret as a local path.
	if h.isLocalCharm(chParams.Charm) {
		return h.addLocalCharm(chParams, chSeries, id)
	}

	// Not a local charm, so grab from the store.
	ch, err := resolveCharmURL(chParams.Charm, h.defaultCharmSchema)
	if err != nil {
		return errors.Trace(err)
	}

	// Verification of the revision piece was done when the bundle was
	// read in.  Ensure that the validated charm has correct revision.
	if charm.CharmStore.Matches(ch.Schema) && change.Params.Revision != nil && *change.Params.Revision >= 0 {
		ch = ch.WithRevision(*change.Params.Revision)
	}
	urlForOrigin := ch
	if change.Params.Revision != nil && *change.Params.Revision >= 0 {
		urlForOrigin = urlForOrigin.WithRevision(*change.Params.Revision)
	}

	// Ensure that we use the architecture from the add charm change params.
	var cons constraints.Value
	if change.Params.Architecture != "" {
		cons = constraints.Value{
			Arch: &change.Params.Architecture,
		}
	}
	platform, err := utils.DeducePlatform(cons, chSeries, h.modelConstraints)
	if err != nil {
		return errors.Trace(err)
	}

	// A channel is needed whether the risk is valid or not.
	var channel charm.Channel
	if charm.CharmHub.Matches(ch.Schema) {
		channel = corecharm.DefaultChannel
		if chParams.Channel != "" {
			channel, err = charm.ParseChannelNormalize(chParams.Channel)
			if err != nil {
				return errors.Trace(err)
			}
		}
	} else {
		channel = corecharm.MakeRiskOnlyChannel(chParams.Channel)
	}

	origin, err := utils.DeduceOrigin(urlForOrigin, channel, platform)
	if err != nil {
		return errors.Trace(err)
	}

	url, resolvedOrigin, supportedSeries, err := h.bundleResolver.ResolveCharm(ch, origin, false) // no --switch possible.
	if err != nil {
		return errors.Annotatef(err, "cannot resolve %q", ch.Name)
	}
	switch {
	case url.Series == "bundle" || resolvedOrigin.Type == "bundle":
		return errors.Errorf("expected charm, got bundle %q %v", ch.Name, resolvedOrigin)
	case resolvedOrigin.Series == "":
		modelCfg, workloadSeries, err := seriesSelectorRequirements(h.deployAPI, h.clock, url)
		if err != nil {
			return errors.Trace(err)
		}
		selector := seriesSelector{
			charmURLSeries:      url.Series,
			seriesFlag:          change.Params.Series,
			supportedSeries:     supportedSeries,
			supportedJujuSeries: workloadSeries,
			conf:                modelCfg,
			fromBundle:          true,
		}

		// Get the series to use.
		resolvedOrigin.Series, err = selector.charmSeries()
		if err != nil {
			return errors.Trace(err)
		}
		if url.Schema != charm.CharmStore.String() {
			url = url.WithSeries(resolvedOrigin.Series)
		}
		logger.Tracef("Using series %s from %v to deploy %v", resolvedOrigin.Series, supportedSeries, url)
	}

	var macaroon *macaroon.Macaroon
	var charmOrigin commoncharm.Origin
	url, macaroon, charmOrigin, err = store.AddCharmWithAuthorizationFromURL(h.deployAPI, h.authorizer, url, resolvedOrigin, h.force)
	if err != nil {
		return errors.Annotatef(err, "cannot add charm %q", ch.Name)
	} else if url == nil {
		return errors.Errorf("unexpected charm URL %q", ch.Name)
	}

	logger.Debugf("added charm %s for channel %s", url, channel)
	charmAlias := url.String()
	h.results[id] = charmAlias
	h.macaroons[*url] = macaroon
	h.addOrigin(*url, channel, charmOrigin)
	return nil
}

func (h *bundleHandler) addLocalCharm(chParams bundlechanges.AddCharmParams, chSeries, id string) error {
	// The charm path could contain the local schema prefix. If that's the
	// case we should remove that before attempting to join with the bundle
	// directory.
	charmPath := chParams.Charm
	if strings.HasPrefix(charmPath, "local:") {
		path, err := charm.ParseURL(charmPath)
		if err != nil {
			return errors.Trace(err)
		}
		charmPath = path.Name
	}
	if !filepath.IsAbs(charmPath) {
		charmPath = filepath.Join(h.bundleDir, charmPath)
	}

	ch, curl, err := corecharm.NewCharmAtPathForceSeries(charmPath, chSeries, h.force)
	if err != nil {
		return errors.Annotatef(err, "cannot deploy local charm at %q", charmPath)
	}

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
	// We know we're a local charm and local charms don't require an
	// explicit tailored origin. Instead we can just use a placeholder
	// to ensure correctness for later on in addApplication.
	h.addOrigin(*curl, corecharm.DefaultRiskChannel, commoncharm.Origin{
		Source: commoncharm.OriginLocal,
	})
	return nil
}

func (h *bundleHandler) makeResourceMap(meta map[string]charmresource.Meta, storeResources map[string]int, localResources map[string]string) map[string]string {
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
		if err == nil || meta[resName].Type == charmresource.TypeFile {
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
	curl, err := resolveCharmURL(resolve(p.Charm, h.results), h.defaultCharmSchema)
	if err != nil {
		return errors.Trace(err)
	} else if curl == nil {
		return errors.Errorf("unexpected application charm URL %q", p.Charm)
	}

	err = verifyEndpointBindings(p.EndpointBindings, h.knownSpaceNames)
	if err != nil {
		return errors.Trace(err)
	}

	channel, err := constructNormalizedChannel(p.Channel)
	if err != nil {
		// This should never happen, essentially we have a charm url that has
		// never been deployed previously, or has never been added with
		// setCharm.
		return errors.Trace(err)
	}
	origin, ok := h.getOrigin(*curl, channel)
	if !ok {
		return errors.Errorf("unexpected charm %q, charm not found for application %q", curl.Name, p.Application)
	}

	// Handle application constraints.
	cons, err := constraints.Parse(p.Constraints)
	if err != nil {
		// This should never happen, as the bundle is already verified.
		return errors.Annotate(err, "invalid constraints for application")
	}

	if charm.CharmHub.Matches(curl.Schema) {
		if cons.HasArch() && *cons.Arch != origin.Architecture {
			return errors.Errorf("unexpected %s application architecture (%s), charm already exists with architecture (%s)", p.Application, *cons.Arch, origin.Architecture)
		}
	}

	chID := application.CharmID{
		URL:    curl,
		Origin: origin,
	}
	macaroon := h.macaroons[*curl]

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

	storageConstraints, err := h.storageConstraints(p.Application, p.Storage)
	if err != nil {
		return errors.Trace(err)
	}

	deviceConstraints, err := h.deviceConstraints(p.Application, p.Devices)
	if err != nil {
		return errors.Trace(err)
	}

	charmInfo, err := h.deployAPI.CharmInfo(chID.URL.String())
	if err != nil {
		return errors.Trace(err)
	}

	resMap := h.makeResourceMap(charmInfo.Meta.Resources, p.Resources, p.LocalResources)

	if err := lxdprofile.ValidateLXDProfile(lxdCharmInfoProfiler{
		CharmInfo: charmInfo,
	}); err != nil && !h.force {
		return errors.Trace(err)
	}

	resNames2IDs, err := h.deployResources(
		p.Application,
		resources.CharmID{
			URL:    chID.URL,
			Origin: chID.Origin,
		},
		macaroon,
		resMap,
		charmInfo.Meta.Resources,
		h.deployAPI,
		h.filesystem,
	)
	if err != nil {
		return errors.Trace(err)
	}

	// Only Kubernetes bundles send the unit count and placement with the deploy API call.
	numUnits := 0
	var placement []*instance.Placement
	if h.data.Type == "kubernetes" {
		numUnits = p.NumUnits
	}

	// For charmstore charms we require a corrected channel for deploying an
	// application. This isn't required for any other store type (local,
	// charmhub).
	// We should remove this when charmstore charms are defunct and remove this
	// specialization.
	switch {
	case charm.Local.Matches(chID.URL.Schema):
		// Figure out what series we need to deploy with. For Local charms,
		// this was determined when addcharm was called.
		selectedSeries, err := h.selectedSeries(charmInfo.Charm(), chID, curl, p.Series)
		if err != nil {
			return errors.Trace(err)
		}
		origin.Series = selectedSeries
	case charm.CharmStore.Matches(chID.URL.Schema):
		// Figure out what series we need to deploy with. For CharmHub charms,
		// this was determined when addcharm was called.
		selectedSeries, err := h.selectedSeries(charmInfo.Charm(), chID, curl, p.Series)
		if err != nil {
			return errors.Trace(err)
		}

		platform, err := utils.DeducePlatform(cons, selectedSeries, h.modelConstraints)
		if err != nil {
			return errors.Trace(err)
		}
		// A channel is needed whether the risk is valid or not.
		channel, _ := charm.MakeChannel("", origin.Risk, "")
		origin, err = utils.DeduceOrigin(chID.URL, channel, platform)
		if err != nil {
			return errors.Trace(err)
		}
	}

	args := application.DeployArgs{
		CharmID:          chID,
		CharmOrigin:      origin,
		Cons:             cons,
		ApplicationName:  p.Application,
		Series:           origin.Series,
		NumUnits:         numUnits,
		Placement:        placement,
		ConfigYAML:       configYAML,
		Storage:          storageConstraints,
		Devices:          deviceConstraints,
		Resources:        resNames2IDs,
		EndpointBindings: p.EndpointBindings,
		Force:            h.force,
	}
	// Deploy the application.
	if err := h.deployAPI.Deploy(args); err != nil {
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

func (h *bundleHandler) storageConstraints(application string, storageMap map[string]string) (map[string]storage.Constraints, error) {
	storageConstraints := h.bundleStorage[application]
	if len(storageMap) > 0 {
		if storageConstraints == nil {
			storageConstraints = make(map[string]storage.Constraints)
		}
		for k, v := range storageMap {
			if _, ok := storageConstraints[k]; ok {
				// storage constraints overridden
				// on the command line.
				continue
			}
			cons, err := storage.ParseConstraints(v)
			if err != nil {
				return nil, errors.Annotate(err, "invalid storage constraints")
			}
			storageConstraints[k] = cons
		}
	}
	return storageConstraints, nil
}

func (h *bundleHandler) deviceConstraints(application string, deviceMap map[string]string) (map[string]devices.Constraints, error) {
	deviceConstraints := h.bundleDevices[application]
	if len(deviceMap) > 0 {
		if deviceConstraints == nil {
			deviceConstraints = make(map[string]devices.Constraints)
		}
		for k, v := range deviceMap {
			if _, ok := deviceConstraints[k]; ok {
				// Device constraints overridden
				// on the command line.
				continue
			}
			cons, err := devices.ParseConstraints(v)
			if err != nil {
				return nil, errors.Annotate(err, "invalid device constraints")
			}
			deviceConstraints[k] = cons
		}
	}
	return deviceConstraints, nil
}

func (h *bundleHandler) selectedSeries(ch charm.CharmMeta, chID application.CharmID, curl *charm.URL, chSeries string) (string, error) {
	if corecharm.IsKubernetes(ch) && charm.MetaFormat(ch) == charm.FormatV1 {
		chSeries = series.Kubernetes.String()
	}

	supportedSeries, err := corecharm.ComputedSeries(ch)
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(supportedSeries) == 0 && chID.URL.Series != "" {
		supportedSeries = []string{chID.URL.Series}
	}

	workloadSeries, err := supportedJujuSeries(h.clock.Now(), chSeries, h.modelConfig.ImageStream())
	if err != nil {
		return "", errors.Trace(err)
	}

	selector := seriesSelector{
		seriesFlag:          chSeries,
		charmURLSeries:      chID.URL.Series,
		supportedSeries:     supportedSeries,
		supportedJujuSeries: workloadSeries,
		conf:                h.modelConfig,
		force:               h.force,
		fromBundle:          true,
	}
	selectedSeries, err := selector.charmSeries()
	return selectedSeries, charmValidationError(curl.Name, errors.Trace(err))
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
	var base *params.Base
	if p.Series != "" && h.deployAPI.BestAPIVersion() >= 8 {
		info, err := series.GetBaseFromSeries(p.Series)
		if err != nil {
			return errors.NotValidf("machine series %q", p.Series)
		}
		p.Series = ""
		base = &params.Base{
			Name:    info.Name,
			Channel: info.Channel.String(),
		}
	}
	machineParams := params.AddMachineParams{
		Constraints: cons,
		Series:      p.Series,
		Base:        base,
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
	resolvedCharm := resolve(p.Charm, h.results)
	curl, err := resolveCharmURL(resolvedCharm, h.defaultCharmSchema)
	if err != nil {
		return errors.Trace(err)
	}
	if curl == nil {
		return errors.Errorf("unexpected upgrade charm URL %q", p.Charm)
	}

	channel, err := constructNormalizedChannel(p.Channel)
	if err != nil {
		// This should never happen, essentially we have a charm url that has
		// never been deployed previously, or has never been added with
		// setCharm.
		return errors.Trace(err)
	}
	origin, ok := h.getOrigin(*curl, channel)
	if !ok {
		return errors.Errorf("unexpected charm %q, charm not found for application %q", curl.Name, p.Application)
	}

	chID := application.CharmID{
		URL:    curl,
		Origin: origin,
	}
	macaroon := h.macaroons[*curl]

	meta, err := utils.GetMetaResources(curl, h.deployAPI)
	if err != nil {
		return errors.Trace(err)
	}
	resMap := h.makeResourceMap(meta, p.Resources, p.LocalResources)

	resourceLister, err := resources.NewClient(h.deployAPI)
	if err != nil {
		return errors.Trace(err)
	}
	filtered, err := utils.GetUpgradeResources(chID, charms.NewClient(h.deployAPI), resourceLister, p.Application, resMap, meta)
	if err != nil {
		return errors.Trace(err)
	}
	var resNames2IDs map[string]string
	if len(filtered) != 0 {
		resNames2IDs, err = h.deployResources(
			p.Application,
			resources.CharmID{
				URL:    chID.URL,
				Origin: chID.Origin,
			},
			macaroon,
			resMap,
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

	if h.deployAPI.BestFacadeVersion("Application") > 12 {
		err = h.deployAPI.SetConfig(model.GenerationMaster, p.Application, string(cfg), nil)
	} else {
		err = h.deployAPI.Update(params.ApplicationUpdate{
			ApplicationName: p.Application,
			SettingsYAML:    string(cfg),
			Generation:      model.GenerationMaster,
		})
	}
	return errors.Annotatef(err, "cannot update options for application %q", p.Application)
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
	result, err := h.deployAPI.Offer(h.targetModelUUID, p.Application, p.Endpoints, h.accountUser, p.OfferName, "")
	if err == nil && len(result) > 0 && result[0].Error != nil {
		err = result[0].Error
	}
	return err
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
		return errors.Errorf("saas offer %q shouldn't include endpoint", p.URL)
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
func (h *bundleHandler) applicationsForMachineChange(changeID string) []string {
	applications := set.NewStrings()
mainloop:
	for _, change := range h.changes {
		for _, required := range change.Requires() {
			if required != changeID {
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

func (h *bundleHandler) addOrigin(curl charm.URL, channel charm.Channel, origin commoncharm.Origin) {
	if _, ok := h.origins[curl]; !ok {
		h.origins[curl] = make(map[string]commoncharm.Origin)
	}
	h.origins[curl][channel.Normalize().String()] = origin
}

func (h *bundleHandler) getOrigin(curl charm.URL, channel charm.Channel) (commoncharm.Origin, bool) {
	c, ok := h.origins[curl]
	if !ok {
		return commoncharm.Origin{}, false
	}
	o, ok := c[channel.Normalize().String()]
	return o, ok
}

func constructNormalizedChannel(channel string) (charm.Channel, error) {
	if channel == "" {
		return charm.Channel{}, nil
	}
	ch, err := charm.ParseChannelNormalize(channel)
	if err != nil {
		return charm.Channel{}, errors.Trace(err)
	}
	return ch, nil
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
// explicitly trust it. Trust requirements may be either specified as an option
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

func addCharmConstraintsParser(defaultConstraints constraints.Value) func(string) bundlechanges.ArchConstraint {
	return func(s string) bundlechanges.ArchConstraint {
		return bundleArchConstraint{
			constraints:        s,
			defaultConstraints: defaultConstraints,
		}
	}
}

type bundleArchConstraint struct {
	constraints        string
	defaultConstraints constraints.Value
}

func (b bundleArchConstraint) Arch() (string, error) {
	cons, err := constraints.Parse(b.constraints)
	if err != nil {
		return "", errors.Trace(err)
	}
	return arch.ConstraintArch(cons, &b.defaultConstraints), nil
}

func verifyEndpointBindings(endpointBindings map[string]string, knownSpaceNames set.Strings) error {
	for _, spaceName := range endpointBindings {
		if !knownSpaceNames.Contains(spaceName) {
			return errors.NotFoundf("space %q", spaceName)
		}
	}
	return nil
}
