// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"reflect"
	"time"

	autorestazure "github.com/Azure/azure-sdk-for-go/Godeps/_workspace/src/github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/azure-sdk-for-go/Godeps/_workspace/src/github.com/Azure/go-autorest/autorest/mocks"
	"github.com/Azure/azure-sdk-for-go/Godeps/_workspace/src/github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/Azure/azure-sdk-for-go/arm/resources"
	"github.com/Azure/azure-sdk-for-go/arm/storage"
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tags"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/azure"
	"github.com/juju/juju/provider/azure/internal/azuretesting"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/version"
)

type environSuite struct {
	testing.BaseSuite

	provider      environs.EnvironProvider
	requests      []*http.Request
	storageClient azuretesting.MockStorageClient
	sender        azuretesting.Senders
	retryClock    mockClock

	tags                          map[string]*string
	group                         *resources.ResourceGroup
	vmSizes                       *compute.VirtualMachineSizeListResult
	storageNameAvailabilityResult *storage.CheckNameAvailabilityResult
	storageAccount                *storage.Account
	storageAccountKeys            *storage.AccountKeys
	vnet                          *network.VirtualNetwork
	nsg                           *network.SecurityGroup
	subnet                        *network.Subnet
	ubuntuServerSKUs              []compute.VirtualMachineImageResource
	publicIPAddress               *network.PublicIPAddress
	oldNetworkInterfaces          *network.InterfaceListResult
	newNetworkInterface           *network.Interface
	jujuAvailabilitySet           *compute.AvailabilitySet
	virtualMachine                *compute.VirtualMachine
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.storageClient = azuretesting.MockStorageClient{}
	s.sender = nil
	s.requests = nil
	s.retryClock = mockClock{Clock: testing.NewClock(time.Time{})}

	s.provider, _ = newProviders(c, azure.ProviderConfig{
		Sender:           azuretesting.NewSerialSender(&s.sender),
		RequestInspector: requestRecorder(&s.requests),
		NewStorageClient: s.storageClient.NewClient,
		RetryClock: &testing.AutoAdvancingClock{
			&s.retryClock, s.retryClock.Advance,
		},
	})

	envTags := map[string]*string{
		"juju-model-uuid":      to.StringPtr(testing.ModelTag.Id()),
		"juju-controller-uuid": to.StringPtr(testing.ModelTag.Id()),
	}
	s.tags = map[string]*string{
		"juju-machine-name": to.StringPtr("machine-0"),
	}

	s.group = &resources.ResourceGroup{
		Location: to.StringPtr("westus"),
		Tags:     &envTags,
	}

	vmSizes := []compute.VirtualMachineSize{{
		Name:                 to.StringPtr("Standard_D1"),
		NumberOfCores:        to.IntPtr(1),
		OsDiskSizeInMB:       to.IntPtr(1047552),
		ResourceDiskSizeInMB: to.IntPtr(51200),
		MemoryInMB:           to.IntPtr(3584),
		MaxDataDiskCount:     to.IntPtr(2),
	}}
	s.vmSizes = &compute.VirtualMachineSizeListResult{Value: &vmSizes}

	s.storageNameAvailabilityResult = &storage.CheckNameAvailabilityResult{
		NameAvailable: to.BoolPtr(true),
	}

	s.storageAccount = &storage.Account{
		Name: to.StringPtr("my-storage-account"),
		Type: to.StringPtr("Standard_LRS"),
		Tags: &envTags,
		Properties: &storage.AccountProperties{
			PrimaryEndpoints: &storage.Endpoints{
				Blob: to.StringPtr(fmt.Sprintf("https://%s.blob.storage.azurestack.local/", fakeStorageAccount)),
			},
		},
	}

	s.storageAccountKeys = &storage.AccountKeys{
		Key1: to.StringPtr("key-1"),
	}

	addressPrefixes := []string{"10.0.0.0/16"}
	s.vnet = &network.VirtualNetwork{
		ID:       to.StringPtr("juju-internal-network"),
		Name:     to.StringPtr("juju-internal-network"),
		Location: to.StringPtr("westus"),
		Tags:     &envTags,
		Properties: &network.VirtualNetworkPropertiesFormat{
			AddressSpace: &network.AddressSpace{&addressPrefixes},
		},
	}

	s.nsg = &network.SecurityGroup{
		ID: to.StringPtr(path.Join(
			"/subscriptions", fakeSubscriptionId,
			"resourceGroups", "juju-testenv-model-"+testing.ModelTag.Id(),
			"providers/Microsoft.Network/networkSecurityGroups/juju-internal-nsg",
		)),
		Tags: &envTags,
	}

	s.subnet = &network.Subnet{
		ID:   to.StringPtr("subnet-id"),
		Name: to.StringPtr("juju-internal-subnet"),
		Properties: &network.SubnetPropertiesFormat{
			AddressPrefix:        to.StringPtr("10.0.0.0/16"),
			NetworkSecurityGroup: &network.SubResource{s.nsg.ID},
		},
	}

	s.ubuntuServerSKUs = []compute.VirtualMachineImageResource{
		{Name: to.StringPtr("12.04-LTS")},
		{Name: to.StringPtr("12.10")},
		{Name: to.StringPtr("14.04-LTS")},
		{Name: to.StringPtr("15.04")},
		{Name: to.StringPtr("15.10")},
		{Name: to.StringPtr("16.04-LTS")},
	}

	s.publicIPAddress = &network.PublicIPAddress{
		ID:       to.StringPtr("public-ip-id"),
		Name:     to.StringPtr("machine-0-public-ip"),
		Location: to.StringPtr("westus"),
		Tags:     &s.tags,
		Properties: &network.PublicIPAddressPropertiesFormat{
			PublicIPAllocationMethod: network.Dynamic,
			IPAddress:                to.StringPtr("1.2.3.4"),
		},
	}

	// Existing IPs/NICs. These are the results of querying NICs so we
	// can tell which IP to allocate.
	oldIPConfigurations := []network.InterfaceIPConfiguration{{
		ID:   to.StringPtr("ip-configuration-0-id"),
		Name: to.StringPtr("ip-configuration-0"),
		Properties: &network.InterfaceIPConfigurationPropertiesFormat{
			PrivateIPAddress:          to.StringPtr("10.0.0.4"),
			PrivateIPAllocationMethod: network.Static,
			Subnet: &network.SubResource{ID: s.subnet.ID},
		},
	}}
	oldNetworkInterfaces := []network.Interface{{
		ID:   to.StringPtr("network-interface-0-id"),
		Name: to.StringPtr("network-interface-0"),
		Properties: &network.InterfacePropertiesFormat{
			IPConfigurations: &oldIPConfigurations,
			Primary:          to.BoolPtr(true),
		},
	}}
	s.oldNetworkInterfaces = &network.InterfaceListResult{
		Value: &oldNetworkInterfaces,
	}

	// The newly created IP/NIC.
	newIPConfigurations := []network.InterfaceIPConfiguration{{
		ID:   to.StringPtr("ip-configuration-1-id"),
		Name: to.StringPtr("primary"),
		Properties: &network.InterfaceIPConfigurationPropertiesFormat{
			PrivateIPAddress:          to.StringPtr("10.0.0.5"),
			PrivateIPAllocationMethod: network.Static,
			Subnet:          &network.SubResource{ID: s.subnet.ID},
			PublicIPAddress: &network.SubResource{ID: s.publicIPAddress.ID},
		},
	}}
	s.newNetworkInterface = &network.Interface{
		ID:       to.StringPtr("network-interface-1-id"),
		Name:     to.StringPtr("network-interface-1"),
		Location: to.StringPtr("westus"),
		Tags:     &s.tags,
		Properties: &network.InterfacePropertiesFormat{
			IPConfigurations: &newIPConfigurations,
		},
	}

	s.jujuAvailabilitySet = &compute.AvailabilitySet{
		ID:       to.StringPtr("juju-availability-set-id"),
		Name:     to.StringPtr("juju"),
		Location: to.StringPtr("westus"),
		Tags:     &envTags,
	}

	sshPublicKeys := []compute.SSHPublicKey{{
		Path:    to.StringPtr("/home/ubuntu/.ssh/authorized_keys"),
		KeyData: to.StringPtr(testing.FakeAuthKeys),
	}}
	networkInterfaceReferences := []compute.NetworkInterfaceReference{{
		ID: s.newNetworkInterface.ID,
		Properties: &compute.NetworkInterfaceReferenceProperties{
			Primary: to.BoolPtr(true),
		},
	}}
	s.virtualMachine = &compute.VirtualMachine{
		ID:       to.StringPtr("machine-0-id"),
		Name:     to.StringPtr("machine-0"),
		Location: to.StringPtr("westus"),
		Tags:     &s.tags,
		Properties: &compute.VirtualMachineProperties{
			HardwareProfile: &compute.HardwareProfile{
				VMSize: "Standard_D1",
			},
			StorageProfile: &compute.StorageProfile{
				ImageReference: &compute.ImageReference{
					Publisher: to.StringPtr("Canonical"),
					Offer:     to.StringPtr("UbuntuServer"),
					Sku:       to.StringPtr("12.10"),
					Version:   to.StringPtr("latest"),
				},
				OsDisk: &compute.OSDisk{
					Name:         to.StringPtr("machine-0"),
					CreateOption: compute.FromImage,
					Caching:      compute.ReadWrite,
					Vhd: &compute.VirtualHardDisk{
						URI: to.StringPtr(fmt.Sprintf(
							"https://%s.blob.storage.azurestack.local/osvhds/machine-0.vhd",
							fakeStorageAccount,
						)),
					},
				},
			},
			OsProfile: &compute.OSProfile{
				ComputerName:  to.StringPtr("machine-0"),
				CustomData:    to.StringPtr("<juju-goes-here>"),
				AdminUsername: to.StringPtr("ubuntu"),
				LinuxConfiguration: &compute.LinuxConfiguration{
					DisablePasswordAuthentication: to.BoolPtr(true),
					SSH: &compute.SSHConfiguration{
						PublicKeys: &sshPublicKeys,
					},
				},
			},
			NetworkProfile: &compute.NetworkProfile{
				NetworkInterfaces: &networkInterfaceReferences,
			},
			AvailabilitySet:   &compute.SubResource{ID: s.jujuAvailabilitySet.ID},
			ProvisioningState: to.StringPtr("Successful"),
		},
	}
}

