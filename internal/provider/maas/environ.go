// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/gomaasapi/v2"
	"github.com/juju/names/v6"
	"github.com/juju/retry"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/cloudconfig/providerinit"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/version"
)

const (
	// The version strings indicating the MAAS API version.
	apiVersion2 = "2.0"
)

var defaultShortRetryStrategy = retry.CallArgs{
	Clock:       clock.WallClock,
	Delay:       200 * time.Millisecond,
	MaxDuration: 5 * time.Second,
}

var defaultLongRetryStrategy = retry.CallArgs{
	Clock:       clock.WallClock,
	Delay:       10 * time.Second,
	MaxDuration: 1200 * time.Second,
}

var GetMAASController = getMAASController

func getMAASController(args gomaasapi.ControllerArgs) (gomaasapi.Controller, error) {
	return gomaasapi.NewController(args)
}

type maasEnviron struct {
	common.CredentialInvalidator

	name string
	uuid string

	// archMutex gates access to supportedArchitectures
	archMutex sync.Mutex

	// ecfgMutex protects the *Unlocked fields below.
	ecfgMutex sync.Mutex

	ecfgUnlocked    *maasModelConfig
	storageUnlocked storage.Storage

	// maasController provides access to the MAAS 2.0 API.
	maasController gomaasapi.Controller

	// namespace is used to create the machine and device hostnames.
	namespace instance.Namespace

	// apiVersion tells us if we are using the MAAS 1.0 or 2.0 api.
	apiVersion string

	// GetCapabilities is a function that connects to MAAS to return its set of
	// capabilities.
	GetCapabilities Capabilities

	// A request may fail to due "eventual consistency" semantics, which
	// should resolve fairly quickly.  A request may also fail due to a slow
	// state transition (for instance an instance taking a while to release
	// a security group after termination).  The former failure mode is
	// dealt with by shortRetryStrategy, the latter by longRetryStrategy
	shortRetryStrategy retry.CallArgs
	longRetryStrategy  retry.CallArgs
}

var (
	_ environs.Environ    = (*maasEnviron)(nil)
	_ environs.Networking = (*maasEnviron)(nil)
)

// Capabilities is an alias for a function that gets
// the capabilities of a MAAS installation.
type Capabilities = func(ctx context.Context, client *gomaasapi.MAASObject, serverURL string) (set.Strings, error)

