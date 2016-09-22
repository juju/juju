// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/Azure/azure-sdk-for-go/arm/authorization"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/mocks"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/azure/internal/ad"
	"github.com/juju/juju/provider/azure/internal/azureauth"
	"github.com/juju/juju/provider/azure/internal/azuretesting"
)

func clockStartTime() time.Time {
	t, _ := time.Parse("2006-Jan-02 3:04am", "2016-Sep-19 9:47am")
	return t
}

type InteractiveSuite struct {
	testing.IsolationSuite
	clock   *testing.Clock
	newUUID func() (utils.UUID, error)
}

var _ = gc.Suite(&InteractiveSuite{})

func deviceCodeSender() autorest.Sender {
	return azuretesting.NewSenderWithValue(azure.DeviceCode{
		DeviceCode: to.StringPtr("device-code"),
		Interval:   to.Int64Ptr(1), // 1 second between polls
		Message:    to.StringPtr("open your browser, etc."),
	})
}

func tokenSender() autorest.Sender {
	return azuretesting.NewSenderWithValue(azure.Token{
		RefreshToken: "refresh-token",
		ExpiresOn:    fmt.Sprint(time.Now().Add(time.Hour).Unix()),
	})
}

func passwordCredentialsListSender() autorest.Sender {
	return azuretesting.NewSenderWithValue(ad.PasswordCredentialsListResult{
		Value: []ad.PasswordCredential{{
			KeyId: "password-credential-key-id",
		}},
	})
}

func updatePasswordCredentialsSender() autorest.Sender {
	sender := mocks.NewSender()
	sender.AppendResponse(mocks.NewResponseWithStatus("", http.StatusNoContent))
	return sender
}

func currentUserSender() autorest.Sender {
	return azuretesting.NewSenderWithValue(ad.AADObject{
		DisplayName: "Foo Bar",
	})
}

func createServicePrincipalSender() autorest.Sender {
	return azuretesting.NewSenderWithValue(ad.ServicePrincipal{
		ApplicationID: "cbb548f1-5039-4836-af0b-727e8571f6a9",
		ObjectID:      "sp-object-id",
	})
}

func createServicePrincipalAlreadyExistsSender() autorest.Sender {
	sender := mocks.NewSender()
	body := mocks.NewBody(`{"odata.error":{"code":"Request_MultipleObjectsWithSameKeyValue"}}`)
	sender.AppendResponse(mocks.NewResponseWithBodyAndStatus(body, http.StatusConflict, ""))
	return sender
}

