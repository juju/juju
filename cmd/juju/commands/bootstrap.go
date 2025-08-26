// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/juju/charm/v12"
	jujuclock "github.com/juju/clock"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"
	"github.com/juju/schema"
	"github.com/juju/utils/v3"
	"github.com/juju/utils/v3/keyvalues"
	"github.com/juju/version/v2"

	"github.com/juju/juju/caas"
	k8s "github.com/juju/juju/caas/kubernetes"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/constants"
	"github.com/juju/juju/cmd/juju/application/refresher"
	"github.com/juju/juju/cmd/juju/common"
	cmdcontroller "github.com/juju/juju/cmd/juju/controller"
	cmdmodel "github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/controller"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	envcontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/sync"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/internal/naturalsort"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/provider/lxd/lxdnames"
	"github.com/juju/juju/proxy"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	jujuversion "github.com/juju/juju/version"
)

// provisionalProviders is the names of providers that are hidden behind
// feature flags.
var provisionalProviders = map[string]string{}

var usageBootstrapSummary = `
Initializes a cloud environment.`[1:]

var usageBootstrapDetailsPartOne = `
Used without arguments, bootstrap will step you through the process of
initializing a Juju cloud environment. Initialization consists of creating
a 'controller' model and provisioning a machine to act as controller.

Controller names may only contain lowercase letters, digits and hyphens, and
may not start with a hyphen.
We recommend you call your controller ‘username-region’ e.g. ‘fred-us-east-1’.
See ` + "`--clouds`" + ` for a list of clouds and credentials.
See ` + "`--regions <cloud>`" + ` for a list of available regions for a given cloud.

Credentials are set beforehand and are distinct from any other
configuration (see `[1:] + "`juju add-credential`" + `).

The 'controller' model typically does not run workloads. It should remain
pristine to run and manage Juju's own infrastructure for the corresponding
cloud. Additional models should be created with ` + "`juju add-model`" + ` for workload purposes.

If ` + "`--bootstrap-constraints`" + ` is used, its values will also apply to any
future controllers provisioned for high availability (HA).

If ` + "`--constraints`" + ` is used, its values will be set as the default
constraints for all future workload machines in the model, exactly as if
the constraints were set with ` + "`juju set-model-constraints`" + `.

It is possible to override constraints and the automatic machine selection
algorithm by assigning a "placement directive" via the '--to' option. This
dictates what machine to use for the controller. This would typically be
used with the MAAS provider (` + "`--to <host>.maas`" + `).

You can change the default timeout and retry delays used during the
bootstrap by changing the following settings in your configuration
(all values represent number of seconds):

    # How long to wait for a connection to the controller
    bootstrap-timeout: 1200  # default: 20 minutes
    # How long to wait between connection attempts to a controller address.
    bootstrap-retry-delay: 5  # default: 5 seconds
    # How often to refresh controller addresses from the API server.
    bootstrap-addresses-delay: 10  # default: 10 seconds

It is possible to override the base e.g. ` + "`ubuntu@22.04`" + `, Juju attempts
to bootstrap on to, by supplying a base argument to ` + "`--bootstrap-base`" + `.

An error is emitted if the determined base is not supported. Using the
` + "`--force`" + ` option to override this check:

    juju bootstrap --bootstrap-base=ubuntu@22.04 --force

The ` + "`--bootstrap-series`" + ` flag can be still used, but is deprecated in favour
of ` + "`--bootstrap-base`" + `.

Private clouds may need to specify their own custom image metadata and
tools/agent. Use ` + "`--metadata-source`" + ` whose value is a local directory.

By default, the Juju version of the agent binary that is downloaded and
installed on all models for the new controller will be the same as that
of the Juju client used to perform the bootstrap.
However, a user can specify a different agent version via the ` + "`--agent-version`" + `
option to bootstrap command. Juju will use this version for models' agents
as long as the client's version is from the same Juju release base.
In other words, a 3.6.1 client can bootstrap any 3.6.x agents but cannot
bootstrap any 3.5.x or 3.4.x agents.
The agent version can be specified a simple numeric version, e.g. 3.6.9.

For example, at the time when 3.6.0, 3.6.1 and 3.6.2 are released and your
agent stream is 'released' (default), then a 3.6.1 client can bootstrap:
   * a 3.6.0 controller by running ` + "`... bootstrap --agent-version=3.6.0 ...`" + `;
   * a 3.6.1 controller by running ` + "`... bootstrap ...`" + `;
   * a 3.6.2 controller by running ` + "`bootstrap --auto-upgrade`" + `.
However, if this client has a copy of the codebase, then a local copy of Juju
will be built and bootstrapped -- 3.6.1.1.

Bootstrapping to a Kubernetes cluster requires that the service set up to handle
requests to the controller be accessible outside the cluster. Typically this
means a service type of LoadBalancer is needed, and Juju does create such a
service if it knows it is supported by the cluster. This is performed by
interrogating the cluster for a well known managed deployment such as microk8s,
GKE or EKS.

When bootstrapping to a Kubernetes cluster Juju does not recognise, there's no
guarantee a load balancer is available, so Juju defaults to a controller
service type of ClusterIP. This may not be suitable, so there are three bootstrap
options available to tell Juju how to set up the controller service. Part of
the solution may require a load balancer for the cluster to be set up manually
first, or perhaps an external k8s service via a FQDN will be used
(this is a cluster specific implementation decision which Juju needs to be
informed about so it can set things up correctly). The three relevant bootstrap
options are (see list of bootstrap config items below for a full explanation):

- ` + "`controller-service-type`" + `
- ` + "`controller-external-name`" + `
- ` + "`controller-external-ips`" + `

Juju advertises those addresses to other controllers, so they must be resolveable from
other controllers for cross-model (cross-controller, actually) relations to work.

If a storage pool is specified using ` + "`--storage-pool`" + `, this will be created
in the controller model.
`

var usageBootstrapConfigTxt = `

Available keys for use with ` + "`--config`" + ` are:
`

var usageBootstrapDetailsPartTwo = `
`

const usageBootstrapExamples = `
    juju bootstrap
    juju bootstrap --clouds
    juju bootstrap --regions aws
    juju bootstrap aws
    juju bootstrap aws/us-east-1
    juju bootstrap google joe-us-east1
    juju bootstrap --config=~/config-rs.yaml google joe-syd
    juju bootstrap --agent-version=2.2.4 aws joe-us-east-1
    juju bootstrap --config bootstrap-timeout=1200 azure joe-eastus
    juju bootstrap aws --storage-pool name=secret --storage-pool type=ebs --storage-pool encrypted=true
	juju bootstrap lxd --bootstrap-base=ubuntu@22.04

For a bootstrap on Kubernetes, setting the service type of the Juju controller service to LoadBalancer:

    juju bootstrap --config controller-service-type=loadbalancer

For a bootstrap on Kubernetes, setting the service type of the Juju controller service to External:

    juju bootstrap --config controller-service-type=external --config controller-external-name=controller.juju.is
`