func (s *environSuite) openEnviron(c *gc.C, attrs ...testing.Attrs) environs.Environ {
	attrs = append([]testing.Attrs{{"storage-account": fakeStorageAccount}}, attrs...)
	return openEnviron(c, s.provider, &s.sender, attrs...)
}

func openEnviron(
	c *gc.C,
	provider environs.EnvironProvider,
	sender *azuretesting.Senders,
	attrs ...testing.Attrs,
) environs.Environ {
	// Opening the environment should not incur network communication,
	// so we don't set s.sender until after opening.
	cfg := makeTestModelConfig(c, attrs...)
	env, err := provider.Open(cfg)
	c.Assert(err, jc.ErrorIsNil)

	// Force an explicit refresh of the access token, so it isn't done
	// implicitly during the tests.
	*sender = azuretesting.Senders{tokenRefreshSender()}
	err = azure.ForceTokenRefresh(env)
	c.Assert(err, jc.ErrorIsNil)
	return env
}

func prepareForBootstrap(
	c *gc.C,
	ctx environs.BootstrapContext,
	provider environs.EnvironProvider,
	sender *azuretesting.Senders,
	attrs ...testing.Attrs,
) environs.Environ {
	// Opening the environment should not incur network communication,
	// so we don't set s.sender until after opening.
	cfg := makeTestModelConfig(c, attrs...)
	*sender = azuretesting.Senders{tokenRefreshSender()}
	cfg, err := provider.BootstrapConfig(environs.BootstrapConfigParams{
		Config:               cfg,
		CloudRegion:          "westus",
		CloudEndpoint:        "https://management.azure.com",
		CloudStorageEndpoint: "https://core.windows.net",
		Credentials:          fakeUserPassCredential(),
	})
	c.Assert(err, jc.ErrorIsNil)
	env, err := provider.PrepareForBootstrap(ctx, cfg)
	c.Assert(err, jc.ErrorIsNil)
	return env
}