func NewEnviron(
	ctx context.Context, cloud environscloudspec.CloudSpec, cfg *config.Config,
	invalidator environs.CredentialInvalidator, getCaps Capabilities,
) (*maasEnviron, error) {
	if getCaps == nil {
		getCaps = getCapabilities
	}
	env := &maasEnviron{
		CredentialInvalidator: common.NewCredentialInvalidator(invalidator, IsAuthorisationFailure),
		name:                  cfg.Name(),
		uuid:                  cfg.UUID(),
		GetCapabilities:       getCaps,
		shortRetryStrategy:    defaultShortRetryStrategy,
		longRetryStrategy:     defaultLongRetryStrategy,
	}
	if err := env.SetConfig(ctx, cfg); err != nil {
		return nil, errors.Trace(err)
	}
	if err := env.SetCloudSpec(ctx, cloud); err != nil {
		return nil, errors.Trace(err)
	}

	var err error
	env.namespace, err = instance.NewNamespace(cfg.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return env, nil
}

// PrepareForBootstrap is part of the Environ interface.
func (env *maasEnviron) PrepareForBootstrap(_ environs.BootstrapContext, _ string) error {
	return nil
}

// Bootstrap is part of the Environ interface.
func (env *maasEnviron) Bootstrap(
	ctx environs.BootstrapContext, callCtx envcontext.ProviderCallContext, args environs.BootstrapParams,
) (*environs.BootstrapResult, error) {
	result, base, finalizer, err := common.BootstrapInstance(ctx, env, callCtx, args)
	if err != nil {
		return nil, err
	}

	// We want to destroy the started instance if it doesn't transition to Deployed.
	defer func() {
		if err != nil {
			if err := env.StopInstances(callCtx, result.Instance.Id()); err != nil {
				logger.Errorf(ctx, "error releasing bootstrap instance: %v", err)
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
		Base:                    *base,
		CloudBootstrapFinalizer: waitingFinalizer,
	}
	return bsResult, nil
}

// ControllerInstances is specified in the Environ interface.
func (env *maasEnviron) ControllerInstances(ctx envcontext.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	instances, err := env.instances(ctx, gomaasapi.MachinesArgs{
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
func (env *maasEnviron) SetConfig(ctx context.Context, cfg *config.Config) error {
	env.ecfgMutex.Lock()
	defer env.ecfgMutex.Unlock()

	// The new config has already been validated by itself, but now we
	// validate the transition from the old config to the new.
	var oldCfg *config.Config
	if env.ecfgUnlocked != nil {
		oldCfg = env.ecfgUnlocked.Config
	}
	cfg, err := env.Provider().Validate(ctx, cfg, oldCfg)
	if err != nil {
		return errors.Trace(err)
	}

	ecfg, err := providerInstance.newConfig(ctx, cfg)
	if err != nil {
		return errors.Trace(err)
	}

	env.ecfgUnlocked = ecfg

	return nil
}

// SetCloudSpec is specified in the environs.Environ interface.
func (env *maasEnviron) SetCloudSpec(_ context.Context, spec environscloudspec.CloudSpec) error {
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

	apiVersion := apiVersion2
	args := gomaasapi.ControllerArgs{
		BaseURL: maasServer,
		APIKey:  maasOAuth,
	}
	// If the user has specified to skip TLS verification, we need to
	// add a new http client with insecure TLS (skip verify).
	if spec.SkipTLSVerify {
		args.HTTPClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
	}
	controller, err := GetMAASController(args)
	if err != nil {
		return errors.Trace(err)
	}

	env.maasController = controller
	env.apiVersion = apiVersion
	env.storageUnlocked = NewStorage(env)

	return nil
}

// ValidateCloudEndpoint returns nil if the current model can talk to the maas
// endpoint.  Used as validation during model upgrades.
// Implements environs.CloudEndpointChecker
func (env *maasEnviron) ValidateCloudEndpoint(ctx context.Context) error {
	_, _, err := env.maasController.APIVersionInfo()
	return errors.Trace(err)
}

func (env *maasEnviron) getSupportedArchitectures(ctx envcontext.ProviderCallContext) ([]string, error) {
	env.archMutex.Lock()
	defer env.archMutex.Unlock()

	resources, err := env.maasController.BootResources()
	if err != nil {
		return nil, env.HandleCredentialError(ctx, err)
	}
	architectures := set.NewStrings()
	for _, resource := range resources {
		architectures.Add(strings.Split(resource.Architecture(), "/")[0])
	}
	return architectures.SortedValues(), nil
}

// SupportsSpaces is specified on environs.Networking.
func (env *maasEnviron) SupportsSpaces() (bool, error) {
	return true, nil
}

// SupportsSpaceDiscovery is specified on environs.Networking.
func (env *maasEnviron) SupportsSpaceDiscovery() (bool, error) {
	return true, nil
}

// SupportsContainerAddresses is specified on environs.Networking.
func (env *maasEnviron) SupportsContainerAddresses(ctx context.Context) (bool, error) {
	return true, nil
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
func (env *maasEnviron) AvailabilityZones(ctx context.Context) (corenetwork.AvailabilityZones, error) {
	zones, err := env.maasController.Zones()
	if err != nil {
		return nil, env.HandleCredentialError(ctx, err)
	}
	availabilityZones := make(corenetwork.AvailabilityZones, len(zones))
	for i, zone := range zones {
		availabilityZones[i] = maasAvailabilityZone{zone.Name()}
	}
	return availabilityZones, nil
}

// InstanceAvailabilityZoneNames returns the availability zone names for each
// of the specified instances.
func (env *maasEnviron) InstanceAvailabilityZoneNames(ctx envcontext.ProviderCallContext, ids []instance.Id) (map[instance.Id]string, error) {
	inst, err := env.Instances(ctx, ids)
	if err != nil && err != environs.ErrPartialInstances {
		return nil, err
	}
	zones := make(map[instance.Id]string, 0)
	for _, inst := range inst {
		if inst == nil {
			continue
		}
		mInst, ok := inst.(*maasInstance)
		if !ok {
			continue
		}
		z, err := mInst.zone()
		if err != nil {
			logger.Errorf(ctx, "could not get availability zone %v", err)
			continue
		}
		zones[inst.Id()] = z
	}
	return zones, nil
}

// DeriveAvailabilityZones is part of the common.ZonedEnviron interface.
func (env *maasEnviron) DeriveAvailabilityZones(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) ([]string, error) {
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

func (env *maasEnviron) parsePlacement(ctx envcontext.ProviderCallContext, placement string) (*maasPlacement, error) {
	pos := strings.IndexRune(placement, '=')
	if pos == -1 {
		// If there's no '=' delimiter, assume it's a node name.
		return &maasPlacement{nodeName: placement}, nil
	}
	switch key, value := placement[:pos], placement[pos+1:]; key {
	case "zone":
		zones, err := env.AvailabilityZones(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if err := zones.Validate(value); err != nil {
			return nil, errors.Trace(err)
		}

		return &maasPlacement{zoneName: value}, nil
	case "system-id":
		return &maasPlacement{systemId: value}, nil
	}

	return nil, errors.Errorf("unknown placement directive: %v", placement)
}

func (env *maasEnviron) PrecheckInstance(ctx envcontext.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	if args.Placement == "" {
		return nil
	}
	_, err := env.parsePlacement(ctx, args.Placement)
	return err
}

// getCapabilities asks the MAAS server for its capabilities, if
// supported by the server.
func getCapabilities(ctx context.Context, client *gomaasapi.MAASObject, serverURL string) (set.Strings, error) {
	caps := make(set.Strings)
	var result gomaasapi.JSONObject

	retryStrategy := defaultShortRetryStrategy
	retryStrategy.IsFatalError = func(err error) bool {
		if err, ok := errors.Cause(err).(gomaasapi.ServerError); ok && err.StatusCode == 404 {
			return true
		}
		return false
	}
	retryStrategy.Func = func() error {
		var err error
		version := client.GetSubObject("version/")
		result, err = version.CallGet("", nil)
		return err
	}
	err := retry.Call(retryStrategy)

	if retry.IsAttemptsExceeded(err) || retry.IsDurationExceeded(err) {
		logger.Debugf(ctx, "Can't connect to maas server at endpoint %q: %v", serverURL, err)
		err = retry.LastError(err)
		return caps, err
	}
	if err != nil {
		err, _ := errors.Cause(err).(gomaasapi.ServerError)
		logger.Debugf(ctx, "Failed attempting to get capabilities from maas endpoint %q: %v", serverURL, err)

		message := "could not connect to MAAS controller - check the endpoint is correct"
		trimmedURL := strings.TrimRight(serverURL, "/")
		if !strings.HasSuffix(trimmedURL, "/MAAS") {
			message += " (it normally ends with /MAAS)"
		}
		return caps, errors.NewNotSupported(nil, message)
	}

	info, err := result.GetMap()
	if err != nil {
		logger.Debugf(ctx, "Invalid data returned from maas endpoint %q: %v", serverURL, err)
		// invalid data of some sort, probably not a MAAS server.
		return caps, errors.New("failed to get expected data from server")
	}
	capsObj, ok := info["capabilities"]
	if !ok {
		return caps, fmt.Errorf("MAAS does not report capabilities")
	}
	items, err := capsObj.GetArray()
	if err != nil {
		logger.Debugf(ctx, "Invalid data returned from maas endpoint %q: %v", serverURL, err)
		return caps, errors.New("failed to get expected data from server")
	}
	for _, item := range items {
		val, err := item.GetString()
		if err != nil {
			logger.Debugf(ctx, "Invalid data returned from maas endpoint %q: %v", serverURL, err)
			return set.NewStrings(), errors.New("failed to get expected data from server")
		}
		caps.Add(val)
	}
	return caps, nil
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

func (env *maasEnviron) buildSpaceMap(ctx envcontext.ProviderCallContext) (map[string]corenetwork.SpaceInfo, error) {
	spaces, err := env.Spaces(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	spaceMap := make(map[string]corenetwork.SpaceInfo)
	empty := set.Strings{}
	for _, space := range spaces {
		jujuName := corenetwork.ConvertSpaceName(string(space.Name), empty)
		spaceMap[jujuName] = space
	}
	return spaceMap, nil
}

func (env *maasEnviron) spaceNamesToSpaceInfo(
	ctx envcontext.ProviderCallContext, positiveSpaces, negativeSpaces []string,
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

// networkSpaceRequirements combines the space requirements for the application
// bindings and the specified constraints and returns a set of provider
// space IDs for which a NIC needs to be provisioned in the instance we are
// about to launch and a second (negative) set of space IDs that must not be
// present in the launched instance NICs.
func (env *maasEnviron) networkSpaceRequirements(ctx envcontext.ProviderCallContext, endpointToProviderSpaceID map[string]corenetwork.Id, cons constraints.Value) (set.Strings, set.Strings, error) {
	positiveSpaceIds := set.NewStrings()
	negativeSpaceIds := set.NewStrings()

	// Iterate the application bindings and add each bound space ID to the
	// positive space set.
	for _, providerSpaceID := range endpointToProviderSpaceID {
		// The alpha space is not part of the MAAS space list. When the
		// code that maps between space IDs and provider space IDs
		// encounters a space that it cannot map, it passes the space
		// name through.
		if providerSpaceID == corenetwork.AlphaSpaceName {
			continue
		}

		positiveSpaceIds.Add(string(providerSpaceID))
	}

	// Convert space constraints into a list of space IDs to include and
	// a list of space IDs to omit.
	positiveSpaceNames, negativeSpaceNames := convertSpacesFromConstraints(cons.Spaces)
	positiveSpaceInfo, negativeSpaceInfo, err := env.spaceNamesToSpaceInfo(ctx, positiveSpaceNames, negativeSpaceNames)
	if err != nil {
		// Spaces are not supported by this MAAS instance.
		if errors.Is(err, errors.NotSupported) {
			return nil, nil, nil
		}

		return nil, nil, env.HandleCredentialError(ctx, err)
	}

	// Append required space IDs from constraints.
	for _, si := range positiveSpaceInfo {
		if si.ProviderId == "" {
			continue
		}
		positiveSpaceIds.Add(string(si.ProviderId))
	}

	// Calculate negative space ID set and check for clashes with the positive set.
	for _, si := range negativeSpaceInfo {
		if si.ProviderId == "" {
			continue
		}

		if positiveSpaceIds.Contains(string(si.ProviderId)) {
			return nil, nil, errors.NewNotValid(nil, fmt.Sprintf("negative space %q from constraints clashes with required spaces for instance NICs", si.Name))
		}

		negativeSpaceIds.Add(string(si.ProviderId))
	}

	return positiveSpaceIds, negativeSpaceIds, nil
}

// acquireNode allocates a machine from MAAS.
func (env *maasEnviron) acquireNode(
	ctx envcontext.ProviderCallContext,
	nodeName, zoneName, systemId string,
	cons constraints.Value,
	positiveSpaceIDs set.Strings,
	negativeSpaceIDs set.Strings,
	volumes []volumeInfo,
) (*maasInstance, error) {
	acquireParams := convertConstraints(cons)
	addInterfaces(&acquireParams, positiveSpaceIDs, negativeSpaceIDs)
	addStorage(&acquireParams, volumes)
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
		return nil, env.HandleCredentialError(ctx, err)
	}
	return &maasInstance{
		machine:           machine,
		constraintMatches: constraintMatches,
		environ:           env,
	}, nil
}

// StartInstance is specified in the InstanceBroker interface.
func (env *maasEnviron) StartInstance(
	ctx envcontext.ProviderCallContext,
	args environs.StartInstanceParams,
) (_ *environs.StartInstanceResult, err error) {
	availabilityZone := args.AvailabilityZone
	var nodeName, systemId string
	if args.Placement != "" {
		placement, err := env.parsePlacement(ctx, args.Placement)
		if err != nil {
			return nil, environs.ZoneIndependentError(err)
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
		zones, err := env.AvailabilityZones(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if err := errors.Trace(zones.Validate(availabilityZone)); err != nil {
			return nil, errors.Trace(err)
		}
		logger.Debugf(ctx, "attempting to acquire node in zone %q", availabilityZone)
	}

	// Storage.
	volumes, err := buildMAASVolumeParameters(args.Volumes, args.Constraints)
	if err != nil {
		return nil, environs.ZoneIndependentError(errors.Annotate(err, "invalid volume parameters"))
	}

	// Calculate network space requirements.
	positiveSpaceIDs, negativeSpaceIDs, err := env.networkSpaceRequirements(ctx, args.EndpointBindings, args.Constraints)
	if err != nil {
		return nil, errors.Trace(err)
	}

	inst, selectNodeErr := env.selectNode(ctx,
		selectNodeArgs{
			Constraints:      args.Constraints,
			AvailabilityZone: availabilityZone,
			NodeName:         nodeName,
			SystemId:         systemId,
			PositiveSpaceIDs: positiveSpaceIDs,
			NegativeSpaceIDs: negativeSpaceIDs,
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
		return nil, environs.ZoneIndependentError(err)
	}

	defer func() {
		if err != nil {
			if err := env.StopInstances(ctx, inst.Id()); err != nil {
				logger.Errorf(ctx, "error releasing failed instance: %v", err)
			}
		}
	}()

	hc, err := inst.hardwareCharacteristics()
	if err != nil {
		return nil, environs.ZoneIndependentError(err)
	}

	selectedTools, err := args.Tools.Match(tools.Filter{
		Arch: *hc.Arch,
	})
	if err != nil {
		return nil, environs.ZoneIndependentError(err)
	}
	if err := args.InstanceConfig.SetTools(selectedTools); err != nil {
		return nil, environs.ZoneIndependentError(err)
	}

	hostname, err := inst.hostname()
	if err != nil {
		return nil, environs.ZoneIndependentError(err)
	}

	if err := instancecfg.FinishInstanceConfig(args.InstanceConfig, env.Config()); err != nil {
		return nil, environs.ZoneIndependentError(err)
	}

	subnetsMap, err := env.subnetToSpaceIds(ctx)
	if err != nil {
		return nil, environs.ZoneIndependentError(err)
	}

	cloudcfg, err := env.newCloudinitConfig(ctx, hostname, args.InstanceConfig.Base.OS)
	if err != nil {
		return nil, environs.ZoneIndependentError(err)
	}

	userdata, err := providerinit.ComposeUserData(args.InstanceConfig, cloudcfg, MAASRenderer{})
	if err != nil {
		return nil, environs.ZoneIndependentError(errors.Annotate(
			err, "could not compose userdata for bootstrap node",
		))
	}
	logger.Debugf(ctx, "maas user data; %d bytes", len(userdata))

	distroSeries, err := env.distroSeries(args)
	if err != nil {
		return nil, environs.ZoneIndependentError(err)
	}
	err = inst.machine.Start(gomaasapi.StartArgs{
		DistroSeries: distroSeries,
		UserData:     string(userdata),
	})
	if err != nil {
		return nil, environs.ZoneIndependentError(err)
	}

	domains, err := env.Domains(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	interfaces, err := maasNetworkInterfaces(ctx, inst, subnetsMap, domains...)
	if err != nil {
		return nil, environs.ZoneIndependentError(err)
	}
	env.tagInstance(ctx, inst, args.InstanceConfig)

	displayName, err := inst.displayName()
	if err != nil {
		return nil, environs.ZoneIndependentError(err)
	}
	logger.Debugf(ctx, "started instance %q", inst.Id())

	requestedVolumes := make([]names.VolumeTag, len(args.Volumes))
	for i, v := range args.Volumes {
		requestedVolumes[i] = v.Tag
	}
	resultVolumes, resultAttachments, err := inst.volumes(
		ctx,
		names.NewMachineTag(args.InstanceConfig.MachineId),
		requestedVolumes,
	)
	if err != nil {
		return nil, environs.ZoneIndependentError(err)
	}
	if len(resultVolumes) != len(requestedVolumes) {
		return nil, environs.ZoneIndependentError(errors.Errorf(
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

func (env *maasEnviron) tagInstance(ctx context.Context, inst *maasInstance, instanceConfig *instancecfg.InstanceConfig) {
	err := inst.machine.SetOwnerData(instanceConfig.Tags)
	if err != nil {
		logger.Errorf(ctx, "could not set owner data for instance: %v", err)
	}
}

func (env *maasEnviron) distroSeries(args environs.StartInstanceParams) (string, error) {
	if args.Constraints.HasImageID() {
		return *args.Constraints.ImageID, nil
	}
	return base.GetSeriesFromBase(args.InstanceConfig.Base)
}

func (env *maasEnviron) waitForNodeDeployment(ctx envcontext.ProviderCallContext, id instance.Id, timeout time.Duration) error {
	retryStrategy := env.longRetryStrategy
	retryStrategy.MaxDuration = timeout
	retryStrategy.IsFatalError = func(err error) bool {
		if errors.Is(err, errors.NotProvisioned) {
			return true
		}
		if denied, _ := env.MaybeInvalidateCredentialError(ctx, err); denied {
			return true
		}
		return false
	}
	retryStrategy.NotifyFunc = func(lastErr error, attempts int) {
		if errors.Is(lastErr, errors.NotFound) {
			logger.Warningf(ctx, "failed to get instance from provider attempt %d", attempts)
		}
	}
	retryStrategy.Func = func() error {
		machine, err := env.getInstance(ctx, id)
		if err != nil {
			return err
		}
		stat := machine.Status(ctx)
		if stat.Status == status.Running {
			return nil
		}
		if stat.Status == status.ProvisioningError {
			return errors.NewNotProvisioned(nil, fmt.Sprintf("instance %q failed to deploy", id))
		}
		return errors.NewNotYetAvailable(nil, "Not yet provisioned")
	}
	err := retry.Call(retryStrategy)
	if retry.IsAttemptsExceeded(err) || retry.IsDurationExceeded(err) {
		return errors.Errorf("instance %q is started but not deployed", id)
	}
	return errors.Trace(err)
}

type selectNodeArgs struct {
	AvailabilityZone string
	NodeName         string
	SystemId         string
	Constraints      constraints.Value
	PositiveSpaceIDs set.Strings
	NegativeSpaceIDs set.Strings
	Volumes          []volumeInfo
}

type selectNodeError struct {
	error
	noMatch bool
}

func (env *maasEnviron) selectNode(ctx envcontext.ProviderCallContext, args selectNodeArgs) (*maasInstance, *selectNodeError) {
	inst, err := env.acquireNode(
		ctx,
		args.NodeName,
		args.AvailabilityZone,
		args.SystemId,
		args.Constraints,
		args.PositiveSpaceIDs,
		args.NegativeSpaceIDs,
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
func (env *maasEnviron) newCloudinitConfig(ctx context.Context, hostname, osname string) (cloudinit.CloudConfig, error) {
	cloudcfg, err := cloudinit.New(osname)
	if err != nil {
		return nil, err
	}

	info := machineInfo{hostname}
	runCmd, err := info.cloudinitRunCmd(cloudcfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	operatingSystem := ostype.OSTypeForName(osname)
	switch operatingSystem {
	case ostype.Ubuntu:
		cloudcfg.SetSystemUpdate(true)
		cloudcfg.AddScripts("set -xe", runCmd)
		// DisableNetworkManagement can still disable the bridge(s) creation.
		if on, set := env.Config().DisableNetworkManagement(); on && set {
			logger.Infof(ctx,
				"network management disabled - not using %q bridge for containers",
				instancecfg.DefaultBridgeName,
			)
			break
		}
		cloudcfg.AddPackage("bridge-utils")
	}
	return cloudcfg, nil
}

func (env *maasEnviron) releaseNodes(ctx envcontext.ProviderCallContext, ids []instance.Id, recurse bool) error {
	args := gomaasapi.ReleaseMachinesArgs{
		SystemIDs: instanceIdsToSystemIDs(ids),
		Comment:   "Released by Juju MAAS provider",
	}
	err := env.maasController.ReleaseMachines(args)

	denied, _ := env.MaybeInvalidateCredentialError(ctx, err)
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
		logger.Infof(ctx, "ignoring error while releasing nodes (%v); all nodes released OK", err)
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

func (env *maasEnviron) releaseNodesIndividually(ctx envcontext.ProviderCallContext, ids []instance.Id) error {
	var lastErr error
	for _, id := range ids {
		err := env.releaseNodes(ctx, []instance.Id{id}, false)
		if err != nil {
			lastErr = err
			logger.Errorf(ctx, "error while releasing node %v (%v)", id, err)
			if denied, _ := env.MaybeInvalidateCredentialError(ctx, err); denied {
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
func (env *maasEnviron) StopInstances(ctx envcontext.ProviderCallContext, ids ...instance.Id) error {
	// Shortcut to exit quickly if 'instances' is an empty slice or nil.
	if len(ids) == 0 {
		return nil
	}

	err := env.releaseNodes(ctx, ids, true)
	if err != nil {
		return errors.Trace(err)
	}
	return common.RemoveStateInstances(env.Storage(), ids...)
}

// Instances returns the instances.Instance objects corresponding to the given
// slice of instance.Id.  The error is ErrNoInstances if no instances
// were found.
func (env *maasEnviron) Instances(ctx context.Context, ids []instance.Id) ([]instances.Instance, error) {
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
	for _, inst := range acquired {
		idMap[inst.Id()] = inst
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
func (env *maasEnviron) acquiredInstances(ctx context.Context, ids []instance.Id) ([]instances.Instance, error) {
	args := gomaasapi.MachinesArgs{
		AgentName: env.uuid,
		SystemIDs: instanceIdsToSystemIDs(ids),
	}

	maasInstances, err := env.instances(ctx, args)
	if err != nil {
		return nil, errors.Trace(err)
	}

	inst := make([]instances.Instance, len(maasInstances))
	for i, mi := range maasInstances {
		inst[i] = mi
	}
	return inst, nil
}

func (env *maasEnviron) instances(ctx context.Context, args gomaasapi.MachinesArgs) ([]*maasInstance, error) {
	machines, err := env.maasController.Machines(args)
	if err != nil {
		return nil, env.HandleCredentialError(ctx, err)
	}

	inst := make([]*maasInstance, len(machines))
	for index, machine := range machines {
		inst[index] = &maasInstance{machine: machine, environ: env}
	}
	return inst, nil
}

func (env *maasEnviron) getInstance(ctx envcontext.ProviderCallContext, instId instance.Id) (instances.Instance, error) {
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

// subnetToSpaceIds fetches the spaces from MAAS and builds a map of subnets to
// space ids.
func (env *maasEnviron) subnetToSpaceIds(ctx envcontext.ProviderCallContext) (map[string]corenetwork.Id, error) {
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
func (env *maasEnviron) Spaces(ctx envcontext.ProviderCallContext) (corenetwork.SpaceInfos, error) {
	spaces, err := env.maasController.Spaces()
	if err != nil {
		return nil, env.HandleCredentialError(ctx, err)
	}

	var result []corenetwork.SpaceInfo

	if len(spaces) == 0 {
		return result, nil
	}

	// At the time of writing, MAAS zones are effectively just labels.
	// They do not indicate subnet availability as on say, AWS.
	// Here, we indicate that all subnets are available in all zones,
	// and leave it up to the provisioner to see if it can provision
	// a suitable instance.
	zones, err := env.maasController.Zones()
	if err != nil {
		return nil, env.HandleCredentialError(ctx, err)
	}
	zoneNames := transform.Slice(zones, func(zone gomaasapi.Zone) string { return zone.Name() })

	for _, space := range spaces {
		subs := space.Subnets()
		if len(subs) == 0 {
			continue
		}

		outSpace := corenetwork.SpaceInfo{
			Name:       corenetwork.SpaceName(space.Name()),
			ProviderId: corenetwork.Id(strconv.Itoa(space.ID())),
			Subnets:    make([]corenetwork.SubnetInfo, len(subs)),
		}

		for i, subnet := range subs {
			subnetInfo := corenetwork.SubnetInfo{
				ProviderId:        corenetwork.Id(strconv.Itoa(subnet.ID())),
				VLANTag:           subnet.VLAN().VID(),
				CIDR:              subnet.CIDR(),
				ProviderSpaceId:   corenetwork.Id(strconv.Itoa(space.ID())),
				AvailabilityZones: zoneNames,
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
	ctx envcontext.ProviderCallContext, instId instance.Id, subnetIds []corenetwork.Id,
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
		subnets, err = env.filteredSubnets(ctx, instId)
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

func (env *maasEnviron) filteredSubnets(
	ctx envcontext.ProviderCallContext, instId instance.Id,
) ([]corenetwork.SubnetInfo, error) {
	args := gomaasapi.MachinesArgs{
		AgentName: env.uuid,
		SystemIDs: []string{string(instId)},
	}
	machines, err := env.maasController.Machines(args)
	if err != nil {
		return nil, env.HandleCredentialError(ctx, err)
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
func (env *maasEnviron) AllInstances(ctx context.Context) ([]instances.Instance, error) {
	return env.acquiredInstances(ctx, nil)
}

// AllRunningInstances implements environs.InstanceBroker.
func (env *maasEnviron) AllRunningInstances(ctx context.Context) ([]instances.Instance, error) {
	// We always get all instances here, so "all" is the same as "running".
	return env.AllInstances(ctx)
}

// Storage is defined by the Environ interface.
func (env *maasEnviron) Storage() storage.Storage {
	env.ecfgMutex.Lock()
	defer env.ecfgMutex.Unlock()
	return env.storageUnlocked
}

func (env *maasEnviron) Destroy(ctx envcontext.ProviderCallContext) error {
	if err := common.Destroy(env, ctx); err != nil {
		return errors.Trace(err)
	}
	return env.Storage().RemoveAll()
}

// DestroyController implements the Environ interface.
func (env *maasEnviron) DestroyController(ctx envcontext.ProviderCallContext, controllerUUID string) error {
	// TODO(wallyworld): destroy hosted model resources
	return env.Destroy(ctx)
}

func (*maasEnviron) Provider() environs.EnvironProvider {
	return &providerInstance
}

func (env *maasEnviron) AllocateContainerAddresses(ctx context.Context, hostInstanceID instance.Id, containerTag names.MachineTag, preparedInfo corenetwork.InterfaceInfos) (corenetwork.InterfaceInfos, error) {
	if len(preparedInfo) == 0 {
		return nil, errors.Errorf("no prepared info to allocate")
	}

	logger.Debugf(ctx, "using prepared container info: %+v", preparedInfo)
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
	params, err := env.prepareDeviceDetails(ctx, deviceName, machine, preparedInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Check to see if we've already tried to allocate information for this device:
	device, err := env.checkForExistingDevice(ctx, params)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if device == nil {
		device, err = env.createAndPopulateDevice(ctx, params)
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
	interfaces, err := env.deviceInterfaceInfo(ctx, device, nameToParentName, params.CIDRToStaticRoutes)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get device interfaces")
	}
	return interfaces, nil
}

func (env *maasEnviron) ReleaseContainerAddresses(ctx envcontext.ProviderCallContext, interfaces []corenetwork.ProviderInterfaceInfo) error {
	hwAddresses := make([]string, len(interfaces))
	for i, info := range interfaces {
		hwAddresses[i] = info.HardwareAddress
	}

	devices, err := env.maasController.Devices(gomaasapi.DevicesArgs{MACAddresses: hwAddresses})
	if err != nil {
		return env.HandleCredentialError(ctx, err)
	}
	logger.Infof(ctx, "found %d MAAS devices to remove", len(devices))

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
		logger.Infof(ctx, "removed MAAS device %s", device.SystemID())
	}
	return nil
}

// AdoptResources updates all the instances to indicate they
// are now associated with the specified controller.
func (env *maasEnviron) AdoptResources(ctx envcontext.ProviderCallContext, controllerUUID string, _ version.Number) error {
	allInstances, err := env.AllInstances(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	var failed []instance.Id
	for _, inst := range allInstances {
		maasInst, ok := inst.(*maasInstance)
		if !ok {
			// This should never happen.
			return errors.Errorf("instance %q wasn't a maasInstance", inst.Id())
		}
		// From the MAAS docs: "[SetOwnerData] will not remove any
		// previous keys unless explicitly passed with an empty
		// string." So not passing all of the keys here is fine.
		// https://maas.ubuntu.com/docs2.0/api.html#machine
		err := maasInst.machine.SetOwnerData(map[string]string{tags.JujuController: controllerUUID})
		if err != nil {
			logger.Errorf(ctx, "error setting controller uuid tag for %q: %v", inst.Id(), err)
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
	ctx envcontext.ProviderCallContext, space *corenetwork.SpaceInfo,
) (*environs.ProviderSpaceInfo, error) {
	return nil, errors.NotSupportedf("provider space info")
}

// Domains gets the domains managed by MAAS. We only need the name of the
// domain at present. If more information is needed this function can be
// updated to parse and return a structure. Client code would need to be
// updated.
func (env *maasEnviron) Domains(ctx envcontext.ProviderCallContext) ([]string, error) {
	maasDomains, err := env.maasController.Domains()
	if err != nil {
		return nil, env.HandleCredentialError(ctx, err)
	}
	var result []string
	for _, domain := range maasDomains {
		result = append(result, domain.Name())
	}
	return result, nil
}
