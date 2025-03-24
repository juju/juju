// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/names/v6"
	"github.com/juju/schema"
	"github.com/juju/version/v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/container"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/configschema"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/storage"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
	coretools "github.com/juju/juju/internal/tools"
)

var logger = internallogger.GetLogger("juju.provider.dummy")

var transientErrorInjection chan error

const BootstrapInstanceId = "localhost"

var errNotPrepared = errors.New("model is not prepared")

// Operation represents an action on the dummy provider.
type Operation interface{}

type OpBootstrap struct {
	Context environs.BootstrapContext
	Env     string
	Args    environs.BootstrapParams
}

type OpFinalizeBootstrap struct {
	Context        environs.BootstrapContext
	Env            string
	InstanceConfig *instancecfg.InstanceConfig
}

type OpDestroy struct {
	Env         string
	Cloud       string
	CloudRegion string
	Error       error
}

// environProvider represents the dummy provider.  There is only ever one
// instance of this type (dummy)
type environProvider struct {
	mu                     sync.Mutex
	ops                    chan<- Operation
	supportsSpaces         bool
	supportsSpaceDiscovery bool
	state                  map[string]*environState
}

// environState represents the state of an environment.
// It can be shared between several environ values,
// so that a given environment can be opened several times.
type environState struct {
	name    string
	ops     chan<- Operation
	mu      sync.Mutex
	maxId   int // maximum instance id allocated so far.
	maxAddr int // maximum allocated address last byte
	insts   map[instance.Id]*dummyInstance
	creator string
}

// environ represents a client's connection to a given environment's
// state.
type environ struct {
	environs.NoContainerAddressesEnviron

	storage.ProviderRegistry
	name         string
	modelUUID    string
	cloud        environscloudspec.CloudSpec
	ecfgMutex    sync.Mutex
	ecfgUnlocked *environConfig
}

var _ environs.Environ = (*environ)(nil)
var _ environs.Networking = (*environ)(nil)

func init() {
	environs.RegisterProvider("dummy", &dummy)
}

// dummy is the dummy environmentProvider singleton.
var dummy = environProvider{
	ops:                    nil,
	state:                  make(map[string]*environState),
	supportsSpaces:         true,
	supportsSpaceDiscovery: false,
}

// newState creates the state for a new environment with the given name.
func newState(name string, ops chan<- Operation) *environState {
	buf := make([]byte, 8192)
	buf = buf[:runtime.Stack(buf, false)]
	s := &environState{
		name:    name,
		ops:     ops,
		insts:   make(map[instance.Id]*dummyInstance),
		creator: string(buf),
	}
	return s
}

// SetSupportsSpaces allows to enable and disable SupportsSpaces for tests.
func SetSupportsSpaces(supports bool) bool {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	current := dummy.supportsSpaces
	dummy.supportsSpaces = supports
	return current
}

// SetSupportsSpaceDiscovery allows to enable and disable
// SupportsSpaceDiscovery for tests.
func SetSupportsSpaceDiscovery(supports bool) bool {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	current := dummy.supportsSpaceDiscovery
	dummy.supportsSpaceDiscovery = supports
	return current
}

// Listen directs subsequent operations on any dummy environment
// to channel c (if not nil).
func Listen(c chan<- Operation) {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	dummy.ops = c
	for _, st := range dummy.state {
		st.mu.Lock()
		st.ops = c
		st.mu.Unlock()
	}
}

var configSchema = configschema.Fields{
	"somebool": {
		Description: "Used to test config validation",
		Type:        configschema.Tbool,
	},
	"broken": {
		Description: "Whitespace-separated Environ methods that should return an error when called",
		Type:        configschema.Tstring,
	},
	"secret": {
		Description: "A secret",
		Type:        configschema.Tstring,
	},
}

var configFields = func() schema.Fields {
	fs, _, err := configSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
}()

var configDefaults = schema.Defaults{
	"broken":   "",
	"secret":   "pork",
	"somebool": false,
}

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (c *environConfig) broken() string {
	return c.attrs["broken"].(string)
}