func tokenRefreshSender() *azuretesting.MockSender {
	// lp:1558657
	tokenRefreshSender := azuretesting.NewSenderWithValue(&autorestazure.Token{
		AccessToken: "access-token",
		ExpiresOn:   fmt.Sprint(time.Now().Add(time.Hour).Unix()),
		Type:        "Bearer",
	})
	tokenRefreshSender.PathPattern = ".*/oauth2/token"
	return tokenRefreshSender
}

func (s *environSuite) initResourceGroupSenders() azuretesting.Senders {
	resourceGroupName := "juju-testenv-model-deadbeef-0bad-400d-8000-4b1d0d06f00d"
	return azuretesting.Senders{
		s.makeSender(".*/resourcegroups/"+resourceGroupName, s.group),
		s.makeSender(".*/virtualnetworks/juju-internal-network", s.vnet),
		s.makeSender(".*/networkSecurityGroups/juju-internal-nsg", s.nsg),
		s.makeSender(".*/virtualnetworks/juju-internal-network/subnets/juju-internal-subnet", s.subnet),
		s.makeSender(".*/checkNameAvailability", s.storageNameAvailabilityResult),
		s.makeSender(".*/storageAccounts/.*", s.storageAccount),
		s.makeSender(".*/storageAccounts/.*/listKeys", s.storageAccountKeys),
	}
}

func (s *environSuite) startInstanceSenders(controller bool) azuretesting.Senders {
	senders := azuretesting.Senders{
		s.vmSizesSender(),
		s.makeSender(".*/subnets/juju-internal-subnet", s.subnet),
		s.makeSender(".*/Canonical/.*/UbuntuServer/skus", s.ubuntuServerSKUs),
		s.makeSender(".*/publicIPAddresses/machine-0-public-ip", s.publicIPAddress),
		s.makeSender(".*/networkInterfaces", s.oldNetworkInterfaces),
		s.makeSender(".*/networkInterfaces/machine-0-primary", s.newNetworkInterface),
	}
	if controller {
		senders = append(senders,
			s.makeSender(".*/networkSecurityGroups/juju-internal-nsg", &network.SecurityGroup{
				Properties: &network.SecurityGroupPropertiesFormat{},
			}),
			s.makeSender(".*/networkSecurityGroups/juju-internal-nsg", &network.SecurityGroup{}),
		)
	}
	senders = append(senders,
		s.makeSender(".*/availabilitySets/.*", s.jujuAvailabilitySet),
		s.makeSender(".*/virtualMachines/machine-0", s.virtualMachine),
	)
	return senders
}

