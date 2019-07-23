// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gomaasapi"
	"github.com/juju/os"
	"github.com/juju/os/series"
	"github.com/juju/utils"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/tools"
)

const (
	// The version strings indicating the MAAS API version.
	apiVersion1 = "1.0"
	apiVersion2 = "2.0"
)

// A request may fail to due "eventual consistency" semantics, which
// should resolve fairly quickly.  A request may also fail due to a slow
// state transition (for instance an instance taking a while to release
// a security group after termination).  The former failure mode is
// dealt with by shortAttempt, the latter by LongAttempt.
var shortAttempt = utils.AttemptStrategy{
	Total: 5 * time.Second,
	Delay: 200 * time.Millisecond,
}

const statusPollInterval = 5 * time.Second

var (
	ReleaseNodes         = releaseNodes
	DeploymentStatusCall = deploymentStatusCall
	GetMAAS2Controller   = getMAAS2Controller
)

func getMAAS2Controller(maasServer, apiKey string) (gomaasapi.Controller, error) {
	return gomaasapi.NewController(gomaasapi.ControllerArgs{
		BaseURL: maasServer,
		APIKey:  apiKey,
	})
}

func releaseNodes(nodes gomaasapi.MAASObject, ids url.Values) error {
	_, err := nodes.CallPost("release", ids)
	return err
}

type maasEnviron struct {
	name string
	uuid string

	// archMutex gates access to supportedArchitectures
	archMutex sync.Mutex

	// ecfgMutex protects the *Unlocked fields below.
	ecfgMutex sync.Mutex

	ecfgUnlocked       *maasModelConfig
	maasClientUnlocked *gomaasapi.MAASObject
	storageUnlocked    storage.Storage

	// maasController provides access to the MAAS 2.0 API.
	maasController gomaasapi.Controller

	// namespace is used to create the machine and device hostnames.
	namespace instance.Namespace

	availabilityZonesMutex sync.Mutex
	availabilityZones      []common.AvailabilityZone

	// apiVersion tells us if we are using the MAAS 1.0 or 2.0 api.
	apiVersion string

	// GetCapabilities is a function that connects to MAAS to return its set of
	// capabilities.
	GetCapabilities MaasCapabilities
}

var _ environs.Environ = (*maasEnviron)(nil)
var _ environs.Networking = (*maasEnviron)(nil)

// MaasCapabilities represents a function that gets the capabilities of a MAAS
// installation.
type MaasCapabilities func(client *gomaasapi.MAASObject, serverURL string) (set.Strings, error)

func NewEnviron(cloud environs.CloudSpec, cfg *config.Config, getCaps MaasCapabilities) (*maasEnviron, error) {
	if getCaps == nil {
		getCaps = getCapabilities
	}
	env := &maasEnviron{
		name:            cfg.Name(),
		uuid:            cfg.UUID(),
		GetCapabilities: getCaps,
	}
	if err := env.SetConfig(cfg); err != nil {
		return nil, errors.Trace(err)
	}
	if err := env.SetCloudSpec(cloud); err != nil {
		return nil, errors.Trace(err)
	}

	var err error
	env.namespace, err = instance.NewNamespace(cfg.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return env, nil
}

func (env *maasEnviron) usingMAAS2() bool {
	return env.apiVersion == apiVersion2
}

// PrepareForBootstrap is part of the Environ interface.
func (env *maasEnviron) PrepareForBootstrap(ctx environs.BootstrapContext, controllerName string) error {
	if ctx.ShouldVerifyCredentials() {
		if err := verifyCredentials(env, nil); err != nil {
			return err
		}
	}
	return nil
}

// Create is part of the Environ interface.
func (env *maasEnviron) Create(ctx context.ProviderCallContext, p environs.CreateParams) error {
	if err := verifyCredentials(env, ctx); err != nil {
		return err
	}
	return nil
}

// Bootstrap is part of the Environ interface.
func (env *maasEnviron) Bootstrap(
	ctx environs.BootstrapContext, callCtx context.ProviderCallContext, args environs.BootstrapParams,
) (*environs.BootstrapResult, error) {
	result, series, finalizer, err := common.BootstrapInstance(ctx, env, callCtx, args)
	if err != nil {
		return nil, err
	}

	// We want to destroy the started instance if it doesn't transition to Deployed.
	defer func() {
		if err != nil {
			if err := env.StopInstances(callCtx, result.Instance.Id()); err != nil {
				logger.Errorf("error releasing bootstrap instance: %v", err)
			}
		}
	}()

	waitingFinalizer := func(
		ctx environs.BootstrapContext,
		icfg *instancecfg.InstanceConfig,
		dialOpts environs.BootstrapDialOpts,
	) error {
		// Wait for bootstrap instance to change to deployed state.
		if err := env.waitForNodeDeployment(callCtx, result.Instance.Id(), dialOpts.Timeout); err != nil {
			return errors.Annotate(err, "bootstrap instance started but did not change to Deployed state")
		}
		return finalizer(ctx, icfg, dialOpts)
	}

	bsResult := &environs.BootstrapResult{
		Arch:                    *result.Hardware.Arch,
		Series:                  series,
		CloudBootstrapFinalizer: waitingFinalizer,
	}
	return bsResult, nil
}

// ControllerInstances is specified in the Environ interface.
func (env *maasEnviron) ControllerInstances(ctx context.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	if !env.usingMAAS2() {
		return env.controllerInstances1(ctx, controllerUUID)
	}
	return env.controllerInstances2(ctx, controllerUUID)
}

func (env *maasEnviron) controllerInstances1(ctx context.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	return common.ProviderStateInstances(env.Storage())
}

func (env *maasEnviron) controllerInstances2(ctx context.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	instances, err := env.instances2(ctx, gomaasapi.MachinesArgs{
		OwnerData: map[string]string{
			tags.JujuIsController: "true",
			tags.JujuController:   controllerUUID,
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(instances) == 0 {
		return nil, environs.ErrNotBootstrapped
	}
	ids := make([]instance.Id, len(instances))
	for i := range instances {
		ids[i] = instances[i].Id()
	}
	return ids, nil
}

// ecfg returns the environment's maasModelConfig, and protects it with a
// mutex.
func (env *maasEnviron) ecfg() *maasModelConfig {
	env.ecfgMutex.Lock()
	cfg := *env.ecfgUnlocked
	env.ecfgMutex.Unlock()
	return &cfg
}

// Config is specified in the Environ interface.
func (env *maasEnviron) Config() *config.Config {
	return env.ecfg().Config
}

// SetConfig is specified in the Environ interface.
func (env *maasEnviron) SetConfig(cfg *config.Config) error {
	env.ecfgMutex.Lock()
	defer env.ecfgMutex.Unlock()

	// The new config has already been validated by itself, but now we
	// validate the transition from the old config to the new.
	var oldCfg *config.Config
	if env.ecfgUnlocked != nil {
		oldCfg = env.ecfgUnlocked.Config
	}
	cfg, err := env.Provider().Validate(cfg, oldCfg)
	if err != nil {
		return errors.Trace(err)
	}

	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return errors.Trace(err)
	}

	env.ecfgUnlocked = ecfg

	return nil
}

// SetCloudSpec is specified in the environs.Environ interface.
func (env *maasEnviron) SetCloudSpec(spec environs.CloudSpec) error {
	env.ecfgMutex.Lock()
	defer env.ecfgMutex.Unlock()

	maasServer, err := parseCloudEndpoint(spec.Endpoint)
	if err != nil {
		return errors.Trace(err)
	}
	maasOAuth, err := parseOAuthToken(*spec.Credential)
	if err != nil {
		return errors.Trace(err)
	}

	// We need to know the version of the server we're on. We support 1.9
	// and 2.0. MAAS 1.9 uses the 1.0 api version and 2.0 uses the 2.0 api
	// version.
	apiVersion := apiVersion2
	controller, err := GetMAAS2Controller(maasServer, maasOAuth)
	switch {
	case gomaasapi.IsUnsupportedVersionError(err):
		apiVersion = apiVersion1
		_, _, includesVersion := gomaasapi.SplitVersionedURL(maasServer)
		versionURL := maasServer
		if !includesVersion {
			versionURL = gomaasapi.AddAPIVersionToURL(maasServer, apiVersion1)
		}
		authClient, err := gomaasapi.NewAuthenticatedClient(versionURL, maasOAuth)
		if err != nil {
			return errors.Trace(err)
		}
		env.maasClientUnlocked = gomaasapi.NewMAAS(*authClient)
		caps, err := env.GetCapabilities(env.maasClientUnlocked, maasServer)
		if err != nil {
			return errors.Trace(err)
		}
		if !caps.Contains(capNetworkDeploymentUbuntu) {
			return errors.NewNotSupported(nil, "MAAS 1.9 or more recent is required")
		}
	case err != nil:
		return errors.Trace(err)
	default:
		env.maasController = controller
	}
	env.apiVersion = apiVersion
	env.storageUnlocked = NewStorage(env)

	return nil
}

func (env *maasEnviron) getSupportedArchitectures(ctx context.ProviderCallContext) ([]string, error) {
	env.archMutex.Lock()
	defer env.archMutex.Unlock()
	fetchArchitectures := env.allArchitecturesWithFallback
	if env.usingMAAS2() {
		fetchArchitectures = env.allArchitectures2
	}
	return fetchArchitectures(ctx)
}

// SupportsSpaces is specified on environs.Networking.
func (env *maasEnviron) SupportsSpaces(ctx context.ProviderCallContext) (bool, error) {
	return true, nil
}

// SupportsSpaceDiscovery is specified on environs.Networking.
func (env *maasEnviron) SupportsSpaceDiscovery(ctx context.ProviderCallContext) (bool, error) {
	return true, nil
}

// SupportsContainerAddresses is specified on environs.Networking.
func (env *maasEnviron) SupportsContainerAddresses(ctx context.ProviderCallContext) (bool, error) {
	return true, nil
}

// allArchitectures2 uses the MAAS2 controller to get architectures from boot
// resources.
func (env *maasEnviron) allArchitectures2(ctx context.ProviderCallContext) ([]string, error) {
	resources, err := env.maasController.BootResources()
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nil, errors.Trace(err)
	}
	architectures := set.NewStrings()
	for _, resource := range resources {
		architectures.Add(strings.Split(resource.Architecture(), "/")[0])
	}
	return architectures.SortedValues(), nil
}

// allArchitectureWithFallback queries MAAS for all of the boot-images
// across all registered nodegroups and collapses them down to unique
// architectures.
func (env *maasEnviron) allArchitecturesWithFallback(ctx context.ProviderCallContext) ([]string, error) {
	architectures, err := env.allArchitectures(ctx)
	if err != nil || len(architectures) == 0 {
		logger.Debugf("error querying boot-images: %v", err)
		logger.Debugf("falling back to listing nodes")
		architectures, err := env.nodeArchitectures(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return architectures, nil
	} else {
		return architectures, nil
	}
}

func (env *maasEnviron) allArchitectures(ctx context.ProviderCallContext) ([]string, error) {
	nodegroups, err := env.getNodegroups(ctx)
	if err != nil {
		return nil, err
	}
	architectures := set.NewStrings()
	for _, nodegroup := range nodegroups {
		bootImages, err := env.nodegroupBootImages(ctx, nodegroup)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot get boot images for nodegroup %v", nodegroup)
		}
		for _, image := range bootImages {
			architectures.Add(image.architecture)
		}
	}
	return architectures.SortedValues(), nil
}

// getNodegroups returns the UUID corresponding to each nodegroup
// in the MAAS installation.
func (env *maasEnviron) getNodegroups(ctx context.ProviderCallContext) ([]string, error) {
	nodegroupsListing := env.getMAASClient().GetSubObject("nodegroups")
	nodegroupsResult, err := nodegroupsListing.CallGet("list", nil)
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nil, err
	}
	list, err := nodegroupsResult.GetArray()
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nil, err
	}
	nodegroups := make([]string, len(list))
	for i, obj := range list {
		nodegroup, err := obj.GetMap()
		if err != nil {
			return nil, err
		}
		uuid, err := nodegroup["uuid"].GetString()
		if err != nil {
			return nil, err
		}
		nodegroups[i] = uuid
	}
	return nodegroups, nil
}

