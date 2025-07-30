// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	corecontext "github.com/juju/juju/core/context"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/sync"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/pki"
	corestorage "github.com/juju/juju/internal/storage"
	coretools "github.com/juju/juju/internal/tools"
)

const (
	// SimplestreamsFetcherContextKey defines a way to change the simplestreams
	// fetcher within a context.
	SimplestreamsFetcherContextKey corecontext.ContextKey = "simplestreams-fetcher"
)

const noToolsMessage = `Juju cannot bootstrap because no agent binaries are available for your model.
You may want to use the 'agent-metadata-url' configuration setting to specify the binaries' location.
`

var (
	logger = internallogger.GetLogger("juju.environs.bootstrap")

	errCancelled = errors.New("cancelled")
)

// BootstrapParams holds the parameters for bootstrapping an environment.
type BootstrapParams struct {
	// ModelConstraints are merged with the bootstrap constraints
	// to choose the initial instance, and will be stored in the
	// initial models' states.
	ModelConstraints constraints.Value

	// BootstrapConstraints are used to choose the initial instance.
	// BootstrapConstraints does not affect the model constraints.
	BootstrapConstraints constraints.Value

	// ControllerName is the controller name.
	ControllerName string

	// BootstrapImage, if specified, is the image ID to use for the
	// initial bootstrap machine.
	BootstrapImage string

	// Cloud contains the properties of the cloud that Juju will be
	// bootstrapped in.
	Cloud cloud.Cloud

	// CloudRegion is the name of the cloud region that Juju will be bootstrapped in.
	CloudRegion string

	// CloudCredentialName is the name of the cloud credential that Juju will be
	// bootstrapped with. This may be empty, for clouds that do not require
	// credentials.
	CloudCredentialName string

	// CloudCredential contains the cloud credential that Juju will be
	// bootstrapped with. This may be nil, for clouds that do not require
	// credentialis.
	CloudCredential *cloud.Credential

	// ControllerConfig is the set of config attributes relevant
	// to a controller.
	ControllerConfig controller.Config

	// ControllerInheritedConfig is the set of config attributes to be shared
	// across all models in the same controller.
	ControllerInheritedConfig map[string]interface{}

	// ControllerModelAuthorizedKeys is a set of pre-allowed authorized keys
	// for the initial controller model.
	ControllerModelAuthorizedKeys []string

	// RegionInheritedConfig holds region specific configuration attributes to
	// be shared across all models in the same controller on a particular
	// cloud.
	RegionInheritedConfig cloud.RegionConfig

	// Placement, if non-empty, holds an environment-specific placement
	// directive used to choose the initial instance.
	Placement string

	// BuildAgent reports whether we should build and upload the local agent
	// binary and override the environment's specified agent-version.
	// It is an error to specify BuildAgent with a nil BuildAgentTarball.
	BuildAgent bool

	// BuildAgentTarball, if non-nil, is a function that may be used to
	// build tools to upload. If this is nil, tools uploading will never
	// take place.
	BuildAgentTarball sync.BuildAgentTarballFunc

	// MetadataDir is an optional path to a local directory containing
	// tools and/or image metadata.
	MetadataDir string

	// AgentVersion, if set, determines the exact tools version that
	// will be used to start the Juju agents.
	AgentVersion *semversion.Number

	// AdminSecret contains the administrator password.
	AdminSecret string

	// SSHServerHostKey is the controller's SSH server host key.
	SSHServerHostKey string

	// CAPrivateKey is the controller's CA certificate private key.
	CAPrivateKey string

	// ControllerServiceType is the service type of a k8s controller.
	ControllerServiceType string

	// ControllerExternalName is the external name of a k8s controller.
	ControllerExternalName string

	// ControllerExternalIPs is the list of external ips for a k8s controller.
	ControllerExternalIPs []string

	// DialOpts contains the bootstrap dial options.
	DialOpts environs.BootstrapDialOpts

	// JujuDbSnapPath is the path to a local .snap file that will be used
	// to run the juju-db service.
	JujuDbSnapPath string

	// JujuDbSnapAssertionsPath is the path to a local .assertfile that
	// will be used to test the contents of the .snap at JujuDbSnap.
	JujuDbSnapAssertionsPath string

	// StoragePools is one or more named storage pools to create
	// in the controller model.
	StoragePools map[string]corestorage.Attrs

	// Force is used to allow a bootstrap to be run on unsupported series.
	Force bool

	// ControllerCharmPath is a local controller charm archive.
	ControllerCharmPath string

	// ControllerCharmChannel is used when fetching the charmhub controller charm.
	ControllerCharmChannel charm.Channel

	// ExtraAgentValuesForTesting are testing only values written to the agent config file.
	ExtraAgentValuesForTesting map[string]string

	// BootstrapBase, if specified, is the base to use for the
	// initial bootstrap machine (deprecated use BootstrapBase).
	BootstrapBase corebase.Base

	// SupportedBootstrapBase is a supported set of bases to use for
	// validating against the bootstrap base.
	SupportedBootstrapBases []corebase.Base
}