func (p *environProvider) newConfig(ctx context.Context, cfg *config.Config) (*environConfig, error) {
	valid, err := p.Validate(ctx, cfg, nil)
	if err != nil {
		return nil, err
	}
	return &environConfig{valid, valid.UnknownAttrs()}, nil
}

func (p *environProvider) Schema() configschema.Fields {
	fields, err := config.Schema(configSchema)
	if err != nil {
		panic(err)
	}
	return fields
}

var _ config.ConfigSchemaSource = (*environProvider)(nil)

// ConfigSchema returns extra config attributes specific
// to this provider only.
func (p *environProvider) ConfigSchema() schema.Fields {
	return configFields
}

// ConfigDefaults returns the default values for the
// provider specific config attributes.
func (p *environProvider) ConfigDefaults() schema.Defaults {
	return configDefaults
}

func (*environProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.EmptyAuthType: {},
		cloud.UserPassAuthType: {
			{
				Name: "username", CredentialAttr: cloud.CredentialAttr{Description: "The username to authenticate with."},
			}, {
				Name: "password", CredentialAttr: cloud.CredentialAttr{
					Description: "The password for the specified username.",
					Hidden:      true,
				},
			},
		},
	}
}

func (*environProvider) DetectCredentials(cloudName string) (*cloud.CloudCredential, error) {
	return cloud.NewEmptyCloudCredential(), nil
}

func (*environProvider) FinalizeCredential(_ environs.FinalizeCredentialContext, args environs.FinalizeCredentialParams) (*cloud.Credential, error) {
	return &args.Credential, nil
}

func (*environProvider) DetectRegions() ([]cloud.Region, error) {
	return []cloud.Region{{Name: "dummy"}}, nil
}

func (p *environProvider) Validate(ctx context.Context, cfg, old *config.Config) (valid *config.Config, err error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(ctx, cfg, old); err != nil {
		return nil, err
	}
	validated, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, err
	}
	// Apply the coerced unknown values back into the config.
	return cfg.Apply(validated)
}

func (e *environ) state() (*environState, error) {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	state, ok := dummy.state[e.modelUUID]
	if !ok {
		return nil, errNotPrepared
	}
	return state, nil
}

// Version is part of the EnvironProvider interface.
func (*environProvider) Version() int {
	return 0
}

func (p *environProvider) Open(ctx context.Context, args environs.OpenParams, invalidator environs.CredentialInvalidator) (environs.Environ, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ecfg, err := p.newConfig(ctx, args.Config)
	if err != nil {
		return nil, err
	}
	env := &environ{
		ProviderRegistry: dummystorage.StorageProviders(),
		name:             ecfg.Name(),
		modelUUID:        args.Config.UUID(),
		cloud:            args.Cloud,
		ecfgUnlocked:     ecfg,
	}
	if err := env.checkBroken("Open"); err != nil {
		return nil, err
	}
	return env, nil
}

// CloudSchema returns the schema used to validate input for add-cloud.  Since
// this provider does not support custom clouds, this always returns nil.
func (p *environProvider) CloudSchema() *jsonschema.Schema {
	return nil
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p *environProvider) Ping(_ context.Context, _ string) error {
	return errors.NotImplementedf("Ping")
}

// ModelConfigDefaults provides a set of default model config attributes that
// should be set on a models config if they have not been specified by the user.
func (p *environProvider) ModelConfigDefaults(_ context.Context) (map[string]any, error) {
	return nil, nil
}

// ValidateCloud is specified in the EnvironProvider interface.
func (p *environProvider) ValidateCloud(ctx context.Context, spec environscloudspec.CloudSpec) error {
	return nil
}

func (e *environ) ecfg() *environConfig {
	e.ecfgMutex.Lock()
	ecfg := e.ecfgUnlocked
	e.ecfgMutex.Unlock()
	return ecfg
}

func (e *environ) checkBroken(method string) error {
	for _, m := range strings.Fields(e.ecfg().broken()) {
		if m == method {
			return fmt.Errorf("dummy.%s is broken", method)
		}
	}
	return nil
}

