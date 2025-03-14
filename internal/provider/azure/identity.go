// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/provider/azure/internal/armtemplates"
	"github.com/juju/juju/internal/provider/azure/internal/azureauth"
)

// SupportsInstanceRoles indicates if Instance Roles are supported by this
// environ.
func (env *azureEnviron) SupportsInstanceRoles(_ envcontext.ProviderCallContext) bool {
	return true
}

// CreateAutoInstanceRole is responsible for setting up an instance role on
// behalf of the user.
// For Azure, this means creating a managed identity with the correct role definition
// assigned to it.
func (env *azureEnviron) CreateAutoInstanceRole(
	ctx envcontext.ProviderCallContext,
	args environs.BootstrapParams,
) (string, error) {
	controllerUUID := args.ControllerConfig.ControllerUUID()
	err := env.initResourceGroup(ctx, controllerUUID, env.config.resourceGroupName != "", true)
	if err != nil {
		return "", errors.Annotate(err, "creating resource group for managed identity")
	}

	instProfileName, err := env.ensureControllerManagedIdentity(
		ctx,
		controllerUUID)
	if err != nil {
		return "", errors.Annotate(err, "creating managed identity")
	}
	return fmt.Sprintf("%s/%s", env.resourceGroup, *instProfileName), nil
}

func (env *azureEnviron) ensureControllerManagedIdentity(
	callCtx envcontext.ProviderCallContext,
	controllerUUID string,
) (*string, error) {
	envTags := tags.ResourceTags(
		names.ModelTag{},
		names.NewControllerTag(controllerUUID),
		env.config,
	)

	identityName := fmt.Sprintf("juju-controller-%s", controllerUUID)
	roleName := fmt.Sprintf("juju-controller-role-%s", controllerUUID)
	roleGUID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(roleName)).String()
	roleAssignmentGUID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(identityName+roleName)).String()
	params := map[string]any{
		"roleName": map[string]string{
			"value": roleName,
		},
		"identityName": map[string]string{
			"value": identityName,
		},
		"roleGuid": map[string]string{
			"value": roleGUID,
		},
		"resourceGroup": map[string]string{
			"value": env.resourceGroup,
		},
		"identityAPIVersion": map[string]string{
			"value": identityAPIVersion,
		},
	}

	// This is the arm resource template to create the managed identity.
	// It needs to be done in the controller resource group.
	res := []armtemplates.Resource{{
		APIVersion: identityAPIVersion,
		Type:       "Microsoft.ManagedIdentity/userAssignedIdentities",
		Name:       identityName,
		Location:   env.location,
		Tags:       envTags,
	}}
	template := armtemplates.Template{Resources: res}
	if err := env.createDeployment(
		callCtx,
		env.resourceGroup,
		identityName,
		template,
	); err != nil {
		return nil, errors.Annotatef(err, "creating managed identity %q", identityName)
	}

	// This is the arm resource template to create:
	// - role definition
	// - assignment of role to identity
	// It needs to be done in the subscription.
	res = []armtemplates.Resource{{
		APIVersion: roleAPIVersion,
		Type:       "Microsoft.Authorization/roleDefinitions",
		Name:       "[parameters('roleGuid')]",
		Location:   env.location,
		Tags:       envTags,
		Properties: map[string]any{
			"roleName":    "[parameters('roleName')]",
			"description": "roles for juju controller",
			"type":        "customRole",
			"permissions": []map[string]any{{
				"actions": azureauth.JujuActions,
			}},
			"assignableScopes": []string{
				"[subscription().id]",
			},
		},
	}, {
		Type:       "Microsoft.Authorization/roleAssignments",
		APIVersion: roleAPIVersion,
		Name:       roleAssignmentGUID,
		Location:   env.location,
		Tags:       envTags,
		DependsOn: []string{
			"[subscriptionResourceId('Microsoft.Authorization/roleDefinitions', parameters('roleGuid'))]",
		},
		Properties: map[string]any{
			"roleDefinitionId": "[subscriptionResourceId('Microsoft.Authorization/roleDefinitions', parameters('roleGuid'))]",
			"principalType":    "ServicePrincipal",
			"principalId":      "[reference(resourceId(subscription().subscriptionId, parameters('resourceGroup'), 'Microsoft.ManagedIdentity/userAssignedIdentities', parameters('identityName')), parameters('identityAPIVersion')).principalId]",
		},
	}}

	template = armtemplates.Template{
		Schema: armtemplates.SubscriptionSchema,
		Parameters: map[string]any{
			"roleName": map[string]string{
				"type": "string",
			},
			"roleGuid": map[string]string{
				"type": "string",
			},
			"identityName": map[string]string{
				"type": "string",
			},
			"resourceGroup": map[string]string{
				"type": "string",
			},
			"identityAPIVersion": map[string]string{
				"type": "string",
			},
		},
		Resources: res,
	}

	logger.Debugf(callCtx, "running deployment to create managed identity role assignment %s", identityName)
	if err := env.createSubscriptionDeployment(
		callCtx,
		env.location,
		identityName, // deployment name
		params,
		template,
	); err != nil {
		// First cancel any in-progress deployment.
		var wg sync.WaitGroup
		var cancelResult error
		logger.Debugf(callCtx, "canceling deployment for managed identity")
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			cancelResult = errors.Annotatef(
				env.cancelDeployment(callCtx, id),
				"canceling deployment %q", id,
			)
		}(identityName)
		wg.Wait()
		if cancelResult != nil && !errors.IsNotFound(cancelResult) {
			return nil, errors.Annotate(cancelResult, "aborting failed bootstrap")
		}

		// Then cleanup the resource group.
		if err := env.Destroy(callCtx); err != nil {
			logger.Errorf(callCtx, "failed to destroy controller: %v", err)
		}
		return nil, errors.Trace(err)
	}
	return &identityName, nil
}

