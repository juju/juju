// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"fmt"
	"regexp"

	"gopkg.in/goose.v2/neutron"
	"gopkg.in/goose.v2/nova"
	"gopkg.in/goose.v2/swift"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	envstorage "github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

var (
	ShortAttempt   = &shortAttempt
	StorageAttempt = &storageAttempt
	CinderAttempt  = &cinderAttempt
)

func InstanceServerDetail(inst instance.Instance) *nova.ServerDetail {
	return inst.(*openstackInstance).serverDetail
}

func InstanceFloatingIP(inst instance.Instance) *string {
	return inst.(*openstackInstance).floatingIP
}

var (
	NovaListAvailabilityZones   = &novaListAvailabilityZones
	AvailabilityZoneAllocations = &availabilityZoneAllocations
	NewOpenstackStorage         = &newOpenstackStorage
)

func NewCinderVolumeSource(s OpenstackStorage) storage.VolumeSource {
	return NewCinderVolumeSourceForModel(s, testing.ModelTag.Id())
}

func NewCinderVolumeSourceForModel(s OpenstackStorage, modelUUID string) storage.VolumeSource {
	const envName = "testmodel"
	return &cinderVolumeSource{
		storageAdapter: s,
		envName:        envName,
		modelUUID:      modelUUID,
		namespace:      fakeNamespace{},
	}
}

type fakeNamespace struct {
	instance.Namespace
}

func (fakeNamespace) Value(s string) string {
	return "juju-" + s
}

func SetUpGlobalGroup(e environs.Environ, name string, apiPort int) (neutron.SecurityGroupV2, error) {
	switching := e.(*Environ).firewaller.(*switchingFirewaller)
	var ctx context.ProviderCallContext
	if err := switching.initFirewaller(); err != nil {
		return neutron.SecurityGroupV2{}, err
	}
	return switching.fw.(*neutronFirewaller).setUpGlobalGroup(ctx, name, apiPort)
}

func EnsureGroup(e environs.Environ, name string, rules []neutron.RuleInfoV2) (neutron.SecurityGroupV2, error) {
	switching := e.(*Environ).firewaller.(*switchingFirewaller)
	var ctx context.ProviderCallContext
	if err := switching.initFirewaller(); err != nil {
		return neutron.SecurityGroupV2{}, err
	}
	return switching.fw.(*neutronFirewaller).ensureGroup(ctx, name, rules)
}

func MachineGroupRegexp(e environs.Environ, machineId string) string {
	switching := e.(*Environ).firewaller.(*switchingFirewaller)
	return switching.fw.(*neutronFirewaller).machineGroupRegexp(machineId)
}

func MachineGroupName(e environs.Environ, controllerUUID, machineId string) string {
	switching := e.(*Environ).firewaller.(*switchingFirewaller)
	return switching.fw.(*neutronFirewaller).machineGroupName(controllerUUID, machineId)
}

func MatchingGroup(e environs.Environ, nameRegExp string) (neutron.SecurityGroupV2, error) {
	switching := e.(*Environ).firewaller.(*switchingFirewaller)
	var ctx context.ProviderCallContext
	if err := switching.initFirewaller(); err != nil {
		return neutron.SecurityGroupV2{}, err
	}
	return switching.fw.(*neutronFirewaller).matchingGroup(ctx, nameRegExp)
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
func ResolveNetwork(ctx context.ProviderCallContext, e environs.Environ, networkName string, external bool) (string, error) {
	return e.(*Environ).networking.ResolveNetwork(ctx, networkName, external)
}

var PortsToRuleInfo = rulesToRuleInfo
var SecGroupMatchesIngressRule = secGroupMatchesIngressRule

var MakeServiceURL = &makeServiceURL

var GetVolumeEndpointURL = getVolumeEndpointURL

func GetModelGroupNames(e environs.Environ) ([]string, error) {
	env := e.(*Environ)
	rawFirewaller := env.firewaller.(*switchingFirewaller).fw
	neutronFw, ok := rawFirewaller.(*neutronFirewaller)
	if !ok {
		return nil, fmt.Errorf("requires an env with a neutron firewaller")
	}
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
