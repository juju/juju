// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/featureflag"
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6-unstable"
	"launchpad.net/gnuflag"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/sync"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	jujuversion "github.com/juju/juju/version"
)

// provisionalProviders is the names of providers that are hidden behind
// feature flags.
var provisionalProviders = map[string]string{
	"vsphere": feature.VSphereProvider,
}

var usageBootstrapSummary = `
Initializes a cloud environment.`[1:]

var usageBootstrapDetails = `
Used without arguments, bootstrap will step you through the process of 
initializing a Juju cloud environment. Initialization consists of creating
a 'controller' model and provisioning a machine to act as controller.

We recommend you call your controller ‘username-region’ e.g. ‘fred-us-west-1’
See --clouds for a list of clouds and credentials.
See --regions <cloud> for a list of available regions for a given cloud.

Credentials are set beforehand and are distinct from any other 
configuration (see `[1:] + "`juju add-credential`" + `).
The 'controller' model typically does not run workloads. It should remain
pristine to run and manage Juju's own infrastructure for the corresponding
cloud. Additional (hosted) models should be created with ` + "`juju create-\nmodel`" + ` for workload purposes.
Note that a 'default' model is also created and becomes the current model
of the environment once the command completes. It can be discarded if
other models are created.

If '--bootstrap-constraints' is used, its values will also apply to any
future controllers provisioned for high availability (HA).

If '--constraints' is used, its values will be set as the default 
constraints for all future workload machines in the model, exactly as if 
the constraints were set with ` + "`juju set-model-constraints`" + `.

It is possible to override constraints and the automatic machine selection
algorithm by assigning a "placement directive" via the '--to' option. This
dictates what machine to use for the controller. This would typically be 
used with the MAAS provider ('--to <host>.maas').

You can change the default timeout and retry delays used during the 
bootstrap by changing the following settings in your configuration
(all values represent number of seconds):
    # How long to wait for a connection to the controller
    bootstrap-timeout: 600 # default: 10 minutes
    # How long to wait between connection attempts to a controller 
address.
    bootstrap-retry-delay: 5 # default: 5 seconds
    # How often to refresh controller addresses from the API server.
    bootstrap-addresses-delay: 10 # default: 10 seconds
    
Private clouds may need to specify their own custom image metadata and
tools/agent. Use '--metadata-source' whose value is a local directory.
The value of '--agent-version' will become the default tools version to
use in all models for this controller. The full binary version is accepted
(e.g.: 2.0.1-xenial-amd64) but only the numeric version (e.g.: 2.0.1) is
used. Otherwise, by default, the version used is that of the client.

Examples:
    juju bootstrap
    juju bootstrap --clouds
    juju bootstrap --regions aws
    juju bootstrap joe-us-east1 google
    juju bootstrap --config=~/config-rs.yaml joe-syd rackspace
    juju bootstrap --config agent-version=1.25.3 joe-us-east-1 aws
    juju bootstrap --config bootstrap-timeout=1200 joe-eastus azure

See also: 
    add-credentials
    add-model
    set-constraints`

// defaultHostedModelName is the name of the hosted model created in each
// controller for deploying workloads to, in addition to the "controller" model.
const defaultHostedModelName = "default"

func newBootstrapCommand() cmd.Command {
	return modelcmd.Wrap(
		&bootstrapCommand{},
		modelcmd.ModelSkipFlags, modelcmd.ModelSkipDefault,
	)
}

// bootstrapCommand is responsible for launching the first machine in a juju
// environment, and setting up everything necessary to continue working.
type bootstrapCommand struct {
	modelcmd.ModelCommandBase

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
	config                common.ConfigFlag

	showClouds          bool
	showRegionsForCloud string
	controllerName      string
	hostedModelName     string
	CredentialName      string
	Cloud               string
	Region              string
	noGUI               bool
}

func (c *bootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "bootstrap",
		Args:    "<controller name> <cloud name>[/region]",
		Purpose: usageBootstrapSummary,
		Doc:     usageBootstrapDetails,
	}
}

