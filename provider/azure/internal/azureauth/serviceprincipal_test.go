// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/authorization/mgmt/2015-07-01/authorization"
	"github.com/Azure/azure-sdk-for-go/services/graphrbac/1.6/graphrbac"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/mocks"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/azure/internal/azureauth"
	"github.com/juju/juju/provider/azure/internal/azuretesting"
)

func clockStartTime() time.Time {
	t, _ := time.Parse("2006-Jan-02 3:04am", "2016-Sep-19 9:47am")
	return t
}

type InteractiveSuite struct {
	testing.IsolationSuite
	clock   *testclock.Clock
	newUUID func() (utils.UUID, error)
}

var _ = gc.Suite(&InteractiveSuite{})

func deviceCodeSender() autorest.Sender {
	return azuretesting.NewSenderWithValue(adal.DeviceCode{
		DeviceCode: to.StringPtr("device-code"),
		Interval:   to.Int64Ptr(1), // 1 second between polls
		Message:    to.StringPtr("open your browser, etc."),
	})
}

func tokenSender() autorest.Sender {
	return azuretesting.NewSenderWithValue(adal.Token{
		RefreshToken: "refresh-token",
		ExpiresOn:    json.Number(fmt.Sprint(time.Now().Add(time.Hour).Unix())),
	})
}

func passwordCredentialsListSender() autorest.Sender {
	v := []graphrbac.PasswordCredential{{
		KeyID: to.StringPtr("password-credential-key-id"),
	}}
	return azuretesting.NewSenderWithValue(graphrbac.PasswordCredentialListResult{
		Value: &v,
	})
}

func updatePasswordCredentialsSender() autorest.Sender {
	sender := mocks.NewSender()
	sender.AppendResponse(mocks.NewResponseWithStatus("", http.StatusNoContent))
	return sender
}

func currentUserSender() autorest.Sender {
	return azuretesting.NewSenderWithValue(graphrbac.User{
		DisplayName: to.StringPtr("Foo Bar"),
	})
}

func createServicePrincipalSender() autorest.Sender {
	return azuretesting.NewSenderWithValue(graphrbac.ServicePrincipal{
		AppID:    to.StringPtr("cbb548f1-5039-4836-af0b-727e8571f6a9"),
		ObjectID: to.StringPtr("sp-object-id"),
	})
}

func createServicePrincipalAlreadyExistsSender(withUTF8BOM bool) autorest.Sender {
	sender := mocks.NewSender()
	bodyData := `{"odata.error":{"code":"Request_MultipleObjectsWithSameKeyValue"}}`
	if withUTF8BOM {
		bodyData = "\ufeff" + bodyData
	}
	body := mocks.NewBody(bodyData)
	sender.AppendResponse(mocks.NewResponseWithBodyAndStatus(body, http.StatusConflict, ""))
	return sender
}

func createServicePrincipalNotExistSender() autorest.Sender {
	sender := mocks.NewSender()
	bodyData := `{"odata.error":{"code":"Request_ResourceNotFound","message":{"lang":"en","value":"... does not exist in the directory ..."}}}`
	body := mocks.NewBody(bodyData)
	sender.AppendResponse(mocks.NewResponseWithBodyAndStatus(body, http.StatusNotFound, ""))
	return sender
}

func createServicePrincipalNotReferenceSender() autorest.Sender {
	sender := mocks.NewSender()
	// Error message cribbed from https://github.com/kubernetes/kubernetes-anywhere/issues/251
	bodyData := `{"odata.error":{"code":"Request_BadRequest","message":{"lang":"en","value":"The appId of the service principal does not reference a valid application object."}}}`
	body := mocks.NewBody(bodyData)
	sender.AppendResponse(mocks.NewResponseWithBodyAndStatus(body, http.StatusBadRequest, ""))
	return sender
}

func servicePrincipalListSender() autorest.Sender {
	return azuretesting.NewSenderWithValue(graphrbac.ServicePrincipalListResult{
		Value: &[]graphrbac.ServicePrincipal{{
			AppID:    to.StringPtr("cbb548f1-5039-4836-af0b-727e8571f6a9"),
			ObjectID: to.StringPtr("sp-object-id"),
		}},
	})
}

func roleDefinitionListSender() autorest.Sender {
	roleDefinitions := []authorization.RoleDefinition{{
		ID:   to.StringPtr("owner-role-id"),
		Name: to.StringPtr("Owner"),
	}}
	return azuretesting.NewSenderWithValue(authorization.RoleDefinitionListResult{
		Value: &roleDefinitions,
	})
}

func roleAssignmentSender() autorest.Sender {
	return azuretesting.NewSenderWithValue(authorization.RoleAssignment{})
}

