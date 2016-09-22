// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"reflect"
	"time"

	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/Azure/azure-sdk-for-go/arm/resources/resources"
	"github.com/Azure/azure-sdk-for-go/arm/storage"
	autorestazure "github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/mocks"
	"github.com/Azure/go-autorest/autorest/to"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

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
	"github.com/juju/juju/provider/azure/internal/armtemplates"
	"github.com/juju/juju/provider/azure/internal/azureauth"
	"github.com/juju/juju/provider/azure/internal/azuretesting"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/version"
)

const storageAccountName = "juju400d80004b1d0d06f00d"

var (
	quantalImageReference = compute.ImageReference{
		Publisher: to.StringPtr("Canonical"),
		Offer:     to.StringPtr("UbuntuServer"),
		Sku:       to.StringPtr("12.10"),
		Version:   to.StringPtr("latest"),
	}
	win2012ImageReference = compute.ImageReference{
		Publisher: to.StringPtr("MicrosoftWindowsServer"),
		Offer:     to.StringPtr("WindowsServer"),
		Sku:       to.StringPtr("2012-Datacenter"),
		Version:   to.StringPtr("latest"),
	}
	centos7ImageReference = compute.ImageReference{
		Publisher: to.StringPtr("OpenLogic"),
		Offer:     to.StringPtr("CentOS"),
		Sku:       to.StringPtr("7.1"),
		Version:   to.StringPtr("latest"),
	}

	sshPublicKeys = []compute.SSHPublicKey{{
		Path:    to.StringPtr("/home/ubuntu/.ssh/authorized_keys"),
		KeyData: to.StringPtr(testing.FakeAuthKeys),
	}}
	linuxOsProfile = compute.OSProfile{
		ComputerName:  to.StringPtr("machine-0"),
		CustomData:    to.StringPtr("<juju-goes-here>"),
		AdminUsername: to.StringPtr("ubuntu"),
		LinuxConfiguration: &compute.LinuxConfiguration{
			DisablePasswordAuthentication: to.BoolPtr(true),
			SSH: &compute.SSHConfiguration{
				PublicKeys: &sshPublicKeys,
			},
		},
	}
	windowsOsProfile = compute.OSProfile{
		ComputerName:  to.StringPtr("machine-0"),
		CustomData:    to.StringPtr("<juju-goes-here>"),
		AdminUsername: to.StringPtr("JujuAdministrator"),
		AdminPassword: to.StringPtr("sorandom"),
		WindowsConfiguration: &compute.WindowsConfiguration{
			ProvisionVMAgent:       to.BoolPtr(true),
			EnableAutomaticUpdates: to.BoolPtr(true),
		},
	}
)

type environSuite struct {
	testing.BaseSuite

	provider      environs.EnvironProvider
	requests      []*http.Request
	storageClient azuretesting.MockStorageClient
	sender        azuretesting.Senders
	retryClock    mockClock

	controllerUUID     string
	envTags            map[string]*string
	vmTags             map[string]*string
	group              *resources.ResourceGroup
	vmSizes            *compute.VirtualMachineSizeListResult
	storageAccounts    []storage.Account
	storageAccount     *storage.Account
	storageAccountKeys *storage.AccountListKeysResult
	ubuntuServerSKUs   []compute.VirtualMachineImageResource
	deployment         *resources.Deployment
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.storageClient = azuretesting.MockStorageClient{}
	s.sender = nil
	s.requests = nil
	s.retryClock = mockClock{Clock: gitjujutesting.NewClock(time.Time{})}

	s.provider = newProvider(c, azure.ProviderConfig{
		Sender:           azuretesting.NewSerialSender(&s.sender),
		RequestInspector: azuretesting.RequestRecorder(&s.requests),
		NewStorageClient: s.storageClient.NewClient,
		RetryClock: &gitjujutesting.AutoAdvancingClock{
			&s.retryClock, s.retryClock.Advance,
		},
		RandomWindowsAdminPassword:        func() string { return "sorandom" },
		InteractiveCreateServicePrincipal: azureauth.InteractiveCreateServicePrincipal,
	})

	s.controllerUUID = testing.ControllerTag.Id()
	s.envTags = map[string]*string{
		"juju-model-uuid":      to.StringPtr(testing.ModelTag.Id()),
		"juju-controller-uuid": to.StringPtr(s.controllerUUID),
	}
	s.vmTags = map[string]*string{
		"juju-model-uuid":      to.StringPtr(testing.ModelTag.Id()),
		"juju-controller-uuid": to.StringPtr(s.controllerUUID),
		"juju-machine-name":    to.StringPtr("machine-0"),
	}

	s.group = &resources.ResourceGroup{
		Location: to.StringPtr("westus"),
		Tags:     &s.envTags,
		Properties: &resources.ResourceGroupProperties{
			ProvisioningState: to.StringPtr("Succeeded"),
		},
	}

	vmSizes := []compute.VirtualMachineSize{{
		Name:                 to.StringPtr("Standard_D1"),
		NumberOfCores:        to.Int32Ptr(1),
		OsDiskSizeInMB:       to.Int32Ptr(1047552),
		ResourceDiskSizeInMB: to.Int32Ptr(51200),
		MemoryInMB:           to.Int32Ptr(3584),
		MaxDataDiskCount:     to.Int32Ptr(2),
	}}
	s.vmSizes = &compute.VirtualMachineSizeListResult{Value: &vmSizes}

	s.storageAccount = &storage.Account{
		Name: to.StringPtr("my-storage-account"),
		Type: to.StringPtr("Standard_LRS"),
		Tags: &s.envTags,
		Properties: &storage.AccountProperties{
			PrimaryEndpoints: &storage.Endpoints{
				Blob: to.StringPtr(fmt.Sprintf("https://%s.blob.storage.azurestack.local/", storageAccountName)),
			},
			ProvisioningState: "Succeeded",
		},
	}

	keys := []storage.AccountKey{{
		KeyName:     to.StringPtr("key-1-name"),
		Value:       to.StringPtr("key-1"),
		Permissions: storage.FULL,
	}}
	s.storageAccountKeys = &storage.AccountListKeysResult{
		Keys: &keys,
	}

	s.ubuntuServerSKUs = []compute.VirtualMachineImageResource{
		{Name: to.StringPtr("12.04-LTS")},
		{Name: to.StringPtr("12.10")},
		{Name: to.StringPtr("14.04-LTS")},
		{Name: to.StringPtr("15.04")},
		{Name: to.StringPtr("15.10")},
		{Name: to.StringPtr("16.04-LTS")},
	}

	s.deployment = nil
}