func (c *bootstrapCommand) SetFlags(f *gnuflag.FlagSet) {
	f.Var(constraints.ConstraintsValue{Target: &c.Constraints}, "constraints", "Set model constraints")
	f.Var(constraints.ConstraintsValue{Target: &c.BootstrapConstraints}, "bootstrap-constraints", "Specify bootstrap machine constraints")
	f.StringVar(&c.BootstrapSeries, "bootstrap-series", "", "Specify the series of the bootstrap machine")
	if featureflag.Enabled(feature.ImageMetadata) {
		f.StringVar(&c.BootstrapImage, "bootstrap-image", "", "Specify the image of the bootstrap machine")
	}
	f.BoolVar(&c.UploadTools, "upload-tools", false, "Upload local version of tools before bootstrapping")
	f.StringVar(&c.MetadataSource, "metadata-source", "", "Local path to use as tools and/or metadata source")
	f.StringVar(&c.Placement, "to", "", "Placement directive indicating an instance to bootstrap")
	f.BoolVar(&c.KeepBrokenEnvironment, "keep-broken", false, "Do not destroy the model if bootstrap fails")
	f.BoolVar(&c.AutoUpgrade, "auto-upgrade", false, "Upgrade to the latest patch release tools on first bootstrap")
	f.StringVar(&c.AgentVersionParam, "agent-version", "", "Version of tools to use for Juju agents")
	f.StringVar(&c.CredentialName, "credential", "", "Credentials to use when bootstrapping")
	f.Var(&c.config, "config", "Specify a controller configuration file, or one or more configuration\n    options\n    (--config config.yaml [--config key=value ...])")
	f.StringVar(&c.hostedModelName, "d", defaultHostedModelName, "Name of the default hosted model for the controller")
	f.StringVar(&c.hostedModelName, "default-model", defaultHostedModelName, "Name of the default hosted model for the controller")
	f.BoolVar(&c.noGUI, "no-gui", false, "Do not install the Juju GUI in the controller when bootstrapping")
	f.BoolVar(&c.showClouds, "clouds", false, "Print the available clouds which can be used to bootstrap a Juju environment")
	f.StringVar(&c.showRegionsForCloud, "regions", "", "Print the available regions for the specified cloud")
}

func (c *bootstrapCommand) Init(args []string) (err error) {
	if c.showClouds && c.showRegionsForCloud != "" {
		return fmt.Errorf("--clouds and --regions can't be used together")
	}
	if c.showClouds {
		return cmd.CheckEmpty(args)
	}
	if c.showRegionsForCloud != "" {
		return cmd.CheckEmpty(args)
	}
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
		vers := jujuversion.Current
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
	if c.AgentVersion != nil && (c.AgentVersion.Major != jujuversion.Current.Major || c.AgentVersion.Minor != jujuversion.Current.Minor) {
		return fmt.Errorf("requested agent version major.minor mismatch")
	}

	// The user must specify two positional arguments: the controller name,
	// and the cloud name (optionally with region specified).
	if len(args) < 2 {
		return errors.New("controller name and cloud name are required")
	}
	c.controllerName = args[0]
	c.Cloud = args[1]
	if i := strings.IndexRune(c.Cloud, '/'); i > 0 {
		c.Cloud, c.Region = c.Cloud[:i], c.Cloud[i+1:]
	}
	return cmd.CheckEmpty(args[2:])
}

// BootstrapInterface provides bootstrap functionality that Run calls to support cleaner testing.
type BootstrapInterface interface {
	Bootstrap(ctx environs.BootstrapContext, environ environs.Environ, args bootstrap.BootstrapParams) error
	CloudRegionDetector(environs.EnvironProvider) (environs.CloudRegionDetector, bool)
}

type bootstrapFuncs struct{}

func (b bootstrapFuncs) Bootstrap(ctx environs.BootstrapContext, env environs.Environ, args bootstrap.BootstrapParams) error {
	return bootstrap.Bootstrap(ctx, env, args)
}

func (b bootstrapFuncs) CloudRegionDetector(provider environs.EnvironProvider) (environs.CloudRegionDetector, bool) {
	detector, ok := provider.(environs.CloudRegionDetector)
	return detector, ok
}

var getBootstrapFuncs = func() BootstrapInterface {
	return &bootstrapFuncs{}
}

var (
	environsPrepare            = environs.Prepare
	environsDestroy            = environs.Destroy
	waitForAgentInitialisation = common.WaitForAgentInitialisation
)

var ambiguousCredentialError = errors.New(`
more than one credential detected
run juju autoload-credentials and specify a credential using the --credential argument`[1:],
)

