// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/schema"
	"github.com/juju/utils"
	"github.com/juju/utils/featureflag"
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6-unstable"

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
	"github.com/juju/juju/provider/lxd/lxdnames"
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

We recommend you call your controller ‘username-region’ e.g. ‘fred-us-east-1’
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
    juju bootstrap aws
    juju bootstrap aws/us-east-1
    juju bootstrap google joe-us-east1
    juju bootstrap --config=~/config-rs.yaml rackspace joe-syd
    juju bootstrap --config agent-version=1.25.3 aws joe-us-east-1
    juju bootstrap --config bootstrap-timeout=1200 azure joe-eastus

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
		modelcmd.WrapSkipModelFlags, modelcmd.WrapSkipDefaultModel,
	)
}

// bootstrapCommand is responsible for launching the first machine in a juju
// environment, and setting up everything necessary to continue working.
type bootstrapCommand struct {
	modelcmd.ModelCommandBase

	Constraints             constraints.Value
	ConstraintsStr          string
	BootstrapConstraints    constraints.Value
	BootstrapConstraintsStr string
	BootstrapSeries         string
	BootstrapImage          string
	BuildAgent              bool
	MetadataSource          string
	Placement               string
	KeepBrokenEnvironment   bool
	AutoUpgrade             bool
	AgentVersionParam       string
	AgentVersion            *version.Number
	config                  common.ConfigFlag
	modelDefaults           common.ConfigFlag

	showClouds          bool
	showRegionsForCloud string
	controllerName      string
	hostedModelName     string
	CredentialName      string
	Cloud               string
	Region              string
	noGUI               bool
	interactive         bool
}

func (c *bootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "bootstrap",
		Args:    "[<cloud name>[/region] [<controller name>]]",
		Purpose: usageBootstrapSummary,
		Doc:     usageBootstrapDetails,
	}
}

func (c *bootstrapCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.ConstraintsStr, "constraints", "", "Set model constraints")
	f.StringVar(&c.BootstrapConstraintsStr, "bootstrap-constraints", "", "Specify bootstrap machine constraints")
	f.StringVar(&c.BootstrapSeries, "bootstrap-series", "", "Specify the series of the bootstrap machine")
	if featureflag.Enabled(feature.ImageMetadata) {
		f.StringVar(&c.BootstrapImage, "bootstrap-image", "", "Specify the image of the bootstrap machine")
	}
	f.BoolVar(&c.BuildAgent, "build-agent", false, "Build local version of agent binary before bootstrapping")
	f.StringVar(&c.MetadataSource, "metadata-source", "", "Local path to use as tools and/or metadata source")
	f.StringVar(&c.Placement, "to", "", "Placement directive indicating an instance to bootstrap")
	f.BoolVar(&c.KeepBrokenEnvironment, "keep-broken", false, "Do not destroy the model if bootstrap fails")
	f.BoolVar(&c.AutoUpgrade, "auto-upgrade", false, "Upgrade to the latest patch release tools on first bootstrap")
	f.StringVar(&c.AgentVersionParam, "agent-version", "", "Version of tools to use for Juju agents")
	f.StringVar(&c.CredentialName, "credential", "", "Credentials to use when bootstrapping")
	f.Var(&c.config, "config", "Specify a controller configuration file, or one or more configuration\n    options\n    (--config config.yaml [--config key=value ...])")
	f.Var(&c.modelDefaults, "model-default", "Specify a configuration file, or one or more configuration\n    options to be set for all models, unless otherwise specified\n    (--config config.yaml [--config key=value ...])")
	f.StringVar(&c.hostedModelName, "d", defaultHostedModelName, "Name of the default hosted model for the controller")
	f.StringVar(&c.hostedModelName, "default-model", defaultHostedModelName, "Name of the default hosted model for the controller")
	f.BoolVar(&c.noGUI, "no-gui", false, "Do not install the Juju GUI in the controller when bootstrapping")
	f.BoolVar(&c.showClouds, "clouds", false, "Print the available clouds which can be used to bootstrap a Juju environment")
	f.StringVar(&c.showRegionsForCloud, "regions", "", "Print the available regions for the specified cloud")
}

