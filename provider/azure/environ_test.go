// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"bytes"
	stdcontext "context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/juju/clock/testclock"
	"github.com/juju/names/v5"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/controller"
	corearch "github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/sync"
	"github.com/juju/juju/environs/tags"
	envtesting "github.com/juju/juju/environs/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/azure"
	"github.com/juju/juju/provider/azure/internal/armtemplates"
	"github.com/juju/juju/provider/azure/internal/azuretesting"
	"github.com/juju/juju/provider/azure/internal/errorutils"
	jujustorage "github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
)

var (
	jammyImageReferenceGen2 = armcompute.ImageReference{
		Publisher: to.Ptr("Canonical"),
		Offer:     to.Ptr("0001-com-ubuntu-server-jammy"),
		SKU:       to.Ptr("22_04-lts-gen2"),
		Version:   to.Ptr("latest"),
	}
	jammyImageReferenceArm64 = armcompute.ImageReference{
		Publisher: to.Ptr("Canonical"),
		Offer:     to.Ptr("0001-com-ubuntu-server-jammy"),
		SKU:       to.Ptr("22_04-lts-arm64"),
		Version:   to.Ptr("latest"),
	}
	centos7ImageReference = armcompute.ImageReference{
		Publisher: to.Ptr("OpenLogic"),
		Offer:     to.Ptr("CentOS"),
		SKU:       to.Ptr("7.3"),
		Version:   to.Ptr("latest"),
	}
)

func toValue[T any](v *T) T {
	if v == nil {
		return *new(T)
	}
	return *v
}

func toMapPtr(in map[string]string) map[string]*string {
	result := make(map[string]*string)
	for k, v := range in {
		result[k] = to.Ptr(v)
	}
	return result
}

type keyBundle struct {
	Key *jsonWebKey `json:"key"`
}

type jsonWebKey struct {
	Kid *string `json:"kid"`
	Kty string  `json:"kty"`
}

type environSuite struct {
	testing.BaseSuite

	provider   environs.EnvironProvider
	requests   []*http.Request
	sender     azuretesting.Senders
	retryClock mockClock

	controllerUUID   string
	envTags          map[string]string
	vmTags           map[string]string
	group            *armresources.ResourceGroup
	skus             []*armcompute.ResourceSKU
	ubuntuServerSKUs []armcompute.VirtualMachineImageResource
	commonDeployment *armresources.DeploymentExtended
	deployment       *armresources.Deployment
	sshPublicKeys    []*armcompute.SSHPublicKey
	linuxOsProfile   armcompute.OSProfile

	callCtx               *context.CloudCallContext
	invalidatedCredential bool
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.sender = nil
	s.requests = nil
	s.retryClock = mockClock{Clock: testclock.NewClock(time.Time{})}

	s.provider = newProvider(c, azure.ProviderConfig{
		Sender:           azuretesting.NewSerialSender(&s.sender),
		RequestInspector: &azuretesting.RequestRecorderPolicy{Requests: &s.requests},
		RetryClock: &testclock.AutoAdvancingClock{
			&s.retryClock, s.retryClock.Advance,
		},
		CreateTokenCredential: func(appId, appPassword, tenantID string, opts azcore.ClientOptions) (azcore.TokenCredential, error) {
			return &azuretesting.FakeCredential{}, nil
		},
	})

	s.controllerUUID = testing.ControllerTag.Id()
	s.envTags = map[string]string{
		"juju-model-uuid":      testing.ModelTag.Id(),
		"juju-controller-uuid": s.controllerUUID,
	}
	s.vmTags = map[string]string{
		"juju-model-uuid":      testing.ModelTag.Id(),
		"juju-controller-uuid": s.controllerUUID,
		"juju-machine-name":    "juju-06f00d-0",
	}

	s.group = &armresources.ResourceGroup{
		Location: to.Ptr("westus"),
		Tags:     toMapPtr(s.envTags),
		Properties: &armresources.ResourceGroupProperties{
			ProvisioningState: to.Ptr("Succeeded"),
		},
	}

	resourceSKUs := []*armcompute.ResourceSKU{{
		Name:         to.Ptr("Standard_A1"),
		Locations:    to.SliceOfPtrs("westus"),
		ResourceType: to.Ptr("virtualMachines"),
		Capabilities: []*armcompute.ResourceSKUCapabilities{{
			Name:  to.Ptr("MemoryGB"),
			Value: to.Ptr("1.75"),
		}, {
			Name:  to.Ptr("vCPUs"),
			Value: to.Ptr("1"),
		}, {
			Name:  to.Ptr("OSVhdSizeMB"),
			Value: to.Ptr("1047552"),
		}},
	}, {
		Name:         to.Ptr("Standard_D1"),
		Locations:    to.SliceOfPtrs("westus"),
		ResourceType: to.Ptr("virtualMachines"),
		Capabilities: []*armcompute.ResourceSKUCapabilities{{
			Name:  to.Ptr("MemoryGB"),
			Value: to.Ptr("3.5"),
		}, {
			Name:  to.Ptr("vCPUs"),
			Value: to.Ptr("1"),
		}, {
			Name:  to.Ptr("OSVhdSizeMB"),
			Value: to.Ptr("1047552"),
		}},
	}, {
		Name:         to.Ptr("Standard_D2"),
		Locations:    to.SliceOfPtrs("westus"),
		ResourceType: to.Ptr("virtualMachines"),
		Capabilities: []*armcompute.ResourceSKUCapabilities{{
			Name:  to.Ptr("MemoryGB"),
			Value: to.Ptr("7"),
		}, {
			Name:  to.Ptr("vCPUs"),
			Value: to.Ptr("2"),
		}, {
			Name:  to.Ptr("OSVhdSizeMB"),
			Value: to.Ptr("1047552"),
		}},
	}, {
		Name:         to.Ptr("Standard_D666"),
		Locations:    to.SliceOfPtrs("westus"),
		ResourceType: to.Ptr("virtualMachines"),
		Restrictions: []*armcompute.ResourceSKURestrictions{{
			ReasonCode: to.Ptr(armcompute.ResourceSKURestrictionsReasonCodeNotAvailableForSubscription),
		}},
		Capabilities: []*armcompute.ResourceSKUCapabilities{{
			Name:  to.Ptr("MemoryGB"),
			Value: to.Ptr("7"),
		}, {
			Name:  to.Ptr("vCPUs"),
			Value: to.Ptr("2"),
		}, {
			Name:  to.Ptr("OSVhdSizeMB"),
			Value: to.Ptr("1047552"),
		}},
	}, {
		Name:         to.Ptr("Standard_D8ps_v5"),
		Locations:    to.SliceOfPtrs("westus"),
		ResourceType: to.Ptr("virtualMachines"),
		Capabilities: []*armcompute.ResourceSKUCapabilities{{
			Name:  to.Ptr("MemoryGB"),
			Value: to.Ptr("32"),
		}, {
			Name:  to.Ptr("vCPUs"),
			Value: to.Ptr("8"),
		}, {
			Name:  to.Ptr("OSVhdSizeMB"),
			Value: to.Ptr("1047552"),
		}},
		Family: to.Ptr("standardDPSv5Family"),
	}}
	s.skus = resourceSKUs

	s.ubuntuServerSKUs = []armcompute.VirtualMachineImageResource{
		{Name: to.Ptr("12.04-lts")},
		{Name: to.Ptr("12.10")},
		{Name: to.Ptr("14.04-lts")},
		{Name: to.Ptr("15.04")},
		{Name: to.Ptr("15.10")},
		{Name: to.Ptr("16.04-lts")},
		{Name: to.Ptr("18.04-lts")},
		{Name: to.Ptr("20_04-lts")},
		{Name: to.Ptr("22_04-lts")},
		{Name: to.Ptr("22_04-lts-gen2")},
		{Name: to.Ptr("22_04-lts-arm64")},
	}

	s.commonDeployment = &armresources.DeploymentExtended{
		Properties: &armresources.DeploymentPropertiesExtended{
			ProvisioningState: to.Ptr(armresources.ProvisioningStateSucceeded),
		},
		Tags: map[string]*string{
			"juju-model-uuid": to.Ptr(testing.ModelTag.Id()),
		},
	}

	s.deployment = nil

	s.sshPublicKeys = []*armcompute.SSHPublicKey{{
		Path:    to.Ptr("/home/ubuntu/.ssh/authorized_keys"),
		KeyData: to.Ptr(testing.FakeAuthKeys),
	}}
	s.linuxOsProfile = armcompute.OSProfile{
		ComputerName:  to.Ptr("juju-06f00d-0"),
		CustomData:    to.Ptr("<juju-goes-here>"),
		AdminUsername: to.Ptr("ubuntu"),
		LinuxConfiguration: &armcompute.LinuxConfiguration{
			DisablePasswordAuthentication: to.Ptr(true),
			SSH: &armcompute.SSHConfiguration{
				PublicKeys: s.sshPublicKeys,
			},
		},
	}

	s.callCtx = &context.CloudCallContext{
		Context: stdcontext.TODO(),
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
		discoverAuthSender(),
		makeResourceGroupNotFoundSender(fmt.Sprintf(".*/resourcegroups/juju-%s-model-deadbeef-.*", cfg.Name())),
	}
	env, err := environs.Open(stdcontext.TODO(), provider, environs.OpenParams{
		Cloud:  fakeCloudSpec(),
		Config: cfg,
	})
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
		discoverAuthSender(),
		makeResourceGroupNotFoundSender(".*/resourcegroups/juju-testmodel-model-deadbeef-.*"),
		makeSender(".*/resourcegroups/juju-testmodel-.*", makeResourceGroupResult()),
	}
	env, err := environs.Open(stdcontext.TODO(), provider, environs.OpenParams{
		Cloud:  fakeCloudSpec(),
		Config: cfg,
	})
	c.Assert(err, jc.ErrorIsNil)

	*sender = azuretesting.Senders{}
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

func discoverAuthSender() *azuretesting.MockSender {
	sender := &azuretesting.MockSender{
		PathPattern: ".*/subscriptions/(" + fakeSubscriptionId + "|" + fakeManagedSubscriptionId + ")",
	}
	resp := azuretesting.NewResponseWithStatus("", http.StatusUnauthorized)
	azuretesting.SetResponseHeaderValues(resp, "WWW-Authenticate", []string{
		fmt.Sprintf(
			`authorization_uri="https://testing.invalid/%s"`,
			fakeTenantId,
		),
	})
	sender.AppendResponse(resp)
	return sender
}

func (s *environSuite) initResourceGroupSenders(resourceGroupName string) azuretesting.Senders {
	senders := azuretesting.Senders{makeSender(".*/resourcegroups/"+resourceGroupName, s.group)}
	return senders
}

type startInstanceSenderParams struct {
	bootstrap               bool
	controller              bool
	subnets                 []*armnetwork.Subnet
	diskEncryptionSetName   string
	vaultName               string
	vaultKeyName            string
	existingNetwork         string
	withQuotaRetry          bool
	withHypervisorGenRetry  bool
	withConflictRetry       bool
	existingAvailabilitySet bool
	existingCommon          bool
	hasSpaceConstraints     bool
}

