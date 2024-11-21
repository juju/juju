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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v3"
	"github.com/google/uuid"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/retry"
	abstractions "github.com/microsoft/kiota-abstractions-go"
	"github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/applications"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/serviceprincipals"
)

var logger = loggo.GetLogger("juju.provider.azure.internal.auth")

const (
	// passwordExpiryDuration is how long the application password we
	// set will remain valid.
	passwordExpiryDuration = 365 * 24 * time.Hour
)

const (
	// These consts represent the Azure cloud types.

	AzureCloud        = "AzureCloud"
	AzureChinaCloud   = "AzureChinaCloud"
	AzureUSGovernment = "AzureUSGovernment"
)

// JujuActions contains the allowed actions needed by
// a Juju controller.
var JujuActions = []string{
	"Microsoft.Compute/*",
	"Microsoft.KeyVault/*",
	"Microsoft.Network/*",
	"Microsoft.Resources/*",
	"Microsoft.Storage/*",
	"Microsoft.ManagedIdentity/userAssignedIdentities/*",
}

// MaybeJujuApplicationObjectID returns the Juju Application Object ID
// if the passed in application ID is the Juju Enterprise App.
// This is only needed for very old credentials. At some point we
// should be able to delete it.
func MaybeJujuApplicationObjectID(appID string) (string, error) {
	if appID == "60a04dc9-1857-425f-8076-5ba81ca53d66" {
		return "8b744cea-179d-4a73-9dff-20d52126030a", nil
	}
	return "", errors.Errorf("unexpected application ID %q", appID)
}

// ServicePrincipalParams are used when creating Juju service principal.
type ServicePrincipalParams struct {
	CloudName string
	// Credential is the authorization needed to contact the
	// Azure graph API.
	Credential azcore.TokenCredential

	// SubscriptionId is the subscription ID of the account creating
	// the service principal.
	SubscriptionId string

	// TenantId is the tenant that the account creating the service
	// principal belongs to.
	TenantId string

	// ApplicationName is the name of the enterprise app with which
	// the service principal is associated.
	ApplicationName string

	// RoleDefinitionName is the name of the role definition holding
	// the allowed actions for a Juju controller.
	RoleDefinitionName string
}

// ServicePrincipalCreator creates a service principal for the
// Juju enterprise application.
type ServicePrincipalCreator struct {
	RequestAdaptor abstractions.RequestAdapter
	Sender         policy.Transporter

	Clock   clock.Clock
	NewUUID func() (uuid.UUID, error)
}

// InteractiveCreate creates a new ServicePrincipal by performing device
// code authentication with Azure AD and creating the service principal
// using the credentials that are obtained. Only GraphEndpoint,
// GraphResourceId, ResourceManagerEndpoint, ResourceManagerResourceId
// and SubscriptionId need to be specified in params, the other values
// will be derived.
func (c *ServicePrincipalCreator) InteractiveCreate(sdkCtx context.Context, stderr io.Writer, params ServicePrincipalParams) (appid, spid, password string, _ error) {
	// Perform the interactive authentication. The user will be prompted to
	// open a URL and input a device code, after which they will have to
	// enter their username and password if they are not already
	// authenticated with Azure.
	fmt.Fprintln(stderr, "Initiating interactive authentication.")
	fmt.Fprintln(stderr)

	if params.Credential == nil {
		cred, err := azidentity.NewDeviceCodeCredential(&azidentity.DeviceCodeCredentialOptions{
			ClientOptions:              azcore.ClientOptions{},
			AdditionallyAllowedTenants: []string{"*"},
			DisableInstanceDiscovery:   false,
			TenantID:                   params.TenantId,
		})
		if err != nil {
			return "", "", "", errors.Trace(err)
		}
		params.Credential = cred
	}

	return c.Create(sdkCtx, params)
}