// PrecheckInstance is specified in the environs.InstancePrechecker interface.
func (*environ) PrecheckInstance(ctx envcontext.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	if args.Placement != "" && args.Placement != "valid" {
		return fmt.Errorf("%s placement is invalid", args.Placement)
	}
	return nil
}

// ValidateProviderForNewModel is part of the [environs.ModelResources] interface.
func (e *environ) ValidateProviderForNewModel(ctx context.Context) error {
	return nil
}

// CreateModelResources is part of the [environs.ModelResources] interface.
func (e *environ) CreateModelResources(ctx context.Context, args environs.CreateParams) error {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	dummy.state[e.modelUUID] = newState(e.name, dummy.ops)
	return nil
}

// PrepareForBootstrap is part of the Environ interface.
func (e *environ) PrepareForBootstrap(ctx environs.BootstrapContext, controllerName string) error {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()

	// The environment has not been prepared, so create it and record it.
	// We don't start listening for State or API connections until
	// Bootstrap has been called.
	envState := newState(e.name, dummy.ops)
	dummy.state[e.modelUUID] = envState

	return nil
}

func (e *environ) Bootstrap(ctx environs.BootstrapContext, callCtx envcontext.ProviderCallContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	availableTools, err := args.AvailableTools.Match(coretools.Filter{OSType: "ubuntu"})
	if err != nil {
		return nil, err
	}
	arch, err := availableTools.OneArch()
	if err != nil {
		return nil, errors.Trace(err)
	}

	defer delay()
	if err := e.checkBroken("Bootstrap"); err != nil {
		return nil, err
	}
	if _, ok := args.ControllerConfig.CACert(); !ok {
		return nil, errors.New("no CA certificate in controller configuration")
	}

	logger.Infof(ctx, "would pick agent binaries from %s", availableTools)

	estate, err := e.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()

	// Create an instance for the bootstrap node.
	logger.Infof(ctx, "creating bootstrap instance")
	i := &dummyInstance{
		id:           BootstrapInstanceId,
		addresses:    network.NewMachineAddresses([]string{"localhost"}).AsProviderAddresses(),
		machineId:    agent.BootstrapControllerId,
		firewallMode: e.Config().FirewallMode(),
		state:        estate,
		controller:   true,
	}
	estate.insts[i.id] = i
	if estate.ops != nil {
		estate.ops <- OpBootstrap{Context: ctx, Env: e.name, Args: args}
	}

	finalize := func(ctx environs.BootstrapContext, icfg *instancecfg.InstanceConfig, _ environs.BootstrapDialOpts) (err error) {
		icfg.Bootstrap.BootstrapMachineInstanceId = BootstrapInstanceId
		if err := instancecfg.FinishInstanceConfig(icfg, e.Config()); err != nil {
			return err
		}

		adminUser := names.NewUserTag("admin@local")
		var cloudCredentialTag names.CloudCredentialTag
		if icfg.Bootstrap.ControllerCloudCredentialName != "" {
			id := fmt.Sprintf(
				"%s/%s/%s",
				icfg.Bootstrap.ControllerCloud.Name,
				adminUser.Id(),
				icfg.Bootstrap.ControllerCloudCredentialName,
			)
			if !names.IsValidCloudCredential(id) {
				return errors.NotValidf("cloud credential ID %q", id)
			}
			cloudCredentialTag = names.NewCloudCredentialTag(id)
		}

		cloudCredentials := make(map[names.CloudCredentialTag]cloud.Credential)
		if icfg.Bootstrap.ControllerCloudCredential != nil && icfg.Bootstrap.ControllerCloudCredentialName != "" {
			cloudCredentials[cloudCredentialTag] = *icfg.Bootstrap.ControllerCloudCredential
		}
		if estate.ops != nil {
			estate.ops <- OpFinalizeBootstrap{Context: ctx, Env: e.name, InstanceConfig: icfg}
		}
		return nil
	}

	bsResult := &environs.BootstrapResult{
		Arch:                    arch,
		Base:                    corebase.MakeDefaultBase("ubuntu", "22.04"),
		CloudBootstrapFinalizer: finalize,
	}
	return bsResult, nil
}