type bootImage struct {
	architecture string
	release      string
}

// nodegroupBootImages returns the set of boot-images for the specified nodegroup.
func (env *maasEnviron) nodegroupBootImages(ctx context.ProviderCallContext, nodegroupUUID string) ([]bootImage, error) {
	nodegroupObject := env.getMAASClient().GetSubObject("nodegroups").GetSubObject(nodegroupUUID)
	bootImagesObject := nodegroupObject.GetSubObject("boot-images/")
	result, err := bootImagesObject.CallGet("", nil)
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nil, err
	}
	list, err := result.GetArray()
	if err != nil {
		return nil, err
	}
	var bootImages []bootImage
	for _, obj := range list {
		bootimage, err := obj.GetMap()
		if err != nil {
			return nil, err
		}
		arch, err := bootimage["architecture"].GetString()
		if err != nil {
			return nil, err
		}
		release, err := bootimage["release"].GetString()
		if err != nil {
			return nil, err
		}
		bootImages = append(bootImages, bootImage{
			architecture: arch,
			release:      release,
		})
	}
	return bootImages, nil
}

// nodeArchitectures returns the architectures of all
// available nodes in the system.
//
// Note: this should only be used if we cannot query
// boot-images.
func (env *maasEnviron) nodeArchitectures(ctx context.ProviderCallContext) ([]string, error) {
	filter := make(url.Values)
	filter.Add("status", gomaasapi.NodeStatusDeclared)
	filter.Add("status", gomaasapi.NodeStatusCommissioning)
	filter.Add("status", gomaasapi.NodeStatusReady)
	filter.Add("status", gomaasapi.NodeStatusReserved)
	filter.Add("status", gomaasapi.NodeStatusAllocated)
	// This is fine - nodeArchitectures is only used in MAAS 1 cases.
	allInstances, err := env.instances1(ctx, filter)
	if err != nil {
		return nil, err
	}
	architectures := make(set.Strings)
	for _, inst := range allInstances {
		inst := inst.(*maas1Instance)
		arch, _, err := inst.architecture()
		if err != nil {
			return nil, err
		}
		architectures.Add(arch)
	}
	// TODO(dfc) why is this sorted
	return architectures.SortedValues(), nil
}

type maasAvailabilityZone struct {
	name string
}

func (z maasAvailabilityZone) Name() string {
	return z.name
}

func (z maasAvailabilityZone) Available() bool {
	// MAAS' physical zone attributes only include name and description;
	// there is no concept of availability.
	return true
}

// AvailabilityZones returns a slice of availability zones
// for the configured region.
func (env *maasEnviron) AvailabilityZones(ctx context.ProviderCallContext) ([]common.AvailabilityZone, error) {
	env.availabilityZonesMutex.Lock()
	defer env.availabilityZonesMutex.Unlock()
	if env.availabilityZones == nil {
		var availabilityZones []common.AvailabilityZone
		var err error
		if env.usingMAAS2() {
			availabilityZones, err = env.availabilityZones2(ctx)
			if err != nil {
				return nil, errors.Trace(err)
			}
		} else {
			availabilityZones, err = env.availabilityZones1(ctx)
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		env.availabilityZones = availabilityZones
	}
	return env.availabilityZones, nil
}

func (env *maasEnviron) availabilityZones1(ctx context.ProviderCallContext) ([]common.AvailabilityZone, error) {
	zonesObject := env.getMAASClient().GetSubObject("zones")
	result, err := zonesObject.CallGet("", nil)
	if err, ok := errors.Cause(err).(gomaasapi.ServerError); ok && err.StatusCode == http.StatusNotFound {
		return nil, errors.NewNotImplemented(nil, "the MAAS server does not support zones")
	}
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nil, errors.Annotate(err, "cannot query ")
	}
	list, err := result.GetArray()
	if err != nil {
		return nil, err
	}
	logger.Debugf("availability zones: %+v", list)
	availabilityZones := make([]common.AvailabilityZone, len(list))
	for i, obj := range list {
		zone, err := obj.GetMap()
		if err != nil {
			return nil, err
		}
		name, err := zone["name"].GetString()
		if err != nil {
			return nil, err
		}
		availabilityZones[i] = maasAvailabilityZone{name}
	}
	return availabilityZones, nil
}

func (env *maasEnviron) availabilityZones2(ctx context.ProviderCallContext) ([]common.AvailabilityZone, error) {
	zones, err := env.maasController.Zones()
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nil, errors.Trace(err)
	}
	availabilityZones := make([]common.AvailabilityZone, len(zones))
	for i, zone := range zones {
		availabilityZones[i] = maasAvailabilityZone{zone.Name()}
	}
	return availabilityZones, nil

}

// InstanceAvailabilityZoneNames returns the availability zone names for each
// of the specified instances.
func (env *maasEnviron) InstanceAvailabilityZoneNames(ctx context.ProviderCallContext, ids []instance.Id) ([]string, error) {
	instances, err := env.Instances(ctx, ids)
	if err != nil && err != environs.ErrPartialInstances {
		return nil, err
	}
	zones := make([]string, len(instances))
	for i, inst := range instances {
		if inst == nil {
			continue
		}
		z, err := inst.(maasInstance).zone()
		if err != nil {
			logger.Errorf("could not get availability zone %v", err)
			continue
		}
		zones[i] = z
	}
	return zones, nil
}

// DeriveAvailabilityZones is part of the common.ZonedEnviron interface.
func (env *maasEnviron) DeriveAvailabilityZones(ctx context.ProviderCallContext, args environs.StartInstanceParams) ([]string, error) {
	if args.Placement != "" {
		placement, err := env.parsePlacement(ctx, args.Placement)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if placement.zoneName != "" {
			return []string{placement.zoneName}, nil
		}
	}
	return nil, nil
}

type maasPlacement struct {
	nodeName string
	zoneName string
	systemId string
}

func (env *maasEnviron) parsePlacement(ctx context.ProviderCallContext, placement string) (*maasPlacement, error) {
	pos := strings.IndexRune(placement, '=')
	if pos == -1 {
		// If there's no '=' delimiter, assume it's a node name.
		return &maasPlacement{nodeName: placement}, nil
	}
	switch key, value := placement[:pos], placement[pos+1:]; key {
	case "zone":
		availabilityZone := value
		err := common.ValidateAvailabilityZone(env, ctx, availabilityZone)
		if err != nil {
			return nil, err
		}
		return &maasPlacement{zoneName: availabilityZone}, nil
	case "system-id":
		return &maasPlacement{systemId: value}, nil
	}

	return nil, errors.Errorf("unknown placement directive: %v", placement)
}

func (env *maasEnviron) PrecheckInstance(ctx context.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	if args.Placement == "" {
		return nil
	}
	_, err := env.parsePlacement(ctx, args.Placement)
	return err
}

const (
	capNetworkDeploymentUbuntu = "network-deployment-ubuntu"
)

