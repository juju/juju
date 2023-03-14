// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"archive/zip"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juju/charm/v10"
	charmresource "github.com/juju/charm/v10/resource"
	jujuclock "github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/client/application"
	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"
)

var logger = loggo.GetLogger("juju.cmd.juju.application.deployer")

// NewDeployerFactory returns a factory setup with the API and
// function dependencies required by every deployer.
func NewDeployerFactory(dep DeployerDependencies) DeployerFactory {
	d := &factory{
		clock:                jujuclock.WallClock,
		model:                dep.Model,
		fileSystem:           dep.FileSystem,
		charmReader:          dep.CharmReader,
		newConsumeDetailsAPI: dep.NewConsumeDetailsAPI,
		steps:                dep.Steps,
	}
	if dep.DeployResources == nil {
		d.deployResources = DeployResources
	}
	return d
}

// GetDeployer returns the correct deployer to use based on the cfg provided.
// A ModelConfigGetter needed to find the deployer.
func (d *factory) GetDeployer(cfg DeployerConfig, getter ModelConfigGetter, resolver Resolver) (Deployer, error) {
	d.setConfig(cfg)
	maybeDeployers := []func() (Deployer, error){
		d.maybeReadLocalBundle,
		func() (Deployer, error) { return d.maybeReadLocalCharm(getter) },
		d.maybePredeployedLocalCharm,
		func() (Deployer, error) { return d.maybeReadRepositoryBundle(resolver) },
		d.repositoryCharm, // This always returns a Deployer
	}
	for _, d := range maybeDeployers {
		if deploy, err := d(); err != nil {
			return nil, errors.Trace(err)
		} else if deploy != nil {
			return deploy, nil
		}
	}
	return nil, errors.NotFoundf("suitable Deployer")
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
	NewConsumeDetailsAPI func(url *charm.OfferURL) (ConsumeDetails, error)
	Steps                []DeployStep
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
	BundleStorage        map[string]map[string]storage.Constraints
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
	NewConsumeDetailsAPI func(url *charm.OfferURL) (ConsumeDetails, error)
	NumUnits             int
	PlacementSpec        string
	Placement            []*instance.Placement
	Resources            map[string]string
	Revision             int
	Base                 series.Base
	Storage              map[string]storage.Constraints
	Trust                bool
	UseExisting          bool
}

type factory struct {
	// DeployerDependencies
	model                ModelCommand
	deployResources      DeployResourcesFunc
	newConsumeDetailsAPI func(url *charm.OfferURL) (ConsumeDetails, error)
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
	base               series.Base
	force              bool
	dryRun             bool
	applicationName    string
	configOptions      common.ConfigFlag
	constraints        constraints.Value
	modelConstraints   constraints.Value
	storage            map[string]storage.Constraints
	bundleStorage      map[string]map[string]storage.Constraints
	devices            map[string]devices.Constraints
	bundleDevices      map[string]map[string]devices.Constraints
	resources          map[string]string
	bindings           map[string]string
	steps              []DeployStep
	useExisting        bool
	bundleMachines     map[string]string
	trust              bool
	flagSet            *gnuflag.FlagSet

	// Private
	clock jujuclock.Clock
}

func (d *factory) maybePredeployedLocalCharm() (Deployer, error) {
	// If the charm's schema is local, we should definitively attempt
	// to deploy a charm that's already deployed in the
	// environment.
	userCharmURL, err := resolveCharmURL(d.charmOrBundle, d.defaultCharmSchema)
	if err != nil {
		if _, err := d.fileSystem.Stat(d.charmOrBundle); os.IsNotExist(errors.Cause(err)) {
			return nil, errors.Errorf("no charm was found at %q", d.charmOrBundle)
		}
		return nil, errors.Trace(err)
	} else if userCharmURL.Schema != "local" {
		logger.Debugf("cannot interpret as a redeployment of a local charm from the controller")
		return nil, nil
	}

	return &predeployedLocalCharm{
		deployCharm:  d.newDeployCharm(),
		userCharmURL: userCharmURL,
	}, nil
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
		steps:            d.steps,
		storage:          d.storage,
		trust:            d.trust,

		validateCharmSeriesWithName:           d.validateCharmSeriesWithName,
		validateResourcesNeededForLocalDeploy: d.validateResourcesNeededForLocalDeploy,
	}
}

