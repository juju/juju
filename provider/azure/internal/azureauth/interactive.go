// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth

import (
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/arm/authorization"
	"github.com/Azure/azure-sdk-for-go/arm/resources/subscriptions"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
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

// InteractiveCreateServicePrincipalFunc is a function type for
// interactively creating service principals for a subscription.
type InteractiveCreateServicePrincipalFunc func(
	stderr io.Writer,
	sender autorest.Sender,
	requestInspector autorest.PrepareDecorator,
	resourceManagerEndpoint string,
	resourceManagerResourceId string,
	graphEndpoint string,
	subscriptionId string,
	clock clock.Clock,
	newUUID func() (utils.UUID, error),
) (appId, password string, _ error)

// InteractiveCreateServicePrincipal interactively creates service
// principals for a subscription.
func InteractiveCreateServicePrincipal(
	stderr io.Writer,
	sender autorest.Sender,
	requestInspector autorest.PrepareDecorator,
	resourceManagerEndpoint string,
	resourceManagerResourceId string,
	graphEndpoint string,
	subscriptionId string,
	clock clock.Clock,
	newUUID func() (utils.UUID, error),
) (appId, password string, _ error) {

	subscriptionsClient := subscriptions.Client{
		subscriptions.NewWithBaseURI(resourceManagerEndpoint),
	}
	useragent.UpdateClient(&subscriptionsClient.Client)
	subscriptionsClient.Sender = sender
	setClientInspectors(&subscriptionsClient.Client, requestInspector, "azure.subscriptions")

	oauthConfig, tenantId, err := OAuthConfig(
		subscriptionsClient,
		resourceManagerEndpoint,
		subscriptionId,
	)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	client := autorest.NewClientWithUserAgent(useragent.JujuPrefix())
	client.Sender = sender
	setClientInspectors(&client, requestInspector, "azure.autorest")

	// Perform the interactive authentication. The user will be prompted to
	// open a URL and input a device code, after which they will have to
	// enter their username and password if they are not already
	// authenticated with Azure.
	fmt.Fprintln(stderr, "Initiating interactive authentication.")
	fmt.Fprintln(stderr)
	clientId := jujuApplicationId
	deviceCode, err := azure.InitiateDeviceAuth(&client, *oauthConfig, clientId, resourceManagerResourceId)
	if err != nil {
		return "", "", errors.Annotate(err, "initiating interactive authentication")
	}
	fmt.Fprintln(stderr, to.String(deviceCode.Message)+"\n")
	token, err := azure.WaitForUserCompletion(&client, deviceCode)
	if err != nil {
		return "", "", errors.Annotate(err, "waiting for interactive authentication to completed")
	}

	// Create service principal tokens that we can use to authorize API
	// requests to Active Directory and Resource Manager. These tokens
	// are only valid for a short amount of time, so we must create a
	// service principal password that can be used to obtain new tokens.
	armSpt, err := azure.NewServicePrincipalTokenFromManualToken(*oauthConfig, clientId, resourceManagerResourceId, *token)
	if err != nil {
		return "", "", errors.Annotate(err, "creating temporary ARM service principal token")
	}
	armSpt.SetSender(&client)
	if err := armSpt.Refresh(); err != nil {
		return "", "", errors.Trace(err)
	}

	// The application requires permissions for both ARM and AD, so we
	// can use the token for both APIs.
	graphResourceId := TokenResource(graphEndpoint)
	graphToken := armSpt.Token
	graphToken.Resource = graphResourceId
	graphSpt, err := azure.NewServicePrincipalTokenFromManualToken(*oauthConfig, clientId, graphResourceId, graphToken)
	if err != nil {
		return "", "", errors.Annotate(err, "creating temporary Graph service principal token")
	}
	graphSpt.SetSender(&client)
	if err := graphSpt.Refresh(); err != nil {
		return "", "", errors.Trace(err)
	}

	directoryURL, err := url.Parse(graphEndpoint)
	if err != nil {
		return "", "", errors.Annotate(err, "parsing identity endpoint")
	}
	directoryURL.Path = path.Join(directoryURL.Path, tenantId)
	directoryClient := ad.NewManagementClient(directoryURL.String())
	authorizationClient := authorization.NewWithBaseURI(resourceManagerEndpoint, subscriptionId)
	useragent.UpdateClient(&authorizationClient.Client)
	directoryClient.Authorizer = graphSpt
	authorizationClient.Authorizer = armSpt
	authorizationClient.Sender = client.Sender
	directoryClient.Sender = client.Sender
	setClientInspectors(&directoryClient.Client, requestInspector, "azure.directory")
	setClientInspectors(&authorizationClient.Client, requestInspector, "azure.authorization")

	userObject, err := ad.UsersClient{directoryClient}.GetCurrentUser()
	if err != nil {
		return "", "", errors.Trace(err)
	}
	fmt.Fprintf(stderr, "Authenticated as %q.\n", userObject.DisplayName)

	fmt.Fprintln(stderr, "Creating/updating service principal.")
	servicePrincipalObjectId, password, err := createOrUpdateServicePrincipal(
		ad.ServicePrincipalsClient{directoryClient},
		subscriptionId,
		clock,
		newUUID,
	)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	fmt.Fprintln(stderr, "Assigning Owner role to service principal.")
	if err := createRoleAssignment(
		authorizationClient,
		subscriptionId,
		servicePrincipalObjectId,
		newUUID,
		clock,
	); err != nil {
		return "", "", errors.Trace(err)
	}
	return jujuApplicationId, password, nil
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

func createOrUpdateServicePrincipal(
	client ad.ServicePrincipalsClient,
	subscriptionId string,
	clock clock.Clock,
	newUUID func() (utils.UUID, error),
) (servicePrincipalObjectId, password string, _ error) {
	passwordCredential, err := preparePasswordCredential(clock, newUUID)
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
		Clock:       clock,
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

func preparePasswordCredential(
	clock clock.Clock,
	newUUID func() (utils.UUID, error),
) (ad.PasswordCredential, error) {
	password, err := newUUID()
	if err != nil {
		return ad.PasswordCredential{}, errors.Annotate(err, "generating password")
	}
	passwordKeyUUID, err := newUUID()
	if err != nil {
		return ad.PasswordCredential{}, errors.Annotate(err, "generating password key ID")
	}
	startDate := clock.Now().UTC()
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

func createRoleAssignment(
	authorizationClient authorization.ManagementClient,
	subscriptionId string,
	servicePrincipalObjectId string,
	newUUID func() (utils.UUID, error),
	clock clock.Clock,
) error {
	// Find the role definition with the name "Owner".
	roleScope := path.Join("subscriptions", subscriptionId)
	roleDefinitionsClient := authorization.RoleDefinitionsClient{authorizationClient}
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
	roleAssignmentUUID, err := newUUID()
	if err != nil {
		return errors.Annotate(err, "generating role assignment ID")
	}
	roleAssignmentsClient := authorization.RoleAssignmentsClient{authorizationClient}
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
		Clock:       clock,
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
