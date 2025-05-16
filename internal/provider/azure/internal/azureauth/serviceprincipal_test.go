// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"regexp"
	stdtesting "testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v3"
	"github.com/google/uuid"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	abstractions "github.com/microsoft/kiota-abstractions-go"
	"github.com/microsoft/kiota-abstractions-go/authentication"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	"github.com/microsoft/kiota-abstractions-go/store"
	nethttplibrary "github.com/microsoft/kiota-http-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"

	"github.com/juju/juju/internal/provider/azure/internal/azureauth"
	"github.com/juju/juju/internal/provider/azure/internal/azuretesting"
	"github.com/juju/juju/internal/testhelpers"
)

type requestResult struct {
	PathPattern string
	Params      map[string]string
	Result      serialization.Parsable
	Err         error
}

type MockRequestAdaptor struct {
	*nethttplibrary.NetHttpRequestAdapter

	results []requestResult
}

func (m *MockRequestAdaptor) Send(ctx context.Context, requestInfo *abstractions.RequestInformation, constructor serialization.ParsableFactory, errorMappings abstractions.ErrorMappings) (serialization.Parsable, error) {
	if len(m.results) == 0 {
		return nil, errors.Errorf("no results for %q", requestInfo.PathParameters)
	}
	res := m.results[0]
	m.results = m.results[1:]
	if res.PathPattern != "" {
		matched, err := regexp.MatchString(res.PathPattern, requestInfo.UrlTemplate)
		if err != nil {
			return nil, err
		}
		if !matched {
			return nil, fmt.Errorf(
				"request path %q did not match pattern %q",
				requestInfo.UrlTemplate, res.PathPattern,
			)
		}
	}
	for k, v := range res.Params {
		if val := requestInfo.PathParameters[k]; val != v {
			return nil, fmt.Errorf(
				"request path parameter %q=%q did not match parameter %q",
				k, v, val,
			)
		}
	}
	return res.Result, res.Err
}

type InteractiveSuite struct {
	testhelpers.IsolationSuite
	clock   testclock.AdvanceableClock
	newUUID func() (uuid.UUID, error)
}

func TestInteractiveSuite(t *stdtesting.T) { tc.Run(t, &InteractiveSuite{}) }

const fakeTenantId = "11111111-1111-1111-1111-111111111111"

func roleDefinitionListSender(name string) *azuretesting.MockSender {
	roleDefinitions := []*armauthorization.RoleDefinition{{
		ID:   to.Ptr("owner-role-id"),
		Name: to.Ptr("name-id"),
		Properties: &armauthorization.RoleDefinitionProperties{
			RoleName: to.Ptr(name),
		},
	}}
	return azuretesting.NewSenderWithValue(armauthorization.RoleDefinitionListResult{
		Value: roleDefinitions,
	})
}

func roleAssignmentSender() *azuretesting.MockSender {
	return azuretesting.NewSenderWithValue(armauthorization.RoleAssignment{})
}

func roleAssignmentAlreadyExistsSender() *azuretesting.MockSender {
	sender := &azuretesting.MockSender{}
	body := azuretesting.NewBody(`{"error":{"code":"RoleAssignmentExists", "message":"Odata v4 compliant message"}}`)
	sender.AppendResponse(azuretesting.NewResponseWithBodyAndStatus(body, http.StatusConflict, ""))
	return sender
}

func roleAssignmentPrincipalNotExistSender() *azuretesting.MockSender {
	sender := &azuretesting.MockSender{}
	// Based on https://github.com/Azure/azure-powershell/issues/655#issuecomment-186332230
	body := azuretesting.NewBody(`{"error":{"code":"PrincipalNotFound","message":"Principal foo does not exist in the directory bar"}}`)
	sender.AppendResponse(azuretesting.NewResponseWithBodyAndStatus(body, http.StatusNotFound, ""))
	return sender
}

func (s *InteractiveSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	uuids := []string{
		"33333333-3333-3333-3333-333333333333", // password
		"44444444-4444-4444-4444-444444444444", // password key ID
		"55555555-5555-5555-5555-555555555555", // role assignment ID
	}
	s.newUUID = func() (res uuid.UUID, err error) {
		res, err = uuid.Parse(uuids[0])
		if err != nil {
			return res, err
		}
		uuids = uuids[1:]
		return res, nil
	}
	s.clock = testclock.NewDilatedWallClock(10 * time.Millisecond)
}