// Create creates a new service principal using the values specified in params.
func (c *ServicePrincipalCreator) Create(sdkCtx context.Context, params ServicePrincipalParams) (appid, spid, password string, err error) {
	var client *msgraphsdkgo.GraphServiceClient
	if c.RequestAdaptor != nil {
		client = msgraphsdkgo.NewGraphServiceClient(c.RequestAdaptor)
	} else {
		client, err = msgraphsdkgo.NewGraphServiceClientWithCredentials(params.Credential, nil)
	}
	if err != nil {
		return "", "", "", errors.Trace(err)
	}

	clientFactory, err := armauthorization.NewClientFactory(params.SubscriptionId, params.Credential, &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: c.Sender,
		},
	})
	if err != nil {
		return "", "", "", errors.Annotate(err, "failed to create auth client")
	}

	applicationName := params.ApplicationName
	if applicationName == "" {
		applicationName = "Juju Application"
	}
	roleDefinitionName := params.RoleDefinitionName
	if roleDefinitionName == "" {
		roleDefinitionName = "Juju Application Role Definition"
	}

	// Create the enterprise application and role definition, followed
	// by the service principal which informs the creation of the credential.

	roleDefinitionId, err := c.ensureRoleDefinition(sdkCtx, clientFactory, params.SubscriptionId, roleDefinitionName)
	if err != nil {
		return "", "", "", errors.Annotate(err, "creating role definition")
	}

	applicationId, err := c.ensureEnterpriseApplication(sdkCtx, client, roleDefinitionId, applicationName)
	if err != nil {
		return "", "", "", errors.Annotate(err, "querying Juju enterprise application")
	}

	servicePrincipalObjectId, password, err := c.createOrUpdateJujuServicePrincipal(sdkCtx, client, applicationId, applicationName)
	if err != nil {
		return "", "", "", errors.Trace(err)
	}
	if err := c.createRoleAssignment(sdkCtx, clientFactory, params.SubscriptionId, servicePrincipalObjectId, roleDefinitionId); err != nil {
		return "", "", "", errors.Trace(err)
	}
	return applicationId, servicePrincipalObjectId, password, nil
}

func (c *ServicePrincipalCreator) getExistingRoleDefinition(ctx context.Context, client *armauthorization.RoleDefinitionsClient, roleScope, roleName string) (string, error) {
	pager := client.NewListPager(roleScope, &armauthorization.RoleDefinitionsClientListOptions{
		Filter: to.Ptr("type eq 'CustomRole'"),
	})
	for pager.More() {
		next, err := pager.NextPage(ctx)
		if err != nil {
			return "", errors.Annotate(err, "fetching role definitions")
		}
		for _, r := range next.Value {
			if r.Properties != nil && toValue(r.Properties.RoleName) == roleName {
				roleDefinitionId := toValue(r.ID)
				return roleDefinitionId, nil
			}
		}
	}
	return "", errors.NotFoundf("role definition %q", roleName)
}

func (c *ServicePrincipalCreator) ensureRoleDefinition(
	ctx context.Context, clientFactory *armauthorization.ClientFactory, subscriptionId, roleName string,
) (string, error) {
	roleScope := path.Join("subscriptions", subscriptionId)

	// Find any existing role definition with the name params.RoleDefinitionName.
	// Try subscription scope first, then tenant scope.
	roleDefinitionClient := clientFactory.NewRoleDefinitionsClient()
	roleDefinitionId, err := c.getExistingRoleDefinition(ctx, roleDefinitionClient, roleScope, roleName)
	if err != nil && errors.Is(err, errors.NotFound) {
		roleDefinitionId, err = c.getExistingRoleDefinition(ctx, roleDefinitionClient, "", roleName)
	}
	if err == nil {
		logger.Debugf("found existing role definition %q", roleDefinitionId)
		return roleDefinitionId, nil
	} else if !errors.Is(err, errors.NotFound) {
		return "", errors.Annotate(err, "finding existing tenant scoped role definition")
	}

	roleDefinitionUUID, err := c.newUUID()
	if err != nil {
		return "", errors.Annotate(err, "generating role definition ID")
	}

	actions := make([]*string, len(JujuActions))
	for i, r := range JujuActions {
		actions[i] = to.Ptr(r)
	}
	rd, err := roleDefinitionClient.CreateOrUpdate(ctx, roleScope, roleDefinitionUUID.String(), armauthorization.RoleDefinition{
		Properties: to.Ptr(armauthorization.RoleDefinitionProperties{
			RoleName:    to.Ptr(roleName),
			Description: to.Ptr("Role definition for Juju controller"),
			AssignableScopes: []*string{
				to.Ptr(roleScope),
			},
			Permissions: []*armauthorization.Permission{{
				Actions: actions,
			}},
		}),
	}, nil)
	if err != nil {
		return "", errors.Annotate(err, "failed to create role definition")
	}
	return toValue(rd.ID), nil
}