func newBootstrapCommand() cmd.Command {
	command := &bootstrapCommand{}
	command.clock = jujuclock.WallClock
	command.CanClearCurrentModel = true
	return modelcmd.Wrap(command,
		modelcmd.WrapSkipModelFlags,
		modelcmd.WrapSkipDefaultModel,
	)
}

// bootstrapCommand is responsible for launching the first machine in a juju
// environment, and setting up everything necessary to continue working.
type bootstrapCommand struct {
	modelcmd.ModelCommandBase

	clock jujuclock.Clock

	Constraints              constraints.Value
	ConstraintsStr           common.ConstraintsFlag
	BootstrapConstraints     constraints.Value
	BootstrapConstraintsStr  common.BootstrapConstraintsFlag
	BootstrapSeries          string
	BootstrapBase            string
	BootstrapImage           string
	BuildAgent               bool
	JujuDbSnapPath           string
	JujuDbSnapAssertionsPath string
	MetadataSource           string
	Placement                string
	KeepBrokenEnvironment    bool
	AutoUpgrade              bool
	AgentVersionParam        string
	AgentVersion             *version.Number
	config                   common.ConfigFlag
	modelDefaults            common.ConfigFlag
	storagePool              common.ConfigFlag

	showClouds          bool
	showRegionsForCloud string
	controllerName      string
	CredentialName      string
	Cloud               string
	Region              string
	noSwitch            bool
	interactive         bool

	// initialModelName is the name of a new model to create
	// in addition to the controller model.
	initialModelName string

	ControllerCharmPath       string
	ControllerCharmChannelStr string
	ControllerCharmChannel    charm.Channel

	// Force is used to allow a bootstrap to be run on unsupported series.
	Force bool
}

func (c *bootstrapCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:     "bootstrap",
		Args:     "[<cloud name>[/region] [<controller name>]]",
		Examples: usageBootstrapExamples,
		SeeAlso: []string{
			"add-credential",
			"autoload-credentials",
			"add-model",
			"controller-config",
			"model-config",
			"set-constraints",
			"show-cloud",
		},
		Purpose: usageBootstrapSummary,
	}
	configKeys := c.configDetails()

	info.Doc = fmt.Sprintf("%s%s\n%s%s",
		usageBootstrapDetailsPartOne,
		usageBootstrapConfigTxt,
		configKeys.Format(),
		usageBootstrapDetailsPartTwo)
	return jujucmd.Info(info)
}

// ConfigCategoryKeys represents the collection of keys supported by the
// --config option during bootstrap grouped by the domain category the keys
// apply to within Juju.
type ConfigCategoryKeys struct {
	// BootstrapKeys describes the set of keys supported by --config as
	// bootstrap only config keys.
	BootstrapKeys map[string]common.PrintConfigSchema

	// ControllerKeys describes the set of keys supported by --config as
	// configuration items that will be applied to the controller during
	// bootstrap.
	ControllerKeys map[string]common.PrintConfigSchema

	// ModelKeys describes the set of keys supported by --config as
	// configuration items that will be applied to the controllers model during
	// bootstrap.
	ModelKeys map[string]common.PrintConfigSchema
}

// Format is responsible for returning a formatted categorised string of all
// the --config keys supported by bootstrap. This is used directly when
// generating help docs.
func (c ConfigCategoryKeys) Format() string {
	builder := strings.Builder{}

	if c.BootstrapKeys != nil {
		fmt.Fprint(&builder, "Bootstrap configuration keys:\n\n")
		output, _ := common.FormatConfigSchema(c.BootstrapKeys)
		fmt.Fprintln(&builder, output)
	}

	if c.ControllerKeys != nil {
		fmt.Fprint(&builder, "Controller configuration keys:\n\n")
		output, _ := common.FormatConfigSchema(c.ControllerKeys)
		fmt.Fprint(&builder, output)
	}

	if c.ModelKeys != nil {
		fmt.Fprint(&builder, "Model configuration keys (affecting the controller model):\n\n")
		output, _ := common.FormatConfigSchema(c.ModelKeys)
		fmt.Fprint(&builder, output)
	}

	return builder.String()
}

func (c *bootstrapCommand) configDetails() ConfigCategoryKeys {
	categoryKeys := ConfigCategoryKeys{}

	if modelCgf, err := cmdmodel.ConfigDetails(); err == nil {
		categoryKeys.ModelKeys = modelCgf
	}
	if controllerCgf, err := cmdcontroller.ConfigDetailsAll(); err == nil {
		categoryKeys.ControllerKeys = controllerCgf
	}

	categoryKeys.BootstrapKeys = make(map[string]common.PrintConfigSchema, len(bootstrap.BootstrapConfigSchema))
	for key, attr := range bootstrap.BootstrapConfigSchema {
		categoryKeys.BootstrapKeys[key] = common.PrintConfigSchema{
			Description: attr.Description,
			Type:        string(attr.Type),
		}
	}
	return categoryKeys
}

func (c *bootstrapCommand) setControllerName(controllerName string) {
	c.controllerName = strings.ToLower(controllerName)
}

func (c *bootstrapCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.Var(&c.ConstraintsStr, "constraints", "Set model constraints")
	f.Var(&c.BootstrapConstraintsStr, "bootstrap-constraints", "Specify bootstrap machine constraints")
	f.StringVar(&c.BootstrapSeries, "bootstrap-series", "", "Specify the series of the bootstrap machine (deprecated; use `bootstrap-base`)")
	f.StringVar(&c.BootstrapBase, "bootstrap-base", "", "Specify the base of the bootstrap machine")
	f.StringVar(&c.BootstrapImage, "bootstrap-image", "", "Specify the image of the bootstrap machine (requires `--bootstrap-constraints` specifying architecture)")
	f.BoolVar(&c.BuildAgent, "build-agent", false, "Build local version of agent binary before bootstrapping")
	f.StringVar(&c.JujuDbSnapPath, "db-snap", "",
		"Path to a locally built `.snap` to use as the internal `juju-db` service.")
	f.StringVar(&c.JujuDbSnapAssertionsPath, "db-snap-asserts", "", "Path to a local `.assert` file. Requires `--db-snap`")
	f.StringVar(&c.MetadataSource, "metadata-source", "", "Local path to use as agent and/or image metadata source")
	f.StringVar(&c.Placement, "to", "", "Placement directive indicating an instance to bootstrap")
	f.BoolVar(&c.KeepBrokenEnvironment, "keep-broken", false,
		"Do not destroy the provisioned controller instance if bootstrap fails")
	f.BoolVar(&c.AutoUpgrade, "auto-upgrade", false, "After bootstrap, upgrade to the latest patch release")
	f.StringVar(&c.AgentVersionParam, "agent-version", "", "Version of agent binaries to use for Juju agents")
	f.StringVar(&c.CredentialName, "credential", "", "Credentials to use when bootstrapping")
	f.Var(&c.config, "config",
		"Specify a controller configuration file, or one or more configuration options. Model config keys only affect the controller model.\n    (`--config config.yaml [--config key=value ...])`")
	f.Var(&c.modelDefaults, "model-default",
		"Specify a configuration file, or one or more configuration\n    options to be set for all models, unless otherwise specified\n    (`--model-default config.yaml [--model-default key=value ...])`")
	f.Var(&c.storagePool, "storage-pool",
		"Specify options for an initial storage pool\n    'name' and 'type' are required, plus any additional attributes\n    (`--storage-pool pool-config.yaml [--storage-pool key=value ...]`)")
	f.StringVar(&c.initialModelName, "add-model", "", "Name of an initial model to create on the new controller")
	f.BoolVar(&c.showClouds, "clouds", false,
		"Print the available clouds which can be used to bootstrap a Juju environment")
	f.StringVar(&c.showRegionsForCloud, "regions", "", "Print the available regions for the specified cloud")
	f.BoolVar(&c.noSwitch, "no-switch", false, "Do not switch to the newly created controller")
	f.BoolVar(&c.Force, "force", false, "Allow the bypassing of checks such as supported series")
	f.StringVar(&c.ControllerCharmPath, "controller-charm-path", "", "Path to a locally built controller charm")
	f.StringVar(&c.ControllerCharmChannelStr, "controller-charm-channel",
		fmt.Sprintf("%d.%d/stable", jujuversion.Current.Major, jujuversion.Current.Minor),
		"The Charmhub channel to download the controller charm from (if not using a local charm)")
}

