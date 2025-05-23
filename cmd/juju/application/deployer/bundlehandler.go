// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/retry"
	"github.com/kr/pretty"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/charms"
	"github.com/juju/juju/api/client/resources"
	commoncharm "github.com/juju/juju/api/common/charm"
	appbundle "github.com/juju/juju/cmd/juju/application/bundle"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/modelcmd"
	coreapplication "github.com/juju/juju/core/application"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	bundlechanges "github.com/juju/juju/internal/bundle/changes"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
)

type bundleDeploySpec struct {
	ctx        *cmd.Context
	filesystem modelcmd.Filesystem

	modelType model.ModelType
	dryRun    bool
	force     bool
	trust     bool

	bundleDataSource  charm.BundleDataSource
	bundleDir         string
	bundleURL         *charm.URL
	bundleOverlayFile []string
	origin            commoncharm.Origin
	modelConstraints  constraints.Value

	deployAPI            DeployerAPI
	bundleResolver       Resolver
	getConsumeDetailsAPI func(*charm.OfferURL) (ConsumeDetails, error)
	deployResources      DeployResourcesFunc
	charmReader          CharmReader

	useExistingMachines bool
	bundleMachines      map[string]string
	bundleStorage       map[string]map[string]storage.Directive
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
func bundleDeploy(ctx context.Context, defaultCharmSchema charm.Schema, bundleData *charm.BundleData, spec bundleDeploySpec) error {
	// TODO: move bundle parsing and checking into the handler.
	h := makeBundleHandler(defaultCharmSchema, bundleData, spec)
	if err := h.makeModel(ctx, spec.useExistingMachines, spec.bundleMachines); err != nil {
		return errors.Trace(err)
	}
	if err := h.resolveCharmsAndEndpoints(ctx); err != nil {
		return errors.Trace(err)
	}
	if err := h.getChanges(ctx); err != nil {
		return errors.Trace(err)
	}
	if err := h.handleChanges(ctx); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// bundleHandler provides helpers and the state required to deploy a bundle.
type bundleHandler struct {
	dryRun    bool
	force     bool
	trust     bool
	modelType model.ModelType

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
	getConsumeDetailsAPI func(*charm.OfferURL) (ConsumeDetails, error)
	deployResources      DeployResourcesFunc
	charmReader          CharmReader

	// bundleStorage contains a mapping of application-specific storage
	// constraints. For each application, the storage directives in the
	// map will replace or augment the storage directives specified
	// in the bundle itself.
	bundleStorage map[string]map[string]storage.Directive

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

	bundleURL *charm.URL

	// unitStatus reflects the environment status and maps unit names to their
	// corresponding machine identifiers. This is kept updated by both change
	// handlers (addCharm, addApplication etc.) and by updateUnitStatus.
	unitStatus map[string]string

	modelConfig *config.Config

	model *bundlechanges.Model

	// origins holds a different origins based on the charm URL and channels for
	// each origin.
	origins map[charm.URL]map[string]commoncharm.Origin

	// knownSpaceNames is a set of the names of existing spaces an application
	// can bind to
	knownSpaceNames set.Strings

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

		modelType:            spec.modelType,
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
		getConsumeDetailsAPI: spec.getConsumeDetailsAPI,
		deployResources:      spec.deployResources,
		charmReader:          spec.charmReader,
		bundleStorage:        spec.bundleStorage,
		bundleDevices:        spec.bundleDevices,
		ctx:                  spec.ctx,
		filesystem:           spec.filesystem,
		data:                 bundleData,
		unitStatus:           make(map[string]string),
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
	ctx context.Context,
	useExistingMachines bool,
	bundleMachines map[string]string,
) error {
	// Initialize the unit status.
	status, err := h.deployAPI.Status(ctx, nil)
	if err != nil {
		return errors.Annotate(err, "cannot get model status")
	}

	h.model, err = appbundle.BuildModelRepresentation(ctx, status, h.deployAPI, useExistingMachines, bundleMachines)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf(context.TODO(), "model: %s", pretty.Sprint(h.model))

	for _, appData := range status.Applications {
		for unit, unitData := range appData.Units {
			h.unitStatus[unit] = unitData.Machine
		}
	}

	h.modelConfig, err = getModelConfig(ctx, h.deployAPI)
	return err
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
func (h *bundleHandler) resolveCharmsAndEndpoints(ctx context.Context) error {
	deployedApps := set.NewStrings()

	for _, name := range h.applications.SortedValues() {
		spec := h.data.Applications[name]
		app := h.model.GetApplication(name)

		var cons constraints.Value
		if app != nil {
			deployedApps.Add(name)

			if h.isLocalCharm(spec.Charm) {
				logger.Debugf(context.TODO(), "%s exists in model uses a local charm, replacing with %q", name, app.Charm)
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
		// charmhub charm. We know we must have a charmhub charm, since we return
		// early for local charms.
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

		var base corebase.Base
		if spec.Base != "" {
			base, err = corebase.ParseBaseFromString(spec.Base)
			if err != nil {
				return errors.Trace(err)
			}
		}

		// We return early with local charms, so here we know the charm must be from charmhub.
		channel, origin, err := h.constructChannelAndOrigin(charm.CharmHub, ch.Revision, base, spec.Channel, cons)
		if err != nil {
			return errors.Trace(err)
		}
		url, origin, _, err := h.bundleResolver.ResolveCharm(ctx, ch, origin, false) // no --switch possible.
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
			origin = origin.WithBase(nil)
		}

		h.ctx.Infof("%s", formatLocatedText(ch, origin))
		if origin.Type == "bundle" {
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

func (h *bundleHandler) resolveCharmChannelAndRevision(ctx context.Context, charmURL string, charmBase corebase.Base, charmChannel, arch string, revision int) (string, int, error) {
	if h.isLocalCharm(charmURL) {
		return charmChannel, -1, nil
	}
	// Resolve and validate a charm URL based on passed in charm.
	ch, err := resolveCharmURL(charmURL, h.defaultCharmSchema)
	if err != nil {
		return "", -1, errors.Trace(err)
	}
	// If the charm URL already contains a revision, return that before
	// attempting to resolve a revision from charmhub.
	if ch.Revision >= 0 && charmChannel != "" {
		return charmChannel, ch.Revision, nil
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

	// We return early with local charms, so here we know the charm must be from charmhub.
	_, origin, err := h.constructChannelAndOrigin(charm.CharmHub, ch.Revision, charmBase, charmChannel, cons)

	if err != nil {
		return "", -1, errors.Trace(err)
	}
	_, origin, _, err = h.bundleResolver.ResolveCharm(ctx, ch, origin, false) // no --switch possible.
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
func (h *bundleHandler) constructChannelAndOrigin(schema charm.Schema, revision int, charmBase corebase.Base, charmChannel string, cons constraints.Value) (charm.Channel, commoncharm.Origin, error) {
	var channel charm.Channel
	if charmChannel != "" {
		var err error
		if channel, err = charm.ParseChannelNormalize(charmChannel); err != nil {
			return charm.Channel{}, commoncharm.Origin{}, errors.Trace(err)
		}
	}

	platform := utils.MakePlatform(cons, charmBase, h.modelConstraints)
	origin, err := utils.MakeOrigin(schema, revision, channel, platform)
	if err != nil {
		return charm.Channel{}, commoncharm.Origin{}, errors.Trace(err)
	}
	return channel, origin, nil
}

func (h *bundleHandler) getChanges(ctx context.Context) error {
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
	if logger.IsLevelEnabled(corelogger.TRACE) {
		logger.Tracef(context.TODO(), "bundlechanges.ChangesConfig.Bundle %s", pretty.Sprint(cfg.Bundle))
		logger.Tracef(context.TODO(), "bundlechanges.ChangesConfig.BundleURL %s", pretty.Sprint(cfg.BundleURL))
		logger.Tracef(context.TODO(), "bundlechanges.ChangesConfig.Model %s", pretty.Sprint(cfg.Model))
	}
	changes, err := bundlechanges.FromData(ctx, cfg)
	if err != nil {
		return errors.Trace(err)
	}
	if logger.IsLevelEnabled(corelogger.TRACE) {
		logger.Tracef(context.TODO(), "changes %s", pretty.Sprint(changes))
	}
	h.changes = changes
	return nil
}

func (h *bundleHandler) handleChanges(ctx context.Context) error {
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
		if logger.IsLevelEnabled(corelogger.TRACE) {
			logger.Tracef(context.TODO(), "%d: change %s", i, pretty.Sprint(change))
		}

		var err error
		switch change := change.(type) {
		case *bundlechanges.AddCharmChange:
			err = h.addCharm(ctx, change)
		case *bundlechanges.AddMachineChange:
			err = h.addMachine(ctx, change)
		case *bundlechanges.AddRelationChange:
			err = h.addRelation(ctx, change)
		case *bundlechanges.AddApplicationChange:
			err = h.addApplication(ctx, change)
		case *bundlechanges.ScaleChange:
			err = h.scaleApplication(ctx, change)
		case *bundlechanges.AddUnitChange:
			err = h.addUnit(ctx, change)
		case *bundlechanges.ExposeChange:
			err = h.exposeApplication(ctx, change)
		case *bundlechanges.SetAnnotationsChange:
			err = h.setAnnotations(ctx, change)
		case *bundlechanges.UpgradeCharmChange:
			err = h.upgradeCharm(ctx, change)
		case *bundlechanges.SetOptionsChange:
			err = h.setOptions(ctx, change)
		case *bundlechanges.SetConstraintsChange:
			err = h.setConstraints(ctx, change)
		case *bundlechanges.CreateOfferChange:
			err = h.createOffer(ctx, change)
		case *bundlechanges.ConsumeOfferChange:
			err = h.consumeOffer(ctx, change)
		case *bundlechanges.GrantOfferAccessChange:
			err = h.grantOfferAccess(ctx, change)
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
func (h *bundleHandler) addCharm(ctx context.Context, change *bundlechanges.AddCharmChange) error {
	if h.dryRun {
		return nil
	}
	id := change.Id()
	chParams := change.Params

	var (
		base corebase.Base
		err  error
	)
	if chParams.Base != "" {
		base, err = corebase.ParseBaseFromString(chParams.Base)
		if err != nil {
			return errors.Trace(err)
		}
	}

	// First attempt to interpret as a local path.
	if h.isLocalCharm(chParams.Charm) {
		return h.addLocalCharm(ctx, chParams, id)
	}

	// Not a local charm, so grab from the store.
	ch, err := resolveCharmURL(chParams.Charm, h.defaultCharmSchema)
	if err != nil {
		return errors.Trace(err)
	}

	// Ensure that we use the architecture from the add charm change params.
	var cons constraints.Value
	if chParams.Architecture != "" {
		cons = constraints.Value{
			Arch: &chParams.Architecture,
		}
	}

	// A channel is needed whether the risk is valid or not.
	channel := corecharm.DefaultChannel
	if chParams.Channel != "" {
		channel, err = charm.ParseChannelNormalize(chParams.Channel)
		if err != nil {
			return errors.Trace(err)
		}
	}

	revision := -1
	if chParams.Revision != nil && *chParams.Revision >= 0 {
		revision = *chParams.Revision
	}

	platform := utils.MakePlatform(cons, base, h.modelConstraints)
	// We return early with local charms, so here we know the charm must be from charmhub.
	origin, err := utils.MakeOrigin(charm.CharmHub, revision, channel, platform)
	if err != nil {
		return errors.Trace(err)
	}

	url, resolvedOrigin, supportedBases, err := h.bundleResolver.ResolveCharm(ctx, ch, origin, false) // no --switch possible.
	if err != nil {
		return errors.Annotatef(err, "cannot resolve %q", ch.Name)
	}
	if resolvedOrigin.Type == "bundle" {
		return errors.Errorf("expected charm, got bundle %q %v", ch.Name, resolvedOrigin)
	}
	selector, err := corecharm.ConfigureBaseSelector(corecharm.SelectorConfig{
		Config:              h.modelConfig,
		Force:               h.force,
		Logger:              logger,
		RequestedBase:       base,
		SupportedCharmBases: supportedBases,
		WorkloadBases:       SupportedJujuBases(),
	})
	if err != nil {
		return errors.Trace(err)
	}

	selectedBase, err := selector.CharmBase()
	if err != nil {
		return errors.Trace(err)
	}
	resolvedOrigin.Base = selectedBase
	logger.Tracef(context.TODO(), "Using channel %s from %v to deploy %v", resolvedOrigin.Base, supportedBases, url)

	var charmOrigin commoncharm.Origin
	charmOrigin, err = h.deployAPI.AddCharm(ctx, url, resolvedOrigin, h.force)
	if err != nil {
		return errors.Annotatef(err, "cannot add charm %q", ch.Name)
	} else if url == nil {
		return errors.Errorf("unexpected charm URL %q", ch.Name)
	}

	logger.Debugf(context.TODO(), "added charm %s for channel %s", url, channel)
	charmAlias := url.String()
	h.results[id] = charmAlias
	h.addOrigin(*url, channel, charmOrigin)
	return nil
}

func (h *bundleHandler) addLocalCharm(ctx context.Context, chParams bundlechanges.AddCharmParams, id string) error {
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

	ch, curl, err := h.charmReader.NewCharmAtPath(charmPath)
	if err != nil {
		return errors.Annotatef(err, "cannot deploy local charm at %q", charmPath)
	}

	if err := lxdprofile.ValidateLXDProfile(lxdCharmProfiler{
		Charm: ch,
	}); err != nil && !h.force {
		return errors.Annotatef(err, "cannot deploy local charm at %q", charmPath)
	}
	if curl, err = h.deployAPI.AddLocalCharm(ctx, curl, ch, h.force); err != nil {
		return err
	}
	logger.Debugf(context.TODO(), "added charm %s", curl)
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
func (h *bundleHandler) addApplication(ctx context.Context, change *bundlechanges.AddApplicationChange) error {
	// TODO: add verbose output for details
	if h.dryRun {
		return nil
	}

	p := change.Params
	resolved, ok := resolve(p.Charm, h.results)
	if !ok {
		return errors.Errorf("attempting to apply %s without prerequisites", change.Description())
	}
	curl, err := resolveCharmURL(resolved, h.defaultCharmSchema)
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
		URL:    curl.String(),
		Origin: origin,
	}

	h.results[change.Id()] = p.Application

	// If this application requires trust and the operator consented to
	// granting it, set the "trust" application option to true. This is
	// equivalent to running 'juju trust $app'.
	if h.trust && applicationRequiresTrust(h.data.Applications[p.Application]) {
		if p.Options == nil {
			p.Options = make(map[string]interface{})
		}

		p.Options[coreapplication.TrustConfigOptionName] = strconv.FormatBool(h.trust)
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

	storageDirectives, err := h.storageDirectives(p.Application, p.Storage)
	if err != nil {
		return errors.Trace(err)
	}

	deviceConstraints, err := h.deviceConstraints(p.Application, p.Devices)
	if err != nil {
		return errors.Trace(err)
	}

	charmInfo, err := h.deployAPI.CharmInfo(ctx, chID.URL)
	if err != nil {
		return errors.Trace(err)
	}

	if h.modelType == model.CAAS {
		if ch := charmInfo.Charm(); charm.MetaFormat(ch) == charm.FormatV1 {
			return errors.NotSupportedf("deploying format v1 charms")
		}
	}

	resMap := h.makeResourceMap(charmInfo.Meta.Resources, p.Resources, p.LocalResources)

	if err := lxdprofile.ValidateLXDProfile(lxdCharmInfoProfiler{
		CharmInfo: charmInfo,
	}); err != nil && !h.force {
		return errors.Trace(err)
	}

	resNames2IDs, err := h.deployResources(
		ctx,
		p.Application,
		resources.CharmID{
			URL:    chID.URL,
			Origin: chID.Origin,
		},
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
	if h.data.Type == bundlechanges.Kubernetes {
		numUnits = p.NumUnits
	}

	if charm.Local.Matches(curl.Schema) {
		var (
			base corebase.Base
			err  error
		)
		if p.Base != "" {
			base, err = corebase.ParseBaseFromString(p.Base)
			if err != nil {
				return errors.Trace(err)
			}
		}
		// Figure out what base we need to deploy with. For Local charms,
		// this was determined when addcharm was called.
		selectedBase, err := h.selectedBase(charmInfo.Charm(), base)
		if err != nil {
			return errors.Trace(err)
		}
		origin.Base = selectedBase
	}

	args := application.DeployArgs{
		CharmID:          chID,
		CharmOrigin:      origin,
		Cons:             cons,
		ApplicationName:  p.Application,
		NumUnits:         numUnits,
		Placement:        placement,
		ConfigYAML:       configYAML,
		Storage:          storageDirectives,
		Devices:          deviceConstraints,
		Resources:        resNames2IDs,
		EndpointBindings: p.EndpointBindings,
		Force:            h.force,
	}
	// Deploy the application.
	if err := h.deployAPI.Deploy(ctx, args); err != nil {
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

func (h *bundleHandler) storageDirectives(application string, storageMap map[string]string) (map[string]storage.Directive, error) {
	storageDirectives := h.bundleStorage[application]
	if len(storageMap) > 0 {
		if storageDirectives == nil {
			storageDirectives = make(map[string]storage.Directive)
		}
		for k, v := range storageMap {
			if _, ok := storageDirectives[k]; ok {
				// storage directives overridden
				// on the command line.
				continue
			}
			cons, err := storage.ParseDirective(v)
			if err != nil {
				return nil, errors.Annotate(err, "invalid storage directive")
			}
			storageDirectives[k] = cons
		}
	}
	return storageDirectives, nil
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

func (h *bundleHandler) selectedBase(ch charm.CharmMeta, chBase corebase.Base) (corebase.Base, error) {
	supportedBases, err := corecharm.ComputedBases(ch)
	if err != nil {
		return corebase.Base{}, errors.Trace(err)
	}
	selector, err := corecharm.ConfigureBaseSelector(corecharm.SelectorConfig{
		Config:              h.modelConfig,
		Force:               h.force,
		Logger:              logger,
		RequestedBase:       chBase,
		SupportedCharmBases: supportedBases,
		WorkloadBases:       SupportedJujuBases(),
	})
	if err != nil {
		return corebase.Base{}, errors.Trace(err)
	}
	selectedBase, err := selector.CharmBase()
	return selectedBase, errors.Trace(err)
}

// scaleApplication updates the number of units for an application.
func (h *bundleHandler) scaleApplication(ctx context.Context, change *bundlechanges.ScaleChange) error {
	if h.dryRun {
		return nil
	}

	p := change.Params

	result, err := h.deployAPI.ScaleApplication(ctx, application.ScaleApplicationParams{
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
func (h *bundleHandler) addMachine(ctx context.Context, change *bundlechanges.AddMachineChange) error {
	p := change.Params
	var (
		verbose []string
		base    corebase.Base
		err     error
	)
	if p.Base != "" {
		base, err = corebase.ParseBaseFromString(p.Base)
		if err != nil {
			return errors.Trace(err)
		}
	}
	if !base.Empty() {
		verbose = append(verbose, fmt.Sprintf("with base %q", base))
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
		apps := h.applicationsForMachineChange(change)
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
	var pBase *params.Base
	if !base.Empty() {
		pBase = &params.Base{
			Name:    base.OS,
			Channel: base.Channel.String(),
		}
	}
	machineParams := params.AddMachineParams{
		Constraints: cons,
		Base:        pBase,
		Jobs:        []model.MachineJob{model.JobHostUnits},
	}
	if ct := p.ContainerType; ct != "" {
		containerType, err := instance.ParseContainerType(ct)
		if err != nil {
			return errors.Annotatef(err, "cannot create machine for holding %s", deployedApps())
		}
		machineParams.ContainerType = containerType
		if p.ParentId != "" {
			logger.Debugf(context.TODO(), "p.ParentId: %q", p.ParentId)
			id, err := h.resolveMachine(ctx, p.ParentId)
			if err != nil {
				return errors.Annotatef(err, "cannot retrieve parent placement for %s", deployedApps())
			}
			// Never create nested containers for deployment.
			machineParams.ParentId = h.topLevelMachine(id)
		}
	}
	logger.Debugf(context.TODO(), "machineParams: %s", pretty.Sprint(machineParams))
	r, err := h.deployAPI.AddMachines(ctx, []params.AddMachineParams{machineParams})
	if err != nil {
		return errors.Annotatef(err, "cannot create machine for holding %s", deployedApps())
	}
	if r[0].Error != nil {
		return errors.Annotatef(r[0].Error, "cannot create machine for holding %s", deployedApps())
	}
	machine := r[0].Machine
	if logger.IsLevelEnabled(corelogger.DEBUG) {
		// Only do the work in for deployedApps, if debugging is enabled.
		if p.ContainerType == "" {
			logger.Debugf(context.TODO(), "created new machine %s for holding %s", machine, deployedApps())
		} else if p.ParentId == "" {
			logger.Debugf(context.TODO(), "created %s container in new machine for holding %s", machine, deployedApps())
		} else {
			logger.Debugf(context.TODO(), "created %s container in machine %s for holding %s", machine, machineParams.ParentId, deployedApps())
		}
	}
	h.results[change.Id()] = machine
	return nil
}

// addRelation creates a relationship between two applications.
func (h *bundleHandler) addRelation(ctx context.Context, change *bundlechanges.AddRelationChange) error {
	if h.dryRun {
		return nil
	}
	p := change.Params
	ep1, err := resolveRelation(p.Endpoint1, h.results)
	if err != nil {
		return errors.Errorf("attempting to apply %s without prerequists", p.Endpoint1)
	}
	ep2, err := resolveRelation(p.Endpoint2, h.results)
	if err != nil {
		return errors.Errorf("attempting to apply %s without prerequisites", p.Endpoint2)
	}
	// TODO(wallyworld) - CMR support in bundles
	_, err = h.deployAPI.AddRelation(ctx, []string{ep1, ep2}, nil)
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
func (h *bundleHandler) addUnit(ctx context.Context, change *bundlechanges.AddUnitChange) error {
	if h.dryRun {
		return nil
	}

	p := change.Params
	applicationName, ok := resolve(p.Application, h.results)
	if !ok {
		// programming error
		return errors.Errorf("attempting to apply %s without prerequisites", change.Description())
	}
	var err error
	var placementArg []*instance.Placement
	targetMachine := p.To
	if targetMachine != "" {
		logger.Debugf(context.TODO(), "addUnit: placement %q", targetMachine)
		// The placement maybe "container:machine"
		container := ""
		if parts := strings.Split(targetMachine, ":"); len(parts) > 1 {
			container = parts[0]
			targetMachine = parts[1]
		}
		targetMachine, err = h.resolveMachine(ctx, targetMachine)
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
		logger.Debugf(context.TODO(), "  resolved: placement %q", directive)
		placementArg = append(placementArg, placement)
	}
	r, err := h.deployAPI.AddUnits(ctx, application.AddUnitsParams{
		ApplicationName: applicationName,
		NumUnits:        1,
		Placement:       placementArg,
	})
	if err != nil {
		return errors.Annotatef(err, "cannot add unit for application %q", applicationName)
	}
	unit := r[0]
	if targetMachine == "" {
		logger.Debugf(context.TODO(), "added %s unit to new machine", unit)
		// In this case, the unit name is stored in results instead of the
		// machine id, which is lazily evaluated later only if required.
		// This way we avoid waiting for watcher updates.
		h.results[change.Id()] = unit
	} else {
		logger.Debugf(context.TODO(), "added %s unit to new machine", unit)
		h.results[change.Id()] = targetMachine
	}

	// Note that the targetMachine can be empty for now, resulting in a partially
	// incomplete unit status. That's ok as the missing info is provided later
	// when it is required.
	h.unitStatus[unit] = targetMachine
	return nil
}

// upgradeCharm will get the application to use the new charm.
func (h *bundleHandler) upgradeCharm(ctx context.Context, change *bundlechanges.UpgradeCharmChange) error {
	if h.dryRun {
		return nil
	}

	p := change.Params
	resolvedCharm, ok := resolve(p.Charm, h.results)
	if !ok {
		// programming error
		return errors.Errorf("attempting to apply %s without prerequisites", change.Description())
	}
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
		URL:    curl.String(),
		Origin: origin,
	}

	resNames2IDs, err := h.upgradeCharmResources(ctx, chID, p)
	if err != nil {
		return errors.Trace(err)
	}

	cfg := application.SetCharmConfig{
		ApplicationName: p.Application,
		CharmID:         chID,
		ResourceIDs:     resNames2IDs,
		Force:           h.force,
	}
	// Bundles only ever deal with the current generation.
	if err := h.deployAPI.SetCharm(ctx, cfg); err != nil {
		return errors.Trace(err)
	}
	h.writeAddedResources(resNames2IDs)

	return nil
}

func (h *bundleHandler) upgradeCharmResources(ctx context.Context, chID application.CharmID, param bundlechanges.UpgradeCharmParams) (map[string]string, error) {
	meta, err := utils.GetMetaResources(ctx, chID.URL, h.deployAPI)
	if err != nil {
		return nil, errors.Trace(err)
	}
	resMap := h.makeResourceMap(meta, param.Resources, param.LocalResources)

	resourceLister, err := resources.NewClient(h.deployAPI)
	if err != nil {
		return nil, errors.Trace(err)
	}
	filtered, err := utils.GetUpgradeResources(ctx, chID, charms.NewClient(h.deployAPI), resourceLister, param.Application, resMap, meta)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var resNames2IDs map[string]string
	if len(filtered) != 0 {
		resNames2IDs, err = h.deployResources(
			ctx,
			param.Application,
			resources.CharmID{
				URL:    chID.URL,
				Origin: chID.Origin,
			},
			resMap,
			filtered,
			h.deployAPI,
			h.filesystem,
		)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return resNames2IDs, nil
}

// setOptions updates application configuration settings.
func (h *bundleHandler) setOptions(ctx context.Context, change *bundlechanges.SetOptionsChange) error {
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

	err = h.deployAPI.SetConfig(ctx, p.Application, string(cfg), nil)
	return errors.Annotatef(err, "cannot update options for application %q", p.Application)
}

// setConstraints updates application constraints.
func (h *bundleHandler) setConstraints(ctx context.Context, change *bundlechanges.SetConstraintsChange) error {
	if h.dryRun {
		return nil
	}
	p := change.Params
	// We know that p.constraints is a valid constraints type due to the validation.
	cons, _ := constraints.Parse(p.Constraints)
	if err := h.deployAPI.SetConstraints(ctx, p.Application, cons); err != nil {
		// This should never happen, as the bundle is already verified.
		return errors.Annotatef(err, "cannot update constraints for application %q", p.Application)
	}

	return nil
}

// exposeApplication exposes an application.
func (h *bundleHandler) exposeApplication(ctx context.Context, change *bundlechanges.ExposeChange) error {
	if h.dryRun {
		return nil
	}

	application, ok := resolve(change.Params.Application, h.results)
	if !ok {
		// programming error
		return errors.Errorf("attempting to apply %s without prerequisites", change.Description())
	}
	exposedEndpoints := make(map[string]params.ExposedEndpoint)
	for endpointName, exposeDetails := range change.Params.ExposedEndpoints {
		exposedEndpoints[endpointName] = params.ExposedEndpoint{
			ExposeToSpaces: exposeDetails.ExposeToSpaces,
			ExposeToCIDRs:  exposeDetails.ExposeToCIDRs,
		}
	}

	if err := h.deployAPI.Expose(ctx, application, exposedEndpoints); err != nil {
		return errors.Annotatef(err, "cannot expose application %s", application)
	}
	return nil
}

// setAnnotations sets annotations for an application or a machine.
func (h *bundleHandler) setAnnotations(ctx context.Context, change *bundlechanges.SetAnnotationsChange) error {
	p := change.Params
	h.ctx.Verbosef("  setting annotations:")
	for key, value := range p.Annotations {
		h.ctx.Verbosef("    %s: %q", key, value)
	}
	if h.dryRun {
		return nil
	}
	eid, ok := resolve(p.Id, h.results)
	if !ok {
		// programming error
		return errors.Errorf("attempting to apply %s without prerequisites", change.Description())
	}
	var tag string
	switch p.EntityType {
	case bundlechanges.MachineType:
		tag = names.NewMachineTag(eid).String()
	case bundlechanges.ApplicationType:
		tag = names.NewApplicationTag(eid).String()
	default:
		return errors.Errorf("unexpected annotation entity type %q", p.EntityType)
	}
	result, err := h.deployAPI.SetAnnotation(ctx, map[string]map[string]string{tag: p.Annotations})
	if err == nil && len(result) > 0 {
		err = result[0].Error
	}
	if err != nil {
		return errors.Annotatef(err, "cannot set annotations for %s %q", p.EntityType, eid)
	}
	return nil
}

// createOffer creates an offer targeting one or more application endpoints.
func (h *bundleHandler) createOffer(ctx context.Context, change *bundlechanges.CreateOfferChange) error {
	if h.dryRun {
		return nil
	}

	p := change.Params
	result, err := h.deployAPI.Offer(ctx, h.targetModelUUID, p.Application, p.Endpoints, h.accountUser, p.OfferName, "")
	if err == nil && len(result) > 0 && result[0].Error != nil {
		err = result[0].Error
	}
	return err
}

// consumeOffer consumes an existing offer
func (h *bundleHandler) consumeOffer(ctx context.Context, change *bundlechanges.ConsumeOfferChange) error {
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
	if url.ModelNamespace == "" {
		url.ModelNamespace = h.accountUser
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
	consumeDetails, err := controllerOfferAPI.GetConsumeDetails(ctx, url.AsLocal().String())
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

	// construct the consume application arguments
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
			ControllerUUID: controllerTag.Id(),
			Alias:          consumeDetails.ControllerInfo.Alias,
			Addrs:          consumeDetails.ControllerInfo.Addrs,
			CACert:         consumeDetails.ControllerInfo.CACert,
		}
	}
	localName, err := h.deployAPI.Consume(ctx, arg)
	if err != nil {
		return errors.Trace(err)
	}
	h.results[change.Id()] = localName
	h.ctx.Infof("Added %s as %s", url.Path(), localName)
	return nil
}

// grantOfferAccess grants access to an offer.
func (h *bundleHandler) grantOfferAccess(ctx context.Context, change *bundlechanges.GrantOfferAccessChange) error {
	if h.dryRun {
		return nil
	}

	p := change.Params

	offerURL := fmt.Sprintf("%s.%s", h.targetModelName, p.Offer)
	if err := h.deployAPI.GrantOffer(ctx, p.User, p.Access, offerURL); err != nil && !isUserAlreadyHasAccessErr(err) {

		return errors.Annotatef(err, "cannot grant %s access to user %s on offer %s", p.Access, p.User, offerURL)
	}
	return nil
}

// applicationsForMachineChange returns the names of the applications for which an
// "addMachine" change is required, as adding machines is required to place
// units, and units belong to applications.
func (h *bundleHandler) applicationsForMachineChange(change *bundlechanges.AddMachineChange) []string {
	applications := set.NewStrings()
	// If this change is a machine, look for AddUnitParams with matching
	// baseMachine.  This will cover the machine and containers on it.
	// If this change is a container, look for AddUnitParams with matching
	// placement directive.
	match := change.Params.Machine()
	matchContainer := names.IsContainerMachine(match)
	for _, change := range h.changes {
		unitAdd, ok := change.(*bundlechanges.AddUnitChange)
		if !ok {
			continue
		}
		if matchContainer {
			if unitAdd.Params.PlacementDescription() != match {
				continue
			}
		} else if unitAdd.Params.BaseMachine() != match {
			continue
		}
		// This is for a debug statement, ignore the error.
		unitApp, _ := names.UnitApplication(unitAdd.Params.Unit())
		applications.Add(unitApp)
	}
	return applications.SortedValues()
}

// resolveMachine returns the machine id resolving the given unit or machine
// placeholder.
func (h *bundleHandler) resolveMachine(ctx context.Context, placeholder string) (string, error) {
	logger.Debugf(context.TODO(), "resolveMachine(%q)", placeholder)
	machineOrUnit, ok := resolve(placeholder, h.results)
	if !ok {
		// programming error
		return "", errors.NotFoundf("machine %s", placeholder)
	}
	if !names.IsValidUnit(machineOrUnit) {
		return machineOrUnit, nil
	}

	if h.unitStatus[machineOrUnit] != "" {
		return h.unitStatus[machineOrUnit], nil
	}

	// This should be optimized to avoid calling full status. This should really
	// use the new all watcher, but we're not there yet. For now call status
	// until we get the machine id.
	var result string
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			status, err := h.deployAPI.Status(ctx, nil)
			if err != nil {
				return errors.Annotate(err, "cannot get model status")
			}

			if status == nil {
				return errors.NotFoundf("unit %s", machineOrUnit)
			}

			for _, appData := range status.Applications {
				for unit, unitData := range appData.Units {
					if unit == machineOrUnit {
						h.unitStatus[unit] = unitData.Machine
						result = unitData.Machine
						return nil
					}
				}
			}

			return errors.NotFoundf("unit %s", machineOrUnit)
		},
		Delay:       1 * time.Second,
		MaxDuration: 5 * time.Minute,
		BackoffFunc: retry.ExpBackoff(1*time.Second, 30*time.Second, 1.5, true),
		Clock:       h.clock,
		Stop:        h.ctx.Done(),
	})
	return result, errors.Trace(err)
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
func resolveRelation(e string, results map[string]string) (string, error) {
	parts := strings.SplitN(e, ":", 2)
	application, ok := resolve(parts[0], results)
	if !ok {
		// programming error
		return "", errors.NotFoundf("application for %s", e)
	}
	if len(parts) == 1 {
		return application, nil
	}
	return fmt.Sprintf("%s:%s", application, parts[1]), nil
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
func resolve(placeholder string, results map[string]string) (string, bool) {
	logger.Debugf(context.TODO(), "resolve %q from %s", placeholder, pretty.Sprint(results))
	if !strings.HasPrefix(placeholder, "$") {
		return placeholder, true
	}
	id := placeholder[1:]
	result, ok := results[id]
	return result, ok
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
	return constraints.ArchOrDefault(cons, &b.defaultConstraints), nil
}

func verifyEndpointBindings(endpointBindings map[string]string, knownSpaceNames set.Strings) error {
	for _, spaceName := range endpointBindings {
		if !knownSpaceNames.Contains(spaceName) {
			return errors.NotFoundf("space %q", spaceName)
		}
	}
	return nil
}
