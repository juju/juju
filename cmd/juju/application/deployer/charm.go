// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/juju/charm/v8"
	jujuclock "github.com/juju/clock"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/macaroon.v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api/client/application"
	applicationapi "github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/resources"
	commoncharm "github.com/juju/juju/api/common/charm"
	app "github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/storage"
)

type deployCharm struct {
	applicationName  string
	attachStorage    []string
	bindings         map[string]string
	configOptions    DeployConfigFlag
	constraints      constraints.Value
	dryRun           bool
	modelConstraints constraints.Value
	csMac            *macaroon.Macaroon
	devices          map[string]devices.Constraints
	deployResources  DeployResourcesFunc
	force            bool
	id               application.CharmID
	flagSet          *gnuflag.FlagSet
	model            ModelCommand
	numUnits         int
	placement        []*instance.Placement
	placementSpec    string
	resources        map[string]string
	series           string
	steps            []DeployStep
	storage          map[string]storage.Constraints
	trust            bool

	validateCharmSeriesWithName           func(series, name string, imageStream string) error
	validateResourcesNeededForLocalDeploy func(charmMeta *charm.Meta) error
}

// deploy is the business logic of deploying a charm after
// it's been prepared.
func (d *deployCharm) deploy(
	ctx *cmd.Context,
	deployAPI DeployerAPI,
) (rErr error) {
	id := d.id
	charmInfo, err := deployAPI.CharmInfo(id.URL.String())
	if err != nil {
		return err
	}

	if len(d.attachStorage) > 0 && deployAPI.BestFacadeVersion("Application") < 5 {
		// DeployArgs.attachStorage is only supported from
		// Application API version 5 and onwards.
		return errors.New("this juju controller does not support --attach-storage")
	}

	// storage cannot be added to a container.
	if len(d.storage) > 0 || len(d.attachStorage) > 0 {
		for _, placement := range d.placement {
			if t, err := instance.ParseContainerType(placement.Scope); err == nil {
				return errors.NotSupportedf("adding storage to %s container", string(t))
			}
		}
	}

	numUnits := d.numUnits
	if charmInfo.Meta.Subordinate {
		if !constraints.IsEmpty(&d.constraints) {
			return errors.New("cannot use --constraints with subordinate application")
		}
		if numUnits == 1 && d.placementSpec == "" {
			numUnits = 0
		} else {
			return errors.New("cannot use --num-units or --to with subordinate application")
		}
	}
	applicationName := d.applicationName
	if applicationName == "" {
		applicationName = charmInfo.Meta.Name
	}

	// Process the --config args.
	// We may have a single file arg specified, in which case
	// it points to a YAML file keyed on the charm name and
	// containing values for any charm settings.
	// We may also have key/value pairs representing
	// charm settings which overrides anything in the YAML file.
	// If more than one file is specified, that is an error.
	var configYAML []byte
	files, err := d.configOptions.AbsoluteFileNames(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if len(files) > 1 {
		return errors.Errorf("only a single config YAML file can be specified, got %d", len(files))
	}
	if len(files) == 1 {
		configYAML, err = ioutil.ReadFile(files[0])
		if err != nil {
			return errors.Trace(err)
		}
	}
	attr, err := d.configOptions.ReadConfigPairs(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	appConfig := make(map[string]string)
	for k, v := range attr {
		appConfig[k] = v.(string)

		// Handle @ syntax for including file contents as values so we
		// are consistent to how 'juju config' works
		if len(appConfig[k]) < 1 || appConfig[k][0] != '@' {
			continue
		}

		if appConfig[k], err = utils.ReadValue(ctx, d.model.Filesystem(), appConfig[k][1:]); err != nil {
			return errors.Trace(err)
		}
	}

	// Expand the trust flag into the appConfig
	if d.trust {
		appConfig[app.TrustConfigOptionName] = strconv.FormatBool(d.trust)
	}

	// Application facade V5 expects charm config to either all be in YAML
	// or config map. If config map is specified, that overrides YAML.
	// So we need to combine the two here to have only one.
	if deployAPI.BestFacadeVersion("Application") < 6 && len(appConfig) > 0 {
		var configFromFile map[string]map[string]string
		err := yaml.Unmarshal(configYAML, &configFromFile)
		if err != nil {
			return errors.Annotate(err, "badly formatted YAML config file")
		}
		if configFromFile == nil {
			configFromFile = make(map[string]map[string]string)
		}
		charmSettings, ok := configFromFile[applicationName]
		if !ok {
			charmSettings = make(map[string]string)
		}
		for k, v := range appConfig {
			charmSettings[k] = v
		}
		appConfig = nil
		configFromFile[applicationName] = charmSettings
		configYAML, err = yaml.Marshal(configFromFile)
		if err != nil {
			return errors.Trace(err)
		}
	}

	bakeryClient, err := d.model.BakeryClient()
	if err != nil {
		return errors.Trace(err)
	}

	uuid, ok := deployAPI.ModelUUID()
	if !ok {
		return errors.New("API connection is controller-only (should never happen)")
	}

	deployInfo := DeploymentInfo{
		CharmID:         id,
		ApplicationName: applicationName,
		ModelUUID:       uuid,
		CharmInfo:       charmInfo,
		Force:           d.force,
	}

	for _, step := range d.steps {
		err = step.RunPre(deployAPI, bakeryClient, ctx, deployInfo)
		if err != nil {
			return errors.Trace(err)
		}
	}

	defer func() {
		for _, step := range d.steps {
			err = errors.Trace(step.RunPost(deployAPI, bakeryClient, ctx, deployInfo, rErr))
			if err != nil {
				rErr = err
			}
		}
	}()

	if id.URL != nil && id.URL.Schema != "local" && len(charmInfo.Meta.Terms) > 0 {
		ctx.Infof("Deployment under prior agreement to terms: %s",
			strings.Join(charmInfo.Meta.Terms, " "))
	}

	ids, err := d.deployResources(
		applicationName,
		resources.CharmID{
			URL:    id.URL,
			Origin: id.Origin,
		},
		d.csMac,
		d.resources,
		charmInfo.Meta.Resources,
		deployAPI,
		d.model.Filesystem(),
	)
	if err != nil {
		return errors.Trace(err)
	}

	if len(appConfig) == 0 {
		appConfig = nil
	}

	ctx.Infof("%s", d.formatDeployingText())
	args := applicationapi.DeployArgs{
		CharmID:          id,
		CharmOrigin:      id.Origin,
		Cons:             d.constraints,
		ApplicationName:  applicationName,
		Series:           d.series,
		NumUnits:         numUnits,
		ConfigYAML:       string(configYAML),
		Config:           appConfig,
		Placement:        d.placement,
		Storage:          d.storage,
		Devices:          d.devices,
		AttachStorage:    d.attachStorage,
		Resources:        ids,
		EndpointBindings: d.bindings,
		Force:            d.force,
	}

	err = deployAPI.Deploy(args)
	if err == nil {
		return nil
	}

	if errors.IsAlreadyExists(err) {
		// Would be nice to be able to access the app name here
		return errors.Wrapf(err, errors.New(`
deploy application using an alias name:
    juju deploy <application> <alias>
or use remove-application to remove the existing one and try again.`,
		), "%s", err.Error())
	}
	return errors.Trace(err)
}

var (
	// BundleOnlyFlags represents what flags are used for bundles only.
	BundleOnlyFlags = []string{
		"overlay", "map-machines",
	}
)

func (d *deployCharm) validateCharmFlags() error {
	if flags := utils.GetFlags(d.flagSet, BundleOnlyFlags); len(flags) > 0 {
		return errors.Errorf("options provided but not supported when deploying a charm: %s", strings.Join(flags, ", "))
	}
	return nil
}

func (d *deployCharm) formatDeployingText() string {
	curl := d.id.URL
	name := d.applicationName
	if name == "" {
		name = curl.Name
	}
	origin := d.id.Origin
	channel := origin.CharmChannel().String()
	if channel != "" {
		channel = fmt.Sprintf(" in channel %s", channel)
	}

	return fmt.Sprintf("Deploying %q from %s charm %q, revision %d%s on %s",
		name, origin.Source, curl.Name, curl.Revision, channel, origin.Series)
}

type predeployedLocalCharm struct {
	deployCharm
	userCharmURL *charm.URL
}

// String returns a string description of the deployer.
func (d *predeployedLocalCharm) String() string {
	str := fmt.Sprintf("deploy predeployed local charm: %s", d.userCharmURL.String())
	origin := d.id.Origin
	if isEmptyOrigin(origin, commoncharm.OriginLocal) {
		return str
	}
	var channel string
	if ch := origin.CharmChannel().String(); ch != "" {
		channel = fmt.Sprintf(" from channel %s", ch)
	}
	return fmt.Sprintf("%s%s", str, channel)
}

// PrepareAndDeploy finishes preparing to deploy a predeployed local charm,
// then deploys it.
func (d *predeployedLocalCharm) PrepareAndDeploy(ctx *cmd.Context, deployAPI DeployerAPI, _ Resolver, _ store.MacaroonGetter) error {
	userCharmURL := d.userCharmURL
	ctx.Verbosef("Preparing to deploy local charm %q again", userCharmURL.Name)
	if d.dryRun {
		ctx.Infof("ignoring dry-run flag for local charms")
	}

	modelCfg, err := getModelConfig(deployAPI)
	if err != nil {
		return errors.Trace(err)
	}

	// Avoid deploying charm if it's not valid for the model.
	if err := d.validateCharmSeriesWithName(userCharmURL.Series, userCharmURL.Name, modelCfg.ImageStream()); err != nil {
		return errors.Trace(err)
	}

	if err := d.validateCharmFlags(); err != nil {
		return errors.Trace(err)
	}

	charmInfo, err := deployAPI.CharmInfo(d.userCharmURL.String())
	if err != nil {
		return errors.Trace(err)
	}
	ctx.Infof("%s", formatLocatedText(d.userCharmURL, commoncharm.Origin{}))

	if err := d.validateResourcesNeededForLocalDeploy(charmInfo.Meta); err != nil {
		return errors.Trace(err)
	}

	platform, err := utils.DeducePlatform(d.constraints, d.userCharmURL.Series, d.modelConstraints)
	if err != nil {
		return errors.Trace(err)
	}
	origin, err := utils.DeduceOrigin(userCharmURL, charm.Channel{}, platform)
	if err != nil {
		return errors.Trace(err)
	}

	d.id = application.CharmID{
		URL:    d.userCharmURL,
		Origin: origin,
	}
	d.series = userCharmURL.Series
	return d.deploy(ctx, deployAPI)
}

type localCharm struct {
	deployCharm
	curl *charm.URL
	ch   charm.Charm
}

// String returns a string description of the deployer.
func (l *localCharm) String() string {
	return fmt.Sprintf("deploy local charm: %s", l.curl.String())
}

// PrepareAndDeploy finishes preparing to deploy a local charm,
// then deploys it.
func (l *localCharm) PrepareAndDeploy(ctx *cmd.Context, deployAPI DeployerAPI, _ Resolver, _ store.MacaroonGetter) error {
	ctx.Verbosef("Preparing to deploy local charm: %q ", l.curl.Name)
	if l.dryRun {
		ctx.Infof("ignoring dry-run flag for local charms")
	}
	if err := l.validateCharmFlags(); err != nil {
		return errors.Trace(err)
	}

	curl, err := deployAPI.AddLocalCharm(l.curl, l.ch, l.force)
	if err != nil {
		return errors.Trace(err)
	}

	platform, err := utils.DeducePlatform(l.constraints, l.curl.Series, l.modelConstraints)
	if err != nil {
		return errors.Trace(err)
	}
	origin, err := utils.DeduceOrigin(curl, charm.Channel{}, platform)
	if err != nil {
		return errors.Trace(err)
	}

	ctx.Infof("%s", formatLocatedText(curl, origin))
	l.id = application.CharmID{
		URL:    curl,
		Origin: origin,
		// Local charms don't need a channel.
	}
	l.series = l.curl.Series
	return l.deploy(ctx, deployAPI)
}

type repositoryCharm struct {
	deployCharm
	userRequestedURL *charm.URL
	clock            jujuclock.Clock
}

// String returns a string description of the deployer.
func (c *repositoryCharm) String() string {
	str := fmt.Sprintf("deploy charm: %s", c.userRequestedURL.String())
	origin := c.id.Origin
	if isEmptyOrigin(origin, commoncharm.OriginCharmStore) {
		return str
	}
	var revision string
	if origin.Revision != nil && *origin.Revision != -1 {
		revision = fmt.Sprintf(" with revision %d", *origin.Revision)
	}
	var channel string
	if ch := origin.CharmChannel().String(); ch != "" {
		if revision != "" {
			channel = fmt.Sprintf(" will refresh from channel %s", ch)
		} else {
			channel = fmt.Sprintf(" from channel %s", ch)
		}
	}
	return fmt.Sprintf("%s%s%s", str, revision, channel)
}

// PrepareAndDeploy finishes preparing to deploy a charm store charm,
// then deploys it.
func (c *repositoryCharm) PrepareAndDeploy(ctx *cmd.Context, deployAPI DeployerAPI, resolver Resolver, macaroonGetter store.MacaroonGetter) error {
	userRequestedURL := c.userRequestedURL
	location := "charmhub"
	if charm.CharmStore.Matches(userRequestedURL.Schema) {
		location = "charm-store"
	}
	ctx.Verbosef("Preparing to deploy %q from the %s", userRequestedURL.Name, location)

	modelCfg, workloadSeries, err := seriesSelectorRequirements(deployAPI, c.clock, userRequestedURL)
	if err != nil {
		return errors.Trace(err)
	}

	// Charm or bundle has been supplied as a URL so we resolve and
	// deploy using the store but pass in the origin command line
	// argument so users can target a specific origin.
	origin := c.id.Origin
	var usingDefaultSeries bool
	if defaultSeries, ok := modelCfg.DefaultSeries(); ok && origin.Series == "" {
		origin.Series = defaultSeries
		usingDefaultSeries = true
	}
	storeCharmOrBundleURL, origin, supportedSeries, err := resolver.ResolveCharm(userRequestedURL, origin, false) // no --switch possible.
	if charm.IsUnsupportedSeriesError(err) {
		msg := fmt.Sprintf("%v. Use --force to deploy the charm anyway.", err)
		if usingDefaultSeries {
			msg = msg + " Used the default-series."
		}
		return errors.New(msg)
	} else if err != nil {
		return errors.Trace(err)
	}
	if err := c.validateCharmFlags(); err != nil {
		return errors.Trace(err)
	}

	selector := seriesSelector{
		charmURLSeries:      userRequestedURL.Series,
		seriesFlag:          c.series,
		supportedSeries:     supportedSeries,
		supportedJujuSeries: workloadSeries,
		force:               c.force,
		conf:                modelCfg,
		fromBundle:          false,
	}

	// Get the series to use.
	series, err := selector.charmSeries()
	logger.Tracef("Using series %q from %v to deploy %v", series, supportedSeries, userRequestedURL)

	imageStream := modelCfg.ImageStream()
	// Avoid deploying charm if it's not valid for the model.
	// We check this first before possibly suggesting --force.
	if err == nil {
		if err2 := c.validateCharmSeriesWithName(series, storeCharmOrBundleURL.Name, imageStream); err2 != nil {
			return errors.Trace(err2)
		}
	}

	if charm.IsUnsupportedSeriesError(err) {
		msg := fmt.Sprintf("%v. Use --force to deploy the charm anyway.", err)
		if usingDefaultSeries {
			msg = msg + " Used the default-series."
		}
		return errors.New(msg)
	}
	if validationErr := charmValidationError(storeCharmOrBundleURL.Name, errors.Trace(err)); validationErr != nil {
		return errors.Trace(validationErr)
	}

	// Ensure we save the origin.
	origin = origin.WithSeries(series)

	// In-order for the url to represent the following updates to the the origin
	// and machine, we need to ensure that the series is actually correct as
	// well in the url.
	deployableURL := storeCharmOrBundleURL
	if charm.CharmHub.Matches(storeCharmOrBundleURL.Schema) {
		deployableURL = storeCharmOrBundleURL.WithSeries(origin.Series)
	}

	if c.dryRun {
		name := c.applicationName
		if name == "" {
			name = deployableURL.Name
		}
		channel := origin.CharmChannel().String()
		if channel != "" {
			channel = fmt.Sprintf(" in channel %s", channel)
		}

		ctx.Infof("%q from %s charm %q, revision %d%s on %s would be deployed",
			name, origin.Source, deployableURL.Name, deployableURL.Revision, channel, origin.Series)
		return nil
	}

	// Store the charm in the controller
	curl, csMac, csOrigin, err := store.AddCharmWithAuthorizationFromURL(deployAPI, macaroonGetter, deployableURL, origin, c.force)
	if err != nil {
		if termErr, ok := errors.Cause(err).(*common.TermsRequiredError); ok {
			return errors.Trace(termErr.UserErr())
		}
		return errors.Annotatef(err, "storing charm %q", deployableURL.Name)
	}
	ctx.Infof("%s", formatLocatedText(curl, csOrigin))

	// If the original series was empty, so we couldn't validate the original
	// charm series, but the charm url wasn't nil, we can check and validate
	// what that one says.
	//
	// Note: it's interesting that the charm url and the series can diverge and
	// tell different things when deploying a charm and in sake of understanding
	// what we deploy, we should converge the two so that both report identical
	// values.
	if curl != nil && series == "" {
		if err := c.validateCharmSeriesWithName(curl.Series, curl.Name, imageStream); err != nil {
			return errors.Trace(err)
		}
	}

	c.series = series
	c.csMac = csMac
	c.id = application.CharmID{
		URL:    curl,
		Origin: csOrigin,
	}
	return c.deploy(ctx, deployAPI)
}

func isEmptyOrigin(origin commoncharm.Origin, source commoncharm.OriginSource) bool {
	other := commoncharm.Origin{}
	if origin == other {
		return true
	}
	other.Source = source
	if origin == other {
		return true
	}
	return false
}

func formatLocatedText(curl *charm.URL, origin commoncharm.Origin) string {
	repository := origin.Source
	if repository == "" || repository == commoncharm.OriginLocal {
		return fmt.Sprintf("Located local charm %q, revision %d", curl.Name, curl.Revision)
	}
	var next string
	if curl.Revision != -1 {
		next = fmt.Sprintf(", revision %d", curl.Revision)
	} else if str := origin.CharmChannel().String(); str != "" {
		next = fmt.Sprintf(", channel %s", str)
	}
	return fmt.Sprintf("Located charm %q in %s%s", curl.Name, repository, next)
}
