// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"archive/tar"
	"compress/bzip2"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	"github.com/juju/utils/set"
	"github.com/juju/utils/ssh"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/gui"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/sync"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/mongo"
	coretools "github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

const noToolsMessage = `Juju cannot bootstrap because no agent binaries are available for your model.
You may want to use the 'agent-metadata-url' configuration setting to specify the binaries' location.
`

var (
	logger = loggo.GetLogger("juju.environs.bootstrap")
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

	// BootstrapSeries, if specified, is the series to use for the
	// initial bootstrap machine.
	BootstrapSeries string

	// BootstrapImage, if specified, is the image ID to use for the
	// initial bootstrap machine.
	BootstrapImage string

	// CloudName is the name of the cloud that Juju will be bootstrapped in.
	CloudName string

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

	// RegionInheritedConfig holds region specific configuration attributes to
	// be shared across all models in the same controller on a particular
	// cloud.
	RegionInheritedConfig cloud.RegionConfig

	// HostedModelConfig is the set of config attributes to be overlaid
	// on the controller config to construct the initial hosted model
	// config.
	HostedModelConfig map[string]interface{}

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
	AgentVersion *version.Number

	// GUIDataSourceBaseURL holds the simplestreams data source base URL
	// used to retrieve the Juju GUI archive installed in the controller.
	// If not set, the Juju GUI is not installed from simplestreams.
	GUIDataSourceBaseURL string

	// AdminSecret contains the administrator password.
	AdminSecret string

	// CAPrivateKey is the controller's CA certificate private key.
	CAPrivateKey string

	// DialOpts contains the bootstrap dial options.
	DialOpts environs.BootstrapDialOpts
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
	// TODO(axw) validate other things.
	return nil
}