func servicePrincipalListSender() autorest.Sender {
	return azuretesting.NewSenderWithValue(ad.ServicePrincipalListResult{
		Value: []ad.ServicePrincipal{{
			ApplicationID: "cbb548f1-5039-4836-af0b-727e8571f6a9",
			ObjectID:      "sp-object-id",
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
	body := mocks.NewBody(`{"error":{"code":"RoleAssignmentExists"}}`)
	sender.AppendResponse(mocks.NewResponseWithBodyAndStatus(body, http.StatusConflict, ""))
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
	s.clock = testing.NewClock(clockStartTime())
}

func (s *InteractiveSuite) TestInteractive(c *gc.C) {

	var requests []*http.Request
	senders := azuretesting.Senders{
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
	}

	var stderr bytes.Buffer
	subscriptionId := "22222222-2222-2222-2222-222222222222"
	appId, password, err := azureauth.InteractiveCreateServicePrincipal(
		&stderr,
		&senders,
		azuretesting.RequestRecorder(&requests),
		"https://arm.invalid",
		"https://graph.invalid",
		subscriptionId,
		s.clock,
		s.newUUID,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appId, gc.Equals, "cbb548f1-5039-4836-af0b-727e8571f6a9")
	c.Assert(password, gc.Equals, "33333333-3333-3333-3333-333333333333")
	c.Assert(stderr.String(), gc.Equals, `
Initiating interactive authentication.

open your browser, etc.

Authenticated as "Foo Bar".
Creating/updating service principal.
Assigning Owner role to service principal.
`[1:])

	// Token refreshes don't go through the inspectors.
	c.Assert(requests, gc.HasLen, 7)
	c.Check(requests[0].URL.Path, gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222")
	c.Check(requests[1].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/oauth2/devicecode")
	c.Check(requests[2].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/oauth2/token")
	c.Check(requests[3].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/me")
	c.Check(requests[4].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/servicePrincipals")
	c.Check(requests[5].URL.Path, gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222/providers/Microsoft.Authorization/roleDefinitions")
	c.Check(requests[6].URL.Path, gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222/providers/Microsoft.Authorization/roleAssignments/55555555-5555-5555-5555-555555555555")

	// The service principal creation includes the password. Check that the
	// password returned from the function is the same as the one set in the
	// request.
	var params ad.ServicePrincipalCreateParameters
	err = json.NewDecoder(requests[4].Body).Decode(&params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(params.PasswordCredentials, gc.HasLen, 1)
	assertPasswordCredential(c, params.PasswordCredentials[0])
}

func assertPasswordCredential(c *gc.C, cred ad.PasswordCredential) {
	startDate := cred.StartDate
	endDate := cred.EndDate
	c.Assert(startDate, gc.Equals, clockStartTime())
	c.Assert(endDate.Sub(startDate), gc.Equals, 365*24*time.Hour)

	cred.StartDate = time.Time{}
	cred.EndDate = time.Time{}
	c.Assert(cred, jc.DeepEquals, ad.PasswordCredential{
		CustomKeyIdentifier: []byte("juju-20160919"),
		KeyId:               "44444444-4444-4444-4444-444444444444",
		Value:               "33333333-3333-3333-3333-333333333333",
	})
}

func (s *InteractiveSuite) TestInteractiveRoleAssignmentAlreadyExists(c *gc.C) {
	var requests []*http.Request
	senders := azuretesting.Senders{
		oauthConfigSender(),
		deviceCodeSender(),
		tokenSender(),
		tokenSender(),
		tokenSender(),
		currentUserSender(),
		createServicePrincipalSender(),
		roleDefinitionListSender(),
		roleAssignmentAlreadyExistsSender(),
	}
	_, _, err := azureauth.InteractiveCreateServicePrincipal(
		ioutil.Discard,
		&senders,
		azuretesting.RequestRecorder(&requests),
		"https://arm.invalid",
		"https://graph.invalid",
		"22222222-2222-2222-2222-222222222222",
		s.clock,
		s.newUUID,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InteractiveSuite) TestInteractiveServicePrincipalAlreadyExists(c *gc.C) {
	var requests []*http.Request
	senders := azuretesting.Senders{
		oauthConfigSender(),
		deviceCodeSender(),
		tokenSender(),
		tokenSender(),
		tokenSender(),
		currentUserSender(),
		createServicePrincipalAlreadyExistsSender(),
		servicePrincipalListSender(),
		passwordCredentialsListSender(),
		updatePasswordCredentialsSender(),
		roleDefinitionListSender(),
		roleAssignmentAlreadyExistsSender(),
	}
	_, password, err := azureauth.InteractiveCreateServicePrincipal(
		ioutil.Discard,
		&senders,
		azuretesting.RequestRecorder(&requests),
		"https://arm.invalid",
		"https://graph.invalid",
		"22222222-2222-2222-2222-222222222222",
		s.clock,
		s.newUUID,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(password, gc.Equals, "33333333-3333-3333-3333-333333333333")

	c.Assert(requests, gc.HasLen, 10)
	c.Check(requests[0].URL.Path, gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222")
	c.Check(requests[1].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/oauth2/devicecode")
	c.Check(requests[2].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/oauth2/token")
	c.Check(requests[3].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/me")
	c.Check(requests[4].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/servicePrincipals")                                  // create
	c.Check(requests[5].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/servicePrincipals")                                  // list
	c.Check(requests[6].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/servicePrincipals/sp-object-id/passwordCredentials") // list
	c.Check(requests[7].URL.Path, gc.Equals, "/11111111-1111-1111-1111-111111111111/servicePrincipals/sp-object-id/passwordCredentials") // update
	c.Check(requests[8].URL.Path, gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222/providers/Microsoft.Authorization/roleDefinitions")
	c.Check(requests[9].URL.Path, gc.Equals, "/subscriptions/22222222-2222-2222-2222-222222222222/providers/Microsoft.Authorization/roleAssignments/55555555-5555-5555-5555-555555555555")

	// Make sure that we don't wipe existing password credentials, and that
	// the new password credential matches the one returned from the
	// function.
	var params ad.PasswordCredentialsUpdateParameters
	err = json.NewDecoder(requests[7].Body).Decode(&params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(params.Value, gc.HasLen, 2)
	c.Assert(params.Value[0], jc.DeepEquals, ad.PasswordCredential{
		KeyId:     "password-credential-key-id",
		StartDate: time.Time{}.UTC(),
		EndDate:   time.Time{}.UTC(),
	})
	assertPasswordCredential(c, params.Value[1])
}