func (e *environ) ControllerInstances(envcontext.ProviderCallContext, string) ([]instance.Id, error) {
	estate, err := e.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	if err := e.checkBroken("ControllerInstances"); err != nil {
		return nil, err
	}
	var controllerInstances []instance.Id
	for _, v := range estate.insts {
		if v.controller {
			controllerInstances = append(controllerInstances, v.Id())
		}
	}
	return controllerInstances, nil
}

func (e *environ) Config() *config.Config {
	return e.ecfg().Config
}

func (e *environ) SetConfig(ctx context.Context, cfg *config.Config) error {
	if err := e.checkBroken("SetConfig"); err != nil {
		return err
	}
	ecfg, err := dummy.newConfig(ctx, cfg)
	if err != nil {
		return err
	}
	e.ecfgMutex.Lock()
	e.ecfgUnlocked = ecfg
	e.ecfgMutex.Unlock()
	return nil
}

// AdoptResources is part of the Environ interface.
func (e *environ) AdoptResources(envcontext.ProviderCallContext, string, version.Number) error {
	// This provider doesn't track instance -> controller.
	return nil
}

func (e *environ) Destroy(envcontext.ProviderCallContext) (res error) {
	defer delay()
	estate, err := e.state()
	if err != nil {
		if err == errNotPrepared {
			return nil
		}
		return err
	}
	defer func() {
		// The estate is a pointer to a structure that is stored in the dummy global.
		// The Listen method can change the ops channel of any state, and will do so
		// under the covers. What we need to do is use the state mutex to add a memory
		// barrier such that the ops channel we see here is the latest.
		estate.mu.Lock()
		ops := estate.ops
		name := estate.name
		delete(dummy.state, e.modelUUID)
		estate.mu.Unlock()
		if ops != nil {
			ops <- OpDestroy{
				Env:         name,
				Cloud:       e.cloud.Name,
				CloudRegion: e.cloud.Region,
				Error:       res,
			}
		}
	}()
	if err := e.checkBroken("Destroy"); err != nil {
		return err
	}
	return nil
}

func (e *environ) DestroyController(ctx envcontext.ProviderCallContext, _ string) error {
	if err := e.Destroy(ctx); err != nil {
		return err
	}
	return nil
}

var unsupportedConstraints = []string{
	constraints.CpuPower,
	constraints.VirtType,
	constraints.ImageID,
}

// ConstraintsValidator is defined on the Environs interface.
func (e *environ) ConstraintsValidator(envcontext.ProviderCallContext) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
	validator.RegisterVocabulary(constraints.Arch, []string{arch.AMD64, arch.ARM64, arch.PPC64EL, arch.S390X, arch.RISCV64})
	return validator, nil
}

