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
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/tags"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/provider/azure"
	"github.com/juju/juju/provider/azure/internal/azuretesting"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

type environSuite struct {
	testing.BaseSuite

	provider      environs.EnvironProvider
	requests      []*http.Request
	storageClient azuretesting.MockStorageClient
	sender        azuretesting.Senders

	tags                          map[string]*string
	vmSizes                       *compute.VirtualMachineSizeListResult
	storageNameAvailabilityResult *storage.CheckNameAvailabilityResult
	storageAccount                *storage.Account
	storageAccountKeys            *storage.AccountKeys
	vnet                          *network.VirtualNetwork
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
	s.provider, _ = newProviders(c, azure.ProviderConfig{
		Sender:           &s.sender,
		RequestInspector: requestRecorder(&s.requests),
		NewStorageClient: s.storageClient.NewClient,
	})

	emptyTags := make(map[string]*string)
	s.tags = map[string]*string{
		"juju-machine-name": to.StringPtr("machine-0"),
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
		Properties: &storage.AccountProperties{
			PrimaryEndpoints: &storage.Endpoints{
				Blob: to.StringPtr(fmt.Sprintf("https://%s.blob.storage.azurestack.local/", fakeStorageAccount)),
			},
		},
	}

	s.storageAccountKeys = &storage.AccountKeys{
		Key1: to.StringPtr("key-1"),
	}

	addressPrefixes := make([]string, 256)
	for i := range addressPrefixes {
		addressPrefixes[i] = fmt.Sprintf("10.%d.0.0/16", i)
	}
	s.vnet = &network.VirtualNetwork{
		ID:       to.StringPtr("juju-internal"),
		Name:     to.StringPtr("juju-internal"),
		Location: to.StringPtr("westus"),
		Tags:     &emptyTags,
		Properties: &network.VirtualNetworkPropertiesFormat{
			AddressSpace: &network.AddressSpace{&addressPrefixes},
		},
	}

	s.subnet = &network.Subnet{
		ID:   to.StringPtr("subnet-id"),
		Name: to.StringPtr("juju-testenv-model-deadbeef-0bad-400d-8000-4b1d0d06f00d"),
		Properties: &network.SubnetPropertiesFormat{
			AddressPrefix: to.StringPtr("10.0.0.0/16"),
		},
	}

	s.ubuntuServerSKUs = []compute.VirtualMachineImageResource{
		{Name: to.StringPtr("12.04-LTS")},
		{Name: to.StringPtr("12.10")},
		{Name: to.StringPtr("14.04-LTS")},
		{Name: to.StringPtr("15.04")},
		{Name: to.StringPtr("15.10")},
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

	// nsgID is the name of the internal network security group. This NSG
	// is created when the environment is created.
	nsgID := path.Join(
		"/subscriptions", fakeSubscriptionId,
		"resourceGroups", "juju-testenv-model-"+testing.ModelTag.Id(),
		"providers/Microsoft.Network/networkSecurityGroups/juju-internal",
	)

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
			IPConfigurations:     &newIPConfigurations,
			NetworkSecurityGroup: &network.SubResource{to.StringPtr(nsgID)},
		},
	}

	s.jujuAvailabilitySet = &compute.AvailabilitySet{
		ID:       to.StringPtr("juju-availability-set-id"),
		Name:     to.StringPtr("juju"),
		Location: to.StringPtr("westus"),
		Tags:     &emptyTags,
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
	cfg, err := cfg.Remove([]string{"controller-resource-group"})
	c.Assert(err, jc.ErrorIsNil)
	*sender = azuretesting.Senders{tokenRefreshSender()}
	env, err := provider.PrepareForBootstrap(ctx, environs.PrepareForBootstrapParams{
		Config:               cfg,
		CloudRegion:          "westus",
		CloudEndpoint:        "https://management.azure.com",
		CloudStorageEndpoint: "https://core.windows.net",
		Credentials:          fakeUserPassCredential(),
	})
	c.Assert(err, jc.ErrorIsNil)
	return env
}