func (c *bootstrapCommand) Init(args []string) (err error) {
	if c.JujuDbSnapPath != "" {
		_, err := c.Filesystem().Stat(c.JujuDbSnapPath)
		if err != nil {
			return errors.Annotatef(err, "problem with --db-snap")
		}
	}

	if c.BootstrapSeries != "" && c.BootstrapBase != "" {
		return errors.New("cannot specify both --bootstrap-series and --bootstrap-base")
	}

	if c.BootstrapSeries != "" {
		base, err := corebase.GetBaseFromSeries(c.BootstrapSeries)
		if err != nil {
			return errors.Errorf("cannot determine base for series %q", c.BootstrapSeries)
		}

		c.BootstrapBase = base.String()
		c.BootstrapSeries = ""
	}
	// Validate the bootstrap base looks like a base.
	if c.BootstrapBase != "" {
		if _, err := corebase.ParseBaseFromString(c.BootstrapBase); err != nil {
			return errors.NotValidf("base %q", c.BootstrapBase)
		}
	}

	// fill in JujuDbSnapAssertionsPath from the same directory as JujuDbSnapPath
	if c.JujuDbSnapAssertionsPath == "" && c.JujuDbSnapPath != "" {
		assertionsPath := strings.Replace(c.JujuDbSnapPath, path.Ext(c.JujuDbSnapPath), ".assert", -1)
		logger.Debugf("--db-snap-asserts unset, assuming %v", assertionsPath)
		c.JujuDbSnapAssertionsPath = assertionsPath
	}

	if c.JujuDbSnapAssertionsPath != "" {
		_, err := c.Filesystem().Stat(c.JujuDbSnapAssertionsPath)
		if err != nil {
			return errors.Annotatef(err, "problem with --db-snap-asserts")
		}
	}

	if c.JujuDbSnapAssertionsPath != "" && c.JujuDbSnapPath == "" {
		return errors.New("--db-snap-asserts requires --db-snap")
	}

	if c.ControllerCharmPath != "" {
		if refresher.IsLocalURL(c.ControllerCharmPath) {
			_, err := c.Filesystem().Stat(c.ControllerCharmPath)
			if err != nil {
				return errors.Annotatef(err, "problem with --controller-charm-path")
			}
			ch, err := charm.ReadCharm(c.ControllerCharmPath)
			if err != nil {
				return errors.Annotatef(err, "--controller-charm-path %q is not a valid charm", c.ControllerCharmPath)
			}
			if ch.Meta().Name != bootstrap.ControllerCharmName {
				return errors.Errorf("--controller-charm-path %q is not a %q charm", c.ControllerCharmPath,
					bootstrap.ControllerCharmName)
			}
		}
		// Assume this is a Charmhub URL
		// TODO(barrettj12): validate the charm exists on CharmHub
	}

	c.ControllerCharmChannel, err = parseControllerCharmChannel(c.ControllerCharmChannelStr)
	if err != nil {
		return errors.NotValidf("controller charm channel %q", c.ControllerCharmChannelStr)
	}

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
	if c.initialModelName == "" {
		c.initialModelName = os.Getenv("JUJU_BOOTSTRAP_MODEL")
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
		return errors.Errorf("this client can only bootstrap %v.%v agents", jujuversion.Current.Major,
			jujuversion.Current.Minor)
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
	if ok := names.IsValidCloud(c.Cloud); !ok {
		return errors.NotValidf("cloud name %q", c.Cloud)
	}
	if len(args) > 1 {
		c.setControllerName(args[1])
		return cmd.CheckEmpty(args[2:])
	}
	return nil
}

func parseControllerCharmChannel(channelStr string) (charm.Channel, error) {
	ch, err := charm.ParseChannel(channelStr)
	if err != nil {
		return charm.Channel{}, err
	}

	if ch.Track == "" {
		ch.Track = fmt.Sprintf("%d.%d", jujuversion.Current.Major, jujuversion.Current.Minor)
	}
	if ch.Risk == "" {
		ch.Risk = charm.Stable
	}
	return ch, nil
}

// BootstrapInterface provides bootstrap functionality that Run calls to support cleaner testing.
type BootstrapInterface interface {
	// Bootstrap bootstraps a controller.
	Bootstrap(ctx environs.BootstrapContext, environ environs.BootstrapEnviron, callCtx envcontext.ProviderCallContext,
		args bootstrap.BootstrapParams) error

	// CloudDetector returns a CloudDetector for the given provider,
	// if the provider supports it.
	CloudDetector(environs.EnvironProvider) (environs.CloudDetector, bool)

	// CloudRegionDetector returns a CloudRegionDetector for the given provider,
	// if the provider supports it.
	CloudRegionDetector(environs.EnvironProvider) (environs.CloudRegionDetector, bool)

	// CloudFinalizer returns a CloudFinalizer for the given provider,
	// if the provider supports it.
	CloudFinalizer(environs.EnvironProvider) (environs.CloudFinalizer, bool)
}

type bootstrapFuncs struct{}

func (b bootstrapFuncs) Bootstrap(ctx environs.BootstrapContext, env environs.BootstrapEnviron,
	callCtx envcontext.ProviderCallContext, args bootstrap.BootstrapParams) error {
	return bootstrap.Bootstrap(ctx, env, callCtx, args)
}

func (b bootstrapFuncs) CloudDetector(provider environs.EnvironProvider) (environs.CloudDetector, bool) {
	detector, ok := provider.(environs.CloudDetector)
	return detector, ok
}

func (b bootstrapFuncs) CloudRegionDetector(provider environs.EnvironProvider) (environs.CloudRegionDetector, bool) {
	detector, ok := provider.(environs.CloudRegionDetector)
	return detector, ok
}

func (b bootstrapFuncs) CloudFinalizer(provider environs.EnvironProvider) (environs.CloudFinalizer, bool) {
	finalizer, ok := provider.(environs.CloudFinalizer)
	return finalizer, ok
}

var getBootstrapFuncs = func() BootstrapInterface {
	return &bootstrapFuncs{}
}

var supportedJujuBases = corebase.ControllerBases

var (
	bootstrapPrepareController = bootstrap.PrepareController
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
	if c.ConstraintsStr.String() != "" {
		cons, aliases, err := constraints.ParseWithAliases(strings.Join(c.ConstraintsStr, " "))
		for k, v := range aliases {
			allAliases[k] = v
		}
		if err != nil {
			return err
		}
		c.Constraints = cons
	}
	if c.BootstrapConstraintsStr.String() != "" {
		cons, aliases, err := constraints.ParseWithAliases(strings.Join(c.BootstrapConstraintsStr, " "))
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

func (c *bootstrapCommand) initializeFirstModel(
	isCAASController bool,
	config bootstrapConfigs,
	store jujuclient.ClientStore,
	environ environs.BootstrapEnviron,
	bootstrapParams *bootstrap.BootstrapParams,
) (*jujuclient.ModelDetails, error) {
	if c.initialModelName == "" || c.initialModelName == "controller" {
		// Nothing to do, but ensure the required model is selected by default.
		return nil, store.SetCurrentModel(c.controllerName, c.initialModelName)
	}

	initialModelUUID, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}

	initialModelType := model.IAAS
	if isCAASController {
		initialModelType = model.CAAS
	}
	modelDetails := &jujuclient.ModelDetails{
		ModelUUID: initialModelUUID.String(),
		ModelType: initialModelType,
	}

	if featureflag.Enabled(feature.Branches) || featureflag.Enabled(feature.Generations) {
		modelDetails.ActiveBranch = model.GenerationMaster
	}

	if err := store.UpdateModel(
		c.controllerName,
		c.initialModelName,
		*modelDetails,
	); err != nil {
		return nil, errors.Trace(err)
	}

	bootstrapParams.InitialModelConfig = c.InitialModelConfig(
		initialModelUUID, config.inheritedControllerAttrs, config.userConfigAttrs, environ,
	)

	if !c.noSwitch {
		// Set the current model to the initial hosted model.
		if err := store.SetCurrentModel(c.controllerName, c.initialModelName); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return modelDetails, nil
}

// Run connects to the environment specified on the command line and bootstraps
// a juju in that environment if none already exists. If there is as yet no environments.yaml file,
// the user is informed how to create one.
func (c *bootstrapCommand) Run(ctx *cmd.Context) (resultErr error) {
	var (
		initialModel     *jujuclient.ModelDetails
		isCAASController bool

		// The stdCtx is used as a placeholder for the defer below. This will
		// be replaced with a context that can be cancelled later on.
		stdCtx = context.TODO()
	)

	defer func() {
		// If the context is an error state, then don't continue on processing
		// the bootstrap command.
		if stdCtx.Err() != nil {
			return
		}

		resultErr = handleChooseCloudRegionError(ctx, resultErr)
		if !c.showClouds && resultErr == nil {
			if initialModel != nil {
				ctx.Infof("Initial model %q added", c.initialModelName)
				return
			}

			workloadType := ""
			if isCAASController {
				workloadType = "k8s "
			}

			ctx.Infof(`
Now you can run
	juju add-model <model-name>
to create a new model to deploy %sworkloads.
`, workloadType)
		}
	}()

	// TODO(stickupkid): Once default-series has been deprecated, it's safe to
	// remove this.
	if err := c.warnDeprecatedModelConfig(ctx); err != nil {
		return err
	}

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

	cloud, provider, err := c.cloud(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	// If region is specified by the user, validate it here.
	// lp#1632735
	if c.Region != "" {
		_, err := jujucloud.RegionByName(cloud.Regions, c.Region)
		if err != nil {
			allRegions := make([]string, len(cloud.Regions))
			for i, one := range cloud.Regions {
				allRegions[i] = one.Name
			}
			if len(allRegions) > 0 {
				naturalsort.Sort(allRegions)
				plural := "s are"
				if len(allRegions) == 1 {
					plural = " is"
				}
				ctx.Infof("Available cloud region%v %v", plural, strings.Join(allRegions, ", "))
			}
			return errors.NotValidf("region %q for cloud %q", c.Region, c.Cloud)
		}
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
		if errors.IsNotFound(err) {
			err = errors.NewNotFound(nil,
				fmt.Sprintf("%v\nSee `juju add-credential %s --help` for instructions", err, cloud.Name))
		}

		if err == cmd.ErrSilent {
			return err
		}
		return errors.Trace(err)
	}

	region, err := common.ChooseCloudRegion(cloud, regionName)
	if err != nil {
		return errors.Trace(err)
	}

	if c.controllerName == "" {
		c.setControllerName(defaultControllerName(cloud.Name, region.Name))
	}

	// set a Region so it's config can be found below.
	if c.Region == "" {
		c.Region = region.Name
	}

	bootstrapCfg, err := c.bootstrapConfigs(ctx, cloud, provider)
	if err != nil {
		return errors.Trace(err)
	}

	isCAASController = jujucloud.CloudIsCAAS(cloud)
	if isCAASController && refresher.IsLocalURL(c.ControllerCharmPath) {
		return errors.NotSupportedf("deploying a local controller charm on a k8s controller")
	}
	if !isCAASController {
		if bootstrapCfg.bootstrap.ControllerServiceType != "" ||
			bootstrapCfg.bootstrap.ControllerExternalName != "" ||
			len(bootstrapCfg.bootstrap.ControllerExternalIPs) > 0 {
			return errors.Errorf("%q, %q and %q\nare only allowed for kubernetes controllers",
				bootstrap.ControllerServiceType, bootstrap.ControllerExternalName, bootstrap.ControllerExternalIPs)
		}
	}

	if bootstrapCfg.controller.ControllerName() != "" {
		return errors.NewNotValid(nil, "controller name cannot be set via config, please use cmd args")
	}

	// Read existing current controller so we can clean up on error.
	var oldCurrentController string
	store := c.ClientStore()
	oldCurrentController, err = modelcmd.DetermineCurrentController(store)
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

	var bootstrapBase corebase.Base
	if c.BootstrapBase != "" {
		var err error
		bootstrapBase, err = corebase.ParseBaseFromString(c.BootstrapBase)
		if err != nil {
			return errors.NotValidf("bootstrap base %q", c.BootstrapBase)
		}
	}

	// Get the supported bootstrap series.
	var imageStream string
	if cfg, ok := bootstrapCfg.bootstrapModel["image-stream"]; ok {
		imageStream = cfg.(string)
	}
	now := c.clock.Now()
	supportedBootstrapBases, err := supportedJujuBases(now, bootstrapBase, imageStream)
	if err != nil {
		return errors.Annotate(err, "error reading supported bootstrap series")
	}
	logger.Tracef("supported bootstrap bases %v", supportedBootstrapBases)

	bootstrapCfg.controller[controller.ControllerName] = c.controllerName

	// Handle Ctrl-C during bootstrap by asking the bootstrap process to stop
	// early (and the above will then clean up resources).
	interrupted := make(chan os.Signal, 1)
	defer close(interrupted)

	ctx.InterruptNotify(interrupted)
	defer ctx.StopInterruptNotify(interrupted)

	var cancel context.CancelFunc
	stdCtx, cancel = context.WithTimeout(context.Background(), bootstrapCfg.bootstrap.BootstrapTimeout)

	go func() {
		for range interrupted {
			select {
			case <-stdCtx.Done():
				// Ctrl-C already pressed
				return

			default:
				// Newline prefix is intentional, so output appears as
				// "^C\nCtrl-C pressed" instead of "^CCtrl-C pressed".
				_, _ = fmt.Fprintln(ctx.GetStderr(), "\nCtrl-C pressed, attempting to stop bootstrap and clean up resources")
				cancel()
			}
		}
	}()

	bootstrapCtx := modelcmd.BootstrapContext(stdCtx, ctx)
	bootstrapPrepareParams := bootstrap.PrepareParams{
		ModelConfig:      bootstrapCfg.bootstrapModel,
		ControllerConfig: bootstrapCfg.controller,
		ControllerName:   c.controllerName,
		Cloud: environscloudspec.CloudSpec{
			Type:             cloud.Type,
			Name:             cloud.Name,
			Region:           region.Name,
			Endpoint:         region.Endpoint,
			IdentityEndpoint: region.IdentityEndpoint,
			StorageEndpoint:  region.StorageEndpoint,
			Credential:       credentials.credential,
			CACertificates:   cloud.CACertificates,
			SkipTLSVerify:    cloud.SkipTLSVerify,
		},
		CredentialName: credentials.name,
		AdminSecret:    bootstrapCfg.bootstrap.AdminSecret,
	}
	environ, err := bootstrapPrepareController(
		isCAASController, bootstrapCtx, store, bootstrapPrepareParams,
	)
	if err != nil {
		return errors.Trace(err)
	}

	// Validate the storage provider config.
	registry := stateenvirons.NewStorageProviderRegistry(environ)
	m := poolmanager.MemSettings{
		Settings: make(map[string]map[string]interface{}),
	}
	pm := poolmanager.New(m, registry)
	for poolName, cfg := range bootstrapCfg.storagePools {
		poolType, _ := cfg[poolmanager.Type].(string)
		_, err = pm.Create(poolName, storage.ProviderType(poolType), cfg)
		if err != nil {
			return errors.NewNotValid(err, "invalid storage provider config")
		}
	}

	bootstrapParams := bootstrap.BootstrapParams{
		ControllerName:            c.controllerName,
		BootstrapBase:             bootstrapBase,
		SupportedBootstrapBases:   supportedBootstrapBases,
		BootstrapImage:            c.BootstrapImage,
		Placement:                 c.Placement,
		BuildAgent:                c.BuildAgent,
		BuildAgentTarball:         sync.BuildAgentTarball,
		AgentVersion:              c.AgentVersion,
		Cloud:                     cloud,
		CloudRegion:               region.Name,
		ControllerConfig:          bootstrapCfg.controller,
		ControllerInheritedConfig: bootstrapCfg.inheritedControllerAttrs,
		RegionInheritedConfig:     cloud.RegionConfig,
		AdminSecret:               bootstrapCfg.bootstrap.AdminSecret,
		CAPrivateKey:              bootstrapCfg.bootstrap.CAPrivateKey,
		SSHServerHostKey:          bootstrapCfg.bootstrap.SSHServerHostKey,
		ControllerServiceType:     bootstrapCfg.bootstrap.ControllerServiceType,
		ControllerExternalName:    bootstrapCfg.bootstrap.ControllerExternalName,
		ControllerExternalIPs:     append([]string(nil), bootstrapCfg.bootstrap.ControllerExternalIPs...),
		JujuDbSnapPath:            c.JujuDbSnapPath,
		JujuDbSnapAssertionsPath:  c.JujuDbSnapAssertionsPath,
		StoragePools:              bootstrapCfg.storagePools,
		ControllerCharmPath:       c.ControllerCharmPath,
		ControllerCharmChannel:    c.ControllerCharmChannel,
		DialOpts: environs.BootstrapDialOpts{
			Timeout:        bootstrapCfg.bootstrap.BootstrapTimeout,
			RetryDelay:     bootstrapCfg.bootstrap.BootstrapRetryDelay,
			AddressesDelay: bootstrapCfg.bootstrap.BootstrapAddressesDelay,
		},
		Force: c.Force,
	}

	initialModel, err = c.initializeFirstModel(
		isCAASController, bootstrapCfg, store, environ, &bootstrapParams,
	)
	if err != nil {
		return errors.Trace(err)
	}

	if !c.noSwitch {
		// set the current controller.
		if err := store.SetCurrentController(c.controllerName); err != nil {
			return errors.Trace(err)
		}
	}

	cloudRegion := c.Cloud
	if region.Name != "" {
		cloudRegion = fmt.Sprintf("%s/%s", cloudRegion, region.Name)
	}
	ctx.Infof(
		"Creating Juju controller %q on %s",
		c.controllerName, cloudRegion,
	)

	cloudCallCtx := envcontext.NewCloudCallContext(stdCtx)
	// At this stage, the credential we intend to use is not yet stored
	// server-side. So, if the credential is not accepted by the provider,
	// we cannot mark it as invalid, just log it as an informative message.
	cloudCallCtx.InvalidateCredentialFunc = func(reason string) error {
		ctx.Infof("Cloud credential %q is not accepted by cloud provider: %v", credentials.name, reason)
		return nil
	}

	// If we error out for any reason, clean up the environment.
	defer func() {
		if resultErr != nil {
			if c.KeepBrokenEnvironment {
				ctx.Infof(`
bootstrap failed but --keep-broken was specified.
This means that cloud resources are left behind, but not registered to
your local client, as the controller was not successfully created.
However, you should be able to ssh into the machine using the user "ubuntu" and
their IP address for diagnosis and investigation.
When you are ready to clean up the failed controller, use your cloud console or
equivalent CLI tools to terminate the instances and remove remaining resources.

See %s.`[1:], "`juju kill-controller`")
			} else {
				logger.Errorf("%v", resultErr)
				logger.Debugf("(error details: %v)", errors.Details(resultErr))
				// Set resultErr to cmd.ErrSilent to prevent
				// logging the error twice.
				resultErr = cmd.ErrSilent
				handleBootstrapError(ctx, func() error {
					return environsDestroy(
						c.controllerName, environ, cloudCallCtx, store,
					)
				})
			}
		}
	}()

	if envMetadataSrc := os.Getenv(constants.EnvJujuMetadataSource); c.MetadataSource == "" && envMetadataSrc != "" {
		c.MetadataSource = envMetadataSrc
		ctx.Infof("Using metadata source directory %q", c.MetadataSource)
	}

	// If --metadata-source is specified, override the default tools metadata source so
	// SyncTools can use it, and also upload any image metadata.
	if c.MetadataSource != "" {
		bootstrapParams.MetadataDir = ctx.AbsPath(c.MetadataSource)
	}

	constraintsValidator, err := environ.ConstraintsValidator(cloudCallCtx)
	if err != nil {
		return errors.Trace(err)
	}

	// Merge in any space constraints that should be implied from controller
	// space config.
	// Do it before calling merge, because the constraints will be validated
	// there.
	bootstrapConstraints := c.Constraints
	bootstrapConstraints.Spaces = bootstrapCfg.controller.AsSpaceConstraints(bootstrapConstraints.Spaces)

	// Merge environ and bootstrap-specific constraints.
	bootstrapParams.BootstrapConstraints, err = constraintsValidator.Merge(bootstrapConstraints, c.BootstrapConstraints)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Infof("combined bootstrap constraints: %v", bootstrapParams.BootstrapConstraints)
	unsupported, err := constraintsValidator.Validate(bootstrapParams.BootstrapConstraints)
	if err != nil {
		return errors.Trace(err)
	}
	if len(unsupported) > 0 {
		logger.Warningf(
			"unsupported constraints: %v", strings.Join(unsupported, ","))
	}

	bootstrapParams.ModelConstraints = c.Constraints

	if credentials.name == "" {
		// credentialName will be empty if the credential was detected.
		// We must supply a name for the credential in the database,
		// so choose one.
		credentials.name = credentials.detectedName
	}
	bootstrapParams.CloudCredential = credentials.credential
	bootstrapParams.CloudCredentialName = credentials.name

	// See if there's any additional agent environment options required.
	// eg JUJU_AGENT_TESTING_OPTIONS=foo=bar,timeout=2s
	// These are written to the agent.conf VALUES section.
	testingOptionsStr := os.Getenv("JUJU_AGENT_TESTING_OPTIONS")
	if len(testingOptionsStr) > 0 {
		opts, err := keyvalues.Parse(
			strings.Split(
				strings.ReplaceAll(testingOptionsStr, " ", ""), ","), false)
		if err != nil {
			return errors.Annotatef(err, "invalid JUJU_AGENT_TESTING_OPTIONS env value %q", testingOptionsStr)
		}
		for k, v := range opts {
			if bootstrapParams.ExtraAgentValuesForTesting == nil {
				bootstrapParams.ExtraAgentValuesForTesting = map[string]string{}
			}
			bootstrapParams.ExtraAgentValuesForTesting[k] = v
		}
	}

	if cloud.Type == k8sconstants.CAASProviderType {
		if cloud.HostCloudRegion == k8s.K8sCloudOther {
			ctx.Infof("Bootstrap to generic Kubernetes cluster")
		} else {
			ctx.Infof("Bootstrap to Kubernetes cluster identified as %s",
				cloud.HostCloudRegion)
		}

	}

	bootstrapFuncs := getBootstrapFuncs()
	if err = bootstrapFuncs.Bootstrap(
		bootstrapCtx,
		environ,
		cloudCallCtx,
		bootstrapParams,
	); err != nil {
		return errors.Annotate(err, "failed to bootstrap model")
	}

	if err = c.controllerDataRefresher(environ, cloudCallCtx, bootstrapCfg); err != nil {
		return errors.Trace(err)
	}

	modelNameToSet := bootstrap.ControllerModelName
	if initialModel != nil {
		modelNameToSet = c.initialModelName
	}
	if err = c.SetModelIdentifier(modelcmd.JoinModelName(c.controllerName, modelNameToSet), false); err != nil {
		return errors.Trace(err)
	}

	// To avoid race conditions when running scripted bootstraps, wait
	// for the controller's machine agent to be ready to accept commands
	// before exiting this bootstrap command.
	return waitForAgentInitialisation(
		bootstrapCtx,
		&c.ModelCommandBase,
		isCAASController,
		c.controllerName,
	)
}

func (c *bootstrapCommand) controllerDataRefresher(
	environ environs.BootstrapEnviron,
	cloudCallCtx *envcontext.CloudCallContext,
	bootstrapCfg bootstrapConfigs,
) error {
	agentVersion := jujuversion.Current
	if c.AgentVersion != nil {
		agentVersion = *c.AgentVersion
	}

	// This logic allows polling for address info later during retries,
	// for example, when a load balancer needs time to be provisioned.
	var addrs []network.ProviderAddress
	var err error
	if env, ok := environ.(environs.InstanceBroker); ok {
		// IAAS.
		addrs, err = common.BootstrapEndpointAddresses(env, cloudCallCtx)
		if err != nil {
			return errors.Trace(err)
		}
	} else if env, ok := environ.(caas.ServiceManager); ok {
		// CAAS.
		var svc *caas.Service
		svc, err = env.GetService(k8sconstants.JujuControllerStackName, caas.ModeWorkload, false)
		if err != nil {
			return errors.Trace(err)
		}
		if len(svc.Addresses) == 0 {
			return errors.NotProvisionedf("k8s controller service %q address", svc.Id)
		}
		addrs = svc.Addresses
	} else {
		// This should never happen.
		return errors.New(
			"supplied BootstrapEnviron implements neither environs.InstanceBroker nor caas.ServiceGetterSetter")
	}

	var proxier proxy.Proxier
	if conInfo, ok := environ.(environs.ConnectorInfo); ok {
		proxier, err = conInfo.ConnectionProxyInfo()
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
	}

	// Use the retrieved bootstrap machine/service addresses to create
	// host/port endpoints for local storage.
	hps := make([]network.MachineHostPort, len(addrs))
	for i, addr := range addrs {
		hps[i] = network.MachineHostPort{
			MachineAddress: addr.MachineAddress,
			NetPort:        network.NetPort(bootstrapCfg.controller.APIPort()),
		}
	}
	return errors.Annotate(
		juju.UpdateControllerDetailsFromLogin(
			c.ClientStore(),
			c.controllerName,
			juju.UpdateControllerParams{
				AgentVersion:           agentVersion.String(),
				CurrentHostPorts:       []network.MachineHostPorts{hps},
				PublicDNSName:          newStringIfNonEmpty(bootstrapCfg.controller.AutocertDNSName()),
				MachineCount:           newInt(1),
				Proxier:                proxier,
				ControllerMachineCount: newInt(1),
			},
		),
		"saving bootstrap endpoint address",
	)
}

func (c *bootstrapCommand) handleCommandLineErrorsAndInfoRequests(ctx *cmd.Context) (bool, error) {
	if c.BootstrapImage != "" {
		if c.BootstrapBase == "" {
			return true, errors.Errorf("--bootstrap-image must be used with --bootstrap-base")
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

func (c *bootstrapCommand) cloud(ctx *cmd.Context) (jujucloud.Cloud, environs.EnvironProvider, error) {
	bootstrapFuncs := getBootstrapFuncs()
	fail := func(err error) (jujucloud.Cloud, environs.EnvironProvider, error) {
		return jujucloud.Cloud{}, nil, err
	}

	// Get the cloud definition identified by c.Cloud. If c.Cloud does not
	// identify a cloud in clouds.yaml, then we check if any of the
	// providers can detect a cloud with the given name. Otherwise, if the
	// cloud name identifies a provider *type* (e.g. "openstack"), then we
	// check if that provider can detect cloud regions, and synthesise a
	// cloud with those regions.
	var provider environs.EnvironProvider
	var cloud jujucloud.Cloud
	cloudptr, err := jujucloud.CloudByName(c.Cloud)
	if errors.IsNotFound(err) {
		cloud, provider, err = c.detectCloud(ctx, bootstrapFuncs)
		if err != nil {
			return fail(errors.Trace(err))
		}
	} else if err != nil {
		return fail(errors.Trace(err))
	} else {
		cloud = *cloudptr
		if err := checkProviderType(cloud.Type); err != nil {
			return fail(errors.Trace(err))
		}
		provider, err = environs.Provider(cloud.Type)
		if err != nil {
			return fail(errors.Trace(err))
		}
	}

	if finalizer, ok := bootstrapFuncs.CloudFinalizer(provider); ok {
		cloud, err = finalizer.FinalizeCloud(ctx, cloud)
		if err != nil {
			return fail(errors.Trace(err))
		}
	}

	if err = c.validateRegion(ctx, &cloud); err != nil {
		return fail(errors.Trace(err))
	}

	// Custom clouds may not have explicitly declared support for any auth-
	// types, in which case we'll assume that they support everything that
	// the provider supports.
	if len(cloud.AuthTypes) == 0 {
		for authType := range provider.CredentialSchemas() {
			cloud.AuthTypes = append(cloud.AuthTypes, authType)
		}
	}

	return cloud, provider, nil
}

func (c *bootstrapCommand) detectCloud(
	ctx *cmd.Context,
	bootstrapFuncs BootstrapInterface,
) (jujucloud.Cloud, environs.EnvironProvider, error) {
	fail := func(err error) (jujucloud.Cloud, environs.EnvironProvider, error) {
		return jujucloud.Cloud{}, nil, err
	}

	// Check if any of the registered providers can give us a cloud with
	// the specified name. The first one wins.
	for _, providerType := range environs.RegisteredProviders() {
		provider, err := environs.Provider(providerType)
		if err != nil {
			return fail(errors.Trace(err))
		}
		cloudDetector, ok := bootstrapFuncs.CloudDetector(provider)
		if !ok {
			continue
		}
		cloud, err := cloudDetector.DetectCloud(c.Cloud)
		if errors.IsNotFound(err) {
			continue
		} else if err != nil {
			return fail(errors.Trace(err))
		}
		return cloud, provider, nil
	}

	ctx.Verbosef("cloud %q not found, trying as a provider name", c.Cloud)
	provider, err := environs.Provider(c.Cloud)
	if errors.IsNotFound(err) {
		return fail(errors.NewNotFound(nil,
			fmt.Sprintf("unknown cloud %q, please try %q", c.Cloud, "juju update-public-clouds")))
	} else if err != nil {
		return fail(errors.Trace(err))
	}
	regionDetector, ok := bootstrapFuncs.CloudRegionDetector(provider)
	if !ok {
		ctx.Verbosef(
			"provider %q does not support detecting regions",
			c.Cloud,
		)
		return fail(errors.NewNotFound(nil,
			fmt.Sprintf("unknown cloud %q, please try %q", c.Cloud, "juju update-public-clouds")))
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
		return fail(errors.Annotatef(err,
			"detecting regions for %q cloud provider",
			c.Cloud,
		))
	}
	schemas := provider.CredentialSchemas()
	authTypes := make([]jujucloud.AuthType, 0, len(schemas))
	for authType := range schemas {
		authTypes = append(authTypes, authType)
	}

	// Since we are iterating over a map, lets sort the authTypes so
	// they are always in a consistent order.
	sort.Sort(jujucloud.AuthTypes(authTypes))
	return jujucloud.Cloud{
		Name:      c.Cloud,
		Type:      c.Cloud,
		AuthTypes: authTypes,
		Endpoint:  cloudEndpoint,
		Regions:   regions,
	}, provider, nil
}

func (c *bootstrapCommand) validateRegion(ctx *cmd.Context, cloud *jujucloud.Cloud) error {
	if c.Region == "" {
		return nil
	}
	if _, err := jujucloud.RegionByName(cloud.Regions, c.Region); err == nil {
		return nil
	}
	allRegions := make([]string, len(cloud.Regions))
	for i, one := range cloud.Regions {
		allRegions[i] = one.Name
	}
	if len(allRegions) > 0 {
		naturalsort.Sort(allRegions)
		plural := "s are"
		if len(allRegions) == 1 {
			plural = " is"
		}
		ctx.Infof("Available cloud region%v %v", plural, strings.Join(allRegions, ", "))
	}
	return errors.NotValidf("region %q for cloud %q", c.Region, c.Cloud)
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
	cloud jujucloud.Cloud,
) (
	creds bootstrapCredentials,
	regionName string,
	err error,
) {
	store := c.ClientStore()

	// When looking for credentials, we should attempt to see if there are any
	// credentials that should be registered, before we get or detect them
	err = common.RegisterCredentials(ctx, store, provider, modelcmd.RegisterCredentialsParams{
		Cloud: cloud,
	})
	if err != nil {
		logger.Errorf("registering credentials errored %s", err)
	}

	var detected bool
	creds.credential, creds.name, regionName, detected, err = common.GetOrDetectCredential(
		ctx, store, provider, modelcmd.GetCredentialsParams{
			Cloud:          cloud,
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
	storagePools             map[string]storage.Attrs
}

func (c *bootstrapCommand) bootstrapConfigs(
	ctx *cmd.Context,
	cloud jujucloud.Cloud,
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

	if userConfigAttrs, err = ensureDefaultBase(userConfigAttrs); err != nil {
		return bootstrapConfigs{}, errors.Trace(err)
	}

	modelDefaultConfigAttrs, err := c.modelDefaults.ReadAttrs(ctx)
	if err != nil {
		return bootstrapConfigs{}, errors.Trace(err)
	}

	if modelDefaultConfigAttrs, err = ensureDefaultBase(modelDefaultConfigAttrs); err != nil {
		return bootstrapConfigs{}, errors.Trace(err)
	}

	// The provider may define some custom attributes specific
	// to the provider. These will be added to the model config.
	var providerAttrs map[string]interface{}
	if ps, ok := provider.(config.ConfigSchemaSource); ok {
		providerAttrs = make(map[string]interface{})
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
		coercedAttrs, err := fields.Coerce(providerAttrs, nil)
		if err != nil {
			return bootstrapConfigs{},
				errors.Annotatef(err, "invalid attribute value(s) for %v cloud", cloud.Type)
		}
		providerAttrs = coercedAttrs.(map[string]interface{})
	}

	storagePoolAttrs, err := c.storagePool.ReadAttrs(ctx)
	if err != nil {
		return bootstrapConfigs{}, errors.Trace(err)
	}
	var storagePools map[string]storage.Attrs
	if len(storagePoolAttrs) > 0 {
		poolName, _ := storagePoolAttrs[poolmanager.Name].(string)
		if poolName == "" {
			return bootstrapConfigs{}, errors.NewNotValid(nil, "storage pool requires a name")
		}
		poolType, _ := storagePoolAttrs[poolmanager.Type].(string)
		if poolType == "" {
			return bootstrapConfigs{}, errors.NewNotValid(nil, "storage pool requires a type")
		}
		storagePools = make(map[string]storage.Attrs)
		storagePools[poolName] = storagePoolAttrs
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
	// Region config values, for the region to be bootstrapped, from clouds.yaml
	// override what is in the cloud config.
	for k, v := range cloud.RegionConfig[c.Region] {
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

	// Store specific attributes are either already specified in model
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

	// Pre-process controller attributes.
	if _, ok := controllerConfigAttrs[controller.CAASOperatorImagePath]; ok {
		return bootstrapConfigs{}, fmt.Errorf("%q is no longer supported controller configuration",
			controller.CAASOperatorImagePath)
	}
	if v, ok := controllerConfigAttrs[controller.CAASImageRepo]; ok {
		if v, ok := v.(string); ok {
			repoDetails, err := docker.LoadImageRepoDetails(v)
			if err != nil {
				return bootstrapConfigs{}, errors.Annotatef(err, "processing %s", controller.CAASImageRepo)
			}
			controllerConfigAttrs[controller.CAASImageRepo] = repoDetails.Content()
		}
	}

	controllerConfig, err := controller.NewConfig(
		controllerUUID.String(),
		bootstrapConfig.CACert,
		controllerConfigAttrs,
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

	// We need to do an Azure specific check here to ensure that
	// if a resource-group-name is specified, the user has also
	// not specified a default model, otherwise we end up with 2
	// models with the same resource group name.
	resourceGroupName, ok := bootstrapModelConfig["resource-group-name"]
	if ok && resourceGroupName != "" && c.initialModelName != "" {
		return bootstrapConfigs{}, errors.Errorf("if using resource-group-name %q then a workload model cannot be specified as well",
			resourceGroupName)
	}

	logger.Debugf("preparing controller with config: %v", bootstrapModelConfig)

	configs := bootstrapConfigs{
		bootstrapModel:           bootstrapModelConfig,
		controller:               controllerConfig,
		bootstrap:                bootstrapConfig,
		inheritedControllerAttrs: inheritedControllerAttrs,
		userConfigAttrs:          userConfigAttrs,
		storagePools:             storagePools,
	}
	return configs, nil
}

// ensureDefaultBase ensures that the default base is set if the default series
// is supplied. It removes the default-series, if there was one.
func ensureDefaultBase(m map[string]interface{}) (map[string]interface{}, error) {
	// TODO(stickupkid): Remove this once series has been deleted and the
	// bases are the default.
	if key, ok := m[config.DefaultSeriesKey]; ok {
		if key == "" {
			m[config.DefaultBaseKey] = ""
		} else {
			s, err := corebase.GetBaseFromSeries(key.(string))
			if err != nil {
				return nil, errors.Trace(err)
			}
			m[config.DefaultBaseKey] = s.String()
		}
		delete(m, config.DefaultSeriesKey)
	}
	return m, nil
}

func (c *bootstrapCommand) InitialModelConfig(
	initialModelUUID utils.UUID,
	inheritedControllerAttrs,
	userConfigAttrs map[string]interface{},
	environ environs.ConfigGetter,
) map[string]interface{} {

	InitialModelConfig := map[string]interface{}{
		"name":         c.initialModelName,
		config.UUIDKey: initialModelUUID.String(),
	}
	for k, v := range inheritedControllerAttrs {
		InitialModelConfig[k] = v
	}

	// We copy across any user supplied attributes to the hosted model config.
	// But only if the attributes have not been removed from the controller
	// model config as part of preparing the controller model.
	controllerModelConfigAttrs := environ.Config().AllAttrs()
	for k, v := range userConfigAttrs {
		if _, ok := controllerModelConfigAttrs[k]; ok {
			InitialModelConfig[k] = v
		}
	}
	// Ensure that certain config attributes are not included in the hosted
	// model config. These attributes may be modified during bootstrap; by
	// removing them from this map, we ensure the modified values are
	// inherited.
	delete(InitialModelConfig, config.AuthorizedKeysKey)
	delete(InitialModelConfig, config.AgentVersionKey)

	return InitialModelConfig
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
	cloud, err := common.CloudByName(c.Cloud)
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

	name, err := queryName(defName, scanner, ctx.Stdout)
	c.setControllerName(name)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *bootstrapCommand) warnDeprecatedModelConfig(ctx *cmd.Context) error {
	var reported bool
	warn := func(attrs map[string]any) {
		if _, ok := attrs["default-series"]; !reported && ok {
			ctx.Warningf("default-series configuration option is deprecated in favour of default-base")
			reported = true
		}
	}

	type read func(*cmd.Context) (map[string]any, error)

	for _, read := range []read{
		c.config.ReadAttrs,
		c.modelDefaults.ReadAttrs,
	} {
		attrs, err := read(ctx)
		if err != nil {
			return err
		}

		warn(attrs)
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
func handleBootstrapError(ctx *cmd.Context, cleanup func() error) {
	ch := make(chan os.Signal, 1)
	ctx.InterruptNotify(ch)
	defer ctx.StopInterruptNotify(ch)
	defer close(ch)
	go func() {
		for range ch {
			// Newline prefix is intentional, so output appears as
			// "^C\nCtrl-C pressed" instead of "^CCtrl-C pressed".
			_, _ = fmt.Fprintln(ctx.GetStderr(), "\nCtrl-C pressed, cleaning up failed bootstrap")
		}
	}()
	logger.Debugf("cleaning up after failed bootstrap")
	if err := cleanup(); err != nil {
		logger.Errorf("error cleaning up: %v", err)
	}
}

func handleChooseCloudRegionError(ctx *cmd.Context, err error) error {
	if !common.IsChooseCloudRegionError(err) {
		return err
	}
	_, _ = fmt.Fprintf(ctx.GetStderr(),
		"%s\n\nSpecify an alternative region, or try %q.\n",
		err, "juju update-public-clouds",
	)
	return cmd.ErrSilent
}

func newInt(i int) *int {
	return &i
}

func newStringIfNonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