func (s *environSuite) networkInterfacesSender(nics ...network.Interface) *azuretesting.MockSender {
	return s.makeSender(".*/networkInterfaces", network.InterfaceListResult{Value: &nics})
}

func (s *environSuite) publicIPAddressesSender(pips ...network.PublicIPAddress) *azuretesting.MockSender {
	return s.makeSender(".*/publicIPAddresses", network.PublicIPAddressListResult{Value: &pips})
}

func (s *environSuite) virtualMachinesSender(vms ...compute.VirtualMachine) *azuretesting.MockSender {
	return s.makeSender(".*/virtualMachines", compute.VirtualMachineListResult{Value: &vms})
}

func (s *environSuite) vmSizesSender() *azuretesting.MockSender {
	return s.makeSender(".*/vmSizes", s.vmSizes)
}

func (s *environSuite) makeSender(pattern string, v interface{}) *azuretesting.MockSender {
	sender := azuretesting.NewSenderWithValue(v)
	sender.PathPattern = pattern
	return sender
}

func makeStartInstanceParams(c *gc.C, series string) environs.StartInstanceParams {
	machineTag := names.NewMachineTag("0")
	apiInfo := &api.Info{
		Addrs:    []string{"localhost:246"},
		CACert:   testing.CACert,
		Password: "admin",
		Tag:      machineTag,
		ModelTag: testing.ModelTag,
	}

	const secureServerConnections = true
	icfg, err := instancecfg.NewInstanceConfig(
		machineTag.Id(), "yanonce", imagemetadata.ReleasedStream,
		series, secureServerConnections, apiInfo,
	)
	c.Assert(err, jc.ErrorIsNil)

	return environs.StartInstanceParams{
		Tools:          makeToolsList(series),
		InstanceConfig: icfg,
	}
}

func makeToolsList(series string) tools.List {
	var toolsVersion version.Binary
	toolsVersion.Number = version.MustParse("1.26.0")
	toolsVersion.Arch = arch.AMD64
	toolsVersion.Series = series
	return tools.List{{
		Version: toolsVersion,
		URL:     fmt.Sprintf("http://example.com/tools/juju-%s.tgz", toolsVersion),
		SHA256:  "1234567890abcdef",
		Size:    1024,
	}}
}

func unmarshalRequestBody(c *gc.C, req *http.Request, out interface{}) {
	bytes, err := ioutil.ReadAll(req.Body)
	c.Assert(err, jc.ErrorIsNil)
	err = json.Unmarshal(bytes, out)
	c.Assert(err, jc.ErrorIsNil)
}

func assertRequestBody(c *gc.C, req *http.Request, expect interface{}) {
	unmarshalled := reflect.New(reflect.TypeOf(expect).Elem()).Interface()
	unmarshalRequestBody(c, req, unmarshalled)
	c.Assert(unmarshalled, jc.DeepEquals, expect)
}

type mockClock struct {
	gitjujutesting.Stub
	*testing.Clock
}

func (c *mockClock) After(d time.Duration) <-chan time.Time {
	c.MethodCall(c, "After", d)
	c.PopNoErr()
	return c.Clock.After(d)
}