// getCapabilities asks the MAAS server for its capabilities, if
// supported by the server.
func getCapabilities(client *gomaasapi.MAASObject, serverURL string) (set.Strings, error) {
	caps := make(set.Strings)
	var result gomaasapi.JSONObject
	var err error

	for a := shortAttempt.Start(); a.Next(); {
		version := client.GetSubObject("version/")
		result, err = version.CallGet("", nil)
		if err == nil {
			break
		}
		if err, ok := errors.Cause(err).(gomaasapi.ServerError); ok && err.StatusCode == 404 {
			logger.Debugf("Failed attempting to get capabilities from maas endpoint %q: %v", serverURL, err)

			message := "could not connect to MAAS controller - check the endpoint is correct"
			trimmedURL := strings.TrimRight(serverURL, "/")
			if !strings.HasSuffix(trimmedURL, "/MAAS") {
				message += " (it normally ends with /MAAS)"
			}
			return caps, errors.NewNotSupported(nil, message)
		}
	}
	if err != nil {
		logger.Debugf("Can't connect to maas server at endpoint %q: %v", serverURL, err)
		return caps, err
	}
	info, err := result.GetMap()
	if err != nil {
		logger.Debugf("Invalid data returned from maas endpoint %q: %v", serverURL, err)
		// invalid data of some sort, probably not a MAAS server.
		return caps, errors.New("failed to get expected data from server")
	}
	capsObj, ok := info["capabilities"]
	if !ok {
		return caps, fmt.Errorf("MAAS does not report capabilities")
	}
	items, err := capsObj.GetArray()
	if err != nil {
		logger.Debugf("Invalid data returned from maas endpoint %q: %v", serverURL, err)
		return caps, errors.New("failed to get expected data from server")
	}
	for _, item := range items {
		val, err := item.GetString()
		if err != nil {
			logger.Debugf("Invalid data returned from maas endpoint %q: %v", serverURL, err)
			return set.NewStrings(), errors.New("failed to get expected data from server")
		}
		caps.Add(val)
	}
	return caps, nil
}

// getMAASClient returns a MAAS client object to use for a request, in a
// lock-protected fashion.
func (env *maasEnviron) getMAASClient() *gomaasapi.MAASObject {
	env.ecfgMutex.Lock()
	defer env.ecfgMutex.Unlock()

	return env.maasClientUnlocked
}

var dashSuffix = regexp.MustCompile("^(.*)-\\d+$")

func spaceNamesToSpaceInfo(
	spaces []string, spaceMap map[string]corenetwork.SpaceInfo,
) ([]corenetwork.SpaceInfo, error) {
	var spaceInfos []corenetwork.SpaceInfo
	for _, name := range spaces {
		info, ok := spaceMap[name]
		if !ok {
			matches := dashSuffix.FindAllStringSubmatch(name, 1)
			if matches == nil {
				return nil, errors.Errorf("unrecognised space in constraint %q", name)
			}
			// A -number was added to the space name when we
			// converted to a juju name, we found
			info, ok = spaceMap[matches[0][1]]
			if !ok {
				return nil, errors.Errorf("unrecognised space in constraint %q", name)
			}
		}
		spaceInfos = append(spaceInfos, info)
	}
	return spaceInfos, nil
}

func (env *maasEnviron) buildSpaceMap(ctx context.ProviderCallContext) (map[string]corenetwork.SpaceInfo, error) {
	spaces, err := env.Spaces(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	spaceMap := make(map[string]corenetwork.SpaceInfo)
	empty := set.Strings{}
	for _, space := range spaces {
		jujuName := network.ConvertSpaceName(space.Name, empty)
		spaceMap[jujuName] = space
	}
	return spaceMap, nil
}

func (env *maasEnviron) spaceNamesToSpaceInfo(
	ctx context.ProviderCallContext, positiveSpaces, negativeSpaces []string,
) ([]corenetwork.SpaceInfo, []corenetwork.SpaceInfo, error) {
	spaceMap, err := env.buildSpaceMap(ctx)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	positiveSpaceIds, err := spaceNamesToSpaceInfo(positiveSpaces, spaceMap)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	negativeSpaceIds, err := spaceNamesToSpaceInfo(negativeSpaces, spaceMap)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return positiveSpaceIds, negativeSpaceIds, nil
}

// acquireNode2 allocates a machine from MAAS2.
func (env *maasEnviron) acquireNode2(
	ctx context.ProviderCallContext,
	nodeName, zoneName, systemId string,
	cons constraints.Value,
	interfaces []interfaceBinding,
	volumes []volumeInfo,
) (maasInstance, error) {
	acquireParams := convertConstraints2(cons)
	positiveSpaceNames, negativeSpaceNames := convertSpacesFromConstraints(cons.Spaces)
	positiveSpaces, negativeSpaces, err := env.spaceNamesToSpaceInfo(ctx, positiveSpaceNames, negativeSpaceNames)
	// If spaces aren't supported the constraints should be empty anyway.
	if err != nil && !errors.IsNotSupported(err) {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nil, errors.Trace(err)
	}
	err = addInterfaces2(&acquireParams, interfaces, positiveSpaces, negativeSpaces)
	if err != nil {
		return nil, errors.Trace(err)
	}
	addStorage2(&acquireParams, volumes)
	acquireParams.AgentName = env.uuid
	if zoneName != "" {
		acquireParams.Zone = zoneName
	}
	if nodeName != "" {
		acquireParams.Hostname = nodeName
	}
	if systemId != "" {
		acquireParams.SystemId = systemId
	}
	machine, constraintMatches, err := env.maasController.AllocateMachine(acquireParams)

	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nil, errors.Trace(err)
	}
	return &maas2Instance{
		machine:           machine,
		constraintMatches: constraintMatches,
		environ:           env,
	}, nil
}

// acquireNode allocates a node from the MAAS.
func (env *maasEnviron) acquireNode(
	ctx context.ProviderCallContext,
	nodeName, zoneName, systemId string,
	cons constraints.Value,
	interfaces []interfaceBinding,
	volumes []volumeInfo,
) (gomaasapi.MAASObject, error) {

	// TODO(axw) 2014-08-18 #1358219
	// We should be requesting preferred architectures if unspecified,
	// like in the other providers.
	//
	// This is slightly complicated in MAAS as there are a finite
	// number of each architecture; preference may also conflict with
	// other constraints, such as tags. Thus, a preference becomes a
	// demand (which may fail) if not handled properly.

	acquireParams := convertConstraints(cons)
	positiveSpaceNames, negativeSpaceNames := convertSpacesFromConstraints(cons.Spaces)
	positiveSpaces, negativeSpaces, err := env.spaceNamesToSpaceInfo(ctx, positiveSpaceNames, negativeSpaceNames)
	// If spaces aren't supported the constraints should be empty anyway.
	if err != nil && !errors.IsNotSupported(err) {
		return gomaasapi.MAASObject{}, errors.Trace(err)
	}
	err = addInterfaces(acquireParams, interfaces, positiveSpaces, negativeSpaces)
	if err != nil {
		return gomaasapi.MAASObject{}, errors.Trace(err)
	}
	addStorage(acquireParams, volumes)
	acquireParams.Add("agent_name", env.uuid)
	if zoneName != "" {
		acquireParams.Add("zone", zoneName)
	}
	if nodeName != "" {
		acquireParams.Add("name", nodeName)
	}
	if systemId != "" {
		acquireParams.Add("system_id", systemId)
	}

	var result gomaasapi.JSONObject
	for a := shortAttempt.Start(); a.Next(); {
		client := env.getMAASClient().GetSubObject("nodes/")
		logger.Tracef("calling acquire with params: %+v", acquireParams)
		result, err = client.CallPost("acquire", acquireParams)
		if err == nil {
			break
		}
	}
	if err != nil {
		return gomaasapi.MAASObject{}, err
	}
	node, err := result.GetMAASObject()
	if err != nil {
		err := errors.Annotate(err, "unexpected result from 'acquire' on MAAS API")
		return gomaasapi.MAASObject{}, err
	}
	return node, nil
}

// startNode installs and boots a node.
func (env *maasEnviron) startNode(node gomaasapi.MAASObject, series string, userdata []byte) (*gomaasapi.MAASObject, error) {
	params := url.Values{
		"distro_series": {series},
		"user_data":     {string(userdata)},
	}
	// Initialize err to a non-nil value as a sentinel for the following
	// loop.
	err := fmt.Errorf("(no error)")
	var result gomaasapi.JSONObject
	for a := shortAttempt.Start(); a.Next() && err != nil; {
		result, err = node.CallPost("start", params)
		if err == nil {
			break
		}
	}

	if err == nil {
		var startedNode gomaasapi.MAASObject
		startedNode, err = result.GetMAASObject()
		if err != nil {
			logger.Errorf("cannot process API response after successfully starting node: %v", err)
			return nil, err
		}
		return &startedNode, nil
	}
	return nil, err
}

func (env *maasEnviron) startNode2(node maas2Instance, series string, userdata []byte) (*maas2Instance, error) {
	err := node.machine.Start(gomaasapi.StartArgs{DistroSeries: series, UserData: string(userdata)})
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Machine.Start updates the machine in-place when it succeeds.
	return &maas2Instance{machine: node.machine}, nil

}

// DistributeInstances implements the state.InstanceDistributor policy.
func (env *maasEnviron) DistributeInstances(
	ctx context.ProviderCallContext, candidates, distributionGroup []instance.Id, limitZones []string,
) ([]instance.Id, error) {
	return common.DistributeInstances(env, ctx, candidates, distributionGroup, limitZones)
}

var availabilityZoneAllocations = common.AvailabilityZoneAllocations

// MaintainInstance is specified in the InstanceBroker interface.
func (*maasEnviron) MaintainInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) error {
	return nil
}

