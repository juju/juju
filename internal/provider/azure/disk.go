// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
	"github.com/google/uuid"
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/internal/provider/azure/internal/azureauth"
	"github.com/juju/juju/internal/provider/azure/internal/errorutils"
	"github.com/juju/juju/internal/storage"
)

const (
	// Disk encryption config attributes.
	encryptedKey             = "encrypted"
	diskEncryptionSetNameKey = "disk-encryption-set-name"
	vaultNamePrefixKey       = "vault-name-prefix"
	vaultKeyNameKey          = "vault-key-name"
	vaultUserIDKey           = "vault-user-id"
)

// diskEncryptionInfo creates the resources needed for encrypting a disk,
// including disk encryption set and vault.
func (env *azureEnviron) diskEncryptionInfo(
	ctx context.Context,
	rootDisk *storage.VolumeParams,
	envTags map[string]string,
) (string, error) {
	if rootDisk == nil {
		return "", nil
	}
	logger.Debugf(ctx, "creating root disk encryption with parameters: %#v", *rootDisk)
	// The "encrypted" value may arrive as a bool or a string.
	encryptedStr, ok := rootDisk.Attributes[encryptedKey].(string)
	encrypted, _ := rootDisk.Attributes[encryptedKey].(bool)
	if !encrypted && ok {
		encrypted, _ = strconv.ParseBool(encryptedStr)
	}
	if !encrypted {
		logger.Debugf(ctx, "encryption not enabled for root disk")
		return "", nil
	}

	encryptionSet, _ := rootDisk.Attributes[diskEncryptionSetNameKey].(string)
	vaultNamePrefix, _ := rootDisk.Attributes[vaultNamePrefixKey].(string)
	keyName, _ := rootDisk.Attributes[vaultKeyNameKey].(string)
	userID, _ := rootDisk.Attributes[vaultUserIDKey].(string)
	if vaultNamePrefix == "" && encryptionSet == "" {
		return "", errors.New("root disk encryption needs either a vault or a disk encryption set to be specified")
	}

	// The disk encryption set may be a reference to an existing one.
	diskEncryptionSetRG, diskEncryptionSetName := referenceInfo(encryptionSet)
	if diskEncryptionSetName == "" {
		diskEncryptionSetName = vaultNamePrefix
	}
	diskEncryptionSetID := fmt.Sprintf(`[resourceId('Microsoft.Compute/diskEncryptionSets', '%s')]`, diskEncryptionSetName)
	if diskEncryptionSetRG != "" {
		diskEncryptionSetID = fmt.Sprintf(`[resourceId('%s', 'Microsoft.Compute/diskEncryptionSets', '%s')]`, diskEncryptionSetRG, diskEncryptionSetName)
	}
	// Do we just have a disk encryption set specified and no vault?
	if vaultNamePrefix == "" {
		return diskEncryptionSetID, nil
	}

	// If we need to create the disk encryption set, it must be in the model's resource group.
	if diskEncryptionSetRG != "" {
		return "", errors.New("do not specify a resource group for a disk encryption set to be created")
	}

	envTagPtr := make(map[string]*string)
	for k, v := range envTags {
		envTagPtr[k] = to.Ptr(v)
	}

	encryptionSets, err := env.encryptionSetsClient()
	if err != nil {
		return "", errors.Trace(err)
	}
	// See if the disk encryption set already exists.
	existingDes, err := encryptionSets.Get(ctx, env.resourceGroup, diskEncryptionSetName, nil)
	if err != nil && !errorutils.IsNotFoundError(err) {
		return "", errors.Trace(err)
	}
	// Record the identity of an existing disk encryption set
	// so we can maintain the access policy on the vault.
	var desIdentity *armcompute.EncryptionSetIdentity
	if err == nil {
		desIdentity = existingDes.Identity
	}
	// The vault name must be unique across the entire subscription.
	if len(vaultNamePrefix) > 15 {
		return "", errors.Errorf("vault name prefix %q too long, must be 15 characters or less", vaultNamePrefix)
	}

	env.mu.Lock()
	defer env.mu.Unlock()

	vaults, err := env.vaultsClient()
	if err != nil {
		return "", errors.Trace(err)
	}
	vaultName := fmt.Sprintf("%s-%s", vaultNamePrefix, env.config.Config.UUID()[:8])
	vault, vaultParams, err := env.ensureVault(ctx, vaults, vaultName, userID, envTagPtr, desIdentity)
	if err != nil {
		return "", env.HandleCredentialError(ctx, errors.Annotatef(err, "creating vault %q", vaultName))
	}

	// Create a key in the vault.
	if keyName == "" {
		keyName = "disk-secret"
	}
	keyRef, err := env.createVaultKey(ctx, *vault.Properties.VaultURI, *vault.Name, keyName)
	if err != nil {
		return "", env.HandleCredentialError(ctx, errors.Annotatef(err, "creating vault key in %q", vaultName))
	}

	// We had an existing disk encryption set.
	if desIdentity != nil {
		return diskEncryptionSetID, nil
	}

	// Create the disk encryption set.
	desIdentity, err = env.ensureDiskEncryptionSet(ctx, encryptionSets, diskEncryptionSetName, envTagPtr, vault.ID, keyRef)
	if err != nil {
		return "", env.HandleCredentialError(ctx, errors.Annotatef(err, "creating disk encryption set %q", diskEncryptionSetName))
	}

	// Update the vault access policies to allow the disk encryption set to access the key.
	vaultAccessPolicies := vaultParams.Properties.AccessPolicies
	vaultAccessPolicies = append(vaultAccessPolicies, vaultAccessPolicy(desIdentity))
	vaultParams.Properties.AccessPolicies = vaultAccessPolicies
	poller, err := vaults.BeginCreateOrUpdate(ctx, env.resourceGroup, vaultName, *vaultParams, nil)
	if err == nil {
		_, err = poller.PollUntilDone(ctx, nil)
	}
	if err != nil {
		return "", env.HandleCredentialError(ctx, errors.Annotatef(err, "updating vault %q access policies ", vaultName))
	}
	return diskEncryptionSetID, nil
}

