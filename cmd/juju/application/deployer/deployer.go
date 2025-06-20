// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"archive/zip"
	"context"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/client/application"
	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/storage"
)

var logger = internallogger.GetLogger("juju.cmd.juju.application.deployer")

// DeployerKind is an interface that provides CreateDeployer function to
// attempt creation of the related deployer.
type DeployerKind interface {
	CreateDeployer(ctx context.Context, d factory) (Deployer, error)
}

// localBundleDeployerKind represents a local bundle deployment
type localBundleDeployerKind struct {
	DataSource charm.BundleDataSource
}

// localPreDeployerKind represents a local pre-deployed charm deployment
type localPreDeployerKind struct {
	userCharmURL *charm.URL
	base         corebase.Base
	ch           charm.Charm
}

// localCharmDeployerKind represents a local charm deployment
type localCharmDeployerKind struct {
	base corebase.Base
	// The simplestreams stream used to identify image ids
	imageStream string
	ch          charm.Charm
	curl        *charm.URL
}

// repositoryBundleDeployerKind represents a repository bundle deployment
type repositoryBundleDeployerKind struct {
	bundleURL    *charm.URL
	bundleOrigin commoncharm.Origin
	resolver     Resolver
}

// repositoryCharmDeployerKind struct represents a repository charm deployment
type repositoryCharmDeployerKind struct {
	deployCharm deployCharm
	charmURL    *charm.URL
}

// NewDeployerFactory returns a factory setup with the API and
// function dependencies required by every deployer.
func NewDeployerFactory(dep DeployerDependencies) DeployerFactory {
	d := &factory{
		clock:                jujuclock.WallClock,
		model:                dep.Model,
		fileSystem:           dep.FileSystem,
		charmReader:          dep.CharmReader,
		newConsumeDetailsAPI: dep.NewConsumeDetailsAPI,
	}
	if dep.DeployResources == nil {
		d.deployResources = DeployResources
	}
	return d
}

// GetDeployer returns the correct deployer to use based on the cfg provided.
// A CharmDeployAPI is needed to find the deployer.
func (d *factory) GetDeployer(ctx context.Context, cfg DeployerConfig, deployAPI CharmDeployAPI, resolver Resolver) (Deployer, error) {
	// Determine the type of deploy we have
	var dk DeployerKind

	// Set the factory config
	d.setConfig(cfg)

	// Check the path and try to catch problems (e.g. ambiguity) and fail early
	if fileStatErr := d.checkPath(); fileStatErr != nil {
		return nil, errors.Trace(fileStatErr)
	}

	maybeCharmOrBundlePath, _ := strings.CutPrefix(d.charmOrBundle, "local:")
	if charm.IsValidLocalCharmOrBundlePath(maybeCharmOrBundlePath) {
		// Ensure charmOrBundle for local charms does not include
		// "local:" prefix
		d.charmOrBundle = maybeCharmOrBundlePath

		// Go for local bundle
		var localBundleErr error
		if dk, localBundleErr = d.localBundleDeployer(); localBundleErr != nil {
			return nil, errors.Trace(localBundleErr)
		}

		// Go for local charm (if it's not set by the localBundleDeployer above)
		if dk == nil {
			var localCharmErr error
			if dk, localCharmErr = d.localCharmDeployer(ctx, deployAPI); localCharmErr != nil {
				return nil, errors.Trace(localCharmErr)
			}
		}
	} else if isLocalSchema(d.charmOrBundle) {
		// Go for local pre-deployed charm
		var localPreDeployedCharmErr error
		if dk, localPreDeployedCharmErr = d.localPreDeployedCharmDeployer(ctx, deployAPI); localPreDeployedCharmErr != nil {
			return nil, errors.Trace(localPreDeployedCharmErr)
		}
	} else {
		// Repository charm or bundle
		userCharmURL, resolveCharmErr := resolveCharmURL(d.charmOrBundle, d.defaultCharmSchema)
		if resolveCharmErr != nil {
			return nil, errors.Trace(resolveCharmErr)
		}

		charmHubSchemaCheck := charm.CharmHub.Matches(userCharmURL.Schema)

		// Check revision
		revision, revErr := d.checkHandleRevision(userCharmURL, charmHubSchemaCheck)
		if revErr != nil {
			return nil, errors.Trace(revErr)
		}

		// Make the origin
		platform := utils.MakePlatform(d.constraints, d.base, d.modelConstraints)
		origin, err := utils.MakeOrigin(charm.CharmHub, revision, d.channel, platform)
		if err != nil {
			return nil, errors.Trace(err)
		}

		// Go for repository bundle
		var bundleErr error
		if dk, bundleErr = d.repoBundleDeployer(ctx, userCharmURL, origin, resolver, charmHubSchemaCheck); bundleErr != nil && !errors.Is(bundleErr, errors.NotValid) {
			// If the error is NotValid, then the URL is resolved alright, but not to a bundle, so no need to raise
			return nil, errors.Trace(bundleErr)
		}

		// Go for repository charm (if it's not set by the repoBundleDeployer above)
		if dk == nil {
			var charmErr error
			dk, charmErr = d.repoCharmDeployer(userCharmURL, origin, charmHubSchemaCheck)
			if charmErr != nil {
				return nil, charmErr
			}
		}
	}

	return dk.CreateDeployer(ctx, *d)
}