func roleAssignmentAlreadyExistsSender() autorest.Sender {
	sender := mocks.NewSender()
	body := mocks.NewBody(`{"error":{"code":"RoleAssignmentExists", "message":"Odata v4 compliant message"}}`)
	sender.AppendResponse(mocks.NewResponseWithBodyAndStatus(body, http.StatusConflict, ""))
	return sender
}

func roleAssignmentPrincipalNotExistSender() autorest.Sender {
	sender := mocks.NewSender()
	// Based on https://github.com/Azure/azure-powershell/issues/655#issuecomment-186332230
	body := mocks.NewBody(`{"error":{"code":"PrincipalNotFound","message":"Principal foo does not exist in the directory bar"}}`)
	sender.AppendResponse(mocks.NewResponseWithBodyAndStatus(body, http.StatusNotFound, ""))
	return sender
}

func (s *InteractiveSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	uuids := []string{
		"33333333-3333-3333-3333-333333333333", // password
		"44444444-4444-4444-4444-444444444444", // password key ID
		"55555555-5555-5555-5555-555555555555", // role assignment ID
	}
	s.newUUID = func() (utils.UUID, error) {
		uuid, err := utils.UUIDFromString(uuids[0])
		if err != nil {
			return utils.UUID{}, err
		}
		uuids = uuids[1:]
		return uuid, nil
	}
	s.clock = testclock.NewClock(clockStartTime())
}

