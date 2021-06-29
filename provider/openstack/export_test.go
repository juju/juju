// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"regexp"

	"github.com/go-goose/goose/v4/neutron"
	"github.com/go-goose/goose/v4/nova"
	"github.com/go-goose/goose/v4/swift"
	"github.com/juju/collections/set"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	envstorage "github.com/juju/juju/environs/storage"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

var (
	ShortAttempt   = &shortAttempt
	StorageAttempt = &storageAttempt
	CinderAttempt  = &cinderAttempt
)

func InstanceServerDetail(inst instances.Instance) *nova.ServerDetail {
	return inst.(*openstackInstance).serverDetail
}

func InstanceFloatingIP(inst instances.Instance) *string {
	return inst.(*openstackInstance).floatingIP
}

var (
	NovaListAvailabilityZones = &novaListAvailabilityZones
	NewOpenstackStorage       = &newOpenstackStorage
)

func NewCinderVolumeSource(s OpenstackStorage, env common.ZonedEnviron) storage.VolumeSource {
	return NewCinderVolumeSourceForModel(s, testing.ModelTag.Id(), env)
}

func NewCinderVolumeSourceForModel(s OpenstackStorage, modelUUID string, env common.ZonedEnviron) storage.VolumeSource {
	const envName = "testmodel"
	return &cinderVolumeSource{
		storageAdapter: s,
		envName:        envName,
		modelUUID:      modelUUID,
		namespace:      fakeNamespace{},
		zonedEnv:       env,
	}
}

type fakeNamespace struct {
	instance.Namespace
}

func (fakeNamespace) Value(s string) string {
	return "juju-" + s
}

func SetUpGlobalGroup(e environs.Environ, ctx context.ProviderCallContext, name string, apiPort int) (neutron.SecurityGroupV2, error) {
	switching := &neutronFirewaller{firewallerBase: firewallerBase{environ: e.(*Environ)}}
	return switching.setUpGlobalGroup(name, apiPort)
}

func EnsureGroup(e environs.Environ, ctx context.ProviderCallContext, name string, rules []neutron.RuleInfoV2) (neutron.SecurityGroupV2, error) {
	switching := &neutronFirewaller{firewallerBase: firewallerBase{environ: e.(*Environ)}}
	return switching.ensureGroup(name, rules)
}

func MachineGroupRegexp(e environs.Environ, machineId string) string {
	switching := &neutronFirewaller{firewallerBase: firewallerBase{environ: e.(*Environ)}}
	return switching.machineGroupRegexp(machineId)
}

func MachineGroupName(e environs.Environ, controllerUUID, machineId string) string {
	switching := &neutronFirewaller{firewallerBase: firewallerBase{environ: e.(*Environ)}}
	return switching.machineGroupName(controllerUUID, machineId)
}

func MatchingGroup(e environs.Environ, ctx context.ProviderCallContext, nameRegExp string) (neutron.SecurityGroupV2, error) {
	switching := &neutronFirewaller{firewallerBase: firewallerBase{environ: e.(*Environ)}}
	return switching.matchingGroup(ctx, nameRegExp)
}

// ImageMetadataStorage returns a Storage object pointing where the goose
// infrastructure sets up its keystone entry for image metadata
func ImageMetadataStorage(e environs.Environ) envstorage.Storage {
	env := e.(*Environ)
	return &openstackstorage{
		containerName: "imagemetadata",
		swift:         swift.New(env.clientUnlocked),
	}
}

// CreateCustomStorage creates a swift container and returns the Storage object
// so you can put data into it.
func CreateCustomStorage(e environs.Environ, containerName string) envstorage.Storage {
	env := e.(*Environ)
	swiftClient := swift.New(env.clientUnlocked)
	if err := swiftClient.CreateContainer(containerName, swift.PublicRead); err != nil {
		panic(err)
	}
	return &openstackstorage{
		containerName: containerName,
		swift:         swiftClient,
	}
}

// BlankContainerStorage creates a Storage object with blank container name.
func BlankContainerStorage() envstorage.Storage {
	return &openstackstorage{}
}

// GetNeutronClient returns the neutron client for the current environs.
func GetNeutronClient(e environs.Environ) *neutron.Client {
	return e.(*Environ).neutron()
}

// GetNovaClient returns the nova client for the current environs.
func GetNovaClient(e environs.Environ) *nova.Client {
	return e.(*Environ).nova()
}

// ResolveNetwork exposes environ helper function resolveNetwork for testing
func ResolveNetwork(e environs.Environ, networkName string, external bool) (string, error) {
	return e.(*Environ).networking.ResolveNetwork(networkName, external)
}

// FindNetworks exposes environ helper function FindNetworks for testing
func FindNetworks(e environs.Environ, internal bool) (set.Strings, error) {
	return e.(*Environ).networking.FindNetworks(internal)
}

var PortsToRuleInfo = rulesToRuleInfo
var SecGroupMatchesIngressRule = secGroupMatchesIngressRule

var MakeServiceURL = &makeServiceURL

var GetVolumeEndpointURL = getVolumeEndpointURL

func GetModelGroupNames(e environs.Environ) ([]string, error) {
	env := e.(*Environ)
	neutronFw := env.firewaller.(*neutronFirewaller)
	groups, err := env.neutron().ListSecurityGroupsV2()
	if err != nil {
		return nil, err
	}
	modelPattern, err := regexp.Compile(neutronFw.jujuGroupRegexp())
	if err != nil {
		return nil, err
	}
	var results []string
	for _, group := range groups {
		if modelPattern.MatchString(group.Name) {
			results = append(results, group.Name)
		}
	}
	return results, nil
}

func GetFirewaller(e environs.Environ) Firewaller {
	env := e.(*Environ)
	return env.firewaller
}