// Bootstrap bootstraps the given environment. The supplied constraints are
// used to provision the instance, and are also set within the bootstrapped
// environment.
func Bootstrap(ctx environs.BootstrapContext, environ environs.Environ, args BootstrapParams) error {
	if err := args.Validate(); err != nil {
		return errors.Annotate(err, "validating bootstrap parameters")
	}

	cfg := environ.Config()
	if authKeys := ssh.SplitAuthorisedKeys(cfg.AuthorizedKeys()); len(authKeys) == 0 {
		// Apparently this can never happen, so it's not tested. But, one day,
		// Config will act differently (it's pretty crazy that, AFAICT, the
		// authorized-keys are optional config settings... but it's impossible
		// to actually *create* a config without them)... and when it does,
		// we'll be here to catch this problem early.
		return errors.Errorf("model configuration has no authorized-keys")
	}

	_, supportsNetworking := environs.SupportsNetworking(environ)
	logger.Debugf("model %q supports service/machine networks: %v", cfg.Name(), supportsNetworking)
	disableNetworkManagement, _ := cfg.DisableNetworkManagement()
	logger.Debugf("network management by juju enabled: %v", !disableNetworkManagement)

	// Set default tools metadata source, add image metadata source,
	// then verify constraints. Providers may rely on image metadata
	// for constraint validation.
	var customImageMetadata []*imagemetadata.ImageMetadata
	if args.MetadataDir != "" {
		var err error
		customImageMetadata, err = setPrivateMetadataSources(args.MetadataDir)
		if err != nil {
			return err
		}
	}

	var bootstrapSeries *string
	if args.BootstrapSeries != "" {
		bootstrapSeries = &args.BootstrapSeries
	}

	var bootstrapArchForImageSearch string
	if args.BootstrapConstraints.Arch != nil {
		bootstrapArchForImageSearch = *args.BootstrapConstraints.Arch
	} else if args.ModelConstraints.Arch != nil {
		bootstrapArchForImageSearch = *args.ModelConstraints.Arch
	} else {
		bootstrapArchForImageSearch = arch.HostArch()
		// We no longer support i386.
		if bootstrapArchForImageSearch == arch.I386 {
			bootstrapArchForImageSearch = arch.AMD64
		}
	}

	ctx.Verbosef("Loading image metadata")
	imageMetadata, err := bootstrapImageMetadata(environ,
		bootstrapSeries,
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

	constraintsValidator, err := environ.ConstraintsValidator()
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

	// The arch we use to find tools isn't the boostrapConstraints arch.
	// We copy the constraints arch to a separate variable and
	// update it from the host arch if not specified.
	// (axw) This is still not quite right:
	// For e.g. if there is a MAAS with only ARM64 machines,
	// on an AMD64 client, we're going to look for only AMD64 tools,
	// limiting what the provider can bootstrap anyway.
	var bootstrapArch string
	if bootstrapConstraints.Arch != nil {
		bootstrapArch = *bootstrapConstraints.Arch
	} else {
		// If no arch is specified as a constraint, we'll bootstrap
		// on the same arch as the client used to bootstrap.
		bootstrapArch = arch.HostArch()
		// We no longer support controllers on i386.
		// If we are bootstrapping from an i386 client,
		// we'll look for amd64 tools.
		if bootstrapArch == arch.I386 {
			bootstrapArch = arch.AMD64
		}
	}

	var availableTools coretools.List
	if !args.BuildAgent {
		ctx.Infof("Looking for packaged Juju agent version %s for %s", args.AgentVersion, bootstrapArch)
		availableTools, err = findPackagedTools(environ, args.AgentVersion, &bootstrapArch, bootstrapSeries)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
	}
	// If there are no prepackaged tools and a specific version has not been
	// requested, look for or build a local binary.
	var builtTools *sync.BuiltAgent
	if len(availableTools) == 0 && (args.AgentVersion == nil || isCompatibleVersion(*args.AgentVersion, jujuversion.Current)) {
		if args.BuildAgentTarball == nil {
			return errors.New("cannot build agent binary to upload")
		}
		if err := validateUploadAllowed(environ, &bootstrapArch, bootstrapSeries, constraintsValidator); err != nil {
			return err
		}
		if args.BuildAgent {
			ctx.Infof("Building local Juju agent binary version %s for %s", args.AgentVersion, bootstrapArch)
		} else {
			ctx.Infof("No packaged binary found, preparing local Juju agent binary")
		}
		var forceVersion version.Number
		availableTools, forceVersion = locallyBuildableTools(bootstrapSeries)
		builtTools, err = args.BuildAgentTarball(args.BuildAgent, &forceVersion, cfg.AgentStream())
		if err != nil {
			return errors.Annotate(err, "cannot package bootstrap agent binary")
		}
		defer os.RemoveAll(builtTools.Dir)
		for i, tool := range availableTools {
			if tool.URL != "" {
				continue
			}
			filename := filepath.Join(builtTools.Dir, builtTools.StorageName)
			tool.URL = fmt.Sprintf("file://%s", filename)
			tool.Size = builtTools.Size
			tool.SHA256 = builtTools.Sha256Hash
			availableTools[i] = tool
		}
	}
	if len(availableTools) == 0 {
		return errors.New(noToolsMessage)
	}

	// If we're uploading, we must override agent-version;
	// if we're not uploading, we want to ensure we have an
	// agent-version set anyway, to appease FinishInstanceConfig.
	// In the latter case, setBootstrapTools will later set
	// agent-version to the correct thing.
	agentVersion := jujuversion.Current
	if args.AgentVersion != nil {
		agentVersion = *args.AgentVersion
	}
	if cfg, err = cfg.Apply(map[string]interface{}{
		"agent-version": agentVersion.String(),
	}); err != nil {
		return err
	}
	if err = environ.SetConfig(cfg); err != nil {
		return err
	}

	ctx.Verbosef("Starting new instance for initial controller")

	result, err := environ.Bootstrap(ctx, environs.BootstrapParams{
		CloudName:            args.CloudName,
		CloudRegion:          args.CloudRegion,
		ControllerConfig:     args.ControllerConfig,
		ModelConstraints:     args.ModelConstraints,
		BootstrapConstraints: bootstrapConstraints,
		BootstrapSeries:      args.BootstrapSeries,
		Placement:            args.Placement,
		AvailableTools:       availableTools,
		ImageMetadata:        imageMetadata,
	})
	if err != nil {
		return err
	}

	matchingTools, err := availableTools.Match(coretools.Filter{
		Arch:   result.Arch,
		Series: result.Series,
	})
	if err != nil {
		return err
	}
	selectedToolsList, err := getBootstrapToolsVersion(matchingTools)
	if err != nil {
		return err
	}
	// We set agent-version to the newest version, so the agent will immediately upgrade itself.
	// Note that this only is relevant if a specific agent version has not been requested, since
	// in that case the specific version will be the only version available.
	newestVersion, _ := matchingTools.Newest()
	if err := setBootstrapToolsVersion(environ, newestVersion); err != nil {
		return err
	}

	logger.Infof("Installing Juju agent on bootstrap instance")
	publicKey, err := userPublicSigningKey()
	if err != nil {
		return err
	}
	instanceConfig, err := instancecfg.NewBootstrapInstanceConfig(
		args.ControllerConfig,
		bootstrapConstraints,
		args.ModelConstraints,
		result.Series,
		publicKey,
	)
	if err != nil {
		return err
	}
	if err := instanceConfig.SetTools(selectedToolsList); err != nil {
		return errors.Trace(err)
	}
	// Make sure we have the most recent environ config as the specified
	// tools version has been updated there.
	cfg = environ.Config()
	if err := finalizeInstanceBootstrapConfig(ctx, instanceConfig, args, cfg, customImageMetadata); err != nil {
		return errors.Annotate(err, "finalizing bootstrap instance config")
	}
	if err := result.Finalize(ctx, instanceConfig, args.DialOpts); err != nil {
		return err
	}
	ctx.Infof("Bootstrap agent now started")
	return nil
}

func finalizeInstanceBootstrapConfig(
	ctx environs.BootstrapContext,
	icfg *instancecfg.InstanceConfig,
	args BootstrapParams,
	cfg *config.Config,
	customImageMetadata []*imagemetadata.ImageMetadata,
) error {
	if icfg.APIInfo != nil || icfg.Controller.MongoInfo != nil {
		return errors.New("machine configuration already has api/state info")
	}
	controllerCfg := icfg.Controller.Config
	caCert, hasCACert := controllerCfg.CACert()
	if !hasCACert {
		return errors.New("controller configuration has no ca-cert")
	}
	icfg.APIInfo = &api.Info{
		Password: args.AdminSecret,
		CACert:   caCert,
		ModelTag: names.NewModelTag(cfg.UUID()),
	}
	icfg.Controller.MongoInfo = &mongo.MongoInfo{
		Password: args.AdminSecret,
		Info:     mongo.Info{CACert: caCert},
	}

	// These really are directly relevant to running a controller.
	// Initially, generate a controller certificate with no host IP
	// addresses in the SAN field. Once the controller is up and the
	// NIC addresses become known, the certificate can be regenerated.
	cert, key, err := controller.GenerateControllerCertAndKey(caCert, args.CAPrivateKey, nil)
	if err != nil {
		return errors.Annotate(err, "cannot generate controller certificate")
	}
	icfg.Bootstrap.StateServingInfo = params.StateServingInfo{
		StatePort:    controllerCfg.StatePort(),
		APIPort:      controllerCfg.APIPort(),
		Cert:         string(cert),
		PrivateKey:   string(key),
		CAPrivateKey: args.CAPrivateKey,
	}
	if _, ok := cfg.AgentVersion(); !ok {
		return errors.New("controller model configuration has no agent-version")
	}

	icfg.Bootstrap.ControllerModelConfig = cfg
	icfg.Bootstrap.CustomImageMetadata = customImageMetadata
	icfg.Bootstrap.ControllerCloudName = args.CloudName
	icfg.Bootstrap.ControllerCloud = args.Cloud
	icfg.Bootstrap.ControllerCloudRegion = args.CloudRegion
	icfg.Bootstrap.ControllerCloudCredential = args.CloudCredential
	icfg.Bootstrap.ControllerCloudCredentialName = args.CloudCredentialName
	icfg.Bootstrap.ControllerConfig = args.ControllerConfig
	icfg.Bootstrap.ControllerInheritedConfig = args.ControllerInheritedConfig
	icfg.Bootstrap.RegionInheritedConfig = args.Cloud.RegionConfig
	icfg.Bootstrap.HostedModelConfig = args.HostedModelConfig
	icfg.Bootstrap.Timeout = args.DialOpts.Timeout
	icfg.Bootstrap.GUI = guiArchive(args.GUIDataSourceBaseURL, func(msg string) {
		ctx.Infof(msg)
	})
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
		b, err := ioutil.ReadFile(path)
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
	environ environs.Environ,
	bootstrapSeries *string,
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
		if bootstrapSeries == nil {
			return nil, errors.NotValidf("no series specified with bootstrap image")
		}
		seriesVersion, err := series.SeriesVersion(*bootstrapSeries)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// The returned metadata does not have information about the
		// storage or virtualisation type. Any provider that wants to
		// filter on those properties should allow for empty values.
		meta := &imagemetadata.ImageMetadata{
			Id:         bootstrapImageId,
			Arch:       bootstrapArch,
			Version:    seriesVersion,
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
	sources, err := environs.ImageMetadataSources(environ)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// This constraint will search image metadata for all supported architectures and series.
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: region,
		Stream:    environ.Config().ImageStream(),
	})
	logger.Debugf("constraints for image metadata lookup %v", imageConstraint)

	// Get image metadata from all data sources.
	// Since order of data source matters, order of image metadata matters too. Append is important here.
	var publicImageMetadata []*imagemetadata.ImageMetadata
	for _, source := range sources {
		sourceMetadata, _, err := imagemetadata.Fetch([]simplestreams.DataSource{source}, imageConstraint)
		if err != nil {
			logger.Debugf("ignoring image metadata in %s: %v", source.Description(), err)
			// Just keep looking...
			continue
		}
		logger.Debugf("found %d image metadata in %s", len(sourceMetadata), source.Description())
		publicImageMetadata = append(publicImageMetadata, sourceMetadata...)
	}

	logger.Debugf("found %d image metadata from all image data sources", len(publicImageMetadata))
	if len(publicImageMetadata) == 0 {
		return nil, errors.New("no image metadata found")
	}
	return publicImageMetadata, nil
}