func (d *factory) maybeReadLocalBundle() (Deployer, error) {
	bundleFile := d.charmOrBundle
	_, statErr := d.model.Filesystem().Stat(bundleFile)
	if statErr == nil && !charm.IsValidLocalCharmOrBundlePath(bundleFile) {
		return nil, errors.Errorf(""+
			"The charm or bundle %q is ambiguous.\n"+
			"To deploy a local charm or bundle, run `juju deploy ./%[1]s`.\n"+
			"To deploy a charm or bundle from CharmHub, run `juju deploy ch:%[1]s`.",
			bundleFile,
		)
	}

	ds, err := charm.LocalBundleDataSource(bundleFile)
	if errors.IsNotFound(err) {
		// Not a local bundle. Return nil, nil to indicate the fallback
		// pipeline should try the next possibility.
		return nil, nil
	}
	if err != nil {
		return nil, errors.Annotate(err, "cannot deploy bundle")
	}
	if err := d.validateBundleFlags(); err != nil {
		return nil, errors.Trace(err)
	}

	platform := utils.MakePlatform(d.constraints, d.base, d.modelConstraints)
	db := d.newDeployBundle(d.defaultCharmSchema, ds)
	var base series.Base
	if platform.Channel != "" {
		base, err = series.ParseBase(platform.OS, platform.Channel)
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
		steps:                d.steps,
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

func (d *factory) maybeReadLocalCharm(getter ModelConfigGetter) (Deployer, error) {
	// NOTE: Here we select the series using the algorithm defined by
	// `seriesSelector.charmSeries`. This serves to override the algorithm found
	// in `charmrepo.NewCharmAtPath` which is outdated (but must still be
	// called since the code is coupled with path interpretation logic which
	// cannot easily be factored out).

	// NOTE: Reading the charm here is only meant to aid in inferring the
	// correct series, if this fails we fall back to the argument series. If
	// reading the charm fails here it will also fail below (the charm is read
	// again below) where it is handled properly. This is just an expedient to
	// get the correct series. A proper refactoring of the charmrepo package is
	// needed for a more elegant fix.
	charmOrBundle := d.charmOrBundle
	if isLocalSchema(charmOrBundle) {
		charmOrBundle = charmOrBundle[6:]
	}

	var (
		imageStream string
		seriesName  string
	)
	if !d.base.Empty() {
		var err error
		seriesName, err = series.GetSeriesFromBase(d.base)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	ch, err := d.charmReader.ReadCharm(charmOrBundle)
	if err == nil {
		modelCfg, err := getModelConfig(getter)
		if err != nil {
			return nil, errors.Trace(err)
		}

		imageStream = modelCfg.ImageStream()
		workloadSeries, err := SupportedJujuSeries(d.clock.Now(), seriesName, imageStream)
		if err != nil {
			return nil, errors.Trace(err)
		}

		supportedSeries, err := corecharm.ComputedSeries(ch)
		if err != nil {
			return nil, errors.Trace(err)
		}
		seriesSelector := corecharm.SeriesSelector{
			SeriesFlag:          seriesName,
			SupportedSeries:     supportedSeries,
			SupportedJujuSeries: workloadSeries,
			Force:               d.force,
			Conf:                modelCfg,
			FromBundle:          false,
			Logger:              logger,
		}

		if len(supportedSeries) == 0 {
			logger.Warningf("%s does not declare supported series in metadata.yml", ch.Meta().Name)
		}

		seriesName, err = seriesSelector.CharmSeries()
		if err = charmValidationError(ch.Meta().Name, errors.Trace(err)); err != nil {
			return nil, errors.Trace(err)
		}
	}

	// Charm may have been supplied via a path reference.
	ch, curl, err := corecharm.NewCharmAtPathForceSeries(charmOrBundle, seriesName, d.force)
	// We check for several types of known error which indicate
	// that the supplied reference was indeed a path but there was
	// an issue reading the charm located there.
	if corecharm.IsMissingSeriesError(err) {
		return nil, err
	} else if corecharm.IsUnsupportedSeriesError(err) {
		return nil, errors.Trace(err)
	} else if errors.Cause(err) == zip.ErrFormat {
		return nil, errors.Errorf("invalid charm or bundle provided at %q", charmOrBundle)
	} else if errors.IsNotFound(err) {
		return nil, errors.Wrap(err, errors.NotFoundf("charm or bundle at %q", charmOrBundle))
	} else if err != nil && err != os.ErrNotExist {
		// If we get a "not exists" error then we attempt to interpret
		// the supplied charm reference as a URL elsewhere, otherwise
		// we return the error.
		return nil, errors.Annotatef(err, "attempting to deploy %q", charmOrBundle)
	} else if err != nil {
		logger.Debugf("cannot interpret as local charm: %v", err)
		return nil, nil
	}

	// Avoid deploying charm if the charm series is not correct for the
	// available image streams.
	if err := d.validateCharmSeriesWithName(seriesName, curl.Name, imageStream); err != nil {
		return nil, errors.Trace(err)
	}
	if err := d.validateResourcesNeededForLocalDeploy(ch.Meta()); err != nil {
		return nil, errors.Trace(err)
	}

	return &localCharm{
		deployCharm: d.newDeployCharm(),
		curl:        curl,
		ch:          ch,
	}, err
}

func (d *factory) maybeReadRepositoryBundle(resolver Resolver) (Deployer, error) {
	curl, err := resolveCharmURL(d.charmOrBundle, d.defaultCharmSchema)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// To deploy by revision, the revision number must be in the origin for a
	// charmhub bundle and in the url for a charmstore bundle.
	if curl.Revision != -1 && charm.CharmHub.Matches(curl.Schema) {
		return nil, errors.Errorf("cannot specify revision in a charm or bundle name. Please use --revision.")
	}

	urlForOrigin := curl
	if d.revision != -1 {
		urlForOrigin = urlForOrigin.WithRevision(d.revision)
	}

	platform := utils.MakePlatform(d.constraints, d.base, d.modelConstraints)
	origin, err := utils.DeduceOrigin(urlForOrigin, d.channel, platform)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Resolve the bundle URL using the channel supplied via the channel
	// supplied. All charms within this bundle unless pinned via a channel are
	// NOT expected to be in the same channel as the bundle channel.
	// The pinning of a bundle does not flow down to charms as well. Each charm
	// has it's own channel supplied via a bundle, if no is supplied then the
	// channel is worked out via the resolving what is available.
	// See: LP:1677404 and LP:1832873
	bundleURL, bundleOrigin, err := resolver.ResolveBundleURL(curl, origin)
	if charm.IsUnsupportedSeriesError(errors.Cause(err)) {
		return nil, errors.Errorf("%v. Use --force to deploy the charm anyway.", err)
	}
	if errors.IsNotValid(err) {
		// The URL resolved alright, but not to a bundle.
		return nil, nil
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	if d.revision != -1 && !d.channel.Empty() && charm.CharmHub.Matches(curl.Schema) {
		return nil, errors.Errorf("revision and channel are mutually exclusive when deploying a bundle. Please choose one.")
	}
	if err := d.validateBundleFlags(); err != nil {
		return nil, errors.Trace(err)
	}

	// Validated, prepare to Deploy
	// TODO(bundles) - Ideally, we would like to expose a GetBundleDataSource method for the charmstore.
	// As a short-term solution and given that we don't
	// want to support (for now at least) multi-doc bundles
	// from the charmstore we simply use the existing
	// charmrepo.v4 API to read the base bundle and
	// wrap it in a BundleDataSource for use by deployBundle.
	dir, err := os.MkdirTemp("", bundleURL.Name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	bundle, err := resolver.GetBundle(bundleURL, bundleOrigin, filepath.Join(dir, bundleURL.Name))
	if err != nil {
		return nil, errors.Trace(err)
	}

	db := d.newDeployBundle(d.defaultCharmSchema, store.NewResolvedBundle(bundle))
	db.bundleURL = bundleURL
	db.bundleOverlayFile = d.bundleOverlayFile
	db.origin = bundleOrigin
	return &repositoryBundle{deployBundle: db}, nil
}

func (d *factory) repositoryCharm() (Deployer, error) {
	// Validate we have a charm store change.
	userRequestedURL, err := resolveCharmURL(d.charmOrBundle, d.defaultCharmSchema)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if charm.CharmHub.Matches(userRequestedURL.Schema) && d.channel.Empty() && d.revision != -1 {
		// Tell the user they need to specify a channel
		return nil, errors.Errorf("specifying a revision requires a channel for future upgrades. Please use --channel")
	}
	// To deploy by revision, the revision number must be in the origin for a
	// charmhub charm.
	if charm.CharmHub.Matches(userRequestedURL.Schema) {
		if userRequestedURL.Revision != -1 {
			return nil, errors.Errorf("cannot specify revision in a charm or bundle name. Please use --revision.")
		}
	}

	urlForOrigin := userRequestedURL
	if d.revision != -1 {
		urlForOrigin = urlForOrigin.WithRevision(d.revision)
	}
	platform := utils.MakePlatform(d.constraints, d.base, d.modelConstraints)
	origin, err := utils.DeduceOrigin(urlForOrigin, d.channel, platform)
	if err != nil {
		return nil, errors.Trace(err)
	}

	deployCharm := d.newDeployCharm()
	deployCharm.id = application.CharmID{
		Origin: origin,
	}
	return &repositoryCharm{
		deployCharm:      deployCharm,
		userRequestedURL: userRequestedURL,
		clock:            d.clock,
	}, nil
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

func seriesSelectorRequirements(api ModelConfigGetter, cl jujuclock.Clock, chURL *charm.URL) (*config.Config, set.Strings, error) {
	// resolver.resolve potentially updates the series of anything
	// passed in. Store this for use in seriesSelector.
	userRequestedSeries := chURL.Series

	modelCfg, err := getModelConfig(api)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	imageStream := modelCfg.ImageStream()
	workloadSeries, err := SupportedJujuSeries(cl.Now(), userRequestedSeries, imageStream)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	return modelCfg, workloadSeries, nil
}

var getModelConfig = func(api ModelConfigGetter) (*config.Config, error) {
	// Separated into a variable for easy overrides
	attrs, err := api.ModelGet()
	if err != nil {
		return nil, errors.Wrap(err, errors.New("cannot fetch model settings"))
	}

	return config.New(config.NoDefaults, attrs)
}

func (d *factory) validateCharmSeries(seriesName string, imageStream string) error {
	// TODO(sidecar): handle systems

	// attempt to locate the charm series from the list of known juju series
	// that we currently support.
	workloadSeries, err := SupportedJujuSeries(d.clock.Now(), seriesName, imageStream)
	if err != nil {
		return errors.Trace(err)
	}

	if !workloadSeries.Contains(seriesName) && !d.force {
		return errors.NotSupportedf("series: %s", seriesName)
	}
	return nil
}

// validateCharmSeriesWithName calls the validateCharmSeries, but handles the
// error return value to check for NotSupported error and returns a custom error
// message if that's found.
func (d *factory) validateCharmSeriesWithName(series, name string, imageStream string) error {
	err := d.validateCharmSeries(series, imageStream)
	return charmValidationError(name, errors.Trace(err))
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

func (d *factory) validateResourcesNeededForLocalDeploy(charmMeta *charm.Meta) error {
	modelType, err := d.model.ModelType()
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
		"series", "base", "to", "resource", "attach-storage",
	}

	return charmOnlyFlags
}