func (d *factory) repoCharmDeployer(userCharmURL *charm.URL, origin commoncharm.Origin, charmHubSchemaCheck bool) (DeployerKind, error) {
	// Check for when revision is set without a channel for future upgrades
	if d.revision != -1 && charmHubSchemaCheck && d.channel.Empty() {
		return nil, errors.Errorf("specifying a revision requires a channel for future upgrades. Please use --channel")
	}
	deployCharm := d.newDeployCharm()
	deployCharm.id = application.CharmID{
		Origin: origin,
	}
	return &repositoryCharmDeployerKind{deployCharm, userCharmURL}, nil
}

func (d *factory) repoBundleDeployer(ctx context.Context, userCharmURL *charm.URL, origin commoncharm.Origin, resolver Resolver, charmHubSchemaCheck bool) (DeployerKind, error) {
	// TODO (cderici): check the validity of the comment below
	// Resolve the bundle URL using the channel supplied via the channel
	// supplied. All charms within this bundle unless pinned via a channel are
	// NOT expected to be in the same channel as the bundle channel.
	// The pinning of a bundle does not flow down to charms as well. Each charm
	// has it's own channel supplied via a bundle, if no is supplied then the
	// channel is worked out via the resolving what is available.
	// See: LP:1677404 and LP:1832873
	bundleURL, bundleOrigin, bundleResolveErr := resolver.ResolveBundleURL(ctx, userCharmURL, origin)
	if corecharm.IsUnsupportedBaseError(errors.Cause(bundleResolveErr)) {
		return nil, errors.Errorf("%v. Use --force to deploy the charm anyway.", bundleResolveErr)
	}
	if bundleResolveErr != nil {
		return nil, errors.Trace(bundleResolveErr)
	} else {
		if d.revision != -1 && charmHubSchemaCheck {
			if !d.channel.Empty() {
				return nil, errors.Errorf("revision and channel are mutually exclusive when deploying a bundle. Please choose one.")
			}
		}
		if err := d.validateBundleFlags(); err != nil {
			return nil, errors.Trace(err)
		}
		// Set the deployer kind to repositoryBundleDeployerKind
		return &repositoryBundleDeployerKind{bundleURL, bundleOrigin, resolver}, nil
	}
}

func (d *factory) localBundleDeployer() (DeployerKind, error) {
	if ds, localBundleDataErr := charm.LocalBundleDataSource(d.charmOrBundle); localBundleDataErr == nil {
		// Set the deployer kind to localBundleDeployerKind
		return &localBundleDeployerKind{DataSource: ds}, nil
	} else if !errors.Is(localBundleDataErr, errors.NotFound) {
		// Only raise if it's not a NotFound.
		// Otherwise, no need to raise, it's not a bundle,
		// continue with trying for local charm.
		return nil, errors.Annotatef(localBundleDataErr, "cannot deploy bundle")
	} else {
		return nil, nil
	}
}

