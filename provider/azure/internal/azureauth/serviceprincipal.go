// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/services/authorization/mgmt/2015-07-01/authorization"
	"github.com/Azure/azure-sdk-for-go/services/graphrbac/1.6/graphrbac"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/date"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/retry"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/provider/azure/internal/tracing"
	"github.com/juju/juju/provider/azure/internal/useragent"
)

var logger = loggo.GetLogger("juju.provider.azure.internal.azureauth")

const (
	// jujuApplicationId is the ID of the Azure application that we use
	// for interactive authentication. When the user logs in, a service
	// principal will be created in their Active Directory tenant for
	// the application.
	jujuApplicationId = "60a04dc9-1857-425f-8076-5ba81ca53d66"

	// JujuApplicationObjectId is the ObjectId of the Azure application.
	jujuApplicationObjectId = "8b744cea-179d-4a73-9dff-20d52126030a"

	// passwordExpiryDuration is how long the application password we
	// set will remain valid.
	passwordExpiryDuration = 365 * 24 * time.Hour
)

// MaybeJujuApplicationObjectID returns the Juju Application Object ID
// if the passed in application ID is the Juju Enterprise App.
func MaybeJujuApplicationObjectID(appID string) (string, error) {
	if appID == jujuApplicationId {
		return jujuApplicationObjectId, nil
	}
	return "", errors.Errorf("unexpected application ID %q", appID)
}

type ServicePrincipalParams struct {
	// GraphEndpoint of the Azure graph API.
	GraphEndpoint string

	// GraphResourceId is the resource ID of the graph API that is
	// used when acquiring access tokens.
	GraphResourceId string

	// GraphTokenProvider is the authorization needed to contact the
	// Azure graph API.
	GraphTokenProvider adal.OAuthTokenProvider

	// AuthEndpoint is the endpoint for auth requests.
	AuthEndpoint string

	// ResourceManagerEndpoint is the endpoint of the azure resource
	// manager API.
	ResourceManagerEndpoint string

	// ResourceManagerResourceId is the resource ID of the resource manager  API that is
	// used when acquiring access tokens.
	ResourceManagerResourceId string

	// ResourceManagerTokenProvider is the authorization needed to
	// contact the Azure resource manager API.
	ResourceManagerTokenProvider adal.OAuthTokenProvider

	// SubscriptionId is the subscription ID of the account creating
	// the service principal.
	SubscriptionId string

	// TenantId is the tenant that the account creating the service
	// principal belongs to.
	TenantId string
}

func (p ServicePrincipalParams) directoryClient(sender autorest.Sender, requestInspector autorest.PrepareDecorator) graphrbac.BaseClient {
	directoryClient := graphrbac.NewWithBaseURI(p.GraphEndpoint, p.TenantId)
	directoryClient.Authorizer = autorest.NewBearerAuthorizer(p.GraphTokenProvider)
	directoryClient.Sender = sender
	setClientInspectors(&directoryClient.Client, requestInspector, CheckForGraphError, "azure.directory")
	return directoryClient
}

func (p ServicePrincipalParams) authorizationClient(sender autorest.Sender, requestInspector autorest.PrepareDecorator) authorization.BaseClient {
	authorizationClient := authorization.NewWithBaseURI(p.ResourceManagerEndpoint, p.SubscriptionId)
	useragent.UpdateClient(&authorizationClient.Client)
	authorizationClient.Authorizer = autorest.NewBearerAuthorizer(p.ResourceManagerTokenProvider)
	authorizationClient.Sender = sender
	setClientInspectors(&authorizationClient.Client, requestInspector, nil, "azure.authorization")
	return authorizationClient
}

func setClientInspectors(
	client *autorest.Client,
	requestInspector autorest.PrepareDecorator,
	responseInspector autorest.RespondDecorator,
	loggingModule string,
) {
	logger := loggo.GetLogger(loggingModule)
	client.ResponseInspector = tracing.RespondDecorator(logger)
	if responseInspector != nil {
		tracer := client.ResponseInspector
		client.ResponseInspector = func(r autorest.Responder) autorest.Responder {
			r = tracer(r)
			r = responseInspector(r)
			return r
		}
	}
	client.RequestInspector = tracing.PrepareDecorator(logger)
	if requestInspector != nil {
		tracer := client.RequestInspector
		client.RequestInspector = func(p autorest.Preparer) autorest.Preparer {
			p = tracer(p)
			p = requestInspector(p)
			return p
		}
	}
}