// StartInstance is specified in the InstanceBroker interface.
func (e *environ) StartInstance(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	defer delay()
	machineId := args.InstanceConfig.MachineId
	logger.Infof(ctx, "dummy startinstance, machine %s", machineId)
	if err := e.checkBroken("StartInstance"); err != nil {
		return nil, err
	}
	estate, err := e.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()

	// check if an error has been injected on the transientErrorInjection channel (testing purposes)
	select {
	case injectedError := <-transientErrorInjection:
		return nil, injectedError
	default:
	}

	if args.InstanceConfig.MachineNonce == "" {
		return nil, errors.New("cannot start instance: missing machine nonce")
	}
	if args.InstanceConfig.IsController() {
		if args.InstanceConfig.APIInfo.Tag != names.NewMachineTag(machineId) {
			return nil, errors.New("entity tag must match started machine")
		}
	}
	if args.InstanceConfig.APIInfo.Tag != names.NewMachineTag(machineId) {
		return nil, errors.New("entity tag must match started machine")
	}
	logger.Infof(ctx, "would pick agent binaries from %s", args.Tools)

	idString := fmt.Sprintf("%s-%d", e.name, estate.maxId)
	// Add the addresses we want to see in the machine doc. This means both
	// IPv4 and IPv6 loopback, as well as the DNS name.
	addrs := network.NewMachineAddresses([]string{idString + ".dns", "127.0.0.1", "::1"}).AsProviderAddresses()
	logger.Debugf(ctx, "StartInstance addresses: %v", addrs)
	i := &dummyInstance{
		id:           instance.Id(idString),
		addresses:    addrs,
		machineId:    machineId,
		firewallMode: e.Config().FirewallMode(),
		state:        estate,
	}

	var hc *instance.HardwareCharacteristics
	// To match current system capability, only provide hardware characteristics for
	// environ machines, not containers.
	if container.ParentId(machineId) == "" {
		// Assume that the provided Availability Zone won't fail,
		// though one is required.
		var zone string
		if args.Placement != "" {
			split := strings.Split(args.Placement, "=")
			if len(split) == 2 && split[0] == "zone" {
				zone = split[1]
			}
		}
		if zone == "" && args.AvailabilityZone != "" {
			zone = args.AvailabilityZone
		}

		// We will just assume the instance hardware characteristics exactly matches
		// the supplied constraints (if specified).
		hc = &instance.HardwareCharacteristics{
			Arch:     args.Constraints.Arch,
			Mem:      args.Constraints.Mem,
			RootDisk: args.Constraints.RootDisk,
			CpuCores: args.Constraints.CpuCores,
			CpuPower: args.Constraints.CpuPower,
			Tags:     args.Constraints.Tags,
		}
		if zone != "" {
			hc.AvailabilityZone = &zone
		}
		// Fill in some expected instance hardware characteristics if constraints not specified.
		if hc.Arch == nil {
			defaultArch := arch.DefaultArchitecture
			hc.Arch = &defaultArch
		}
		if hc.Mem == nil {
			mem := uint64(1024)
			hc.Mem = &mem
		}
		if hc.RootDisk == nil {
			disk := uint64(8192)
			hc.RootDisk = &disk
		}
		if hc.CpuCores == nil {
			cores := uint64(1)
			hc.CpuCores = &cores
		}
	}
	// Simulate subnetsToZones gets populated when spaces given in constraints.
	spaces := args.Constraints.IncludeSpaces()
	var subnetsToZones map[network.Id][]string
	for isp := range spaces {
		// Simulate 2 subnets per space.
		if subnetsToZones == nil {
			subnetsToZones = make(map[network.Id][]string)
		}
		for isn := 0; isn < 2; isn++ {
			providerId := fmt.Sprintf("subnet-%d", isp+isn)
			zone := fmt.Sprintf("zone%d", isp+isn)
			subnetsToZones[network.Id(providerId)] = []string{zone}
		}
	}
	// Simulate creating volumes when requested.
	volumes := make([]storage.Volume, len(args.Volumes))
	for iv, v := range args.Volumes {
		persistent, _ := v.Attributes["persistent"].(bool)
		volumes[iv] = storage.Volume{
			Tag: v.Tag,
			VolumeInfo: storage.VolumeInfo{
				Size:       v.Size,
				Persistent: persistent,
			},
		}
	}
	// Simulate attaching volumes when requested.
	volumeAttachments := make([]storage.VolumeAttachment, len(args.VolumeAttachments))
	for iv, v := range args.VolumeAttachments {
		volumeAttachments[iv] = storage.VolumeAttachment{
			Volume:  v.Volume,
			Machine: v.Machine,
			VolumeAttachmentInfo: storage.VolumeAttachmentInfo{
				DeviceName: fmt.Sprintf("sd%c", 'b'+rune(iv)),
				ReadOnly:   v.ReadOnly,
			},
		}
	}
	estate.insts[i.id] = i
	estate.maxId++
	return &environs.StartInstanceResult{
		Instance: i,
		Hardware: hc,
	}, nil
}

