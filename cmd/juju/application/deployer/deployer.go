// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"archive/zip"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juju/charm/v8"
	"github.com/juju/charm/v8/resource"
	"github.com/juju/charmrepo/v6"
	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"

	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/juju/common"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/resource/resourceadapters"
	"github.com/juju/juju/storage"
)

var logger = loggo.GetLogger("juju.cmd.juju.application.deployer")

// NewDeployerFactory returns a factory setup with the API and
// function dependencies required by every deployer.
func NewDeployerFactory(dep DeployerDependencies) DeployerFactory {
	d := &factory{
		clock:                jujuclock.WallClock,
		model:                dep.Model,
		newConsumeDetailsAPI: dep.NewConsumeDetailsAPI,
		steps:                dep.Steps,
	}
	if dep.DeployResources == nil {
		d.deployResources = resourceadapters.DeployResources
	}
	return d
}

// GetDeployer returns the correct deployer to use based on the cfg provided.
// A ModelConfigGetter and CharmStoreAdaptor needed to find the deployer.
func (d *factory) GetDeployer(cfg DeployerConfig, getter ModelConfigGetter, resolver Resolver) (Deployer, error) {
	d.setConfig(cfg)
	maybeDeployers := []func() (Deployer, error){
		d.maybeReadLocalBundle,
		func() (Deployer, error) { return d.maybeReadLocalCharm(getter) },
		d.maybePredeployedLocalCharm,
		func() (Deployer, error) { return d.maybeReadCharmstoreBundle(resolver) },
		d.charmStoreCharm, // This always returns a Deployer
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
	d.bundleOverlayFile = cfg.BundleOverlayFile
	d.channel = cfg.Channel
	d.series = cfg.Series
	d.force = cfg.Force
	d.dryRun = cfg.DryRun
	d.applicationName = cfg.ApplicationName
	d.configOptions = cfg.ConfigOptions
	d.constraints = cfg.Constraints
	d.storage = cfg.Storage
	d.bundleStorage = cfg.BundleStorage
	d.devices = cfg.Devices
	d.bundleDevices = cfg.BundleDevices
	d.resources = cfg.Resources
	d.bindings = cfg.Bindings
	d.useExisting = cfg.UseExisting
	d.bundleMachines = cfg.BundleMachines
	d.trust = cfg.Trust
	d.flagSet = cfg.FlagSet
}

// DeployerDependencies are required for any deployer to be run.
type DeployerDependencies struct {
	DeployResources      resourceadapters.DeployResourcesFunc
	Model                ModelCommand
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
	Channel              corecharm.Channel
	CharmOrBundle        string
	ConfigOptions        common.ConfigFlag
	ConstraintsStr       string
	Constraints          constraints.Value
	Devices              map[string]devices.Constraints
	DeployResources      resourceadapters.DeployResourcesFunc
	DryRun               bool
	FlagSet              *gnuflag.FlagSet
	Force                bool
	NewConsumeDetailsAPI func(url *charm.OfferURL) (ConsumeDetails, error)
	NumUnits             int
	PlacementSpec        string
	Placement            []*instance.Placement
	Resources            map[string]string
	Series               string
	Storage              map[string]storage.Constraints
	Trust                bool
	UseExisting          bool
}

type factory struct {
	// DeployerDependencies
	model                ModelCommand
	deployResources      resourceadapters.DeployResourcesFunc
	newConsumeDetailsAPI func(url *charm.OfferURL) (ConsumeDetails, error)

	// DeployerConfig
	placementSpec     string
	placement         []*instance.Placement
	numUnits          int
	attachStorage     []string
	charmOrBundle     string
	bundleOverlayFile []string
	channel           corecharm.Channel
	series            string
	force             bool
	dryRun            bool
	applicationName   string
	configOptions     common.ConfigFlag
	constraints       constraints.Value
	storage           map[string]storage.Constraints
	bundleStorage     map[string]map[string]storage.Constraints
	devices           map[string]devices.Constraints
	bundleDevices     map[string]map[string]devices.Constraints
	resources         map[string]string
	bindings          map[string]string
	steps             []DeployStep
	useExisting       bool
	bundleMachines    map[string]string
	trust             bool
	flagSet           *gnuflag.FlagSet

	// Private
	clock jujuclock.Clock
}

func (d *factory) maybePredeployedLocalCharm() (Deployer, error) {
	// If the charm's schema is local, we should definitively attempt
	// to deploy a charm that's already deployed in the
	// environment.
	userCharmURL, err := charm.ParseURL(d.charmOrBundle)
	if err != nil {
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
		applicationName: d.applicationName,
		attachStorage:   d.attachStorage,
		bindings:        d.bindings,
		configOptions:   d.configOptions,
		constraints:     d.constraints,
		devices:         d.devices,
		deployResources: d.deployResources,
		flagSet:         d.flagSet,
		force:           d.force,
		model:           d.model,
		numUnits:        d.numUnits,
		placement:       d.placement,
		placementSpec:   d.placementSpec,
		resources:       d.resources,
		steps:           d.steps,
		storage:         d.storage,
		trust:           d.trust,

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
			"To deploy a charm or bundle from the store, run `juju deploy cs:%[1]s`.",
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
	return &localBundle{deployBundle: d.newDeployBundle(ds)}, nil
}

// newDeployBundle returns the config needed to eventually call
// deployBundle.deploy.  This is used by all types of bundles to
// be deployed
func (d *factory) newDeployBundle(ds charm.BundleDataSource) deployBundle {
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
	}
}

func (d *factory) maybeReadLocalCharm(getter ModelConfigGetter) (Deployer, error) {
	// NOTE: Here we select the series using the algorithm defined by
	// `seriesSelector.CharmSeries`. This serves to override the algorithm found in
	// `charmrepo.NewCharmAtPath` which is outdated (but must still be
	// called since the code is coupled with path interpretation logic which
	// cannot easily be factored out).

	// NOTE: Reading the charm here is only meant to aid in inferring the correct
	// series, if this fails we fall back to the argument series. If reading
	// the charm fails here it will also fail below (the charm is read again
	// below) where it is handled properly. This is just an expedient to get
	// the correct series. A proper refactoring of the charmrepo package is
	// needed for a more elegant fix.
	seriesName := d.series
	ch, err := charm.ReadCharm(d.charmOrBundle)

	var imageStream string
	if err == nil {
		modelCfg, err := getModelConfig(getter)
		if err != nil {
			return nil, errors.Trace(err)
		}

		imageStream = modelCfg.ImageStream()
		workloadSeries, err := supportedJujuSeries(d.clock.Now(), d.series, imageStream)
		if err != nil {
			return nil, errors.Trace(err)
		}

		supportedSeries := ch.Meta().ComputedSeries()
		seriesSelector := seriesSelector{
			seriesFlag:          seriesName,
			supportedSeries:     supportedSeries,
			supportedJujuSeries: workloadSeries,
			force:               d.force,
			conf:                modelCfg,
			fromBundle:          false,
		}

		if len(supportedSeries) == 0 {
			logger.Warningf("%s does not declare supported series in metadata.yml", ch.Meta().Name)
		}

		seriesName, err = seriesSelector.charmSeries()
		if err = charmValidationError(seriesName, ch.Meta().Name, errors.Trace(err)); err != nil {
			return nil, errors.Trace(err)
		}
	}

	// Charm may have been supplied via a path reference.
	ch, curl, err := charmrepo.NewCharmAtPathForceSeries(d.charmOrBundle, seriesName, d.force)
	// We check for several types of known error which indicate
	// that the supplied reference was indeed a path but there was
	// an issue reading the charm located there.
	if charm.IsMissingSeriesError(err) {
		return nil, err
	} else if charm.IsUnsupportedSeriesError(err) {
		return nil, errors.Trace(err)
	} else if errors.Cause(err) == zip.ErrFormat {
		return nil, errors.Errorf("invalid charm or bundle provided at %q", d.charmOrBundle)
	} else if _, ok := err.(*charmrepo.NotFoundError); ok {
		return nil, errors.Wrap(err, errors.NotFoundf("charm or bundle at %q", d.charmOrBundle))
	} else if err != nil && err != os.ErrNotExist {
		// If we get a "not exists" error then we attempt to interpret
		// the supplied charm reference as a URL elsewhere, otherwise
		// we return the error.
		return nil, errors.Trace(err)
	} else if err != nil {
		logger.Debugf("cannot interpret as local charm: %v", err)
		return nil, nil
	}

	// Avoid deploying charm if it's not valid for the model.
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

func (d *factory) maybeReadCharmstoreBundle(resolver Resolver) (Deployer, error) {
	curl, err := resolveCharmURL(d.charmOrBundle)
	if err != nil {
		return nil, errors.Trace(err)
	}
	origin, err := utils.DeduceOrigin(curl, d.channel)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// validate this is a charmstore bundle
	bundleURL, origin, err := resolver.ResolveBundleURL(curl, origin)
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
	dir, err := ioutil.TempDir("", bundleURL.Name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	bundle, err := resolver.GetBundle(bundleURL, filepath.Join(dir, bundleURL.Name))
	if err != nil {
		return nil, errors.Trace(err)
	}

	db := d.newDeployBundle(store.NewResolvedBundle(bundle))
	db.bundleURL = bundleURL
	db.origin = origin
	db.bundleOverlayFile = d.bundleOverlayFile
	return &charmstoreBundle{deployBundle: db}, nil
}

func (d *factory) charmStoreCharm() (Deployer, error) {
	// Validate we have a charm store change
	userRequestedURL, err := resolveCharmURL(d.charmOrBundle)
	if err != nil {
		return nil, errors.Trace(err)
	}
	origin, err := utils.DeduceOrigin(userRequestedURL, d.channel)
	if err != nil {
		return nil, errors.Trace(err)
	}

	deployCharm := d.newDeployCharm()
	deployCharm.origin = origin
	return &charmStoreCharm{
		deployCharm:      deployCharm,
		userRequestedURL: userRequestedURL,
		clock:            d.clock,
	}, nil
}

func resolveCharmURL(path string) (*charm.URL, error) {
	// If the charmhub integration is in effect, then we ensure that schema is
	// defined and we can parse the URL correctly.
	if featureflag.Enabled(feature.CharmHubIntegration) {
		var err error
		path, err = charm.EnsureSchema(path)
		if err != nil {
			return nil, errors.Trace(err)
		}

		return charm.ParseURL(path)
	}

	u, err := url.Parse(path)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// We don't expect the charmhub url scheme to show up here, as the feature
	// flag isn't enabled. Return
	if charm.CharmHub.Matches(u.Scheme) {
		// Replicate the charm url parsing error here to keep things consistent.
		return nil, errors.Errorf(`unexpected charm schema: cannot parse URL %q: schema "ch" not valid`, path)
	}

	// If we find a scheme that is empty, force it to become a charmstore scheme
	// so every other subsequent parse url call knows the correct type.
	if u.Scheme == "" {
		return charm.ParseURL(fmt.Sprintf("cs:%s", u.Path))
	}

	return charm.ParseURL(path)
}

// Returns the first string that isn't empty.
// If all strings are empty, then return an empty string.
func getPotentialSeriesName(series ...string) string {
	for _, s := range series {
		if s != "" {
			return s
		}
	}
	return ""
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

var getModelConfig = func(api ModelConfigGetter) (*config.Config, error) {
	// Separated into a variable for easy overrides
	attrs, err := api.ModelGet()
	if err != nil {
		return nil, errors.Wrap(err, errors.New("cannot fetch model settings"))
	}

	return config.New(config.NoDefaults, attrs)
}

func (d *factory) validateCharmSeries(seriesName string, imageStream string) error {
	// TODO(new-charms): handle systems

	// attempt to locate the charm series from the list of known juju series
	// that we currently support.
	workloadSeries, err := supportedJujuSeries(d.clock.Now(), seriesName, imageStream)
	if err != nil {
		return errors.Trace(err)
	}

	var found bool
	for _, name := range workloadSeries.Values() {
		if name == seriesName {
			found = true
			break
		}
	}
	if !found && !d.force {
		return errors.NotSupportedf("series: %s", seriesName)
	}

	modelType, err := d.model.ModelType()
	if err != nil {
		return errors.Trace(err)
	}

	// TODO(new-charms): handle charm v2
	return model.ValidateSeries(modelType, seriesName, true)
}

// validateCharmSeriesWithName calls the validateCharmSeries, but handles the
// error return value to check for NotSupported error and returns a custom error
// message if that's found.
func (d *factory) validateCharmSeriesWithName(series, name string, imageStream string) error {
	err := d.validateCharmSeries(series, imageStream)
	return charmValidationError(series, name, errors.Trace(err))
}

// charmValidationError consumes an error along with a charmSeries and name
// to help provide better feedback to the user when somethings gone wrong around
// validating a charm validation
func charmValidationError(charmSeries, name string, err error) error {
	if err != nil {
		if errors.IsNotSupported(err) {
			return errors.Errorf("%v is not available on the following %v", name, err)
		}
		return errors.Trace(err)
	}
	return nil
}

func (d *factory) validateResourcesNeededForLocalDeploy(charmMeta *charm.Meta) error {
	modelType, err := d.model.ModelType()
	if err != nil {
		return errors.Trace(err)
	}
	if modelType != model.CAAS {
		return nil
	}
	var missingImages []string
	for resName, resMeta := range charmMeta.Resources {
		if resMeta.Type == resource.TypeContainerImage {
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
		"series", "to", "resource", "attach-storage",
	}

	return charmOnlyFlags
}