func (c *ServicePrincipalCreator) ensureEnterpriseApplication(
	ctx context.Context, client *msgraphsdkgo.GraphServiceClient, roleDefinitionId, name string,
) (string, error) {
	applicationClient := client.Applications()

	req := &applications.ApplicationsRequestBuilderGetRequestConfiguration{
		QueryParameters: &applications.ApplicationsRequestBuilderGetQueryParameters{
			Filter: to.Ptr(fmt.Sprintf("displayName eq '%s'", name)),
		},
	}
	resp, err := applicationClient.Get(ctx, req)
	if err != nil {
		return "", errors.Annotate(err, "listing applications")
	}
	result := resp.GetValue()
	if len(result) > 0 {
		id := toValue(result[0].GetAppId())
		logger.Debugf("found existing Juju application %q", id)
		return id, nil
	}

	appUUID, err := c.newUUID()
	if err != nil {
		return "", errors.Annotate(err, "generating application ID")
	}

	parts := strings.Split(roleDefinitionId, "/")
	roleUUID, err := uuid.Parse(parts[len(parts)-1])
	if err != nil {
		return "", errors.Annotate(err, "parsing role definition UUID")
	}

	description := "Permissions for " + name
	requestBody := models.NewApplication()
	requestBody.SetId(to.Ptr(appUUID.String()))
	requestBody.SetDisplayName(to.Ptr(name))
	requestBody.SetDescription(to.Ptr(description))
	appRole := models.NewAppRole()
	appRole.SetId(to.Ptr(roleUUID))
	appRole.SetValue(to.Ptr(strings.ReplaceAll(name, " ", "")))
	appRole.SetAllowedMemberTypes([]string{"Application"})
	appRole.SetDisplayName(to.Ptr(name))
	appRole.SetDescription(to.Ptr(description))
	appRole.SetIsEnabled(to.Ptr(true))
	requestBody.SetAppRoles([]models.AppRoleable{
		appRole,
	})

	application, err := applicationClient.Post(ctx, requestBody, nil)
	if err != nil {
		return "", errors.Annotate(ReportableError(err), "creating application")
	}
	return toValue(application.GetAppId()), nil
}

func (c *ServicePrincipalCreator) createOrUpdateJujuServicePrincipal(
	ctx context.Context, client *msgraphsdkgo.GraphServiceClient, applicationId, applicationName string,
) (servicePrincipalObjectId, password string, _ error) {
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

	addPassword := func(servicePrincipal models.ServicePrincipalable) (string, string, error) {
		requestBody := serviceprincipals.NewItemAddPasswordPostRequestBody()
		requestBody.SetPasswordCredential(passwordCredential)

		spID := toValue(servicePrincipal.GetId())
		addPassword, err := client.ServicePrincipals().ByServicePrincipalId(spID).AddPassword().Post(context.Background(), requestBody, nil)
		if err != nil {
			return "", "", errors.Annotate(ReportableError(err), "creating service principal password")
		}
		return toValue(servicePrincipal.GetId()), toValue(addPassword.GetSecretText()), nil
	}

	servicePrincipal, err := c.createOrUpdateServicePrincipal(ctx, client, applicationId, applicationName)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	id, password, err := addPassword(servicePrincipal)
	if err != nil {
		return "", "", errors.Annotate(err, "creating service principal password")
	}
	return id, password, nil
}