func (e *environ) StopInstances(_ envcontext.ProviderCallContext, ids ...instance.Id) error {
	defer delay()
	if err := e.checkBroken("StopInstance"); err != nil {
		return err
	}
	estate, err := e.state()
	if err != nil {
		return err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	for _, id := range ids {
		delete(estate.insts, id)
	}
	return nil
}

func (e *environ) Instances(_ context.Context, ids []instance.Id) (insts []instances.Instance, err error) {
	defer delay()
	if err := e.checkBroken("Instances"); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	estate, err := e.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	notFound := 0
	for _, id := range ids {
		inst := estate.insts[id]
		if inst == nil {
			err = environs.ErrPartialInstances
			notFound++
			insts = append(insts, nil)
		} else {
			insts = append(insts, inst)
		}
	}
	if notFound == len(ids) {
		return nil, environs.ErrNoInstances
	}
	return
}

// SupportsSpaces is specified on environs.Networking.
func (env *environ) SupportsSpaces() (bool, error) {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	if !dummy.supportsSpaces {
		return false, errors.NotSupportedf("spaces")
	}
	return true, nil
}

// SupportsSpaceDiscovery is specified on environs.Networking.
func (env *environ) SupportsSpaceDiscovery() (bool, error) {
	if err := env.checkBroken("SupportsSpaceDiscovery"); err != nil {
		return false, err
	}
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	if !dummy.supportsSpaceDiscovery {
		return false, nil
	}
	return true, nil
}

// Spaces is specified on environs.Networking.
func (env *environ) Spaces(_ envcontext.ProviderCallContext) (network.SpaceInfos, error) {
	if err := env.checkBroken("Spaces"); err != nil {
		return []network.SpaceInfo{}, err
	}
	return []network.SpaceInfo{{
		Name:       "foo",
		ProviderId: network.Id("0"),
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("1"),
			AvailabilityZones: []string{"zone1"},
		}, {
			ProviderId:        network.Id("2"),
			AvailabilityZones: []string{"zone1"},
		}}}, {
		Name:       "Another Foo 99!",
		ProviderId: "1",
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("3"),
			AvailabilityZones: []string{"zone1"},
		}}}, {
		Name:       "foo-",
		ProviderId: "2",
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("4"),
			AvailabilityZones: []string{"zone1"},
		}}}, {
		Name:       "---",
		ProviderId: "3",
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("5"),
			AvailabilityZones: []string{"zone1"},
		}}}}, nil
}

// NetworkInterfaces implements Environ.NetworkInterfaces().
func (env *environ) NetworkInterfaces(_ envcontext.ProviderCallContext, ids []instance.Id) ([]network.InterfaceInfos, error) {
	if err := env.checkBroken("NetworkInterfaces"); err != nil {
		return nil, err
	}

	estate, err := env.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()

	// Simulate 3 NICs - primary and secondary enabled plus a disabled NIC.
	// all configured using DHCP and having fake DNS servers and gateway.
	infos := make([]network.InterfaceInfos, len(ids))
	for idIndex := range ids {
		infos[idIndex] = make(network.InterfaceInfos, 3)
		for i, netName := range []string{"private", "public", "disabled"} {
			infos[idIndex][i] = network.InterfaceInfo{
				DeviceIndex:      i,
				ProviderId:       network.Id(fmt.Sprintf("dummy-eth%d", i)),
				ProviderSubnetId: network.Id("dummy-" + netName),
				InterfaceType:    network.EthernetDevice,
				InterfaceName:    fmt.Sprintf("eth%d", i),
				VLANTag:          i,
				MACAddress:       fmt.Sprintf("aa:bb:cc:dd:ee:f%d", i),
				Disabled:         i == 2,
				NoAutoStart:      i%2 != 0,
				Addresses: network.ProviderAddresses{
					network.NewMachineAddress(
						fmt.Sprintf("0.%d.0.%d", (i+1)*10+idIndex, estate.maxAddr+2),
						network.WithCIDR(fmt.Sprintf("0.%d.0.0/24", (i+1)*10)),
						network.WithConfigType(network.ConfigDHCP),
					).AsProviderAddress(),
				},
				DNSServers: network.NewMachineAddresses([]string{"ns1.dummy", "ns2.dummy"}).AsProviderAddresses(),
				GatewayAddress: network.NewMachineAddress(
					fmt.Sprintf("0.%d.0.1", (i+1)*10+idIndex),
				).AsProviderAddress(),
				Origin: network.OriginProvider,
			}
		}
	}

	return infos, nil
}