// StartInstance is specified in the InstanceBroker interface.
func (env *maasEnviron) StartInstance(
	ctx context.ProviderCallContext,
	args environs.StartInstanceParams,
) (_ *environs.StartInstanceResult, err error) {

	availabilityZone := args.AvailabilityZone
	var nodeName, systemId string
	if args.Placement != "" {
		placement, err := env.parsePlacement(ctx, args.Placement)
		if err != nil {
			return nil, common.ZoneIndependentError(err)
		}
		// NOTE(axw) we wipe out args.AvailabilityZone if the
		// user specified a specific node or system ID via
		// placement, as placement must always take precedence.
		switch {
		case placement.systemId != "":
			availabilityZone = ""
			systemId = placement.systemId
		case placement.nodeName != "":
			availabilityZone = ""
			nodeName = placement.nodeName
		}
	}
	if availabilityZone != "" {
		if err := common.ValidateAvailabilityZone(env, ctx, availabilityZone); err != nil {
			return nil, errors.Trace(err)
		}
		logger.Debugf("attempting to acquire node in zone %q", availabilityZone)
	}

	// Storage.
	volumes, err := buildMAASVolumeParameters(args.Volumes, args.Constraints)
	if err != nil {
		return nil, common.ZoneIndependentError(errors.Annotate(err, "invalid volume parameters"))
	}

	var interfaceBindings []interfaceBinding
	if len(args.EndpointBindings) != 0 {
		for endpoint, spaceProviderID := range args.EndpointBindings {
			interfaceBindings = append(interfaceBindings, interfaceBinding{
				Name:            endpoint,
				SpaceProviderId: string(spaceProviderID),
			})
		}
	}
	selectNode := env.selectNode2
	if !env.usingMAAS2() {
		selectNode = env.selectNode
	}
	inst, selectNodeErr := selectNode(ctx,
		selectNodeArgs{
			Constraints:      args.Constraints,
			AvailabilityZone: availabilityZone,
			NodeName:         nodeName,
			SystemId:         systemId,
			Interfaces:       interfaceBindings,
			Volumes:          volumes,
		})
	if selectNodeErr != nil {
		err := errors.Annotate(selectNodeErr, "failed to acquire node")
		if selectNodeErr.noMatch && availabilityZone != "" {
			// The error was due to MAAS not being able to
			// find provide a machine matching the specified
			// constraints in the zone; try again in another.
			return nil, errors.Trace(err)
		}
		return nil, common.ZoneIndependentError(err)
	}

	defer func() {
		if err != nil {
			if err := env.StopInstances(ctx, inst.Id()); err != nil {
				logger.Errorf("error releasing failed instance: %v", err)
			}
		}
	}()

	hc, err := inst.hardwareCharacteristics()
	if err != nil {
		return nil, common.ZoneIndependentError(err)
	}

	series := args.Tools.OneSeries()
	selectedTools, err := args.Tools.Match(tools.Filter{
		Arch: *hc.Arch,
	})
	if err != nil {
		return nil, common.ZoneIndependentError(err)
	}
	if err := args.InstanceConfig.SetTools(selectedTools); err != nil {
		return nil, common.ZoneIndependentError(err)
	}

	hostname, err := inst.hostname()
	if err != nil {
		return nil, common.ZoneIndependentError(err)
	}

	if err := instancecfg.FinishInstanceConfig(args.InstanceConfig, env.Config()); err != nil {
		return nil, common.ZoneIndependentError(err)
	}

	subnetsMap, err := env.subnetToSpaceIds(ctx)
	if err != nil {
		return nil, common.ZoneIndependentError(err)
	}

	cloudcfg, err := env.newCloudinitConfig(hostname, series)
	if err != nil {
		return nil, common.ZoneIndependentError(err)
	}

	userdata, err := providerinit.ComposeUserData(args.InstanceConfig, cloudcfg, MAASRenderer{})
	if err != nil {
		return nil, common.ZoneIndependentError(errors.Annotate(
			err, "could not compose userdata for bootstrap node",
		))
	}
	logger.Debugf("maas user data; %d bytes", len(userdata))

	var displayName string
	var interfaces []network.InterfaceInfo
	if !env.usingMAAS2() {
		inst1 := inst.(*maas1Instance)
		startedNode, err := env.startNode(*inst1.maasObject, series, userdata)
		if err != nil {
			return nil, common.ZoneIndependentError(err)
		}
		// Once the instance has started the response should contain the
		// assigned IP addresses, even when NICs are set to "auto" instead of
		// "static". So instead of selectedNode, which only contains the
		// acquire-time details (no IP addresses for NICs set to "auto" vs
		// "static"),e we use the up-to-date startedNode response to get the
		// interfaces.
		interfaces, err = maasObjectNetworkInterfaces(ctx, startedNode, subnetsMap)
		if err != nil {
			return nil, common.ZoneIndependentError(err)
		}
		env.tagInstance1(inst1, args.InstanceConfig)
		displayName, err = inst1.displayName()
		if err != nil {
			return nil, common.ZoneIndependentError(err)
		}
	} else {
		inst2 := inst.(*maas2Instance)
		startedInst, err := env.startNode2(*inst2, series, userdata)
		if err != nil {
			return nil, common.ZoneIndependentError(err)
		}
		domains, err := env.Domains(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		interfaces, err = maas2NetworkInterfaces(ctx, startedInst, subnetsMap, domains...)
		if err != nil {
			return nil, common.ZoneIndependentError(err)
		}
		env.tagInstance2(inst2, args.InstanceConfig)

		displayName, err = inst2.displayName()
		if err != nil {
			return nil, common.ZoneIndependentError(err)
		}
	}
	logger.Debugf("started instance %q", inst.Id())

	requestedVolumes := make([]names.VolumeTag, len(args.Volumes))
	for i, v := range args.Volumes {
		requestedVolumes[i] = v.Tag
	}
	resultVolumes, resultAttachments, err := inst.volumes(
		names.NewMachineTag(args.InstanceConfig.MachineId),
		requestedVolumes,
	)
	if err != nil {
		return nil, common.ZoneIndependentError(err)
	}
	if len(resultVolumes) != len(requestedVolumes) {
		return nil, common.ZoneIndependentError(errors.Errorf(
			"requested %v storage volumes. %v returned",
			len(requestedVolumes), len(resultVolumes),
		))
	}

	return &environs.StartInstanceResult{
		DisplayName:       displayName,
		Instance:          inst,
		Hardware:          hc,
		NetworkInfo:       interfaces,
		Volumes:           resultVolumes,
		VolumeAttachments: resultAttachments,
	}, nil
}

func instanceConfiguredInterfaceNames(
	ctx context.ProviderCallContext, usingMAAS2 bool, inst instances.Instance, subnetsMap map[string]corenetwork.Id,
) ([]string, error) {
	var (
		interfaces []network.InterfaceInfo
		err        error
	)
	if !usingMAAS2 {
		inst1 := inst.(*maas1Instance)
		interfaces, err = maasObjectNetworkInterfaces(ctx, inst1.maasObject, subnetsMap)
		if err != nil {
			return nil, errors.Trace(err)
		}
	} else {
		inst2 := inst.(*maas2Instance)
		interfaces, err = maas2NetworkInterfaces(ctx, inst2, subnetsMap)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	nameToNumAliases := make(map[string]int)
	var linkedNames []string
	for _, iface := range interfaces {
		if iface.CIDR == "" { // CIDR comes from a linked subnet.
			continue
		}

		switch iface.ConfigType {
		case network.ConfigUnknown, network.ConfigManual:
			continue // link is unconfigured
		}

		finalName := iface.InterfaceName
		numAliases, seen := nameToNumAliases[iface.InterfaceName]
		if !seen {
			nameToNumAliases[iface.InterfaceName] = 0
		} else {
			numAliases++ // aliases start from 1
			finalName += fmt.Sprintf(":%d", numAliases)
			nameToNumAliases[iface.InterfaceName] = numAliases
		}

		linkedNames = append(linkedNames, finalName)
	}
	systemID := extractSystemId(inst.Id())
	logger.Infof("interface names to bridge for node %q: %v", systemID, linkedNames)

	return linkedNames, nil
}

func (env *maasEnviron) tagInstance1(inst *maas1Instance, instanceConfig *instancecfg.InstanceConfig) {
	if !multiwatcher.AnyJobNeedsState(instanceConfig.Jobs...) {
		return
	}
	err := common.AddStateInstance(env.Storage(), inst.Id())
	if err != nil {
		logger.Errorf("could not record instance in provider-state: %v", err)
	}
}

func (env *maasEnviron) tagInstance2(inst *maas2Instance, instanceConfig *instancecfg.InstanceConfig) {
	err := inst.machine.SetOwnerData(instanceConfig.Tags)
	if err != nil {
		logger.Errorf("could not set owner data for instance: %v", err)
	}
}

func (env *maasEnviron) waitForNodeDeployment(ctx context.ProviderCallContext, id instance.Id, timeout time.Duration) error {
	if env.usingMAAS2() {
		return env.waitForNodeDeployment2(ctx, id, timeout)
	}
	systemId := extractSystemId(id)

	longAttempt := utils.AttemptStrategy{
		Delay: 10 * time.Second,
		Total: timeout,
	}

	for a := longAttempt.Start(); a.Next(); {
		statusValues, err := env.deploymentStatus(ctx, id)
		if errors.IsNotImplemented(err) {
			return nil
		}
		if err != nil {
			common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
			return errors.Trace(err)
		}
		if statusValues[systemId] == "Deployed" {
			return nil
		}
		if statusValues[systemId] == "Failed deployment" {
			return errors.Errorf("instance %q failed to deploy", id)
		}
	}
	return errors.Errorf("instance %q is started but not deployed", id)
}

func (env *maasEnviron) waitForNodeDeployment2(ctx context.ProviderCallContext, id instance.Id, timeout time.Duration) error {
	// TODO(katco): 2016-08-09: lp:1611427
	longAttempt := utils.AttemptStrategy{
		Delay: 10 * time.Second,
		Total: timeout,
	}

	retryCount := 1
	for a := longAttempt.Start(); a.Next(); {
		machine, err := env.getInstance(ctx, id)
		if err != nil {
			logger.Warningf("failed to get instance from provider attempt %d", retryCount)
			if denied := common.MaybeHandleCredentialError(IsAuthorisationFailure, err, ctx); denied {
				break
			}

			retryCount++
			continue
		}
		stat := machine.Status(ctx)
		if stat.Status == status.Running {
			return nil
		}
		if stat.Status == status.ProvisioningError {
			return errors.Errorf("instance %q failed to deploy", id)

		}
	}
	return errors.Errorf("instance %q is started but not deployed", id)
}

func (env *maasEnviron) deploymentStatusOne(ctx context.ProviderCallContext, id instance.Id) (string, string) {
	results, err := env.deploymentStatus(ctx, id)
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return "", ""
	}
	systemId := extractSystemId(id)
	substatus := env.getDeploymentSubstatus(ctx, systemId)
	return results[systemId], substatus
}

func (env *maasEnviron) getDeploymentSubstatus(ctx context.ProviderCallContext, systemId string) string {
	nodesAPI := env.getMAASClient().GetSubObject("nodes")
	result, err := nodesAPI.CallGet("list", nil)
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return ""
	}
	slices, err := result.GetArray()
	if err != nil {
		return ""
	}
	for _, slice := range slices {
		resultMap, err := slice.GetMap()
		if err != nil {
			continue
		}
		sysId, err := resultMap["system_id"].GetString()
		if err != nil {
			continue
		}
		if sysId == systemId {
			message, err := resultMap["substatus_message"].GetString()
			if err != nil {
				logger.Warningf("could not get string for substatus_message: %v", resultMap["substatus_message"])
				return ""
			}
			return message
		}
	}

	return ""
}