func (s *InteractiveSuite) TestInteractive(c *tc.C) {
	ra, err := nethttplibrary.NewNetHttpRequestAdapter(&authentication.AnonymousAuthenticationProvider{})
	c.Assert(err, tc.ErrorIsNil)

	sp := models.NewServicePrincipal()
	sp.SetAppId(to.Ptr("app-id"))
	sp.SetId(to.Ptr("sp-object-id"))
	cred := models.NewPasswordCredential()
	cred.SetSecretText(to.Ptr("33333333-3333-3333-3333-333333333333"))

	app := models.NewApplication()
	app.SetAppId(to.Ptr("app-id"))
	appResp := models.NewApplicationCollectionResponse()
	appResp.SetValue([]models.Applicationable{app})

	mockAdaptor := &MockRequestAdaptor{NetHttpRequestAdapter: ra}
	mockAdaptor.results = []requestResult{{
		PathPattern: regexp.QuoteMeta("{+baseurl}/applications") + ".*",
		Result:      appResp,
	}, {
		PathPattern: regexp.QuoteMeta("{+baseurl}/servicePrincipals(appId='{appId}')") + ".*",
		Params:      map[string]string{"appId": "app-id"},
		Result:      sp,
	}, {
		PathPattern: regexp.QuoteMeta("{+baseurl}/servicePrincipals/{servicePrincipal%2Did}/addPassword"),
		Params:      map[string]string{"servicePrincipal%2Did": "sp-object-id"},
		Result:      cred,
	}}

	subscriptionId := "22222222-2222-2222-2222-222222222222"
	authSenders := &azuretesting.Senders{
		roleDefinitionListSender("Some other role"),
		roleDefinitionListSender("Juju Role Definition - " + subscriptionId),
		roleAssignmentSender(),
	}
	spc := azureauth.ServicePrincipalCreator{
		Sender:         authSenders,
		RequestAdaptor: mockAdaptor,
		Clock:          s.clock,
		NewUUID:        s.newUUID,
	}

	var stderr bytes.Buffer
	sdkCtx := c.Context()

	appId, spObjectId, password, err := spc.InteractiveCreate(sdkCtx, &stderr, azureauth.ServicePrincipalParams{
		CloudName:      "AzureCloud",
		SubscriptionId: subscriptionId,
		TenantId:       fakeTenantId,
		Credential:     &azuretesting.FakeCredential{},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appId, tc.Equals, "app-id")
	c.Assert(password, tc.Equals, "33333333-3333-3333-3333-333333333333")
	c.Assert(spObjectId, tc.Equals, "sp-object-id")
	c.Assert(stderr.String(), tc.Equals, `
Initiating interactive authentication.

`[1:])
}

func (s *InteractiveSuite) TestInteractiveRoleAssignmentAlreadyExists(c *tc.C) {
	ra, err := nethttplibrary.NewNetHttpRequestAdapter(&authentication.AnonymousAuthenticationProvider{})
	c.Assert(err, tc.ErrorIsNil)

	sp := models.NewServicePrincipal()
	sp.SetAppId(to.Ptr("app-id"))
	sp.SetId(to.Ptr("sp-object-id"))
	cred := models.NewPasswordCredential()
	cred.SetSecretText(to.Ptr("33333333-3333-3333-3333-333333333333"))

	app := models.NewApplication()
	app.SetAppId(to.Ptr("app-id"))
	appResp := models.NewApplicationCollectionResponse()
	appResp.SetValue([]models.Applicationable{app})

	mockAdaptor := &MockRequestAdaptor{NetHttpRequestAdapter: ra}
	mockAdaptor.results = []requestResult{{
		PathPattern: regexp.QuoteMeta("{+baseurl}/applications") + ".*",
		Result:      appResp,
	}, {
		PathPattern: regexp.QuoteMeta("{+baseurl}/servicePrincipals(appId='{appId}')") + ".*",
		Params:      map[string]string{"appId": "app-id"},
		Result:      sp,
	}, {
		PathPattern: regexp.QuoteMeta("{+baseurl}/servicePrincipals/{servicePrincipal%2Did}/addPassword"),
		Params:      map[string]string{"servicePrincipal%2Did": "sp-object-id"},
		Result:      cred,
	}}

	subscriptionId := "22222222-2222-2222-2222-222222222222"
	authSenders := &azuretesting.Senders{
		roleDefinitionListSender("Juju Role Definition - " + subscriptionId),
		roleAssignmentAlreadyExistsSender(),
	}
	spc := azureauth.ServicePrincipalCreator{
		Sender:         authSenders,
		RequestAdaptor: mockAdaptor,
		Clock:          s.clock,
		NewUUID:        s.newUUID,
	}

	var stderr bytes.Buffer
	sdkCtx := c.Context()

	appId, spObjectId, password, err := spc.InteractiveCreate(sdkCtx, &stderr, azureauth.ServicePrincipalParams{
		CloudName:      "AzureCloud",
		SubscriptionId: subscriptionId,
		TenantId:       fakeTenantId,
		Credential:     &azuretesting.FakeCredential{},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appId, tc.Equals, "app-id")
	c.Assert(password, tc.Equals, "33333333-3333-3333-3333-333333333333")
	c.Assert(spObjectId, tc.Equals, "sp-object-id")
	c.Assert(stderr.String(), tc.Equals, `
Initiating interactive authentication.

`[1:])
}

func dataError(code string) error {
	result := odataerrors.NewODataError()
	mainErr := odataerrors.NewMainError()
	mainErr.SetCode(to.Ptr(code))
	bs := store.NewInMemoryBackingStore()
	result.SetBackingStore(bs)
	result.SetErrorEscaped(mainErr)
	return result
}

func (s *InteractiveSuite) TestInteractiveServicePrincipalNotFound(c *tc.C) {
	ra, err := nethttplibrary.NewNetHttpRequestAdapter(&authentication.AnonymousAuthenticationProvider{})
	c.Assert(err, tc.ErrorIsNil)

	sp := models.NewServicePrincipal()
	sp.SetAppId(to.Ptr("app-id"))
	sp.SetId(to.Ptr("sp-object-id"))
	cred := models.NewPasswordCredential()
	cred.SetSecretText(to.Ptr("33333333-3333-3333-3333-333333333333"))

	app := models.NewApplication()
	app.SetAppId(to.Ptr("app-id"))
	appResp := models.NewApplicationCollectionResponse()
	appResp.SetValue([]models.Applicationable{app})

	mockAdaptor := &MockRequestAdaptor{NetHttpRequestAdapter: ra}
	mockAdaptor.results = []requestResult{{
		PathPattern: regexp.QuoteMeta("{+baseurl}/applications") + ".*",
		Result:      appResp,
	}, {
		PathPattern: regexp.QuoteMeta("{+baseurl}/servicePrincipals(appId='{appId}')") + ".*",
		Params:      map[string]string{"appId": "app-id"},
		Err:         dataError("Request_ResourceNotFound"),
	}, {
		PathPattern: regexp.QuoteMeta("{+baseurl}/servicePrincipals"),
		Result:      sp,
	}, {
		PathPattern: regexp.QuoteMeta("{+baseurl}/servicePrincipals/{servicePrincipal%2Did}/addPassword"),
		Params:      map[string]string{"servicePrincipal%2Did": "sp-object-id"},
		Result:      cred,
	}}

	subscriptionId := "22222222-2222-2222-2222-222222222222"
	authSenders := &azuretesting.Senders{
		roleDefinitionListSender("Juju Role Definition - " + subscriptionId),
		roleAssignmentSender(),
	}
	spc := azureauth.ServicePrincipalCreator{
		Sender:         authSenders,
		RequestAdaptor: mockAdaptor,
		Clock:          s.clock,
		NewUUID:        s.newUUID,
	}

	var stderr bytes.Buffer
	sdkCtx := c.Context()

	appId, spObjectId, password, err := spc.InteractiveCreate(sdkCtx, &stderr, azureauth.ServicePrincipalParams{
		CloudName:      "AzureCloud",
		SubscriptionId: subscriptionId,
		TenantId:       fakeTenantId,
		Credential:     &azuretesting.FakeCredential{},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appId, tc.Equals, "app-id")
	c.Assert(password, tc.Equals, "33333333-3333-3333-3333-333333333333")
	c.Assert(spObjectId, tc.Equals, "sp-object-id")
}

func (s *InteractiveSuite) TestInteractiveServicePrincipalNotFoundRace(c *tc.C) {
	ra, err := nethttplibrary.NewNetHttpRequestAdapter(&authentication.AnonymousAuthenticationProvider{})
	c.Assert(err, tc.ErrorIsNil)

	sp := models.NewServicePrincipal()
	sp.SetAppId(to.Ptr("app-id"))
	sp.SetId(to.Ptr("sp-object-id"))
	cred := models.NewPasswordCredential()
	cred.SetSecretText(to.Ptr("33333333-3333-3333-3333-333333333333"))

	app := models.NewApplication()
	app.SetAppId(to.Ptr("app-id"))
	appResp := models.NewApplicationCollectionResponse()
	appResp.SetValue([]models.Applicationable{app})

	mockAdaptor := &MockRequestAdaptor{NetHttpRequestAdapter: ra}
	mockAdaptor.results = []requestResult{{
		PathPattern: regexp.QuoteMeta("{+baseurl}/applications") + ".*",
		Result:      appResp,
	}, {
		PathPattern: regexp.QuoteMeta("{+baseurl}/servicePrincipals(appId='{appId}')") + ".*",
		Params:      map[string]string{"appId": "app-id"},
		Err:         dataError("Request_ResourceNotFound"),
	}, {
		Err: dataError("Request_MultipleObjectsWithSameKeyValue"),
	}, {
		PathPattern: regexp.QuoteMeta("{+baseurl}/servicePrincipals(appId='{appId}')") + ".*",
		Params:      map[string]string{"appId": "app-id"},
		Result:      sp,
	}, {
		PathPattern: regexp.QuoteMeta("{+baseurl}/servicePrincipals/{servicePrincipal%2Did}/addPassword"),
		Params:      map[string]string{"servicePrincipal%2Did": "sp-object-id"},
		Result:      cred,
	}}

	subscriptionId := "22222222-2222-2222-2222-222222222222"
	authSenders := &azuretesting.Senders{
		roleDefinitionListSender("Juju Role Definition - " + subscriptionId),
		roleAssignmentSender(),
	}
	spc := azureauth.ServicePrincipalCreator{
		Sender:         authSenders,
		RequestAdaptor: mockAdaptor,
		Clock:          s.clock,
		NewUUID:        s.newUUID,
	}

	var stderr bytes.Buffer
	sdkCtx := c.Context()

	appId, spObjectId, password, err := spc.InteractiveCreate(sdkCtx, &stderr, azureauth.ServicePrincipalParams{
		CloudName:      "AzureCloud",
		SubscriptionId: subscriptionId,
		TenantId:       fakeTenantId, Credential: &azuretesting.FakeCredential{},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appId, tc.Equals, "app-id")
	c.Assert(password, tc.Equals, "33333333-3333-3333-3333-333333333333")
	c.Assert(spObjectId, tc.Equals, "sp-object-id")
}

func (s *InteractiveSuite) TestInteractiveRetriesRoleAssignment(c *tc.C) {
	ra, err := nethttplibrary.NewNetHttpRequestAdapter(&authentication.AnonymousAuthenticationProvider{})
	c.Assert(err, tc.ErrorIsNil)

	sp := models.NewServicePrincipal()
	sp.SetAppId(to.Ptr("app-id"))
	sp.SetId(to.Ptr("sp-object-id"))
	cred := models.NewPasswordCredential()
	cred.SetSecretText(to.Ptr("33333333-3333-3333-3333-333333333333"))

	app := models.NewApplication()
	app.SetAppId(to.Ptr("app-id"))
	appResp := models.NewApplicationCollectionResponse()
	appResp.SetValue([]models.Applicationable{app})

	mockAdaptor := &MockRequestAdaptor{NetHttpRequestAdapter: ra}
	mockAdaptor.results = []requestResult{{
		PathPattern: regexp.QuoteMeta("{+baseurl}/applications") + ".*",
		Result:      appResp,
	}, {
		PathPattern: regexp.QuoteMeta("{+baseurl}/servicePrincipals(appId='{appId}')") + ".*",
		Params:      map[string]string{"appId": "app-id"},
		Result:      sp,
	}, {
		PathPattern: regexp.QuoteMeta("{+baseurl}/servicePrincipals/{servicePrincipal%2Did}/addPassword"),
		Params:      map[string]string{"servicePrincipal%2Did": "sp-object-id"},
		Result:      cred,
	}}

	subscriptionId := "22222222-2222-2222-2222-222222222222"
	authSenders := &azuretesting.Senders{
		roleDefinitionListSender("Juju Role Definition - " + subscriptionId),
		roleAssignmentPrincipalNotExistSender(),
		roleAssignmentSender(),
	}
	spc := azureauth.ServicePrincipalCreator{
		Sender:         authSenders,
		RequestAdaptor: mockAdaptor,
		Clock:          s.clock,
		NewUUID:        s.newUUID,
	}

	var stderr bytes.Buffer
	sdkCtx := c.Context()
	appId, spObjectId, password, err := spc.InteractiveCreate(sdkCtx, &stderr, azureauth.ServicePrincipalParams{
		CloudName:      "AzureCloud",
		SubscriptionId: subscriptionId,
		TenantId:       fakeTenantId, Credential: &azuretesting.FakeCredential{},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appId, tc.Equals, "app-id")
	c.Assert(password, tc.Equals, "33333333-3333-3333-3333-333333333333")
	c.Assert(spObjectId, tc.Equals, "sp-object-id")
}