func (s *environSuite) openEnviron(c *gc.C, attrs ...testing.Attrs) environs.Environ {
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
	env, err := provider.Open(environs.OpenParams{
		Cloud:  fakeCloudSpec(),
		Config: cfg,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Force an explicit refresh of the access token, so it isn't done
	// implicitly during the tests.
	*sender = azuretesting.Senders{
		discoverAuthSender(),
		tokenRefreshSender(),
	}
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
	cfg, err := provider.PrepareConfig(environs.PrepareConfigParams{
		Config: makeTestModelConfig(c, attrs...),
		Cloud:  fakeCloudSpec(),
	})
	c.Assert(err, jc.ErrorIsNil)

	env, err := provider.Open(environs.OpenParams{
		Cloud:  fakeCloudSpec(),
		Config: cfg,
	})
	c.Assert(err, jc.ErrorIsNil)

	*sender = azuretesting.Senders{
		discoverAuthSender(),
		tokenRefreshSender(),
	}
	err = env.PrepareForBootstrap(ctx)
	c.Assert(err, jc.ErrorIsNil)
	return env
}

func fakeCloudSpec() environs.CloudSpec {
	return environs.CloudSpec{
		Type:             "azure",
		Name:             "azure",
		Region:           "westus",
		Endpoint:         "https://api.azurestack.local",
		IdentityEndpoint: "https://login.microsoftonline.com",
		StorageEndpoint:  "https://storage.azurestack.local",
		Credential:       fakeServicePrincipalCredential(),
	}
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

func discoverAuthSender() *azuretesting.MockSender {
	const fakeTenantId = "11111111-1111-1111-1111-111111111111"
	sender := mocks.NewSender()
	resp := mocks.NewResponseWithStatus("", http.StatusUnauthorized)
	mocks.SetResponseHeaderValues(resp, "WWW-Authenticate", []string{
		fmt.Sprintf(
			`authorization_uri="https://testing.invalid/%s"`,
			fakeTenantId,
		),
	})
	sender.AppendResponse(resp)
	return &azuretesting.MockSender{
		Sender:      sender,
		PathPattern: ".*/subscriptions/" + fakeSubscriptionId,
	}
}

func (s *environSuite) initResourceGroupSenders() azuretesting.Senders {
	resourceGroupName := "juju-testenv-model-deadbeef-0bad-400d-8000-4b1d0d06f00d"
	senders := azuretesting.Senders{s.makeSender(".*/resourcegroups/"+resourceGroupName, s.group)}
	return senders
}

func (s *environSuite) startInstanceSenders(controller bool) azuretesting.Senders {
	senders := azuretesting.Senders{s.vmSizesSender()}
	if s.ubuntuServerSKUs != nil {
		senders = append(senders, s.makeSender(".*/Canonical/.*/UbuntuServer/skus", s.ubuntuServerSKUs))
	}
	senders = append(senders, s.makeSender("/deployments/machine-0", s.deployment))
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

func (s *environSuite) storageAccountSender() *azuretesting.MockSender {
	return s.makeSender(".*/storageAccounts/"+storageAccountName, s.storageAccount)
}

func (s *environSuite) storageAccountKeysSender() *azuretesting.MockSender {
	return s.makeSender(".*/storageAccounts/.*/listKeys", s.storageAccountKeys)
}

func (s *environSuite) makeSender(pattern string, v interface{}) *azuretesting.MockSender {
	sender := azuretesting.NewSenderWithValue(v)
	sender.PathPattern = pattern
	return sender
}

func makeStartInstanceParams(c *gc.C, controllerUUID, series string) environs.StartInstanceParams {
	machineTag := names.NewMachineTag("0")
	apiInfo := &api.Info{
		Addrs:    []string{"localhost:17777"},
		CACert:   testing.CACert,
		Password: "admin",
		Tag:      machineTag,
		ModelTag: testing.ModelTag,
	}

	icfg, err := instancecfg.NewInstanceConfig(
		names.NewControllerTag(controllerUUID),
		machineTag.Id(), "yanonce", imagemetadata.ReleasedStream,
		series, apiInfo,
	)
	c.Assert(err, jc.ErrorIsNil)
	icfg.Tags = map[string]string{
		tags.JujuModel:      testing.ModelTag.Id(),
		tags.JujuController: controllerUUID,
	}

	return environs.StartInstanceParams{
		ControllerUUID: controllerUUID,
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
	*gitjujutesting.Clock
}

func (c *mockClock) After(d time.Duration) <-chan time.Time {
	c.MethodCall(c, "After", d)
	c.PopNoErr()
	return c.Clock.After(d)
}

func (s *environSuite) TestOpen(c *gc.C) {
	env := s.openEnviron(c)
	c.Assert(env, gc.NotNil)
}

func (s *environSuite) TestCloudEndpointManagementURI(c *gc.C) {
	env := s.openEnviron(c)

	sender := mocks.NewSender()
	sender.AppendResponse(mocks.NewResponseWithContent("{}"))
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
	result, err := env.StartInstance(makeStartInstanceParams(c, s.controllerUUID, "quantal"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.NotNil)
	c.Assert(result.Instance, gc.NotNil)
	c.Assert(result.NetworkInfo, gc.HasLen, 0)
	c.Assert(result.Volumes, gc.HasLen, 0)
	c.Assert(result.VolumeAttachments, gc.HasLen, 0)

	arch := "amd64"
	mem := uint64(3584)
	rootDisk := uint64(30 * 1024) // 30 GiB
	cpuCores := uint64(1)
	c.Assert(result.Hardware, jc.DeepEquals, &instance.HardwareCharacteristics{
		Arch:     &arch,
		Mem:      &mem,
		RootDisk: &rootDisk,
		CpuCores: &cpuCores,
	})
	s.assertStartInstanceRequests(c, s.requests, assertStartInstanceRequestsParams{
		imageReference: &quantalImageReference,
		diskSizeGB:     32,
		osProfile:      &linuxOsProfile,
	})
}

func (s *environSuite) TestStartInstanceWindowsMinRootDisk(c *gc.C) {
	// The minimum OS disk size for Windows machines is 127GiB.
	cons := constraints.MustParse("root-disk=44G")
	s.testStartInstanceWindows(c, cons, 127*1024, 136)
}

func (s *environSuite) TestStartInstanceWindowsGrowableRootDisk(c *gc.C) {
	// The OS disk size may be grown larger than 127GiB.
	cons := constraints.MustParse("root-disk=200G")
	s.testStartInstanceWindows(c, cons, 200*1024, 214)
}

func (s *environSuite) testStartInstanceWindows(
	c *gc.C, cons constraints.Value,
	expect uint64, requestValue int,
) {
	// Starting a Windows VM, we should not expect an image query.
	s.PatchValue(&s.ubuntuServerSKUs, nil)

	env := s.openEnviron(c)
	s.sender = s.startInstanceSenders(false)
	s.requests = nil
	args := makeStartInstanceParams(c, s.controllerUUID, "win2012")
	args.Constraints = cons
	result, err := env.StartInstance(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.NotNil)
	c.Assert(result.Hardware.RootDisk, jc.DeepEquals, &expect)

	vmExtensionSettings := map[string]interface{}{
		"commandToExecute": `` +
			`move C:\AzureData\CustomData.bin C:\AzureData\CustomData.ps1 && ` +
			`powershell.exe -ExecutionPolicy Unrestricted -File C:\AzureData\CustomData.ps1 && ` +
			`del /q C:\AzureData\CustomData.ps1`,
	}
	s.assertStartInstanceRequests(c, s.requests, assertStartInstanceRequestsParams{
		imageReference: &win2012ImageReference,
		diskSizeGB:     requestValue,
		vmExtension: &compute.VirtualMachineExtensionProperties{
			Publisher:               to.StringPtr("Microsoft.Compute"),
			Type:                    to.StringPtr("CustomScriptExtension"),
			TypeHandlerVersion:      to.StringPtr("1.4"),
			AutoUpgradeMinorVersion: to.BoolPtr(true),
			Settings:                &vmExtensionSettings,
		},
		osProfile: &windowsOsProfile,
	})
}

func (s *environSuite) TestStartInstanceCentOS(c *gc.C) {
	// Starting a CentOS VM, we should not expect an image query.
	s.PatchValue(&s.ubuntuServerSKUs, nil)

	env := s.openEnviron(c)
	s.sender = s.startInstanceSenders(false)
	s.requests = nil
	args := makeStartInstanceParams(c, s.controllerUUID, "centos7")
	_, err := env.StartInstance(args)
	c.Assert(err, jc.ErrorIsNil)

	vmExtensionSettings := map[string]interface{}{
		"commandToExecute": `bash -c 'base64 -d /var/lib/waagent/CustomData | bash'`,
	}
	s.assertStartInstanceRequests(c, s.requests, assertStartInstanceRequestsParams{
		imageReference: &centos7ImageReference,
		diskSizeGB:     32,
		vmExtension: &compute.VirtualMachineExtensionProperties{
			Publisher:               to.StringPtr("Microsoft.OSTCExtensions"),
			Type:                    to.StringPtr("CustomScriptForLinux"),
			TypeHandlerVersion:      to.StringPtr("1.4"),
			AutoUpgradeMinorVersion: to.BoolPtr(true),
			Settings:                &vmExtensionSettings,
		},
		osProfile: &linuxOsProfile,
	})
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
	rateLimitedSender.AppendAndRepeatResponse(mocks.NewResponseWithBodyAndStatus(
		mocks.NewBody("{}"), // empty JSON response to appease go-autorest
		http.StatusTooManyRequests,
		"(」゜ロ゜)」",
	), failures)
	successSender := senders[len(senders)-1]
	senders = senders[:len(senders)-1]
	for i := 0; i < failures; i++ {
		senders = append(senders, rateLimitedSender)
	}
	senders = append(senders, successSender)
	s.sender = senders

	_, err := env.StartInstance(makeStartInstanceParams(c, s.controllerUUID, "quantal"))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.requests, gc.HasLen, numExpectedStartInstanceRequests+failures)
	s.assertStartInstanceRequests(c, s.requests[:numExpectedStartInstanceRequests], assertStartInstanceRequestsParams{
		imageReference: &quantalImageReference,
		diskSizeGB:     32,
		osProfile:      &linuxOsProfile,
	})

	// The final requests should all be identical.
	for i := numExpectedStartInstanceRequests; i < numExpectedStartInstanceRequests+failures; i++ {
		c.Assert(s.requests[i].Method, gc.Equals, "PUT")
		c.Assert(s.requests[i].URL.Path, gc.Equals, s.requests[numExpectedStartInstanceRequests-1].URL.Path)
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

	// Make the VirtualMachines.Get call respond with enough 429
	// (StatusTooManyRequests) failures to cause the method to give
	// up retrying.
	rateLimitedSender := mocks.NewSender()
	rateLimitedSender.AppendAndRepeatResponse(mocks.NewResponseWithBodyAndStatus(
		mocks.NewBody("{}"), // empty JSON response to appease go-autorest
		http.StatusTooManyRequests,
		"(」゜ロ゜)」",
	), failures)
	senders = senders[:len(senders)-1]
	for i := 0; i < failures; i++ {
		senders = append(senders, rateLimitedSender)
	}
	s.sender = senders

	_, err := env.StartInstance(makeStartInstanceParams(c, s.controllerUUID, "quantal"))
	c.Assert(err, gc.ErrorMatches, `creating virtual machine "machine-0": creating deployment "machine-0": max duration exceeded: .*`)

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
	unitsDeployed := "mysql/0 wordpress/0"
	s.vmTags[tags.JujuUnitsDeployed] = &unitsDeployed
	s.sender = s.startInstanceSenders(false)
	s.requests = nil
	params := makeStartInstanceParams(c, s.controllerUUID, "quantal")
	params.InstanceConfig.Tags[tags.JujuUnitsDeployed] = unitsDeployed

	_, err := env.StartInstance(params)
	c.Assert(err, jc.ErrorIsNil)
	s.assertStartInstanceRequests(c, s.requests, assertStartInstanceRequestsParams{
		availabilitySetName: "mysql",
		imageReference:      &quantalImageReference,
		diskSizeGB:          32,
		osProfile:           &linuxOsProfile,
	})
}

const numExpectedStartInstanceRequests = 3

type assertStartInstanceRequestsParams struct {
	availabilitySetName string
	imageReference      *compute.ImageReference
	vmExtension         *compute.VirtualMachineExtensionProperties
	diskSizeGB          int
	osProfile           *compute.OSProfile
}

func (s *environSuite) assertStartInstanceRequests(
	c *gc.C,
	requests []*http.Request,
	args assertStartInstanceRequestsParams,
) startInstanceRequests {
	nsgId := `[resourceId('Microsoft.Network/networkSecurityGroups', 'juju-internal-nsg')]`
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
			Priority:                 to.Int32Ptr(100),
			Direction:                network.Inbound,
		},
	}, {
		Name: to.StringPtr("JujuAPIInbound"),
		Properties: &network.SecurityRulePropertiesFormat{
			Description:              to.StringPtr("Allow API connections to controller machines"),
			Protocol:                 network.TCP,
			SourceAddressPrefix:      to.StringPtr("*"),
			SourcePortRange:          to.StringPtr("*"),
			DestinationAddressPrefix: to.StringPtr("192.168.16.0/20"),
			DestinationPortRange:     to.StringPtr("17777"),
			Access:                   network.Allow,
			Priority:                 to.Int32Ptr(101),
			Direction:                network.Inbound,
		},
	}}
	subnets := []network.Subnet{{
		Name: to.StringPtr("juju-internal-subnet"),
		Properties: &network.SubnetPropertiesFormat{
			AddressPrefix: to.StringPtr("192.168.0.0/20"),
			NetworkSecurityGroup: &network.SecurityGroup{
				ID: to.StringPtr(nsgId),
			},
		},
	}, {
		Name: to.StringPtr("juju-controller-subnet"),
		Properties: &network.SubnetPropertiesFormat{
			AddressPrefix: to.StringPtr("192.168.16.0/20"),
			NetworkSecurityGroup: &network.SecurityGroup{
				ID: to.StringPtr(nsgId),
			},
		},
	}}

	subnetName := "juju-internal-subnet"
	privateIPAddress := "192.168.0.4"
	if args.availabilitySetName == "juju-controller" {
		subnetName = "juju-controller-subnet"
		privateIPAddress = "192.168.16.4"
	}
	subnetId := fmt.Sprintf(
		`[concat(resourceId('Microsoft.Network/virtualNetworks', 'juju-internal-network'), '/subnets/%s')]`,
		subnetName,
	)

	publicIPAddressId := `[resourceId('Microsoft.Network/publicIPAddresses', 'machine-0-public-ip')]`

	ipConfigurations := []network.InterfaceIPConfiguration{{
		Name: to.StringPtr("primary"),
		Properties: &network.InterfaceIPConfigurationPropertiesFormat{
			Primary:                   to.BoolPtr(true),
			PrivateIPAddress:          to.StringPtr(privateIPAddress),
			PrivateIPAllocationMethod: network.Static,
			Subnet: &network.Subnet{ID: to.StringPtr(subnetId)},
			PublicIPAddress: &network.PublicIPAddress{
				ID: to.StringPtr(publicIPAddressId),
			},
		},
	}}

	nicId := `[resourceId('Microsoft.Network/networkInterfaces', 'machine-0-primary')]`
	nics := []compute.NetworkInterfaceReference{{
		ID: to.StringPtr(nicId),
		Properties: &compute.NetworkInterfaceReferenceProperties{
			Primary: to.BoolPtr(true),
		},
	}}
	vmDependsOn := []string{
		nicId,
		`[resourceId('Microsoft.Storage/storageAccounts', '` + storageAccountName + `')]`,
	}

	addressPrefixes := []string{"192.168.0.0/20", "192.168.16.0/20"}
	templateResources := []armtemplates.Resource{{
		APIVersion: network.APIVersion,
		Type:       "Microsoft.Network/networkSecurityGroups",
		Name:       "juju-internal-nsg",
		Location:   "westus",
		Tags:       to.StringMap(s.envTags),
		Properties: &network.SecurityGroupPropertiesFormat{
			SecurityRules: &securityRules,
		},
	}, {
		APIVersion: network.APIVersion,
		Type:       "Microsoft.Network/virtualNetworks",
		Name:       "juju-internal-network",
		Location:   "westus",
		Tags:       to.StringMap(s.envTags),
		Properties: &network.VirtualNetworkPropertiesFormat{
			AddressSpace: &network.AddressSpace{&addressPrefixes},
			Subnets:      &subnets,
		},
		DependsOn: []string{nsgId},
	}, {
		APIVersion: storage.APIVersion,
		Type:       "Microsoft.Storage/storageAccounts",
		Name:       storageAccountName,
		Location:   "westus",
		Tags:       to.StringMap(s.envTags),
		StorageSku: &storage.Sku{
			Name: storage.SkuName("Standard_LRS"),
		},
	}}

	var availabilitySetSubResource *compute.SubResource
	if args.availabilitySetName != "" {
		availabilitySetId := fmt.Sprintf(
			`[resourceId('Microsoft.Compute/availabilitySets','%s')]`,
			args.availabilitySetName,
		)
		templateResources = append(templateResources, armtemplates.Resource{
			APIVersion: compute.APIVersion,
			Type:       "Microsoft.Compute/availabilitySets",
			Name:       args.availabilitySetName,
			Location:   "westus",
			Tags:       to.StringMap(s.envTags),
		})
		availabilitySetSubResource = &compute.SubResource{
			ID: to.StringPtr(availabilitySetId),
		}
		vmDependsOn = append([]string{availabilitySetId}, vmDependsOn...)
	}

	templateResources = append(templateResources, []armtemplates.Resource{{
		APIVersion: network.APIVersion,
		Type:       "Microsoft.Network/publicIPAddresses",
		Name:       "machine-0-public-ip",
		Location:   "westus",
		Tags:       to.StringMap(s.vmTags),
		Properties: &network.PublicIPAddressPropertiesFormat{
			PublicIPAllocationMethod: network.Dynamic,
		},
	}, {
		APIVersion: network.APIVersion,
		Type:       "Microsoft.Network/networkInterfaces",
		Name:       "machine-0-primary",
		Location:   "westus",
		Tags:       to.StringMap(s.vmTags),
		Properties: &network.InterfacePropertiesFormat{
			IPConfigurations: &ipConfigurations,
		},
		DependsOn: []string{
			publicIPAddressId,
			`[resourceId('Microsoft.Network/virtualNetworks', 'juju-internal-network')]`,
		},
	}, {
		APIVersion: compute.APIVersion,
		Type:       "Microsoft.Compute/virtualMachines",
		Name:       "machine-0",
		Location:   "westus",
		Tags:       to.StringMap(s.vmTags),
		Properties: &compute.VirtualMachineProperties{
			HardwareProfile: &compute.HardwareProfile{
				VMSize: "Standard_D1",
			},
			StorageProfile: &compute.StorageProfile{
				ImageReference: args.imageReference,
				OsDisk: &compute.OSDisk{
					Name:         to.StringPtr("machine-0"),
					CreateOption: compute.FromImage,
					Caching:      compute.ReadWrite,
					Vhd: &compute.VirtualHardDisk{
						URI: to.StringPtr(fmt.Sprintf(
							`[concat(reference(resourceId('Microsoft.Storage/storageAccounts', '%s'), '%s').primaryEndpoints.blob, 'osvhds/machine-0.vhd')]`,
							storageAccountName, storage.APIVersion,
						)),
					},
					DiskSizeGB: to.Int32Ptr(int32(args.diskSizeGB)),
				},
			},
			OsProfile:       args.osProfile,
			NetworkProfile:  &compute.NetworkProfile{&nics},
			AvailabilitySet: availabilitySetSubResource,
		},
		DependsOn: vmDependsOn,
	}}...)
	if args.vmExtension != nil {
		templateResources = append(templateResources, armtemplates.Resource{
			APIVersion: compute.APIVersion,
			Type:       "Microsoft.Compute/virtualMachines/extensions",
			Name:       "machine-0/JujuCustomScriptExtension",
			Location:   "westus",
			Tags:       to.StringMap(s.vmTags),
			Properties: args.vmExtension,
			DependsOn:  []string{"Microsoft.Compute/virtualMachines/machine-0"},
		})
	}
	templateMap := map[string]interface{}{
		"$schema":        "http://schema.management.azure.com/schemas/2015-01-01/deploymentTemplate.json#",
		"contentVersion": "1.0.0.0",
		"resources":      templateResources,
	}
	deployment := &resources.Deployment{
		&resources.DeploymentProperties{
			Template: &templateMap,
			Mode:     resources.Incremental,
		},
	}

	// Validate HTTP request bodies.
	var startInstanceRequests startInstanceRequests
	if args.vmExtension != nil {
		// It must be Windows or CentOS, so
		// there should be no image query.
		c.Assert(requests, gc.HasLen, numExpectedStartInstanceRequests-1)
		c.Assert(requests[0].Method, gc.Equals, "GET") // vmSizes
		c.Assert(requests[1].Method, gc.Equals, "PUT") // create deployment
		startInstanceRequests.vmSizes = requests[0]
		startInstanceRequests.deployment = requests[1]
	} else {
		c.Assert(requests, gc.HasLen, numExpectedStartInstanceRequests)
		c.Assert(requests[0].Method, gc.Equals, "GET") // vmSizes
		c.Assert(requests[1].Method, gc.Equals, "GET") // skus
		c.Assert(requests[2].Method, gc.Equals, "PUT") // create deployment
		startInstanceRequests.vmSizes = requests[0]
		startInstanceRequests.skus = requests[1]
		startInstanceRequests.deployment = requests[2]
	}

	// Marshal/unmarshal the deployment we expect, so it's in map form.
	var expected resources.Deployment
	data, err := json.Marshal(&deployment)
	c.Assert(err, jc.ErrorIsNil)
	err = json.Unmarshal(data, &expected)
	c.Assert(err, jc.ErrorIsNil)

	// Check that we send what we expect. CustomData is non-deterministic,
	// so don't compare it.
	// TODO(axw) shouldn't CustomData be deterministic? Look into this.
	var actual resources.Deployment
	unmarshalRequestBody(c, startInstanceRequests.deployment, &actual)
	c.Assert(actual.Properties, gc.NotNil)
	c.Assert(actual.Properties.Template, gc.NotNil)
	resources := (*actual.Properties.Template)["resources"].([]interface{})
	c.Assert(resources, gc.HasLen, len(templateResources))

	vmResourceIndex := len(resources) - 1
	if args.vmExtension != nil {
		vmResourceIndex--
	}
	vmResource := resources[vmResourceIndex].(map[string]interface{})
	vmResourceProperties := vmResource["properties"].(map[string]interface{})
	osProfile := vmResourceProperties["osProfile"].(map[string]interface{})
	osProfile["customData"] = "<juju-goes-here>"
	c.Assert(actual, jc.DeepEquals, expected)

	return startInstanceRequests
}

type startInstanceRequests struct {
	vmSizes    *http.Request
	skus       *http.Request
	deployment *http.Request
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
			ControllerConfig: testing.FakeControllerConfig(),
			AvailableTools:   makeToolsList("quantal"),
			BootstrapSeries:  "quantal",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Arch, gc.Equals, "amd64")
	c.Assert(result.Series, gc.Equals, "quantal")

	c.Assert(len(s.requests), gc.Equals, numExpectedStartInstanceRequests+1)
	s.vmTags[tags.JujuIsController] = to.StringPtr("true")
	s.assertStartInstanceRequests(c, s.requests[1:], assertStartInstanceRequestsParams{
		availabilitySetName: "juju-controller",
		imageReference:      &quantalImageReference,
		diskSizeGB:          32,
		osProfile:           &linuxOsProfile,
	})
}

func (s *environSuite) TestAllInstancesResourceGroupNotFound(c *gc.C) {
	env := s.openEnviron(c)
	sender := mocks.NewSender()
	sender.AppendResponse(mocks.NewResponseWithStatus(
		"resource group not found", http.StatusNotFound,
	))
	s.sender = azuretesting.Senders{sender}
	_, err := env.AllInstances()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environSuite) TestStopInstancesNotFound(c *gc.C) {
	env := s.openEnviron(c)
	sender0 := mocks.NewSender()
	sender0.AppendResponse(mocks.NewResponseWithStatus(
		"vm not found", http.StatusNotFound,
	))
	sender1 := mocks.NewSender()
	sender1.AppendResponse(mocks.NewResponseWithStatus(
		"vm not found", http.StatusNotFound,
	))
	s.sender = azuretesting.Senders{sender0, sender1}
	err := env.StopInstances("a", "b")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environSuite) TestStopInstances(c *gc.C) {
	env := s.openEnviron(c)

	// Security group has rules for machine-0, as well as a rule that doesn't match.
	nsg := makeSecurityGroup(
		makeSecurityRule("machine-0-80", "192.168.0.4", "80"),
		makeSecurityRule("machine-0-1000-2000", "192.168.0.4", "1000-2000"),
		makeSecurityRule("machine-42", "192.168.0.5", "*"),
	)

	// Create an IP configuration with a public IP reference. This will
	// cause an update to the NIC to detach public IPs.
	nic0IPConfiguration := makeIPConfiguration("192.168.0.4")
	nic0IPConfiguration.Properties.PublicIPAddress = &network.PublicIPAddress{}
	nic0 := makeNetworkInterface("nic-0", "machine-0", nic0IPConfiguration)

	s.sender = azuretesting.Senders{
		s.makeSender(".*/deployments/machine-0/cancel", nil), // POST
		s.storageAccountSender(),
		s.storageAccountKeysSender(),
		s.networkInterfacesSender(nic0),
		s.publicIPAddressesSender(makePublicIPAddress("pip-0", "machine-0", "1.2.3.4")),
		s.makeSender(".*/virtualMachines/machine-0", nil),                                                 // DELETE
		s.makeSender(".*/networkSecurityGroups/juju-internal-nsg", nsg),                                   // GET
		s.makeSender(".*/networkSecurityGroups/juju-internal-nsg/securityRules/machine-0-80", nil),        // DELETE
		s.makeSender(".*/networkSecurityGroups/juju-internal-nsg/securityRules/machine-0-1000-2000", nil), // DELETE
		s.makeSender(".*/networkInterfaces/nic-0", nil),                                                   // DELETE
		s.makeSender(".*/publicIPAddresses/pip-0", nil),                                                   // DELETE
		s.makeSender(".*/deployments/machine-0", nil),                                                     // DELETE
	}
	err := env.StopInstances("machine-0")
	c.Assert(err, jc.ErrorIsNil)

	s.storageClient.CheckCallNames(c,
		"NewClient", "DeleteBlobIfExists",
	)
	s.storageClient.CheckCall(c, 1, "DeleteBlobIfExists", "osvhds", "machine-0")
}

func (s *environSuite) TestStopInstancesMultiple(c *gc.C) {
	env := s.openEnviron(c)

	vmDeleteSender0 := s.makeSender(".*/virtualMachines/machine-[01]", nil)
	vmDeleteSender1 := s.makeSender(".*/virtualMachines/machine-[01]", nil)
	vmDeleteSender0.SetError(errors.New("blargh"))
	vmDeleteSender1.SetError(errors.New("blargh"))

	s.sender = azuretesting.Senders{
		s.makeSender(".*/deployments/machine-[01]/cancel", nil), // POST
		s.makeSender(".*/deployments/machine-[01]/cancel", nil), // POST

		// We should only query the NICs, public IPs, and storage
		// account/keys, regardless of how many instances are deleted.
		s.storageAccountSender(),
		s.storageAccountKeysSender(),
		s.networkInterfacesSender(),
		s.publicIPAddressesSender(),

		vmDeleteSender0,
		vmDeleteSender1,
	}
	err := env.StopInstances("machine-0", "machine-1")
	c.Assert(err, gc.ErrorMatches, `deleting instance "machine-[01]":.*blargh`)
}

func (s *environSuite) TestStopInstancesDeploymentNotFound(c *gc.C) {
	env := s.openEnviron(c)

	cancelSender := mocks.NewSender()
	cancelSender.AppendResponse(mocks.NewResponseWithStatus(
		"deployment not found", http.StatusNotFound,
	))
	s.sender = azuretesting.Senders{cancelSender}
	err := env.StopInstances("machine-0")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environSuite) TestStopInstancesStorageAccountNoKeys(c *gc.C) {
	s.PatchValue(&s.storageAccountKeys.Keys, nil)
	s.testStopInstancesStorageAccountNotFound(c)
}

func (s *environSuite) TestStopInstancesStorageAccountNoFullKey(c *gc.C) {
	keys := *s.storageAccountKeys.Keys
	s.PatchValue(&keys[0].Permissions, storage.READ)
	s.testStopInstancesStorageAccountNotFound(c)
}

func (s *environSuite) testStopInstancesStorageAccountNotFound(c *gc.C) {
	env := s.openEnviron(c)
	s.sender = azuretesting.Senders{
		s.makeSender("/deployments/machine-0", s.deployment), // Cancel
		s.storageAccountSender(),
		s.storageAccountKeysSender(),
		s.networkInterfacesSender(),                                                     // GET: no NICs
		s.publicIPAddressesSender(),                                                     // GET: no public IPs
		s.makeSender(".*/virtualMachines/machine-0", nil),                               // DELETE
		s.makeSender(".*/networkSecurityGroups/juju-internal-nsg", makeSecurityGroup()), // GET: no rules
		s.makeSender(".*/deployments/machine-0", nil),                                   // DELETE
	}
	err := env.StopInstances("machine-0")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environSuite) TestStopInstancesStorageAccountError(c *gc.C) {
	env := s.openEnviron(c)
	errorSender := s.storageAccountSender()
	errorSender.SetError(errors.New("blargh"))
	s.sender = azuretesting.Senders{
		s.makeSender("/deployments/machine-0", s.deployment), // Cancel
		errorSender,
	}
	err := env.StopInstances("machine-0")
	c.Assert(err, gc.ErrorMatches, "getting storage account:.*blargh")
}

func (s *environSuite) TestStopInstancesStorageAccountKeysError(c *gc.C) {
	env := s.openEnviron(c)
	errorSender := s.storageAccountKeysSender()
	errorSender.SetError(errors.New("blargh"))
	s.sender = azuretesting.Senders{
		s.makeSender("/deployments/machine-0", s.deployment), // Cancel
		s.storageAccountSender(),
		errorSender,
	}
	err := env.StopInstances("machine-0")
	c.Assert(err, gc.ErrorMatches, "getting storage account key:.*blargh")
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

func (s *environSuite) TestDestroyController(c *gc.C) {
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
	err := env.DestroyController(s.controllerUUID)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.requests, gc.HasLen, 3)
	c.Assert(s.requests[0].Method, gc.Equals, "GET")
	c.Assert(s.requests[0].URL.Query().Get("$filter"), gc.Equals, fmt.Sprintf(
		"tagname eq 'juju-controller-uuid' and tagvalue eq '%s'",
		testing.ControllerTag.Id(),
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

func (s *environSuite) TestDestroyControllerErrors(c *gc.C) {
	groups := []resources.ResourceGroup{
		{Name: to.StringPtr("group1")},
		{Name: to.StringPtr("group2")},
	}
	result := resources.ResourceGroupListResult{Value: &groups}

	makeErrorSender := func(err string) *azuretesting.MockSender {
		errorSender := &azuretesting.MockSender{
			Sender:      mocks.NewSender(),
			PathPattern: ".*/resourcegroups/group[12].*",
		}
		errorSender.SetError(errors.New(err))
		return errorSender
	}

	env := s.openEnviron(c)
	s.requests = nil
	s.sender = azuretesting.Senders{
		s.makeSender(".*/resourcegroups", result), // GET
		makeErrorSender("foo"),                    // DELETE
		makeErrorSender("bar"),                    // DELETE
	}
	destroyErr := env.DestroyController(s.controllerUUID)
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

	c.Check(destroyErr, gc.ErrorMatches,
		`deleting resource group "group1":.*; `+
			`deleting resource group "group2":.*`)
	c.Check(destroyErr, gc.ErrorMatches, ".*foo.*")
	c.Check(destroyErr, gc.ErrorMatches, ".*bar.*")
}