// deploymentStatus returns the deployment state of MAAS instances with
// the specified Juju instance ids.
// Note: the result is a map of MAAS systemId to state.
func (env *maasEnviron) deploymentStatus(ctx context.ProviderCallContext, ids ...instance.Id) (map[string]string, error) {
	nodesAPI := env.getMAASClient().GetSubObject("nodes")
	result, err := DeploymentStatusCall(nodesAPI, ids...)
	if err != nil {
		if err, ok := errors.Cause(err).(gomaasapi.ServerError); ok && err.StatusCode == http.StatusBadRequest {
			return nil, errors.NewNotImplemented(err, "deployment status")
		}
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nil, errors.Trace(err)
	}
	resultMap, err := result.GetMap()
	if err != nil {
		return nil, errors.Trace(err)
	}
	statusValues := make(map[string]string)
	for systemId, jsonValue := range resultMap {
		status, err := jsonValue.GetString()
		if err != nil {
			return nil, errors.Trace(err)
		}
		statusValues[systemId] = status
	}
	return statusValues, nil
}

func deploymentStatusCall(nodes gomaasapi.MAASObject, ids ...instance.Id) (gomaasapi.JSONObject, error) {
	filter := getSystemIdValues("nodes", ids)
	return nodes.CallGet("deployment_status", filter)
}

type selectNodeArgs struct {
	AvailabilityZone string
	NodeName         string
	SystemId         string
	Constraints      constraints.Value
	Interfaces       []interfaceBinding
	Volumes          []volumeInfo
}

type selectNodeError struct {
	error
	noMatch bool
}

func (env *maasEnviron) selectNode(ctx context.ProviderCallContext, args selectNodeArgs) (maasInstance, *selectNodeError) {
	node, err := env.acquireNode(
		ctx,
		args.NodeName,
		args.AvailabilityZone,
		args.SystemId,
		args.Constraints,
		args.Interfaces,
		args.Volumes,
	)
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nil, &selectNodeError{
			error:   errors.Trace(err),
			noMatch: isConflictError(err),
		}
	}
	return &maas1Instance{
		maasObject:   &node,
		environ:      env,
		statusGetter: env.deploymentStatusOne,
	}, nil
}

func isConflictError(err error) bool {
	serverErr, ok := errors.Cause(err).(gomaasapi.ServerError)
	return ok && serverErr.StatusCode == http.StatusConflict
}

func (env *maasEnviron) selectNode2(ctx context.ProviderCallContext, args selectNodeArgs) (maasInstance, *selectNodeError) {
	inst, err := env.acquireNode2(
		ctx,
		args.NodeName,
		args.AvailabilityZone,
		args.SystemId,
		args.Constraints,
		args.Interfaces,
		args.Volumes,
	)
	if err != nil {
		return nil, &selectNodeError{
			error:   errors.Trace(err),
			noMatch: gomaasapi.IsNoMatchError(err),
		}
	}
	return inst, nil
}

// newCloudinitConfig creates a cloudinit.Config structure suitable as a base
// for initialising a MAAS node.
func (env *maasEnviron) newCloudinitConfig(hostname, forSeries string) (cloudinit.CloudConfig, error) {
	cloudcfg, err := cloudinit.New(forSeries)
	if err != nil {
		return nil, err
	}

	info := machineInfo{hostname}
	runCmd, err := info.cloudinitRunCmd(cloudcfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	operatingSystem, err := series.GetOSFromSeries(forSeries)
	if err != nil {
		return nil, errors.Trace(err)
	}
	switch operatingSystem {
	case os.Windows:
		cloudcfg.AddScripts(runCmd)
	case os.Ubuntu:
		cloudcfg.SetSystemUpdate(true)
		cloudcfg.AddScripts("set -xe", runCmd)
		// DisableNetworkManagement can still disable the bridge(s) creation.
		if on, set := env.Config().DisableNetworkManagement(); on && set {
			logger.Infof(
				"network management disabled - not using %q bridge for containers",
				instancecfg.DefaultBridgeName,
			)
			break
		}
		cloudcfg.AddPackage("bridge-utils")
	}
	return cloudcfg, nil
}

func (env *maasEnviron) releaseNodes1(ctx context.ProviderCallContext, nodes gomaasapi.MAASObject, ids url.Values, recurse bool) error {
	err := ReleaseNodes(nodes, ids)
	if err == nil {
		return nil
	}
	if denied := common.MaybeHandleCredentialError(IsAuthorisationFailure, err, ctx); denied {
		return errors.Annotate(err, "cannot release nodes")
	}
	maasErr, ok := gomaasapi.GetServerError(err)
	if !ok {
		return errors.Annotate(err, "cannot release nodes")
	}

	// StatusCode 409 means a node couldn't be released due to
	// a state conflict. Likely it's already released or disk
	// erasing. We're assuming an error of 409 *only* means it's
	// safe to assume the instance is already released.
	// MaaS also releases (or attempts) all nodes, and raises
	// a single error on failure. So even with an error 409, all
	// nodes have been released.
	if maasErr.StatusCode == 409 {
		logger.Infof("ignoring error while releasing nodes (%v); all nodes released OK", err)
		return nil
	}

	// a status code of 400, 403 or 404 means one of the nodes
	// couldn't be found and none have been released. We have
	// to release all the ones we can individually.
	if maasErr.StatusCode != 400 && maasErr.StatusCode != 403 && maasErr.StatusCode != 404 {
		return errors.Annotate(err, "cannot release nodes")
	}
	if !recurse {
		// this node has already been released and we're golden
		return nil
	}

	var lastErr error
	for _, id := range ids["nodes"] {
		idFilter := url.Values{}
		idFilter.Add("nodes", id)
		err := env.releaseNodes1(ctx, nodes, idFilter, false)
		if err != nil {
			lastErr = err
			logger.Errorf("error while releasing node %v (%v)", id, err)
			if denied := common.MaybeHandleCredentialError(IsAuthorisationFailure, err, ctx); denied {
				break
			}
		}
	}
	return errors.Trace(lastErr)

}

func (env *maasEnviron) releaseNodes2(ctx context.ProviderCallContext, ids []instance.Id, recurse bool) error {
	args := gomaasapi.ReleaseMachinesArgs{
		SystemIDs: instanceIdsToSystemIDs(ids),
		Comment:   "Released by Juju MAAS provider",
	}
	err := env.maasController.ReleaseMachines(args)

	denied := common.MaybeHandleCredentialError(IsAuthorisationFailure, err, ctx)
	switch {
	case err == nil:
		return nil
	case gomaasapi.IsCannotCompleteError(err):
		// CannotCompleteError means a node couldn't be released due to
		// a state conflict. Likely it's already released or disk
		// erasing. We're assuming this error *only* means it's
		// safe to assume the instance is already released.
		// MaaS also releases (or attempts) all nodes, and raises
		// a single error on failure. So even with an error 409, all
		// nodes have been released.
		logger.Infof("ignoring error while releasing nodes (%v); all nodes released OK", err)
		return nil
	case gomaasapi.IsBadRequestError(err), denied:
		// a status code of 400 or 403 means one of the nodes
		// couldn't be found and none have been released. We have to
		// release all the ones we can individually.
		if !recurse {
			// this node has already been released and we're golden
			return nil
		}
		return env.releaseNodesIndividually(ctx, ids)

	default:
		return errors.Annotatef(err, "cannot release nodes")
	}
}

func (env *maasEnviron) releaseNodesIndividually(ctx context.ProviderCallContext, ids []instance.Id) error {
	var lastErr error
	for _, id := range ids {
		err := env.releaseNodes2(ctx, []instance.Id{id}, false)
		if err != nil {
			lastErr = err
			logger.Errorf("error while releasing node %v (%v)", id, err)
			if denied := common.MaybeHandleCredentialError(IsAuthorisationFailure, err, ctx); denied {
				break
			}
		}
	}
	return errors.Trace(lastErr)
}

func instanceIdsToSystemIDs(ids []instance.Id) []string {
	systemIDs := make([]string, len(ids))
	for index, id := range ids {
		systemIDs[index] = string(id)
	}
	return systemIDs
}

// StopInstances is specified in the InstanceBroker interface.
func (env *maasEnviron) StopInstances(ctx context.ProviderCallContext, ids ...instance.Id) error {
	// Shortcut to exit quickly if 'instances' is an empty slice or nil.
	if len(ids) == 0 {
		return nil
	}

	if env.usingMAAS2() {
		err := env.releaseNodes2(ctx, ids, true)
		if err != nil {
			return errors.Trace(err)
		}
	} else {
		nodes := env.getMAASClient().GetSubObject("nodes")
		err := env.releaseNodes1(ctx, nodes, getSystemIdValues("nodes", ids), true)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return common.RemoveStateInstances(env.Storage(), ids...)

}

// Instances returns the instances.Instance objects corresponding to the given
// slice of instance.Id.  The error is ErrNoInstances if no instances
// were found.
func (env *maasEnviron) Instances(ctx context.ProviderCallContext, ids []instance.Id) ([]instances.Instance, error) {
	if len(ids) == 0 {
		// This would be treated as "return all instances" below, so
		// treat it as a special case.
		// The interface requires us to return this particular error
		// if no instances were found.
		return nil, environs.ErrNoInstances
	}
	acquired, err := env.acquiredInstances(ctx, ids)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(acquired) == 0 {
		return nil, environs.ErrNoInstances
	}

	idMap := make(map[instance.Id]instances.Instance)
	for _, instance := range acquired {
		idMap[instance.Id()] = instance
	}

	missing := false
	result := make([]instances.Instance, len(ids))
	for index, id := range ids {
		val, ok := idMap[id]
		if !ok {
			missing = true
			continue
		}
		result[index] = val
	}

	if missing {
		return result, environs.ErrPartialInstances
	}
	return result, nil
}

// acquireInstances calls the MAAS API to list acquired nodes.
//
// The "ids" slice is a filter for specific instance IDs.
// Due to how this works in the HTTP API, an empty "ids"
// matches all instances (not none as you might expect).
func (env *maasEnviron) acquiredInstances(ctx context.ProviderCallContext, ids []instance.Id) ([]instances.Instance, error) {
	if !env.usingMAAS2() {
		filter := getSystemIdValues("id", ids)
		filter.Add("agent_name", env.uuid)
		return env.instances1(ctx, filter)
	}
	args := gomaasapi.MachinesArgs{
		AgentName: env.uuid,
		SystemIDs: instanceIdsToSystemIDs(ids),
	}
	return env.instances2(ctx, args)
}

// instances calls the MAAS API to list nodes matching the given filter.
func (env *maasEnviron) instances1(ctx context.ProviderCallContext, filter url.Values) ([]instances.Instance, error) {
	nodeListing := env.getMAASClient().GetSubObject("nodes")
	listNodeObjects, err := nodeListing.CallGet("list", filter)
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nil, err
	}
	listNodes, err := listNodeObjects.GetArray()
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nil, err
	}
	instances := make([]instances.Instance, len(listNodes))
	for index, nodeObj := range listNodes {
		node, err := nodeObj.GetMAASObject()
		if err != nil {
			return nil, err
		}
		instances[index] = &maas1Instance{
			maasObject:   &node,
			environ:      env,
			statusGetter: env.deploymentStatusOne,
		}
	}
	return instances, nil
}

