// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	stdcontext "context"
	"fmt"
	"strconv"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-07-01/compute"
	keyvaultservices "github.com/Azure/azure-sdk-for-go/services/keyvault/2016-10-01/keyvault"
	"github.com/Azure/azure-sdk-for-go/services/keyvault/mgmt/2018-02-14/keyvault"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/errors"
	"github.com/juju/retry"
	uuid "github.com/satori/go.uuid"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/azure/internal/azureauth"
	"github.com/juju/juju/provider/azure/internal/errorutils"
	"github.com/juju/juju/storage"
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
	stdCtx stdcontext.Context,
	ctx context.ProviderCallContext,
	rootDisk *storage.VolumeParams,
	envTags map[string]string,
) (string, error) {
	if rootDisk == nil {
		return "", nil
	}
	logger.Debugf("creating root disk encryption with parameters: %#v", *rootDisk)
	// The "encrypted" value may arrive as a bool or a string.
	encryptedStr, ok := rootDisk.Attributes[encryptedKey].(string)
	encrypted, _ := rootDisk.Attributes[encryptedKey].(bool)
	if !encrypted && ok {
		encrypted, _ = strconv.ParseBool(encryptedStr)
	}
	if !encrypted {
		logger.Debugf("encryption not enabled for root disk")
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
		envTagPtr[k] = to.StringPtr(v)
	}

	// See if the disk encryption set already exists.
	encryptionSetClient := compute.DiskEncryptionSetsClient{
		BaseClient: env.compute,
	}
	existingDes, err := encryptionSetClient.Get(stdCtx, env.resourceGroup, diskEncryptionSetName)
	if err != nil && !isNotFoundResult(existingDes.Response) {
		return "", errors.Trace(err)
	}
	// Record the identity of an existing disk encryption set
	// so we can maintain the access policy on the vault.
	var desIdentity *compute.EncryptionSetIdentity
	if err == nil {
		desIdentity = existingDes.Identity
	}
	// The vault name must be unique across the entire subscription.
	if len(vaultNamePrefix) > 15 {
		return "", errors.Errorf("vault name prefix %q too long, must be 15 characters or less", vaultNamePrefix)
	}

	env.mu.Lock()
	defer env.mu.Unlock()

	vaultName := fmt.Sprintf("%s-%s", vaultNamePrefix, env.config.Config.UUID()[:8])
	vault, vaultParams, err := env.ensureVault(stdCtx, vaultName, userID, envTagPtr, desIdentity)
	if err != nil {
		return "", errorutils.HandleCredentialError(errors.Annotatef(err, "creating vault %q", vaultName), ctx)
	}

	// Create a key in the vault.
	if keyName == "" {
		keyName = "disk-secret"
	}
	keyRef, err := env.createVaultKey(stdCtx, *vault.Properties.VaultURI, keyName)
	if err != nil {
		return "", errorutils.HandleCredentialError(errors.Annotatef(err, "creating vault key in %q", vaultName), ctx)
	}

	// We had an existing disk encryption set.
	if desIdentity != nil {
		return diskEncryptionSetID, nil
	}

	// Create the disk encryption set.
	desIdentity, err = env.ensureDiskEncryptionSet(stdCtx, diskEncryptionSetName, envTagPtr, vault.ID, keyRef)
	if err != nil {
		return "", errorutils.HandleCredentialError(errors.Annotatef(err, "creating vault key in %q", vaultName), ctx)
	}

	// Update the vault access policies to allow the disk encryption set to access the key.
	vaultClient := keyvault.VaultsClient{
		BaseClient: env.vault,
	}
	vaultAccessPolicies := *vaultParams.Properties.AccessPolicies
	vaultAccessPolicies = append(vaultAccessPolicies, vaultAccessPolicy(desIdentity))
	vaultParams.Properties.AccessPolicies = &vaultAccessPolicies
	vaultFuture, err := vaultClient.CreateOrUpdate(stdCtx, env.resourceGroup, vaultName, *vaultParams)
	if err != nil {
		return "", errorutils.HandleCredentialError(errors.Annotatef(err, "updating vault %q access policies ", vaultName), ctx)
	}
	if err := vaultFuture.WaitForCompletionRef(stdCtx, vaultClient.Client); err != nil {
		return "", errorutils.HandleCredentialError(errors.Annotatef(err, "creating vault %q", vaultName), ctx)
	}
	return diskEncryptionSetID, nil
}

