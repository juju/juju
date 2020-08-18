// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/juju/charm/v7"
	"github.com/juju/charmrepo/v5/csclient/params"
	jujuclock "github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/macaroon.v2"
	"gopkg.in/yaml.v2"

	applicationapi "github.com/juju/juju/api/application"
	app "github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/charmstore"
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
	id              charmstore.CharmID
	flagSet         *gnuflag.FlagSet
	model           ModelCommand
	numUnits        int
	origin          applicationapi.CharmOrigin
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
		id,
		d.csMac,
		d.resources,
		charmInfo.Meta.Resources,
		deployAPI,
	)
	if err != nil {
		return errors.Trace(err)
	}

	if len(appConfig) == 0 {
		appConfig = nil
	}

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

type predeployedLocalCharm struct {
	deployCharm
	userCharmURL *charm.URL
}

// PrepareAndDeploy finishes preparing to deploy a predeployed local charm,
// then deploys it.
func (d *predeployedLocalCharm) PrepareAndDeploy(ctx *cmd.Context, deployAPI DeployerAPI, _ *store.CharmStoreAdaptor) error {
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
		return err
	}

	if err := d.validateResourcesNeededForLocalDeploy(charmInfo.Meta); err != nil {
		return errors.Trace(err)
	}
	formattedCharmURL := d.userCharmURL.String()
	ctx.Infof("Located charm %q.", formattedCharmURL)
	ctx.Infof("Deploying charm %q.", formattedCharmURL)

	d.origin, err = deduceOrigin(userCharmURL)
	if err != nil {
		return errors.Trace(err)
	}

	d.id = charmstore.CharmID{URL: d.userCharmURL}
	d.series = userCharmURL.Series

	return d.deploy(ctx, deployAPI)
}

type localCharm struct {
	deployCharm
	curl *charm.URL
	ch   charm.Charm
}

// PrepareAndDeploy finishes preparing to deploy a local charm,
// then deploys it.
func (l *localCharm) PrepareAndDeploy(ctx *cmd.Context, deployAPI DeployerAPI, _ *store.CharmStoreAdaptor) error {
	ctx.Verbosef("Preparing to deploy local charm: %q ", l.curl.Name)
	if err := l.validateCharmFlags(); err != nil {
		return errors.Trace(err)
	}

	curl, err := deployAPI.AddLocalCharm(l.curl, l.ch, l.force)
	if err != nil {
		return errors.Trace(err)
	}

	l.id = charmstore.CharmID{
		URL: curl,
		// Local charms don't need a channel.
	}
	l.series = l.curl.Series

	l.origin, err = deduceOrigin(curl)
	if err != nil {
		return errors.Trace(err)
	}

	ctx.Infof("Deploying charm %q.", curl.String())
	return l.deploy(ctx, deployAPI)
}

type charmStoreCharm struct {
	deployCharm
	userRequestedURL *charm.URL
	channel          params.Channel
	clock            jujuclock.Clock
}

// PrepareAndDeploy finishes preparing to deploy a charm store charm,
// then deploys it.
func (c *charmStoreCharm) PrepareAndDeploy(ctx *cmd.Context, deployAPI DeployerAPI, cstore *store.CharmStoreAdaptor) error {
	userRequestedURL := c.userRequestedURL
	ctx.Verbosef("Preparing to deploy %q from the charm store", userRequestedURL.Name)

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
	// deploy using the store but pass in the channel command line
	// argument so users can target a specific channel.
	storeCharmOrBundleURL, channel, supportedSeries, err := store.ResolveCharm(
		cstore.ResolveWithPreferredChannel, userRequestedURL, c.channel,
	)
	if charm.IsUnsupportedSeriesError(err) {
		return errors.Errorf("%v. Use --force to deploy the charm anyway.", err)
	} else if err != nil {
		return errors.Trace(err)
	}

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

	// Store the charm in the controller
	curl, csMac, err := store.AddCharmFromURL(deployAPI, cstore.MacaroonGetter, storeCharmOrBundleURL, channel, c.force)
	if err != nil {
		if termErr, ok := errors.Cause(err).(*common.TermsRequiredError); ok {
			return errors.Trace(termErr.UserErr())
		}
		return errors.Annotatef(err, "storing charm for URL %q", storeCharmOrBundleURL)
	}

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

	formattedCharmURL := curl.String()
	ctx.Infof("Located charm %q.", formattedCharmURL)
	ctx.Infof("Deploying charm %q.", formattedCharmURL)

	c.id = charmstore.CharmID{
		URL:     curl,
		Channel: channel,
	}
	c.origin, err = deduceOrigin(curl)
	if err != nil {
		return errors.Trace(err)
	}
	c.series = series
	c.csMac = csMac
	return c.deploy(ctx, deployAPI)
}