func (s *environSuite) TestOpen(c *gc.C) {
	cfg := makeTestModelConfig(c)
	env, err := s.provider.Open(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
}

func (s *environSuite) TestCloudEndpointManagementURI(c *gc.C) {
	env := s.openEnviron(c)

	sender := mocks.NewSender()
	sender.EmitContent("{}")
	s.sender = azuretesting.Senders{sender}
	s.requests = nil
	env.AllInstances() // trigger a query

	c.Assert(s.requests, gc.HasLen, 1)
	c.Assert(s.requests[0].URL.Host, gc.Equals, "api.azurestack.local")
}

func (s *environSuite) TestStartInstance(c *gc.C) {
	env := s.openEnviron(c)
	s.sender = s.startInstanceSenders(false)
	s.requests = nil
	result, err := env.StartInstance(makeStartInstanceParams(c, "quantal"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.NotNil)
	c.Assert(result.Instance, gc.NotNil)
	c.Assert(result.NetworkInfo, gc.HasLen, 0)
	c.Assert(result.Volumes, gc.HasLen, 0)
	c.Assert(result.VolumeAttachments, gc.HasLen, 0)

	arch := "amd64"
	mem := uint64(3584)
	rootDisk := uint64(29495) // ~30 GB
	cpuCores := uint64(1)
	c.Assert(result.Hardware, jc.DeepEquals, &instance.HardwareCharacteristics{
		Arch:     &arch,
		Mem:      &mem,
		RootDisk: &rootDisk,
		CpuCores: &cpuCores,
	})
	requests := s.assertStartInstanceRequests(c, s.requests)
	availabilitySetName := path.Base(requests.availabilitySet.URL.Path)
	c.Assert(availabilitySetName, gc.Equals, "juju")
}

func (s *environSuite) TestStartInstanceTooManyRequests(c *gc.C) {
	env := s.openEnviron(c)
	senders := s.startInstanceSenders(false)
	s.requests = nil

	// 6 failures to get to 1 minute, and show that we cap it there.
	const failures = 6

	// Make the VirtualMachines.CreateOrUpdate call respond with
	// 429 (StatusTooManyRequests) failures, and then with success.
	rateLimitedSender := mocks.NewSender()
	rateLimitedSender.EmitStatus("(」゜ロ゜)」", http.StatusTooManyRequests)
	successSender := senders[len(senders)-1]
	senders = senders[:len(senders)-1]
	for i := 0; i < failures; i++ {
		senders = append(senders, rateLimitedSender)
	}
	senders = append(senders, successSender)
	s.sender = senders

	_, err := env.StartInstance(makeStartInstanceParams(c, "quantal"))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.requests, gc.HasLen, 8+failures)
	s.assertStartInstanceRequests(c, s.requests[:8])

	// The last two requests should match the third-to-last, which
	// is checked by assertStartInstanceRequests.
	for i := 8; i < 8+failures; i++ {
		c.Assert(s.requests[i].Method, gc.Equals, "PUT")
		assertCreateVirtualMachineRequestBody(c, s.requests[i], s.virtualMachine)
	}

	s.retryClock.CheckCalls(c, []gitjujutesting.StubCall{
		{"After", []interface{}{5 * time.Second}},
		{"After", []interface{}{10 * time.Second}},
		{"After", []interface{}{20 * time.Second}},
		{"After", []interface{}{40 * time.Second}},
		{"After", []interface{}{1 * time.Minute}},
		{"After", []interface{}{1 * time.Minute}},
	})
}

func (s *environSuite) TestStartInstanceTooManyRequestsTimeout(c *gc.C) {
	env := s.openEnviron(c)
	senders := s.startInstanceSenders(false)
	s.requests = nil

	// 8 failures to get to 5 minutes, which is as long as we'll keep
	// retrying before giving up.
	const failures = 8

	// Make the VirtualMachines.CreateOrUpdate call respond with
	// enough 429 (StatusTooManyRequests) failures to cause the
	// method to give up retrying.
	rateLimitedSender := mocks.NewSender()
	rateLimitedSender.EmitStatus("(」゜ロ゜)」", http.StatusTooManyRequests)
	senders = senders[:len(senders)-1]
	for i := 0; i < failures; i++ {
		senders = append(senders, rateLimitedSender)
	}
	s.sender = senders

	_, err := env.StartInstance(makeStartInstanceParams(c, "quantal"))
	c.Assert(err, gc.ErrorMatches, "creating virtual machine.*: max duration exceeded: .*failed with.*")

	s.retryClock.CheckCalls(c, []gitjujutesting.StubCall{
		{"After", []interface{}{5 * time.Second}},  // t0 + 5s
		{"After", []interface{}{10 * time.Second}}, // t0 + 15s
		{"After", []interface{}{20 * time.Second}}, // t0 + 35s
		{"After", []interface{}{40 * time.Second}}, // t0 + 1m15s
		{"After", []interface{}{1 * time.Minute}},  // t0 + 2m15s
		{"After", []interface{}{1 * time.Minute}},  // t0 + 3m15s
		{"After", []interface{}{1 * time.Minute}},  // t0 + 4m15s
		// There would be another call here, but since the time
		// exceeds the give minute limit, retrying is aborted.
	})
}

func (s *environSuite) TestStartInstanceDistributionGroup(c *gc.C) {
	c.Skip("TODO: test StartInstance's DistributionGroup behaviour")
}

func (s *environSuite) TestStartInstanceServiceAvailabilitySet(c *gc.C) {
	env := s.openEnviron(c)
	s.sender = s.startInstanceSenders(false)
	s.requests = nil
	unitsDeployed := "mysql/0 wordpress/0"
	params := makeStartInstanceParams(c, "quantal")
	params.InstanceConfig.Tags[tags.JujuUnitsDeployed] = unitsDeployed
	_, err := env.StartInstance(params)
	c.Assert(err, jc.ErrorIsNil)
	s.tags[tags.JujuUnitsDeployed] = &unitsDeployed
	requests := s.assertStartInstanceRequests(c, s.requests)
	availabilitySetName := path.Base(requests.availabilitySet.URL.Path)
	c.Assert(availabilitySetName, gc.Equals, "mysql")
}

func (s *environSuite) assertStartInstanceRequests(c *gc.C, requests []*http.Request) startInstanceRequests {
	// Clear the fields that don't get sent in the request.
	s.publicIPAddress.ID = nil
	s.publicIPAddress.Name = nil
	s.publicIPAddress.Properties.IPAddress = nil
	s.newNetworkInterface.ID = nil
	s.newNetworkInterface.Name = nil
	(*s.newNetworkInterface.Properties.IPConfigurations)[0].ID = nil
	s.jujuAvailabilitySet.ID = nil
	s.jujuAvailabilitySet.Name = nil
	s.virtualMachine.ID = nil
	s.virtualMachine.Name = nil
	s.virtualMachine.Properties.ProvisioningState = nil

	// Validate HTTP request bodies.
	c.Assert(requests, gc.HasLen, 8)
	c.Assert(requests[0].Method, gc.Equals, "GET") // vmSizes
	c.Assert(requests[1].Method, gc.Equals, "GET") // juju-testenv-model-deadbeef-0bad-400d-8000-4b1d0d06f00d
	c.Assert(requests[2].Method, gc.Equals, "GET") // skus
	c.Assert(requests[3].Method, gc.Equals, "PUT")
	assertRequestBody(c, requests[3], s.publicIPAddress)
	c.Assert(requests[4].Method, gc.Equals, "GET") // NICs
	c.Assert(requests[5].Method, gc.Equals, "PUT")
	assertRequestBody(c, requests[5], s.newNetworkInterface)
	c.Assert(requests[6].Method, gc.Equals, "PUT")
	assertRequestBody(c, requests[6], s.jujuAvailabilitySet)
	c.Assert(requests[7].Method, gc.Equals, "PUT")
	assertCreateVirtualMachineRequestBody(c, requests[7], s.virtualMachine)

	return startInstanceRequests{
		vmSizes:          requests[0],
		subnet:           requests[1],
		skus:             requests[2],
		publicIPAddress:  requests[3],
		nics:             requests[4],
		networkInterface: requests[5],
		availabilitySet:  requests[6],
		virtualMachine:   requests[7],
	}
}

func assertCreateVirtualMachineRequestBody(c *gc.C, req *http.Request, expect *compute.VirtualMachine) {
	// CustomData is non-deterministic, so don't compare it.
	// TODO(axw) shouldn't CustomData be deterministic? Look into this.
	var virtualMachine compute.VirtualMachine
	unmarshalRequestBody(c, req, &virtualMachine)
	c.Assert(to.String(virtualMachine.Properties.OsProfile.CustomData), gc.Not(gc.HasLen), 0)
	virtualMachine.Properties.OsProfile.CustomData = to.StringPtr("<juju-goes-here>")
	c.Assert(&virtualMachine, jc.DeepEquals, expect)
}

type startInstanceRequests struct {
	vmSizes          *http.Request
	subnet           *http.Request
	skus             *http.Request
	publicIPAddress  *http.Request
	nics             *http.Request
	networkInterface *http.Request
	availabilitySet  *http.Request
	virtualMachine   *http.Request
}

func (s *environSuite) TestBootstrap(c *gc.C) {
	defer envtesting.DisableFinishBootstrap()()

	ctx := envtesting.BootstrapContext(c)
	env := prepareForBootstrap(c, ctx, s.provider, &s.sender)

	s.sender = s.initResourceGroupSenders()
	s.sender = append(s.sender, s.startInstanceSenders(true)...)
	s.requests = nil
	result, err := env.Bootstrap(
		ctx, environs.BootstrapParams{
			AvailableTools: makeToolsList(series.LatestLts()),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Arch, gc.Equals, "amd64")
	c.Assert(result.Series, gc.Equals, series.LatestLts())

	c.Assert(len(s.requests), gc.Equals, 17)

	c.Assert(s.requests[0].Method, gc.Equals, "PUT")  // resource group
	c.Assert(s.requests[1].Method, gc.Equals, "PUT")  // vnet
	c.Assert(s.requests[2].Method, gc.Equals, "PUT")  // network security group
	c.Assert(s.requests[3].Method, gc.Equals, "PUT")  // subnet
	c.Assert(s.requests[4].Method, gc.Equals, "POST") // check storage account name
	c.Assert(s.requests[5].Method, gc.Equals, "PUT")  // create storage account
	c.Assert(s.requests[6].Method, gc.Equals, "POST") // get storage account keys

	assertRequestBody(c, s.requests[0], &s.group)

	s.vnet.ID = nil
	s.vnet.Name = nil
	assertRequestBody(c, s.requests[1], s.vnet)

	securityRules := []network.SecurityRule{{
		Name: to.StringPtr("SSHInbound"),
		Properties: &network.SecurityRulePropertiesFormat{
			Description:              to.StringPtr("Allow SSH access to all machines"),
			Protocol:                 network.TCP,
			SourceAddressPrefix:      to.StringPtr("*"),
			SourcePortRange:          to.StringPtr("*"),
			DestinationAddressPrefix: to.StringPtr("*"),
			DestinationPortRange:     to.StringPtr("22"),
			Access:                   network.Allow,
			Priority:                 to.IntPtr(100),
			Direction:                network.Inbound,
		},
	}}
	assertRequestBody(c, s.requests[2], &network.SecurityGroup{
		Location: to.StringPtr("westus"),
		Tags:     s.nsg.Tags,
		Properties: &network.SecurityGroupPropertiesFormat{
			SecurityRules: &securityRules,
		},
	})

	s.subnet.ID = nil
	s.subnet.Name = nil
	assertRequestBody(c, s.requests[3], s.subnet)

	assertRequestBody(c, s.requests[4], &storage.AccountCheckNameAvailabilityParameters{
		Name: to.StringPtr(fakeStorageAccount),
		Type: to.StringPtr("Microsoft.Storage/storageAccounts"),
	})

	assertRequestBody(c, s.requests[5], &storage.AccountCreateParameters{
		Location: to.StringPtr("westus"),
		Tags:     s.storageAccount.Tags,
		Properties: &storage.AccountPropertiesCreateParameters{
			AccountType: "Standard_LRS",
		},
	})
}

func (s *environSuite) TestAllInstancesResourceGroupNotFound(c *gc.C) {
	env := s.openEnviron(c)
	sender := mocks.NewSender()
	sender.EmitStatus("resource group not found", http.StatusNotFound)
	s.sender = azuretesting.Senders{sender}
	_, err := env.AllInstances()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environSuite) TestStopInstancesNotFound(c *gc.C) {
	env := s.openEnviron(c)
	sender := mocks.NewSender()
	sender.EmitStatus("vm not found", http.StatusNotFound)
	s.sender = azuretesting.Senders{sender, sender, sender}
	err := env.StopInstances("a", "b")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environSuite) TestStopInstances(c *gc.C) {
	env := s.openEnviron(c)

	// Security group has rules for machine-0 but not machine-1, and
	// has a rule that doesn't match either.
	nsg := makeSecurityGroup(
		makeSecurityRule("machine-0-80", "10.0.0.4", "80"),
		makeSecurityRule("machine-0-1000-2000", "10.0.0.4", "1000-2000"),
		makeSecurityRule("machine-42", "10.0.0.5", "*"),
	)

	// Create an IP configuration with a public IP reference. This will
	// cause an update to the NIC to detach public IPs.
	nic0IPConfiguration := makeIPConfiguration("10.0.0.4")
	nic0IPConfiguration.Properties.PublicIPAddress = &network.SubResource{}
	nic0 := makeNetworkInterface("nic-0", "machine-0", nic0IPConfiguration)

	s.sender = azuretesting.Senders{
		s.networkInterfacesSender(
			nic0,
			makeNetworkInterface("nic-1", "machine-1"),
			makeNetworkInterface("nic-2", "machine-1"),
		),
		s.virtualMachinesSender(makeVirtualMachine("machine-0")),
		s.publicIPAddressesSender(
			makePublicIPAddress("pip-0", "machine-0", "1.2.3.4"),
		),
		s.makeSender(".*/virtualMachines/machine-0", nil),                                                 // DELETE
		s.makeSender(".*/networkSecurityGroups/juju-internal-nsg", nsg),                                   // GET
		s.makeSender(".*/networkSecurityGroups/juju-internal-nsg/securityRules/machine-0-80", nil),        // DELETE
		s.makeSender(".*/networkSecurityGroups/juju-internal-nsg/securityRules/machine-0-1000-2000", nil), // DELETE
		s.makeSender(".*/networkInterfaces/nic-0", nic0),                                                  // PUT
		s.makeSender(".*/publicIPAddresses/pip-0", nil),                                                   // DELETE
		s.makeSender(".*/networkInterfaces/nic-0", nil),                                                   // DELETE
		s.makeSender(".*/virtualMachines/machine-1", nil),                                                 // DELETE
		s.makeSender(".*/networkSecurityGroups/juju-internal-nsg", nsg),                                   // GET
		s.makeSender(".*/networkInterfaces/nic-1", nil),                                                   // DELETE
		s.makeSender(".*/networkInterfaces/nic-2", nil),                                                   // DELETE
	}
	err := env.StopInstances("machine-0", "machine-1", "machine-2")
	c.Assert(err, jc.ErrorIsNil)

	s.storageClient.CheckCallNames(c,
		"NewClient", "DeleteBlobIfExists", "DeleteBlobIfExists",
	)
	s.storageClient.CheckCall(c, 1, "DeleteBlobIfExists", "osvhds", "machine-0")
	s.storageClient.CheckCall(c, 2, "DeleteBlobIfExists", "osvhds", "machine-1")
}

func (s *environSuite) TestConstraintsValidatorUnsupported(c *gc.C) {
	validator := s.constraintsValidator(c)
	unsupported, err := validator.Validate(constraints.MustParse(
		"arch=amd64 tags=foo cpu-power=100 virt-type=kvm",
	))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsupported, jc.SameContents, []string{"tags", "cpu-power", "virt-type"})
}

func (s *environSuite) TestConstraintsValidatorVocabulary(c *gc.C) {
	validator := s.constraintsValidator(c)
	_, err := validator.Validate(constraints.MustParse("arch=armhf"))
	c.Assert(err, gc.ErrorMatches,
		"invalid constraint value: arch=armhf\nvalid values are: \\[amd64\\]",
	)
	_, err = validator.Validate(constraints.MustParse("instance-type=t1.micro"))
	c.Assert(err, gc.ErrorMatches,
		"invalid constraint value: instance-type=t1.micro\nvalid values are: \\[D1 Standard_D1\\]",
	)
}

func (s *environSuite) TestConstraintsValidatorMerge(c *gc.C) {
	validator := s.constraintsValidator(c)
	cons, err := validator.Merge(
		constraints.MustParse("mem=3G arch=amd64"),
		constraints.MustParse("instance-type=D1"),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons.String(), gc.Equals, "instance-type=D1")
}

func (s *environSuite) constraintsValidator(c *gc.C) constraints.Validator {
	env := s.openEnviron(c)
	s.sender = azuretesting.Senders{s.vmSizesSender()}
	validator, err := env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)
	return validator
}