func (env *maasEnviron) instances2(ctx context.ProviderCallContext, args gomaasapi.MachinesArgs) ([]instances.Instance, error) {
	machines, err := env.maasController.Machines(args)
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nil, errors.Trace(err)
	}
	instances := make([]instances.Instance, len(machines))
	for index, machine := range machines {
		instances[index] = &maas2Instance{machine: machine, environ: env}
	}
	return instances, nil
}

// subnetsFromNode fetches all the subnets for a specific node.
func (env *maasEnviron) subnetsFromNode(ctx context.ProviderCallContext, nodeId string) ([]gomaasapi.JSONObject, error) {
	client := env.getMAASClient().GetSubObject("nodes").GetSubObject(nodeId)
	json, err := client.CallGet("", nil)
	if err != nil {
		if maasErr, ok := errors.Cause(err).(gomaasapi.ServerError); ok && maasErr.StatusCode == http.StatusNotFound {
			return nil, errors.NotFoundf("intance %q", nodeId)
		}
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nil, errors.Trace(err)
	}
	nodeMap, err := json.GetMap()
	if err != nil {
		return nil, errors.Trace(err)
	}
	interfacesArray, err := nodeMap["interface_set"].GetArray()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var subnets []gomaasapi.JSONObject
	for _, iface := range interfacesArray {
		ifaceMap, err := iface.GetMap()
		if err != nil {
			return nil, errors.Trace(err)
		}
		linksArray, err := ifaceMap["links"].GetArray()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, link := range linksArray {
			linkMap, err := link.GetMap()
			if err != nil {
				return nil, errors.Trace(err)
			}
			subnet, ok := linkMap["subnet"]
			if !ok {
				return nil, errors.New("subnet not found")
			}
			subnets = append(subnets, subnet)
		}
	}
	return subnets, nil
}

// subnetFromJson populates a network.SubnetInfo from a gomaasapi.JSONObject
// representing a single subnet. This can come from either the subnets api
// endpoint or the node endpoint.
func (env *maasEnviron) subnetFromJson(
	subnet gomaasapi.JSONObject, spaceId corenetwork.Id,
) (corenetwork.SubnetInfo, error) {
	var subnetInfo corenetwork.SubnetInfo
	fields, err := subnet.GetMap()
	if err != nil {
		return subnetInfo, errors.Trace(err)
	}
	subnetIdFloat, err := fields["id"].GetFloat64()
	if err != nil {
		return subnetInfo, errors.Annotatef(err, "cannot get subnet Id")
	}
	subnetId := strconv.Itoa(int(subnetIdFloat))
	cidr, err := fields["cidr"].GetString()
	if err != nil {
		return subnetInfo, errors.Annotatef(err, "cannot get cidr")
	}
	vid := 0
	vidField, ok := fields["vid"]
	if ok && !vidField.IsNil() {
		// vid is optional, so assume it's 0 when missing or nil.
		vidFloat, err := vidField.GetFloat64()
		if err != nil {
			return subnetInfo, errors.Errorf("cannot get vlan tag: %v", err)
		}
		vid = int(vidFloat)
	}

	subnetInfo = corenetwork.SubnetInfo{
		ProviderId:      corenetwork.Id(subnetId),
		VLANTag:         vid,
		CIDR:            cidr,
		ProviderSpaceId: spaceId,
	}
	return subnetInfo, nil
}

// filteredSubnets fetches subnets, filtering optionally by nodeId and/or a
// slice of subnetIds. If subnetIds is empty then all subnets for that node are
// fetched. If nodeId is empty, all subnets are returned (filtering by subnetIds
// first, if set).
func (env *maasEnviron) filteredSubnets(
	ctx context.ProviderCallContext, nodeId string, subnetIds []corenetwork.Id,
) ([]corenetwork.SubnetInfo, error) {
	var jsonNets []gomaasapi.JSONObject
	var err error
	if nodeId != "" {
		jsonNets, err = env.subnetsFromNode(ctx, nodeId)
		if err != nil {
			return nil, errors.Trace(err)
		}
	} else {
		jsonNets, err = env.fetchAllSubnets(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	subnetIdSet := make(map[string]bool)
	for _, netId := range subnetIds {
		subnetIdSet[string(netId)] = false
	}

	subnetsMap, err := env.subnetToSpaceIds(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var subnets []corenetwork.SubnetInfo
	for _, jsonNet := range jsonNets {
		fields, err := jsonNet.GetMap()
		if err != nil {
			return nil, err
		}
		subnetIdFloat, err := fields["id"].GetFloat64()
		if err != nil {
			return nil, errors.Annotate(err, "cannot get subnet Id")
		}
		subnetId := strconv.Itoa(int(subnetIdFloat))
		// If we're filtering by subnet id check if this subnet is one
		// we're looking for.
		if len(subnetIds) != 0 {
			_, ok := subnetIdSet[subnetId]
			if !ok {
				// This id is not what we're looking for.
				continue
			}
			subnetIdSet[subnetId] = true
		}
		cidr, err := fields["cidr"].GetString()
		if err != nil {
			return nil, errors.Annotatef(err, "cannot get subnet %q cidr", subnetId)
		}
		spaceId, ok := subnetsMap[cidr]
		if !ok {
			logger.Warningf("unrecognised subnet: %q, setting empty space id", cidr)
			spaceId = network.UnknownId
		}

		subnetInfo, err := env.subnetFromJson(jsonNet, spaceId)
		if err != nil {
			return nil, errors.Trace(err)
		}
		subnets = append(subnets, subnetInfo)
		logger.Tracef("found subnet with info %#v", subnetInfo)
	}
	return subnets, checkNotFound(subnetIdSet)
}

func (env *maasEnviron) getInstance(ctx context.ProviderCallContext, instId instance.Id) (instances.Instance, error) {
	instances, err := env.acquiredInstances(ctx, []instance.Id{instId})
	if err != nil {
		// This path can never trigger on MAAS 2, but MAAS 2 doesn't
		// return an error for a machine not found, it just returns
		// empty results. The clause below catches that.
		if maasErr, ok := errors.Cause(err).(gomaasapi.ServerError); ok && maasErr.StatusCode == http.StatusNotFound {
			return nil, errors.NotFoundf("instance %q", instId)
		}
		return nil, errors.Annotatef(err, "getting instance %q", instId)
	}
	if len(instances) == 0 {
		return nil, errors.NotFoundf("instance %q", instId)
	}
	inst := instances[0]
	return inst, nil
}

// fetchAllSubnets calls the MAAS subnets API to get all subnets and returns the
// JSON response or an error. If capNetworkDeploymentUbuntu is not available, an
// error satisfying errors.IsNotSupported will be returned.
func (env *maasEnviron) fetchAllSubnets(ctx context.ProviderCallContext) ([]gomaasapi.JSONObject, error) {
	client := env.getMAASClient().GetSubObject("subnets")

	json, err := client.CallGet("", nil)
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nil, errors.Trace(err)
	}
	return json.GetArray()
}

// subnetToSpaceIds fetches the spaces from MAAS and builds a map of subnets to
// space ids.
func (env *maasEnviron) subnetToSpaceIds(ctx context.ProviderCallContext) (map[string]corenetwork.Id, error) {
	subnetsMap := make(map[string]corenetwork.Id)
	spaces, err := env.Spaces(ctx)
	if err != nil {
		return subnetsMap, errors.Trace(err)
	}
	for _, space := range spaces {
		for _, subnet := range space.Subnets {
			subnetsMap[subnet.CIDR] = space.ProviderId
		}
	}
	return subnetsMap, nil
}

// Spaces returns all the spaces, that have subnets, known to the provider.
// Space name is not filled in as the provider doesn't know the juju name for
// the space.
func (env *maasEnviron) Spaces(ctx context.ProviderCallContext) ([]corenetwork.SpaceInfo, error) {
	if !env.usingMAAS2() {
		return env.spaces1(ctx)
	}
	return env.spaces2(ctx)
}

func (env *maasEnviron) spaces1(ctx context.ProviderCallContext) ([]corenetwork.SpaceInfo, error) {
	spacesClient := env.getMAASClient().GetSubObject("spaces")
	spacesJson, err := spacesClient.CallGet("", nil)
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nil, errors.Trace(err)
	}
	spacesArray, err := spacesJson.GetArray()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var spaces []corenetwork.SpaceInfo
	for _, spaceJson := range spacesArray {
		spaceMap, err := spaceJson.GetMap()
		if err != nil {
			return nil, errors.Trace(err)
		}
		providerIdRaw, err := spaceMap["id"].GetFloat64()
		if err != nil {
			return nil, errors.Trace(err)
		}
		providerId := corenetwork.Id(fmt.Sprintf("%.0f", providerIdRaw))
		name, err := spaceMap["name"].GetString()
		if err != nil {
			return nil, errors.Trace(err)
		}

		space := corenetwork.SpaceInfo{Name: name, ProviderId: providerId}
		subnetsArray, err := spaceMap["subnets"].GetArray()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, subnetJson := range subnetsArray {
			subnet, err := env.subnetFromJson(subnetJson, providerId)
			if err != nil {
				return nil, errors.Trace(err)
			}
			space.Subnets = append(space.Subnets, subnet)
		}
		// Skip spaces with no subnets.
		if len(space.Subnets) > 0 {
			spaces = append(spaces, space)
		}
	}
	return spaces, nil
}

func (env *maasEnviron) spaces2(ctx context.ProviderCallContext) ([]corenetwork.SpaceInfo, error) {
	spaces, err := env.maasController.Spaces()
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nil, errors.Trace(err)
	}
	var result []corenetwork.SpaceInfo
	for _, space := range spaces {
		if len(space.Subnets()) == 0 {
			continue
		}
		outSpace := corenetwork.SpaceInfo{
			Name:       space.Name(),
			ProviderId: corenetwork.Id(strconv.Itoa(space.ID())),
			Subnets:    make([]corenetwork.SubnetInfo, len(space.Subnets())),
		}
		for i, subnet := range space.Subnets() {
			subnetInfo := corenetwork.SubnetInfo{
				ProviderId:      corenetwork.Id(strconv.Itoa(subnet.ID())),
				VLANTag:         subnet.VLAN().VID(),
				CIDR:            subnet.CIDR(),
				ProviderSpaceId: corenetwork.Id(strconv.Itoa(space.ID())),
			}
			outSpace.Subnets[i] = subnetInfo
		}
		result = append(result, outSpace)
	}
	return result, nil
}

