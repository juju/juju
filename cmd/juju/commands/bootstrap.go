// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/featureflag"
	"gopkg.in/juju/charm.v6-unstable"
	"launchpad.net/gnuflag"

	apiblock "github.com/juju/juju/api/block"
	"github.com/juju/juju/apiserver"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/network"
	"github.com/juju/juju/version"
)

// provisionalProviders is the names of providers that are hidden behind
// feature flags.
var provisionalProviders = map[string]string{
	"vsphere": feature.VSphereProvider,
}

const bootstrapDoc = `
bootstrap starts a new controller in the specified cloud (it will return
an error if a controller with the same name has already been bootstrapped).
Bootstrapping a controller will provision a new machine and run the controller on
that machine.

The controller will be setup with an intial controller model called "admin" as well
as a hosted model which can be used to run workloads.

If boostrap-constraints are specified in the bootstrap command, 
they will apply to the machine provisioned for the juju controller, 
and any future controllers provisioned for HA.

If constraints are specified, they will be set as the default constraints 
on the model for all future workload machines, 
exactly as if the constraints were set with juju set-constraints.

It is possible to override constraints and the automatic machine selection
algorithm by using the "--to" flag. The value associated with "--to" is a
"placement directive", which tells Juju how to identify the first machine to use.
For more information on placement directives, see "juju help placement".

Bootstrap initialises the cloud environment synchronously and displays information
about the current installation steps.  The time for bootstrap to complete varies
across cloud providers from a few seconds to several minutes.  Once bootstrap has
completed, you can run other juju commands against your model. You can change
the default timeout and retry delays used during the bootstrap by changing the
following settings in your environments.yaml (all values represent number of seconds):

    # How long to wait for a connection to the controller
    bootstrap-timeout: 600 # default: 10 minutes
    # How long to wait between connection attempts to a controller address.
    bootstrap-retry-delay: 5 # default: 5 seconds
    # How often to refresh controller addresses from the API server.
    bootstrap-addresses-delay: 10 # default: 10 seconds

Private clouds may need to specify their own custom image metadata, and
possibly upload Juju tools to cloud storage if no outgoing Internet access is
available. In this case, use the --metadata-source parameter to point
bootstrap to a local directory from which to upload tools and/or image
metadata.

If agent-version is specifed, this is the default tools version to use when running the Juju agents.
Only the numeric version is relevant. To enable ease of scripting, the full binary version
is accepted (eg 1.24.4-trusty-amd64) but only the numeric version (eg 1.24.4) is used.
By default, Juju will bootstrap using the exact same version as the client.

See Also:
   juju help glossary
   juju list-controllers
   juju list-models
   juju help switch
   juju help constraints
   juju help set-constraints
   juju help placement
`

func newBootstrapCommand() cmd.Command {
	return modelcmd.Wrap(&bootstrapCommand{
		CredentialStore: jujuclient.NewFileCredentialStore(),
	})
}

// bootstrapCommand is responsible for launching the first machine in a juju
// environment, and setting up everything necessary to continue working.
type bootstrapCommand struct {
	modelcmd.ModelCommandBase
	CredentialStore jujuclient.CredentialStore

	Constraints           constraints.Value
	BootstrapConstraints  constraints.Value
	BootstrapSeries       string
	BootstrapImage        string
	UploadTools           bool
	MetadataSource        string
	Placement             string
	KeepBrokenEnvironment bool
	AutoUpgrade           bool
	AgentVersionParam     string
	AgentVersion          *version.Number
	config                configFlag

	controllerName string
	CredentialName string
	Cloud          string
	Region         string
}

func (c *bootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "bootstrap",
		Purpose: "start up an environment from scratch",
		Args:    "<controllername> <cloud>[/<region>]",
		Doc:     bootstrapDoc,
	}
}