func (s *environSuite) startInstanceSenders(c *gc.C, args startInstanceSenderParams) azuretesting.Senders {
	senders := azuretesting.Senders{}
	if args.existingAvailabilitySet {
		senders = append(senders, makeSender("/availabilitySets/mysql", &armcompute.AvailabilitySet{}))
	} else {
		senders = append(senders, s.resourceSKUsSender())
		if s.ubuntuServerSKUs != nil {
			senders = append(senders, makeSender(".*/Canonical/.*/0001-com-ubuntu-server-jammy/skus", s.ubuntuServerSKUs))
		}
	}

	if !args.bootstrap {
		// When starting an instance, we must wait for the common
		// deployment to complete.
		if !args.existingAvailabilitySet && !args.existingCommon {
			senders = append(senders, makeSender("/deployments/common", s.commonDeployment))
		}

		if args.vaultName != "" {
			senders = append(senders, makeSender("/diskEncryptionSets/"+args.diskEncryptionSetName, &armcompute.DiskEncryptionSet{
				Identity: &armcompute.EncryptionSetIdentity{
					PrincipalID: to.Ptr("foo"),
					TenantID:    to.Ptr(fakeTenantId),
				},
			}))
			vaultName := args.vaultName + "-deadbeef"
			deletedVaultSender := azuretesting.MockSender{}
			deletedVaultSender.PathPattern = ".*/locations/westus/deletedVaults/" + vaultName
			deletedVaultSender.AppendAndRepeatResponse(azuretesting.NewResponseWithStatus(
				"vault not found", http.StatusNotFound,
			), 1)
			senders = append(senders, &deletedVaultSender)
			senders = append(senders, makeSender("/vaults/"+vaultName, &armkeyvault.Vault{
				ID:   to.Ptr("vault-id"),
				Name: to.Ptr(vaultName),
				Properties: &armkeyvault.VaultProperties{
					VaultURI: to.Ptr("https://vault-uri"),
				},
			}))
			senders = append(senders, makeSender(fmt.Sprintf("keys/%s/create", args.vaultKeyName), &keyBundle{
				Key: &jsonWebKey{Kid: to.Ptr("https://key-url")},
			}))
		}
	}

	if !args.hasSpaceConstraints && (!args.bootstrap || args.existingNetwork != "") {
		vnetName := "juju-internal-network"
		if args.bootstrap {
			vnetName = args.existingNetwork
			senders = append(senders, makeSender("/deployments/common", s.commonDeployment))
		}
		if len(args.subnets) == 0 {
			subnetName := "juju-internal-subnet"
			if args.bootstrap || args.controller {
				subnetName = "juju-controller-subnet"
			}
			args.subnets = []*armnetwork.Subnet{{
				ID:   to.Ptr(fmt.Sprintf("/virtualNetworks/%s/subnet/%s", vnetName, subnetName)),
				Name: to.Ptr(subnetName),
				Properties: &armnetwork.SubnetPropertiesFormat{
					AddressPrefix: to.Ptr("192.168.0.0/20"),
				},
			}}
		}
		senders = append(senders, makeSender(fmt.Sprintf("/virtualNetworks/%s/subnets", vnetName), armnetwork.SubnetListResult{
			Value: args.subnets,
		}))
	}
	if args.withQuotaRetry {
		quotaErr := newAzureResponseError(c, http.StatusBadRequest, "QuotaExceeded", "")
		senders = append(senders, s.makeErrorSender("/deployments/juju-06f00d-0", quotaErr, 1))
		return senders
	}
	if args.withHypervisorGenRetry {
		requestError := errorutils.RequestError{
			ServiceError: &errorutils.ServiceError{
				Code:    "BadRequest",
				Message: "The selected VM size 'Standard_D2_v2' cannot boot Hypervisor Generation '2'. If this was a Create operation please check that the Hypervisor Generation of the Image matches the Hypervisor Generation of the selected VM Size. If this was an Update operation please select a Hypervisor Generation '2' VM Size. For more information, see https://aka.ms/azuregen2vm",
			},
		}
		rErr, err := json.Marshal(requestError)
		c.Assert(err, jc.ErrorIsNil)
		hypervisorGenErr := newAzureResponseError(c, http.StatusBadRequest,
			"DeploymentFailed", string(rErr),
		)
		senders = append(senders, s.makeErrorSender("/deployments/juju-06f00d-0", hypervisorGenErr, 1))
		return senders
	}
	if args.withConflictRetry {
		conflictErr := newAzureResponseError(c, http.StatusConflict, "Conflict", "")
		senders = append(senders, s.makeErrorSender("/deployments/juju-06f00d-0", conflictErr, 1))
		return senders
	}
	senders = append(senders, makeSender("/deployments/juju-06f00d-0", s.deployment))
	return senders
}

func (s *environSuite) startInstanceSendersNoSizes() azuretesting.Senders {
	senders := azuretesting.Senders{}
	if s.ubuntuServerSKUs != nil {
		senders = append(senders, makeSender(".*/Canonical/.*/0001-com-ubuntu-server-jammy/skus", s.ubuntuServerSKUs))
	}
	senders = append(senders, makeSender("/deployments/juju-06f00d-0", s.deployment))
	return senders
}

func (s *environSuite) networkInterfacesSender(nics ...*armnetwork.Interface) *azuretesting.MockSender {
	return makeSender(".*/networkInterfaces", armnetwork.InterfaceListResult{Value: nics})
}

func (s *environSuite) publicIPAddressesSender(pips ...*armnetwork.PublicIPAddress) *azuretesting.MockSender {
	return makeSender(".*/publicIPAddresses", armnetwork.PublicIPAddressListResult{Value: pips})
}

func (s *environSuite) resourceSKUsSender() *azuretesting.MockSender {
	return makeSender(".*/skus", armcompute.ResourceSKUsResult{Value: s.skus})
}

func makeResourceGroupNotFoundSender(pattern string) *azuretesting.MockSender {
	sender := azuretesting.MockSender{}
	sender.PathPattern = pattern
	sender.AppendAndRepeatResponse(azuretesting.NewResponseWithStatus(
		"resource group not found", http.StatusNotFound,
	), 1)
	return &sender
}

func makeSender(pattern string, v interface{}) *azuretesting.MockSender {
	sender := azuretesting.NewSenderWithValue(v)
	sender.PathPattern = pattern
	return sender
}

func makeSenderWithStatus(pattern string, statusCode int) *azuretesting.MockSender {
	sender := azuretesting.MockSender{}
	sender.PathPattern = pattern
	sender.AppendResponse(azuretesting.NewResponseWithStatus("", statusCode))
	return &sender
}

func newAzureResponseError(c *gc.C, code int, status, message string) error {
	header := make(http.Header)
	header.Set("Content-Type", "application/json")
	requestError := errorutils.RequestError{
		ServiceError: &errorutils.ServiceError{
			Code: "DeployError",
			Details: []errorutils.ServiceErrorDetail{
				{Code: status, Message: message},
			},
		},
	}
	body, err := json.Marshal(requestError)
	c.Assert(err, jc.ErrorIsNil)
	return &azcore.ResponseError{
		ErrorCode:  status,
		StatusCode: code,
		RawResponse: &http.Response{
			Request: &http.Request{
				URL: &url.URL{},
			},
			Header:     header,
			StatusCode: code,
			Body:       io.NopCloser(bytes.NewBuffer(body)),
		},
	}
}

func (s *environSuite) makeErrorSender(pattern string, err error, repeat int) *azuretesting.MockSender {
	sender := &azuretesting.MockSender{}
	sender.PathPattern = pattern
	sender.SetAndRepeatError(err, repeat)
	return sender
}

func makeStartInstanceParams(c *gc.C, controllerUUID string, base corebase.Base) environs.StartInstanceParams {
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
		base, apiInfo,
	)
	c.Assert(err, jc.ErrorIsNil)
	icfg.ControllerConfig = controller.Config{}
	icfg.Tags = map[string]string{
		tags.JujuModel:      testing.ModelTag.Id(),
		tags.JujuController: controllerUUID,
	}

	return environs.StartInstanceParams{
		ControllerUUID: controllerUUID,
		Tools:          makeToolsList(base.OS),
		InstanceConfig: icfg,
	}
}

func makeToolsList(osType string) tools.List {
	var toolsVersion version.Binary
	toolsVersion.Number = version.MustParse("1.26.0")
	toolsVersion.Arch = corearch.AMD64
	toolsVersion.Release = osType
	return tools.List{{
		Version: toolsVersion,
		URL:     fmt.Sprintf("http://example.com/tools/juju-%s.tgz", toolsVersion),
		SHA256:  "1234567890abcdef",
		Size:    1024,
	}}
}