type ServicePrincipalCreator struct {
	Sender           autorest.Sender
	RequestInspector autorest.PrepareDecorator
	Clock            clock.Clock
	NewUUID          func() (utils.UUID, error)
}

// InteractiveCreate creates a new ServicePrincipal by performing device
// code authentication with Azure AD and creating the service principal
// using the credentials that are obtained. Only GraphEndpoint,
// GraphResourceId, ResourceManagerEndpoint, ResourceManagerResourceId
// and SubscriptionId need to be specified in params, the other values
// will be derived.
func (c *ServicePrincipalCreator) InteractiveCreate(sdkCtx context.Context, stderr io.Writer, params ServicePrincipalParams) (appid, spid, password string, _ error) {
	adEndpoint := params.AuthEndpoint
	if adEndpoint == "" {
		adEndpoint = "https://login.windows.net"
	}
	oauthConfig, err := adal.NewOAuthConfig(adEndpoint, params.TenantId)
	if err != nil {
		return "", "", "", errors.Annotate(err, "getting OAuth configuration")
	}

	client := autorest.NewClientWithUserAgent("")
	useragent.UpdateClient(&client)
	client.Sender = c.Sender
	setClientInspectors(&client, c.RequestInspector, nil, "azure.autorest")

	// Perform the interactive authentication. The user will be prompted to
	// open a URL and input a device code, after which they will have to
	// enter their username and password if they are not already
	// authenticated with Azure.
	fmt.Fprintln(stderr, "Initiating interactive authentication.")
	fmt.Fprintln(stderr)
	clientId := jujuApplicationId
	deviceCode, err := adal.InitiateDeviceAuthWithContext(sdkCtx, &client, *oauthConfig, clientId, params.ResourceManagerResourceId)
	if err != nil {
		return "", "", "", errors.Annotate(err, "initiating interactive authentication")
	}
	fmt.Fprintln(stderr, toValue(deviceCode.Message)+"\n")
	token, err := adal.WaitForUserCompletionWithContext(sdkCtx, &client, deviceCode)
	if err != nil {
		return "", "", "", errors.Annotate(err, "waiting for interactive authentication to completed")
	}

	// Create service principal tokens that we can use to authorize API
	// requests to Active Directory and Resource Manager. These tokens
	// are only valid for a short amount of time, so we must create a
	// service principal password that can be used to obtain new tokens.
	armSpt, err := adal.NewServicePrincipalTokenFromManualToken(*oauthConfig, clientId, params.ResourceManagerResourceId, *token)
	if err != nil {
		return "", "", "", errors.Annotate(err, "creating temporary ARM service principal token")
	}
	armSpt.SetSender(&client)
	if err := armSpt.Refresh(); err != nil {
		return "", "", "", errors.Trace(err)
	}

	// The application requires permissions for both ARM and AD, so we
	// can use the token for both APIs.
	graphToken := armSpt.Token()
	graphToken.Resource = params.GraphResourceId
	graphSpt, err := adal.NewServicePrincipalTokenFromManualToken(*oauthConfig, clientId, params.GraphResourceId, graphToken)
	if err != nil {
		return "", "", "", errors.Annotate(err, "creating temporary Graph service principal token")
	}
	graphSpt.SetSender(&client)
	if err := graphSpt.Refresh(); err != nil {
		return "", "", "", errors.Trace(err)
	}
	params.GraphTokenProvider = graphSpt
	params.ResourceManagerTokenProvider = armSpt

	userObject, err := graphrbac.SignedInUserClient{params.directoryClient(c.Sender, c.RequestInspector)}.Get(sdkCtx)
	if err != nil {
		return "", "", "", errors.Trace(err)
	}
	fmt.Fprintf(stderr, "Authenticated as %q.\n", toValue(userObject.DisplayName))

	return c.Create(sdkCtx, params)
}

// Create creates a new service principal using the values specified in params.
func (c *ServicePrincipalCreator) Create(sdkCtx context.Context, params ServicePrincipalParams) (appid, spid, password string, _ error) {
	servicePrincipalObjectId, password, err := c.createOrUpdateServicePrincipal(sdkCtx, params)
	if err != nil {
		return "", "", "", errors.Trace(err)
	}
	if err := c.createRoleAssignment(sdkCtx, params, servicePrincipalObjectId); err != nil {
		return "", "", "", errors.Trace(err)
	}
	return jujuApplicationId, servicePrincipalObjectId, password, nil
}