func (s *environSuite) TestAgentMirror(c *gc.C) {
	env := s.openEnviron(c)
	c.Assert(env, gc.Implements, new(envtools.HasAgentMirror))
	cloudSpec, err := env.(envtools.HasAgentMirror).AgentMirror()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloudSpec, gc.Equals, simplestreams.CloudSpec{
		Region:   "westus",
		Endpoint: "https://storage.azurestack.local/",
	})
}

func (s *environSuite) TestDestroyHostedModel(c *gc.C) {
	env := s.openEnviron(c, testing.Attrs{"controller-uuid": utils.MustNewUUID().String()})
	s.sender = azuretesting.Senders{
		s.makeSender(".*/resourcegroups/juju-testenv-model-"+testing.ModelTag.Id(), nil), // DELETE
	}
	err := env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.requests, gc.HasLen, 1)
	c.Assert(s.requests[0].Method, gc.Equals, "DELETE")
}

func (s *environSuite) TestDestroyControllerModel(c *gc.C) {
	groups := []resources.ResourceGroup{{
		Name: to.StringPtr("group1"),
	}, {
		Name: to.StringPtr("group2"),
	}}
	result := resources.ResourceGroupListResult{Value: &groups}

	env := s.openEnviron(c)
	s.sender = azuretesting.Senders{
		s.makeSender(".*/resourcegroups", result),        // GET
		s.makeSender(".*/resourcegroups/group[12]", nil), // DELETE
		s.makeSender(".*/resourcegroups/group[12]", nil), // DELETE
	}
	err := env.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.requests, gc.HasLen, 3)
	c.Assert(s.requests[0].Method, gc.Equals, "GET")
	c.Assert(s.requests[0].URL.Query().Get("$filter"), gc.Equals, fmt.Sprintf(
		"tagname eq 'juju-controller-uuid' and tagvalue eq '%s'",
		testing.ModelTag.Id(),
	))
	c.Assert(s.requests[1].Method, gc.Equals, "DELETE")
	c.Assert(s.requests[2].Method, gc.Equals, "DELETE")

	// Groups are deleted concurrently, so there's no known order.
	groupsDeleted := []string{
		path.Base(s.requests[1].URL.Path),
		path.Base(s.requests[2].URL.Path),
	}
	c.Assert(groupsDeleted, jc.SameContents, []string{"group1", "group2"})
}