// Validate validates the bootstrap parameters.
func (p BootstrapParams) Validate() error {
	if p.AdminSecret == "" {
		return errors.New("admin-secret is empty")
	}
	if p.ControllerConfig.ControllerUUID() == "" {
		return errors.New("controller configuration has no controller UUID")
	}
	if _, hasCACert := p.ControllerConfig.CACert(); !hasCACert {
		return errors.New("controller configuration has no ca-cert")
	}
	if p.CAPrivateKey == "" {
		return errors.New("empty ca-private-key")
	}
	if len(p.SupportedBootstrapBases) == 0 {
		return errors.NotValidf("supported bootstrap bases")
	}

	// TODO(axw) validate other things.
	return nil
}

// withDefaultControllerConstraints returns the given constraints,
// updated to choose a default instance type appropriate for a
// controller machine. We use this only if the user does not specify
// any constraints that would otherwise control the instance type
// selection.
func withDefaultControllerConstraints(cons constraints.Value) constraints.Value {
	if !cons.HasInstanceType() && !cons.HasCpuCores() && !cons.HasCpuPower() && !cons.HasMem() {
		// A default of 3.5GiB will result in machines with up to 4GiB of memory, eg
		// - 3.75GiB on AWS, Google
		// - 3.5GiB on Azure
		var mem uint64 = 3.5 * 1024
		cons.Mem = &mem
	}
	// If we're bootstrapping a controller on a lxd virtual machine, we want to
	// ensure that it has at least 2 cores. Less than 2 cores can cause the
	// controller to become unresponsive when installing.
	if !cons.HasCpuCores() && cons.HasVirtType() && *cons.VirtType == "virtual-machine" {
		var cores = uint64(2)
		cons.CpuCores = &cores
	}
	return cons
}

// withDefaultCAASControllerConstraints returns the given constraints,
// updated to choose a default instance type appropriate for a
// controller machine. We use this only if the user does not specify
// any constraints that would otherwise control the instance type
// selection.
func withDefaultCAASControllerConstraints(cons constraints.Value) constraints.Value {
	if !cons.HasInstanceType() && !cons.HasCpuCores() && !cons.HasCpuPower() && !cons.HasMem() {
		// TODO(caas): Set memory constraints for mongod and controller containers independently.
		var mem uint64 = 1.5 * 1024
		cons.Mem = &mem
	}
	return cons
}