func (c *bootstrapCommand) SetFlags(f *gnuflag.FlagSet) {
	f.Var(constraints.ConstraintsValue{Target: &c.Constraints}, "constraints", "set model constraints")
	f.Var(constraints.ConstraintsValue{Target: &c.BootstrapConstraints}, "bootstrap-constraints", "specify bootstrap machine constraints")
	f.StringVar(&c.BootstrapSeries, "bootstrap-series", "", "specify the series of the bootstrap machine")
	if featureflag.Enabled(feature.ImageMetadata) {
		f.StringVar(&c.BootstrapImage, "bootstrap-image", "", "specify the image of the bootstrap machine")
	}
	f.BoolVar(&c.UploadTools, "upload-tools", false, "upload local version of tools before bootstrapping")
	f.StringVar(&c.MetadataSource, "metadata-source", "", "local path to use as tools and/or metadata source")
	f.StringVar(&c.Placement, "to", "", "a placement directive indicating an instance to bootstrap")
	f.BoolVar(&c.KeepBrokenEnvironment, "keep-broken", false, "do not destroy the model if bootstrap fails")
	f.BoolVar(&c.AutoUpgrade, "auto-upgrade", false, "upgrade to the latest patch release tools on first bootstrap")
	f.StringVar(&c.AgentVersionParam, "agent-version", "", "the version of tools to use for Juju agents")
	f.StringVar(&c.CredentialName, "credential", "", "the credentials to use when bootstrapping")
	f.Var(&c.config, "config", "specify a controller config file, or one or more controller configuration options (--config config.yaml [--config k=v ...])")
}

func (c *bootstrapCommand) Init(args []string) (err error) {
	if c.AgentVersionParam != "" && c.UploadTools {
		return fmt.Errorf("--agent-version and --upload-tools can't be used together")
	}
	if c.BootstrapSeries != "" && !charm.IsValidSeries(c.BootstrapSeries) {
		return errors.NotValidf("series %q", c.BootstrapSeries)
	}
	if c.BootstrapImage != "" {
		if c.BootstrapSeries == "" {
			return errors.Errorf("--bootstrap-image must be used with --bootstrap-series")
		}
		cons, err := constraints.Merge(c.Constraints, c.BootstrapConstraints)
		if err != nil {
			return errors.Trace(err)
		}
		if !cons.HasArch() {
			return errors.Errorf("--bootstrap-image must be used with --bootstrap-constraints, specifying architecture")
		}
	}

	// Parse the placement directive. Bootstrap currently only
	// supports provider-specific placement directives.
	if c.Placement != "" {
		_, err = instance.ParsePlacement(c.Placement)
		if err != instance.ErrPlacementScopeMissing {
			// We only support unscoped placement directives for bootstrap.
			return fmt.Errorf("unsupported bootstrap placement directive %q", c.Placement)
		}
	}
	if !c.AutoUpgrade {
		// With no auto upgrade chosen, we default to the version matching the bootstrap client.
		vers := version.Current
		c.AgentVersion = &vers
	}
	if c.AgentVersionParam != "" {
		if vers, err := version.ParseBinary(c.AgentVersionParam); err == nil {
			c.AgentVersion = &vers.Number
		} else if vers, err := version.Parse(c.AgentVersionParam); err == nil {
			c.AgentVersion = &vers
		} else {
			return err
		}
	}
	if c.AgentVersion != nil && (c.AgentVersion.Major != version.Current.Major || c.AgentVersion.Minor != version.Current.Minor) {
		return fmt.Errorf("requested agent version major.minor mismatch")
	}

	// The user must specify two positional arguments: the controller name,
	// and the cloud name (optionally with region specified).
	if len(args) < 2 {
		return errors.New("controller name and cloud name are required")
	}
	c.controllerName = bootstrappedControllerName(args[0])
	c.Cloud = args[1]
	if i := strings.IndexRune(c.Cloud, '/'); i > 0 {
		c.Cloud, c.Region = c.Cloud[:i], c.Cloud[i+1:]
	}
	return cmd.CheckEmpty(args[2:])
}

var bootstrappedControllerName = func(controllerName string) string {
	return fmt.Sprintf("local.%s", controllerName)
}

// BootstrapInterface provides bootstrap functionality that Run calls to support cleaner testing.
type BootstrapInterface interface {
	Bootstrap(ctx environs.BootstrapContext, environ environs.Environ, args bootstrap.BootstrapParams) error
}

type bootstrapFuncs struct{}

