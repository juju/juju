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
	"runtime"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2018-10-01/compute"
	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-08-01/network"
	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2018-05-01/resources"
	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2018-07-01/storage"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/mocks"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/clock/testclock"
	"github.com/juju/names/v4"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/sync"
	"github.com/juju/juju/environs/tags"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/azure"
	"github.com/juju/juju/provider/azure/internal/armtemplates"
	"github.com/juju/juju/provider/azure/internal/azurestorage"
	"github.com/juju/juju/provider/azure/internal/azuretesting"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
)

const (
	storageAccountName = "juju400d80004b1d0d06f00d"

	computeAPIVersion = "2018-10-01"
	networkAPIVersion = "2018-08-01"
	storageAPIVersion = "2018-07-01"
)

var (
	xenialImageReference = compute.ImageReference{
		Publisher: to.StringPtr("Canonical"),
		Offer:     to.StringPtr("UbuntuServer"),
		Sku:       to.StringPtr("18.04-LTS"),
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
		Sku:       to.StringPtr("7.3"),
		Version:   to.StringPtr("latest"),
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

	provider        environs.EnvironProvider
	requests        []*http.Request
	osvhdsContainer azuretesting.MockStorageContainer
	storageClient   azuretesting.MockStorageClient
	sender          azuretesting.Senders
	retryClock      mockClock

	controllerUUID     string
	envTags            map[string]*string
	vmTags             map[string]*string
	group              *resources.Group
	skus               *compute.ResourceSkusResult
	storageAccounts    []storage.Account
	storageAccount     *storage.Account
	storageAccountKeys *storage.AccountListKeysResult
	ubuntuServerSKUs   []compute.VirtualMachineImageResource
	commonDeployment   *resources.DeploymentExtended
	deployment         *resources.Deployment
	sshPublicKeys      []compute.SSHPublicKey
	linuxOsProfile     compute.OSProfile

	callCtx               *context.CloudCallContext
	invalidatedCredential bool
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.osvhdsContainer = azuretesting.MockStorageContainer{}
	s.storageClient = azuretesting.MockStorageClient{
		Containers: map[string]azurestorage.Container{
			"osvhds": &s.osvhdsContainer,
		},
	}
	s.sender = nil
	s.requests = nil
	s.retryClock = mockClock{Clock: testclock.NewClock(time.Time{})}

	s.provider = newProvider(c, azure.ProviderConfig{
		Sender:           azuretesting.NewSerialSender(&s.sender),
		RequestInspector: azuretesting.RequestRecorder(&s.requests),
		NewStorageClient: s.storageClient.NewClient,
		RetryClock: &testclock.AutoAdvancingClock{
			&s.retryClock, s.retryClock.Advance,
		},
		RandomWindowsAdminPassword: func() string { return "sorandom" },
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

	s.group = &resources.Group{
		Location: to.StringPtr("westus"),
		Tags:     s.envTags,
		Properties: &resources.GroupProperties{
			ProvisioningState: to.StringPtr("Succeeded"),
		},
	}

	resourceSkus := []compute.ResourceSku{{
		Name:         to.StringPtr("Standard_A1"),
		Locations:    to.StringSlicePtr([]string{"westus"}),
		ResourceType: to.StringPtr("virtualMachines"),
		Capabilities: &[]compute.ResourceSkuCapabilities{{
			Name:  to.StringPtr("MemoryGB"),
			Value: to.StringPtr("1.75"),
		}, {
			Name:  to.StringPtr("vCPUs"),
			Value: to.StringPtr("1"),
		}, {
			Name:  to.StringPtr("OSVhdSizeMB"),
			Value: to.StringPtr("1047552"),
		}},
	}, {
		Name:         to.StringPtr("Standard_D1"),
		Locations:    to.StringSlicePtr([]string{"westus"}),
		ResourceType: to.StringPtr("virtualMachines"),
		Capabilities: &[]compute.ResourceSkuCapabilities{{
			Name:  to.StringPtr("MemoryGB"),
			Value: to.StringPtr("3.5"),
		}, {
			Name:  to.StringPtr("vCPUs"),
			Value: to.StringPtr("1"),
		}, {
			Name:  to.StringPtr("OSVhdSizeMB"),
			Value: to.StringPtr("1047552"),
		}},
	}, {
		Name:         to.StringPtr("Standard_D2"),
		Locations:    to.StringSlicePtr([]string{"westus"}),
		ResourceType: to.StringPtr("virtualMachines"),
		Capabilities: &[]compute.ResourceSkuCapabilities{{
			Name:  to.StringPtr("MemoryGB"),
			Value: to.StringPtr("7"),
		}, {
			Name:  to.StringPtr("vCPUs"),
			Value: to.StringPtr("2"),
		}, {
			Name:  to.StringPtr("OSVhdSizeMB"),
			Value: to.StringPtr("1047552"),
		}},
	}, {
		Name:         to.StringPtr("Standard_D666"),
		Locations:    to.StringSlicePtr([]string{"westus"}),
		ResourceType: to.StringPtr("virtualMachines"),
		Restrictions: &[]compute.ResourceSkuRestrictions{{
			ReasonCode: compute.NotAvailableForSubscription,
		}},
		Capabilities: &[]compute.ResourceSkuCapabilities{{
			Name:  to.StringPtr("MemoryGB"),
			Value: to.StringPtr("7"),
		}, {
			Name:  to.StringPtr("vCPUs"),
			Value: to.StringPtr("2"),
		}, {
			Name:  to.StringPtr("OSVhdSizeMB"),
			Value: to.StringPtr("1047552"),
		}},
	}}
	s.skus = &compute.ResourceSkusResult{Value: &resourceSkus}

	s.storageAccount = &storage.Account{
		Name: to.StringPtr("my-storage-account"),
		Type: to.StringPtr("Standard_LRS"),
		Tags: s.envTags,
		AccountProperties: &storage.AccountProperties{
			PrimaryEndpoints: &storage.Endpoints{
				Blob: to.StringPtr(fmt.Sprintf("https://%s.blob.storage.azurestack.local/", storageAccountName)),
			},
			ProvisioningState: "Succeeded",
		},
	}

	keys := []storage.AccountKey{{
		KeyName:     to.StringPtr("key-1-name"),
		Value:       to.StringPtr("key-1"),
		Permissions: storage.Full,
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
		{Name: to.StringPtr("18.04-LTS")},
	}

	s.commonDeployment = &resources.DeploymentExtended{
		Properties: &resources.DeploymentPropertiesExtended{
			ProvisioningState: to.StringPtr("Succeeded"),
		},
	}

	s.deployment = nil

	s.sshPublicKeys = []compute.SSHPublicKey{{
		Path:    to.StringPtr("/home/ubuntu/.ssh/authorized_keys"),
		KeyData: to.StringPtr(testing.FakeAuthKeys),
	}}
	s.linuxOsProfile = compute.OSProfile{
		ComputerName:  to.StringPtr("machine-0"),
		CustomData:    to.StringPtr("<juju-goes-here>"),
		AdminUsername: to.StringPtr("ubuntu"),
		LinuxConfiguration: &compute.LinuxConfiguration{
			DisablePasswordAuthentication: to.BoolPtr(true),
			SSH: &compute.SSHConfiguration{
				PublicKeys: &s.sshPublicKeys,
			},
		},
	}

	s.callCtx = &context.CloudCallContext{
		InvalidateCredentialFunc: func(string) error {
			s.invalidatedCredential = true
			return nil
		},
	}
}

func (s *environSuite) TearDownTest(c *gc.C) {
	s.invalidatedCredential = false
	s.BaseSuite.TearDownTest(c)
}

func (s *environSuite) openEnviron(c *gc.C, attrs ...testing.Attrs) environs.Environ {
	env := openEnviron(c, s.provider, &s.sender, attrs...)
	s.requests = nil
	return env
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
	*sender = azuretesting.Senders{
		makeResourceGroupNotFoundSender(fmt.Sprintf(".*/resourcegroups/juju-%s-model-deadbeef-.*", cfg.Name())),
		makeSender(fmt.Sprintf(".*/resourcegroups/juju-%s-.*", cfg.Name()), makeResourceGroupResult()),
	}
	env, err := environs.Open(provider, environs.OpenParams{
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
	azure.SetRetries(env)
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

	*sender = azuretesting.Senders{
		makeResourceGroupNotFoundSender(".*/resourcegroups/juju-testmodel-model-deadbeef-.*"),
		makeSender(".*/resourcegroups/juju-testmodel-.*", makeResourceGroupResult()),
	}
	env, err := environs.Open(provider, environs.OpenParams{
		Cloud:  fakeCloudSpec(),
		Config: cfg,
	})
	c.Assert(err, jc.ErrorIsNil)

	*sender = azuretesting.Senders{
		discoverAuthSender(),
		tokenRefreshSender(),
	}
	err = env.PrepareForBootstrap(ctx, "controller-1")
	c.Assert(err, jc.ErrorIsNil)
	return env
}

func fakeCloudSpec() environscloudspec.CloudSpec {
	return environscloudspec.CloudSpec{
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
	tokenRefreshSender := azuretesting.NewSenderWithValue(&adal.Token{
		AccessToken: "access-token",
		ExpiresOn:   json.Number(fmt.Sprint(time.Now().Add(time.Hour).Unix())),
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

func (s *environSuite) initResourceGroupSenders(resourceGroupName string) azuretesting.Senders {
	senders := azuretesting.Senders{makeSender(".*/resourcegroups/"+resourceGroupName, s.group)}
	return senders
}

func (s *environSuite) startInstanceSenders(bootstrap bool) azuretesting.Senders {
	senders := azuretesting.Senders{s.resourceSkusSender()}
	if s.ubuntuServerSKUs != nil {
		senders = append(senders, makeSender(".*/Canonical/.*/UbuntuServer/skus", s.ubuntuServerSKUs))
	}
	if !bootstrap {
		// When starting an instance, we must wait for the common
		// deployment to complete.
		senders = append(senders, makeSender("/deployments/common", s.commonDeployment))

		// If the deployment has any providers, then we assume
		// storage accounts are in use, for unmanaged storage.
		if s.commonDeployment.Properties.Providers != nil {
			storageAccount := &storage.Account{
				AccountProperties: &storage.AccountProperties{
					PrimaryEndpoints: &storage.Endpoints{
						Blob: to.StringPtr("https://blob.storage/"),
					},
				},
			}
			senders = append(senders, makeSender("/storageAccounts/juju400d80004b1d0d06f00d", storageAccount))
		}
	}
	senders = append(senders, makeSender("/deployments/machine-0", s.deployment))
	return senders
}

func (s *environSuite) startInstanceSendersNoSizes() azuretesting.Senders {
	senders := azuretesting.Senders{}
	if s.ubuntuServerSKUs != nil {
		senders = append(senders, makeSender(".*/Canonical/.*/UbuntuServer/skus", s.ubuntuServerSKUs))
	}
	senders = append(senders, makeSender("/deployments/machine-0", s.deployment))
	return senders
}

func (s *environSuite) networkInterfacesSender(nics ...network.Interface) *azuretesting.MockSender {
	return makeSender(".*/networkInterfaces", network.InterfaceListResult{Value: &nics})
}

func (s *environSuite) publicIPAddressesSender(pips ...network.PublicIPAddress) *azuretesting.MockSender {
	return makeSender(".*/publicIPAddresses", network.PublicIPAddressListResult{Value: &pips})
}

func (s *environSuite) resourceSkusSender() *azuretesting.MockSender {
	return makeSender(".*/skus", s.skus)
}

func (s *environSuite) storageAccountSender() *azuretesting.MockSender {
	return makeSender(".*/storageAccounts/"+storageAccountName, s.storageAccount)
}

func (s *environSuite) storageAccountErrorSender(c *gc.C, err error, repeat int) *azuretesting.MockSender {
	return s.makeErrorSenderWithContent(c, ".*/storageAccounts/"+storageAccountName, s.storageAccount, err, repeat)
}

func (s *environSuite) storageAccountKeysSender() *azuretesting.MockSender {
	return makeSender(".*/storageAccounts/.*/listKeys", s.storageAccountKeys)
}

func (s *environSuite) storageAccountKeysErrorSender(c *gc.C, err error, repeat int) *azuretesting.MockSender {
	return s.makeErrorSenderWithContent(c, ".*/storageAccounts/.*/listKeys", s.storageAccountKeys, err, repeat)
}

func makeResourceGroupNotFoundSender(pattern string) *azuretesting.MockSender {
	sender := azuretesting.MockSender{Sender: mocks.NewSender()}
	sender.PathPattern = pattern
	sender.AppendAndRepeatResponse(mocks.NewResponseWithStatus(
		"resource group not found", http.StatusNotFound,
	), 1)
	return &sender
}

func makeSender(pattern string, v interface{}) *azuretesting.MockSender {
	sender := azuretesting.NewSenderWithValue(v)
	sender.PathPattern = pattern
	return sender
}

func (s *environSuite) makeErrorSender(c *gc.C, pattern string, err error, repeat int) *azuretesting.MockSender {
	return s.makeErrorSenderWithContent(c, pattern, nil, err, repeat)
}

func (s *environSuite) makeErrorSenderWithContent(c *gc.C, pattern string, v interface{}, err error, repeat int) *azuretesting.MockSender {
	sender := azuretesting.NewSenderWithValue(nil)
	sender.PathPattern = pattern
	content, jerr := json.Marshal(v)
	c.Assert(jerr, jc.ErrorIsNil)
	sender.AppendResponse(mocks.NewResponseWithContent(string(content)))
	sender.SetAndRepeatError(err, repeat)
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
	*testclock.Clock
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
	env.AllRunningInstances(s.callCtx) // trigger a query

	c.Assert(s.requests, gc.HasLen, 1)
	c.Assert(s.requests[0].URL.Host, gc.Equals, "api.azurestack.local")
}

func (s *environSuite) TestCloudEndpointManagementURIWithCredentialError(c *gc.C) {
	env := s.openEnviron(c)
	s.createSenderWithUnauthorisedStatusCode(c)
	s.requests = nil

	c.Assert(s.invalidatedCredential, jc.IsFalse)
	env.AllRunningInstances(s.callCtx) // trigger a query
	c.Assert(s.requests, gc.HasLen, 1)
	c.Assert(s.requests[0].URL.Host, gc.Equals, "api.azurestack.local")
	c.Assert(s.invalidatedCredential, jc.IsTrue)
}

func (s *environSuite) TestStartInstance(c *gc.C) {
	s.assertStartInstance(c, nil, true)
}

func (s *environSuite) TestStartInstancePrivateIP(c *gc.C) {
	s.assertStartInstance(c, nil, false)
}

func (s *environSuite) TestStartInstanceRootDiskSmallerThanMin(c *gc.C) {
	wantedRootDisk := 22
	s.assertStartInstance(c, &wantedRootDisk, true)
}

func (s *environSuite) TestStartInstanceRootDiskLargerThanMin(c *gc.C) {
	wantedRootDisk := 40
	s.assertStartInstance(c, &wantedRootDisk, true)
}

func (s *environSuite) assertStartInstance(c *gc.C, wantedRootDisk *int, publicIP bool) {
	env := s.openEnviron(c, testing.Attrs{"use-public-ip": publicIP})
	s.sender = s.startInstanceSenders(false)
	s.requests = nil
	args := makeStartInstanceParams(c, s.controllerUUID, "bionic")
	expectedRootDisk := uint64(30 * 1024) // 30 GiB
	expectedDiskSize := 32
	if wantedRootDisk != nil {
		cons := constraints.MustParse(fmt.Sprintf("root-disk=%dG", *wantedRootDisk))
		args.Constraints = cons
		if *wantedRootDisk > 30 {
			expectedRootDisk = uint64(*wantedRootDisk * 1024)
			expectedDiskSize = *wantedRootDisk + 2
		}
	}
	result, err := env.StartInstance(s.callCtx, args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.NotNil)
	c.Assert(result.Instance, gc.NotNil)
	c.Assert(result.NetworkInfo, gc.HasLen, 0)
	c.Assert(result.Volumes, gc.HasLen, 0)
	c.Assert(result.VolumeAttachments, gc.HasLen, 0)

	arch := "amd64"
	mem := uint64(1792)
	cpuCores := uint64(1)
	c.Assert(result.Hardware, jc.DeepEquals, &instance.HardwareCharacteristics{
		Arch:     &arch,
		Mem:      &mem,
		RootDisk: &expectedRootDisk,
		CpuCores: &cpuCores,
	})
	s.assertStartInstanceRequests(c, s.requests, assertStartInstanceRequestsParams{
		imageReference: &xenialImageReference,
		diskSizeGB:     expectedDiskSize,
		osProfile:      &s.linuxOsProfile,
		instanceType:   "Standard_A1",
		publicIP:       publicIP,
	})
}

func (s *environSuite) TestStartInstanceNoAuthorizedKeys(c *gc.C) {
	env := s.openEnviron(c)
	cfg, err := env.Config().Remove([]string{"authorized-keys"})
	c.Assert(err, jc.ErrorIsNil)
	err = env.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	s.sender = s.startInstanceSenders(false)
	s.requests = nil
	_, err = env.StartInstance(s.callCtx, makeStartInstanceParams(c, s.controllerUUID, "bionic"))
	c.Assert(err, jc.ErrorIsNil)

	s.PatchValue(&s.sshPublicKeys, []compute.SSHPublicKey{{
		Path:    to.StringPtr("/home/ubuntu/.ssh/authorized_keys"),
		KeyData: to.StringPtr("public"),
	}})
	s.assertStartInstanceRequests(c, s.requests, assertStartInstanceRequestsParams{
		imageReference: &xenialImageReference,
		diskSizeGB:     32,
		osProfile:      &s.linuxOsProfile,
		instanceType:   "Standard_A1",
		publicIP:       true,
	})
}

func (s *environSuite) createSenderWithUnauthorisedStatusCode(c *gc.C) {
	mockSender := mocks.NewSender()
	mockSender.AppendAndRepeatResponse(mocks.NewResponseWithStatus("401 Unauthorized", http.StatusUnauthorized), 2)
	s.sender = azuretesting.Senders{mockSender}
}

func (s *environSuite) TestStartInstanceInvalidCredential(c *gc.C) {
	env := s.openEnviron(c)
	cfg, err := env.Config().Remove([]string{"authorized-keys"})
	c.Assert(err, jc.ErrorIsNil)
	err = env.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	s.createSenderWithUnauthorisedStatusCode(c)
	s.requests = nil
	c.Assert(s.invalidatedCredential, jc.IsFalse)

	_, err = env.StartInstance(s.callCtx, makeStartInstanceParams(c, s.controllerUUID, "bionic"))
	c.Assert(err, gc.NotNil)
	c.Assert(s.invalidatedCredential, jc.IsTrue)
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
	result, err := env.StartInstance(s.callCtx, args)
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
		osProfile:    &windowsOsProfile,
		instanceType: "Standard_A1",
		publicIP:     true,
	})
}

func (s *environSuite) TestStartInstanceCentOS(c *gc.C) {
	for _, series := range []string{"centos7", "centos8"} {
		s.assertStartInstanceCentOS(c, series)
	}
}

func (s *environSuite) assertStartInstanceCentOS(c *gc.C, series string) {
	// Starting a CentOS VM, we should not expect an image query.
	s.PatchValue(&s.ubuntuServerSKUs, nil)

	env := s.openEnviron(c)
	s.sender = s.startInstanceSenders(false)
	s.requests = nil
	args := makeStartInstanceParams(c, s.controllerUUID, series)
	_, err := env.StartInstance(s.callCtx, args)
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
		osProfile:    &s.linuxOsProfile,
		instanceType: "Standard_A1",
		publicIP:     true,
	})
}

func (s *environSuite) TestStartInstanceCommonDeployment(c *gc.C) {
	// StartInstance waits for the "common" deployment to complete
	// successfully before creating the VM deployment. If the deployment
	// is seen to be in a terminal state, the process will stop
	// immediately.
	s.commonDeployment.Properties.ProvisioningState = to.StringPtr("Failed")

	env := s.openEnviron(c)
	senders := s.startInstanceSenders(false)
	s.sender = senders
	s.requests = nil

	_, err := env.StartInstance(s.callCtx, makeStartInstanceParams(c, s.controllerUUID, "bionic"))
	c.Assert(err, gc.ErrorMatches,
		`creating virtual machine "machine-0": `+
			`waiting for common resources to be created: `+
			`common resource deployment status is "Failed"`)
}

func (s *environSuite) TestStartInstanceCommonDeploymentStorageAccount(c *gc.C) {
	resourceTypes := []resources.ProviderResourceType{{
		ResourceType: to.StringPtr("storageAccounts"),
	}}
	providers := []resources.Provider{{
		Namespace:     to.StringPtr("Microsoft.Storage"),
		ResourceTypes: &resourceTypes,
	}}
	s.commonDeployment.Properties.Providers = &providers

	env := s.openEnviron(c)
	senders := s.startInstanceSenders(false)
	s.sender = senders
	s.requests = nil

	_, err := env.StartInstance(s.callCtx, makeStartInstanceParams(c, s.controllerUUID, "bionic"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertStartInstanceRequests(c, s.requests[1:], assertStartInstanceRequestsParams{
		imageReference:   &xenialImageReference,
		diskSizeGB:       32,
		osProfile:        &s.linuxOsProfile,
		instanceType:     "Standard_A1",
		unmanagedStorage: true,
		publicIP:         true,
	})
}

func (s *environSuite) TestStartInstanceCommonDeploymentWithStorageAccountAndAvailabilitySetName(c *gc.C) {
	resourceTypes := []resources.ProviderResourceType{{
		ResourceType: to.StringPtr("storageAccounts"),
	}}
	providers := []resources.Provider{{
		Namespace:     to.StringPtr("Microsoft.Storage"),
		ResourceTypes: &resourceTypes,
	}}
	s.commonDeployment.Properties.Providers = &providers

	env := s.openEnviron(c)
	unitsDeployed := "mysql/0 wordpress/0"
	s.vmTags[tags.JujuUnitsDeployed] = &unitsDeployed
	s.sender = s.startInstanceSenders(false)
	s.requests = nil
	params := makeStartInstanceParams(c, s.controllerUUID, "bionic")
	params.InstanceConfig.Tags[tags.JujuUnitsDeployed] = unitsDeployed

	_, err := env.StartInstance(s.callCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	s.assertStartInstanceRequests(c, s.requests[1:], assertStartInstanceRequestsParams{
		availabilitySetName: "mysql",
		imageReference:      &xenialImageReference,
		diskSizeGB:          32,
		osProfile:           &s.linuxOsProfile,
		instanceType:        "Standard_A1",
		unmanagedStorage:    true,
		publicIP:            true,
	})
}

func (s *environSuite) TestStartInstanceCommonDeploymentRetryTimeout(c *gc.C) {
	// StartInstance waits for the "common" deployment to complete
	// successfully before creating the VM deployment.
	s.commonDeployment.Properties.ProvisioningState = to.StringPtr("Running")

	env := s.openEnviron(c)
	senders := s.startInstanceSenders(false)

	const failures = 60 // 5 minutes / 5 seconds
	head, tail := senders[:2], senders[2:]
	for i := 0; i < failures; i++ {
		head = append(head, makeSender("/deployments/common", s.commonDeployment))
	}
	senders = append(head, tail...)
	s.sender = senders
	s.requests = nil

	_, err := env.StartInstance(s.callCtx, makeStartInstanceParams(c, s.controllerUUID, "bionic"))
	c.Assert(err, gc.ErrorMatches,
		`creating virtual machine "machine-0": `+
			`waiting for common resources to be created: `+
			`max duration exceeded: deployment incomplete`)

	var expectedCalls []gitjujutesting.StubCall
	for i := 0; i < failures; i++ {
		expectedCalls = append(expectedCalls, gitjujutesting.StubCall{
			"After", []interface{}{5 * time.Second},
		})
	}
	s.retryClock.CheckCalls(c, expectedCalls)
}

func (s *environSuite) TestStartInstanceServiceAvailabilitySet(c *gc.C) {
	env := s.openEnviron(c)
	unitsDeployed := "mysql/0 wordpress/0"
	s.vmTags[tags.JujuUnitsDeployed] = &unitsDeployed
	s.sender = s.startInstanceSenders(false)
	s.requests = nil
	params := makeStartInstanceParams(c, s.controllerUUID, "bionic")
	params.InstanceConfig.Tags[tags.JujuUnitsDeployed] = unitsDeployed

	_, err := env.StartInstance(s.callCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	s.assertStartInstanceRequests(c, s.requests, assertStartInstanceRequestsParams{
		availabilitySetName: "mysql",
		imageReference:      &xenialImageReference,
		diskSizeGB:          32,
		osProfile:           &s.linuxOsProfile,
		instanceType:        "Standard_A1",
		publicIP:            true,
	})
}

// numExpectedStartInstanceRequests is the number of expected requests base
// by StartInstance method calls. The number is one less for Bootstrap, which
// does not require a query on the common deployment.
const numExpectedStartInstanceRequests = 4

type assertStartInstanceRequestsParams struct {
	autocert            bool
	availabilitySetName string
	imageReference      *compute.ImageReference
	vmExtension         *compute.VirtualMachineExtensionProperties
	diskSizeGB          int
	osProfile           *compute.OSProfile
	needsProviderInit   bool
	customResourceGroup bool
	unmanagedStorage    bool
	instanceType        string
	publicIP            bool
}

func (s *environSuite) assertStartInstanceRequests(
	c *gc.C,
	requests []*http.Request,
	args assertStartInstanceRequestsParams,
) startInstanceRequests {
	nsgId := `[resourceId('Microsoft.Network/networkSecurityGroups', 'juju-internal-nsg')]`
	securityRules := []network.SecurityRule{{
		Name: to.StringPtr("SSHInbound"),
		SecurityRulePropertiesFormat: &network.SecurityRulePropertiesFormat{
			Description:              to.StringPtr("Allow SSH access to all machines"),
			Protocol:                 network.SecurityRuleProtocolTCP,
			SourceAddressPrefix:      to.StringPtr("*"),
			SourcePortRange:          to.StringPtr("*"),
			DestinationAddressPrefix: to.StringPtr("*"),
			DestinationPortRange:     to.StringPtr("22"),
			Access:                   network.SecurityRuleAccessAllow,
			Priority:                 to.Int32Ptr(100),
			Direction:                network.SecurityRuleDirectionInbound,
		},
	}}
	if args.autocert {
		// Since a DNS name has been provided, Let's Encrypt is enabled.
		// Therefore ports 443 (for the API server) and 80 (for the HTTP
		// challenge) are accessible.
		securityRules = append(securityRules, network.SecurityRule{
			Name: to.StringPtr("JujuAPIInbound443"),
			SecurityRulePropertiesFormat: &network.SecurityRulePropertiesFormat{
				Description:              to.StringPtr("Allow API connections to controller machines"),
				Protocol:                 network.SecurityRuleProtocolTCP,
				SourceAddressPrefix:      to.StringPtr("*"),
				SourcePortRange:          to.StringPtr("*"),
				DestinationAddressPrefix: to.StringPtr("192.168.16.0/20"),
				DestinationPortRange:     to.StringPtr("443"),
				Access:                   network.SecurityRuleAccessAllow,
				Priority:                 to.Int32Ptr(101),
				Direction:                network.SecurityRuleDirectionInbound,
			},
		}, network.SecurityRule{
			Name: to.StringPtr("JujuAPIInbound80"),
			SecurityRulePropertiesFormat: &network.SecurityRulePropertiesFormat{
				Description:              to.StringPtr("Allow API connections to controller machines"),
				Protocol:                 network.SecurityRuleProtocolTCP,
				SourceAddressPrefix:      to.StringPtr("*"),
				SourcePortRange:          to.StringPtr("*"),
				DestinationAddressPrefix: to.StringPtr("192.168.16.0/20"),
				DestinationPortRange:     to.StringPtr("80"),
				Access:                   network.SecurityRuleAccessAllow,
				Priority:                 to.Int32Ptr(102),
				Direction:                network.SecurityRuleDirectionInbound,
			},
		})
	} else {
		port := fmt.Sprint(testing.FakeControllerConfig()["api-port"])
		securityRules = append(securityRules, network.SecurityRule{
			Name: to.StringPtr("JujuAPIInbound" + port),
			SecurityRulePropertiesFormat: &network.SecurityRulePropertiesFormat{
				Description:              to.StringPtr("Allow API connections to controller machines"),
				Protocol:                 network.SecurityRuleProtocolTCP,
				SourceAddressPrefix:      to.StringPtr("*"),
				SourcePortRange:          to.StringPtr("*"),
				DestinationAddressPrefix: to.StringPtr("192.168.16.0/20"),
				DestinationPortRange:     to.StringPtr(port),
				Access:                   network.SecurityRuleAccessAllow,
				Priority:                 to.Int32Ptr(101),
				Direction:                network.SecurityRuleDirectionInbound,
			},
		})
	}
	subnets := []network.Subnet{{
		Name: to.StringPtr("juju-internal-subnet"),
		SubnetPropertiesFormat: &network.SubnetPropertiesFormat{
			AddressPrefix: to.StringPtr("192.168.0.0/20"),
			NetworkSecurityGroup: &network.SecurityGroup{
				ID: to.StringPtr(nsgId),
			},
		},
	}, {
		Name: to.StringPtr("juju-controller-subnet"),
		SubnetPropertiesFormat: &network.SubnetPropertiesFormat{
			AddressPrefix: to.StringPtr("192.168.16.0/20"),
			NetworkSecurityGroup: &network.SecurityGroup{
				ID: to.StringPtr(nsgId),
			},
		},
	}}

	createCommonResources := false
	subnetName := "juju-internal-subnet"
	if args.availabilitySetName == "juju-controller" {
		subnetName = "juju-controller-subnet"
		createCommonResources = true
	}
	subnetId := fmt.Sprintf(
		`[concat(resourceId('Microsoft.Network/virtualNetworks', 'juju-internal-network'), '/subnets/%s')]`,
		subnetName,
	)

	var nicDependsOn []string
	if createCommonResources {
		nicDependsOn = append(nicDependsOn,
			`[resourceId('Microsoft.Network/virtualNetworks', 'juju-internal-network')]`,
		)
	}
	var publicIPAddress *network.PublicIPAddress
	if args.publicIP {
		publicIPAddressId := `[resourceId('Microsoft.Network/publicIPAddresses', 'machine-0-public-ip')]`
		publicIPAddress = &network.PublicIPAddress{
			ID: to.StringPtr(publicIPAddressId),
		}
		nicDependsOn = append(nicDependsOn, publicIPAddressId)
	}

	ipConfigurations := []network.InterfaceIPConfiguration{{
		Name: to.StringPtr("primary"),
		InterfaceIPConfigurationPropertiesFormat: &network.InterfaceIPConfigurationPropertiesFormat{
			Primary:                   to.BoolPtr(true),
			PrivateIPAllocationMethod: network.Dynamic,
			Subnet:                    &network.Subnet{ID: to.StringPtr(subnetId)},
			PublicIPAddress:           publicIPAddress,
		},
	}}

	nicId := `[resourceId('Microsoft.Network/networkInterfaces', 'machine-0-primary')]`
	nics := []compute.NetworkInterfaceReference{{
		ID: to.StringPtr(nicId),
		NetworkInterfaceReferenceProperties: &compute.NetworkInterfaceReferenceProperties{
			Primary: to.BoolPtr(true),
		},
	}}

	var vmDependsOn []string
	var templateResources []armtemplates.Resource
	if createCommonResources {
		addressPrefixes := []string{"192.168.0.0/20", "192.168.16.0/20"}
		templateResources = append(templateResources, []armtemplates.Resource{{
			APIVersion: networkAPIVersion,
			Type:       "Microsoft.Network/networkSecurityGroups",
			Name:       "juju-internal-nsg",
			Location:   "westus",
			Tags:       to.StringMap(s.envTags),
			Properties: &network.SecurityGroupPropertiesFormat{
				SecurityRules: &securityRules,
			},
		}, {
			APIVersion: networkAPIVersion,
			Type:       "Microsoft.Network/virtualNetworks",
			Name:       "juju-internal-network",
			Location:   "westus",
			Tags:       to.StringMap(s.envTags),
			Properties: &network.VirtualNetworkPropertiesFormat{
				AddressSpace: &network.AddressSpace{&addressPrefixes},
				Subnets:      &subnets,
			},
			DependsOn: []string{nsgId},
		}}...)
		if args.unmanagedStorage {
			templateResources = append(templateResources, armtemplates.Resource{
				APIVersion: storageAPIVersion,
				Type:       "Microsoft.Storage/storageAccounts",
				Name:       storageAccountName,
				Location:   "westus",
				Tags:       to.StringMap(s.envTags),
				Sku:        &armtemplates.Sku{Name: "Standard_LRS"},
			})
			vmDependsOn = append(vmDependsOn,
				`[resourceId('Microsoft.Storage/storageAccounts', '`+storageAccountName+`')]`,
			)
		}
	}

	var availabilitySetSubResource *compute.SubResource
	if args.availabilitySetName != "" {
		availabilitySetId := fmt.Sprintf(
			`[resourceId('Microsoft.Compute/availabilitySets','%s')]`,
			args.availabilitySetName,
		)
		var (
			availabilitySetProperties  interface{}
			availabilityStorageOptions *armtemplates.Sku
		)
		if !args.unmanagedStorage {
			availabilitySetProperties = &compute.AvailabilitySetProperties{
				PlatformFaultDomainCount: to.Int32Ptr(3),
			}
			availabilityStorageOptions = &armtemplates.Sku{Name: "Aligned"}
		}
		templateResources = append(templateResources, armtemplates.Resource{
			APIVersion: computeAPIVersion,
			Type:       "Microsoft.Compute/availabilitySets",
			Name:       args.availabilitySetName,
			Location:   "westus",
			Tags:       to.StringMap(s.envTags),
			Properties: availabilitySetProperties,
			Sku:        availabilityStorageOptions,
		})
		availabilitySetSubResource = &compute.SubResource{
			ID: to.StringPtr(availabilitySetId),
		}
		vmDependsOn = append(vmDependsOn, availabilitySetId)
	}

	osDisk := &compute.OSDisk{
		Name:         to.StringPtr("machine-0"),
		CreateOption: compute.DiskCreateOptionTypesFromImage,
		Caching:      compute.CachingTypesReadWrite,
		DiskSizeGB:   to.Int32Ptr(int32(args.diskSizeGB)),
	}
	if args.unmanagedStorage {
		osDisk.Vhd = &compute.VirtualHardDisk{
			URI: to.StringPtr(`https://blob.storage/osvhds/machine-0.vhd`),
		}
	} else {
		osDisk.ManagedDisk = &compute.ManagedDiskParameters{
			StorageAccountType: "Standard_LRS",
		}
	}

	if args.publicIP {
		templateResources = append(templateResources, armtemplates.Resource{
			APIVersion: networkAPIVersion,
			Type:       "Microsoft.Network/publicIPAddresses",
			Name:       "machine-0-public-ip",
			Location:   "westus",
			Tags:       to.StringMap(s.vmTags),
			Properties: &network.PublicIPAddressPropertiesFormat{
				PublicIPAllocationMethod: network.Static,
				PublicIPAddressVersion:   "IPv4",
			},
			Sku: &armtemplates.Sku{Name: "Standard"},
		})
	}
	templateResources = append(templateResources, []armtemplates.Resource{{
		APIVersion: networkAPIVersion,
		Type:       "Microsoft.Network/networkInterfaces",
		Name:       "machine-0-primary",
		Location:   "westus",
		Tags:       to.StringMap(s.vmTags),
		Properties: &network.InterfacePropertiesFormat{
			IPConfigurations: &ipConfigurations,
		},
		DependsOn: nicDependsOn,
	}, {
		APIVersion: computeAPIVersion,
		Type:       "Microsoft.Compute/virtualMachines",
		Name:       "machine-0",
		Location:   "westus",
		Tags:       to.StringMap(s.vmTags),
		Properties: &compute.VirtualMachineProperties{
			HardwareProfile: &compute.HardwareProfile{
				VMSize: compute.VirtualMachineSizeTypes(args.instanceType),
			},
			StorageProfile: &compute.StorageProfile{
				ImageReference: args.imageReference,
				OsDisk:         osDisk,
			},
			OsProfile:       args.osProfile,
			NetworkProfile:  &compute.NetworkProfile{&nics},
			AvailabilitySet: availabilitySetSubResource,
		},
		DependsOn: append(vmDependsOn, nicId),
	}}...)
	if args.vmExtension != nil {
		templateResources = append(templateResources, armtemplates.Resource{
			APIVersion: computeAPIVersion,
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
		Properties: &resources.DeploymentProperties{
			Template: &templateMap,
			Mode:     resources.Incremental,
		},
	}

	var i int
	nexti := func() int {
		i++
		return i - 1
	}

	// Validate HTTP request bodies.
	var startInstanceRequests startInstanceRequests
	if args.vmExtension != nil {
		// It must be Windows or CentOS, so
		// there should be no image query.
		c.Assert(requests, gc.HasLen, numExpectedStartInstanceRequests-1)
		c.Assert(requests[nexti()].Method, gc.Equals, "GET") // vmSizes
		startInstanceRequests.vmSizes = requests[0]
	} else {
		if createCommonResources {
			c.Assert(requests, gc.HasLen, numExpectedStartInstanceRequests-1)
		} else {
			c.Assert(requests, gc.HasLen, numExpectedStartInstanceRequests)
		}
		if args.needsProviderInit {
			if args.customResourceGroup {
				c.Assert(requests[nexti()].Method, gc.Equals, "GET") // resource groups
			} else {
				c.Assert(requests[nexti()].Method, gc.Equals, "PUT") // resource groups
			}
			c.Assert(requests[nexti()].Method, gc.Equals, "GET") // skus
			startInstanceRequests.resourceGroups = requests[0]
			startInstanceRequests.skus = requests[1]
		} else {
			c.Assert(requests[nexti()].Method, gc.Equals, "GET") // vmSizes
			c.Assert(requests[nexti()].Method, gc.Equals, "GET") // skus
			startInstanceRequests.vmSizes = requests[0]
			startInstanceRequests.skus = requests[1]
		}
	}
	if !createCommonResources {
		c.Assert(requests[nexti()].Method, gc.Equals, "GET") // wait for common deployment
	}
	ideployment := nexti()
	c.Assert(requests[ideployment].Method, gc.Equals, "PUT") // create deployment
	startInstanceRequests.deployment = requests[ideployment]

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
	resources, ok := actual.Properties.Template.(map[string]interface{})["resources"].([]interface{})
	c.Assert(ok, jc.IsTrue)
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
	resourceGroups *http.Request
	vmSizes        *http.Request
	skus           *http.Request
	deployment     *http.Request
}

const resourceGroupName = "juju-testmodel-deadbeef"

func (s *environSuite) TestBootstrap(c *gc.C) {
	defer envtesting.DisableFinishBootstrap()()

	ctx := envtesting.BootstrapContext(c)
	env := prepareForBootstrap(c, ctx, s.provider, &s.sender)

	s.sender = s.initResourceGroupSenders(resourceGroupName)
	s.sender = append(s.sender, s.startInstanceSenders(true)...)
	s.requests = nil
	result, err := env.Bootstrap(
		ctx, s.callCtx, environs.BootstrapParams{
			ControllerConfig:         testing.FakeControllerConfig(),
			AvailableTools:           makeToolsList("bionic"),
			BootstrapSeries:          "bionic",
			BootstrapConstraints:     constraints.MustParse("mem=3.5G"),
			SupportedBootstrapSeries: testing.FakeSupportedJujuSeries,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Arch, gc.Equals, "amd64")
	c.Assert(result.Series, gc.Equals, "bionic")

	c.Assert(len(s.requests), gc.Equals, numExpectedStartInstanceRequests)
	s.vmTags[tags.JujuIsController] = to.StringPtr("true")
	s.assertStartInstanceRequests(c, s.requests[1:], assertStartInstanceRequestsParams{
		availabilitySetName: "juju-controller",
		imageReference:      &xenialImageReference,
		diskSizeGB:          32,
		osProfile:           &s.linuxOsProfile,
		instanceType:        "Standard_D1",
		publicIP:            true,
	})
}

func (s *environSuite) TestBootstrapPrivateIP(c *gc.C) {
	defer envtesting.DisableFinishBootstrap()()

	ctx := envtesting.BootstrapContext(c)
	env := prepareForBootstrap(c, ctx, s.provider, &s.sender, testing.Attrs{"use-public-ip": false})

	s.sender = s.initResourceGroupSenders(resourceGroupName)
	s.sender = append(s.sender, s.startInstanceSenders(true)...)
	s.requests = nil
	result, err := env.Bootstrap(
		ctx, s.callCtx, environs.BootstrapParams{
			ControllerConfig:         testing.FakeControllerConfig(),
			AvailableTools:           makeToolsList("bionic"),
			BootstrapSeries:          "bionic",
			BootstrapConstraints:     constraints.MustParse("mem=3.5G"),
			SupportedBootstrapSeries: testing.FakeSupportedJujuSeries,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Arch, gc.Equals, "amd64")
	c.Assert(result.Series, gc.Equals, "bionic")

	c.Assert(len(s.requests), gc.Equals, numExpectedStartInstanceRequests)
	s.vmTags[tags.JujuIsController] = to.StringPtr("true")
	s.assertStartInstanceRequests(c, s.requests[1:], assertStartInstanceRequestsParams{
		availabilitySetName: "juju-controller",
		imageReference:      &xenialImageReference,
		diskSizeGB:          32,
		osProfile:           &s.linuxOsProfile,
		instanceType:        "Standard_D1",
	})
}

func (s *environSuite) TestBootstrapWithInvalidCredential(c *gc.C) {
	defer envtesting.DisableFinishBootstrap()()

	ctx := envtesting.BootstrapContext(c)
	env := prepareForBootstrap(c, ctx, s.provider, &s.sender)

	s.createSenderWithUnauthorisedStatusCode(c)
	s.sender = append(s.sender, s.startInstanceSenders(true)...)
	s.requests = nil

	c.Assert(s.invalidatedCredential, jc.IsFalse)
	_, err := env.Bootstrap(
		ctx, s.callCtx, environs.BootstrapParams{
			ControllerConfig:         testing.FakeControllerConfig(),
			AvailableTools:           makeToolsList("bionic"),
			BootstrapSeries:          "bionic",
			BootstrapConstraints:     constraints.MustParse("mem=3.5G"),
			SupportedBootstrapSeries: testing.FakeSupportedJujuSeries,
		},
	)
	c.Assert(err, gc.NotNil)
	c.Assert(s.invalidatedCredential, jc.IsTrue)

	// Successful bootstrap expects 4 but we expecte to bail out after getting an authorised error.
	c.Assert(len(s.requests), gc.Equals, 1)
}

func (s *environSuite) TestBootstrapInstanceConstraints(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("bootstrap not supported on Windows")
	}
	defer envtesting.DisableFinishBootstrap()()

	ctx := envtesting.BootstrapContext(c)
	env := prepareForBootstrap(c, ctx, s.provider, &s.sender)

	s.sender = append(s.sender, s.resourceSkusSender())
	s.sender = append(s.sender, s.initResourceGroupSenders(resourceGroupName)...)
	s.sender = append(s.sender, s.startInstanceSendersNoSizes()...)
	s.requests = nil
	err := bootstrap.Bootstrap(
		ctx, env, s.callCtx, bootstrap.BootstrapParams{
			ControllerConfig: testing.FakeControllerConfig(),
			AdminSecret:      jujutesting.AdminSecret,
			CAPrivateKey:     testing.CAKey,
			BootstrapSeries:  "bionic",
			BuildAgentTarball: func(build bool, ver *version.Number, _ string) (*sync.BuiltAgent, error) {
				c.Assert(build, jc.IsFalse)
				return &sync.BuiltAgent{Dir: c.MkDir()}, nil
			},
			SupportedBootstrapSeries: testing.FakeSupportedJujuSeries,
		},
	)
	// If we aren't on amd64, this should correctly fail. See also:
	// lp#1638706: environSuite.TestBootstrapInstanceConstraints fails on rare archs and series
	if arch.HostArch() != "amd64" {
		wantErr := fmt.Sprintf("model %q of type %s does not support instances running on %q",
			env.Config().Name(),
			env.Config().Type(),
			arch.HostArch())
		c.Assert(err, gc.ErrorMatches, wantErr)
		c.SucceedNow()
	}
	// amd64 should pass the rest of the test.
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(len(s.requests), gc.Equals, numExpectedStartInstanceRequests)
	s.vmTags[tags.JujuIsController] = to.StringPtr("true")
	s.assertStartInstanceRequests(c, s.requests[1:], assertStartInstanceRequestsParams{
		availabilitySetName: "juju-controller",
		imageReference:      &xenialImageReference,
		diskSizeGB:          32,
		osProfile:           &s.linuxOsProfile,
		needsProviderInit:   true,
		instanceType:        "Standard_D1",
		publicIP:            true,
	})
}

func (s *environSuite) TestBootstrapCustomResourceGroup(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("bootstrap not supported on Windows")
	}
	defer envtesting.DisableFinishBootstrap()()

	ctx := envtesting.BootstrapContext(c)
	env := prepareForBootstrap(c, ctx, s.provider, &s.sender, testing.Attrs{"resource-group-name": "foo"})

	s.sender = append(s.sender, s.resourceSkusSender())
	s.sender = append(s.sender, s.initResourceGroupSenders("foo")...)
	s.sender = append(s.sender, s.startInstanceSendersNoSizes()...)
	s.requests = nil
	err := bootstrap.Bootstrap(
		ctx, env, s.callCtx, bootstrap.BootstrapParams{
			ControllerConfig: testing.FakeControllerConfig(),
			AdminSecret:      jujutesting.AdminSecret,
			CAPrivateKey:     testing.CAKey,
			BootstrapSeries:  "bionic",
			BuildAgentTarball: func(build bool, ver *version.Number, _ string) (*sync.BuiltAgent, error) {
				c.Assert(build, jc.IsFalse)
				return &sync.BuiltAgent{Dir: c.MkDir()}, nil
			},
			SupportedBootstrapSeries: testing.FakeSupportedJujuSeries,
		},
	)
	// If we aren't on amd64, this should correctly fail. See also:
	// lp#1638706: environSuite.TestBootstrapInstanceConstraints fails on rare archs and series
	if arch.HostArch() != "amd64" {
		wantErr := fmt.Sprintf("model %q of type %s does not support instances running on %q",
			env.Config().Name(),
			env.Config().Type(),
			arch.HostArch())
		c.Assert(err, gc.ErrorMatches, wantErr)
		c.SucceedNow()
	}
	// amd64 should pass the rest of the test.
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(len(s.requests), gc.Equals, numExpectedStartInstanceRequests)
	s.vmTags[tags.JujuIsController] = to.StringPtr("true")
	s.assertStartInstanceRequests(c, s.requests[1:], assertStartInstanceRequestsParams{
		availabilitySetName: "juju-controller",
		imageReference:      &xenialImageReference,
		diskSizeGB:          32,
		osProfile:           &s.linuxOsProfile,
		needsProviderInit:   true,
		customResourceGroup: true,
		instanceType:        "Standard_D1",
		publicIP:            true,
	})
}

func (s *environSuite) TestBootstrapWithAutocert(c *gc.C) {
	defer envtesting.DisableFinishBootstrap()()

	ctx := envtesting.BootstrapContext(c)
	env := prepareForBootstrap(c, ctx, s.provider, &s.sender)

	s.sender = s.initResourceGroupSenders(resourceGroupName)
	s.sender = append(s.sender, s.startInstanceSenders(true)...)
	s.requests = nil
	config := testing.FakeControllerConfig()
	config["api-port"] = 443
	config["autocert-dns-name"] = "example.com"
	result, err := env.Bootstrap(
		ctx, s.callCtx, environs.BootstrapParams{
			ControllerConfig:         config,
			AvailableTools:           makeToolsList("bionic"),
			BootstrapSeries:          "bionic",
			BootstrapConstraints:     constraints.MustParse("mem=3.5G"),
			SupportedBootstrapSeries: testing.FakeSupportedJujuSeries,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Arch, gc.Equals, "amd64")
	c.Assert(result.Series, gc.Equals, "bionic")

	c.Assert(len(s.requests), gc.Equals, numExpectedStartInstanceRequests)
	s.vmTags[tags.JujuIsController] = to.StringPtr("true")
	s.assertStartInstanceRequests(c, s.requests[1:], assertStartInstanceRequestsParams{
		autocert:            true,
		availabilitySetName: "juju-controller",
		imageReference:      &xenialImageReference,
		diskSizeGB:          32,
		osProfile:           &s.linuxOsProfile,
		instanceType:        "Standard_D1",
		publicIP:            true,
	})
}

func (s *environSuite) TestAllRunningInstancesResourceGroupNotFound(c *gc.C) {
	env := s.openEnviron(c)
	azure.SetRetries(env)
	sender := mocks.NewSender()
	sender.AppendAndRepeatResponse(mocks.NewResponseWithStatus(
		"resource group not found", http.StatusNotFound,
	), 2)
	s.sender = azuretesting.Senders{sender}
	_, err := env.AllRunningInstances(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environSuite) TestAllRunningInstancesIgnoresCommonDeployment(c *gc.C) {
	env := s.openEnviron(c)

	dependencies := []resources.Dependency{{
		ID: to.StringPtr("whatever"),
	}}
	deployments := []resources.DeploymentExtended{{
		// common deployment should be ignored
		Name: to.StringPtr("common"),
		Properties: &resources.DeploymentPropertiesExtended{
			ProvisioningState: to.StringPtr("Succeeded"),
			Dependencies:      &dependencies,
		},
	}}
	result := resources.DeploymentListResult{Value: &deployments}
	s.sender = azuretesting.Senders{
		makeSender("/deployments", result),
	}

	instances, err := env.AllRunningInstances(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instances, gc.HasLen, 0)
}

func (s *environSuite) TestStopInstancesNotFound(c *gc.C) {
	env := s.openEnviron(c)
	sender0 := mocks.NewSender()
	sender0.AppendAndRepeatResponse(mocks.NewResponseWithStatus(
		"vm not found", http.StatusNotFound,
	), 2)
	sender1 := mocks.NewSender()
	sender1.AppendAndRepeatResponse(mocks.NewResponseWithStatus(
		"vm not found", http.StatusNotFound,
	), 2)
	s.sender = azuretesting.Senders{sender0, sender1}
	err := env.StopInstances(s.callCtx, "a", "b")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environSuite) TestStopInstancesInvalidCredential(c *gc.C) {
	env := s.openEnviron(c)
	s.createSenderWithUnauthorisedStatusCode(c)
	c.Assert(s.invalidatedCredential, jc.IsFalse)
	err := env.StopInstances(s.callCtx, "a", "b")
	c.Assert(err, gc.NotNil)
	c.Assert(s.invalidatedCredential, jc.IsTrue)
	// This call is expected to have made 2 calls. Although we do understand that we could have gotten
	// an invalid credential for one of the instances, the actual stop command to cloud api is done
	// in a separate go routine for each instance. These goroutine do not really communicate with each other.
	// There will be as many routines as there are instances and only once they all complete, will we have a chance to
	// stop proceeding.
	// There is also a retry from go-autorest to count
	c.Assert(s.requests, gc.HasLen, 3)
}

func (s *environSuite) TestStopInstancesResourceGroupNotFound(c *gc.C) {
	// skip storage, so we get to deleting security rules
	s.PatchValue(&s.storageAccountKeys.Keys, nil)
	env := s.openEnviron(c)
	azure.SetRetries(env)

	nsgErr := autorest.NewErrorWithError(errors.New("autorest/azure: Service returned an error."), "network.SecurityGroupsClient", "Get", &http.Response{StatusCode: http.StatusNotFound}, "Failure responding to request")
	nsgSender := s.makeErrorSenderWithContent(c, ".*/networkSecurityGroups/juju-internal-nsg", makeSecurityGroup(), nsgErr, 2)

	s.sender = azuretesting.Senders{
		makeSender("/deployments/machine-0", s.deployment), // Cancel
		s.storageAccountSender(),
		s.storageAccountKeysSender(),
		s.networkInterfacesSender(),                     // GET: no NICs
		s.publicIPAddressesSender(),                     // GET: no public IPs
		makeSender(".*/virtualMachines/machine-0", nil), // DELETE
		makeSender(".*/disks/machine-0", nil),           // DELETE
		nsgSender,                                       // GET with failure
		makeSender(".*/deployments/machine-0", nil),     // DELETE
	}
	err := env.StopInstances(s.callCtx, "machine-0")
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
	nic0IPConfiguration.PublicIPAddress = &network.PublicIPAddress{}
	nic0 := makeNetworkInterface("nic-0", "machine-0", nic0IPConfiguration)

	s.sender = azuretesting.Senders{
		makeSender(".*/deployments/machine-0/cancel", nil), // POST
		s.storageAccountSender(),
		s.storageAccountKeysSender(),
		s.networkInterfacesSender(nic0),
		s.publicIPAddressesSender(makePublicIPAddress("pip-0", "machine-0", "1.2.3.4")),
		makeSender(".*/virtualMachines/machine-0", nil),                                                 // DELETE
		makeSender(".*/networkSecurityGroups/juju-internal-nsg", nsg),                                   // GET
		makeSender(".*/networkSecurityGroups/juju-internal-nsg/securityRules/machine-0-80", nil),        // DELETE
		makeSender(".*/networkSecurityGroups/juju-internal-nsg/securityRules/machine-0-1000-2000", nil), // DELETE
		makeSender(".*/networkInterfaces/nic-0", nil),                                                   // DELETE
		makeSender(".*/publicIPAddresses/pip-0", nil),                                                   // DELETE
		makeSender(".*/deployments/machine-0", nil),                                                     // DELETE
	}

	machine0Blob := azuretesting.MockStorageBlob{Name_: "machine-0"}
	s.osvhdsContainer.Blobs_ = []azurestorage.Blob{&machine0Blob}

	err := env.StopInstances(s.callCtx, "machine-0")
	c.Assert(err, jc.ErrorIsNil)

	s.storageClient.CheckCallNames(c, "NewClient", "GetContainerReference")
	s.storageClient.CheckCall(c, 1, "GetContainerReference", "osvhds")
	s.osvhdsContainer.CheckCallNames(c, "Blob")
	s.osvhdsContainer.CheckCall(c, 0, "Blob", "machine-0")
	machine0Blob.CheckCallNames(c, "DeleteIfExists")
}

func (s *environSuite) TestStopInstancesMultiple(c *gc.C) {
	env := s.openEnviron(c)
	azure.SetRetries(env)

	vmDeleteSender0 := s.makeErrorSender(c, ".*/virtualMachines/machine-[01]", errors.New("blargh"), 2)
	vmDeleteSender1 := s.makeErrorSender(c, ".*/virtualMachines/machine-[01]", errors.New("blargh"), 2)

	s.sender = azuretesting.Senders{
		makeSender(".*/deployments/machine-[01]/cancel", nil), // POST
		makeSender(".*/deployments/machine-[01]/cancel", nil), // POST

		// We should only query the NICs, public IPs, and storage
		// account/keys, regardless of how many instances are deleted.
		s.storageAccountSender(),
		s.storageAccountKeysSender(),
		s.networkInterfacesSender(),
		s.publicIPAddressesSender(),

		vmDeleteSender0,
		vmDeleteSender1,
	}
	err := env.StopInstances(s.callCtx, "machine-0", "machine-1")
	c.Assert(err, gc.ErrorMatches, `deleting instance "machine-[01]":.*blargh`)
}

func (s *environSuite) TestStopInstancesDeploymentNotFound(c *gc.C) {
	env := s.openEnviron(c)

	cancelSender := mocks.NewSender()
	cancelSender.AppendAndRepeatResponse(mocks.NewResponseWithStatus(
		"deployment not found", http.StatusNotFound,
	), 2)
	s.sender = azuretesting.Senders{cancelSender}
	err := env.StopInstances(s.callCtx, "machine-0")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environSuite) TestStopInstancesStorageAccountNoKeys(c *gc.C) {
	s.PatchValue(&s.storageAccountKeys.Keys, nil)
	s.testStopInstancesStorageAccountNotFound(c)
}

func (s *environSuite) TestStopInstancesStorageAccountNoFullKey(c *gc.C) {
	keys := *s.storageAccountKeys.Keys
	s.PatchValue(&keys[0].Permissions, storage.Read)
	s.testStopInstancesStorageAccountNotFound(c)
}

func (s *environSuite) testStopInstancesStorageAccountNotFound(c *gc.C) {
	env := s.openEnviron(c)
	s.sender = azuretesting.Senders{
		makeSender("/deployments/machine-0", s.deployment), // Cancel
		s.storageAccountSender(),
		s.storageAccountKeysSender(),
		s.networkInterfacesSender(),                                                   // GET: no NICs
		s.publicIPAddressesSender(),                                                   // GET: no public IPs
		makeSender(".*/virtualMachines/machine-0", nil),                               // DELETE
		makeSender(".*/disks/machine-0", nil),                                         // DELETE
		makeSender(".*/networkSecurityGroups/juju-internal-nsg", makeSecurityGroup()), // GET: no rules
		makeSender(".*/deployments/machine-0", nil),                                   // DELETE
	}
	err := env.StopInstances(s.callCtx, "machine-0")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environSuite) TestStopInstancesStorageAccountError(c *gc.C) {
	env := s.openEnviron(c)
	azure.SetRetries(env)
	errorSender := s.storageAccountErrorSender(c, errors.New("blargh"), 2)
	s.sender = azuretesting.Senders{
		makeSender("/deployments/machine-0", s.deployment), // Cancel
		errorSender,
	}
	err := env.StopInstances(s.callCtx, "machine-0")
	c.Assert(err, gc.ErrorMatches, "getting storage account:.*blargh")
}

func (s *environSuite) TestStopInstancesStorageAccountKeysError(c *gc.C) {
	env := s.openEnviron(c)
	azure.SetRetries(env)
	errorSender := s.storageAccountKeysErrorSender(c, errors.New("blargh"), 2)
	s.sender = azuretesting.Senders{
		makeSender("/deployments/machine-0", s.deployment), // Cancel
		s.storageAccountSender(),
		errorSender,
	}
	err := env.StopInstances(s.callCtx, "machine-0")
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
		"invalid constraint value: instance-type=t1.micro\nvalid values are: \\[A1 D1 D2 Standard_A1 Standard_D1 Standard_D2\\]",
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
	s.sender = azuretesting.Senders{s.resourceSkusSender()}
	validator, err := env.ConstraintsValidator(context.NewCloudCallContext())
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
		makeSender(".*/resourcegroups/juju-testmodel-"+testing.ModelTag.Id()[:8], nil), // DELETE
	}
	err := env.Destroy(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.requests, gc.HasLen, 1)
	c.Assert(s.requests[0].Method, gc.Equals, "DELETE")
}

func (s *environSuite) TestDestroyHostedModelCustomResourceGroup(c *gc.C) {
	env := s.openEnviron(c,
		testing.Attrs{"controller-uuid": utils.MustNewUUID().String(), "resource-group-name": "foo"})
	s.sender = azuretesting.Senders{
		makeSender("/deployments/purge-resource-group", nil), // PUT
	}
	err := env.Destroy(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.requests, gc.HasLen, 1)
	c.Assert(s.requests[0].Method, gc.Equals, "PUT")
	c.Assert(s.requests[0].URL.Path, gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222/resourcegroups/foo/providers/Microsoft.Resources/deployments/purge-resource-group")
}

func (s *environSuite) TestDestroyHostedModelWithInvalidCredential(c *gc.C) {
	env := s.openEnviron(c, testing.Attrs{"controller-uuid": utils.MustNewUUID().String()})
	s.createSenderWithUnauthorisedStatusCode(c)
	c.Assert(s.invalidatedCredential, jc.IsFalse)
	err := env.Destroy(s.callCtx)
	c.Assert(err, gc.NotNil)
	c.Assert(s.invalidatedCredential, jc.IsTrue)
	c.Assert(s.requests, gc.HasLen, 1)
	c.Assert(s.requests[0].Method, gc.Equals, "DELETE")
}

func (s *environSuite) TestDestroyController(c *gc.C) {
	groups := []resources.Group{{
		Name: to.StringPtr("group1"),
	}, {
		Name: to.StringPtr("group2"),
	}}
	result := resources.GroupListResult{Value: &groups}

	env := s.openEnviron(c)
	s.sender = azuretesting.Senders{
		makeSender(".*/resourcegroups", result),        // GET
		makeSender(".*/resourcegroups/group[12]", nil), // DELETE
		makeSender(".*/resourcegroups/group[12]", nil), // DELETE
	}
	err := env.DestroyController(s.callCtx, s.controllerUUID)
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

func (s *environSuite) TestDestroyControllerWithInvalidCredential(c *gc.C) {
	env := s.openEnviron(c)
	s.createSenderWithUnauthorisedStatusCode(c)

	c.Assert(s.invalidatedCredential, jc.IsFalse)
	err := env.DestroyController(s.callCtx, s.controllerUUID)
	c.Assert(err, gc.NotNil)
	c.Assert(s.invalidatedCredential, jc.IsTrue)

	c.Assert(s.requests, gc.HasLen, 1)
	c.Assert(s.requests[0].Method, gc.Equals, "GET")
	c.Assert(s.requests[0].URL.Query().Get("$filter"), gc.Equals, fmt.Sprintf(
		"tagname eq 'juju-controller-uuid' and tagvalue eq '%s'",
		testing.ControllerTag.Id(),
	))
}

func (s *environSuite) TestDestroyControllerErrors(c *gc.C) {
	groups := []resources.Group{
		{Name: to.StringPtr("group1")},
		{Name: to.StringPtr("group2")},
	}
	result := resources.GroupListResult{Value: &groups}

	makeErrorSender := func(err string) *azuretesting.MockSender {
		errorSender := s.makeErrorSender(c, ".*/resourcegroups/group[12].*", errors.New(err), 2)
		return errorSender
	}

	env := s.openEnviron(c)
	s.requests = nil
	s.sender = azuretesting.Senders{
		makeSender(".*/resourcegroups", result), // GET
		makeErrorSender("foo"),                  // DELETE
		makeErrorSender("bar"),                  // DELETE
	}
	destroyErr := env.DestroyController(s.callCtx, s.controllerUUID)
	// checked below, once we know the order of deletions.

	c.Assert(s.requests, gc.HasLen, 5)
	c.Assert(s.requests[0].Method, gc.Equals, "GET")
	c.Assert(s.requests[1].Method, gc.Equals, "DELETE")
	c.Assert(s.requests[2].Method, gc.Equals, "DELETE") // retry
	c.Assert(s.requests[3].Method, gc.Equals, "DELETE")
	c.Assert(s.requests[4].Method, gc.Equals, "DELETE") // retry

	// Groups are deleted concurrently, so there's no known order.
	groupsDeleted := []string{
		path.Base(s.requests[1].URL.Path),
		path.Base(s.requests[2].URL.Path),
		path.Base(s.requests[3].URL.Path),
		path.Base(s.requests[4].URL.Path),
	}
	c.Assert(groupsDeleted, jc.SameContents, []string{"group1", "group1", "group2", "group2"})

	c.Check(destroyErr, gc.ErrorMatches,
		`deleting resource group "group1":.*; `+
			`deleting resource group "group2":.*`)
	c.Check(destroyErr, gc.ErrorMatches, ".*(foo|bar).*")
}

func (s *environSuite) TestInstanceInformation(c *gc.C) {
	env := s.openEnviron(c)
	s.sender = s.startInstanceSenders(false)
	types, err := env.InstanceTypes(s.callCtx, constraints.Value{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(types.InstanceTypes, gc.HasLen, 6)

	cons := constraints.MustParse("mem=4G")
	types, err = env.InstanceTypes(s.callCtx, cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(types.InstanceTypes, gc.HasLen, 2)
}

func (s *environSuite) TestInstanceInformationWithInvalidCredential(c *gc.C) {
	env := s.openEnviron(c)
	s.createSenderWithUnauthorisedStatusCode(c)

	c.Assert(s.invalidatedCredential, jc.IsFalse)
	_, err := env.InstanceTypes(s.callCtx, constraints.Value{})
	c.Assert(err, gc.NotNil)
	c.Assert(s.invalidatedCredential, jc.IsTrue)
}

func (s *environSuite) TestAdoptResources(c *gc.C) {
	providersResult := makeProvidersResult()
	resourcesResult := makeResourcesResult()

	res1 := (*resourcesResult.Value)[0]
	res1.Properties = &map[string]interface{}{"has-properties": true}

	res2 := (*resourcesResult.Value)[1]
	res2.Properties = &map[string]interface{}{"has-properties": true}

	env := s.openEnviron(c)

	s.sender = azuretesting.Senders{
		makeSender(".*/resourcegroups/juju-testmodel-.*", makeResourceGroupResult()),
		makeSender(".*/resourcegroups/juju-testmodel-.*", nil),

		makeSender(".*/providers", providersResult),
		makeSender(".*/resourceGroups/juju-testmodel-.*/resources", resourcesResult),

		// First request is the get, second is the update. (The
		// lowercase resourcegroups here is a quirk of the
		// autogenerated Go SDK.)
		makeSender(".*/resourcegroups/.*/providers/Beck.Replica/liars/scissor/boxing-day-blues", res1),
		makeSender(".*/resourcegroups/.*/providers/Beck.Replica/liars/scissor/boxing-day-blues", res1),

		makeSender(".*/resourcegroups/.*/providers/Tuneyards.Bizness/micachu/drop-dead", res2),
		makeSender(".*/resourcegroups/.*/providers/Tuneyards.Bizness/micachu/drop-dead", res2),
	}

	err := env.AdoptResources(s.callCtx, "new-controller", version.MustParse("1.2.4"))
	c.Assert(err, jc.ErrorIsNil)

	// Check that properties and tags are preserved and the correct
	// API version is sent.
	checkAPIVersion := func(ix uint, expectedMethod, expectedVersion string) {
		req := s.requests[ix]
		c.Check(req.Method, gc.Equals, expectedMethod)
		c.Check(req.URL.Query().Get("api-version"), gc.Equals, expectedVersion)
	}
	// Resource group get and update.
	checkAPIVersion(0, "GET", "2018-05-01")
	checkAPIVersion(1, "PUT", "2018-05-01")
	// Resources.
	checkAPIVersion(4, "GET", "2018-05-01")
	checkAPIVersion(5, "PUT", "2018-05-01")
	checkAPIVersion(6, "GET", "2018-05-01")
	checkAPIVersion(7, "PUT", "2018-05-01")

	checkTagsAndProperties := func(ix uint) {
		req := s.requests[ix]
		data := make([]byte, req.ContentLength)
		_, err := req.Body.Read(data)
		c.Assert(err, jc.ErrorIsNil)

		var resource resources.GenericResource
		err = json.Unmarshal(data, &resource)
		c.Assert(err, jc.ErrorIsNil)

		rTags := to.StringMap(resource.Tags)
		c.Check(rTags["something else"], gc.Equals, "good")
		c.Check(rTags[tags.JujuController], gc.Equals, "new-controller")
		c.Check(resource.Properties, gc.DeepEquals, map[string]interface{}{"has-properties": true})
	}
	checkTagsAndProperties(5)
	checkTagsAndProperties(7)

	// Also check the tags are right for the resource group.
	req := s.requests[1] // the resource group update.
	data := make([]byte, req.ContentLength)
	_, err = req.Body.Read(data)
	c.Assert(err, jc.ErrorIsNil)
	var group resources.Group
	err = json.Unmarshal(data, &group)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the provisioning state wasn't sent back.
	c.Check((*group.Properties).ProvisioningState, gc.IsNil)

	gTags := to.StringMap(group.Tags)
	c.Check(gTags["something else"], gc.Equals, "good")
	c.Check(gTags[tags.JujuController], gc.Equals, "new-controller")
	c.Check(gTags[tags.JujuModel], gc.Equals, "deadbeef-0bad-400d-8000-4b1d0d06f00d")
}

func makeProvidersResult() resources.ProviderListResult {
	providers := []resources.Provider{{
		Namespace: to.StringPtr("Beck.Replica"),
		ResourceTypes: &[]resources.ProviderResourceType{{
			ResourceType: to.StringPtr("battles/ladida"),
			APIVersions:  &[]string{"2016-12-15", "2014-02-02"},
		}, {
			ResourceType: to.StringPtr("liars/scissor"),
			APIVersions:  &[]string{"2018-05-01", "2015-03-02"},
		}},
	}, {
		Namespace: to.StringPtr("Tuneyards.Bizness"),
		ResourceTypes: &[]resources.ProviderResourceType{{
			ResourceType: to.StringPtr("slaves/debbie"),
			APIVersions:  &[]string{"2016-12-14", "2014-04-02"},
		}, {
			ResourceType: to.StringPtr("micachu"),
			APIVersions:  &[]string{"2018-05-01", "2015-05-02"},
		}},
	}}
	return resources.ProviderListResult{Value: &providers}
}

func makeResourcesResult() resources.ListResult {
	theResources := []resources.GenericResourceExpanded{{
		ID:       to.StringPtr("/subscriptions/foo/resourcegroups/bar/providers/Beck.Replica/liars/scissor/boxing-day-blues"),
		Name:     to.StringPtr("boxing-day-blues"),
		Type:     to.StringPtr("Beck.Replica/liars/scissor"),
		Location: to.StringPtr("westus"),
		Tags: map[string]*string{
			tags.JujuController: to.StringPtr("old-controller"),
			"something else":    to.StringPtr("good"),
		},
	}, {
		ID:       to.StringPtr("/subscriptions/foo/resourcegroups/bar/providers/Tuneyards.Bizness/micachu/drop-dead"),
		Name:     to.StringPtr("drop-dead"),
		Type:     to.StringPtr("Tuneyards.Bizness/micachu"),
		Location: to.StringPtr("westus"),
		Tags: map[string]*string{
			tags.JujuController: to.StringPtr("old-controller"),
			"something else":    to.StringPtr("good"),
		},
	}}
	return resources.ListResult{Value: &theResources}
}

func makeResourceGroupResult() resources.Group {
	return resources.Group{
		Name:     to.StringPtr("charles"),
		Location: to.StringPtr("westus"),
		Properties: &resources.GroupProperties{
			ProvisioningState: to.StringPtr("very yes"),
		},
		Tags: map[string]*string{
			tags.JujuController: to.StringPtr("old-controller"),
			tags.JujuModel:      to.StringPtr("deadbeef-0bad-400d-8000-4b1d0d06f00d"),
			"something else":    to.StringPtr("good"),
		},
	}
}

func (s *environSuite) TestAdoptResourcesErrorGettingGroup(c *gc.C) {
	env := s.openEnviron(c)
	sender := s.makeErrorSender(
		c,
		".*/resourcegroups/juju-testmodel-.*",
		errors.New("uhoh"),
		4)
	s.sender = azuretesting.Senders{sender}

	err := env.AdoptResources(s.callCtx, "new-controller", version.MustParse("1.0.0"))
	c.Assert(err, gc.ErrorMatches, ".*uhoh$")
	c.Assert(s.requests, gc.HasLen, 2)
}

func (s *environSuite) TestAdoptResourcesErrorUpdatingGroup(c *gc.C) {
	env := s.openEnviron(c)
	errorSender := s.makeErrorSender(
		c,
		".*/resourcegroups/juju-testmodel-.*",
		errors.New("uhoh"),
		2)
	s.sender = azuretesting.Senders{
		makeSender(".*/resourcegroups/juju-testmodel-.*", makeResourceGroupResult()),
		errorSender,
	}

	err := env.AdoptResources(s.callCtx, "new-controller", version.MustParse("1.0.0"))
	c.Assert(err, gc.ErrorMatches, ".*uhoh$")
	c.Assert(s.requests, gc.HasLen, 3)
}

func (s *environSuite) TestAdoptResourcesErrorGettingVersions(c *gc.C) {
	env := s.openEnviron(c)
	errorSender := s.makeErrorSender(
		c,
		".*/providers",
		errors.New("uhoh"),
		2)
	s.sender = azuretesting.Senders{
		makeSender(".*/resourcegroups/juju-testmodel-.*", makeResourceGroupResult()),
		makeSender(".*/resourcegroups/juju-testmodel-.*", nil),
		errorSender,
	}

	err := env.AdoptResources(s.callCtx, "new-controller", version.MustParse("1.0.0"))
	c.Assert(err, gc.ErrorMatches, ".*uhoh$")
	c.Assert(s.requests, gc.HasLen, 4)
}

func (s *environSuite) TestAdoptResourcesErrorListingResources(c *gc.C) {
	env := s.openEnviron(c)
	errorSender := s.makeErrorSender(
		c,
		".*/resourceGroups/juju-testmodel-.*/resources",
		errors.New("ouch!"),
		2)
	s.sender = azuretesting.Senders{
		makeSender(".*/resourcegroups/juju-testmodel-.*", makeResourceGroupResult()),
		makeSender(".*/resourcegroups/juju-testmodel-.*", nil),
		makeSender(".*/providers", resources.ProviderListResult{}),
		errorSender,
	}

	err := env.AdoptResources(s.callCtx, "new-controller", version.MustParse("1.0.0"))
	c.Assert(err, gc.ErrorMatches, ".*ouch!$")
	c.Assert(s.requests, gc.HasLen, 5)
}

func (s *environSuite) TestAdoptResourcesWithInvalidCredential(c *gc.C) {
	env := s.openEnviron(c)
	s.createSenderWithUnauthorisedStatusCode(c)

	c.Assert(s.invalidatedCredential, jc.IsFalse)
	err := env.AdoptResources(s.callCtx, "new-controller", version.MustParse("1.0.0"))
	c.Assert(err, gc.NotNil)
	c.Assert(s.invalidatedCredential, jc.IsTrue)
}

func (s *environSuite) TestAdoptResourcesNoUpdateNeeded(c *gc.C) {
	providersResult := makeProvidersResult()
	resourcesResult := makeResourcesResult()

	// Give the first resource the right controller tag so it doesn't need updating.
	res1 := (*resourcesResult.Value)[0]
	res1.Tags[tags.JujuController] = to.StringPtr("new-controller")
	res2 := (*resourcesResult.Value)[1]

	env := s.openEnviron(c)

	s.sender = azuretesting.Senders{
		makeSender(".*/resourcegroups/juju-testmodel-.*", makeResourceGroupResult()),
		makeSender(".*/resourcegroups/juju-testmodel-.*", nil),
		makeSender(".*/providers", providersResult),
		makeSender(".*/resourceGroups/juju-testmodel-.*/resources", resourcesResult),

		// Doesn't bother updating res1, continues to do res2.
		makeSender(".*/resourcegroups/.*/providers/Tuneyards.Bizness/micachu/drop-dead", res2),
		makeSender(".*/resourcegroups/.*/providers/Tuneyards.Bizness/micachu/drop-dead", res2),
	}

	err := env.AdoptResources(s.callCtx, "new-controller", version.MustParse("1.2.4"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(s.requests, gc.HasLen, 6)
}

func (s *environSuite) TestAdoptResourcesErrorGettingFullResource(c *gc.C) {
	providersResult := makeProvidersResult()
	resourcesResult := makeResourcesResult()

	res2 := (*resourcesResult.Value)[1]

	env := s.openEnviron(c)

	errorSender := s.makeErrorSender(
		c,
		".*/resourcegroups/.*/providers/Beck.Replica/liars/scissor/boxing-day-blues",
		errors.New("flagrant error! virus=very yes"),
		2)

	s.sender = azuretesting.Senders{
		makeSender(".*/resourcegroups/juju-testmodel-.*", makeResourceGroupResult()),
		makeSender(".*/resourcegroups/juju-testmodel-.*", nil),
		makeSender(".*/providers", providersResult),
		makeSender(".*/resourceGroups/juju-testmodel-.*/resources", resourcesResult),

		// The first resource yields an error but the update continues.
		errorSender,

		makeSender(".*/resourcegroups/.*/providers/Tuneyards.Bizness/micachu/drop-dead", res2),
		makeSender(".*/resourcegroups/.*/providers/Tuneyards.Bizness/micachu/drop-dead", res2),
	}

	err := env.AdoptResources(s.callCtx, "new-controller", version.MustParse("1.2.4"))
	c.Check(err, gc.ErrorMatches, `failed to update controller for some resources: \[boxing-day-blues\]`)
	c.Check(s.requests, gc.HasLen, 8)
}

func (s *environSuite) TestAdoptResourcesErrorUpdating(c *gc.C) {
	providersResult := makeProvidersResult()
	resourcesResult := makeResourcesResult()

	res1 := (*resourcesResult.Value)[0]
	res2 := (*resourcesResult.Value)[1]

	env := s.openEnviron(c)

	errorSender := s.makeErrorSender(
		c,
		".*/resourcegroups/.*/providers/Beck.Replica/liars/scissor/boxing-day-blues",
		errors.New("oopsie"),
		2)

	s.sender = azuretesting.Senders{
		makeSender(".*/resourcegroups/juju-testmodel-.*", makeResourceGroupResult()),
		makeSender(".*/resourcegroups/juju-testmodel-.*", nil),
		makeSender(".*/providers", providersResult),
		makeSender(".*/resourceGroups/juju-testmodel-.*/resources", resourcesResult),

		// Updating the first resource yields an error but the update continues.
		makeSender(".*/resourcegroups/.*/providers/Beck.Replica/liars/scissor/boxing-day-blues", res1),
		errorSender,

		makeSender(".*/resourcegroups/.*/providers/Tuneyards.Bizness/micachu/drop-dead", res2),
		makeSender(".*/resourcegroups/.*/providers/Tuneyards.Bizness/micachu/drop-dead", res2),
	}

	err := env.AdoptResources(s.callCtx, "new-controller", version.MustParse("1.2.4"))
	c.Check(err, gc.ErrorMatches, `failed to update controller for some resources: \[boxing-day-blues\]`)
	c.Check(s.requests, gc.HasLen, 9)
}