// managedIdentityResourceId returns the Azure resource id for a managed identity
// specified by identityInfo.
// identityInfo may specify just the identity name, or resource group/identity, or
// subscription/resource group/identity.
func (env *azureEnviron) managedIdentityResourceId(identityInfo string) string {
	subscriptionId := env.subscriptionId
	groupName := env.resourceGroup
	instanceRoleName := identityInfo
	roleParts := strings.Split(instanceRoleName, "/")
	switch len(roleParts) {
	case 3:
		// subscription/group/identity
		subscriptionId = roleParts[0]
		groupName = roleParts[1]
		instanceRoleName = roleParts[2]
	case 2:
		// group/identity
		groupName = roleParts[0]
		instanceRoleName = roleParts[1]
	}
	identityId := fmt.Sprintf(
		"/subscriptions/%s/resourcegroups/%s/providers/Microsoft.ManagedIdentity/userAssignedIdentities/%s",
		subscriptionId,
		groupName,
		instanceRoleName,
	)
	return identityId
}

// managedIdentityGroup returns the resource group from the identity info string.
func (env *azureEnviron) managedIdentityGroup(identityInfo string) string {
	groupName := env.resourceGroup
	roleParts := strings.Split(identityInfo, "/")
	switch len(roleParts) {
	case 3:
		// subscription/group/identity
		groupName = roleParts[1]
	case 2:
		// group/identity
		groupName = roleParts[0]
	}
	return groupName
}

// FinaliseBootstrapCredential is responsible for performing and finalisation
// steps to a credential being passwed to a newly bootstrapped controller. This
// was introduced to help with the transformation to instance roles.
func (env *azureEnviron) FinaliseBootstrapCredential(
	_ environs.BootstrapContext,
	args environs.BootstrapParams,
	cred *cloud.Credential,
) (*cloud.Credential, error) {
	if !args.BootstrapConstraints.HasInstanceRole() {
		return cred, nil
	}
	if len(strings.Split(*args.BootstrapConstraints.InstanceRole, "/")) > 3 {
		return nil, errors.NotValidf("managaed identity %q", *args.BootstrapConstraints.InstanceRole)
	}
	newCred := cloud.NewCredential(cloud.ManagedIdentityAuthType, map[string]string{
		credManagedIdentity:    *args.BootstrapConstraints.InstanceRole,
		credAttrSubscriptionId: env.subscriptionId,
	})
	return &newCred, nil
}