// Subnets returns basic information about the specified subnets known
// by the provider for the specified instance. subnetIds must not be
// empty. Implements NetworkingEnviron.Subnets.
func (env *maasEnviron) Subnets(
	ctx context.ProviderCallContext, instId instance.Id, subnetIds []corenetwork.Id,
) ([]corenetwork.SubnetInfo, error) {
	if env.usingMAAS2() {
		return env.subnets2(ctx, instId, subnetIds)
	}
	return env.subnets1(ctx, instId, subnetIds)
}

func (env *maasEnviron) subnets1(
	ctx context.ProviderCallContext, instId instance.Id, subnetIds []corenetwork.Id,
) ([]corenetwork.SubnetInfo, error) {
	var nodeId string
	if instId != instance.UnknownId {
		inst, err := env.getInstance(ctx, instId)
		if err != nil {
			return nil, errors.Trace(err)
		}
		nodeId, err = env.nodeIdFromInstance(inst)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	subnets, err := env.filteredSubnets(ctx, nodeId, subnetIds)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if instId != instance.UnknownId {
		logger.Debugf("instance %q has subnets %v", instId, subnets)
	} else {
		logger.Debugf("found subnets %v", subnets)
	}

	return subnets, nil
}

func (env *maasEnviron) subnets2(
	ctx context.ProviderCallContext, instId instance.Id, subnetIds []corenetwork.Id,
) ([]corenetwork.SubnetInfo, error) {
	var subnets []corenetwork.SubnetInfo
	if instId == instance.UnknownId {
		spaces, err := env.Spaces(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, space := range spaces {
			subnets = append(subnets, space.Subnets...)
		}
	} else {
		var err error
		subnets, err = env.filteredSubnets2(ctx, instId)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	if len(subnetIds) == 0 {
		return subnets, nil
	}
	var result []corenetwork.SubnetInfo
	subnetMap := make(map[string]bool)
	for _, subnetId := range subnetIds {
		subnetMap[string(subnetId)] = false
	}
	for _, subnet := range subnets {
		_, ok := subnetMap[string(subnet.ProviderId)]
		if !ok {
			// This id is not what we're looking for.
			continue
		}
		subnetMap[string(subnet.ProviderId)] = true
		result = append(result, subnet)
	}

	return result, checkNotFound(subnetMap)
}

func (env *maasEnviron) filteredSubnets2(
	ctx context.ProviderCallContext, instId instance.Id,
) ([]corenetwork.SubnetInfo, error) {
	args := gomaasapi.MachinesArgs{
		AgentName: env.uuid,
		SystemIDs: []string{string(instId)},
	}
	machines, err := env.maasController.Machines(args)
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nil, errors.Trace(err)
	}
	if len(machines) == 0 {
		return nil, errors.NotFoundf("machine %v", instId)
	} else if len(machines) > 1 {
		return nil, errors.Errorf("unexpected response getting machine details %v: %v", instId, machines)
	}

	machine := machines[0]
	spaceMap, err := env.buildSpaceMap(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var result []corenetwork.SubnetInfo
	for _, iface := range machine.InterfaceSet() {
		for _, link := range iface.Links() {
			subnet := link.Subnet()
			space, ok := spaceMap[subnet.Space()]
			if !ok {
				return nil, errors.Errorf("missing space %v on subnet %v", subnet.Space(), subnet.CIDR())
			}
			subnetInfo := corenetwork.SubnetInfo{
				ProviderId:      corenetwork.Id(strconv.Itoa(subnet.ID())),
				VLANTag:         subnet.VLAN().VID(),
				CIDR:            subnet.CIDR(),
				ProviderSpaceId: space.ProviderId,
			}
			result = append(result, subnetInfo)
		}
	}
	return result, nil
}

func checkNotFound(subnetIdSet map[string]bool) error {
	var notFound []string
	for subnetId, found := range subnetIdSet {
		if !found {
			notFound = append(notFound, subnetId)
		}
	}
	if len(notFound) != 0 {
		return errors.Errorf("failed to find the following subnets: %v", strings.Join(notFound, ", "))
	}
	return nil
}

// AllInstances implements environs.InstanceBroker.
func (env *maasEnviron) AllInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	return env.acquiredInstances(ctx, nil)
}

// AllRunningInstances implements environs.InstanceBroker.
func (env *maasEnviron) AllRunningInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	// We always get all instances here, so "all" is the same as "running".
	return env.AllInstances(ctx)
}

// Storage is defined by the Environ interface.
func (env *maasEnviron) Storage() storage.Storage {
	env.ecfgMutex.Lock()
	defer env.ecfgMutex.Unlock()
	return env.storageUnlocked
}

func (env *maasEnviron) Destroy(ctx context.ProviderCallContext) error {
	if err := common.Destroy(env, ctx); err != nil {
		return errors.Trace(err)
	}
	return env.Storage().RemoveAll()
}

// DestroyController implements the Environ interface.
func (env *maasEnviron) DestroyController(ctx context.ProviderCallContext, controllerUUID string) error {
	// TODO(wallyworld): destroy hosted model resources
	return env.Destroy(ctx)
}

func (*maasEnviron) Provider() environs.EnvironProvider {
	return &providerInstance
}

func (env *maasEnviron) nodeIdFromInstance(inst instances.Instance) (string, error) {
	maasInst := inst.(*maas1Instance)
	maasObj := maasInst.maasObject
	nodeId, err := maasObj.GetField("system_id")
	if err != nil {
		return "", err
	}
	return nodeId, err
}

func (env *maasEnviron) AllocateContainerAddresses(ctx context.ProviderCallContext, hostInstanceID instance.Id, containerTag names.MachineTag, preparedInfo []network.InterfaceInfo) ([]network.InterfaceInfo, error) {
	if len(preparedInfo) == 0 {
		return nil, errors.Errorf("no prepared info to allocate")
	}
	logger.Debugf("using prepared container info: %+v", preparedInfo)
	if !env.usingMAAS2() {
		return env.allocateContainerAddresses1(ctx, hostInstanceID, containerTag, preparedInfo)
	}
	return env.allocateContainerAddresses2(ctx, hostInstanceID, containerTag, preparedInfo)
}

func (env *maasEnviron) allocateContainerAddresses1(ctx context.ProviderCallContext, hostInstanceID instance.Id, containerTag names.MachineTag, preparedInfo []network.InterfaceInfo) ([]network.InterfaceInfo, error) {
	subnetCIDRToVLANID := make(map[string]string)
	subnetsAPI := env.getMAASClient().GetSubObject("subnets")
	result, err := subnetsAPI.CallGet("", nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get subnets")
	}
	subnetsJSON, err := getJSONBytes(result)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get subnets JSON")
	}
	var subnets []maasSubnet
	if err := json.Unmarshal(subnetsJSON, &subnets); err != nil {
		return nil, errors.Annotate(err, "cannot parse subnets JSON")
	}
	for _, subnet := range subnets {
		subnetCIDRToVLANID[subnet.CIDR] = strconv.Itoa(subnet.VLAN.ID)
	}

	var primaryNICInfo network.InterfaceInfo
	for _, nic := range preparedInfo {
		if nic.InterfaceName == "eth0" {
			primaryNICInfo = nic
			break
		}
	}
	if primaryNICInfo.InterfaceName == "" {
		return nil, errors.Errorf("cannot find primary interface for container")
	}
	logger.Debugf("primary device NIC prepared info: %+v", primaryNICInfo)

	deviceName, err := env.namespace.Hostname(containerTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	primaryMACAddress := primaryNICInfo.MACAddress
	containerDevice, err := env.createDevice(hostInstanceID, deviceName, primaryMACAddress)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create device for container")
	}
	deviceID := instance.Id(containerDevice.ResourceURI)
	logger.Debugf("created device %q with primary MAC address %q", deviceID, primaryMACAddress)

	interfaces, err := env.deviceInterfaces(deviceID)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get device interfaces")
	}
	if len(interfaces) != 1 {
		return nil, errors.Errorf("expected 1 device interface, got %d", len(interfaces))
	}

	primaryNICName := interfaces[0].Name
	primaryNICID := strconv.Itoa(interfaces[0].ID)
	primaryNICSubnetCIDR := primaryNICInfo.CIDR
	primaryNICVLANID, hasSubnet := subnetCIDRToVLANID[primaryNICSubnetCIDR]
	if hasSubnet {
		updatedPrimaryNIC, err := env.updateDeviceInterface(deviceID, primaryNICID, primaryNICName, primaryMACAddress, primaryNICVLANID)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot update device interface %q", interfaces[0].Name)
		}
		logger.Debugf("device %q primary interface %q updated: %+v", containerDevice.SystemID, primaryNICName, updatedPrimaryNIC)
	}

	deviceNICIDs := make([]string, len(preparedInfo))
	nameToParentName := make(map[string]string)
	for i, nic := range preparedInfo {
		maasNICID := ""
		nameToParentName[nic.InterfaceName] = nic.ParentInterfaceName
		nicVLANID, knownSubnet := subnetCIDRToVLANID[nic.CIDR]
		if nic.InterfaceName != primaryNICName {
			if !knownSubnet {
				logger.Warningf("NIC %v has no subnet - setting to manual and using untagged VLAN", nic.InterfaceName)
				nicVLANID = primaryNICVLANID
			} else {
				logger.Infof("linking NIC %v to subnet %v - using static IP", nic.InterfaceName, nic.CIDR)
			}

			createdNIC, err := env.createDeviceInterface(deviceID, nic.InterfaceName, nic.MACAddress, nicVLANID)
			if err != nil {
				return nil, errors.Annotate(err, "creating device interface")
			}
			maasNICID = strconv.Itoa(createdNIC.ID)
			logger.Debugf("created device interface: %+v", createdNIC)
		} else {
			maasNICID = primaryNICID
		}
		deviceNICIDs[i] = maasNICID
		subnetID := string(nic.ProviderSubnetId)

		if !knownSubnet {
			continue
		}

		linkedInterface, err := env.linkDeviceInterfaceToSubnet(deviceID, maasNICID, subnetID, modeStatic)
		if err != nil {
			logger.Warningf("linking NIC %v to subnet %v failed: %v", nic.InterfaceName, nic.CIDR, err)
		} else {
			logger.Debugf("linked device interface to subnet: %+v", linkedInterface)
		}
	}

	finalInterfaces, err := env.deviceInterfaceInfo(deviceID, nameToParentName)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get device interfaces")
	}
	logger.Debugf("allocated device interfaces: %+v", finalInterfaces)

	return finalInterfaces, nil
}

