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
	"github.com/juju/retry"
	"github.com/juju/utils/v3"
	abstractions "github.com/microsoft/kiota-abstractions-go"
	"github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/serviceprincipals"
)

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

// ServicePrincipalParams are used when creating Juju service principal.
type ServicePrincipalParams struct {
	// Credential is the authorization needed to contact the
	// Azure graph API.
	Credential azcore.TokenCredential

	// SubscriptionId is the subscription ID of the account creating
	// the service principal.
	SubscriptionId string

	// TenantId is the tenant that the account creating the service
	// principal belongs to.
	TenantId string
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
			ClientID:                   jujuApplicationId,
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
	servicePrincipalObjectId, password, err := c.createOrUpdateServicePrincipal(sdkCtx, client)
	if err != nil {
		return "", "", "", errors.Trace(err)
	}
	if err := c.createRoleAssignment(sdkCtx, params, servicePrincipalObjectId); err != nil {
		return "", "", "", errors.Trace(err)
	}
	return jujuApplicationId, servicePrincipalObjectId, password, nil
}

func (c *ServicePrincipalCreator) createOrUpdateServicePrincipal(sdkCtx context.Context, client *msgraphsdkgo.GraphServiceClient) (servicePrincipalObjectId, password string, _ error) {
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

	// The service principal might already exist, so we need to query
	// its object ID, and fetch the existing password credentials
	// to update.
	servicePrincipal, err := client.ServicePrincipalsWithAppId(to.Ptr(jujuApplicationId)).Get(sdkCtx, nil)
	if err == nil {
		id, password, err := addPassword(servicePrincipal)
		if err != nil {
			return "", "", errors.Annotate(err, "creating service principal password")
		}
		return id, password, nil
	}
	if !isNotFound(err) {
		return "", "", errors.Annotate(ReportableError(err), "looking for existing service principal")
	}

	createServicePrincipal := func() error {
		requestBody := models.NewServicePrincipal()
		requestBody.SetAppId(to.Ptr(jujuApplicationId))
		requestBody.SetAccountEnabled(to.Ptr(true))
		servicePrincipal, err = client.ServicePrincipals().Post(sdkCtx, requestBody, nil)
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
			return "", "", errors.Trace(err)
		}
		// The service principal already exists, so we'll fall out
		// and update the service principal's password credentials.
		servicePrincipal, err = client.ServicePrincipalsWithAppId(to.Ptr(jujuApplicationId)).Get(sdkCtx, nil)
		if err != nil {
			return "", "", errors.Annotate(ReportableError(err), "looking for service principal")
		}
	}

	id, password, err := addPassword(servicePrincipal)
	if err != nil {
		return "", "", errors.Annotate(err, "creating service principal password")
	}
	return id, password, nil
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

func (c *ServicePrincipalCreator) createRoleAssignment(sdkCtx context.Context, params ServicePrincipalParams, servicePrincipalObjectId string) error {
	// Find the role definition with the name "Owner".
	roleScope := path.Join("subscriptions", params.SubscriptionId)

	clientFactory, err := armauthorization.NewClientFactory(params.SubscriptionId, params.Credential, &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: c.Sender,
		},
	})
	if err != nil {
		return errors.Annotate(err, "failed to create auth client")
	}
	roleDefinitionClient := clientFactory.NewRoleDefinitionsClient()
	pager := roleDefinitionClient.NewListPager(roleScope, &armauthorization.RoleDefinitionsClientListOptions{
		Filter: to.Ptr("roleName eq 'Owner'"),
	})
	var roleDefinitionId string
done:
	for pager.More() {
		next, err := pager.NextPage(sdkCtx)
		if err != nil {
			return errors.Annotate(err, "fetching role definitions")
		}
		for _, r := range next.Value {
			roleDefinitionId = toValue(r.ID)
			break done
		}
	}

	if roleDefinitionId == "" {
		return errors.NotFoundf("Owner role definition")
	}

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
				sdkCtx,
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
		u, err := utils.NewUUID()
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