func (d *factory) localCharmDeployer(ctx context.Context, getter ModelConfigGetter) (DeployerKind, error) {
	// Charm may have been supplied via a path reference.
	ch, curl, err := d.charmReader.NewCharmAtPath(d.charmOrBundle)

	// We check for several types of known error which indicate
	// that the supplied reference was indeed a path but there was
	// an issue reading the charm located there.
	if errors.Is(err, corecharm.MissingBaseError) {
		return nil, err
	} else if corecharm.IsUnsupportedBaseError(err) {
		return nil, errors.Trace(err)
	} else if errors.Cause(err) == zip.ErrFormat {
		return nil, errors.Errorf("invalid charm or bundle provided at %q", d.charmOrBundle)
	} else if errors.Is(err, errors.NotFound) {
		return nil, errors.Wrap(err, errors.NotFoundf("charm or bundle at %q", d.charmOrBundle))
	} else if errors.Is(err, os.ErrNotExist) {
		logger.Debugf(context.TODO(), "cannot interpret as local charm: %v", err)
		return nil, nil
	} else if err != nil {
		// If we get a "not exists" error then we attempt to interpret
		// the supplied charm reference as a URL elsewhere, otherwise
		// we return the error.
		return nil, errors.Annotatef(err, "attempting to deploy %q", d.charmOrBundle)
	}

	base, imageStream, err := d.determineBaseForCharm(ctx, ch, getter)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &localCharmDeployerKind{base, imageStream, ch, curl}, nil
}

func (d *factory) localPreDeployedCharmDeployer(ctx context.Context, deployAPI CharmDeployAPI) (DeployerKind, error) {
	// If the charm's schema is local, we should definitively attempt
	// to deploy a charm that's already deployed in the
	// environment.
	userCharmURL, resolveCharmErr := resolveCharmURL(d.charmOrBundle, d.defaultCharmSchema)
	if resolveCharmErr != nil {
		return nil, errors.Trace(resolveCharmErr)
	}
	if !charm.Local.Matches(userCharmURL.Schema) {
		return nil, errors.Errorf("cannot interpret as a redeployment of a local charm from the controller")
	}

	charmInfo, err := deployAPI.CharmInfo(ctx, userCharmURL.String())
	if err != nil {
		return nil, errors.Trace(err)
	}
	ch := charmInfo.Charm()

	base, _, err := d.determineBaseForCharm(ctx, ch, deployAPI)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &localPreDeployerKind{base: base, userCharmURL: userCharmURL, ch: ch}, nil
}

func (d *factory) determineBaseForCharm(ctx context.Context, ch charm.Charm, getter ModelConfigGetter) (corebase.Base, string, error) {
	var (
		selectedBase corebase.Base
	)
	modelCfg, err := getModelConfig(ctx, getter)
	if err != nil {
		return corebase.Base{}, "", errors.Trace(err)
	}

	supportedBases, err := corecharm.ComputedBases(ch)
	if err != nil {
		return corebase.Base{}, "", errors.Trace(err)
	}
	baseSelector, err := corecharm.ConfigureBaseSelector(corecharm.SelectorConfig{
		Config:              modelCfg,
		Force:               d.force,
		Logger:              logger,
		RequestedBase:       d.base,
		SupportedCharmBases: supportedBases,
		WorkloadBases:       SupportedJujuBases(),
		UsingImageID:        d.constraints.HasImageID() || d.modelConstraints.HasImageID(),
	})
	if err != nil {
		return corebase.Base{}, "", errors.Trace(err)
	}

	selectedBase, err = baseSelector.CharmBase()
	if err = charmValidationError(ch.Meta().Name, errors.Trace(err)); err != nil {
		return corebase.Base{}, "", errors.Trace(err)
	}
	return selectedBase, modelCfg.ImageStream(), nil
}

func (d *factory) checkHandleRevision(userCharmURL *charm.URL, charmHubSchemaCheck bool) (int, error) {
	// To deploy by revision, the revision number must be in the origin for a charmhub bundle
	if userCharmURL.Revision != -1 && charmHubSchemaCheck {
		return -1, errors.Errorf("cannot specify revision in a charm or bundle name. Please use --revision.")
	}

	if d.revision != -1 {
		return d.revision, nil
	}
	return userCharmURL.Revision, nil
}

