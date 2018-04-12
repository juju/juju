// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth

import (
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/arm/authorization"
	"github.com/Azure/azure-sdk-for-go/arm/resources/subscriptions"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/retry"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/provider/azure/internal/ad"
	"github.com/juju/juju/provider/azure/internal/errorutils"
	"github.com/juju/juju/provider/azure/internal/tracing"
	"github.com/juju/juju/provider/azure/internal/useragent"
)

var logger = loggo.GetLogger("juju.provider.azure.internal.azureauth")

const (
	// jujuApplicationId is the ID of the Azure application that we use
	// for interactive authentication. When the user logs in, a service
	// principal will be created in their Active Directory tenant for
	// the application.
	jujuApplicationId = "cbb548f1-5039-4836-af0b-727e8571f6a9"

	// passwordExpiryDuration is how long the application password we
	// set will remain valid.
	passwordExpiryDuration = 365 * 24 * time.Hour
)

type ServicePrincipalParams struct {
	// GraphEndpoint of the Azure graph API.
	GraphEndpoint string

	// GraphResourceId is the resource ID of the graph API that is
	// used when acquiring access tokens.
	GraphResourceId string

	// GraphAuthorizer is the authorization needed to contact the
	// Azure graph API.
	GraphAuthorizer autorest.Authorizer

	// ResourceManagerEndpoint is the endpoint of the azure resource
	// manager API.
	ResourceManagerEndpoint string

	// ResourceManagerResourceId is the resource ID of the resource mnager  API that is
	// used when acquiring access tokens.
	ResourceManagerResourceId string

	// ResourceManagerAuthorizer is the authorization needed to
	// contact the Azure resource manager API.
	ResourceManagerAuthorizer autorest.Authorizer

	// SubscriptionId is the subscription ID of the account creating
	// the service principal.
	SubscriptionId string

	// TenantId is the tenant that the account creating the service
	// principal belongs to.
	TenantId string
}

func (p ServicePrincipalParams) directoryClient(sender autorest.Sender, requestInspector autorest.PrepareDecorator) ad.ManagementClient {
	baseURL := p.GraphEndpoint
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	baseURL += p.TenantId
	directoryClient := ad.NewManagementClient(baseURL)
	directoryClient.Authorizer = p.GraphAuthorizer
	directoryClient.Sender = sender
	setClientInspectors(&directoryClient.Client, requestInspector, "azure.directory")
	return directoryClient
}

func (p ServicePrincipalParams) authorizationClient(sender autorest.Sender, requestInspector autorest.PrepareDecorator) authorization.ManagementClient {
	authorizationClient := authorization.NewWithBaseURI(p.ResourceManagerEndpoint, p.SubscriptionId)
	useragent.UpdateClient(&authorizationClient.Client)
	authorizationClient.Authorizer = p.ResourceManagerAuthorizer
	authorizationClient.Sender = sender
	setClientInspectors(&authorizationClient.Client, requestInspector, "azure.authorization")
	return authorizationClient
}