func tokenRefreshSender() *azuretesting.MockSender {
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
		s.makeSender(".*/resourcegroups/"+resourceGroupName, &resources.Group{}),
		s.makeSender(".*/virtualnetworks/juju-internal", s.vnet),
		s.makeSender(".*/networkSecurityGroups/juju-internal", &network.SecurityGroup{}),
		s.makeSender(".*/virtualnetworks/juju-internal/subnets/"+resourceGroupName, &s.subnet),
		s.makeSender(".*/checkNameAvailability", s.storageNameAvailabilityResult),
		s.makeSender(".*/storageAccounts/.*", s.storageAccount),
		s.makeSender(".*/storageAccounts/.*/listKeys", s.storageAccountKeys),
	}
}

func (s *environSuite) startInstanceSenders(controller bool) azuretesting.Senders {
	senders := azuretesting.Senders{
		s.vmSizesSender(),
		s.makeSender(".*/subnets/juju-testenv-model-deadbeef-0bad-400d-8000-4b1d0d06f00d", s.subnet),
		s.makeSender(".*/Canonical/.*/UbuntuServer/skus", s.ubuntuServerSKUs),
		s.makeSender(".*/publicIPAddresses/machine-0-public-ip", s.publicIPAddress),
		s.makeSender(".*/networkInterfaces", s.oldNetworkInterfaces),
		s.makeSender(".*/networkInterfaces/machine-0-primary", s.newNetworkInterface),
	}
	if controller {
		senders = append(senders,
			s.makeSender(".*/networkSecurityGroups/juju-internal", &network.SecurityGroup{
				Properties: &network.SecurityGroupPropertiesFormat{},
			}),
			s.makeSender(".*/networkSecurityGroups/juju-internal", &network.SecurityGroup{}),
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
	stateInfo := &mongo.MongoInfo{
		Info: mongo.Info{
			CACert: testing.CACert,
			Addrs:  []string{"localhost:123"},
		},
		Password: "password",
		Tag:      machineTag,
	}
	apiInfo := &api.Info{
		Addrs:    []string{"localhost:246"},
		CACert:   testing.CACert,
		Password: "admin",
		Tag:      machineTag,
		ModelTag: testing.ModelTag,
	}

	const secureServerConnections = true
	var networks []string
	icfg, err := instancecfg.NewInstanceConfig(
		machineTag.Id(), "yanonce", imagemetadata.ReleasedStream,
		series, "", secureServerConnections, networks, stateInfo, apiInfo,
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
	requests := s.assertStartInstanceRequests(c)
	availabilitySetName := path.Base(requests.availabilitySet.URL.Path)
	c.Assert(availabilitySetName, gc.Equals, "juju")
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
	requests := s.assertStartInstanceRequests(c)
	availabilitySetName := path.Base(requests.availabilitySet.URL.Path)
	c.Assert(availabilitySetName, gc.Equals, "mysql")
}

func (s *environSuite) assertStartInstanceRequests(c *gc.C) startInstanceRequests {
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
	c.Assert(s.requests, gc.HasLen, 8)
	c.Assert(s.requests[0].Method, gc.Equals, "GET") // vmSizes
	c.Assert(s.requests[1].Method, gc.Equals, "GET") // juju-testenv-model-deadbeef-0bad-400d-8000-4b1d0d06f00d
	c.Assert(s.requests[2].Method, gc.Equals, "GET") // skus
	c.Assert(s.requests[3].Method, gc.Equals, "PUT")
	assertRequestBody(c, s.requests[3], s.publicIPAddress)
	c.Assert(s.requests[4].Method, gc.Equals, "GET") // NICs
	c.Assert(s.requests[5].Method, gc.Equals, "PUT")
	assertRequestBody(c, s.requests[5], s.newNetworkInterface)
	c.Assert(s.requests[6].Method, gc.Equals, "PUT")
	assertRequestBody(c, s.requests[6], s.jujuAvailabilitySet)
	c.Assert(s.requests[7].Method, gc.Equals, "PUT")

	// CustomData is non-deterministic, so don't compare it.
	// TODO(axw) shouldn't CustomData be deterministic? Look into this.
	var virtualMachine compute.VirtualMachine
	unmarshalRequestBody(c, s.requests[7], &virtualMachine)
	c.Assert(to.String(virtualMachine.Properties.OsProfile.CustomData), gc.Not(gc.HasLen), 0)
	virtualMachine.Properties.OsProfile.CustomData = to.StringPtr("<juju-goes-here>")
	c.Assert(&virtualMachine, jc.DeepEquals, s.virtualMachine)

	return startInstanceRequests{
		vmSizes:          s.requests[0],
		subnet:           s.requests[1],
		skus:             s.requests[2],
		publicIPAddress:  s.requests[3],
		nics:             s.requests[4],
		networkInterface: s.requests[5],
		availabilitySet:  s.requests[6],
		virtualMachine:   s.requests[7],
	}
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
			AvailableTools: makeToolsList("trusty"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Arch, gc.Equals, "amd64")
	c.Assert(result.Series, gc.Equals, "trusty")

	c.Assert(len(s.requests), gc.Equals, 17)

	c.Assert(s.requests[0].Method, gc.Equals, "PUT")  // resource group
	c.Assert(s.requests[1].Method, gc.Equals, "PUT")  // vnet
	c.Assert(s.requests[2].Method, gc.Equals, "PUT")  // network security group
	c.Assert(s.requests[3].Method, gc.Equals, "PUT")  // subnet
	c.Assert(s.requests[4].Method, gc.Equals, "POST") // check storage account name
	c.Assert(s.requests[5].Method, gc.Equals, "PUT")  // create storage account
	c.Assert(s.requests[6].Method, gc.Equals, "POST") // get storage account keys

	emptyTags := map[string]*string{}
	assertRequestBody(c, s.requests[0], &resources.Group{
		Location: to.StringPtr("westus"),
		Tags:     &emptyTags,
	})

	s.vnet.ID = nil
	s.vnet.Name = nil
	assertRequestBody(c, s.requests[1], s.vnet)

	securityRules := []network.SecurityRule{{
		Name: to.StringPtr("SSHInbound"),
		Properties: &network.SecurityRulePropertiesFormat{
			Description:              to.StringPtr("Allow SSH access to all machines"),
			Protocol:                 network.SecurityRuleProtocolTCP,
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
		Tags:     &emptyTags,
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
		Tags:     &emptyTags,
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
		s.makeSender(".*/virtualMachines/machine-0", nil),                                             // DELETE
		s.makeSender(".*/networkSecurityGroups/juju-internal", nsg),                                   // GET
		s.makeSender(".*/networkSecurityGroups/juju-internal/securityRules/machine-0-80", nil),        // DELETE
		s.makeSender(".*/networkSecurityGroups/juju-internal/securityRules/machine-0-1000-2000", nil), // DELETE
		s.makeSender(".*/networkInterfaces/nic-0", nic0),                                              // PUT
		s.makeSender(".*/publicIPAddresses/pip-0", nil),                                               // DELETE
		s.makeSender(".*/networkInterfaces/nic-0", nil),                                               // DELETE
		s.makeSender(".*/virtualMachines/machine-1", nil),                                             // DELETE
		s.makeSender(".*/networkSecurityGroups/juju-internal", nsg),                                   // GET
		s.makeSender(".*/networkInterfaces/nic-1", nil),                                               // DELETE
		s.makeSender(".*/networkInterfaces/nic-2", nil),                                               // DELETE
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
		"arch=amd64 tags=foo cpu-power=100",
	))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsupported, jc.SameContents, []string{"tags", "cpu-power"})
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