// fromStringOrNil returns a UUID parsed from the input string.
// Same behavior as Parse(), but returns uuid.Nil instead of an error.
func fromStringOrNil(input string) uuid.UUID {
	result, err := uuid.Parse(input)
	if err != nil {
		return uuid.Nil
	}
	return result
}

func vaultAccessPolicy(desIdentity *armcompute.EncryptionSetIdentity) *armkeyvault.AccessPolicyEntry {
	tenantID := fromStringOrNil(toValue(desIdentity.TenantID))
	return &armkeyvault.AccessPolicyEntry{
		TenantID: to.Ptr(tenantID.String()),
		ObjectID: desIdentity.PrincipalID,
		Permissions: &armkeyvault.Permissions{
			Keys: to.SliceOfPtrs(armkeyvault.KeyPermissionsWrapKey, armkeyvault.KeyPermissionsUnwrapKey,
				armkeyvault.KeyPermissionsList, armkeyvault.KeyPermissionsGet),
		},
	}
}

// ensureDiskEncryptionSet creates or updates a disk encryption set
// to use the specified vault and key.
func (env *azureEnviron) ensureDiskEncryptionSet(
	ctx context.Context,
	encryptionSets *armcompute.DiskEncryptionSetsClient,
	encryptionSetName string,
	envTags map[string]*string,
	vaultID, vaultKey *string,
) (*armcompute.EncryptionSetIdentity, error) {
	logger.Debugf(ctx, "ensure disk encryption set %q", encryptionSetName)
	poller, err := encryptionSets.BeginCreateOrUpdate(ctx, env.resourceGroup, encryptionSetName, armcompute.DiskEncryptionSet{
		Location: to.Ptr(env.location),
		Tags:     envTags,
		Identity: &armcompute.EncryptionSetIdentity{
			Type: to.Ptr(armcompute.DiskEncryptionSetIdentityTypeSystemAssigned),
		},
		Properties: &armcompute.EncryptionSetProperties{
			ActiveKey: &armcompute.KeyForDiskEncryptionSet{
				SourceVault: &armcompute.SourceVault{
					ID: vaultID,
				},
				KeyURL: vaultKey,
			},
		},
	}, nil)
	var result armcompute.DiskEncryptionSetsClientCreateOrUpdateResponse
	if err == nil {
		result, err = poller.PollUntilDone(ctx, nil)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result.Identity, nil
}

// ensureVault creates a vault and adds an access policy for the
// specified disk encryption set identity.
func (env *azureEnviron) ensureVault(
	ctx context.Context,
	vaults *armkeyvault.VaultsClient,
	vaultName string,
	userID string,
	envTags map[string]*string,
	desIdentity *armcompute.EncryptionSetIdentity,
) (*armkeyvault.Vault, *armkeyvault.VaultCreateOrUpdateParameters, error) {
	logger.Debugf(ctx, "ensure vault key %q", vaultName)
	vaultTenantID := fromStringOrNil(env.tenantId)
	// Create the vault with full access for the tenant.
	allKeyPermissions := armkeyvault.PossibleKeyPermissionsValues()

	credAttrs := env.cloud.Credential.Attributes()
	appObjectID := credAttrs[credAttrApplicationObjectId]
	// Older credentials don't have the application object id set,
	// so look it up here and record it for next time.
	if appObjectID == "" {
		appID := credAttrs[credAttrAppId]
		var err error
		appObjectID, err = azureauth.MaybeJujuApplicationObjectID(appID)
		if err != nil {
			return nil, nil, errors.Annotatef(err, "credential missing %s for %q", credAttrApplicationObjectId, appID)
		}
		credAttrs[credAttrApplicationObjectId] = appObjectID
		cred := cloud.NewCredential(env.cloud.Credential.AuthType(), credAttrs)
		env.cloud.Credential = &cred
	}

	vaultAccessPolicies := []*armkeyvault.AccessPolicyEntry{{
		TenantID: to.Ptr(vaultTenantID.String()),
		ObjectID: to.Ptr(appObjectID),
		Permissions: &armkeyvault.Permissions{
			Keys: to.SliceOfPtrs(allKeyPermissions...),
		},
	}}
	if userID != "" {
		vaultAccessPolicies = append(vaultAccessPolicies, &armkeyvault.AccessPolicyEntry{
			TenantID: to.Ptr(vaultTenantID.String()),
			ObjectID: to.Ptr(userID),
			Permissions: &armkeyvault.Permissions{
				Keys: to.SliceOfPtrs(allKeyPermissions...),
			},
		})
	}
	if desIdentity != nil {
		vaultAccessPolicies = append(vaultAccessPolicies, vaultAccessPolicy(desIdentity))
	}
	vaultParams := armkeyvault.VaultCreateOrUpdateParameters{
		Location: to.Ptr(env.location),
		Tags:     envTags,
		Properties: &armkeyvault.VaultProperties{
			TenantID:                 to.Ptr(vaultTenantID.String()),
			EnabledForDiskEncryption: to.Ptr(true),
			EnableSoftDelete:         to.Ptr(true),
			EnablePurgeProtection:    to.Ptr(true),
			CreateMode:               to.Ptr(armkeyvault.CreateModeDefault),
			NetworkACLs: &armkeyvault.NetworkRuleSet{
				Bypass:        to.Ptr(armkeyvault.NetworkRuleBypassOptionsAzureServices),
				DefaultAction: to.Ptr(armkeyvault.NetworkRuleActionAllow),
			},
			SKU: &armkeyvault.SKU{
				Family: to.Ptr(armkeyvault.SKUFamilyA),
				Name:   to.Ptr(armkeyvault.SKUNameStandard),
			},
			AccessPolicies: vaultAccessPolicies,
		},
	}

	// Before creating check to see if the key vault has been soft deleted.
	_, err := vaults.GetDeleted(ctx, vaultName, env.location, nil)
	if err != nil {
		if !errorutils.IsNotFoundError(err) && !errorutils.IsForbiddenError(err) {
			return nil, nil, errors.Annotatef(err, "checking for an existing soft deleted vault %q", vaultName)
		}
	}
	if !errorutils.IsNotFoundError(err) && !errorutils.IsForbiddenError(err) {
		logger.Debugf(ctx, "key vault %q has been soft deleted", vaultName)
		vaultParams.Properties.CreateMode = to.Ptr(armkeyvault.CreateModeRecover)
	}
	var result armkeyvault.VaultsClientCreateOrUpdateResponse
	poller, err := vaults.BeginCreateOrUpdate(ctx, env.resourceGroup, vaultName, vaultParams, nil)
	if err == nil {
		result, err = poller.PollUntilDone(ctx, nil)
	}
	if err != nil {
		return nil, nil, errors.Annotatef(err, "creating vault")
	}
	return &result.Vault, &vaultParams, nil
}

func (env *azureEnviron) deleteVault(ctx context.Context, vaultName string) error {
	logger.Debugf(ctx, "delete vault key %q", vaultName)
	vaults, err := env.vaultsClient()
	if err != nil {
		return errors.Trace(err)
	}
	_, err = vaults.Delete(ctx, env.resourceGroup, vaultName, nil)
	if err != nil {
		if !errorutils.IsNotFoundError(env.HandleCredentialError(ctx, err)) {
			return errors.Annotatef(err, "deleting vault key %q", vaultName)
		}
	}
	return nil
}

// createVaultKey creates, or recovers a soft deleted key,
// in the specified vault.
func (env *azureEnviron) createVaultKey(
	ctx context.Context,
	vaultBaseURI string,
	vaultName string,
	keyName string,
) (*string, error) {
	logger.Debugf(ctx, "create vault key %q in %q", keyName, vaultName)
	keyClient, err := azkeys.NewClient(vaultBaseURI, env.credential, &azkeys.ClientOptions{
		ClientOptions: env.clientOptions})
	if err != nil {
		return nil, errors.Annotatef(err, "creating vault key client for %q", vaultName)
	}

	resp, err := keyClient.CreateKey(
		ctx,
		keyName,
		azkeys.CreateKeyParameters{
			Kty: to.Ptr(azkeys.KeyTypeRSA),
			// TODO(wallyworld) - make these configurable via storage pool attributes
			KeySize: to.Ptr(int32(4096)),
			KeyOps: []*azkeys.KeyOperation{
				to.Ptr(azkeys.KeyOperationWrapKey),
				to.Ptr(azkeys.KeyOperationUnwrapKey),
			},
			KeyAttributes: &azkeys.KeyAttributes{
				Enabled: to.Ptr(true),
			},
		},
		nil)
	if err == nil {
		return to.Ptr(string(toValue(resp.Key.KID))), nil
	}
	if !errorutils.IsConflictError(err) {
		return nil, errors.Trace(err)
	}

	// If the key was previously soft deleted, recover it.
	result, err := keyClient.RecoverDeletedKey(ctx, keyName, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "restoring soft deleted vault key %q in %q", keyName, vaultName)
	}
	return to.Ptr(string(toValue(result.Key.KID))), nil
}