func setClientInspectors(
	client *autorest.Client,
	requestInspector autorest.PrepareDecorator,
	loggingModule string,
) {
	logger := loggo.GetLogger(loggingModule)
	client.ResponseInspector = tracing.RespondDecorator(logger)
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
func (c *ServicePrincipalCreator) InteractiveCreate(stderr io.Writer, params ServicePrincipalParams) (appid, password string, _ error) {
	subscriptionsClient := subscriptions.GroupClient{
		subscriptions.NewWithBaseURI(params.ResourceManagerEndpoint),
	}
	useragent.UpdateClient(&subscriptionsClient.Client)
	subscriptionsClient.Sender = c.Sender
	setClientInspectors(&subscriptionsClient.Client, c.RequestInspector, "azure.subscriptions")

	oauthConfig, tenantId, err := OAuthConfig(
		subscriptionsClient,
		params.ResourceManagerEndpoint,
		params.SubscriptionId,
	)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	client := autorest.NewClientWithUserAgent("")
	useragent.UpdateClient(&client)
	client.Sender = c.Sender
	setClientInspectors(&client, c.RequestInspector, "azure.autorest")

	// Perform the interactive authentication. The user will be prompted to
	// open a URL and input a device code, after which they will have to
	// enter their username and password if they are not already
	// authenticated with Azure.
	fmt.Fprintln(stderr, "Initiating interactive authentication.")
	fmt.Fprintln(stderr)
	clientId := jujuApplicationId
	deviceCode, err := adal.InitiateDeviceAuth(&client, *oauthConfig, clientId, params.ResourceManagerResourceId)
	if err != nil {
		return "", "", errors.Annotate(err, "initiating interactive authentication")
	}
	fmt.Fprintln(stderr, to.String(deviceCode.Message)+"\n")
	token, err := adal.WaitForUserCompletion(&client, deviceCode)
	if err != nil {
		return "", "", errors.Annotate(err, "waiting for interactive authentication to completed")
	}

	// Create service principal tokens that we can use to authorize API
	// requests to Active Directory and Resource Manager. These tokens
	// are only valid for a short amount of time, so we must create a
	// service principal password that can be used to obtain new tokens.
	armSpt, err := adal.NewServicePrincipalTokenFromManualToken(*oauthConfig, clientId, params.ResourceManagerResourceId, *token)
	if err != nil {
		return "", "", errors.Annotate(err, "creating temporary ARM service principal token")
	}
	armSpt.SetSender(&client)
	if err := armSpt.Refresh(); err != nil {
		return "", "", errors.Trace(err)
	}

	// The application requires permissions for both ARM and AD, so we
	// can use the token for both APIs.
	graphToken := armSpt.Token
	graphToken.Resource = params.GraphResourceId
	graphSpt, err := adal.NewServicePrincipalTokenFromManualToken(*oauthConfig, clientId, params.GraphResourceId, graphToken)
	if err != nil {
		return "", "", errors.Annotate(err, "creating temporary Graph service principal token")
	}
	graphSpt.SetSender(&client)
	if err := graphSpt.Refresh(); err != nil {
		return "", "", errors.Trace(err)
	}
	params.GraphAuthorizer = autorest.NewBearerAuthorizer(graphSpt)
	params.ResourceManagerAuthorizer = autorest.NewBearerAuthorizer(armSpt)
	params.TenantId = tenantId

	userObject, err := ad.UsersClient{params.directoryClient(c.Sender, c.RequestInspector)}.GetCurrentUser()
	if err != nil {
		return "", "", errors.Trace(err)
	}
	fmt.Fprintf(stderr, "Authenticated as %q.\n", userObject.DisplayName)

	return c.Create(params)
}

// Create creates a new service principal using the values specified in params.
func (c *ServicePrincipalCreator) Create(params ServicePrincipalParams) (appid, password string, _ error) {
	servicePrincipalObjectId, password, err := c.createOrUpdateServicePrincipal(params)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	if err := c.createRoleAssignment(params, servicePrincipalObjectId); err != nil {
		return "", "", errors.Trace(err)
	}

	return jujuApplicationId, password, nil
}

func (c *ServicePrincipalCreator) createOrUpdateServicePrincipal(params ServicePrincipalParams) (servicePrincipalObjectId, password string, _ error) {
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
	client := ad.ServicePrincipalsClient{params.directoryClient(c.Sender, c.RequestInspector)}
	var servicePrincipal ad.ServicePrincipal
	createServicePrincipal := func() error {
		var err error
		servicePrincipal, err = client.Create(
			ad.ServicePrincipalCreateParameters{
				ApplicationID:       jujuApplicationId,
				AccountEnabled:      true,
				PasswordCredentials: []ad.PasswordCredential{passwordCredential},
			},
			nil, // abort
		)
		return err
	}
	retryArgs := retry.CallArgs{
		Func: createServicePrincipal,
		IsFatalError: func(err error) bool {
			serviceErr, ok := errorutils.ServiceError(err)
			if ok && (strings.Contains(serviceErr.Message, " does not reference ") ||
				strings.Contains(serviceErr.Message, " does not exist ")) {
				// The application doesn't exist yet, retry later.
				return false
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
		return servicePrincipal.ObjectID, passwordCredential.Value, nil
	}

	// The service principal already exists, so we need to query
	// its object ID, and fetch the existing password credentials
	// to update.
	servicePrincipal, err = getServicePrincipal(client)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	if err := addServicePrincipalPasswordCredential(
		client, servicePrincipal.ObjectID,
		passwordCredential,
	); err != nil {
		return "", "", errors.Annotate(err, "updating password credentials")
	}
	return servicePrincipal.ObjectID, passwordCredential.Value, nil
}

func isMultipleObjectsWithSameKeyValueErr(err error) bool {
	if err, ok := errorutils.ServiceError(err); ok {
		return err.Code == "Request_MultipleObjectsWithSameKeyValue"
	}
	return false
}

func (c *ServicePrincipalCreator) preparePasswordCredential() (ad.PasswordCredential, error) {
	password, err := c.newUUID()
	if err != nil {
		return ad.PasswordCredential{}, errors.Annotate(err, "generating password")
	}
	passwordKeyUUID, err := c.newUUID()
	if err != nil {
		return ad.PasswordCredential{}, errors.Annotate(err, "generating password key ID")
	}
	startDate := c.clock().Now().UTC()
	endDate := startDate.Add(passwordExpiryDuration)
	return ad.PasswordCredential{
		CustomKeyIdentifier: []byte("juju-" + startDate.Format("20060102")),
		KeyId:               passwordKeyUUID.String(),
		Value:               password.String(),
		StartDate:           startDate,
		EndDate:             endDate,
	}, nil
}

func addServicePrincipalPasswordCredential(
	client ad.ServicePrincipalsClient,
	servicePrincipalObjectId string,
	passwordCredential ad.PasswordCredential,
) error {
	existing, err := client.ListPasswordCredentials(servicePrincipalObjectId)
	if err != nil {
		return errors.Trace(err)
	}
	passwordCredentials := append(existing.Value, passwordCredential)
	_, err = client.UpdatePasswordCredentials(
		servicePrincipalObjectId,
		ad.PasswordCredentialsUpdateParameters{passwordCredentials},
	)
	return errors.Trace(err)
}

func getServicePrincipal(client ad.ServicePrincipalsClient) (ad.ServicePrincipal, error) {
	// TODO(axw) filter by Service Principal Name (SPN).
	// It works without that, but the response is noisy.
	result, err := client.List("")
	if err != nil {
		return ad.ServicePrincipal{}, errors.Annotate(err, "listing service principals")
	}
	for _, sp := range result.Value {
		if sp.ApplicationID == jujuApplicationId {
			return sp, nil
		}
	}
	return ad.ServicePrincipal{}, errors.NotFoundf("service principal")
}

func (c *ServicePrincipalCreator) createRoleAssignment(params ServicePrincipalParams, servicePrincipalObjectId string) error {
	client := params.authorizationClient(c.Sender, c.RequestInspector)
	// Find the role definition with the name "Owner".
	roleScope := path.Join("subscriptions", params.SubscriptionId)
	roleDefinitionsClient := authorization.RoleDefinitionsClient{client}
	result, err := roleDefinitionsClient.List(roleScope, "roleName eq 'Owner'")
	if err != nil {
		return errors.Annotate(err, "listing role definitions")
	}
	if result.Value == nil || len(*result.Value) == 0 {
		return errors.NotFoundf("Owner role definition")
	}
	roleDefinitionId := (*result.Value)[0].ID

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
				roleScope, roleAssignmentName,
				authorization.RoleAssignmentCreateParameters{
					Properties: &authorization.RoleAssignmentProperties{
						RoleDefinitionID: roleDefinitionId,
						PrincipalID:      to.StringPtr(servicePrincipalObjectId),
					},
				},
			)
			return err
		},
		IsFatalError: func(err error) bool {
			serviceErr, ok := errorutils.ServiceError(err)
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
		if err, ok := errorutils.ServiceError(err); ok {
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