func vaultAccessPolicy(desIdentity *compute.EncryptionSetIdentity) keyvault.AccessPolicyEntry {
	tenantID := uuid.FromStringOrNil(to.String(desIdentity.TenantID))
	return keyvault.AccessPolicyEntry{
		TenantID: &tenantID,
		ObjectID: desIdentity.PrincipalID,
		Permissions: &keyvault.Permissions{
			Keys: &[]keyvault.KeyPermissions{
				keyvault.KeyPermissionsWrapKey, keyvault.KeyPermissionsUnwrapKey,
				keyvault.KeyPermissionsList, keyvault.KeyPermissionsGet,
			},
		},
	}
}

// ensureDiskEncryptionSet creates or updates a disk encryption set
// to use the specified vault and key.
func (env *azureEnviron) ensureDiskEncryptionSet(
	stdCtx stdcontext.Context,
	encryptionSetName string,
	envTags map[string]*string,
	vaultID, vaultKey *string,
) (*compute.EncryptionSetIdentity, error) {
	logger.Debugf("ensure disk encryption set %q", encryptionSetName)
	encryptionSetClient := compute.DiskEncryptionSetsClient{
		BaseClient: env.compute,
	}
	diskEncryptionSetFuture, err := encryptionSetClient.CreateOrUpdate(stdCtx, env.resourceGroup, encryptionSetName, compute.DiskEncryptionSet{
		Location: to.StringPtr(env.location),
		Tags:     envTags,
		Identity: &compute.EncryptionSetIdentity{
			Type: compute.SystemAssigned,
		},
		EncryptionSetProperties: &compute.EncryptionSetProperties{
			ActiveKey: &compute.KeyVaultAndKeyReference{
				SourceVault: &compute.SourceVault{
					ID: vaultID,
				},
				KeyURL: vaultKey,
			},
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := diskEncryptionSetFuture.WaitForCompletionRef(stdCtx, encryptionSetClient.Client); err != nil {
		return nil, errors.Trace(err)
	}
	encryptionSetValue, err := encryptionSetClient.Get(stdCtx, env.resourceGroup, encryptionSetName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return encryptionSetValue.Identity, nil
}

// ensureVault creates a vault and adds an access policy for the
// specified disk encryption set identity.
func (env *azureEnviron) ensureVault(
	stdCtx stdcontext.Context,
	vaultName string,
	userID string,
	envTags map[string]*string,
	desIdentity *compute.EncryptionSetIdentity,
) (*keyvault.Vault, *keyvault.VaultCreateOrUpdateParameters, error) {
	logger.Debugf("ensure vault key %q", vaultName)
	vaultClient := keyvault.VaultsClient{
		BaseClient: env.vault,
	}
	vaultTenantID := uuid.FromStringOrNil(env.authorizer.tenantID)
	// Create the vault with full access for the tenant.
	allKeyPermissions := keyvault.PossibleKeyPermissionsValues()

	vaultAccessPolicies := []keyvault.AccessPolicyEntry{{
		TenantID: &vaultTenantID,
		ObjectID: to.StringPtr(azureauth.JujuApplicationObjectId),
		Permissions: &keyvault.Permissions{
			Keys: &allKeyPermissions,
		},
	}}
	if userID != "" {
		vaultAccessPolicies = append(vaultAccessPolicies, keyvault.AccessPolicyEntry{
			TenantID: &vaultTenantID,
			ObjectID: to.StringPtr(userID),
			Permissions: &keyvault.Permissions{
				Keys: &allKeyPermissions,
			},
		})
	}
	if desIdentity != nil {
		vaultAccessPolicies = append(vaultAccessPolicies, vaultAccessPolicy(desIdentity))
	}
	vaultParams := keyvault.VaultCreateOrUpdateParameters{
		Location: to.StringPtr(env.location),
		Tags:     envTags,
		Properties: &keyvault.VaultProperties{
			TenantID:                 &vaultTenantID,
			EnabledForDiskEncryption: to.BoolPtr(true),
			EnableSoftDelete:         to.BoolPtr(true),
			EnablePurgeProtection:    to.BoolPtr(true),
			CreateMode:               keyvault.CreateModeDefault,
			NetworkAcls: &keyvault.NetworkRuleSet{
				Bypass:        keyvault.AzureServices,
				DefaultAction: keyvault.Allow,
			},
			Sku: &keyvault.Sku{
				Family: to.StringPtr("A"),
				Name:   keyvault.Standard,
			},
			AccessPolicies: &vaultAccessPolicies,
		},
	}

	// Before creating check to see if the key vault has been soft deleted.
	softDeletedKeyVault, err := vaultClient.GetDeleted(stdCtx, vaultName, env.location)
	if err != nil {
		if !isNotFoundResult(softDeletedKeyVault.Response) && !isForbiddenResult(softDeletedKeyVault.Response) {
			return nil, nil, errors.Annotatef(err, "checking for an existing soft deleted vault %q", vaultName)
		}
	}
	if !isNotFoundResult(softDeletedKeyVault.Response) && !isForbiddenResult(softDeletedKeyVault.Response) {
		logger.Debugf("key vault %q has been soft deleted", vaultName)
		vaultParams.Properties.CreateMode = keyvault.CreateModeRecover
	}
	vaultFuture, err := vaultClient.CreateOrUpdate(stdCtx, env.resourceGroup, vaultName, vaultParams)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "creating vault")
	}
	if err := vaultFuture.WaitForCompletionRef(stdCtx, vaultClient.Client); err != nil {
		return nil, nil, errors.Trace(err)
	}
	vault, err := vaultClient.Get(stdCtx, env.resourceGroup, vaultName)
	if err != nil {
		return nil, nil, errors.Annotate(err, "getting created vault")
	}
	return &vault, &vaultParams, nil
}

func (env *azureEnviron) deleteVault(stdCtx stdcontext.Context, ctx context.ProviderCallContext, vaultName string) error {
	logger.Debugf("delete vault key %q", vaultName)
	vaultClient := keyvault.VaultsClient{
		BaseClient: env.vault,
	}
	result, err := vaultClient.Delete(stdCtx, env.resourceGroup, vaultName)
	if err != nil {
		errorutils.HandleCredentialError(err, ctx)
		if !isNotFoundResult(result) {
			return errors.Annotatef(err, "deleting vault key %q", vaultName)
		}
	}
	return nil
}

// createVaultKey creates, or recovers a soft deleted key,
// in the specified vault.
func (env *azureEnviron) createVaultKey(
	stdCtx stdcontext.Context,
	vaultBaseURI string,
	keyName string,
) (*string, error) {
	logger.Debugf("create vault key %q in %q", keyName, vaultBaseURI)
	resp, err := env.vaultKey.CreateKey(
		stdCtx,
		vaultBaseURI,
		keyName,
		keyvaultservices.KeyCreateParameters{
			KeyOps: &[]keyvaultservices.JSONWebKeyOperation{
				keyvaultservices.WrapKey,
				keyvaultservices.UnwrapKey,
			},
			// TODO(wallyworld) - make these configurable via storage pool attributes
			KeyAttributes: &keyvaultservices.KeyAttributes{
				Enabled: to.BoolPtr(true),
			},
			KeySize: to.Int32Ptr(4096),
			Kty:     keyvaultservices.RSA,
		})
	if err == nil {
		return resp.Key.Kid, nil
	}
	if !isConflictResult(resp.Response) {
		return nil, errors.Trace(err)
	}

	// If the key was previously soft deleted, recover it.
	recoveredKey, err := env.vaultKey.RecoverDeletedKey(stdCtx, vaultBaseURI, keyName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := env.waitForResource(
		func() error {
			_, err = env.vaultKey.GetKey(stdCtx, vaultBaseURI, keyName, "")
			return err
		},
	); err != nil {
		return nil, errors.Trace(err)
	}
	return recoveredKey.Key.Kid, nil
}

func (env *azureEnviron) waitForResource(getResource func() error) error {
	return retry.Call(retry.CallArgs{
		Func:        getResource,
		Attempts:    -1,
		Delay:       5 * time.Second,
		MaxDuration: 5 * time.Minute,
		Clock:       env.provider.config.RetryClock,
	})
}