type azShim struct {
	name      string
	available bool
}

func (az azShim) Name() string {
	return az.name
}

func (az azShim) Available() bool {
	return az.available
}

// AvailabilityZones implements environs.ZonedEnviron.
func (env *environ) AvailabilityZones(ctx context.Context) (network.AvailabilityZones, error) {
	return network.AvailabilityZones{
		azShim{"zone1", true},
		azShim{"zone2", false},
		azShim{"zone3", true},
		azShim{"zone4", true},
	}, nil
}

// InstanceAvailabilityZoneNames implements environs.ZonedEnviron.
func (env *environ) InstanceAvailabilityZoneNames(ctx envcontext.ProviderCallContext, ids []instance.Id) (map[instance.Id]string, error) {
	if err := env.checkBroken("InstanceAvailabilityZoneNames"); err != nil {
		return nil, errors.NotSupportedf("instance availability zones")
	}
	availabilityZones, err := env.AvailabilityZones(ctx)
	if err != nil {
		return nil, err
	}
	azMaxIndex := len(availabilityZones) - 1
	azIndex := 0
	returnValue := make(map[instance.Id]string, 0)
	for _, id := range ids {
		if availabilityZones[azIndex].Available() {
			returnValue[id] = availabilityZones[azIndex].Name()
		} else {
			// Based on knowledge of how the AZs are set up above
			// in AvailabilityZones()
			azIndex++
			returnValue[id] = availabilityZones[azIndex].Name()
		}
		azIndex++
		if azIndex == azMaxIndex {
			azIndex = 0
		}
	}
	return returnValue, nil
}

// DeriveAvailabilityZones is part of the common.ZonedEnviron interface.
func (env *environ) DeriveAvailabilityZones(envcontext.ProviderCallContext, environs.StartInstanceParams) ([]string, error) {
	return nil, nil
}

// Subnets implements environs.Environ.Subnets.
func (env *environ) Subnets(
	ctx envcontext.ProviderCallContext, instId instance.Id, subnetIds []network.Id,
) ([]network.SubnetInfo, error) {
	if err := env.checkBroken("Subnets"); err != nil {
		return nil, err
	}

	estate, err := env.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()

	if ok, _ := env.SupportsSpaceDiscovery(); ok {
		// Space discovery needs more subnets to work with.
		return env.subnetsForSpaceDiscovery(estate)
	}

	allSubnets := []network.SubnetInfo{{
		CIDR:              "0.10.0.0/24",
		ProviderId:        "dummy-private",
		AvailabilityZones: []string{"zone1", "zone2"},
	}, {
		CIDR:       "0.20.0.0/24",
		ProviderId: "dummy-public",
	}}

	// Filter result by ids, if given.
	var result []network.SubnetInfo
	for _, subId := range subnetIds {
		switch subId {
		case "dummy-private":
			result = append(result, allSubnets[0])
		case "dummy-public":
			result = append(result, allSubnets[1])
		}
	}
	if len(subnetIds) == 0 {
		result = append([]network.SubnetInfo{}, allSubnets...)
	}
	return result, nil
}

func (env *environ) subnetsForSpaceDiscovery(estate *environState) ([]network.SubnetInfo, error) {
	result := []network.SubnetInfo{{
		ProviderId:        network.Id("1"),
		AvailabilityZones: []string{"zone1"},
		CIDR:              "192.168.1.0/24",
	}, {
		ProviderId:        network.Id("2"),
		AvailabilityZones: []string{"zone1"},
		CIDR:              "192.168.2.0/24",
		VLANTag:           1,
	}, {
		ProviderId:        network.Id("3"),
		AvailabilityZones: []string{"zone1"},
		CIDR:              "192.168.3.0/24",
	}, {
		ProviderId:        network.Id("4"),
		AvailabilityZones: []string{"zone1"},
		CIDR:              "192.168.4.0/24",
	}, {
		ProviderId:        network.Id("5"),
		AvailabilityZones: []string{"zone1"},
		CIDR:              "192.168.5.0/24",
	}}
	return result, nil
}