func (c *ServicePrincipalCreator) createOrUpdateServicePrincipal(sdkCtx context.Context, params ServicePrincipalParams) (servicePrincipalObjectId, password string, _ error) {
	passwordCredential, err := c.preparePasswordCredential()
	if err != nil {
		return "", "", errors.Annotate(err, "preparing password credential")
	}

	// Attempt to create the service principal. When the user
	// authenticates, Azure will replicate the application
	// into the user's AAD. This happens asynchronously, so
	// it may not exist by the time we try to create the
	// service principal; thus, we retry until it exists. The
	// error checking is based on the logic in azure-cli's
	// create_service_principal_for_rbac.
	client := graphrbac.ServicePrincipalsClient{params.directoryClient(c.Sender, c.RequestInspector)}
	var servicePrincipal graphrbac.ServicePrincipal
	createServicePrincipal := func() error {
		var err error
		creds := []graphrbac.PasswordCredential{passwordCredential}
		servicePrincipal, err = client.Create(
			sdkCtx,
			graphrbac.ServicePrincipalCreateParameters{
				AppID:               to.Ptr(jujuApplicationId),
				AccountEnabled:      to.Ptr(true),
				PasswordCredentials: &creds,
			},
		)
		return err
	}
	retryArgs := retry.CallArgs{
		Func: createServicePrincipal,
		IsFatalError: func(err error) bool {
			if ge := AsGraphError(err); ge != nil {
				if strings.Contains(ge.Message(), " does not reference ") || strings.Contains(ge.Message(), " does not exist ") {
					// The application doesn't exist yet, retry later.
					return false
				}
			}
			return true
		},
		Clock:       c.clock(),
		Delay:       5 * time.Second,
		MaxDuration: time.Minute,
	}
	if err := retry.Call(retryArgs); err != nil {
		if !isMultipleObjectsWithSameKeyValueErr(err) {
			return "", "", errors.Annotate(err, "creating service principal")
		}
		// The service principal already exists, so we'll fall out
		// and update the service principal's password credentials.
	} else {
		// The service principal was created successfully, with the
		// requested password credential.
		return toValue(servicePrincipal.ObjectID), toValue(passwordCredential.Value), nil
	}

	// The service principal already exists, so we need to query
	// its object ID, and fetch the existing password credentials
	// to update.
	servicePrincipal, err = getServicePrincipal(sdkCtx, client, jujuApplicationId)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	if err := addServicePrincipalPasswordCredential(
		sdkCtx,
		client,
		toValue(servicePrincipal.ObjectID),
		passwordCredential,
	); err != nil {
		return "", "", errors.Annotate(err, "updating password credentials")
	}
	return toValue(servicePrincipal.ObjectID), toValue(passwordCredential.Value), nil
}

func isMultipleObjectsWithSameKeyValueErr(err error) bool {
	if ge := AsGraphError(err); ge != nil {
		return ge.Code() == "Request_MultipleObjectsWithSameKeyValue"
	}
	return false
}

func (c *ServicePrincipalCreator) preparePasswordCredential() (graphrbac.PasswordCredential, error) {
	password, err := c.newUUID()
	if err != nil {
		return graphrbac.PasswordCredential{}, errors.Annotate(err, "generating password")
	}
	passwordKeyUUID, err := c.newUUID()
	if err != nil {
		return graphrbac.PasswordCredential{}, errors.Annotate(err, "generating password key ID")
	}
	startDate := c.clock().Now().UTC()
	endDate := startDate.Add(passwordExpiryDuration)
	return graphrbac.PasswordCredential{
		CustomKeyIdentifier: to.Ptr([]byte("juju-" + startDate.Format("20060102"))),
		KeyID:               to.Ptr(passwordKeyUUID.String()),
		Value:               to.Ptr(password.String()),
		StartDate:           &date.Time{startDate},
		EndDate:             &date.Time{endDate},
	}, nil
}

func addServicePrincipalPasswordCredential(
	sdkCtx context.Context,
	client graphrbac.ServicePrincipalsClient,
	servicePrincipalObjectId string,
	passwordCredential graphrbac.PasswordCredential,
) error {
	existing, err := client.ListPasswordCredentials(sdkCtx, servicePrincipalObjectId)
	if err != nil {
		return errors.Trace(err)
	}
	var existingValues []graphrbac.PasswordCredential
	if existing.Value != nil {
		existingValues = *existing.Value
	}
	passwordCredentials := append(existingValues, passwordCredential)
	_, err = client.UpdatePasswordCredentials(
		sdkCtx,
		servicePrincipalObjectId,
		graphrbac.PasswordCredentialsUpdateParameters{&passwordCredentials},
	)
	return errors.Trace(err)
}