func (d *factory) checkPath() error {
	_, fileStatErr := d.fileSystem.Stat(d.charmOrBundle)
	// Check for path ambiguity where we don't have a valid local path,
	// but such a path does exist
	if fileStatErr == nil && !charm.IsValidLocalCharmOrBundlePath(d.charmOrBundle) {
		return errors.Errorf(""+
			"The charm or bundle %q is ambiguous.\n"+
			"To deploy a local charm or bundle, run `juju deploy ./%[1]s`.\n"+
			"To deploy a charm or bundle from CharmHub, run `juju deploy ch:%[1]s`.",
			d.charmOrBundle,
		)
	}
	// Check in case we do have a valid path, but it doesn't exist
	if fileStatErr != nil && charm.IsValidLocalCharmOrBundlePath(d.charmOrBundle) && os.IsNotExist(errors.Cause(fileStatErr)) {
		return errors.Errorf("no charm was found at %q", d.charmOrBundle)
	}
	return nil
}

func (d *factory) setConfig(cfg DeployerConfig) {
	d.placementSpec = cfg.PlacementSpec
	d.placement = cfg.Placement
	d.numUnits = cfg.NumUnits
	d.attachStorage = cfg.AttachStorage
	d.charmOrBundle = cfg.CharmOrBundle
	d.defaultCharmSchema = cfg.DefaultCharmSchema
	d.bundleOverlayFile = cfg.BundleOverlayFile
	d.channel = cfg.Channel
	d.base = cfg.Base
	d.force = cfg.Force
	d.dryRun = cfg.DryRun
	d.applicationName = cfg.ApplicationName
	d.configOptions = cfg.ConfigOptions
	d.constraints = cfg.Constraints
	d.modelConstraints = cfg.ModelConstraints
	d.storage = cfg.Storage
	d.bundleStorage = cfg.BundleStorage
	d.devices = cfg.Devices
	d.bundleDevices = cfg.BundleDevices
	d.resources = cfg.Resources
	d.revision = cfg.Revision
	d.bindings = cfg.Bindings
	d.useExisting = cfg.UseExisting
	d.bundleMachines = cfg.BundleMachines
	d.trust = cfg.Trust
	d.flagSet = cfg.FlagSet
}

// DeployerDependencies are required for any deployer to be run.
type DeployerDependencies struct {
	DeployResources      DeployResourcesFunc
	Model                ModelCommand
	FileSystem           modelcmd.Filesystem
	CharmReader          CharmReader
	NewConsumeDetailsAPI func(ctx context.Context, url *crossmodel.OfferURL) (ConsumeDetails, error)
	DeployKind           DeployerFactory
}

// DeployerConfig is the data required to choose a deployer and then run
// PrepareAndDeploy.
// TODO (hml) 2020-08-14
// Is there a better structure for this config?
type DeployerConfig struct {
	Model                ModelCommand
	ApplicationName      string
	AttachStorage        []string
	Bindings             map[string]string
	BindToSpaces         string
	BundleDevices        map[string]map[string]devices.Constraints
	BundleMachines       map[string]string
	BundleOverlayFile    []string
	BundleStorage        map[string]map[string]storage.Directive
	Channel              charm.Channel
	CharmOrBundle        string
	DefaultCharmSchema   charm.Schema
	ConfigOptions        common.ConfigFlag
	ConstraintsStr       string
	Constraints          constraints.Value
	ModelConstraints     constraints.Value
	Devices              map[string]devices.Constraints
	DeployResources      DeployResourcesFunc
	DryRun               bool
	FlagSet              *gnuflag.FlagSet
	Force                bool
	NewConsumeDetailsAPI func(url *crossmodel.OfferURL) (ConsumeDetails, error)
	NumUnits             int
	PlacementSpec        string
	Placement            []*instance.Placement
	Resources            map[string]string
	Revision             int
	Base                 corebase.Base
	Storage              map[string]storage.Directive
	Trust                bool
	UseExisting          bool
}

type factory struct {
	// DeployerDependencies
	model                ModelCommand
	deployResources      DeployResourcesFunc
	newConsumeDetailsAPI func(ctx context.Context, url *crossmodel.OfferURL) (ConsumeDetails, error)
	fileSystem           modelcmd.Filesystem
	charmReader          CharmReader

	// DeployerConfig
	defaultCharmSchema charm.Schema
	placementSpec      string
	placement          []*instance.Placement
	numUnits           int
	attachStorage      []string
	charmOrBundle      string
	bundleOverlayFile  []string
	channel            charm.Channel
	revision           int
	base               corebase.Base
	force              bool
	dryRun             bool
	applicationName    string
	configOptions      common.ConfigFlag
	constraints        constraints.Value
	modelConstraints   constraints.Value
	storage            map[string]storage.Directive
	bundleStorage      map[string]map[string]storage.Directive
	devices            map[string]devices.Constraints
	bundleDevices      map[string]map[string]devices.Constraints
	resources          map[string]string
	bindings           map[string]string
	useExisting        bool
	bundleMachines     map[string]string
	trust              bool
	flagSet            *gnuflag.FlagSet

	// Private
	clock jujuclock.Clock
}