func (e *environ) AllInstances(ctx context.Context) ([]instances.Instance, error) {
	return e.instancesForMethod(ctx, "AllInstances")
}

func (e *environ) AllRunningInstances(ctx context.Context) ([]instances.Instance, error) {
	return e.instancesForMethod(ctx, "AllRunningInstances")
}

func (e *environ) instancesForMethod(_ context.Context, method string) ([]instances.Instance, error) {
	defer delay()
	if err := e.checkBroken(method); err != nil {
		return nil, err
	}
	var insts []instances.Instance
	estate, err := e.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	for _, v := range estate.insts {
		insts = append(insts, v)
	}
	return insts, nil
}

func (*environ) Provider() environs.EnvironProvider {
	return &dummy
}

type dummyInstance struct {
	state        *environState
	id           instance.Id
	status       string
	machineId    string
	firewallMode string
	controller   bool

	mu        sync.Mutex
	addresses []network.ProviderAddress
	broken    []string
}

func (inst *dummyInstance) Id() instance.Id {
	return inst.id
}

func (inst *dummyInstance) Status(envcontext.ProviderCallContext) instance.Status {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	// TODO(perrito666) add a provider status -> juju status mapping.
	jujuStatus := status.Pending
	if inst.status != "" {
		dummyStatus := status.Status(inst.status)
		if dummyStatus.KnownInstanceStatus() {
			jujuStatus = dummyStatus
		}
	}

	return instance.Status{
		Status:  jujuStatus,
		Message: inst.status,
	}

}

// SetInstanceAddresses sets the addresses associated with the given
// dummy instance.
func SetInstanceAddresses(inst instances.Instance, addrs []network.ProviderAddress) {
	inst0 := inst.(*dummyInstance)
	inst0.mu.Lock()
	inst0.addresses = append(inst0.addresses[:0], addrs...)
	logger.Debugf(context.TODO(), "setting instance %q addresses to %v", inst0.Id(), addrs)
	inst0.mu.Unlock()
}

// SetInstanceStatus sets the status associated with the given
// dummy instance.
func SetInstanceStatus(inst instances.Instance, status string) {
	inst0 := inst.(*dummyInstance)
	inst0.mu.Lock()
	inst0.status = status
	inst0.mu.Unlock()
}

func (inst *dummyInstance) checkBroken(method string) error {
	for _, m := range inst.broken {
		if m == method {
			return fmt.Errorf("dummyInstance.%s is broken", method)
		}
	}
	return nil
}

func (inst *dummyInstance) Addresses(envcontext.ProviderCallContext) (network.ProviderAddresses, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if err := inst.checkBroken("Addresses"); err != nil {
		return nil, err
	}
	return append([]network.ProviderAddress{}, inst.addresses...), nil
}

// providerDelay controls the delay before dummy responds.
// non empty values in JUJU_DUMMY_DELAY will be parsed as
// time.Durations into this value.
var providerDelay, _ = time.ParseDuration(os.Getenv("JUJU_DUMMY_DELAY")) // parse errors are ignored

// pause execution to simulate the latency of a real provider
func delay() {
	if providerDelay > 0 {
		logger.Infof(context.TODO(), "pausing for %v", providerDelay)
		<-time.After(providerDelay)
	}
}

// ProviderSpaceInfo implements NetworkingEnviron.
func (*environ) ProviderSpaceInfo(envcontext.ProviderCallContext, *network.SpaceInfo) (*environs.ProviderSpaceInfo, error) {
	return nil, errors.NotSupportedf("provider space info")
}

// MaybeWriteLXDProfile implements environs.LXDProfiler.
func (*environ) MaybeWriteLXDProfile(string, lxdprofile.Profile) error {
	return nil
}

// LXDProfileNames implements environs.LXDProfiler.
func (*environ) LXDProfileNames(string) ([]string, error) {
	return nil, nil
}

// AssignLXDProfiles implements environs.LXDProfiler.
func (*environ) AssignLXDProfiles(_ string, profilesNames []string, _ []lxdprofile.ProfilePost) (current []string, err error) {
	return profilesNames, nil
}
