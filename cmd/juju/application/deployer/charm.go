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
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/macaroon.v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api/application"
	applicationapi "github.com/juju/juju/api/application"
	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/api/resources/client"
	app "github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/resource/resourceadapters"
	"github.com/juju/juju/storage"
)

type deployCharm struct {
	applicationName string
	attachStorage   []string
	bindings        map[string]string
	configOptions   common.ConfigFlag
	constraints     constraints.Value
	csMac           *macaroon.Macaroon
	devices         map[string]devices.Constraints
	deployResources resourceadapters.DeployResourcesFunc
	force           bool
	id              application.CharmID
	flagSet         *gnuflag.FlagSet
	model           ModelCommand
	numUnits        int
	origin          commoncharm.Origin
	placement       []*instance.Placement
	placementSpec   string
	resources       map[string]string
	series          string
	steps           []DeployStep
	storage         map[string]storage.Constraints
	trust           bool

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
		client.CharmID{
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

	ctx.Infof(d.formatDeployingText())
	args := applicationapi.DeployArgs{
		CharmID:          id,
		CharmOrigin:      d.origin,
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
	}
	return errors.Trace(deployAPI.Deploy(args))
}

var (
	// BundleOnlyFlags represents what flags are used for bundles only.
	// TODO(thumper): support dry-run for apps as well as bundles.
	BundleOnlyFlags = []string{
		"overlay", "dry-run", "map-machines",
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
	channel := origin.CoreChannel().String()
	if channel != "" {
		channel = fmt.Sprintf(" in channel %s", channel)
	}

	return fmt.Sprintf("Deploying %q from %s charm %q, revision %d%s",
		name, origin.Source, curl.Name, curl.Revision, channel)
}

type predeployedLocalCharm struct {
	deployCharm
	userCharmURL *charm.URL
}

// String returns a string description of the deployer.
func (d *predeployedLocalCharm) String() string {
	str := fmt.Sprintf("deploy predeployed local charm: %s", d.userCharmURL.String())
	if isEmptyOrigin(d.origin, commoncharm.OriginLocal) {
		return str
	}
	var channel string
	if ch := d.origin.CoreChannel().String(); ch != "" {
		channel = fmt.Sprintf(" from channel %s", ch)
	}
	return fmt.Sprintf("%s%s", str, channel)
}

// PrepareAndDeploy finishes preparing to deploy a predeployed local charm,
// then deploys it.
func (d *predeployedLocalCharm) PrepareAndDeploy(ctx *cmd.Context, deployAPI DeployerAPI, _ Resolver, _ store.MacaroonGetter) error {
	userCharmURL := d.userCharmURL
	ctx.Verbosef("Preparing to deploy local charm %q again", userCharmURL.Name)

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
	ctx.Infof(formatLocatedText(d.userCharmURL, commoncharm.Origin{}))

	if err := d.validateResourcesNeededForLocalDeploy(charmInfo.Meta); err != nil {
		return errors.Trace(err)
	}

	// We know 100% that this will be a local charm, so don't attempt to
	// deduce the origin and just use the correct one to prevent any case that
	// the origin could be wrong.
	origin := commoncharm.Origin{
		Source: commoncharm.OriginLocal,
	}

	d.id = application.CharmID{
		URL:    d.userCharmURL,
		Origin: origin,
	}
	d.series = userCharmURL.Series
	d.origin = origin
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
	if err := l.validateCharmFlags(); err != nil {
		return errors.Trace(err)
	}

	curl, err := deployAPI.AddLocalCharm(l.curl, l.ch, l.force)
	if err != nil {
		return errors.Trace(err)
	}

	// We know 100% that this will be a local charm, so don't attempt to
	// deduce the origin and just use the correct one to prevent any case that
	// the origin could be wrong.
	origin := commoncharm.Origin{
		Source: commoncharm.OriginLocal,
	}

	ctx.Infof(formatLocatedText(curl, origin))
	l.id = application.CharmID{
		URL:    curl,
		Origin: origin,
		// Local charms don't need a channel.
	}
	l.series = l.curl.Series
	l.origin = origin
	return l.deploy(ctx, deployAPI)
}

type charmStoreCharm struct {
	deployCharm
	userRequestedURL *charm.URL
	clock            jujuclock.Clock
}

// String returns a string description of the deployer.
func (c *charmStoreCharm) String() string {
	str := fmt.Sprintf("deploy charm store charm: %s", c.userRequestedURL.String())
	if isEmptyOrigin(c.origin, commoncharm.OriginCharmStore) {
		return str
	}
	var channel string
	if ch := c.origin.CoreChannel().String(); ch != "" {
		channel = fmt.Sprintf(" from channel %s", ch)
	}
	return fmt.Sprintf("%s%s", str, channel)
}

// PrepareAndDeploy finishes preparing to deploy a charm store charm,
// then deploys it.
func (c *charmStoreCharm) PrepareAndDeploy(ctx *cmd.Context, deployAPI DeployerAPI, resolver Resolver, macaroonGetter store.MacaroonGetter) error {
	userRequestedURL := c.userRequestedURL
	location := "hub"
	if userRequestedURL.Schema == "cs" {
		location = "store"
	}
	ctx.Verbosef("Preparing to deploy %q from the charm-%s", userRequestedURL.Name, location)

	// resolver.resolve potentially updates the series of anything
	// passed in. Store this for use in seriesSelector.
	userRequestedSeries := userRequestedURL.Series

	modelCfg, err := getModelConfig(deployAPI)
	if err != nil {
		return errors.Trace(err)
	}

	imageStream := modelCfg.ImageStream()
	workloadSeries, err := supportedJujuSeries(c.clock.Now(), userRequestedSeries, imageStream)
	if err != nil {
		return errors.Trace(err)
	}

	// Charm or bundle has been supplied as a URL so we resolve and
	// deploy using the store but pass in the origin command line
	// argument so users can target a specific origin.
	storeCharmOrBundleURL, origin, supportedSeries, err := resolver.ResolveCharm(userRequestedURL, c.origin)
	if charm.IsUnsupportedSeriesError(err) {
		return errors.Errorf("%v. Use --force to deploy the charm anyway.", err)
	} else if err != nil {
		return errors.Trace(err)
	}
	c.origin = origin
	if err := c.validateCharmFlags(); err != nil {
		return errors.Trace(err)
	}

	selector := seriesSelector{
		charmURLSeries:      userRequestedSeries,
		seriesFlag:          c.series,
		supportedSeries:     supportedSeries,
		supportedJujuSeries: workloadSeries,
		force:               c.force,
		conf:                modelCfg,
		fromBundle:          false,
	}

	// Get the series to use.
	series, err := selector.charmSeries()

	// Avoid deploying charm if it's not valid for the model.
	// We check this first before possibly suggesting --force.
	if err == nil {
		if err2 := c.validateCharmSeriesWithName(series, storeCharmOrBundleURL.Name, imageStream); err2 != nil {
			return errors.Trace(err2)
		}
	}

	if charm.IsUnsupportedSeriesError(err) {
		return errors.Errorf("%v. Use --force to deploy the charm anyway.", err)
	}
	// although we try and get the charmSeries from the charm series
	// selector it will return an error and an empty string for the series.
	// So we need to approximate what the seriesName should be when
	// displaying an error to the user. We do this by getting the potential
	// series name.
	seriesName := getPotentialSeriesName(series, storeCharmOrBundleURL.Series, userRequestedSeries)
	if validationErr := charmValidationError(seriesName, storeCharmOrBundleURL.Name, errors.Trace(err)); validationErr != nil {
		return errors.Trace(validationErr)
	}

	origin.Series = seriesName
	c.origin = origin

	// Store the charm in the controller
	curl, csMac, csOrigin, err := store.AddCharmWithAuthorizationFromURL(deployAPI, macaroonGetter, storeCharmOrBundleURL, c.origin, c.force)
	if err != nil {
		if termErr, ok := errors.Cause(err).(*common.TermsRequiredError); ok {
			return errors.Trace(termErr.UserErr())
		}
		return errors.Annotatef(err, "storing charm for URL %q", storeCharmOrBundleURL)
	}
	ctx.Infof(formatLocatedText(curl, csOrigin))

	// If the original series was empty, so we couldn't validate the original
	// charm series, but the charm url wasn't nil, we can check and validate
	// what that one says.
	//
	// Note: it's interesting that the charm url and the series can diverge and
	// tell different things when deploying a charm and in sake of understanding
	// what we deploy, we should converge the two so that both report identical
	// values.
	if curl != nil && series == "" {
		if err := c.validateCharmSeriesWithName(curl.Series, storeCharmOrBundleURL.Name, imageStream); err != nil {
			return errors.Trace(err)
		}
	}

	c.series = series
	c.csMac = csMac
	c.origin = csOrigin
	c.id = application.CharmID{
		URL:    curl,
		Origin: c.origin,
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
	return fmt.Sprintf("Located charm %q in %s, revision %d", curl.Name, repository, curl.Revision)
}