func (env *maasEnviron) allocateContainerAddresses2(ctx context.ProviderCallContext, hostInstanceID instance.Id, containerTag names.MachineTag, preparedInfo []network.InterfaceInfo) ([]network.InterfaceInfo, error) {
	args := gomaasapi.MachinesArgs{
		AgentName: env.uuid,
		SystemIDs: []string{string(hostInstanceID)},
	}
	machines, err := env.maasController.Machines(args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(machines) != 1 {
		return nil, errors.Errorf("failed to identify unique machine with ID %q; got %v", hostInstanceID, machines)
	}
	machine := machines[0]
	deviceName, err := env.namespace.Hostname(containerTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	params, err := env.prepareDeviceDetails(deviceName, machine, preparedInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Check to see if we've already tried to allocate information for this device:
	device, err := env.checkForExistingDevice(params)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if device == nil {
		device, err = env.createAndPopulateDevice(params)
		if err != nil {
			return nil, errors.Annotatef(err,
				"failed to create MAAS device for %q",
				params.Name)
		}
	}

	// TODO(jam): the old code used to reload the device from its SystemID()
	nameToParentName := make(map[string]string)
	for _, nic := range preparedInfo {
		nameToParentName[nic.InterfaceName] = nic.ParentInterfaceName
	}
	interfaces, err := env.deviceInterfaceInfo2(device, nameToParentName, params.CIDRToStaticRoutes)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get device interfaces")
	}
	return interfaces, nil
}

func (env *maasEnviron) ReleaseContainerAddresses(ctx context.ProviderCallContext, interfaces []network.ProviderInterfaceInfo) error {
	macAddresses := make([]string, len(interfaces))
	for i, info := range interfaces {
		macAddresses[i] = info.MACAddress
	}
	if !env.usingMAAS2() {
		return env.releaseContainerAddresses1(ctx, macAddresses)
	}
	return env.releaseContainerAddresses2(ctx, macAddresses)
}

func (env *maasEnviron) releaseContainerAddresses1(ctx context.ProviderCallContext, macAddresses []string) error {
	devicesAPI := env.getMAASClient().GetSubObject("devices")
	values := url.Values{}
	for _, address := range macAddresses {
		values.Add("mac_address", address)
	}
	result, err := devicesAPI.CallGet("list", values)
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return errors.Trace(err)
	}
	devicesArray, err := result.GetArray()
	if err != nil {
		return errors.Trace(err)
	}
	deviceIds := make([]string, len(devicesArray))
	for i, deviceItem := range devicesArray {
		deviceMap, err := deviceItem.GetMap()
		if err != nil {
			return errors.Trace(err)
		}
		id, err := deviceMap["system_id"].GetString()
		if err != nil {
			return errors.Trace(err)
		}
		deviceIds[i] = id
	}

	// If one device matched on multiple MAC addresses (like for
	// multi-nic containers) it will be in the slice multiple
	// times. Skip devices we've seen already.
	deviceIdSet := set.NewStrings(deviceIds...)
	deviceIds = deviceIdSet.SortedValues()

	for _, id := range deviceIds {
		err := devicesAPI.GetSubObject(id).Delete()
		if err != nil {
			common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
			return errors.Annotatef(err, "deleting device %s", id)
		}
	}
	return nil
}

func (env *maasEnviron) releaseContainerAddresses2(ctx context.ProviderCallContext, macAddresses []string) error {
	devices, err := env.maasController.Devices(gomaasapi.DevicesArgs{MACAddresses: macAddresses})
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return errors.Trace(err)
	}
	// If one device matched on multiple MAC addresses (like for
	// multi-nic containers) it will be in the slice multiple
	// times. Skip devices we've seen already.
	seen := set.NewStrings()
	for _, device := range devices {
		if seen.Contains(device.SystemID()) {
			continue
		}
		seen.Add(device.SystemID())

		err = device.Delete()
		if err != nil {
			return errors.Annotatef(err, "deleting device %s", device.SystemID())
		}
	}
	return nil
}

// AdoptResources updates all the instances to indicate they
// are now associated with the specified controller.
func (env *maasEnviron) AdoptResources(ctx context.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	if !env.usingMAAS2() {
		// We don't track instance -> controller for MAAS1.
		return nil
	}

	instances, err := env.AllInstances(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	var failed []instance.Id
	for _, inst := range instances {
		maas2Instance, ok := inst.(*maas2Instance)
		if !ok {
			// This should never happen.
			return errors.Errorf("instance %q wasn't a maas2Instance", inst.Id())
		}
		// From the MAAS docs: "[SetOwnerData] will not remove any
		// previous keys unless explicitly passed with an empty
		// string." So not passing all of the keys here is fine.
		// https://maas.ubuntu.com/docs2.0/api.html#machine
		err := maas2Instance.machine.SetOwnerData(map[string]string{tags.JujuController: controllerUUID})
		if err != nil {
			logger.Errorf("error setting controller uuid tag for %q: %v", inst.Id(), err)
			failed = append(failed, inst.Id())
		}
	}

	if failed != nil {
		return errors.Errorf("failed to update controller for some instances: %v", failed)
	}
	return nil
}

// ProviderSpaceInfo implements environs.NetworkingEnviron.
func (*maasEnviron) ProviderSpaceInfo(
	ctx context.ProviderCallContext, space *corenetwork.SpaceInfo,
) (*environs.ProviderSpaceInfo, error) {
	return nil, errors.NotSupportedf("provider space info")
}

// AreSpacesRoutable implements environs.NetworkingEnviron.
func (*maasEnviron) AreSpacesRoutable(ctx context.ProviderCallContext, space1, space2 *environs.ProviderSpaceInfo) (bool, error) {
	return false, nil
}

// SSHAddresses implements environs.SSHAddresses.
func (*maasEnviron) SSHAddresses(ctx context.ProviderCallContext, addresses []network.Address) ([]network.Address, error) {
	return addresses, nil
}

// SuperSubnets implements environs.SuperSubnets
func (*maasEnviron) SuperSubnets(ctx context.ProviderCallContext) ([]string, error) {
	return nil, errors.NotSupportedf("super subnets")
}

// Get the domains managed by MAAS. Currently we only need the name of the domain. If more information is needed
// This function can be updated to parse and return a structure. Client code would need to be updated.
func (env *maasEnviron) Domains(ctx context.ProviderCallContext) ([]string, error) {
	maasDomains, err := env.maasController.Domains()
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nil, errors.Trace(err)
	}
	result := []string{}
	for _, domain := range maasDomains {
		result = append(result, domain.Name())
	}
	return result, nil
}