func (c *ServicePrincipalCreator) createOrUpdateServicePrincipal(ctx context.Context, client *msgraphsdkgo.GraphServiceClient, appId, label string) (models.ServicePrincipalable, error) {
	// The service principal might already exist, so we need to query its application ID.
	servicePrincipal, err := client.ServicePrincipalsWithAppId(to.Ptr(appId)).Get(ctx, nil)
	if err == nil {
		return servicePrincipal, nil
	}
	if !isNotFound(err) {
		return nil, errors.Annotatef(ReportableError(err), "looking for existing service principal for %s", label)
	}

	createServicePrincipal := func() error {
		requestBody := models.NewServicePrincipal()
		requestBody.SetAppId(to.Ptr(appId))
		requestBody.SetAccountEnabled(to.Ptr(true))
		servicePrincipal, err = client.ServicePrincipals().Post(ctx, requestBody, nil)
		return errors.Annotate(ReportableError(err), "creating service principal")
	}
	retryArgs := retry.CallArgs{
		Func: createServicePrincipal,
		IsFatalError: func(err error) bool {
			err = ReportableError(err)
			if strings.Contains(err.Error(), " does not reference ") || strings.Contains(err.Error(), " does not exist ") {
				return false
			}
			return true
		},
		Clock:       c.clock(),
		Delay:       5 * time.Second,
		MaxDuration: time.Minute,
	}
	if err := retry.Call(retryArgs); err != nil {
		if !isAlreadyExists(err) {
			return nil, errors.Trace(err)
		}
		servicePrincipal, err = client.ServicePrincipalsWithAppId(to.Ptr(appId)).Get(ctx, nil)
		if err != nil {
			return nil, errors.Annotatef(ReportableError(err), "looking for service principal for %s", label)
		}
	}
	return servicePrincipal, nil
}

func (c *ServicePrincipalCreator) preparePasswordCredential() (*models.PasswordCredential, error) {
	passwordKeyUUID, err := c.newUUID()
	if err != nil {
		return nil, errors.Annotate(err, "generating password key ID")
	}
	startDate := c.clock().Now().UTC()
	endDate := startDate.Add(passwordExpiryDuration)

	cred := models.NewPasswordCredential()
	cred.SetCustomKeyIdentifier([]byte("juju-" + startDate.Format("20060102")))
	cred.SetKeyId(to.Ptr(passwordKeyUUID))
	cred.SetStartDateTime(&startDate)
	cred.SetEndDateTime(&endDate)
	return cred, nil
}

func (c *ServicePrincipalCreator) createRoleAssignment(
	ctx context.Context, clientFactory *armauthorization.ClientFactory,
	subscriptionId, servicePrincipalObjectId, roleDefinitionId string,
) error {
	// Find the role definition with the name "Owner".
	roleScope := path.Join("subscriptions", subscriptionId)

	// The UUID value for the role assignment name is unimportant. Azure
	// will prevent multiple role assignments for the same role definition
	// and principal pair.
	roleAssignmentUUID, err := c.newUUID()
	if err != nil {
		return errors.Annotate(err, "generating role assignment ID")
	}
	roleAssignmentName := roleAssignmentUUID.String()
	roleAssignmentClient := clientFactory.NewRoleAssignmentsClient()
	retryArgs := retry.CallArgs{
		Func: func() error {
			_, err := roleAssignmentClient.Create(
				ctx,
				roleScope, roleAssignmentName,
				armauthorization.RoleAssignmentCreateParameters{
					Properties: &armauthorization.RoleAssignmentProperties{
						RoleDefinitionID: to.Ptr(roleDefinitionId),
						PrincipalID:      to.Ptr(servicePrincipalObjectId),
						PrincipalType:    to.Ptr(armauthorization.PrincipalTypeServicePrincipal),
					},
				}, nil,
			)
			return err
		},
		IsFatalError: func(err error) bool {
			if strings.Contains(err.Error(), " does not exist in the directory ") {
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
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			const serviceErrorCodeRoleAssignmentExists = "RoleAssignmentExists"
			if respErr.ErrorCode == serviceErrorCodeRoleAssignmentExists {
				return nil
			}
		}
		return errors.Annotate(err, "creating role assignment")
	}
	return nil
}

func isAlreadyExists(err error) bool {
	dataErr, ok := AsDataError(err)
	return ok && dataErr.Code() == "Request_MultipleObjectsWithSameKeyValue"
}

func isNotFound(err error) bool {
	dataErr, ok := AsDataError(err)
	return ok && dataErr.Code() == "Request_ResourceNotFound"
}

func (c *ServicePrincipalCreator) clock() clock.Clock {
	if c.Clock == nil {
		return clock.WallClock
	}
	return c.Clock
}

func (c *ServicePrincipalCreator) newUUID() (uuid.UUID, error) {
	if c.NewUUID == nil {
		u, err := uuid.NewUUID()
		if err != nil {
			return uuid.UUID{}, errors.Trace(err)
		}
		return uuid.Parse(u.String())
	}
	return c.NewUUID()
}

func toValue[T any](v *T) T {
	if v == nil {
		return *new(T)
	}
	return *v
}