func (s *InteractiveSuite) TestInteractive(c *gc.C) {
	var requests []*http.Request
	spc := azureauth.ServicePrincipalCreator{
		Sender: &azuretesting.Senders{
			oauthConfigSender(),
			deviceCodeSender(),
			tokenSender(), // CheckForUserCompletion returns a token.

			// Token.Refresh returns a token. We do this
			// twice: once for ARM, and once for AAD.
			tokenSender(),
			tokenSender(),

			currentUserSender(),
			createServicePrincipalSender(),
			roleDefinitionListSender(),
			roleAssignmentSender(),
		},
		RequestInspector: azuretesting.RequestRecorder(&requests),
		Clock:            s.clock,
		NewUUID:          s.newUUID,
	}

	var stderr bytes.Buffer
	subscriptionId := "22222222-2222-2222-2222-222222222222"
	sdkCtx := context.Background()

	appId, password, err := spc.InteractiveCreate(sdkCtx, &stderr, azureauth.ServicePrincipalParams{
		GraphEndpoint:             "https://graph.invalid",
		GraphResourceId:           "https://graph.invalid",
		ResourceManagerEndpoint:   "https://arm.invalid",
		ResourceManagerResourceId: "https://management.core.invalid/",
		SubscriptionId:            subscriptionId,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appId, gc.Equals, "cbb548f1-5039-4836-af0b-727e8571f6a9")
	c.Assert(password, gc.Equals, "33333333-3333-3333-3333-333333333333")
	c.Assert(stderr.String(), gc.Equals, `
Initiating interactive authentication.

open your browser, etc.

Authenticated as "Foo Bar".
`[1:])

	c.Assert(requests, gc.HasLen, 9)
	c.Check(requests[0].URL.Path, gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222")
	c.Check(requests[1].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/oauth2/devicecode")
	c.Check(requests[2].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/oauth2/token")
	c.Check(requests[3].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/oauth2/token")
	c.Check(requests[4].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/oauth2/token")
	c.Check(requests[5].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/me")
	c.Check(requests[6].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/servicePrincipals")
	c.Check(requests[7].URL.Path, gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222/providers/Microsoft.Authorization/roleDefinitions")
	c.Check(requests[8].URL.Path, gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222/providers/Microsoft.Authorization/roleAssignments/55555555-5555-5555-5555-555555555555")

	// The service principal creation includes the password. Check that the
	// password returned from the function is the same as the one set in the
	// request.
	var params graphrbac.ServicePrincipalCreateParameters
	err = json.NewDecoder(requests[6].Body).Decode(&params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert((*params.PasswordCredentials), gc.HasLen, 1)
	assertPasswordCredential(c, (*params.PasswordCredentials)[0])
}

func assertPasswordCredential(c *gc.C, cred graphrbac.PasswordCredential) {
	var startDate, endDate time.Time
	if cred.StartDate != nil {
		startDate = cred.StartDate.Time
	}
	if cred.EndDate != nil {
		endDate = cred.EndDate.Time
	}
	c.Assert(startDate, gc.Equals, clockStartTime())
	c.Assert(endDate.Sub(startDate), gc.Equals, 365*24*time.Hour)

	cred.StartDate = nil
	cred.EndDate = nil
	c.Assert(cred, jc.DeepEquals, graphrbac.PasswordCredential{
		CustomKeyIdentifier: to.ByteSlicePtr([]byte("juju-20160919")),
		KeyID:               to.StringPtr("44444444-4444-4444-4444-444444444444"),
		Value:               to.StringPtr("33333333-3333-3333-3333-333333333333"),
	})
}

func (s *InteractiveSuite) TestInteractiveRoleAssignmentAlreadyExists(c *gc.C) {
	var requests []*http.Request
	spc := azureauth.ServicePrincipalCreator{
		Sender: &azuretesting.Senders{
			oauthConfigSender(),
			deviceCodeSender(),
			tokenSender(),
			tokenSender(),
			tokenSender(),
			currentUserSender(),
			createServicePrincipalSender(),
			roleDefinitionListSender(),
			roleAssignmentAlreadyExistsSender(),
		},
		RequestInspector: azuretesting.RequestRecorder(&requests),
		Clock:            s.clock,
		NewUUID:          s.newUUID,
	}
	sdkCtx := context.Background()
	_, _, err := spc.InteractiveCreate(sdkCtx, ioutil.Discard, azureauth.ServicePrincipalParams{
		GraphEndpoint:             "https://graph.invalid",
		GraphResourceId:           "https://graph.invalid",
		ResourceManagerEndpoint:   "https://arm.invalid",
		ResourceManagerResourceId: "https://management.core.invalid/",
		SubscriptionId:            "22222222-2222-2222-2222-222222222222",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InteractiveSuite) TestInteractiveServicePrincipalAlreadyExists(c *gc.C) {
	s.testInteractiveServicePrincipalAlreadyExists(c, false)
}

func (s *InteractiveSuite) TestInteractiveServicePrincipalAlreadyExistsWithUTF8BOM(c *gc.C) {
	// We have observed that Azure sometimes responds with UTF-8 BOMs in
	// JSON-encoded responses. Go's JSON decoder does not like this, so
	// we have to strip it off. See:
	//     https://bugs.launchpad.net/juju/+bug/1657448
	s.testInteractiveServicePrincipalAlreadyExists(c, true)
}

func (s *InteractiveSuite) testInteractiveServicePrincipalAlreadyExists(c *gc.C, withUTF8BOM bool) {
	var requests []*http.Request
	spc := azureauth.ServicePrincipalCreator{
		Sender: &azuretesting.Senders{
			oauthConfigSender(),
			deviceCodeSender(),
			tokenSender(),
			tokenSender(),
			tokenSender(),
			currentUserSender(),
			createServicePrincipalAlreadyExistsSender(withUTF8BOM),
			servicePrincipalListSender(),
			passwordCredentialsListSender(),
			updatePasswordCredentialsSender(),
			roleDefinitionListSender(),
			roleAssignmentAlreadyExistsSender(),
		},
		RequestInspector: azuretesting.RequestRecorder(&requests),
		Clock:            s.clock,
		NewUUID:          s.newUUID,
	}
	sdkCtx := context.Background()
	_, password, err := spc.InteractiveCreate(sdkCtx, ioutil.Discard, azureauth.ServicePrincipalParams{
		GraphEndpoint:             "https://graph.invalid",
		GraphResourceId:           "https://graph.invalid",
		ResourceManagerEndpoint:   "https://arm.invalid",
		ResourceManagerResourceId: "https://management.core.invalid/",
		SubscriptionId:            "22222222-2222-2222-2222-222222222222",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(password, gc.Equals, "33333333-3333-3333-3333-333333333333")

	c.Assert(requests, gc.HasLen, 12)
	c.Check(requests[0].URL.Path, gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222")
	c.Check(requests[1].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/oauth2/devicecode")
	c.Check(requests[2].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/oauth2/token")
	c.Check(requests[3].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/oauth2/token")
	c.Check(requests[4].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/oauth2/token")
	c.Check(requests[5].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/me")
	c.Check(requests[6].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/servicePrincipals")                                  // create
	c.Check(requests[7].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/servicePrincipals")                                  // list
	c.Check(requests[8].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/servicePrincipals/sp-object-id/passwordCredentials") // list
	c.Check(requests[9].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/servicePrincipals/sp-object-id/passwordCredentials") // update
	c.Check(requests[10].URL.Path, gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222/providers/Microsoft.Authorization/roleDefinitions")
	c.Check(requests[11].URL.Path, gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222/providers/Microsoft.Authorization/roleAssignments/55555555-5555-5555-5555-555555555555")

	// Make sure that we don't wipe existing password credentials, and that
	// the new password credential matches the one returned from the
	// function.
	var params graphrbac.PasswordCredentialsUpdateParameters
	err = json.NewDecoder(requests[9].Body).Decode(&params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert((*params.Value), gc.HasLen, 2)
	c.Assert((*params.Value)[0], jc.DeepEquals, graphrbac.PasswordCredential{
		KeyID: to.StringPtr("password-credential-key-id"),
	})
	assertPasswordCredential(c, (*params.Value)[1])
}

func (s *InteractiveSuite) TestInteractiveServicePrincipalApplicationNotExist(c *gc.C) {
	s.testInteractiveRetriesCreateServicePrincipal(c, createServicePrincipalNotExistSender())
}

func (s *InteractiveSuite) TestInteractiveServicePrincipalApplicationNotReference(c *gc.C) {
	s.testInteractiveRetriesCreateServicePrincipal(c, createServicePrincipalNotReferenceSender())
}

func (s *InteractiveSuite) testInteractiveRetriesCreateServicePrincipal(c *gc.C, errorSender autorest.Sender) {
	var requests []*http.Request
	spc := azureauth.ServicePrincipalCreator{
		Sender: &azuretesting.Senders{
			oauthConfigSender(),
			deviceCodeSender(),
			tokenSender(),
			tokenSender(),
			tokenSender(),
			currentUserSender(),
			errorSender,
			createServicePrincipalSender(),
			roleDefinitionListSender(),
			roleAssignmentAlreadyExistsSender(),
		},
		RequestInspector: azuretesting.RequestRecorder(&requests),
		Clock: &testclock.AutoAdvancingClock{
			Clock:   s.clock,
			Advance: s.clock.Advance,
		},
		NewUUID: s.newUUID,
	}
	sdkCtx := context.Background()
	_, password, err := spc.InteractiveCreate(sdkCtx, ioutil.Discard, azureauth.ServicePrincipalParams{
		GraphEndpoint:             "https://graph.invalid",
		GraphResourceId:           "https://graph.invalid",
		ResourceManagerEndpoint:   "https://arm.invalid",
		ResourceManagerResourceId: "https://management.core.invalid/",
		SubscriptionId:            "22222222-2222-2222-2222-222222222222",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(password, gc.Equals, "33333333-3333-3333-3333-333333333333")

	c.Assert(requests, gc.HasLen, 10)
	c.Check(requests[0].URL.Path, gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222")
	c.Check(requests[1].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/oauth2/devicecode")
	c.Check(requests[2].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/oauth2/token")
	c.Check(requests[3].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/oauth2/token")
	c.Check(requests[4].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/oauth2/token")
	c.Check(requests[5].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/me")
	c.Check(requests[6].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/servicePrincipals") // create
	c.Check(requests[7].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/servicePrincipals") // create
	c.Check(requests[8].URL.Path, gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222/providers/Microsoft.Authorization/roleDefinitions")
	c.Check(requests[9].URL.Path, gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222/providers/Microsoft.Authorization/roleAssignments/55555555-5555-5555-5555-555555555555")
}

func (s *InteractiveSuite) TestInteractiveRetriesRoleAssignment(c *gc.C) {
	var requests []*http.Request
	spc := azureauth.ServicePrincipalCreator{
		Sender: &azuretesting.Senders{
			oauthConfigSender(),
			deviceCodeSender(),
			tokenSender(),
			tokenSender(),
			tokenSender(),
			currentUserSender(),
			createServicePrincipalSender(),
			roleDefinitionListSender(),
			roleAssignmentPrincipalNotExistSender(),
			roleAssignmentSender(),
		},
		RequestInspector: azuretesting.RequestRecorder(&requests),
		Clock: &testclock.AutoAdvancingClock{
			Clock:   s.clock,
			Advance: s.clock.Advance,
		},
		NewUUID: s.newUUID,
	}
	sdkCtx := context.Background()
	_, password, err := spc.InteractiveCreate(sdkCtx, ioutil.Discard, azureauth.ServicePrincipalParams{
		GraphEndpoint:             "https://graph.invalid",
		GraphResourceId:           "https://graph.invalid",
		ResourceManagerEndpoint:   "https://arm.invalid",
		ResourceManagerResourceId: "https://management.core.invalid/",
		SubscriptionId:            "22222222-2222-2222-2222-222222222222",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(password, gc.Equals, "33333333-3333-3333-3333-333333333333")

	c.Assert(requests, gc.HasLen, 10)
	c.Check(requests[0].URL.Path, gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222")
	c.Check(requests[1].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/oauth2/devicecode")
	c.Check(requests[2].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/oauth2/token")
	c.Check(requests[3].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/oauth2/token")
	c.Check(requests[4].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/oauth2/token")
	c.Check(requests[5].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/me")
	c.Check(requests[6].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/servicePrincipals") // create
	c.Check(requests[7].URL.Path, gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222/providers/Microsoft.Authorization/roleDefinitions")
	c.Check(requests[8].URL.Path, gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222/providers/Microsoft.Authorization/roleAssignments/55555555-5555-5555-5555-555555555555")
	c.Check(requests[9].URL.Path, gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222/providers/Microsoft.Authorization/roleAssignments/55555555-5555-5555-5555-555555555555")
}