func unmarshalRequestBody(c *gc.C, req *http.Request, out interface{}) {
	bytes, err := io.ReadAll(req.Body)
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

func (s *environSuite) TestStartInstance(c *gc.C) {
	s.assertStartInstance(c, nil, nil, true, false, false, false)
}

func (s *environSuite) TestStartInstancePrivateIP(c *gc.C) {
	s.assertStartInstance(c, nil, nil, false, false, false, false)
}

func (s *environSuite) TestStartInstanceRootDiskSmallerThanMin(c *gc.C) {
	wantedRootDisk := 22
	s.assertStartInstance(c, &wantedRootDisk, nil, true, false, false, false)
}

func (s *environSuite) TestStartInstanceRootDiskLargerThanMin(c *gc.C) {
	wantedRootDisk := 40
	s.assertStartInstance(c, &wantedRootDisk, nil, true, false, false, false)
}

func (s *environSuite) TestStartInstanceQuotaRetry(c *gc.C) {
	s.assertStartInstance(c, nil, nil, false, true, false, false)
}

func (s *environSuite) TestStartInstanceHypervisorGenRetry(c *gc.C) {
	s.assertStartInstance(c, nil, nil, false, false, true, false)
}

func (s *environSuite) TestStartInstanceConflictRetry(c *gc.C) {
	s.assertStartInstance(c, nil, nil, false, false, false, true)
}

func (s *environSuite) assertStartInstance(
	c *gc.C, wantedRootDisk *int, rootDiskSourceParams map[string]interface{},
	publicIP, withQuotaRetry, withHypervisorGenRetry, withConflictRetry bool,
) {
	env := s.openEnviron(c)

	args := makeStartInstanceParams(c, s.controllerUUID, corebase.MakeDefaultBase("ubuntu", "22.04"))
	if withConflictRetry {
		s.vmTags[tags.JujuUnitsDeployed] = "mysql/0 wordpress/0"
		args.InstanceConfig.Tags[tags.JujuUnitsDeployed] = "mysql/0 wordpress/0"
	}
	diskEncryptionSetName := ""
	vaultName := ""
	vaultKeyName := ""
	if len(rootDiskSourceParams) > 0 {
		encrypted, _ := rootDiskSourceParams["encrypted"].(string)
		if encrypted == "true" {
			args.RootDisk = &jujustorage.VolumeParams{
				Attributes: rootDiskSourceParams,
			}
			diskEncryptionSetName, _ = rootDiskSourceParams["disk-encryption-set-name"].(string)
			vaultName, _ = rootDiskSourceParams["vault-name-prefix"].(string)
			vaultKeyName, _ = rootDiskSourceParams["vault-key-name"].(string)
		}
	}
	s.sender = s.startInstanceSenders(c, startInstanceSenderParams{
		bootstrap:              false,
		diskEncryptionSetName:  diskEncryptionSetName,
		vaultName:              vaultName,
		vaultKeyName:           vaultKeyName,
		withQuotaRetry:         withQuotaRetry,
		withHypervisorGenRetry: withHypervisorGenRetry,
		withConflictRetry:      withConflictRetry,
	})
	if withConflictRetry {
		// Retry after a conflict - the same instance creation senders are
		// used except that the availability set now exists.
		s.sender = append(s.sender, s.startInstanceSenders(c, startInstanceSenderParams{
			bootstrap:               false,
			diskEncryptionSetName:   diskEncryptionSetName,
			vaultName:               vaultName,
			vaultKeyName:            vaultKeyName,
			withQuotaRetry:          withQuotaRetry,
			existingAvailabilitySet: true,
		})...)
	}
	if withHypervisorGenRetry {
		// Retry after a hypervisor generation error - the same instance creation senders are
		// used except that the VM size is changed.
		// s.sender = append(s.sender, makeSender(".*/deployments/juju-06f00d-0/cancel", http.StatusNoContent))
		s.sender = append(s.sender, s.startInstanceSenders(c, startInstanceSenderParams{
			bootstrap:             false,
			diskEncryptionSetName: diskEncryptionSetName,
			vaultName:             vaultName,
			vaultKeyName:          vaultKeyName,
			existingCommon:        true,
		})...)

	}
	if withQuotaRetry {
		// Retry after a quota error - the same instance creation senders are
		// used except that the availability set now exists.
		s.sender = append(s.sender, makeSenderWithStatus(".*/deployments/juju-06f00d-0/cancel", http.StatusNoContent))
		s.sender = append(s.sender, s.startInstanceSenders(c, startInstanceSenderParams{
			bootstrap:             false,
			diskEncryptionSetName: diskEncryptionSetName,
			vaultName:             vaultName,
			vaultKeyName:          vaultKeyName,
			existingCommon:        true,
		})...)
	}
	s.requests = nil
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
	if !publicIP {
		args.Constraints.AllocatePublicIP = &publicIP
	}
	result, err := env.StartInstance(s.callCtx, args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.NotNil)
	c.Assert(result.Instance, gc.NotNil)
	c.Assert(result.NetworkInfo, gc.HasLen, 0)
	c.Assert(result.Volumes, gc.HasLen, 0)
	c.Assert(result.VolumeAttachments, gc.HasLen, 0)

	arch := corearch.DefaultArchitecture
	mem := uint64(1792)
	if withQuotaRetry {
		mem = uint64(3584)
	}
	cpuCores := uint64(1)
	c.Assert(result.Hardware, jc.DeepEquals, &instance.HardwareCharacteristics{
		Arch:     &arch,
		Mem:      &mem,
		RootDisk: &expectedRootDisk,
		CpuCores: &cpuCores,
	})
	startParams := assertStartInstanceRequestsParams{
		imageReference:         &jammyImageReferenceGen2,
		diskSizeGB:             expectedDiskSize,
		osProfile:              &s.linuxOsProfile,
		instanceType:           "Standard_A1",
		publicIP:               publicIP,
		diskEncryptionSet:      diskEncryptionSetName,
		vaultName:              vaultName,
		withQuotaRetry:         withQuotaRetry,
		withHypervisorGenRetry: withHypervisorGenRetry,
		withConflictRetry:      withConflictRetry,
	}
	if withConflictRetry {
		startParams.availabilitySetName = "mysql"
	}
	s.assertStartInstanceRequests(c, s.requests, startParams)
}

func (s *environSuite) TestStartInstanceNoAuthorizedKeys(c *gc.C) {
	env := s.openEnviron(c)
	cfg, err := env.Config().Remove([]string{"authorized-keys"})
	c.Assert(err, jc.ErrorIsNil)
	err = env.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	s.sender = s.startInstanceSenders(c, startInstanceSenderParams{bootstrap: false})
	s.requests = nil
	_, err = env.StartInstance(s.callCtx, makeStartInstanceParams(c, s.controllerUUID, corebase.MakeDefaultBase("ubuntu", "22.04")))
	c.Assert(err, jc.ErrorIsNil)

	s.linuxOsProfile.LinuxConfiguration.SSH.PublicKeys = []*armcompute.SSHPublicKey{{
		Path:    to.Ptr("/home/ubuntu/.ssh/authorized_keys"),
		KeyData: to.Ptr("public"),
	}}
	s.assertStartInstanceRequests(c, s.requests, assertStartInstanceRequestsParams{
		imageReference: &jammyImageReferenceGen2,
		diskSizeGB:     32,
		osProfile:      &s.linuxOsProfile,
		instanceType:   "Standard_A1",
		publicIP:       true,
	})
}

func (s *environSuite) createSenderWithUnauthorisedStatusCode(c *gc.C) {
	unauthSender := &azuretesting.MockSender{}
	unauthSender.AppendAndRepeatResponse(azuretesting.NewResponseWithStatus("401 Unauthorized", http.StatusUnauthorized), 3)
	s.sender = azuretesting.Senders{unauthSender, unauthSender, unauthSender}
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

	_, err = env.StartInstance(s.callCtx, makeStartInstanceParams(c, s.controllerUUID, corebase.MakeDefaultBase("ubuntu", "22.04")))
	c.Assert(err, gc.NotNil)
	c.Assert(s.invalidatedCredential, jc.IsTrue)
}

func (s *environSuite) TestStartControllerInstance(c *gc.C) {
	env := s.openEnviron(c)

	s.sender = s.startInstanceSenders(c, startInstanceSenderParams{
		bootstrap:  false,
		controller: true,
	})

	params := makeStartInstanceParams(c, s.controllerUUID, corebase.MakeDefaultBase("ubuntu", "22.04"))
	params.InstanceConfig.Jobs = []model.MachineJob{model.JobManageModel}
	params.InstanceConfig.ControllerConfig["api-port"] = 17070
	_, err := env.StartInstance(s.callCtx, params)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environSuite) TestStartInstanceCentOS(c *gc.C) {
	// Starting a CentOS VM, we should not expect an image query.
	s.PatchValue(&s.ubuntuServerSKUs, nil)

	env := s.openEnviron(c)
	s.sender = s.startInstanceSenders(c, startInstanceSenderParams{bootstrap: false})
	s.requests = nil
	args := makeStartInstanceParams(c, s.controllerUUID, corebase.MakeDefaultBase("centos", "7"))
	_, err := env.StartInstance(s.callCtx, args)
	c.Assert(err, jc.ErrorIsNil)

	vmExtensionSettings := map[string]interface{}{
		"commandToExecute": `bash -c 'base64 -d /var/lib/waagent/CustomData | bash'`,
	}
	s.assertStartInstanceRequests(c, s.requests, assertStartInstanceRequestsParams{
		imageReference: &centos7ImageReference,
		diskSizeGB:     32,
		vmExtension: &armcompute.VirtualMachineExtensionProperties{
			Publisher:               to.Ptr("Microsoft.OSTCExtensions"),
			Type:                    to.Ptr("CustomScriptForLinux"),
			TypeHandlerVersion:      to.Ptr("1.4"),
			AutoUpgradeMinorVersion: to.Ptr(true),
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
	s.commonDeployment.Properties.ProvisioningState = to.Ptr(armresources.ProvisioningStateFailed)

	env := s.openEnviron(c)
	senders := s.startInstanceSenders(c, startInstanceSenderParams{bootstrap: false})
	s.sender = senders
	s.requests = nil

	_, err := env.StartInstance(s.callCtx, makeStartInstanceParams(c, s.controllerUUID, corebase.MakeDefaultBase("ubuntu", "22.04")))
	c.Assert(err, gc.ErrorMatches,
		`creating virtual machine "juju-06f00d-0": `+
			`waiting for common resources to be created: `+
			`"common" resource deployment status is "Failed"`)
}

func (s *environSuite) TestStartInstanceCommonDeploymentRetryTimeout(c *gc.C) {
	// StartInstance waits for the "common" deployment to complete
	// successfully before creating the VM deployment.
	s.commonDeployment.Properties.ProvisioningState = to.Ptr(armresources.ProvisioningStateCreating)

	env := s.openEnviron(c)
	senders := s.startInstanceSenders(c, startInstanceSenderParams{bootstrap: false})

	const failures = 60 // 5 minutes / 5 seconds
	head, tail := senders[:2], senders[2:]
	for i := 0; i < failures; i++ {
		head = append(head, makeSender("/deployments/common", s.commonDeployment))
	}
	senders = append(head, tail...)
	s.sender = senders
	s.requests = nil

	_, err := env.StartInstance(s.callCtx, makeStartInstanceParams(c, s.controllerUUID, corebase.MakeDefaultBase("ubuntu", "22.04")))
	c.Assert(err, gc.ErrorMatches,
		`creating virtual machine "juju-06f00d-0": `+
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
	s.vmTags[tags.JujuUnitsDeployed] = "mysql/0 wordpress/0"
	s.sender = s.startInstanceSenders(c, startInstanceSenderParams{bootstrap: false})
	s.requests = nil
	params := makeStartInstanceParams(c, s.controllerUUID, corebase.MakeDefaultBase("ubuntu", "22.04"))
	params.InstanceConfig.Tags[tags.JujuUnitsDeployed] = "mysql/0 wordpress/0"

	_, err := env.StartInstance(s.callCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	s.assertStartInstanceRequests(c, s.requests, assertStartInstanceRequestsParams{
		availabilitySetName: "mysql",
		imageReference:      &jammyImageReferenceGen2,
		diskSizeGB:          32,
		osProfile:           &s.linuxOsProfile,
		instanceType:        "Standard_A1",
		publicIP:            true,
	})
}

func (s *environSuite) TestStartInstanceWithSpaceConstraints(c *gc.C) {
	env := s.openEnviron(c)
	s.sender = s.startInstanceSenders(c, startInstanceSenderParams{bootstrap: false, hasSpaceConstraints: true})
	s.requests = nil
	params := makeStartInstanceParams(c, s.controllerUUID, corebase.MakeDefaultBase("ubuntu", "22.04"))
	params.Constraints.Spaces = &[]string{"foo", "bar"}
	params.SubnetsToZones = []map[corenetwork.Id][]string{
		{"/path/to/subnet1": nil},
		{"/path/to/subnet2": nil},
	}

	_, err := env.StartInstance(s.callCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	s.assertStartInstanceRequests(c, s.requests, assertStartInstanceRequestsParams{
		imageReference:      &jammyImageReferenceGen2,
		diskSizeGB:          32,
		osProfile:           &s.linuxOsProfile,
		instanceType:        "Standard_A1",
		publicIP:            true,
		subnets:             []string{"/path/to/subnet1", "/path/to/subnet2"},
		hasSpaceConstraints: true,
	})
}

func (s *environSuite) TestStartInstanceWithInvalidPlacement(c *gc.C) {
	env := s.openEnviron(c)
	s.sender = s.startInstanceSenders(c, startInstanceSenderParams{bootstrap: false})
	s.requests = nil
	params := makeStartInstanceParams(c, s.controllerUUID, corebase.MakeDefaultBase("ubuntu", "22.04"))
	params.Placement = "foo"

	_, err := env.StartInstance(s.callCtx, params)
	c.Assert(err, gc.ErrorMatches, `creating virtual machine "juju-06f00d-0": unknown placement directive: foo`)
}

func (s *environSuite) TestStartInstanceWithInvalidSubnet(c *gc.C) {
	env := s.openEnviron(c)
	s.sender = s.startInstanceSenders(c, startInstanceSenderParams{bootstrap: false})
	s.requests = nil
	params := makeStartInstanceParams(c, s.controllerUUID, corebase.MakeDefaultBase("ubuntu", "22.04"))
	params.Placement = "subnet=foo"

	_, err := env.StartInstance(s.callCtx, params)
	c.Assert(err, gc.ErrorMatches, `creating virtual machine "juju-06f00d-0": subnet "foo" not found`)
}

func (s *environSuite) TestStartInstanceWithPlacementNoSpacesConstraint(c *gc.C) {
	env := s.openEnviron(c)
	subnets := []*armnetwork.Subnet{{
		ID:   to.Ptr("/path/to/subnet1"),
		Name: to.Ptr("subnet1"),
		Properties: &armnetwork.SubnetPropertiesFormat{
			AddressPrefix: to.Ptr("192.168.0.0/20"),
		},
	}, {
		ID:   to.Ptr("/path/to/subnet2"),
		Name: to.Ptr("subnet2"),
		Properties: &armnetwork.SubnetPropertiesFormat{
			AddressPrefix: to.Ptr("192.168.1.0/20"),
		},
	}}
	s.sender = s.startInstanceSenders(c, startInstanceSenderParams{
		bootstrap: false,
		subnets:   subnets,
	})
	s.requests = nil
	params := makeStartInstanceParams(c, s.controllerUUID, corebase.MakeDefaultBase("ubuntu", "22.04"))
	params.Placement = "subnet=subnet2"

	_, err := env.StartInstance(s.callCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	s.assertStartInstanceRequests(c, s.requests, assertStartInstanceRequestsParams{
		imageReference:  &jammyImageReferenceGen2,
		diskSizeGB:      32,
		osProfile:       &s.linuxOsProfile,
		instanceType:    "Standard_A1",
		publicIP:        true,
		subnets:         []string{"/path/to/subnet2"},
		placementSubnet: "subnet2",
	})
}

func (s *environSuite) TestStartInstanceWithPlacement(c *gc.C) {
	env := s.openEnviron(c)
	subnets := []*armnetwork.Subnet{{
		ID:   to.Ptr("/path/to/subnet1"),
		Name: to.Ptr("subnet1"),
		Properties: &armnetwork.SubnetPropertiesFormat{
			AddressPrefix: to.Ptr("192.168.0.0/20"),
		},
	}, {
		ID:   to.Ptr("/path/to/subnet2"),
		Name: to.Ptr("subnet2"),
		Properties: &armnetwork.SubnetPropertiesFormat{
			AddressPrefix: to.Ptr("192.168.1.0/20"),
		},
	}}
	s.sender = s.startInstanceSenders(c, startInstanceSenderParams{
		bootstrap: false,
		subnets:   subnets,
	})
	s.requests = nil
	params := makeStartInstanceParams(c, s.controllerUUID, corebase.MakeDefaultBase("ubuntu", "22.04"))
	params.Constraints.Spaces = &[]string{"foo", "bar"}
	params.SubnetsToZones = []map[corenetwork.Id][]string{
		{"/path/to/subnet1": nil},
		{"/path/to/subnet2": nil},
	}
	params.Placement = "subnet=subnet2"

	_, err := env.StartInstance(s.callCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	s.assertStartInstanceRequests(c, s.requests, assertStartInstanceRequestsParams{
		imageReference:  &jammyImageReferenceGen2,
		diskSizeGB:      32,
		osProfile:       &s.linuxOsProfile,
		instanceType:    "Standard_A1",
		publicIP:        true,
		subnets:         []string{"/path/to/subnet2"},
		placementSubnet: "subnet2",
	})
}

// numExpectedStartInstanceRequests is the number of expected requests base
// by StartInstance method calls. The number is one less for Bootstrap, which
// does not require a query on the common deployment.
const (
	numExpectedStartInstanceRequests          = 5
	numExpectedBootstrapStartInstanceRequests = 4
)

type assertStartInstanceRequestsParams struct {
	autocert               bool
	availabilitySetName    string
	imageReference         *armcompute.ImageReference
	vmExtension            *armcompute.VirtualMachineExtensionProperties
	diskSizeGB             int
	diskEncryptionSet      string
	vaultName              string
	osProfile              *armcompute.OSProfile
	needsProviderInit      bool
	resourceGroupName      string
	instanceType           string
	publicIP               bool
	existingNetwork        string
	subnets                []string
	placementSubnet        string
	withQuotaRetry         bool
	withHypervisorGenRetry bool
	withConflictRetry      bool
	hasSpaceConstraints    bool
	managedIdentity        string
}

func (s *environSuite) assertStartInstanceRequests(
	c *gc.C,
	requests []*http.Request,
	args assertStartInstanceRequestsParams,
) startInstanceRequests {
	nsgId := `[resourceId('Microsoft.Network/networkSecurityGroups', 'juju-internal-nsg')]`
	securityRules := []*armnetwork.SecurityRule{{
		Name: to.Ptr("SSHInbound"),
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Description:              to.Ptr("Allow SSH access to all machines"),
			Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolTCP),
			SourceAddressPrefix:      to.Ptr("*"),
			SourcePortRange:          to.Ptr("*"),
			DestinationAddressPrefix: to.Ptr("*"),
			DestinationPortRange:     to.Ptr("22"),
			Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
			Priority:                 to.Ptr(int32(100)),
			Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
		},
	}}
	if args.autocert {
		// Since a DNS name has been provided, Let's Encrypt is enabled.
		// Therefore ports 443 (for the API server) and 80 (for the HTTP
		// challenge) are accessible.
		securityRules = append(securityRules, &armnetwork.SecurityRule{
			Name: to.Ptr("JujuAPIInbound443"),
			Properties: &armnetwork.SecurityRulePropertiesFormat{
				Description:              to.Ptr("Allow API connections to controller machines"),
				Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolTCP),
				SourceAddressPrefix:      to.Ptr("*"),
				SourcePortRange:          to.Ptr("*"),
				DestinationAddressPrefix: to.Ptr("192.168.16.0/20"),
				DestinationPortRange:     to.Ptr("443"),
				Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
				Priority:                 to.Ptr(int32(101)),
				Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
			},
		}, &armnetwork.SecurityRule{
			Name: to.Ptr("JujuAPIInbound80"),
			Properties: &armnetwork.SecurityRulePropertiesFormat{
				Description:              to.Ptr("Allow API connections to controller machines"),
				Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolTCP),
				SourceAddressPrefix:      to.Ptr("*"),
				SourcePortRange:          to.Ptr("*"),
				DestinationAddressPrefix: to.Ptr("192.168.16.0/20"),
				DestinationPortRange:     to.Ptr("80"),
				Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
				Priority:                 to.Ptr(int32(102)),
				Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
			},
		})
	} else {
		port := fmt.Sprint(testing.FakeControllerConfig()["api-port"])
		securityRules = append(securityRules, &armnetwork.SecurityRule{
			Name: to.Ptr("JujuAPIInbound" + port),
			Properties: &armnetwork.SecurityRulePropertiesFormat{
				Description:              to.Ptr("Allow API connections to controller machines"),
				Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolTCP),
				SourceAddressPrefix:      to.Ptr("*"),
				SourcePortRange:          to.Ptr("*"),
				DestinationAddressPrefix: to.Ptr("192.168.16.0/20"),
				DestinationPortRange:     to.Ptr(port),
				Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
				Priority:                 to.Ptr(int32(101)),
				Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
			},
		})
	}
	subnets := []*armnetwork.Subnet{{
		Name: to.Ptr("juju-internal-subnet"),
		Properties: &armnetwork.SubnetPropertiesFormat{
			AddressPrefix: to.Ptr("192.168.0.0/20"),
			NetworkSecurityGroup: &armnetwork.SecurityGroup{
				ID: to.Ptr(nsgId),
			},
		},
	}, {
		Name: to.Ptr("juju-controller-subnet"),
		Properties: &armnetwork.SubnetPropertiesFormat{
			AddressPrefix: to.Ptr("192.168.16.0/20"),
			NetworkSecurityGroup: &armnetwork.SecurityGroup{
				ID: to.Ptr(nsgId),
			},
		},
	}}

	bootstrapping := false
	subnetName := "juju-internal-subnet"
	if args.availabilitySetName == "juju-controller" {
		subnetName = "juju-controller-subnet"
		bootstrapping = true
	}
	if args.placementSubnet != "" {
		subnetName = args.placementSubnet
	}

	var templateResources []armtemplates.Resource
	var vmDependsOn []string
	if bootstrapping {
		if args.existingNetwork == "" {
			addressPrefixes := to.SliceOfPtrs("192.168.0.0/20", "192.168.16.0/20")
			templateResources = append(templateResources, armtemplates.Resource{
				APIVersion: azure.NetworkAPIVersion,
				Type:       "Microsoft.Network/networkSecurityGroups",
				Name:       "juju-internal-nsg",
				Location:   "westus",
				Tags:       s.envTags,
				Properties: &armnetwork.SecurityGroupPropertiesFormat{
					SecurityRules: securityRules,
				},
			})
			if args.placementSubnet == "" {
				templateResources = append(templateResources, armtemplates.Resource{
					APIVersion: azure.NetworkAPIVersion,
					Type:       "Microsoft.Network/virtualNetworks",
					Name:       "juju-internal-network",
					Location:   "westus",
					Tags:       s.envTags,
					Properties: &armnetwork.VirtualNetworkPropertiesFormat{
						AddressSpace: &armnetwork.AddressSpace{addressPrefixes},
						Subnets:      subnets,
					},
					DependsOn: []string{
						"[resourceId('Microsoft.Network/networkSecurityGroups', 'juju-internal-nsg')]"},
				})
			}
		}
	}

	var availabilitySetSubResource *armcompute.SubResource
	if args.availabilitySetName != "" {
		availabilitySetId := fmt.Sprintf(
			`[resourceId('Microsoft.Compute/availabilitySets','%s')]`,
			args.availabilitySetName,
		)
		availabilitySetProperties := &armcompute.AvailabilitySetProperties{
			PlatformFaultDomainCount: to.Ptr(int32(3)),
		}
		templateResources = append(templateResources, armtemplates.Resource{
			APIVersion: azure.ComputeAPIVersion,
			Type:       "Microsoft.Compute/availabilitySets",
			Name:       args.availabilitySetName,
			Location:   "westus",
			Tags:       s.envTags,
			Properties: availabilitySetProperties,
			Sku:        &armtemplates.Sku{Name: "Aligned"},
		})
		availabilitySetSubResource = &armcompute.SubResource{
			ID: to.Ptr(availabilitySetId),
		}
		vmDependsOn = append(vmDependsOn, availabilitySetId)
	}

	internalNetwork := "juju-internal-network"
	if args.existingNetwork != "" {
		internalNetwork = args.existingNetwork
	}
	rgName := "juju-testmodel-deadbeef"
	if args.resourceGroupName != "" {
		rgName = args.resourceGroupName
	}
	var subnetIds = args.subnets
	if len(subnetIds) == 0 {
		if bootstrapping {
			if args.existingNetwork == "" {
				subnetIds = []string{fmt.Sprintf(
					`[concat(resourceId('%s', 'Microsoft.Network/virtualNetworks', '%s'), '/subnets/%s')]`,
					rgName,
					internalNetwork,
					subnetName,
				)}
			} else {
				subnetIds = []string{fmt.Sprintf("/virtualNetworks/%s/subnet/juju-controller-subnet", internalNetwork)}
			}
		} else {
			subnetIds = []string{fmt.Sprintf("/virtualNetworks/%s/subnet/juju-internal-subnet", internalNetwork)}
		}
	}

	var nicDependsOn []string
	if bootstrapping && args.existingNetwork == "" {
		nicDependsOn = append(nicDependsOn,
			`[resourceId('Microsoft.Network/networkSecurityGroups', 'juju-internal-nsg')]`,
		)
		if args.existingNetwork == "" {
			nicDependsOn = append(nicDependsOn,
				fmt.Sprintf(`[resourceId('%s', 'Microsoft.Network/virtualNetworks', 'juju-internal-network')]`, rgName),
			)
		}
	}
	var publicIPAddress *armnetwork.PublicIPAddress
	if args.publicIP {
		publicIPAddressId := `[resourceId('Microsoft.Network/publicIPAddresses', 'juju-06f00d-0-public-ip')]`
		publicIPAddress = &armnetwork.PublicIPAddress{
			ID: to.Ptr(publicIPAddressId),
		}
	}

	var nicResources []armtemplates.Resource
	var nics []*armcompute.NetworkInterfaceReference
	for i, subnetId := range subnetIds {
		primary := i == 0
		name := "primary"
		if i > 0 {
			name = fmt.Sprintf("interface-%d", i)
		}
		ipConfigurations := []*armnetwork.InterfaceIPConfiguration{{
			Name: to.Ptr(name),
			Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
				Primary:                   to.Ptr(primary),
				PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
				Subnet:                    &armnetwork.Subnet{ID: to.Ptr(subnetId)},
			},
		}}
		if primary && publicIPAddress != nil {
			ipConfigurations[0].Properties.PublicIPAddress = publicIPAddress
			nicDependsOn = append(nicDependsOn, *publicIPAddress.ID)
		}

		nicId := fmt.Sprintf(`[resourceId('Microsoft.Network/networkInterfaces', 'juju-06f00d-0-%s')]`, name)
		nics = append(nics, &armcompute.NetworkInterfaceReference{
			ID: to.Ptr(nicId),
			Properties: &armcompute.NetworkInterfaceReferenceProperties{
				Primary: to.Ptr(primary),
			},
		})
		vmDependsOn = append(vmDependsOn, nicId)
		nicResources = append(nicResources, armtemplates.Resource{
			APIVersion: azure.NetworkAPIVersion,
			Type:       "Microsoft.Network/networkInterfaces",
			Name:       "juju-06f00d-0-" + name,
			Location:   "westus",
			Tags:       s.vmTags,
			Properties: &armnetwork.InterfacePropertiesFormat{
				IPConfigurations: ipConfigurations,
			},
			DependsOn: nicDependsOn,
		})
	}

	osDisk := &armcompute.OSDisk{
		Name:         to.Ptr("juju-06f00d-0"),
		CreateOption: to.Ptr(armcompute.DiskCreateOptionTypesFromImage),
		Caching:      to.Ptr(armcompute.CachingTypesReadWrite),
		DiskSizeGB:   to.Ptr(int32(args.diskSizeGB)),
		ManagedDisk: &armcompute.ManagedDiskParameters{
			StorageAccountType: to.Ptr(armcompute.StorageAccountTypesStandardLRS),
		},
	}
	if args.diskEncryptionSet != "" {
		osDisk.ManagedDisk.DiskEncryptionSet = &armcompute.DiskEncryptionSetParameters{
			ID: to.Ptr(
				fmt.Sprintf("[resourceId('Microsoft.Compute/diskEncryptionSets', '%s')]", args.diskEncryptionSet)),
		}
	}

	if args.publicIP {
		templateResources = append(templateResources, armtemplates.Resource{
			APIVersion: azure.NetworkAPIVersion,
			Type:       "Microsoft.Network/publicIPAddresses",
			Name:       "juju-06f00d-0-public-ip",
			Location:   "westus",
			Tags:       s.vmTags,
			Properties: &armnetwork.PublicIPAddressPropertiesFormat{
				PublicIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodStatic),
				PublicIPAddressVersion:   to.Ptr(armnetwork.IPVersionIPv4),
			},
			Sku: &armtemplates.Sku{Name: "Standard"},
		})
	}
	templateResources = append(templateResources, nicResources...)
	vmTemplate := armtemplates.Resource{
		APIVersion: azure.ComputeAPIVersion,
		Type:       "Microsoft.Compute/virtualMachines",
		Name:       "juju-06f00d-0",
		Location:   "westus",
		Tags:       s.vmTags,
		Properties: &armcompute.VirtualMachineProperties{
			HardwareProfile: &armcompute.HardwareProfile{
				VMSize: to.Ptr(armcompute.VirtualMachineSizeTypes(args.instanceType)),
			},
			StorageProfile: &armcompute.StorageProfile{
				ImageReference: args.imageReference,
				OSDisk:         osDisk,
			},
			OSProfile: args.osProfile,
			NetworkProfile: &armcompute.NetworkProfile{
				NetworkInterfaces: nics,
			},
			AvailabilitySet: availabilitySetSubResource,
		},
		DependsOn: vmDependsOn,
	}
	if args.managedIdentity != "" {
		vmTemplate.Identity = &armcompute.VirtualMachineIdentity{
			Type: to.Ptr(armcompute.ResourceIdentityTypeUserAssigned),
			UserAssignedIdentities: map[string]*armcompute.UserAssignedIdentitiesValue{
				fmt.Sprintf(
					"/subscriptions/%s/resourcegroups/%s/providers/Microsoft.ManagedIdentity/userAssignedIdentities/%s",
					fakeManagedSubscriptionId, resourceGroupName, args.managedIdentity): nil,
			},
		}
	}
	templateResources = append(templateResources, vmTemplate)
	if args.vmExtension != nil {
		templateResources = append(templateResources, armtemplates.Resource{
			APIVersion: azure.ComputeAPIVersion,
			Type:       "Microsoft.Compute/virtualMachines/extensions",
			Name:       "juju-06f00d-0/JujuCustomScriptExtension",
			Location:   "westus",
			Tags:       s.vmTags,
			Properties: args.vmExtension,
			DependsOn:  []string{"Microsoft.Compute/virtualMachines/juju-06f00d-0"},
		})
	}
	templateMap := map[string]interface{}{
		"$schema":        "https://schema.management.azure.com/schemas/2015-01-01/deploymentTemplate.json#",
		"contentVersion": "1.0.0.0",
		"resources":      templateResources,
	}
	deployment := &armresources.Deployment{
		Properties: &armresources.DeploymentProperties{
			Template: &templateMap,
			Mode:     to.Ptr(armresources.DeploymentModeIncremental),
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
		if bootstrapping {
			if args.existingNetwork == "" {
				c.Assert(requests, gc.HasLen, numExpectedBootstrapStartInstanceRequests-1)
			} else {
				c.Assert(requests, gc.HasLen, numExpectedBootstrapStartInstanceRequests+1)
			}
		} else {
			if args.diskEncryptionSet != "" && args.vaultName != "" {
				c.Assert(requests, gc.HasLen, numExpectedStartInstanceRequests+4)
			} else if args.hasSpaceConstraints {
				c.Assert(requests, gc.HasLen, numExpectedStartInstanceRequests-1)
			} else if args.withConflictRetry {
				c.Assert(requests, gc.HasLen, numExpectedStartInstanceRequests+3)
			} else if args.withQuotaRetry {
				c.Assert(requests, gc.HasLen, numExpectedStartInstanceRequests+5)
			} else if args.withHypervisorGenRetry {
				c.Assert(requests, gc.HasLen, numExpectedStartInstanceRequests+4)
			} else {
				c.Assert(requests, gc.HasLen, numExpectedStartInstanceRequests)
			}
		}
		if args.needsProviderInit {
			if args.resourceGroupName != "" {
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
	if !bootstrapping {
		c.Assert(requests[nexti()].Method, gc.Equals, "GET") // wait for common deployment
		if len(args.subnets) == 0 {
			c.Assert(requests[nexti()].Method, gc.Equals, "GET") // subnets
		}
	}
	if args.placementSubnet != "" {
		c.Assert(requests[nexti()].Method, gc.Equals, "GET") // get subnets
	}
	if args.vaultName != "" {
		c.Assert(requests[nexti()].Method, gc.Equals, "GET")  // deleted vaults
		c.Assert(requests[nexti()].Method, gc.Equals, "PUT")  // create vault
		c.Assert(requests[nexti()].Method, gc.Equals, "POST") // get token
		c.Assert(requests[nexti()].Method, gc.Equals, "GET")  // newly created vault
	}
	if bootstrapping && args.existingNetwork != "" {
		c.Assert(requests[nexti()].Method, gc.Equals, "GET") // wait for common deployment
		c.Assert(requests[nexti()].Method, gc.Equals, "GET") // subnets
	}
	ideployment := nexti()
	c.Assert(requests[ideployment].Method, gc.Equals, "PUT") // create deployment
	startInstanceRequests.deployment = requests[ideployment]

	// Marshal/unmarshal the deployment we expect, so it's in map form.
	var expected armresources.Deployment
	data, err := json.Marshal(&deployment)
	c.Assert(err, jc.ErrorIsNil)
	err = json.Unmarshal(data, &expected)
	c.Assert(err, jc.ErrorIsNil)

	// Check that we send what we expect. CustomData is non-deterministic,
	// so don't compare it.
	// TODO(axw) shouldn't CustomData be deterministic? Look into this.
	var actual armresources.Deployment
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

	// Fix the round tripping of the vm identities.
	resources, ok = expected.Properties.Template.(map[string]interface{})["resources"].([]interface{})
	c.Assert(ok, jc.IsTrue)
	identity, _ := resources[vmResourceIndex].(map[string]interface{})["identity"]
	if identity != nil {
		userAssignedIdentities, _ := identity.(map[string]interface{})["userAssignedIdentities"].(map[string]interface{})
		for k := range userAssignedIdentities {
			userAssignedIdentities[k] = map[string]interface{}{}
		}
	}

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

	ctx := envtesting.BootstrapTODOContext(c)
	env := prepareForBootstrap(c, ctx, s.provider, &s.sender)

	s.sender = s.initResourceGroupSenders(resourceGroupName)
	s.sender = append(s.sender, s.startInstanceSenders(c, startInstanceSenderParams{bootstrap: true})...)
	s.requests = nil
	result, err := env.Bootstrap(
		ctx, s.callCtx, environs.BootstrapParams{
			ControllerConfig:        testing.FakeControllerConfig(),
			AvailableTools:          makeToolsList("ubuntu"),
			BootstrapBase:           corebase.MustParseBaseFromString("ubuntu@22.04"),
			BootstrapConstraints:    constraints.MustParse("mem=3.5G"),
			SupportedBootstrapBases: testing.FakeSupportedJujuBases,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Arch, gc.Equals, "amd64")
	c.Assert(result.Base.DisplayString(), gc.Equals, "ubuntu@22.04")

	c.Assert(len(s.requests), gc.Equals, numExpectedBootstrapStartInstanceRequests)
	s.vmTags[tags.JujuIsController] = "true"
	s.assertStartInstanceRequests(c, s.requests[1:], assertStartInstanceRequestsParams{
		availabilitySetName: "juju-controller",
		imageReference:      &jammyImageReferenceGen2,
		diskSizeGB:          32,
		osProfile:           &s.linuxOsProfile,
		instanceType:        "Standard_D1",
		publicIP:            true,
	})
}

func (s *environSuite) TestBootstrapPrivateIP(c *gc.C) {
	defer envtesting.DisableFinishBootstrap()()

	ctx := envtesting.BootstrapTODOContext(c)
	env := prepareForBootstrap(c, ctx, s.provider, &s.sender)

	s.sender = s.initResourceGroupSenders(resourceGroupName)
	s.sender = append(s.sender, s.startInstanceSenders(c, startInstanceSenderParams{bootstrap: true})...)
	s.requests = nil
	result, err := env.Bootstrap(
		ctx, s.callCtx, environs.BootstrapParams{
			ControllerConfig:        testing.FakeControllerConfig(),
			AvailableTools:          makeToolsList("ubuntu"),
			BootstrapBase:           corebase.MustParseBaseFromString("ubuntu@22.04"),
			BootstrapConstraints:    constraints.MustParse("mem=3.5G allocate-public-ip=false"),
			SupportedBootstrapBases: testing.FakeSupportedJujuBases,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Arch, gc.Equals, "amd64")
	c.Assert(result.Base.DisplayString(), gc.Equals, "ubuntu@22.04")

	c.Assert(len(s.requests), gc.Equals, numExpectedBootstrapStartInstanceRequests)
	s.vmTags[tags.JujuIsController] = "true"
	s.assertStartInstanceRequests(c, s.requests[1:], assertStartInstanceRequestsParams{
		availabilitySetName: "juju-controller",
		imageReference:      &jammyImageReferenceGen2,
		diskSizeGB:          32,
		osProfile:           &s.linuxOsProfile,
		instanceType:        "Standard_D1",
	})
}

func (s *environSuite) TestBootstrapCustomNetwork(c *gc.C) {
	defer envtesting.DisableFinishBootstrap()()

	ctx := envtesting.BootstrapTODOContext(c)
	env := prepareForBootstrap(c, ctx, s.provider, &s.sender, testing.Attrs{"network": "mynetwork"})

	s.sender = s.initResourceGroupSenders(resourceGroupName)
	s.sender = append(s.sender, s.startInstanceSenders(c, startInstanceSenderParams{bootstrap: true, existingNetwork: "mynetwork"})...)
	s.requests = nil
	result, err := env.Bootstrap(
		ctx, s.callCtx, environs.BootstrapParams{
			ControllerConfig:        testing.FakeControllerConfig(),
			AvailableTools:          makeToolsList("ubuntu"),
			BootstrapBase:           corebase.MustParseBaseFromString("ubuntu@22.04"),
			BootstrapConstraints:    constraints.MustParse("mem=3.5G"),
			SupportedBootstrapBases: testing.FakeSupportedJujuBases,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Arch, gc.Equals, "amd64")
	c.Assert(result.Base.DisplayString(), gc.Equals, "ubuntu@22.04")

	// 2 extra requests for network setup.
	c.Assert(len(s.requests), gc.Equals, numExpectedBootstrapStartInstanceRequests+2)
	s.vmTags[tags.JujuIsController] = "true"
	s.assertStartInstanceRequests(c, s.requests[1:], assertStartInstanceRequestsParams{
		availabilitySetName: "juju-controller",
		imageReference:      &jammyImageReferenceGen2,
		diskSizeGB:          32,
		osProfile:           &s.linuxOsProfile,
		instanceType:        "Standard_D1",
		publicIP:            true,
		existingNetwork:     "mynetwork",
	})
}

func (s *environSuite) TestBootstrapUserSpecifiedManagedIdentity(c *gc.C) {
	defer envtesting.DisableFinishBootstrap()()

	ctx := envtesting.BootstrapTODOContext(c)
	env := prepareForBootstrap(c, ctx, s.provider, &s.sender)

	s.sender = s.initResourceGroupSenders(resourceGroupName)
	s.sender = append(s.sender, s.startInstanceSenders(c, startInstanceSenderParams{bootstrap: true})...)
	s.requests = nil
	result, err := env.Bootstrap(
		ctx, s.callCtx, environs.BootstrapParams{
			ControllerConfig:        testing.FakeControllerConfig(),
			AvailableTools:          makeToolsList("ubuntu"),
			BootstrapBase:           corebase.MustParseBaseFromString("ubuntu@22.04"),
			BootstrapConstraints:    constraints.MustParse("mem=3.5G instance-role=myidentity"),
			SupportedBootstrapBases: testing.FakeSupportedJujuBases,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Arch, gc.Equals, "amd64")
	c.Assert(result.Base.DisplayString(), gc.Equals, "ubuntu@22.04")

	c.Assert(len(s.requests), gc.Equals, numExpectedBootstrapStartInstanceRequests)
	s.vmTags[tags.JujuIsController] = "true"
	s.assertStartInstanceRequests(c, s.requests[1:], assertStartInstanceRequestsParams{
		availabilitySetName: "juju-controller",
		imageReference:      &jammyImageReferenceGen2,
		diskSizeGB:          32,
		osProfile:           &s.linuxOsProfile,
		instanceType:        "Standard_D1",
		publicIP:            true,
		managedIdentity:     "myidentity",
	})
}

func (s *environSuite) TestBootstrapWithInvalidCredential(c *gc.C) {
	defer envtesting.DisableFinishBootstrap()()

	ctx := envtesting.BootstrapTODOContext(c)
	env := prepareForBootstrap(c, ctx, s.provider, &s.sender)

	s.createSenderWithUnauthorisedStatusCode(c)
	s.sender = append(s.sender, s.startInstanceSenders(c, startInstanceSenderParams{bootstrap: true})...)
	s.requests = nil

	c.Assert(s.invalidatedCredential, jc.IsFalse)
	_, err := env.Bootstrap(
		ctx, s.callCtx, environs.BootstrapParams{
			ControllerConfig:        testing.FakeControllerConfig(),
			AvailableTools:          makeToolsList("ubuntu"),
			BootstrapBase:           corebase.MustParseBaseFromString("ubuntu@22.04"),
			BootstrapConstraints:    constraints.MustParse("mem=3.5G"),
			SupportedBootstrapBases: testing.FakeSupportedJujuBases,
		},
	)
	c.Assert(err, gc.NotNil)
	c.Assert(s.invalidatedCredential, jc.IsTrue)

	// Successful bootstrap expects 4 but we expect to bail out after getting an authorised error.
	c.Assert(s.requests, gc.HasLen, 1)
}

func (s *environSuite) TestBootstrapInstanceConstraints(c *gc.C) {
	defer envtesting.DisableFinishBootstrap()()

	ctx := envtesting.BootstrapTODOContext(c)
	env := prepareForBootstrap(c, ctx, s.provider, &s.sender)

	s.sender = append(s.sender, s.resourceSKUsSender())
	s.sender = append(s.sender, s.initResourceGroupSenders(resourceGroupName)...)
	s.sender = append(s.sender, s.startInstanceSendersNoSizes()...)
	s.requests = nil
	err := bootstrap.Bootstrap(
		ctx, env, s.callCtx, bootstrap.BootstrapParams{
			ControllerConfig: testing.FakeControllerConfig(),
			AdminSecret:      jujutesting.AdminSecret,
			CAPrivateKey:     testing.CAKey,
			BootstrapBase:    corebase.MustParseBaseFromString("ubuntu@22.04"),
			BuildAgentTarball: func(
				build bool, _ string, _ func(version.Number) version.Number,
			) (*sync.BuiltAgent, error) {
				c.Assert(build, jc.IsFalse)
				return &sync.BuiltAgent{Dir: c.MkDir()}, nil
			},
			SupportedBootstrapBases: testing.FakeSupportedJujuBases,
		},
	)

	var imageReference *armcompute.ImageReference
	var instanceType string
	if corearch.HostArch() == "amd64" {
		imageReference = &jammyImageReferenceGen2
		instanceType = "Standard_D1"
	} else if corearch.HostArch() == "arm64" {
		imageReference = &jammyImageReferenceArm64
		instanceType = "Standard_D8ps_v5"
	} else {
		// If we aren't on amd64/arm64, this should correctly fail. See also:
		// lp#1638706: environSuite.TestBootstrapInstanceConstraints fails on rare archs and series
		wantErr := fmt.Sprintf("model %q of type %s does not support instances running on %q",
			env.Config().Name(),
			env.Config().Type(),
			corearch.HostArch())
		c.Assert(err, gc.ErrorMatches, wantErr)
		c.SucceedNow()
		return
	}
	// amd64 should pass the rest of the test.
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(len(s.requests), gc.Equals, numExpectedBootstrapStartInstanceRequests)
	s.vmTags[tags.JujuIsController] = "true"
	s.assertStartInstanceRequests(c, s.requests[1:], assertStartInstanceRequestsParams{
		availabilitySetName: "juju-controller",
		imageReference:      imageReference,
		diskSizeGB:          32,
		osProfile:           &s.linuxOsProfile,
		needsProviderInit:   true,
		instanceType:        instanceType,
		publicIP:            true,
	})
}

func (s *environSuite) TestBootstrapCustomResourceGroup(c *gc.C) {
	defer envtesting.DisableFinishBootstrap()()

	ctx := envtesting.BootstrapTODOContext(c)
	env := prepareForBootstrap(c, ctx, s.provider, &s.sender, testing.Attrs{"resource-group-name": "foo"})

	s.sender = append(s.sender, s.resourceSKUsSender())
	s.sender = append(s.sender, s.initResourceGroupSenders("foo")...)
	s.sender = append(s.sender, s.startInstanceSendersNoSizes()...)
	s.requests = nil
	err := bootstrap.Bootstrap(
		ctx, env, s.callCtx, bootstrap.BootstrapParams{
			ControllerConfig: testing.FakeControllerConfig(),
			AdminSecret:      jujutesting.AdminSecret,
			CAPrivateKey:     testing.CAKey,
			BootstrapBase:    corebase.MustParseBaseFromString("ubuntu@22.04"),
			BuildAgentTarball: func(
				build bool, _ string, _ func(version.Number) version.Number,
			) (*sync.BuiltAgent, error) {
				c.Assert(build, jc.IsFalse)
				return &sync.BuiltAgent{Dir: c.MkDir()}, nil
			},
			SupportedBootstrapBases: testing.FakeSupportedJujuBases,
		},
	)
	var imageReference *armcompute.ImageReference
	var instanceType string
	if corearch.HostArch() == "amd64" {
		imageReference = &jammyImageReferenceGen2
		instanceType = "Standard_D1"
	} else if corearch.HostArch() == "arm64" {
		imageReference = &jammyImageReferenceArm64
		instanceType = "Standard_D8ps_v5"
	} else {
		// If we aren't on amd64/arm64, this should correctly fail. See also:
		// lp#1638706: environSuite.TestBootstrapInstanceConstraints fails on rare archs and series
		wantErr := fmt.Sprintf("model %q of type %s does not support instances running on %q",
			env.Config().Name(),
			env.Config().Type(),
			corearch.HostArch())
		c.Assert(err, gc.ErrorMatches, wantErr)
		c.SucceedNow()
		return
	}
	// amd64 should pass the rest of the test.
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(len(s.requests), gc.Equals, numExpectedBootstrapStartInstanceRequests)
	s.vmTags[tags.JujuIsController] = "true"
	s.assertStartInstanceRequests(c, s.requests[1:], assertStartInstanceRequestsParams{
		availabilitySetName: "juju-controller",
		imageReference:      imageReference,
		diskSizeGB:          32,
		osProfile:           &s.linuxOsProfile,
		needsProviderInit:   true,
		resourceGroupName:   "foo",
		instanceType:        instanceType,
		publicIP:            true,
	})
}

func (s *environSuite) TestBootstrapWithAutocert(c *gc.C) {
	defer envtesting.DisableFinishBootstrap()()

	ctx := envtesting.BootstrapTODOContext(c)
	env := prepareForBootstrap(c, ctx, s.provider, &s.sender)

	s.sender = s.initResourceGroupSenders(resourceGroupName)
	s.sender = append(s.sender, s.startInstanceSenders(c, startInstanceSenderParams{bootstrap: true})...)
	s.requests = nil
	config := testing.FakeControllerConfig()
	config["api-port"] = 443
	config["autocert-dns-name"] = "example.com"
	result, err := env.Bootstrap(
		ctx, s.callCtx, environs.BootstrapParams{
			ControllerConfig:        config,
			AvailableTools:          makeToolsList("ubuntu"),
			BootstrapBase:           corebase.MustParseBaseFromString("ubuntu@22.04"),
			BootstrapConstraints:    constraints.MustParse("mem=3.5G"),
			SupportedBootstrapBases: testing.FakeSupportedJujuBases,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Arch, gc.Equals, "amd64")
	c.Assert(result.Base.DisplayString(), gc.Equals, "ubuntu@22.04")

	c.Assert(len(s.requests), gc.Equals, numExpectedBootstrapStartInstanceRequests)
	s.vmTags[tags.JujuIsController] = "true"
	s.assertStartInstanceRequests(c, s.requests[1:], assertStartInstanceRequestsParams{
		autocert:            true,
		availabilitySetName: "juju-controller",
		imageReference:      &jammyImageReferenceGen2,
		diskSizeGB:          32,
		osProfile:           &s.linuxOsProfile,
		instanceType:        "Standard_D1",
		publicIP:            true,
	})
}

func (s *environSuite) TestAllRunningInstancesResourceGroupNotFound(c *gc.C) {
	env := s.openEnviron(c)
	sender := &azuretesting.MockSender{}
	sender.AppendAndRepeatResponse(azuretesting.NewResponseWithStatus(
		"resource group not found", http.StatusNotFound,
	), 2)
	s.sender = azuretesting.Senders{sender, sender}
	_, err := env.AllRunningInstances(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environSuite) TestAllRunningInstancesIgnoresCommonDeployment(c *gc.C) {
	env := s.openEnviron(c)

	dependencies := []*armresources.Dependency{{
		ID: to.Ptr("whatever"),
	}}
	deployments := []*armresources.DeploymentExtended{{
		// common deployment should be ignored
		Name: to.Ptr("common"),
		Properties: &armresources.DeploymentPropertiesExtended{
			ProvisioningState: to.Ptr(armresources.ProvisioningStateSucceeded),
			Dependencies:      dependencies,
		},
	}}
	s.sender = azuretesting.Senders{
		makeSender("/deployments", armresources.DeploymentListResult{Value: deployments}),
		makeSender("/virtualMachines", armcompute.VirtualMachineListResult{}),
	}

	instances, err := env.AllRunningInstances(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instances, gc.HasLen, 0)
}

func (s *environSuite) TestStopInstancesNotFound(c *gc.C) {
	env := s.openEnviron(c)
	sender0 := &azuretesting.MockSender{}
	sender0.AppendAndRepeatResponse(azuretesting.NewResponseWithStatus(
		"vm not found", http.StatusNotFound,
	), 2)
	sender1 := &azuretesting.MockSender{}
	sender1.AppendAndRepeatResponse(azuretesting.NewResponseWithStatus(
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
	err := env.StopInstances(s.callCtx, "a")
	c.Assert(err, gc.NotNil)
	c.Assert(s.invalidatedCredential, jc.IsTrue)
	c.Assert(s.requests, gc.HasLen, 1)
}

func (s *environSuite) TestStopInstancesNoSecurityGroup(c *gc.C) {
	env := s.openEnviron(c)

	// Make a NIC with the Juju security group so we can
	nic0IPConfiguration := makeIPConfiguration("192.168.0.4")
	nic0IPConfiguration.Properties.Primary = to.Ptr(true)
	internalSubnetId := path.Join(
		"/subscriptions", fakeManagedSubscriptionId,
		"resourceGroups/juju-testmodel-model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"providers/Microsoft.Network/virtualNetworks/juju-internal-network/subnets/juju-internal-subnet",
	)
	nic0IPConfiguration.Properties.Subnet = &armnetwork.Subnet{
		ID:         &internalSubnetId,
		Properties: &armnetwork.SubnetPropertiesFormat{},
	}
	nic0IPConfiguration.Properties.PublicIPAddress = &armnetwork.PublicIPAddress{}
	nic0 := makeNetworkInterface("nic-0", "juju-06f00d-0", nic0IPConfiguration)
	s.sender = azuretesting.Senders{
		makeSenderWithStatus(".*/deployments/juju-06f00d-0/cancel", http.StatusNoContent), // Cancel
		s.networkInterfacesSender(nic0),                                            // GET: no NICs
		s.publicIPAddressesSender(),                                                // GET: no public IPs
		makeSender(".*/virtualMachines/juju-06f00d-0", nil),                        // DELETE
		makeSender(".*/disks/juju-06f00d-0", nil),                                  // DELETE
		makeSender(internalSubnetId, nic0IPConfiguration.Properties.Subnet),        // GET: subnets to get security group
		makeSender(".*/networkInterfaces/nic-0", nil),                              // DELETE
		makeSenderWithStatus(".*/deployments/juju-06f00d-0", http.StatusNoContent), // DELETE
	}
	err := env.StopInstances(s.callCtx, "juju-06f00d-0")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environSuite) TestStopInstances(c *gc.C) {
	env := s.openEnviron(c)

	// Security group has rules for juju-06f00d-0, as well as a rule that doesn't match.
	nsg := makeSecurityGroup(
		makeSecurityRule("juju-06f00d-0-80", "192.168.0.4", "80"),
		makeSecurityRule("juju-06f00d-0-1000-2000", "192.168.0.4", "1000-2000"),
		makeSecurityRule("machine-42", "192.168.0.5", "*"),
	)

	// Create an IP configuration with a public IP reference. This will
	// cause an update to the NIC to detach public IPs.
	nic0IPConfiguration := makeIPConfiguration("192.168.0.4")
	nic0IPConfiguration.Properties.PublicIPAddress = &armnetwork.PublicIPAddress{}
	nic0IPConfiguration.Properties.Primary = to.Ptr(true)
	nic0 := makeNetworkInterface("nic-0", "juju-06f00d-0", nic0IPConfiguration)
	nic0.Properties.NetworkSecurityGroup = &nsg

	s.sender = azuretesting.Senders{
		makeSenderWithStatus(".*/deployments/juju-06f00d-0/cancel", http.StatusNoContent), // POST
		s.networkInterfacesSender(nic0),
		s.publicIPAddressesSender(makePublicIPAddress("pip-0", "juju-06f00d-0", "1.2.3.4")),
		makeSender(".*/virtualMachines/juju-06f00d-0", nil),                                                 // DELETE
		makeSender(".*/disks/juju-06f00d-0", nil),                                                           // GET
		makeSender(".*/networkSecurityGroups/juju-internal-nsg/securityRules/juju-06f00d-0-80", nil),        // DELETE
		makeSender(".*/networkSecurityGroups/juju-internal-nsg/securityRules/juju-06f00d-0-1000-2000", nil), // DELETE
		makeSender(".*/networkInterfaces/nic-0", nil),                                                       // DELETE
		makeSender(".*/publicIPAddresses/pip-0", nil),                                                       // DELETE
		makeSenderWithStatus(".*/deployments/juju-06f00d-0", http.StatusNoContent),                          // DELETE
	}

	err := env.StopInstances(s.callCtx, "juju-06f00d-0")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environSuite) TestStopInstancesMultiple(c *gc.C) {
	env := s.openEnviron(c)

	vmDeleteSender0 := s.makeErrorSender(".*/virtualMachines/juju-06f00d-[01]", errors.New("blargh"), 2)
	vmDeleteSender1 := s.makeErrorSender(".*/virtualMachines/juju-06f00d-[01]", errors.New("blargh"), 2)

	s.sender = azuretesting.Senders{
		makeSenderWithStatus(".*/deployments/juju-06f00d-[01]/cancel", http.StatusNoContent), // POST
		makeSenderWithStatus(".*/deployments/juju-06f00d-[01]/cancel", http.StatusNoContent), // POST

		// We should only query the NICs and public IPs
		// regardless of how many instances are deleted.
		s.networkInterfacesSender(),
		s.publicIPAddressesSender(),

		vmDeleteSender0,
		vmDeleteSender1,
	}
	err := env.StopInstances(s.callCtx, "juju-06f00d-0", "juju-06f00d-1")
	c.Assert(err, gc.ErrorMatches, `deleting instance "juju-06f00d-[01]":.*blargh`)
}

func (s *environSuite) TestStopInstancesDeploymentNotFound(c *gc.C) {
	env := s.openEnviron(c)

	cancelSender := &azuretesting.MockSender{}
	cancelSender.AppendAndRepeatResponse(azuretesting.NewResponseWithStatus(
		"deployment not found", http.StatusNotFound,
	), 2)
	s.sender = azuretesting.Senders{cancelSender}
	err := env.StopInstances(s.callCtx, "juju-06f00d-0")
	c.Assert(err, jc.ErrorIsNil)
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
	_, err := validator.Validate(constraints.MustParse("arch=s390x"))
	c.Assert(err, gc.ErrorMatches,
		"invalid constraint value: arch=s390x\nvalid values are: amd64 arm64",
	)
	_, err = validator.Validate(constraints.MustParse("instance-type=t1.micro"))
	c.Assert(err, gc.ErrorMatches,
		"invalid constraint value: instance-type=t1.micro\nvalid values are: A1 D1 D2 D8ps_v5 Standard_A1 Standard_D1 Standard_D2 Standard_D8ps_v5",
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

func (s *environSuite) TestConstraintsConflict(c *gc.C) {
	validator := s.constraintsValidator(c)
	_, err := validator.Validate(constraints.MustParse("arch=amd64 instance-type=Standard_D1"))
	c.Assert(err, jc.ErrorIsNil)
	_, err = validator.Validate(constraints.MustParse("arch=arm64 instance-type=Standard_D1"))
	c.Assert(err, gc.ErrorMatches, `ambiguous constraints: "arch" overlaps with "instance-type": instance-type="Standard_D1" expected arch="amd64" not "arm64"`)
}

func (s *environSuite) constraintsValidator(c *gc.C) constraints.Validator {
	env := s.openEnviron(c)
	s.sender = azuretesting.Senders{s.resourceSKUsSender()}
	validator, err := env.ConstraintsValidator(context.NewEmptyCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)
	return validator
}

func (s *environSuite) TestHasRegion(c *gc.C) {
	env := s.openEnviron(c)
	c.Assert(env, gc.Implements, new(simplestreams.HasRegion))
	cloudSpec, err := env.(simplestreams.HasRegion).Region()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloudSpec, gc.Equals, simplestreams.CloudSpec{
		Region:   "westus",
		Endpoint: "https://api.azurestack.local",
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
	res := []*armresources.GenericResourceExpanded{{
		ID:   to.Ptr("id-0"),
		Name: to.Ptr("juju-06f00d-0"),
		Type: to.Ptr("Microsoft.Compute/virtualMachines"),
	}, {
		ID:   to.Ptr("id-0"),
		Name: to.Ptr("juju-06f00d-0-disk"),
		Type: to.Ptr("Microsoft.Compute/disks"),
	}, {
		ID:   to.Ptr("networkSecurityGroups/nsg-0"),
		Name: to.Ptr("nsg-0"),
		Type: to.Ptr("Microsoft.Network/networkSecurityGroups"),
	}, {
		ID:   to.Ptr("vaults/secret-0"),
		Name: to.Ptr("secret-0"),
		Type: to.Ptr("Microsoft.KeyVault/vaults"),
	}}
	resourceListResult := armresources.ResourceListResult{Value: res}

	nic0IPConfiguration := makeIPConfiguration("192.168.0.4")
	nic0IPConfiguration.Properties.PublicIPAddress = &armnetwork.PublicIPAddress{}
	nic0 := makeNetworkInterface("nic-0", "juju-06f00d-0", nic0IPConfiguration)

	s.sender = azuretesting.Senders{
		makeSender(".*/resourceGroups/foo/resources.*", resourceListResult),               // GET
		makeSenderWithStatus(".*/deployments/juju-06f00d-0/cancel", http.StatusNoContent), // POST
		s.networkInterfacesSender(nic0),
		s.publicIPAddressesSender(makePublicIPAddress("pip-0", "juju-06f00d-0", "1.2.3.4")),
		makeSender(".*/virtualMachines/juju-06f00d-0", nil),                                                              // DELETE
		makeSender(".*/disks/juju-06f00d-0", nil),                                                                        // DELETE
		makeSender(".*/networkInterfaces/nic-0", nil),                                                                    // DELETE
		makeSender(".*/publicIPAddresses/pip-0", nil),                                                                    // DELETE
		makeSenderWithStatus(".*/deployments/juju-06f00d-0", http.StatusNoContent),                                       // DELETE
		s.makeErrorSender("/networkSecurityGroups/nsg-0", newAzureResponseError(c, http.StatusConflict, "InUse", ""), 1), // DELETE
		makeSender("/networkSecurityGroups/nsg-0", nil),                                                                  // DELETE
		makeSender(".*/vaults/secret-0", nil),                                                                            // DELETE
	}
	err := env.Destroy(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.requests, gc.HasLen, 12)
	c.Assert(s.requests[0].Method, gc.Equals, "GET")
	c.Assert(s.requests[0].URL.Query().Get("$filter"), gc.Equals, fmt.Sprintf(
		"tagName eq 'juju-model-uuid' and tagValue eq '%s'",
		testing.ModelTag.Id(),
	))
	c.Assert(s.requests[7].Method, gc.Equals, "DELETE")
	c.Assert(s.requests[8].Method, gc.Equals, "DELETE")
	c.Assert(s.requests[9].Method, gc.Equals, "DELETE")
	c.Assert(s.requests[10].Method, gc.Equals, "DELETE")
	c.Assert(s.requests[11].Method, gc.Equals, "DELETE")
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
	groups := []*armresources.ResourceGroup{{
		Name: to.Ptr("group1"),
	}, {
		Name: to.Ptr("group2"),
	}}
	result := armresources.ResourceGroupListResult{Value: groups}

	env := s.openEnviron(c)
	s.sender = azuretesting.Senders{
		makeSender(".*/resourcegroups", result),                        // GET
		makeSender(".*/resourcegroups/group[12]", nil),                 // DELETE
		makeSender(".*/resourcegroups/group[12]", nil),                 // DELETE
		makeSender(".*/roleDefinitions*", nil),                         // GET
		makeSender(".*/roleAssignments*", nil),                         // GET
		makeSender(".*/userAssignedIdentities/juju-controller-*", nil), // DELETE
	}
	err := env.DestroyController(s.callCtx, s.controllerUUID)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.requests, gc.HasLen, 6)
	c.Assert(s.requests[0].Method, gc.Equals, "GET")
	c.Assert(s.requests[0].URL.Query().Get("$filter"), gc.Equals, fmt.Sprintf(
		"tagName eq 'juju-controller-uuid' and tagValue eq '%s'",
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

	c.Assert(s.requests[3].Method, gc.Equals, "GET")
	c.Assert(s.requests[4].Method, gc.Equals, "GET")
	c.Assert(s.requests[5].Method, gc.Equals, "DELETE")
	c.Assert(path.Base(s.requests[5].URL.Path), gc.Equals, "juju-controller-"+testing.ControllerTag.Id())
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
		"tagName eq 'juju-controller-uuid' and tagValue eq '%s'",
		testing.ControllerTag.Id(),
	))
}

func (s *environSuite) TestDestroyControllerErrors(c *gc.C) {
	groups := []*armresources.ResourceGroup{
		{Name: to.Ptr("group1")},
		{Name: to.Ptr("group2")},
	}
	result := armresources.ResourceGroupListResult{Value: groups}

	makeErrorSender := func(err string) *azuretesting.MockSender {
		errorSender := s.makeErrorSender(".*/resourcegroups/group[12].*", errors.New(err), 2)
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
	c.Check(destroyErr, gc.ErrorMatches, ".*(foo|bar).*")
}

func (s *environSuite) TestInstanceInformation(c *gc.C) {
	env := s.openEnviron(c)
	s.sender = s.startInstanceSenders(c, startInstanceSenderParams{bootstrap: false})
	types, err := env.InstanceTypes(s.callCtx, constraints.Value{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(types.InstanceTypes, gc.HasLen, 6)

	cons := constraints.MustParse("mem=4G")
	types, err = env.InstanceTypes(s.callCtx, cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(types.InstanceTypes, gc.HasLen, 4)
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

	res1 := resourcesResult.Value[0]
	res1.Properties = &map[string]interface{}{"has-properties": true}

	res2 := resourcesResult.Value[1]
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
	checkAPIVersion(0, "GET", "2021-04-01")
	checkAPIVersion(1, "PUT", "2021-04-01")
	// Resources.
	checkAPIVersion(4, "GET", "2021-07-01")
	checkAPIVersion(5, "PUT", "2021-07-01")
	checkAPIVersion(6, "GET", "2021-07-01")
	checkAPIVersion(7, "PUT", "2021-07-01")

	checkTagsAndProperties := func(ix uint) {
		req := s.requests[ix]
		data := make([]byte, req.ContentLength)
		_, err := req.Body.Read(data)
		c.Assert(err, jc.ErrorIsNil)

		var resource armresources.GenericResource
		err = json.Unmarshal(data, &resource)
		c.Assert(err, jc.ErrorIsNil)

		rTags := resource.Tags
		c.Check(toValue(rTags["something else"]), gc.Equals, "good")
		c.Check(toValue(rTags[tags.JujuController]), gc.Equals, "new-controller")
		c.Check(resource.Properties, gc.DeepEquals, map[string]interface{}{"has-properties": true})
	}
	checkTagsAndProperties(5)
	checkTagsAndProperties(7)

	// Also check the tags are right for the resource group.
	req := s.requests[1] // the resource group update.
	data := make([]byte, req.ContentLength)
	_, err = req.Body.Read(data)
	c.Assert(err, jc.ErrorIsNil)
	var group armresources.ResourceGroup
	err = json.Unmarshal(data, &group)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the provisioning state wasn't sent back.
	c.Check((*group.Properties).ProvisioningState, gc.IsNil)

	gTags := group.Tags
	c.Check(toValue(gTags["something else"]), gc.Equals, "good")
	c.Check(toValue(gTags[tags.JujuController]), gc.Equals, "new-controller")
	c.Check(toValue(gTags[tags.JujuModel]), gc.Equals, "deadbeef-0bad-400d-8000-4b1d0d06f00d")
}

func makeProvidersResult() armresources.ProviderListResult {
	providers := []*armresources.Provider{{
		Namespace: to.Ptr("Beck.Replica"),
		ResourceTypes: []*armresources.ProviderResourceType{{
			ResourceType: to.Ptr("battles/ladida"),
			APIVersions:  to.SliceOfPtrs("2016-12-15", "2014-02-02"),
		}, {
			ResourceType: to.Ptr("liars/scissor"),
			APIVersions:  to.SliceOfPtrs("2021-07-01", "2015-03-02"),
		}},
	}, {
		Namespace: to.Ptr("Tuneyards.Bizness"),
		ResourceTypes: []*armresources.ProviderResourceType{{
			ResourceType: to.Ptr("slaves/debbie"),
			APIVersions:  to.SliceOfPtrs("2016-12-14", "2014-04-02"),
		}, {
			ResourceType: to.Ptr("micachu"),
			APIVersions:  to.SliceOfPtrs("2021-07-01", "2015-05-02"),
		}},
	}}
	return armresources.ProviderListResult{Value: providers}
}

func makeResourcesResult() armresources.ResourceListResult {
	theResources := []*armresources.GenericResourceExpanded{{
		ID:       to.Ptr("/subscriptions/foo/resourcegroups/bar/providers/Beck.Replica/liars/scissor/boxing-day-blues"),
		Name:     to.Ptr("boxing-day-blues"),
		Type:     to.Ptr("Beck.Replica/liars/scissor"),
		Location: to.Ptr("westus"),
		Tags: map[string]*string{
			tags.JujuController: to.Ptr("old-controller"),
			"something else":    to.Ptr("good"),
		},
	}, {
		ID:       to.Ptr("/subscriptions/foo/resourcegroups/bar/providers/Tuneyards.Bizness/micachu/drop-dead"),
		Name:     to.Ptr("drop-dead"),
		Type:     to.Ptr("Tuneyards.Bizness/micachu"),
		Location: to.Ptr("westus"),
		Tags: map[string]*string{
			tags.JujuController: to.Ptr("old-controller"),
			"something else":    to.Ptr("good"),
		},
	}}
	return armresources.ResourceListResult{Value: theResources}
}

func makeResourceGroupResult() *armresources.ResourceGroup {
	return &armresources.ResourceGroup{
		Name:     to.Ptr("charles"),
		Location: to.Ptr("westus"),
		Properties: &armresources.ResourceGroupProperties{
			ProvisioningState: to.Ptr("very yes"),
		},
		Tags: map[string]*string{
			tags.JujuController: to.Ptr("old-controller"),
			tags.JujuModel:      to.Ptr("deadbeef-0bad-400d-8000-4b1d0d06f00d"),
			"something else":    to.Ptr("good"),
		},
	}
}

func (s *environSuite) TestAdoptResourcesErrorGettingGroup(c *gc.C) {
	env := s.openEnviron(c)
	sender := s.makeErrorSender(
		".*/resourcegroups/juju-testmodel-.*",
		errors.New("uhoh"),
		4)
	s.sender = azuretesting.Senders{sender}

	err := env.AdoptResources(s.callCtx, "new-controller", version.MustParse("1.0.0"))
	c.Assert(err, gc.ErrorMatches, ".*uhoh$")
	c.Assert(s.requests, gc.HasLen, 1)
}

func (s *environSuite) TestAdoptResourcesErrorUpdatingGroup(c *gc.C) {
	env := s.openEnviron(c)
	errorSender := s.makeErrorSender(
		".*/resourcegroups/juju-testmodel-.*",
		errors.New("uhoh"),
		2)
	s.sender = azuretesting.Senders{
		makeSender(".*/resourcegroups/juju-testmodel-.*", makeResourceGroupResult()),
		errorSender,
	}

	err := env.AdoptResources(s.callCtx, "new-controller", version.MustParse("1.0.0"))
	c.Assert(err, gc.ErrorMatches, ".*uhoh$")
	c.Assert(s.requests, gc.HasLen, 2)
}

func (s *environSuite) TestAdoptResourcesErrorGettingVersions(c *gc.C) {
	env := s.openEnviron(c)
	errorSender := s.makeErrorSender(
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
	c.Assert(s.requests, gc.HasLen, 3)
}

func (s *environSuite) TestAdoptResourcesErrorListingResources(c *gc.C) {
	env := s.openEnviron(c)
	errorSender := s.makeErrorSender(
		".*/resourceGroups/juju-testmodel-.*/resources",
		errors.New("ouch!"),
		2)
	s.sender = azuretesting.Senders{
		makeSender(".*/resourcegroups/juju-testmodel-.*", makeResourceGroupResult()),
		makeSender(".*/resourcegroups/juju-testmodel-.*", nil),
		makeSender(".*/providers", armresources.ProviderListResult{}),
		errorSender,
	}

	err := env.AdoptResources(s.callCtx, "new-controller", version.MustParse("1.0.0"))
	c.Assert(err, gc.ErrorMatches, ".*ouch!$")
	c.Assert(s.requests, gc.HasLen, 4)
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
	res1 := resourcesResult.Value[0]
	res1.Tags[tags.JujuController] = to.Ptr("new-controller")
	res2 := resourcesResult.Value[1]

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

	res2 := resourcesResult.Value[1]

	env := s.openEnviron(c)

	errorSender := s.makeErrorSender(
		".*/resourcegroups/.*/providers/Beck.Replica/liars/scissor/boxing-day-blues",
		errors.New("flagrant error! virus=very yes"),
		1)

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
	c.Check(s.requests, gc.HasLen, 7)
}

func (s *environSuite) TestAdoptResourcesErrorUpdating(c *gc.C) {
	providersResult := makeProvidersResult()
	resourcesResult := makeResourcesResult()

	res1 := resourcesResult.Value[0]
	res2 := resourcesResult.Value[1]

	env := s.openEnviron(c)

	errorSender := s.makeErrorSender(
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
	c.Check(s.requests, gc.HasLen, 8)
}

func (s *environSuite) TestStartInstanceEncryptedRootDiskExistingDES(c *gc.C) {
	rootDiskParams := map[string]interface{}{
		"encrypted":                "true",
		"disk-encryption-set-name": "my-disk-encryption-set",
	}
	s.assertStartInstance(c, nil, rootDiskParams, true, false, false, false)
}

func (s *environSuite) TestStartInstanceEncryptedRootDisk(c *gc.C) {
	rootDiskParams := map[string]interface{}{
		"encrypted":                "true",
		"disk-encryption-set-name": "my-disk-encryption-set",
		"vault-name-prefix":        "my-vault",
		"vault-key-name":           "shhhh",
	}
	s.assertStartInstance(c, nil, rootDiskParams, true, false, false, false)
}

func (s *environSuite) TestGetArchFromResourceSKUARM64(c *gc.C) {
	arch := azure.GetArchFromResourceSKU(&armcompute.ResourceSKU{
		Family: to.Ptr("standardDPSv5Family"),
	})
	c.Assert(arch, gc.Equals, corearch.ARM64)

	arch = azure.GetArchFromResourceSKU(&armcompute.ResourceSKU{
		Family: to.Ptr("standardDPLSv5Family"),
	})
	c.Assert(arch, gc.Equals, corearch.ARM64)

	arch = azure.GetArchFromResourceSKU(&armcompute.ResourceSKU{
		Family: to.Ptr("standardEPSv5Family"),
	})
	c.Assert(arch, gc.Equals, corearch.ARM64)
}

func (s *environSuite) TestGetArchFromResourceSKUAMD64(c *gc.C) {
	arch := azure.GetArchFromResourceSKU(&armcompute.ResourceSKU{
		Family: to.Ptr(""),
	})
	c.Assert(arch, gc.Equals, corearch.AMD64)

	arch = azure.GetArchFromResourceSKU(&armcompute.ResourceSKU{
		Family: to.Ptr("StandardNCadsH100v5Family"),
	})
	c.Assert(arch, gc.Equals, corearch.AMD64)

	arch = azure.GetArchFromResourceSKU(&armcompute.ResourceSKU{
		Family: to.Ptr("StandardNCADSA100v4Family"),
	})
	c.Assert(arch, gc.Equals, corearch.AMD64)
}