func getServicePrincipal(sdkCtx context.Context, client graphrbac.ServicePrincipalsClient, appID string) (graphrbac.ServicePrincipal, error) {
	filter := fmt.Sprintf(
		"appId eq '%s'",
		appID,
	)
	it, err := client.ListComplete(sdkCtx, filter)
	if err != nil {
		return graphrbac.ServicePrincipal{}, errors.Annotate(err, "listing service principals")
	}
	for it.NotDone() {
		sp := it.Value()
		if toValue(sp.AppID) == appID {
			return sp, nil
		}
		if err := it.NextWithContext(sdkCtx); err != nil {
			return graphrbac.ServicePrincipal{}, errors.Annotate(err, "listing service principals")
		}
	}
	return graphrbac.ServicePrincipal{}, errors.NotFoundf("service principal")
}

func (c *ServicePrincipalCreator) createRoleAssignment(sdkCtx context.Context, params ServicePrincipalParams, servicePrincipalObjectId string) error {
	client := params.authorizationClient(c.Sender, c.RequestInspector)
	// Find the role definition with the name "Owner".
	roleScope := path.Join("subscriptions", params.SubscriptionId)
	roleDefinitionsClient := authorization.RoleDefinitionsClient{client}
	result, err := roleDefinitionsClient.List(sdkCtx, roleScope, "roleName eq 'Owner'")
	if err != nil {
		return errors.Annotate(err, "listing role definitions")
	}
	ownerRoles := result.Values()
	if len(ownerRoles) == 0 {
		return errors.NotFoundf("Owner role definition")
	}
	roleDefinitionId := ownerRoles[0].ID

	// The UUID value for the role assignment name is unimportant. Azure
	// will prevent multiple role assignments for the same role definition
	// and principal pair.
	roleAssignmentUUID, err := c.newUUID()
	if err != nil {
		return errors.Annotate(err, "generating role assignment ID")
	}
	roleAssignmentsClient := authorization.RoleAssignmentsClient{client}
	roleAssignmentName := roleAssignmentUUID.String()
	retryArgs := retry.CallArgs{
		Func: func() error {
			_, err := roleAssignmentsClient.Create(
				sdkCtx,
				roleScope, roleAssignmentName,
				authorization.RoleAssignmentCreateParameters{
					Properties: &authorization.RoleAssignmentProperties{
						RoleDefinitionID: roleDefinitionId,
						PrincipalID:      to.Ptr(servicePrincipalObjectId),
					},
				},
			)
			return err
		},
		IsFatalError: func(err error) bool {
			serviceErr, ok := serviceError(err)
			if ok && strings.Contains(serviceErr.Message, " does not exist in the directory ") {
				// The service principal doesn't exist yet, retry later.
				return false
			}
			return true
		},
		Clock:       c.clock(),
		Delay:       5 * time.Second,
		MaxDuration: time.Minute,
	}
	if err := retry.Call(retryArgs); err != nil {
		if err, ok := serviceError(err); ok {
			const serviceErrorCodeRoleAssignmentExists = "RoleAssignmentExists"
			if err.Code == serviceErrorCodeRoleAssignmentExists {
				return nil
			}
		}
		return errors.Annotate(err, "creating role assignment")
	}
	return nil
}

func (c *ServicePrincipalCreator) clock() clock.Clock {
	if c.Clock == nil {
		return clock.WallClock
	}
	return c.Clock
}

func (c *ServicePrincipalCreator) newUUID() (utils.UUID, error) {
	if c.NewUUID == nil {
		return utils.NewUUID()
	}
	return c.NewUUID()
}

// serviceError returns the *azure.ServiceError underlying the
// supplied error, if any, and a bool indicating whether one
// was found.
func serviceError(err error) (*azure.ServiceError, bool) {
	if err == nil {
		return nil, false
	}
	err = errors.Cause(err)
	if d, ok := err.(autorest.DetailedError); ok {
		err = errors.Cause(d.Original)
	}
	if se, ok := err.(*azure.ServiceError); ok {
		return se, true
	}
	if r, ok := err.(*azure.RequestError); ok {
		return r.ServiceError, true
	}
	// The error Azure gives us back can also be a struct
	// not a pointer.
	if se, ok := err.(azure.ServiceError); ok {
		return &se, true
	}
	if r, ok := err.(azure.RequestError); ok {
		return r.ServiceError, true
	}
	return nil, false
}

func toValue[T any](v *T) T {
	if v == nil {
		return *new(T)
	}
	return *v
}