func (c *bootstrapCommand) Init(args []string) (err error) {
	if c.showClouds && c.showRegionsForCloud != "" {
		return errors.New("--clouds and --regions can't be used together")
	}
	if c.showClouds {
		return cmd.CheckEmpty(args)
	}
	if c.showRegionsForCloud != "" {
		return cmd.CheckEmpty(args)
	}
	if c.AgentVersionParam != "" && c.BuildAgent {
		return errors.New("--agent-version and --build-agent can't be used together")
	}
	if c.BootstrapSeries != "" && !charm.IsValidSeries(c.BootstrapSeries) {
		return errors.NotValidf("series %q", c.BootstrapSeries)
	}

	// Parse the placement directive. Bootstrap currently only
	// supports provider-specific placement directives.
	if c.Placement != "" {
		_, err = instance.ParsePlacement(c.Placement)
		if err != instance.ErrPlacementScopeMissing {
			// We only support unscoped placement directives for bootstrap.
			return errors.Errorf("unsupported bootstrap placement directive %q", c.Placement)
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
		return errors.New("requested agent version major.minor mismatch")
	}

	switch len(args) {
	case 0:
		// no args or flags, go interactive.
		c.interactive = true
		return nil
	}
	c.Cloud = args[0]
	if i := strings.IndexRune(c.Cloud, '/'); i > 0 {
		c.Cloud, c.Region = c.Cloud[:i], c.Cloud[i+1:]
	}
	if len(args) > 1 {
		c.controllerName = args[1]
		return cmd.CheckEmpty(args[2:])
	}
	return nil
}

// BootstrapInterface provides bootstrap functionality that Run calls to support cleaner testing.
type BootstrapInterface interface {
	// Bootstrap bootstraps a controller.
	Bootstrap(ctx environs.BootstrapContext, environ environs.Environ, args bootstrap.BootstrapParams) error

	// CloudDetector returns a CloudDetector for the given provider,
	// if the provider supports it.
	CloudDetector(environs.EnvironProvider) (environs.CloudDetector, bool)

	// CloudRegionDetector returns a CloudRegionDetector for the given provider,
	// if the provider supports it.
	CloudRegionDetector(environs.EnvironProvider) (environs.CloudRegionDetector, bool)
}

type bootstrapFuncs struct{}

func (b bootstrapFuncs) Bootstrap(ctx environs.BootstrapContext, env environs.Environ, args bootstrap.BootstrapParams) error {
	return bootstrap.Bootstrap(ctx, env, args)
}

func (b bootstrapFuncs) CloudDetector(provider environs.EnvironProvider) (environs.CloudDetector, bool) {
	detector, ok := provider.(environs.CloudDetector)
	return detector, ok
}

func (b bootstrapFuncs) CloudRegionDetector(provider environs.EnvironProvider) (environs.CloudRegionDetector, bool) {
	detector, ok := provider.(environs.CloudRegionDetector)
	return detector, ok
}

var getBootstrapFuncs = func() BootstrapInterface {
	return &bootstrapFuncs{}
}

var (
	bootstrapPrepare           = bootstrap.Prepare
	environsDestroy            = environs.Destroy
	waitForAgentInitialisation = common.WaitForAgentInitialisation
)

var ambiguousDetectedCredentialError = errors.New(`
more than one credential detected
run juju autoload-credentials and specify a credential using the --credential argument`[1:],
)

var ambiguousCredentialError = errors.New(`
more than one credential is available
specify a credential using the --credential argument`[1:],
)

func (c *bootstrapCommand) parseConstraints(ctx *cmd.Context) (err error) {
	allAliases := map[string]string{}
	defer common.WarnConstraintAliases(ctx, allAliases)
	if c.ConstraintsStr != "" {
		cons, aliases, err := constraints.ParseWithAliases(c.ConstraintsStr)
		for k, v := range aliases {
			allAliases[k] = v
		}
		if err != nil {
			return err
		}
		c.Constraints = cons
	}
	if c.BootstrapConstraintsStr != "" {
		cons, aliases, err := constraints.ParseWithAliases(c.BootstrapConstraintsStr)
		for k, v := range aliases {
			allAliases[k] = v
		}
		if err != nil {
			return err
		}
		c.BootstrapConstraints = cons
	}
	return nil
}

// Run connects to the environment specified on the command line and bootstraps
// a juju in that environment if none already exists. If there is as yet no environments.yaml file,
// the user is informed how to create one.
func (c *bootstrapCommand) Run(ctx *cmd.Context) (resultErr error) {
	defer func() {
		resultErr = handleChooseCloudRegionError(ctx, resultErr)
	}()

	if err := c.parseConstraints(ctx); err != nil {
		return err
	}

	// Start by checking for usage errors, requests for information
	finished, err := c.handleCommandLineErrorsAndInfoRequests(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if finished {
		return nil
	}

	// Run interactive bootstrap if needed/asked for
	if c.interactive {
		if err := c.runInteractive(ctx); err != nil {
			return errors.Trace(err)
		}
		// now run normal bootstrap using info gained above.
	}

	cloud, err := c.cloud(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	provider, err := environs.Provider(cloud.Type)
	if err != nil {
		return errors.Trace(err)
	}
	// Custom clouds may not have explicitly declared support for any auth-
	// types, in which case we'll assume that they support everything that
	// the provider supports.
	if len(cloud.AuthTypes) == 0 {
		for authType := range provider.CredentialSchemas() {
			cloud.AuthTypes = append(cloud.AuthTypes, authType)
		}
	}

	credentials, regionName, err := c.credentialsAndRegionName(ctx, provider, cloud)
	if err != nil {
		return errors.Trace(err)
	}

	region, err := common.ChooseCloudRegion(*cloud, regionName)
	if err != nil {
		return errors.Trace(err)
	}
	if c.controllerName == "" {
		c.controllerName = defaultControllerName(cloud.Name, region.Name)
	}

	config, err := c.bootstrapConfigs(ctx, cloud, provider)
	if err != nil {
		return errors.Trace(err)
	}

	// Read existing current controller so we can clean up on error.
	var oldCurrentController string
	store := c.ClientStore()
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

	environ, err := bootstrapPrepare(
		modelcmd.BootstrapContext(ctx), store,
		bootstrap.PrepareParams{
			ModelConfig:      config.bootstrapModel,
			ControllerConfig: config.controller,
			ControllerName:   c.controllerName,
			Cloud: environs.CloudSpec{
				Type:             cloud.Type,
				Name:             cloud.Name,
				Region:           region.Name,
				Endpoint:         region.Endpoint,
				IdentityEndpoint: region.IdentityEndpoint,
				StorageEndpoint:  region.StorageEndpoint,
				Credential:       credentials.credential,
			},
			CredentialName: credentials.name,
			AdminSecret:    config.bootstrap.AdminSecret,
		},
	)
	if err != nil {
		return errors.Trace(err)
	}

	hostedModelUUID, err := utils.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}

	// Set the current model to the initial hosted model.
	if err := store.UpdateModel(c.controllerName, c.hostedModelName, jujuclient.ModelDetails{
		hostedModelUUID.String(),
	}); err != nil {
		return errors.Trace(err)
	}
	if err := store.SetCurrentModel(c.controllerName, c.hostedModelName); err != nil {
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
				ctx.Infof(`
bootstrap failed but --keep-broken was specified so resources are not being destroyed.
When you have finished diagnosing the problem, remember to clean up the failed controller.
See `[1:] + "`juju kill-controller`" + `.`)
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

	hostedModelConfig := c.hostedModelConfig(
		hostedModelUUID, config.inheritedControllerAttrs, config.userConfigAttrs, environ)

	// Check whether the Juju GUI must be installed in the controller.
	// Leaving this value empty means no GUI will be installed.
	var guiDataSourceBaseURL string
	if !c.noGUI {
		guiDataSourceBaseURL = common.GUIDataSourceBaseURL()
	}

	if credentials.name == "" {
		// credentialName will be empty if the credential was detected.
		// We must supply a name for the credential in the database,
		// so choose one.
		credentials.name = credentials.detectedName
	}

	bootstrapFuncs := getBootstrapFuncs()
	err = bootstrapFuncs.Bootstrap(modelcmd.BootstrapContext(ctx), environ, bootstrap.BootstrapParams{
		ModelConstraints:          c.Constraints,
		BootstrapConstraints:      bootstrapConstraints,
		BootstrapSeries:           c.BootstrapSeries,
		BootstrapImage:            c.BootstrapImage,
		Placement:                 c.Placement,
		BuildAgent:                c.BuildAgent,
		BuildAgentTarball:         sync.BuildAgentTarball,
		AgentVersion:              c.AgentVersion,
		MetadataDir:               metadataDir,
		Cloud:                     *cloud,
		CloudRegion:               region.Name,
		CloudCredential:           credentials.credential,
		CloudCredentialName:       credentials.name,
		ControllerConfig:          config.controller,
		ControllerInheritedConfig: config.inheritedControllerAttrs,
		RegionInheritedConfig:     cloud.RegionConfig,
		HostedModelConfig:         hostedModelConfig,
		GUIDataSourceBaseURL:      guiDataSourceBaseURL,
		AdminSecret:               config.bootstrap.AdminSecret,
		CAPrivateKey:              config.bootstrap.CAPrivateKey,
		DialOpts: environs.BootstrapDialOpts{
			Timeout:        config.bootstrap.BootstrapTimeout,
			RetryDelay:     config.bootstrap.BootstrapRetryDelay,
			AddressesDelay: config.bootstrap.BootstrapAddressesDelay,
		},
	})
	if err != nil {
		return errors.Annotate(err, "failed to bootstrap model")
	}

	if err := c.SetModelName(modelcmd.JoinModelName(c.controllerName, c.hostedModelName)); err != nil {
		return errors.Trace(err)
	}

	agentVersion := jujuversion.Current
	if c.AgentVersion != nil {
		agentVersion = *c.AgentVersion
	}
	err = common.SetBootstrapEndpointAddress(c.ClientStore(), c.controllerName, agentVersion, config.controller.APIPort(), environ)
	if err != nil {
		return errors.Annotate(err, "saving bootstrap endpoint address")
	}

	// To avoid race conditions when running scripted bootstraps, wait
	// for the controller's machine agent to be ready to accept commands
	// before exiting this bootstrap command.
	return waitForAgentInitialisation(ctx, &c.ModelCommandBase, c.controllerName, c.hostedModelName)
}

func (c *bootstrapCommand) handleCommandLineErrorsAndInfoRequests(ctx *cmd.Context) (bool, error) {
	if c.BootstrapImage != "" {
		if c.BootstrapSeries == "" {
			return true, errors.Errorf("--bootstrap-image must be used with --bootstrap-series")
		}
		cons, err := constraints.Merge(c.Constraints, c.BootstrapConstraints)
		if err != nil {
			return true, errors.Trace(err)
		}
		if !cons.HasArch() {
			return true, errors.Errorf("--bootstrap-image must be used with --bootstrap-constraints, specifying architecture")
		}
	}
	if c.showClouds {
		return true, printClouds(ctx, c.ClientStore())
	}
	if c.showRegionsForCloud != "" {
		return true, printCloudRegions(ctx, c.showRegionsForCloud)
	}

	return false, nil
}

func (c *bootstrapCommand) cloud(ctx *cmd.Context) (*jujucloud.Cloud, error) {
	bootstrapFuncs := getBootstrapFuncs()

	// Get the cloud definition identified by c.Cloud. If c.Cloud does not
	// identify a cloud in clouds.yaml, then we check if any of the
	// providers can detect a cloud with the given name. Otherwise, if the
	// cloud name identifies a provider *type* (e.g. "openstack"), then we
	// check if that provider can detect cloud regions, and synthesise a
	// cloud with those regions.
	cloud, err := jujucloud.CloudByName(c.Cloud)
	if errors.IsNotFound(err) {
		cloud, err = c.detectCloud(ctx, bootstrapFuncs)
		if err != nil {
			return nil, errors.Trace(err)
		}
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	if err := checkProviderType(cloud.Type); errors.IsNotFound(err) {
		// This error will get handled later.
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	return cloud, nil
}

func (c *bootstrapCommand) detectCloud(ctx *cmd.Context, bootstrapFuncs BootstrapInterface) (*jujucloud.Cloud, error) {
	// Check if any of the registered providers can give us a cloud with
	// the specified name. The first one wins.
	for _, providerType := range environs.RegisteredProviders() {
		provider, err := environs.Provider(providerType)
		if err != nil {
			return nil, errors.Trace(err)
		}
		cloudDetector, ok := bootstrapFuncs.CloudDetector(provider)
		if !ok {
			continue
		}
		cloud, err := cloudDetector.DetectCloud(c.Cloud)
		if errors.IsNotFound(err) {
			continue
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		return &cloud, nil
	}

	ctx.Verbosef("cloud %q not found, trying as a provider name", c.Cloud)
	provider, err := environs.Provider(c.Cloud)
	if errors.IsNotFound(err) {
		return nil, errors.NewNotFound(nil, fmt.Sprintf("unknown cloud %q, please try %q", c.Cloud, "juju update-clouds"))
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	regionDetector, ok := bootstrapFuncs.CloudRegionDetector(provider)
	if !ok {
		ctx.Verbosef(
			"provider %q does not support detecting regions",
			c.Cloud,
		)
		return nil, errors.NewNotFound(nil, fmt.Sprintf("unknown cloud %q, please try %q", c.Cloud, "juju update-clouds"))
	}

	var cloudEndpoint string
	regions, err := regionDetector.DetectRegions()
	if errors.IsNotFound(err) {
		// It's not an error to have no regions. If the
		// provider does not support regions, then we
		// reinterpret the supplied region name as the
		// cloud's endpoint. This enables the user to
		// supply, for example, maas/<IP> or manual/<IP>.
		if c.Region != "" {
			ctx.Verbosef("interpreting %q as the cloud endpoint", c.Region)
			cloudEndpoint = c.Region
			c.Region = ""
		}
	} else if err != nil {
		return nil, errors.Annotatef(err,
			"detecting regions for %q cloud provider",
			c.Cloud,
		)
	}
	schemas := provider.CredentialSchemas()
	authTypes := make([]jujucloud.AuthType, 0, len(schemas))
	for authType := range schemas {
		authTypes = append(authTypes, authType)
	}

	// Since we are iterating over a map, lets sort the authTypes so
	// they are always in a consistent order.
	sort.Sort(jujucloud.AuthTypes(authTypes))
	return &jujucloud.Cloud{
		Name:      c.Cloud,
		Type:      c.Cloud,
		AuthTypes: authTypes,
		Endpoint:  cloudEndpoint,
		Regions:   regions,
	}, nil
}

type bootstrapCredentials struct {
	credential   *jujucloud.Credential
	name         string
	detectedName string
}

// Get the credentials and region name.
func (c *bootstrapCommand) credentialsAndRegionName(
	ctx *cmd.Context,
	provider environs.EnvironProvider,
	cloud *jujucloud.Cloud,
) (
	creds bootstrapCredentials,
	regionName string,
	err error,
) {
	var detected bool
	store := c.ClientStore()
	creds.credential, creds.name, regionName, detected, err = common.GetOrDetectCredential(
		ctx, store, provider, modelcmd.GetCredentialsParams{
			Cloud:          *cloud,
			CloudRegion:    c.Region,
			CredentialName: c.CredentialName,
		},
	)
	switch errors.Cause(err) {
	case nil:
	case modelcmd.ErrMultipleCredentials:
		return bootstrapCredentials{}, "", ambiguousCredentialError
	case common.ErrMultipleDetectedCredentials:
		return bootstrapCredentials{}, "", ambiguousDetectedCredentialError
	default:
		return bootstrapCredentials{}, "", errors.Trace(err)
	}
	logger.Debugf(
		"authenticating with region %q and credential %q (%v)",
		regionName, creds.name, creds.credential.Label,
	)
	if detected {
		creds.detectedName = creds.name
		creds.name = ""
	}
	logger.Tracef("credential: %v", creds.credential)
	return creds, regionName, nil
}

type bootstrapConfigs struct {
	bootstrapModel           map[string]interface{}
	controller               controller.Config
	bootstrap                bootstrap.Config
	inheritedControllerAttrs map[string]interface{}
	userConfigAttrs          map[string]interface{}
}

func (c *bootstrapCommand) bootstrapConfigs(
	ctx *cmd.Context,
	cloud *jujucloud.Cloud,
	provider environs.EnvironProvider,
) (
	bootstrapConfigs,
	error,
) {

	controllerModelUUID, err := utils.NewUUID()
	if err != nil {
		return bootstrapConfigs{}, errors.Trace(err)
	}
	controllerUUID, err := utils.NewUUID()
	if err != nil {
		return bootstrapConfigs{}, errors.Trace(err)
	}

	// Create a model config, and split out any controller
	// and bootstrap config attributes.
	combinedConfig := map[string]interface{}{
		"type":         cloud.Type,
		"name":         bootstrap.ControllerModelName,
		config.UUIDKey: controllerModelUUID.String(),
	}

	userConfigAttrs, err := c.config.ReadAttrs(ctx)
	if err != nil {
		return bootstrapConfigs{}, errors.Trace(err)
	}
	modelDefaultConfigAttrs, err := c.modelDefaults.ReadAttrs(ctx)
	if err != nil {
		return bootstrapConfigs{}, errors.Trace(err)
	}
	// The provider may define some custom attributes specific
	// to the provider. These will be added to the model config.
	providerAttrs := make(map[string]interface{})
	if ps, ok := provider.(config.ConfigSchemaSource); ok {
		for attr := range ps.ConfigSchema() {
			// Start with the model defaults, and if also specified
			// in the user config attrs, they override the model default.
			if v, ok := modelDefaultConfigAttrs[attr]; ok {
				providerAttrs[attr] = v
			}
			if v, ok := userConfigAttrs[attr]; ok {
				providerAttrs[attr] = v
			}
		}
		fields := schema.FieldMap(ps.ConfigSchema(), ps.ConfigDefaults())
		if coercedAttrs, err := fields.Coerce(providerAttrs, nil); err != nil {
			return bootstrapConfigs{},
				errors.Annotatef(err, "invalid attribute value(s) for %v cloud", cloud.Type)
		} else {
			providerAttrs = coercedAttrs.(map[string]interface{})
		}
	}

	bootstrapConfigAttrs := make(map[string]interface{})
	controllerConfigAttrs := make(map[string]interface{})
	// Based on the attribute names in clouds.yaml, create
	// a map of shared config for all models on this cloud.
	inheritedControllerAttrs := make(map[string]interface{})
	for k, v := range cloud.Config {
		switch {
		case bootstrap.IsBootstrapAttribute(k):
			bootstrapConfigAttrs[k] = v
			continue
		case controller.ControllerOnlyAttribute(k):
			controllerConfigAttrs[k] = v
			continue
		}
		inheritedControllerAttrs[k] = v
	}
	// Model defaults are added to the inherited controller attributes.
	// Any command line set model defaults override what is in the cloud config.
	for k, v := range modelDefaultConfigAttrs {
		switch {
		case bootstrap.IsBootstrapAttribute(k):
			return bootstrapConfigs{},
				errors.Errorf("%q is a bootstrap only attribute, and cannot be set as a model-default", k)
		case controller.ControllerOnlyAttribute(k):
			return bootstrapConfigs{},
				errors.Errorf("%q is a controller attribute, and cannot be set as a model-default", k)
		}
		inheritedControllerAttrs[k] = v
	}

	// Start with the model defaults, then add in user config attributes.
	for k, v := range modelDefaultConfigAttrs {
		combinedConfig[k] = v
	}

	// Provider specific attributes are either already specified in model
	// config (but may have been coerced), or were not present. Either way,
	// copy them in.
	logger.Debugf("provider attrs: %v", providerAttrs)
	for k, v := range providerAttrs {
		combinedConfig[k] = v
	}

	for k, v := range inheritedControllerAttrs {
		combinedConfig[k] = v
	}

	for k, v := range userConfigAttrs {
		combinedConfig[k] = v
	}

	// Add in any default attribute values if not already
	// specified, making the recorded bootstrap config
	// immutable to changes in Juju.
	for k, v := range config.ConfigDefaults() {
		if _, ok := combinedConfig[k]; !ok {
			combinedConfig[k] = v
		}
	}

	bootstrapModelConfig := make(map[string]interface{})
	for k, v := range combinedConfig {
		switch {
		case bootstrap.IsBootstrapAttribute(k):
			bootstrapConfigAttrs[k] = v
		case controller.ControllerOnlyAttribute(k):
			controllerConfigAttrs[k] = v
		default:
			bootstrapModelConfig[k] = v
		}
	}

	bootstrapConfig, err := bootstrap.NewConfig(bootstrapConfigAttrs)
	if err != nil {
		return bootstrapConfigs{}, errors.Annotate(err, "constructing bootstrap config")
	}
	controllerConfig, err := controller.NewConfig(
		controllerUUID.String(), bootstrapConfig.CACert, controllerConfigAttrs,
	)
	if err != nil {
		return bootstrapConfigs{}, errors.Annotate(err, "constructing controller config")
	}
	if controllerConfig.AutocertDNSName() != "" {
		if _, ok := controllerConfigAttrs[controller.APIPort]; !ok {
			// The configuration did not explicitly mention the API port,
			// so default to 443 because it is not usually possible to
			// obtain autocert certificates without listening on port 443.
			controllerConfig[controller.APIPort] = 443
		}
	}

	if err := common.FinalizeAuthorizedKeys(ctx, bootstrapModelConfig); err != nil {
		return bootstrapConfigs{}, errors.Annotate(err, "finalizing authorized-keys")
	}
	logger.Debugf("preparing controller with config: %v", bootstrapModelConfig)

	configs := bootstrapConfigs{
		bootstrapModel:           bootstrapModelConfig,
		controller:               controllerConfig,
		bootstrap:                bootstrapConfig,
		inheritedControllerAttrs: inheritedControllerAttrs,
		userConfigAttrs:          userConfigAttrs,
	}
	return configs, nil
}

func (c *bootstrapCommand) hostedModelConfig(
	hostedModelUUID utils.UUID,
	inheritedControllerAttrs,
	userConfigAttrs map[string]interface{},
	environ environs.Environ,
) map[string]interface{} {

	hostedModelConfig := map[string]interface{}{
		"name":         c.hostedModelName,
		config.UUIDKey: hostedModelUUID.String(),
	}
	for k, v := range inheritedControllerAttrs {
		hostedModelConfig[k] = v
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
	delete(hostedModelConfig, config.AuthorizedKeysKey)
	delete(hostedModelConfig, config.AgentVersionKey)

	return hostedModelConfig
}

// runInteractive queries the user about bootstrap config interactively at the
// command prompt.
func (c *bootstrapCommand) runInteractive(ctx *cmd.Context) error {
	scanner := bufio.NewScanner(ctx.Stdin)
	clouds, err := assembleClouds()
	if err != nil {
		return errors.Trace(err)
	}
	c.Cloud, err = queryCloud(clouds, lxdnames.DefaultCloud, scanner, ctx.Stdout)
	if err != nil {
		return errors.Trace(err)
	}
	cloud, err := jujucloud.CloudByName(c.Cloud)
	if err != nil {
		return errors.Trace(err)
	}

	switch len(cloud.Regions) {
	case 0:
		// No region to choose, nothing to do.
	case 1:
		// If there's just one, don't prompt, just use it.
		c.Region = cloud.Regions[0].Name
	default:
		c.Region, err = queryRegion(c.Cloud, cloud.Regions, scanner, ctx.Stdout)
		if err != nil {
			return errors.Trace(err)
		}
	}

	defName := defaultControllerName(c.Cloud, c.Region)

	c.controllerName, err = queryName(defName, scanner, ctx.Stdout)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
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

func handleChooseCloudRegionError(ctx *cmd.Context, err error) error {
	if !common.IsChooseCloudRegionError(err) {
		return err
	}
	fmt.Fprintf(ctx.GetStderr(),
		"%s\n\nSpecify an alternative region, or try %q.\n",
		err, "juju update-clouds",
	)
	return cmd.ErrSilent
}