// Run connects to the environment specified on the command line and bootstraps
// a juju in that environment if none already exists. If there is as yet no environments.yaml file,
// the user is informed how to create one.
func (c *bootstrapCommand) Run(ctx *cmd.Context) (resultErr error) {
	if c.showClouds {
		return printClouds(ctx, c.ClientStore())
	}
	if c.showRegionsForCloud != "" {
		return printCloudRegions(ctx, c.showRegionsForCloud)
	}

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
			return errors.NewNotFound(nil, fmt.Sprintf("unknown cloud %q, please try %q", c.Cloud, "juju update-clouds"))
		} else if err != nil {
			return errors.Trace(err)
		}
		detector, ok := bootstrapFuncs.CloudRegionDetector(provider)
		if !ok {
			ctx.Verbosef(
				"provider %q does not support detecting regions",
				c.Cloud,
			)
			return errors.NewNotFound(nil, fmt.Sprintf("unknown cloud %q, please try %q", c.Cloud, "juju update-clouds"))
		}
		var cloudEndpoint string
		regions, err := detector.DetectRegions()
		if errors.IsNotFound(err) {
			// It's not an error to have no regions. If the
			// provider does not support regions, then we
			// reinterpret the supplied region name as the
			// cloud's endpoint. This enables the user to
			// supply, for example, maas/<IP> or manual/<IP>.
			if c.Region != "" {
				ctx.Verbosef("interpreting %q as the cloud endpoint")
				cloudEndpoint = c.Region
				c.Region = ""
			}
		} else if err != nil {
			return errors.Annotatef(err,
				"detecting regions for %q cloud provider",
				c.Cloud,
			)
		}
		schemas := provider.CredentialSchemas()
		authTypes := make([]jujucloud.AuthType, 0, len(schemas))
		for authType := range schemas {
			authTypes = append(authTypes, authType)
		}
		cloud = &jujucloud.Cloud{
			Type:      c.Cloud,
			AuthTypes: authTypes,
			Endpoint:  cloudEndpoint,
			Regions:   regions,
		}
	} else if err != nil {
		return errors.Trace(err)
	}
	if err := checkProviderType(cloud.Type); errors.IsNotFound(err) {
		// This error will get handled later.
	} else if err != nil {
		return errors.Trace(err)
	}

	// Custom clouds may not have explicitly declared support for any auth types.
	if len(cloud.AuthTypes) == 0 {
		cloud.AuthTypes = append(cloud.AuthTypes, jujucloud.EmptyAuthType)
	}

	// Get the credentials and region name.
	store := c.ClientStore()
	credential, credentialName, regionName, err := modelcmd.GetCredentials(
		store, c.Region, c.CredentialName, c.Cloud, cloud.Type,
	)
	if errors.IsNotFound(err) && c.CredentialName == "" {
		// No credential was explicitly specified, and no credential
		// was found in credentials.yaml; have the provider detect
		// credentials from the environment.
		ctx.Verbosef("no credentials found, checking environment")
		detected, err := modelcmd.DetectCredential(c.Cloud, cloud.Type)
		if errors.Cause(err) == modelcmd.ErrMultipleCredentials {
			return ambiguousCredentialError
		} else if err != nil {
			return errors.Trace(err)
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
		fmt.Fprintf(ctx.GetStderr(),
			"%s\n\nSpecify an alternative region, or try %q.",
			err, "juju update-clouds",
		)
		return cmd.ErrSilent
	}

	hostedModelUUID, err := utils.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}
	controllerUUID, err := utils.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}

	// Create an environment config from the cloud and credentials.
	configAttrs := map[string]interface{}{
		"type":                       cloud.Type,
		"name":                       environs.ControllerModelName,
		config.UUIDKey:               controllerUUID.String(),
		controller.ControllerUUIDKey: controllerUUID.String(),
	}
	for k, v := range cloud.Config {
		configAttrs[k] = v
	}
	userConfigAttrs, err := c.config.ReadAttrs(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	for k, v := range userConfigAttrs {
		configAttrs[k] = v
	}
	logger.Debugf("preparing controller with config: %v", configAttrs)

	// Read existing current controller so we can clean up on error.
	var oldCurrentController string
	oldCurrentController, err = store.CurrentController()
	if errors.IsNotFound(err) {
		oldCurrentController = ""
	} else if err != nil {
		return errors.Annotate(err, "error reading current controller")
	}

	defer func() {
		if resultErr == nil || errors.IsAlreadyExists(resultErr) {
			return
		}
		if oldCurrentController != "" {
			if err := store.SetCurrentController(oldCurrentController); err != nil {
				logger.Errorf(
					"cannot reset current controller to %q: %v",
					oldCurrentController, err,
				)
			}
		}
		if err := store.RemoveController(c.controllerName); err != nil {
			logger.Errorf(
				"cannot destroy newly created controller %q details: %v",
				c.controllerName, err,
			)
		}
	}()

	environ, err := environsPrepare(
		modelcmd.BootstrapContext(ctx), store,
		environs.PrepareParams{
			BaseConfig:           configAttrs,
			ControllerName:       c.controllerName,
			CloudName:            c.Cloud,
			CloudRegion:          region.Name,
			CloudEndpoint:        region.Endpoint,
			CloudStorageEndpoint: region.StorageEndpoint,
			Credential:           *credential,
			CredentialName:       credentialName,
		},
	)
	if err != nil {
		return errors.Trace(err)
	}

	// Set the current model to the initial hosted model.
	accountName, err := store.CurrentAccount(c.controllerName)
	if err != nil {
		return errors.Trace(err)
	}
	if err := store.UpdateModel(c.controllerName, accountName, c.hostedModelName, jujuclient.ModelDetails{
		hostedModelUUID.String(),
	}); err != nil {
		return errors.Trace(err)
	}
	if err := store.SetCurrentModel(c.controllerName, accountName, c.hostedModelName); err != nil {
		return errors.Trace(err)
	}

	// Set the current controller so "juju status" can be run while
	// bootstrapping is underway.
	if err := store.SetCurrentController(c.controllerName); err != nil {
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
				logger.Infof(`
bootstrap failed but --keep-broken was specified so model is not being destroyed.
When you are finished diagnosing the problem, remember to run juju destroy-model --force
to clean up the model.`[1:])
			} else {
				handleBootstrapError(ctx, resultErr, func() error {
					return environsDestroy(
						c.controllerName, environ, store,
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

	hostedModelConfig := map[string]interface{}{
		"name":         c.hostedModelName,
		config.UUIDKey: hostedModelUUID.String(),
	}

	// We copy across any user supplied attributes to the hosted model config.
	// But only if the attributes have not been removed from the controller
	// model config as part of preparing the controller model.
	controllerModelConfigAttrs := environ.Config().AllAttrs()
	for k, v := range userConfigAttrs {
		if _, ok := controllerModelConfigAttrs[k]; ok {
			hostedModelConfig[k] = v
		}
	}
	// Ensure that certain config attributes are not included in the hosted
	// model config. These attributes may be modified during bootstrap; by
	// removing them from this map, we ensure the modified values are
	// inherited.
	delete(hostedModelConfig, config.AuthKeysConfig)
	delete(hostedModelConfig, config.AgentVersionKey)

	// Based on the attribute names in clouds.yaml, create
	// a map of shared config for all models on this cloud.
	sharedAttrs := make(map[string]interface{})
	for k := range cloud.Config {
		if v, ok := controllerModelConfigAttrs[k]; ok {
			sharedAttrs[k] = v
		}
	}

	// Check whether the Juju GUI must be installed in the controller.
	// Leaving this value empty means no GUI will be installed.
	var guiDataSourceBaseURL string
	if !c.noGUI {
		guiDataSourceBaseURL = common.GUIDataSourceBaseURL()
	}

	err = bootstrapFuncs.Bootstrap(modelcmd.BootstrapContext(ctx), environ, bootstrap.BootstrapParams{
		ControllerUUID:       controllerUUID.String(),
		ModelConstraints:     c.Constraints,
		BootstrapConstraints: bootstrapConstraints,
		BootstrapSeries:      c.BootstrapSeries,
		BootstrapImage:       c.BootstrapImage,
		Placement:            c.Placement,
		UploadTools:          c.UploadTools,
		BuildToolsTarball:    sync.BuildToolsTarball,
		AgentVersion:         c.AgentVersion,
		MetadataDir:          metadataDir,
		Cloud:                *cloud,
		CloudName:            c.Cloud,
		CloudRegion:          region.Name,
		CloudCredential:      credential,
		CloudCredentialName:  credentialName,
		ModelConfigDefaults:  sharedAttrs,
		HostedModelConfig:    hostedModelConfig,
		GUIDataSourceBaseURL: guiDataSourceBaseURL,
	})
	if err != nil {
		return errors.Annotate(err, "failed to bootstrap model")
	}

	if err := c.SetModelName(c.hostedModelName); err != nil {
		return errors.Trace(err)
	}

	controllerCfg := controller.ControllerConfig(controllerModelConfigAttrs)
	err = common.SetBootstrapEndpointAddress(c.ClientStore(), c.controllerName, controllerCfg.APIPort(), environ)
	if err != nil {
		return errors.Annotate(err, "saving bootstrap endpoint address")
	}

	// To avoid race conditions when running scripted bootstraps, wait
	// for the controller's machine agent to be ready to accept commands
	// before exiting this bootstrap command.
	return waitForAgentInitialisation(ctx, &c.ModelCommandBase, c.controllerName)
}

// getRegion returns the cloud.Region to use, based on the specified
// region name.  If no region name is specified, and there is at least
// one region, we use the first region in the list.
func getRegion(cloud *jujucloud.Cloud, cloudName, regionName string) (jujucloud.Region, error) {
	if regionName != "" {
		region, err := jujucloud.RegionByName(cloud.Regions, regionName)
		if err != nil {
			return jujucloud.Region{}, errors.Trace(err)
		}
		return *region, nil
	}
	if len(cloud.Regions) > 0 {
		// No region was specified, use the first region in the list.
		return cloud.Regions[0], nil
	}
	return jujucloud.Region{
		"", // no region name
		cloud.Endpoint,
		cloud.StorageEndpoint,
	}, nil
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