func (s *environSuite) TestDestroyControllerModelErrors(c *gc.C) {
	groups := []resources.ResourceGroup{
		{Name: to.StringPtr("group1")},
		{Name: to.StringPtr("group2")},
	}
	result := resources.ResourceGroupListResult{Value: &groups}

	errorSender1 := s.makeSender(".*/resourcegroups/group[12]", nil)
	errorSender1.EmitStatus("foo", http.StatusInternalServerError)
	errorSender2 := s.makeSender(".*/resourcegroups/group[12]", nil)
	errorSender2.EmitStatus("bar", http.StatusInternalServerError)

	env := s.openEnviron(c)
	s.requests = nil
	s.sender = azuretesting.Senders{
		s.makeSender(".*/resourcegroups", result), // GET
		errorSender1,                              // DELETE
		errorSender2,                              // DELETE
	}
	destroyErr := env.Destroy()
	// checked below, once we know the order of deletions.

	c.Assert(s.requests, gc.HasLen, 3)
	c.Assert(s.requests[0].Method, gc.Equals, "GET")
	c.Assert(s.requests[1].Method, gc.Equals, "DELETE")
	c.Assert(s.requests[2].Method, gc.Equals, "DELETE")

	// Groups are deleted concurrently, so there's no known order.
	groupsDeleted := []string{
		path.Base(s.requests[1].URL.Path),
		path.Base(s.requests[2].URL.Path),
	}
	c.Assert(groupsDeleted, jc.SameContents, []string{"group1", "group2"})

	// The errors are guaranteed to be in the order that the names appeared
	// in the list response.
	firstErr := "foo"
	secondErr := "bar"
	if groupsDeleted[0] == "group2" {
		firstErr, secondErr = secondErr, firstErr
	}
	c.Assert(destroyErr, gc.ErrorMatches,
		`deleting resource group "group1":.* failed with `+firstErr+`; `+
			`deleting resource group "group2":.* failed with `+secondErr)
}