func (b bootstrapFuncs) Bootstrap(ctx environs.BootstrapContext, env environs.Environ, args bootstrap.BootstrapParams) error {
	return bootstrap.Bootstrap(ctx, env, args)
}

var getBootstrapFuncs = func() BootstrapInterface {
	return &bootstrapFuncs{}
}

var (
	environsPrepare = environs.Prepare
	environsDestroy = environs.Destroy
)

var ambiguousCredentialError = errors.New(`
more than one credential detected
run juju autoload-credentials and specify a credential using the --credential argument`[1:],
)

// Run connects to the environment specified on the command line and bootstraps
// a juju in that environment if none already exists. If there is as yet no environments.yaml file,
// the user is informed how to create one.
func (c *bootstrapCommand) Run(ctx *cmd.Context) (resultErr error) {
	bootstrapFuncs := getBootstrapFuncs()

	// Get the cloud definition identified by c.Cloud. If c.Cloud does not
	// identify a cloud in clouds.yaml, but is the name of a provider, and
	// that provider implements environs.CloudRegionDetector, we'll
	// synthesise a Cloud structure with the detected regions and no auth-
	// types.
	cloud, err := jujucloud.CloudByName(c.Cloud)
	if errors.IsNotFound(err) {
		ctx.Verbosef("cloud %q not found, trying as a provider name", c.Cloud)
		provider, err := environs.Provider(c.Cloud)
		if errors.IsNotFound(err) {
			return errors.NotFoundf("cloud %q", c.Cloud)
		} else if err != nil {
			return errors.Trace(err)
		}
		detector, ok := provider.(environs.CloudRegionDetector)
		if !ok {
			ctx.Verbosef(
				"provider %q does not support detecting regions",
				c.Cloud,
			)
			return errors.NotFoundf("cloud %q", c.Cloud)
		}
		regions, err := detector.DetectRegions()
		if err != nil && !errors.IsNotFound(err) {
			// It's not an error to have no regions.
			return errors.Annotatef(err,
				"detecting regions for %q cloud provider",
				c.Cloud,
			)
		}
		cloud = &jujucloud.Cloud{
			Type:    c.Cloud,
			Regions: regions,
		}
	} else if err != nil {
		return errors.Trace(err)
	}
	if err := checkProviderType(cloud.Type); errors.IsNotFound(err) {
		// This error will get handled later.
	} else if err != nil {
		return errors.Trace(err)
	}

	// Get the credentials and region name.
	credential, regionName, err := c.getCredentials(ctx, c.Cloud, cloud)
	if errors.IsNotFound(err) && c.CredentialName == "" {
		// No credential was explicitly specified, and no credential
		// was found in credentials.yaml; have the provider detect
		// credentials from the environment.
		ctx.Verbosef("no credentials found, checking environment")
		provider, err := environs.Provider(cloud.Type)
		if err != nil {
			return errors.Trace(err)
		}
		detected, err := provider.DetectCredentials()
		if err != nil {
			return errors.Annotatef(err, "detecting credentials for %q cloud provider", c.Cloud)
		}
		logger.Tracef("provider detected credentials: %v", detected)
		if len(detected.AuthCredentials) == 0 {
			return errors.NotFoundf("credentials for cloud %q", c.Cloud)
		}
		if len(detected.AuthCredentials) > 1 {
			return ambiguousCredentialError
		}
		// We have one credential so extract it from the map.
		var oneCredential jujucloud.Credential
		for _, oneCredential = range detected.AuthCredentials {
		}
		credential = &oneCredential
		regionName = c.Region
		if regionName == "" {
			regionName = detected.DefaultRegion
		}
		logger.Tracef("authenticating with region %q and %v", regionName, credential)
	} else if err != nil {
		return errors.Trace(err)
	}

	region, err := getRegion(cloud, c.Cloud, regionName)
	if err != nil {
		return errors.Trace(err)
	}

	// Create an environment config from the cloud and credentials. The
	// controller's model should be called "admin".
	configAttrs := map[string]interface{}{
		"type": cloud.Type,
		// TODO(axw) for now we call the initial model the same as the
		// controller, without the "local." prefix. This is necessary
		// to make CI happy. Once CI is updated, we'll switch over to
		// "admin".
		"name": configstore.AdminModelName(c.controllerName),
	}
	userConfigAttrs, err := c.config.ReadAttrs(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	for k, v := range userConfigAttrs {
		configAttrs[k] = v
	}
	logger.Debugf("preparing controller with config: %v", configAttrs)
	cfg, err := config.New(config.UseDefaults, configAttrs)
	if err != nil {
		return errors.Annotate(err, "creating environment configuration")
	}
	store, err := configstore.Default()
	if err != nil {
		return errors.Trace(err)
	}
	controllerStore := c.ClientStore()
	environ, err := environsPrepare(
		modelcmd.BootstrapContext(ctx), store, controllerStore, c.controllerName,
		environs.PrepareForBootstrapParams{
			Config:               cfg,
			Credentials:          *credential,
			CloudRegion:          region.Name,
			CloudEndpoint:        region.Endpoint,
			CloudStorageEndpoint: region.StorageEndpoint,
		},
	)
	if err != nil {
		return errors.Trace(err)
	}

	// Set the current controller so "juju status" can be run while
	// bootstrapping is underway.
	if err := modelcmd.WriteCurrentController(c.controllerName); err != nil {
		return errors.Trace(err)
	}

	cloudRegion := c.Cloud
	if region.Name != "" {
		cloudRegion = fmt.Sprintf("%s/%s", cloudRegion, region.Name)
	}
	ctx.Infof(
		"Creating Juju controller %q on %s",
		c.controllerName, cloudRegion,
	)

	// If we error out for any reason, clean up the environment.
	defer func() {
		if resultErr != nil {
			if c.KeepBrokenEnvironment {
				logger.Warningf(`
bootstrap failed but --keep-broken was specified so model is not being destroyed.
When you are finished diagnosing the problem, remember to run juju destroy-model --force
to clean up the model.`[1:])
			} else {
				handleBootstrapError(ctx, resultErr, func() error {
					return environsDestroy(
						c.controllerName, environ, store, controllerStore,
					)
				})
			}
		}
	}()

	// Block interruption during bootstrap. Providers may also
	// register for interrupt notification so they can exit early.
	interrupted := make(chan os.Signal, 1)
	defer close(interrupted)
	ctx.InterruptNotify(interrupted)
	defer ctx.StopInterruptNotify(interrupted)
	go func() {
		for _ = range interrupted {
			ctx.Infof("Interrupt signalled: waiting for bootstrap to exit")
		}
	}()

	// If --metadata-source is specified, override the default tools metadata source so
	// SyncTools can use it, and also upload any image metadata.
	var metadataDir string
	if c.MetadataSource != "" {
		metadataDir = ctx.AbsPath(c.MetadataSource)
	}

	// Merge environ and bootstrap-specific constraints.
	constraintsValidator, err := environ.ConstraintsValidator()
	if err != nil {
		return errors.Trace(err)
	}
	bootstrapConstraints, err := constraintsValidator.Merge(
		c.Constraints, c.BootstrapConstraints,
	)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Infof("combined bootstrap constraints: %v", bootstrapConstraints)

	err = bootstrapFuncs.Bootstrap(modelcmd.BootstrapContext(ctx), environ, bootstrap.BootstrapParams{
		EnvironConstraints:   c.Constraints,
		BootstrapConstraints: bootstrapConstraints,
		BootstrapSeries:      c.BootstrapSeries,
		BootstrapImage:       c.BootstrapImage,
		Placement:            c.Placement,
		UploadTools:          c.UploadTools,
		AgentVersion:         c.AgentVersion,
		MetadataDir:          metadataDir,
	})
	if err != nil {
		return errors.Annotate(err, "failed to bootstrap model")
	}

	if err := c.SetModelName(cfg.Name()); err != nil {
		return errors.Trace(err)
	}

	err = c.setBootstrapEndpointAddress(store, environ)
	if err != nil {
		return errors.Annotate(err, "saving bootstrap endpoint address")
	}

	// To avoid race conditions when running scripted bootstraps, wait
	// for the controller's machine agent to be ready to accept commands
	// before exiting this bootstrap command.
	return c.waitForAgentInitialisation(ctx)
}

// credentialByName returns the credential and default region to use for the
// specified cloud, optionally specifying a credential name. If no credential
// name is specified, then use the default credential for the cloud if one has
// been specified. The credential name is returned also, in case the default
// credential is used. If there is only one credential, it is implicitly the
// default.
//
// If there exists no matching credentials, an error satisfying
// errors.IsNotFound will be returned.
func credentialByName(
	store jujuclient.CredentialGetter, cloudName, credentialName string,
) (_ *jujucloud.Credential, credentialNameUsed string, defaultRegion string, _ error) {

	cloudCredentials, err := store.CredentialForCloud(cloudName)
	if err != nil {
		return nil, "", "", errors.Annotate(err, "loading credentials")
	}
	if credentialName == "" {
		// No credential specified, so use the default for the cloud.
		credentialName = cloudCredentials.DefaultCredential
		if credentialName == "" && len(cloudCredentials.AuthCredentials) == 1 {
			for credentialName = range cloudCredentials.AuthCredentials {
			}
		}
	}
	credential, ok := cloudCredentials.AuthCredentials[credentialName]
	if !ok {
		return nil, "", "", errors.NotFoundf(
			"%q credential for cloud %q", credentialName, cloudName,
		)
	}
	return &credential, credentialName, cloudCredentials.DefaultRegion, nil
}

func (c *bootstrapCommand) getCredentials(
	ctx *cmd.Context,
	cloudName string,
	cloud *jujucloud.Cloud,
) (_ *jujucloud.Credential, region string, _ error) {

	credential, credentialName, defaultRegion, err := credentialByName(
		c.CredentialStore, cloudName, c.CredentialName,
	)
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	regionName := c.Region
	if regionName == "" {
		regionName = defaultRegion
	}

	readFile := func(f string) ([]byte, error) {
		f, err := utils.NormalizePath(f)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return ioutil.ReadFile(ctx.AbsPath(f))
	}

	// Finalize credential against schemas supported by the provider.
	provider, err := environs.Provider(cloud.Type)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	credential, err = jujucloud.FinalizeCredential(
		*credential, provider.CredentialSchemas(), readFile,
	)
	if err != nil {
		return nil, "", errors.Annotatef(
			err, "validating %q credential for cloud %q",
			credentialName, cloudName,
		)
	}
	return credential, regionName, nil
}

// getRegion returns the cloud.Region to use, based on the specified
// region name, and the region name selected if none was specified.
//
// If no region name is specified, and there is at least one region,
// we use the first region in the list.
//
// If no region name is specified, and there are no regions at all,
// then we synthesise a region from the cloud's endpoint information
// and just pass this on to the provider.
func getRegion(cloud *jujucloud.Cloud, cloudName, regionName string) (jujucloud.Region, error) {
	if len(cloud.Regions) == 0 {
		// The cloud does not specify regions, so assume
		// that the cloud provider does not have a concept
		// of regions, or has no pre-defined regions, and
		// defer validation to the provider.
		region := jujucloud.Region{
			regionName,
			cloud.Endpoint,
			cloud.StorageEndpoint,
		}
		return region, nil
	}
	if regionName == "" {
		// No region was specified, use the first region in the list.
		return cloud.Regions[0], nil
	}
	for _, region := range cloud.Regions {
		if region.Name == regionName {
			return region, nil
		}
	}
	return jujucloud.Region{}, errors.NewNotFound(nil, fmt.Sprintf(
		"region %q in cloud %q not found (expected one of %q)",
		regionName, cloudName, cloudRegionNames(cloud),
	))
}

func cloudRegionNames(cloud *jujucloud.Cloud) []string {
	var regionNames []string
	for _, region := range cloud.Regions {
		regionNames = append(regionNames, region.Name)
	}
	return regionNames
}

var (
	bootstrapReadyPollDelay = 1 * time.Second
	bootstrapReadyPollCount = 60
	blockAPI                = getBlockAPI
)

// getBlockAPI returns a block api for listing blocks.
func getBlockAPI(c *modelcmd.ModelCommandBase) (block.BlockListAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	return apiblock.NewClient(root), nil
}

// waitForAgentInitialisation polls the bootstrapped controller with a read-only
// command which will fail until the controller is fully initialised.
// TODO(wallyworld) - add a bespoke command to maybe the admin facade for this purpose.
func (c *bootstrapCommand) waitForAgentInitialisation(ctx *cmd.Context) (err error) {
	attempts := utils.AttemptStrategy{
		Min:   bootstrapReadyPollCount,
		Delay: bootstrapReadyPollDelay,
	}
	var client block.BlockListAPI
	for attempt := attempts.Start(); attempt.Next(); {
		client, err = blockAPI(&c.ModelCommandBase)
		if err != nil {
			// Logins are prevented whilst space discovery is ongoing.
			errorMessage := err.Error()
			if strings.Contains(errorMessage, "space discovery still in progress") {
				continue
			}
			return err
		}
		_, err = client.List()
		client.Close()
		if err == nil {
			ctx.Infof("Bootstrap complete, %s now available.", c.controllerName)
			return nil
		}
		// As the API server is coming up, it goes through a number of steps.
		// Initially the upgrade steps run, but the api server allows some
		// calls to be processed during the upgrade, but not the list blocks.
		// It is also possible that the underlying database causes connections
		// to be dropped as it is initialising, or reconfiguring. These can
		// lead to EOF or "connection is shut down" error messages. We skip
		// these too, hoping that things come back up before the end of the
		// retry poll count.
		errorMessage := err.Error()
		if strings.Contains(errorMessage, apiserver.UpgradeInProgressError.Error()) ||
			strings.HasSuffix(errorMessage, "EOF") ||
			strings.HasSuffix(errorMessage, "connection is shut down") {
			ctx.Infof("Waiting for API to become available")
			continue
		}
		return err
	}
	return err
}

// checkProviderType ensures the provider type is okay.
func checkProviderType(envType string) error {
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
	flag, ok := provisionalProviders[envType]
	if ok && !featureflag.Enabled(flag) {
		msg := `the %q provider is provisional in this version of Juju. To use it anyway, set JUJU_DEV_FEATURE_FLAGS="%s" in your shell model`
		return errors.Errorf(msg, envType, flag)
	}
	return nil
}

// handleBootstrapError is called to clean up if bootstrap fails.
func handleBootstrapError(ctx *cmd.Context, err error, cleanup func() error) {
	ch := make(chan os.Signal, 1)
	ctx.InterruptNotify(ch)
	defer ctx.StopInterruptNotify(ch)
	defer close(ch)
	go func() {
		for _ = range ch {
			fmt.Fprintln(ctx.GetStderr(), "Cleaning up failed bootstrap")
		}
	}()
	if err := cleanup(); err != nil {
		logger.Errorf("error cleaning up: %v", err)
	}
}

var allInstances = func(environ environs.Environ) ([]instance.Instance, error) {
	return environ.AllInstances()
}

// setBootstrapEndpointAddress writes the API endpoint address of the
// bootstrap server into the connection information. This should only be run
// once directly after Bootstrap. It assumes that there is just one instance
// in the environment - the bootstrap instance.
func (c *bootstrapCommand) setBootstrapEndpointAddress(
	legacyStore configstore.Storage,
	environ environs.Environ,
) error {
	instances, err := allInstances(environ)
	if err != nil {
		return errors.Trace(err)
	}
	length := len(instances)
	if length == 0 {
		return errors.Errorf("found no instances, expected at least one")
	}
	if length > 1 {
		logger.Warningf("expected one instance, got %d", length)
	}
	bootstrapInstance := instances[0]

	// Don't use c.ConnectionEndpoint as it attempts to contact the state
	// server if no addresses are found in connection info.
	netAddrs, err := bootstrapInstance.Addresses()
	if err != nil {
		return errors.Annotate(err, "failed to get bootstrap instance addresses")
	}
	cfg := environ.Config()
	apiPort := cfg.APIPort()
	apiHostPorts := network.AddressesWithPort(netAddrs, apiPort)
	return juju.UpdateControllerAddresses(c.ClientStore(), legacyStore, c.controllerName, nil, apiHostPorts...)
}