// newDeployCharm returns the config needed to eventually call
// deployCharm.deploy.  This is used by all types of charms to
// be deployed
func (d *factory) newDeployCharm() deployCharm {
	return deployCharm{
		applicationName:  d.applicationName,
		attachStorage:    d.attachStorage,
		bindings:         d.bindings,
		configOptions:    &d.configOptions,
		constraints:      d.constraints,
		dryRun:           d.dryRun,
		modelConstraints: d.modelConstraints,
		devices:          d.devices,
		deployResources:  d.deployResources,
		flagSet:          d.flagSet,
		force:            d.force,
		model:            d.model,
		numUnits:         d.numUnits,
		placement:        d.placement,
		placementSpec:    d.placementSpec,
		resources:        d.resources,
		baseFlag:         d.base,
		storage:          d.storage,
		trust:            d.trust,
	}
}

func (dt *localBundleDeployerKind) CreateDeployer(_ context.Context, d factory) (Deployer, error) {
	if err := d.validateBundleFlags(); err != nil {
		return nil, errors.Trace(err)
	}

	platform := utils.MakePlatform(d.constraints, d.base, d.modelConstraints)
	db := d.newDeployBundle(d.defaultCharmSchema, dt.DataSource)
	var base corebase.Base
	var err error
	if platform.Channel != "" {
		base, err = corebase.ParseBase(platform.OS, platform.Channel)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	db.origin = commoncharm.Origin{
		Source:       commoncharm.OriginLocal,
		Architecture: platform.Architecture,
		Base:         base,
	}
	return &localBundle{deployBundle: db}, nil
}

// newDeployBundle returns the config needed to eventually call
// deployBundle.deploy.  This is used by all types of bundles to
// be deployed
func (d *factory) newDeployBundle(_ charm.Schema, ds charm.BundleDataSource) deployBundle {
	return deployBundle{
		model:                d.model,
		dryRun:               d.dryRun,
		force:                d.force,
		trust:                d.trust,
		bundleDataSource:     ds,
		newConsumeDetailsAPI: d.newConsumeDetailsAPI,
		deployResources:      d.deployResources,
		useExistingMachines:  d.useExisting,
		bundleMachines:       d.bundleMachines,
		bundleStorage:        d.bundleStorage,
		bundleDevices:        d.bundleDevices,
		bundleOverlayFile:    d.bundleOverlayFile,
		bundleDir:            d.charmOrBundle,
		modelConstraints:     d.modelConstraints,
		charmReader:          d.charmReader,
		defaultCharmSchema:   d.defaultCharmSchema,
	}
}

func (dk *localPreDeployerKind) CreateDeployer(ctx context.Context, d factory) (Deployer, error) {
	if err := d.validateResourcesNeededForLocalDeploy(ctx, dk.ch.Meta()); err != nil {
		return nil, errors.Trace(err)
	}
	return &predeployedLocalCharm{
		deployCharm:  d.newDeployCharm(),
		userCharmURL: dk.userCharmURL,
		base:         dk.base,
	}, nil
}

func (dk *localCharmDeployerKind) CreateDeployer(ctx context.Context, d factory) (Deployer, error) {
	// Avoid deploying charm if the charm base is not correct for the
	// available image streams.
	var err error
	if err := d.validateResourcesNeededForLocalDeploy(ctx, dk.ch.Meta()); err != nil {
		return nil, errors.Trace(err)
	}

	return &localCharm{
		deployCharm: d.newDeployCharm(),
		curl:        dk.curl,
		base:        dk.base,
		ch:          dk.ch,
	}, err
}

func (dk *repositoryCharmDeployerKind) CreateDeployer(_ context.Context, d factory) (Deployer, error) {
	return &repositoryCharm{
		deployCharm:                    dk.deployCharm,
		userRequestedURL:               dk.charmURL,
		clock:                          d.clock,
		uploadExistingPendingResources: UploadExistingPendingResources,
	}, nil

}

func (dk *repositoryBundleDeployerKind) CreateDeployer(ctx context.Context, d factory) (Deployer, error) {

	// Validated, prepare to Deploy
	// TODO(bundles) - Ideally, we would like to expose a GetBundleDataSource method for the charmstore.
	// As a short-term solution and given that we don't
	// want to support (for now at least) multi-doc bundles
	// from the charmstore we simply use the existing
	// charmrepo.v4 API to read the base bundle and
	// wrap it in a BundleDataSource for use by deployBundle.
	dir, err := os.MkdirTemp("", dk.bundleURL.Name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	bundle, err := dk.resolver.GetBundle(ctx, dk.bundleURL, dk.bundleOrigin, filepath.Join(dir, dk.bundleURL.Name))
	if err != nil {
		return nil, errors.Trace(err)
	}

	db := d.newDeployBundle(d.defaultCharmSchema, store.NewResolvedBundle(bundle))
	db.bundleURL = dk.bundleURL
	db.bundleOverlayFile = d.bundleOverlayFile
	db.origin = dk.bundleOrigin
	return &repositoryBundle{deployBundle: db}, nil
}

func resolveCharmURL(path string, defaultSchema charm.Schema) (*charm.URL, error) {
	var err error
	path, err = charm.EnsureSchema(path, defaultSchema)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return charm.ParseURL(path)
}

func isLocalSchema(u string) bool {
	raw, err := url.Parse(u)
	if err != nil {
		return false
	}
	switch charm.Schema(raw.Scheme) {
	case charm.Local:
		return true
	}

	return false
}

func appsRequiringTrust(appSpecList map[string]*charm.ApplicationSpec) []string {
	var tl []string
	for a, appSpec := range appSpecList {
		if applicationRequiresTrust(appSpec) {
			tl = append(tl, a)
		}
	}

	// Since map iterations are random we should sort the list to ensure
	// consistent output in any errors containing the returned list contents.
	sort.Strings(tl)
	return tl
}

var getModelConfig = func(ctx context.Context, api ModelConfigGetter) (*config.Config, error) {
	// Separated into a variable for easy overrides
	attrs, err := api.ModelGet(ctx)
	if err != nil {
		return nil, errors.Wrap(err, errors.New("cannot fetch model settings"))
	}

	return config.New(config.NoDefaults, attrs)
}

// charmValidationError consumes an error along with a charmSeries and name
// to help provide better feedback to the user when somethings gone wrong around
// validating a charm validation
func charmValidationError(name string, err error) error {
	if errors.Is(err, errors.NotSupported) {
		return errors.Errorf("%v is not available on the following %v", name, err)
	}
	return errors.Trace(err)
}

func (d *factory) validateResourcesNeededForLocalDeploy(ctx context.Context, charmMeta *charm.Meta) error {
	modelType, err := d.model.ModelType(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	// If the target model is IAAS, then we don't need to validate the resources
	// for a given deploy.
	if modelType == model.IAAS {
		return nil
	}

	var missingImages []string
	for resName, resMeta := range charmMeta.Resources {
		if resMeta.Type == charmresource.TypeContainerImage {
			if _, ok := d.resources[resName]; !ok {
				missingImages = append(missingImages, resName)
			}
		}
	}
	if len(missingImages) > 0 {
		return errors.Errorf("local charm missing OCI images for: %v", strings.Join(missingImages, ", "))
	}
	return nil
}

func (d *factory) validateBundleFlags() error {
	if flags := utils.GetFlags(d.flagSet, CharmOnlyFlags()); len(flags) > 0 {
		return errors.Errorf("options provided but not supported when deploying a bundle: %s", strings.Join(flags, ", "))
	}
	return nil
}

// CharmOnlyFlags and BundleOnlyFlags are used to validate flags based on
// whether we are deploying a charm or a bundle.
func CharmOnlyFlags() []string {
	charmOnlyFlags := []string{
		"bind", "config", "constraints", "n", "num-units",
		"base", "to", "resource", "attach-storage",
	}

	return charmOnlyFlags
}