// getBootstrapToolsVersion returns the newest tools from the given tools list.
func getBootstrapToolsVersion(possibleTools coretools.List) (coretools.List, error) {
	if len(possibleTools) == 0 {
		return nil, errors.New("no bootstrap tools available")
	}
	var newVersion version.Number
	newVersion, toolsList := possibleTools.Newest()
	logger.Infof("newest version: %s", newVersion)
	bootstrapVersion := newVersion
	// We should only ever bootstrap the exact same version as the client,
	// or we risk bootstrap incompatibility.
	if !isCompatibleVersion(newVersion, jujuversion.Current) {
		compatibleVersion, compatibleTools := findCompatibleTools(possibleTools, jujuversion.Current)
		if len(compatibleTools) == 0 {
			logger.Infof(
				"failed to find %s tools, will attempt to use %s",
				jujuversion.Current, newVersion,
			)
		} else {
			bootstrapVersion, toolsList = compatibleVersion, compatibleTools
		}
	}
	logger.Infof("picked bootstrap tools version: %s", bootstrapVersion)
	return toolsList, nil
}

// setBootstrapToolsVersion updates the agent-version configuration attribute.
func setBootstrapToolsVersion(environ environs.Environ, toolsVersion version.Number) error {
	cfg := environ.Config()
	if agentVersion, _ := cfg.AgentVersion(); agentVersion != toolsVersion {
		cfg, err := cfg.Apply(map[string]interface{}{
			"agent-version": toolsVersion.String(),
		})
		if err == nil {
			err = environ.SetConfig(cfg)
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
func findCompatibleTools(possibleTools coretools.List, version version.Number) (version.Number, coretools.List) {
	var compatibleTools coretools.List
	for _, tools := range possibleTools {
		if isCompatibleVersion(tools.Version.Number, version) {
			compatibleTools = append(compatibleTools, tools)
		}
	}
	return compatibleTools.Newest()
}

func isCompatibleVersion(v1, v2 version.Number) bool {
	v1.Build = 0
	v2.Build = 0
	return v1.Compare(v2) == 0
}

// setPrivateMetadataSources sets the default tools metadata source
// for tools syncing, and adds an image metadata source after verifying
// the contents.
func setPrivateMetadataSources(metadataDir string) ([]*imagemetadata.ImageMetadata, error) {
	logger.Infof("Setting default tools and image metadata sources: %s", metadataDir)
	tools.DefaultBaseURL = metadataDir

	imageMetadataDir := filepath.Join(metadataDir, storage.BaseImagesPath)
	if _, err := os.Stat(imageMetadataDir); err != nil {
		if !os.IsNotExist(err) {
			return nil, errors.Annotate(err, "cannot access image metadata")
		}
		return nil, nil
	}

	baseURL := fmt.Sprintf("file://%s", filepath.ToSlash(imageMetadataDir))
	publicKey, _ := simplestreams.UserPublicSigningKey()
	datasource := simplestreams.NewURLSignedDataSource("bootstrap metadata", baseURL, publicKey, utils.NoVerifySSLHostnames, simplestreams.CUSTOM_CLOUD_DATA, false)

	// Read the image metadata, as we'll want to upload it to the environment.
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{})
	existingMetadata, _, err := imagemetadata.Fetch([]simplestreams.DataSource{datasource}, imageConstraint)
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Annotate(err, "cannot read image metadata")
	}

	// Add an image metadata datasource for constraint validation, etc.
	environs.RegisterUserImageDataSourceFunc("bootstrap metadata", func(environs.Environ) (simplestreams.DataSource, error) {
		return datasource, nil
	})
	logger.Infof("custom image metadata added to search path")
	return existingMetadata, nil
}

// guiArchive returns information on the GUI archive that will be uploaded
// to the controller. Possible errors in retrieving the GUI archive information
// do not prevent the model to be bootstrapped. If dataSourceBaseURL is
// non-empty, remote GUI archive info is retrieved from simplestreams using it
// as the base URL. The given logProgress function is used to inform users
// about errors or progress in setting up the Juju GUI.
func guiArchive(dataSourceBaseURL string, logProgress func(string)) *coretools.GUIArchive {
	// The environment variable is only used for development purposes.
	path := os.Getenv("JUJU_GUI")
	if path != "" {
		vers, err := guiVersion(path)
		if err != nil {
			logProgress(fmt.Sprintf("Cannot use Juju GUI at %q: %s", path, err))
			return nil
		}
		hash, size, err := hashAndSize(path)
		if err != nil {
			logProgress(fmt.Sprintf("Cannot use Juju GUI at %q: %s", path, err))
			return nil
		}
		logProgress(fmt.Sprintf("Fetching Juju GUI %s from local archive", vers))
		return &coretools.GUIArchive{
			Version: vers,
			URL:     "file://" + filepath.ToSlash(path),
			SHA256:  hash,
			Size:    size,
		}
	}
	// Check if the user requested to bootstrap with no GUI.
	if dataSourceBaseURL == "" {
		logProgress("Juju GUI installation has been disabled")
		return nil
	}
	// Fetch GUI archives info from simplestreams.
	source := gui.NewDataSource(dataSourceBaseURL)
	allMeta, err := guiFetchMetadata(gui.ReleasedStream, source)
	if err != nil {
		logProgress(fmt.Sprintf("Unable to fetch Juju GUI info: %s", err))
		return nil
	}
	if len(allMeta) == 0 {
		logProgress("No available Juju GUI archives found")
		return nil
	}
	// Metadata info are returned in descending version order.
	logProgress(fmt.Sprintf("Fetching Juju GUI %s", allMeta[0].Version))
	return &coretools.GUIArchive{
		Version: allMeta[0].Version,
		URL:     allMeta[0].FullPath,
		SHA256:  allMeta[0].SHA256,
		Size:    allMeta[0].Size,
	}
}

// guiFetchMetadata is defined for testing purposes.
var guiFetchMetadata = gui.FetchMetadata

// guiVersion retrieves the GUI version from the juju-gui-* directory included
// in the bz2 archive at the given path.
func guiVersion(path string) (version.Number, error) {
	var number version.Number
	f, err := os.Open(path)
	if err != nil {
		return number, errors.Annotate(err, "cannot open Juju GUI archive")
	}
	defer f.Close()
	prefix := "jujugui-"
	r := tar.NewReader(bzip2.NewReader(f))
	for {
		hdr, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return number, errors.New("cannot read Juju GUI archive")
		}
		info := hdr.FileInfo()
		if !info.IsDir() || !strings.HasPrefix(hdr.Name, prefix) {
			continue
		}
		n := info.Name()[len(prefix):]
		number, err = version.Parse(n)
		if err != nil {
			return number, errors.Errorf("cannot parse version %q", n)
		}
		return number, nil
	}
	return number, errors.New("cannot find Juju GUI version")
}

// hashAndSize calculates and returns the SHA256 hash and the size of the file
// located at the given path.
func hashAndSize(path string) (hash string, size int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, errors.Mask(err)
	}
	defer f.Close()
	h := sha256.New()
	size, err = io.Copy(h, f)
	if err != nil {
		return "", 0, errors.Mask(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), size, nil
}