func bootstrapCAAS(
	ctx environs.BootstrapContext,
	environ environs.BootstrapEnviron,
	args BootstrapParams,
	bootstrapParams environs.BootstrapParams,
) error {
	if args.BuildAgent {
		return errors.NotSupportedf("--build-agent when bootstrapping a k8s controller")
	}
	if args.BootstrapImage != "" {
		return errors.NotSupportedf("--bootstrap-image when bootstrapping a k8s controller")
	}
	if !args.BootstrapBase.Empty() {
		return errors.NotSupportedf("--bootstrap-series or --bootstrap-base when bootstrapping a k8s controller")
	}

	constraintsValidator, err := environ.ConstraintsValidator(ctx)
	if err != nil {
		return err
	}
	bootstrapConstraints, err := constraintsValidator.Merge(
		args.ModelConstraints, args.BootstrapConstraints,
	)
	if err != nil {
		return errors.Trace(err)
	}
	bootstrapConstraints = withDefaultCAASControllerConstraints(bootstrapConstraints)
	bootstrapParams.BootstrapConstraints = bootstrapConstraints

	result, err := environ.Bootstrap(ctx, bootstrapParams)
	if err != nil {
		return errors.Trace(err)
	}

	podConfig, err := podcfg.NewBootstrapControllerPodConfig(
		args.ControllerConfig,
		args.ControllerName,
		result.Base.OS,
		bootstrapConstraints,
	)
	if err != nil {
		return errors.Trace(err)
	}

	jujuVersion := jujuversion.Current
	if args.AgentVersion != nil {
		jujuVersion = *args.AgentVersion
	}
	// set agent version before finalizing bootstrap config
	if err := setBootstrapAgentVersion(ctx, environ, jujuVersion); err != nil {
		return errors.Trace(err)
	}
	podConfig.JujuVersion = jujuVersion
	if err := finalizePodBootstrapConfig(ctx, podConfig, args, environ.Config()); err != nil {
		return errors.Annotate(err, "finalizing bootstrap instance config")
	}
	if err := result.CaasBootstrapFinalizer(ctx, podConfig, args.DialOpts); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func bootstrapIAAS(
	ctx environs.BootstrapContext,
	environ environs.BootstrapEnviron,
	args BootstrapParams,
	bootstrapParams environs.BootstrapParams,
) error {
	cfg := environ.Config()

	_, supportsNetworking := environs.SupportsNetworking(environ)
	logger.Debugf(ctx, "model %q supports application/machine networks: %v", cfg.Name(), supportsNetworking)
	disableNetworkManagement, _ := cfg.DisableNetworkManagement()
	logger.Debugf(ctx, "network management by juju enabled: %v", !disableNetworkManagement)

	var ss *simplestreams.Simplestreams
	if value := ctx.Value(SimplestreamsFetcherContextKey); value == nil {
		ss = simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	} else if s, ok := value.(*simplestreams.Simplestreams); ok {
		ss = s
	} else {
		return errors.Errorf("expected a valid simple streams type")
	}

	// Set default tools metadata source, add image metadata source,
	// then verify constraints. Providers may rely on image metadata
	// for constraint validation.
	var customImageMetadata []*imagemetadata.ImageMetadata
	if args.MetadataDir != "" {
		var err error
		customImageMetadata, err = setPrivateMetadataSources(ctx, ss, args.MetadataDir)
		if err != nil {
			return errors.Trace(err)
		}
	}

	// If the provider allows advance discovery of the series and hw
	// characteristics of the instance we are about to bootstrap, use this
	// information to backfill in any missing series and/or arch contstraints.
	if detector, supported := environ.(environs.HardwareCharacteristicsDetector); supported {
		detectedBase, err := detector.DetectBase()
		if err != nil {
			return errors.Trace(err)
		}
		detectedHW, err := detector.DetectHardware()
		if err != nil {
			return errors.Trace(err)
		}

		if args.BootstrapBase.Empty() && !detectedBase.Empty() {
			args.BootstrapBase = detectedBase
			logger.Debugf(ctx, "auto-selecting bootstrap series %q", args.BootstrapBase.String())
		}
		if args.BootstrapConstraints.Arch == nil &&
			args.ModelConstraints.Arch == nil &&
			detectedHW != nil &&
			detectedHW.Arch != nil {
			arch := *detectedHW.Arch
			args.BootstrapConstraints.Arch = &arch
			if detector.UpdateModelConstraints() {
				args.ModelConstraints.Arch = &arch
			}
			logger.Debugf(ctx, "auto-selecting bootstrap arch %q", arch)
		}
	}

	requestedBootstrapBase, err := corebase.ValidateBase(
		args.SupportedBootstrapBases,
		args.BootstrapBase,
		config.PreferredBase(cfg),
	)
	if !args.Force && err != nil {
		// If the base isn't valid (i.e. non-ubuntu) then don't prompt users to use
		// the --force flag.
		if requestedBootstrapBase.OS != corebase.UbuntuOS {
			return errors.NotValidf("non-ubuntu bootstrap base %q", requestedBootstrapBase.String())
		}
		return errors.Annotatef(err, "use --force to override")
	}
	bootstrapBase := requestedBootstrapBase

	var bootstrapArchForImageSearch string
	if args.BootstrapConstraints.Arch != nil {
		bootstrapArchForImageSearch = *args.BootstrapConstraints.Arch
	} else if args.ModelConstraints.Arch != nil {
		bootstrapArchForImageSearch = *args.ModelConstraints.Arch
	} else {
		bootstrapArchForImageSearch = arch.HostArch()
	}

	ctx.Verbosef("Loading image metadata")
	imageMetadata, err := bootstrapImageMetadata(ctx, environ,
		ss,
		&bootstrapBase,
		bootstrapArchForImageSearch,
		args.BootstrapImage,
		&customImageMetadata,
	)
	if err != nil {
		return errors.Trace(err)
	}

	// We want to determine a list of valid architectures for which to pick tools and images.
	// This includes architectures from custom and other available image metadata.
	architectures := set.NewStrings()
	if len(customImageMetadata) > 0 {
		for _, customMetadata := range customImageMetadata {
			architectures.Add(customMetadata.Arch)
		}
	}
	if len(imageMetadata) > 0 {
		for _, iMetadata := range imageMetadata {
			architectures.Add(iMetadata.Arch)
		}
	}
	bootstrapParams.ImageMetadata = imageMetadata

	constraintsValidator, err := environ.ConstraintsValidator(ctx)
	if err != nil {
		return err
	}
	constraintsValidator.UpdateVocabulary(constraints.Arch, architectures.SortedValues())

	bootstrapConstraints, err := constraintsValidator.Merge(
		args.ModelConstraints, args.BootstrapConstraints,
	)
	if err != nil {
		return errors.Trace(err)
	}
	// The follow is used to determine if we should apply the default
	// constraints when we bootstrap. Generally speaking this should always be
	// applied, but there are exceptions to the rule e.g. local LXD
	if checker, ok := environ.(environs.DefaultConstraintsChecker); !ok || checker.ShouldApplyControllerConstraints(bootstrapConstraints) {
		bootstrapConstraints = withDefaultControllerConstraints(bootstrapConstraints)
	}
	bootstrapParams.BootstrapConstraints = bootstrapConstraints

	var bootstrapArch string
	if bootstrapConstraints.Arch != nil {
		bootstrapArch = *bootstrapConstraints.Arch
	} else {
		// If no arch is specified as a constraint and we couldn't
		// auto-discover the arch from the provider, we'll fall back
		// to bootstrapping on the same arch as the CLI client.
		bootstrapArch = localToolsArch()
	}

	agentVersion := jujuversion.Current
	var availableTools coretools.List
	if !args.BuildAgent {
		latestPatchTxt := ""
		versionTxt := fmt.Sprintf("%v", args.AgentVersion)
		if args.AgentVersion == nil {
			latestPatchTxt = "latest patch of "
			versionTxt = fmt.Sprintf("%v.%v", agentVersion.Major, agentVersion.Minor)
		}
		ctx.Infof("Looking for %vpackaged Juju agent version %s for %s", latestPatchTxt, versionTxt, bootstrapArch)

		availableTools, err = findPackagedTools(ctx, environ, ss, args.AgentVersion, &bootstrapArch, &bootstrapBase)
		if err != nil && !errors.Is(err, errors.NotFound) {
			return err
		}
		if len(availableTools) != 0 {
			if args.AgentVersion == nil {
				// If agent version was not specified in the arguments,
				// we always want the latest/newest available.
				agentVersion, availableTools = availableTools.Newest()
			}
			for _, tool := range availableTools {
				ctx.Infof("Located Juju agent version %s at %s", tool.Version, tool.URL)
			}
		}
	}
	// If there are no prepackaged tools and a specific version has not been
	// requested, look for or build a local binary.
	var builtTools *sync.BuiltAgent
	if len(availableTools) == 0 && (args.AgentVersion == nil || isCompatibleVersion(*args.AgentVersion, jujuversion.Current)) {
		if args.BuildAgentTarball == nil {
			return errors.New("cannot build agent binary to upload")
		}
		if err = validateUploadAllowed(environ, &bootstrapArch, &bootstrapBase, constraintsValidator); err != nil {
			return err
		}
		if args.BuildAgent {
			ctx.Infof("Building local Juju agent binary version %s for %s", args.AgentVersion, bootstrapArch)
		} else {
			ctx.Infof("No packaged binary found, preparing local Juju agent binary")
		}
		var forceVersion semversion.Number
		availableTools, forceVersion, err = locallyBuildableTools()
		if err != nil {
			return errors.Annotate(err, "cannot package bootstrap agent binary")
		}
		builtTools, err = args.BuildAgentTarball(
			args.BuildAgent, "bootstrap",
			func(semversion.Number) semversion.Number { return forceVersion },
		)
		if err != nil {
			return errors.Annotate(err, "cannot package bootstrap agent binary")
		}
		defer os.RemoveAll(builtTools.Dir)
		// Combine the built agent information with the list of
		// available tools.
		for i, tool := range availableTools {
			if tool.URL != "" {
				continue
			}
			filename := filepath.Join(builtTools.Dir, builtTools.StorageName)
			tool.URL = fmt.Sprintf("file://%s", filename)
			tool.Size = builtTools.Size
			tool.SHA256 = builtTools.Sha256Hash

			// Use the version from the built tools but with the
			// corrected series and arch - this ensures the build
			// number is right if we found a valid official build.
			version := builtTools.Version
			version.Release = tool.Version.Release
			version.Arch = tool.Version.Arch
			// But if not an official build or is the edge snap, use the forced version.
			if !builtTools.Official || jujuversion.Grade == jujuversion.GradeDevel {
				version.Number = forceVersion
			}
			tool.Version = version
			availableTools[i] = tool
		}
	}
	if len(availableTools) == 0 {
		return errors.New(noToolsMessage)
	}
	bootstrapParams.AvailableTools = availableTools

	// TODO (anastasiamac 2018-02-02) By this stage, we will have a list
	// of available tools (agent binaries) but they should all be the same
	// version. Need to do check here, otherwise the provider.Bootstrap call
	// may fail. This also means that compatibility check, currently done
	// after provider.Bootstrap call in getBootstrapToolsVersion,
	// should be done here.

	// If we're uploading, we must override agent-version;
	// if we're not uploading, we want to ensure we have an
	// agent-version set anyway, to appease FinishInstanceConfig.
	// In the latter case, setBootstrapTools will later set
	// agent-version to the correct thing.
	if args.AgentVersion != nil {
		agentVersion = *args.AgentVersion
	}
	if cfg, err = cfg.Apply(map[string]interface{}{
		"agent-version": agentVersion.String(),
	}); err != nil {
		return errors.Trace(err)
	}
	if err = environ.SetConfig(ctx, cfg); err != nil {
		return errors.Trace(err)
	}

	if bootstrapParams.BootstrapConstraints.HasInstanceRole() {
		if args.CloudCredential != nil {
			if args.CloudCredential.AuthType() == cloud.InstanceRoleAuthType {
				return errors.NotSupportedf("instance role constraint with instance role credential")
			}
			if args.CloudCredential.AuthType() == cloud.ManagedIdentityAuthType {
				return errors.NotSupportedf("instance role constraint with managed identity credential")
			}
		}
		instanceRoleEnviron, ok := environ.(environs.InstanceRole)
		if !ok || !instanceRoleEnviron.SupportsInstanceRoles(ctx) {
			return errors.NewNotSupported(nil, "instance role constraint for provider")
		}

		bootstrapParams, err = finaliseInstanceRole(ctx, instanceRoleEnviron, bootstrapParams)
		if err != nil {
			return errors.Annotate(err, "finalising instance role for provider")
		}
	}

	ctx.Verbosef("Starting new instance for initial controller")

	result, err := environ.Bootstrap(ctx, bootstrapParams)
	if err != nil {
		return errors.Trace(err)
	}

	publicKey, err := userPublicSigningKey()
	if err != nil {
		return errors.Trace(err)
	}
	instanceConfig, err := instancecfg.NewBootstrapInstanceConfig(
		args.ControllerConfig,
		bootstrapParams.BootstrapConstraints,
		args.ModelConstraints,
		result.Base,
		publicKey,
		args.ExtraAgentValuesForTesting,
	)
	if err != nil {
		return errors.Trace(err)
	}

	// Set SSHServerHostKey if provided by the user.
	instanceConfig.Bootstrap.StateInitializationParams.SSHServerHostKey = args.SSHServerHostKey

	matchingTools, err := bootstrapParams.AvailableTools.Match(coretools.Filter{
		Arch:   result.Arch,
		OSType: result.Base.OS,
	})
	if err != nil {
		return errors.Annotatef(err, "expected tools for %q", result.Base.OS)
	}
	selectedToolsList, err := getBootstrapToolsVersion(ctx, matchingTools)
	if err != nil {
		return errors.Trace(err)
	}
	// We set agent-version to the newest version, so the agent will immediately upgrade itself.
	// Note that this only is relevant if a specific agent version has not been requested, since
	// in that case the specific version will be the only version available.
	newestToolVersion, _ := matchingTools.Newest()
	// set agent version before finalizing bootstrap config
	if err := setBootstrapAgentVersion(ctx, environ, newestToolVersion); err != nil {
		return errors.Trace(err)
	}

	ctx.Infof("Installing Juju agent on bootstrap instance")
	if err := instanceConfig.SetTools(selectedToolsList); err != nil {
		return errors.Trace(err)
	}

	if err := instanceConfig.SetSnapSource(args.JujuDbSnapPath, args.JujuDbSnapAssertionsPath); err != nil {
		return errors.Trace(err)
	}

	if err := instanceConfig.SetControllerCharm(args.ControllerCharmPath); err != nil {
		return errors.Trace(err)
	}
	instanceConfig.Bootstrap.ControllerCharmChannel = args.ControllerCharmChannel

	var environVersion int
	if e, ok := environ.(environs.Environ); ok {
		environVersion = e.Provider().Version()
	}

	if finalizer, ok := environ.(environs.BootstrapCredentialsFinaliser); ok {
		cred, err := finalizer.FinaliseBootstrapCredential(
			ctx,
			bootstrapParams,
			args.CloudCredential)

		if err != nil {
			return errors.Annotate(err, "finalizing bootstrap credential")
		}

		args.CloudCredential = cred
	}

	// Make sure we have the most recent environ config as the specified
	// tools version has been updated there.
	if err := finalizeInstanceBootstrapConfig(
		ctx, instanceConfig, args, environ.Config(), environVersion, customImageMetadata,
	); err != nil {
		return errors.Annotate(err, "finalizing bootstrap instance config")
	}
	if err := result.CloudBootstrapFinalizer(ctx, instanceConfig, args.DialOpts); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func finaliseInstanceRole(
	ctx context.Context,
	ir environs.InstanceRole,
	args environs.BootstrapParams,
) (environs.BootstrapParams, error) {
	if *args.BootstrapConstraints.InstanceRole !=
		environs.InstanceProfileAutoCreate {
		return args, nil
	}
	irName, err := ir.CreateAutoInstanceRole(ctx, args)
	args.BootstrapConstraints.InstanceRole = &irName
	return args, err
}

// Bootstrap bootstraps the given environment. The supplied constraints are
// used to provision the instance, and are also set within the bootstrapped
// environment.
func Bootstrap(
	ctx environs.BootstrapContext,
	environ environs.BootstrapEnviron,
	args BootstrapParams,
) error {
	if err := args.Validate(); err != nil {
		return errors.Annotate(err, "validating bootstrap parameters")
	}

	bootstrapParams := environs.BootstrapParams{
		// We set the authorized keys that are allowed to ssh to the controller
		// instance during bootstrap.
		AuthorizedKeys:             args.ControllerModelAuthorizedKeys,
		CloudName:                  args.Cloud.Name,
		CloudRegion:                args.CloudRegion,
		ControllerConfig:           args.ControllerConfig,
		ModelConstraints:           args.ModelConstraints,
		StoragePools:               args.StoragePools,
		BootstrapBase:              args.BootstrapBase,
		SupportedBootstrapBases:    args.SupportedBootstrapBases,
		Placement:                  args.Placement,
		Force:                      args.Force,
		ExtraAgentValuesForTesting: args.ExtraAgentValuesForTesting,
	}
	doBootstrap := bootstrapIAAS
	if cloud.CloudIsCAAS(args.Cloud) {
		doBootstrap = bootstrapCAAS
	}

	if err := doBootstrap(ctx, environ, args, bootstrapParams); err != nil {
		return errors.Trace(err)
	}
	if IsContextDone(ctx) {
		ctx.Infof("Bootstrap cancelled, you may need to manually remove the bootstrap instance")
		return Cancelled()
	}

	ctx.Infof("Bootstrap agent now started")
	return nil
}

func finalizeInstanceBootstrapConfig(
	ctx environs.BootstrapContext,
	icfg *instancecfg.InstanceConfig,
	args BootstrapParams,
	cfg *config.Config,
	environVersion int,
	customImageMetadata []*imagemetadata.ImageMetadata,
) error {
	if icfg.APIInfo != nil {
		return errors.New("machine configuration already has api info")
	}
	controllerCfg := icfg.ControllerConfig
	caCert, hasCACert := controllerCfg.CACert()
	if !hasCACert {
		return errors.New("controller configuration has no ca-cert")
	}
	icfg.APIInfo = &api.Info{
		Password: args.AdminSecret,
		CACert:   caCert,
		ModelTag: names.NewModelTag(cfg.UUID()),
	}

	authority, err := pki.NewDefaultAuthorityPemCAKey(
		[]byte(caCert), []byte(args.CAPrivateKey))
	if err != nil {
		return errors.Annotate(err, "loading juju certificate authority")
	}

	leaf, err := authority.LeafRequestForGroup(pki.DefaultLeafGroup).
		AddDNSNames(controller.DefaultDNSNames...).
		Commit()

	if err != nil {
		return errors.Annotate(err, "make juju default controller cert")
	}

	cert, key, err := leaf.ToPemParts()
	if err != nil {
		return errors.Annotate(err, "encoding default controller cert to pem")
	}

	agentVersion, has := cfg.AgentVersion()
	if !has {
		return errors.New("finalising instance bootstrap config, agent version not set on model config")
	}

	icfg.Bootstrap.ControllerAgentInfo = controller.ControllerAgentInfo{
		APIPort:      controllerCfg.APIPort(),
		Cert:         string(cert),
		PrivateKey:   string(key),
		CAPrivateKey: args.CAPrivateKey,
	}
	icfg.Bootstrap.StateInitializationParams.AgentVersion = agentVersion
	icfg.Bootstrap.StateInitializationParams.ControllerModelAuthorizedKeys = args.ControllerModelAuthorizedKeys
	icfg.Bootstrap.ControllerModelConfig = cfg
	icfg.Bootstrap.ControllerModelEnvironVersion = environVersion
	icfg.Bootstrap.CustomImageMetadata = customImageMetadata
	icfg.Bootstrap.ControllerCloud = args.Cloud
	icfg.Bootstrap.ControllerCloudRegion = args.CloudRegion
	icfg.Bootstrap.ControllerCloudCredential = args.CloudCredential
	icfg.Bootstrap.ControllerCloudCredentialName = args.CloudCredentialName
	icfg.Bootstrap.ControllerConfig = args.ControllerConfig
	icfg.Bootstrap.ControllerInheritedConfig = args.ControllerInheritedConfig
	icfg.Bootstrap.RegionInheritedConfig = args.Cloud.RegionConfig
	icfg.Bootstrap.StoragePools = args.StoragePools
	icfg.Bootstrap.Timeout = args.DialOpts.Timeout
	icfg.Bootstrap.JujuDbSnapPath = args.JujuDbSnapPath
	icfg.Bootstrap.JujuDbSnapAssertionsPath = args.JujuDbSnapAssertionsPath
	icfg.Bootstrap.ControllerCharm = args.ControllerCharmPath
	icfg.Bootstrap.ControllerCharmChannel = args.ControllerCharmChannel
	return nil
}

func finalizePodBootstrapConfig(
	ctx environs.BootstrapContext,
	pcfg *podcfg.ControllerPodConfig,
	args BootstrapParams,
	cfg *config.Config,
) error {
	if pcfg.APIInfo != nil {
		return errors.New("machine configuration already has api info")
	}

	controllerCfg := pcfg.Controller
	caCert, hasCACert := controllerCfg.CACert()
	if !hasCACert {
		return errors.New("controller configuration has no ca-cert")
	}
	pcfg.APIInfo = &api.Info{
		Password: args.AdminSecret,
		CACert:   caCert,
		ModelTag: names.NewModelTag(cfg.UUID()),
	}

	authority, err := pki.NewDefaultAuthorityPemCAKey(
		[]byte(caCert), []byte(args.CAPrivateKey))
	if err != nil {
		return errors.Annotate(err, "loading juju certificate authority")
	}

	// We generate a controller certificate with a set of well known static dns
	// names. IP addresses are left for other workers to make subsequent
	// certificates.
	leaf, err := authority.LeafRequestForGroup(pki.DefaultLeafGroup).
		AddDNSNames(controller.DefaultDNSNames...).
		Commit()

	if err != nil {
		return errors.Annotate(err, "make juju default controller cert")
	}

	cert, key, err := leaf.ToPemParts()
	if err != nil {
		return errors.Annotate(err, "encoding default controller cert to pem")
	}

	pcfg.Bootstrap.ControllerAgentInfo = controller.ControllerAgentInfo{
		APIPort:      controllerCfg.APIPort(),
		Cert:         string(cert),
		PrivateKey:   string(key),
		CAPrivateKey: args.CAPrivateKey,
	}
	if _, ok := cfg.AgentVersion(); !ok {
		return errors.New("controller model configuration has no agent-version")
	}

	pcfg.AgentEnvironment = make(map[string]string)
	for k, v := range args.ExtraAgentValuesForTesting {
		pcfg.AgentEnvironment[k] = v
	}

	pcfg.Bootstrap.ControllerModelAuthorizedKeys = args.ControllerModelAuthorizedKeys
	pcfg.Bootstrap.ControllerModelConfig = cfg
	pcfg.Bootstrap.ControllerCloud = args.Cloud
	pcfg.Bootstrap.ControllerCloudRegion = args.CloudRegion
	pcfg.Bootstrap.ControllerCloudCredential = args.CloudCredential
	pcfg.Bootstrap.ControllerCloudCredentialName = args.CloudCredentialName
	pcfg.Bootstrap.ControllerConfig = args.ControllerConfig
	pcfg.Bootstrap.ControllerInheritedConfig = args.ControllerInheritedConfig
	pcfg.Bootstrap.StoragePools = args.StoragePools
	pcfg.Bootstrap.Timeout = args.DialOpts.Timeout
	pcfg.Bootstrap.ControllerServiceType = args.ControllerServiceType
	pcfg.Bootstrap.ControllerExternalName = args.ControllerExternalName
	pcfg.Bootstrap.ControllerExternalIPs = append([]string(nil), args.ControllerExternalIPs...)
	pcfg.Bootstrap.ControllerCharmPath = args.ControllerCharmPath
	pcfg.Bootstrap.ControllerCharmChannel = args.ControllerCharmChannel
	pcfg.Bootstrap.SSHServerHostKey = args.SSHServerHostKey
	return nil
}

func userPublicSigningKey() (string, error) {
	signingKeyFile := os.Getenv("JUJU_STREAMS_PUBLICKEY_FILE")
	signingKey := ""
	if signingKeyFile != "" {
		path, err := utils.NormalizePath(signingKeyFile)
		if err != nil {
			return "", errors.Annotatef(err, "cannot expand key file path: %s", signingKeyFile)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return "", errors.Annotatef(err, "invalid public key file: %s", path)
		}
		signingKey = string(b)
	}
	return signingKey, nil
}

// bootstrapImageMetadata returns the image metadata to use for bootstrapping
// the given environment. If the environment provider does not make use of
// simplestreams, no metadata will be returned.
//
// If a bootstrap image ID is specified, image metadata will be synthesised
// using that image ID, and the architecture and series specified by the
// initiator. In addition, the custom image metadata that is saved into the
// state database will have the synthesised image metadata added to it.
func bootstrapImageMetadata(
	ctx context.Context,
	environ environs.BootstrapEnviron,
	fetcher imagemetadata.SimplestreamsFetcher,
	bootstrapBase *corebase.Base,
	bootstrapArch string,
	bootstrapImageId string,
	customImageMetadata *[]*imagemetadata.ImageMetadata,
) ([]*imagemetadata.ImageMetadata, error) {

	hasRegion, ok := environ.(simplestreams.HasRegion)
	if !ok {
		if bootstrapImageId != "" {
			// We only support specifying image IDs for providers
			// that use simplestreams for now.
			return nil, errors.NotSupportedf(
				"specifying bootstrap image for %q provider",
				environ.Config().Type(),
			)
		}
		// No region, no metadata.
		return nil, nil
	}
	region, err := hasRegion.Region()
	if err != nil {
		return nil, errors.Trace(err)
	}

	if bootstrapImageId != "" {
		if bootstrapBase == nil {
			return nil, errors.NotValidf("no base specified with bootstrap image")
		}
		// The returned metadata does not have information about the
		// storage or virtualisation type. Any provider that wants to
		// filter on those properties should allow for empty values.
		meta := &imagemetadata.ImageMetadata{
			Id:         bootstrapImageId,
			Arch:       bootstrapArch,
			Version:    bootstrapBase.Channel.Track,
			RegionName: region.Region,
			Endpoint:   region.Endpoint,
			Stream:     environ.Config().ImageStream(),
		}
		*customImageMetadata = append(*customImageMetadata, meta)
		return []*imagemetadata.ImageMetadata{meta}, nil
	}

	// For providers that support making use of simplestreams
	// image metadata, search public image metadata. We need
	// to pass this onto Bootstrap for selecting images.
	sources, err := environs.ImageMetadataSources(environ, fetcher)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// This constraint will search image metadata for all supported architectures and series.
	imageConstraint, err := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: region,
		Stream:    environ.Config().ImageStream(),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Debugf(ctx, "constraints for image metadata lookup %v", imageConstraint)

	// Get image metadata from all data sources.
	// Since order of data source matters, order of image metadata matters too. Append is important here.
	var publicImageMetadata []*imagemetadata.ImageMetadata
	for _, source := range sources {
		sourceMetadata, _, err := imagemetadata.Fetch(ctx, fetcher, []simplestreams.DataSource{source}, imageConstraint)
		if errors.Is(err, errors.NotFound) || errors.Is(err, errors.Unauthorized) {
			logger.Debugf(ctx, "ignoring image metadata in %s: %v", source.Description(), err)
			// Just keep looking...
			continue
		} else if err != nil {
			// When we get an actual protocol/unexpected error, we need to stop.
			return nil, errors.Annotatef(err, "failed looking for image metadata in %s", source.Description())
		}
		logger.Debugf(ctx, "found %d image metadata in %s", len(sourceMetadata), source.Description())
		publicImageMetadata = append(publicImageMetadata, sourceMetadata...)
	}

	logger.Debugf(ctx, "found %d image metadata from all image data sources", len(publicImageMetadata))
	return publicImageMetadata, nil
}

// getBootstrapToolsVersion returns the newest tools from the given tools list.
func getBootstrapToolsVersion(ctx context.Context, possibleTools coretools.List) (coretools.List, error) {
	if len(possibleTools) == 0 {
		return nil, errors.New("no bootstrap agent binaries available")
	}
	var newVersion semversion.Number
	newVersion, toolsList := possibleTools.Newest()
	logger.Infof(ctx, "newest version: %s", newVersion)
	bootstrapVersion := newVersion
	// We should only ever bootstrap the exact same version as the client,
	// or we risk bootstrap incompatibility.
	if !isCompatibleVersion(newVersion, jujuversion.Current) {
		compatibleVersion, compatibleTools := findCompatibleTools(possibleTools, jujuversion.Current)
		if len(compatibleTools) == 0 {
			logger.Infof(ctx,
				"failed to find %s agent binaries, will attempt to use %s",
				jujuversion.Current, newVersion,
			)
		} else {
			bootstrapVersion, toolsList = compatibleVersion, compatibleTools
		}
	}
	logger.Infof(ctx, "picked bootstrap agent binary version: %s", bootstrapVersion)
	return toolsList, nil
}

// setBootstrapAgentVersion updates the agent-version configuration attribute.
func setBootstrapAgentVersion(ctx context.Context, environ environs.Configer, toolsVersion semversion.Number) error {
	cfg := environ.Config()
	if agentVersion, _ := cfg.AgentVersion(); agentVersion != toolsVersion {
		cfg, err := cfg.Apply(map[string]interface{}{
			"agent-version": toolsVersion.String(),
		})
		if err == nil {
			err = environ.SetConfig(ctx, cfg)
		}
		if err != nil {
			return errors.Errorf("failed to update model configuration: %v", err)
		}
	}
	return nil
}

// findCompatibleTools finds tools in the list that have the same major, minor
// and patch level as jujuversion.Current.
//
// Build number is not important to match; uploaded tools will have
// incremented build number, and we want to match them.
func findCompatibleTools(possibleTools coretools.List, version semversion.Number) (semversion.Number, coretools.List) {
	var compatibleTools coretools.List
	for _, tools := range possibleTools {
		if isCompatibleVersion(tools.Version.Number, version) {
			compatibleTools = append(compatibleTools, tools)
		}
	}
	return compatibleTools.Newest()
}

func isCompatibleVersion(v1, v2 semversion.Number) bool {
	x := v1.ToPatch()
	y := v2.ToPatch()
	return x.Compare(y) == 0
}

// setPrivateMetadataSources verifies the specified metadataDir exists,
// uses it to set the default agent metadata source for agent binaries,
// and adds an image metadata source after verifying the contents. If the
// directory ends in tools, only the default tools metadata source will be
// set. Same for images.
func setPrivateMetadataSources(ctx context.Context, fetcher imagemetadata.SimplestreamsFetcher, metadataDir string) ([]*imagemetadata.ImageMetadata, error) {
	if _, err := os.Stat(metadataDir); err != nil {
		if !os.IsNotExist(err) {
			return nil, errors.Annotate(err, "cannot access simplestreams metadata directory")
		}
		return nil, errors.NotFoundf("simplestreams metadata source: %s", metadataDir)
	}

	agentBinaryMetadataDir := metadataDir
	ending := filepath.Base(agentBinaryMetadataDir)
	if ending != storage.BaseToolsPath {
		agentBinaryMetadataDir = filepath.Join(metadataDir, storage.BaseToolsPath)
	}
	if _, err := os.Stat(agentBinaryMetadataDir); err != nil {
		if !os.IsNotExist(err) {
			return nil, errors.Annotate(err, "cannot access agent metadata")
		}
		logger.Debugf(ctx, "no agent directory found, using default agent metadata source: %s", tools.DefaultBaseURL)
	} else {
		if ending == storage.BaseToolsPath {
			// As the specified metadataDir ended in 'tools'
			// assume that is the only metadata to find and return.
			tools.DefaultBaseURL = filepath.Dir(metadataDir)
			logger.Debugf(ctx, "setting default agent metadata source: %s", tools.DefaultBaseURL)
			return nil, nil
		} else {
			tools.DefaultBaseURL = metadataDir
			logger.Debugf(ctx, "setting default agent metadata source: %s", tools.DefaultBaseURL)
		}
	}

	imageMetadataDir := metadataDir
	ending = filepath.Base(imageMetadataDir)
	if ending != storage.BaseImagesPath {
		imageMetadataDir = filepath.Join(metadataDir, storage.BaseImagesPath)
	}
	if _, err := os.Stat(imageMetadataDir); err != nil {
		if !os.IsNotExist(err) {
			return nil, errors.Annotate(err, "cannot access image metadata")
		}
		return nil, nil
	} else {
		logger.Debugf(ctx, "setting default image metadata source: %s", imageMetadataDir)
	}

	baseURL := fmt.Sprintf("file://%s", filepath.ToSlash(imageMetadataDir))
	publicKey, err := simplestreams.UserPublicSigningKey()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// TODO: (hml) 2020-01-08
	// Why ignore the the model-config "ssl-hostname-verification" value in
	// the config here? Its default value is true.
	dataSourceConfig := simplestreams.Config{
		Description:          "bootstrap metadata",
		BaseURL:              baseURL,
		PublicSigningKey:     publicKey,
		HostnameVerification: false,
		Priority:             simplestreams.CUSTOM_CLOUD_DATA,
	}
	if err := dataSourceConfig.Validate(); err != nil {
		return nil, errors.Annotate(err, "simplestreams config validation failed")
	}
	dataSource := fetcher.NewDataSource(dataSourceConfig)

	// Read the image metadata, as we'll want to upload it to the environment.
	imageConstraint, err := imagemetadata.NewImageConstraint(simplestreams.LookupParams{})
	if err != nil {
		return nil, errors.Trace(err)
	}
	existingMetadata, _, err := imagemetadata.Fetch(ctx, fetcher, []simplestreams.DataSource{dataSource}, imageConstraint)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Annotatef(err, "cannot read image metadata in %s", dataSource.Description())
	}

	// Add an image metadata datasource for constraint validation, etc.
	environs.RegisterUserImageDataSourceFunc("bootstrap metadata", func(environs.Environ) (simplestreams.DataSource, error) {
		return dataSource, nil
	})
	logger.Infof(ctx, "custom image metadata added to search path")
	return existingMetadata, nil
}

// Cancelled returns an error that satisfies IsCancelled.
func Cancelled() error {
	return errCancelled
}

// IsContextDone returns true if the context is done.
func IsContextDone(ctx context.Context) bool {
	return ctx.Err() != nil
}
