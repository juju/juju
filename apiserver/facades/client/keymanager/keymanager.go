// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/utils/v3"
	"github.com/juju/utils/v3/ssh"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.keymanager")

// The comment values used by juju internal ssh keys.
var internalComments = set.NewStrings([]string{"juju-client-key", "juju-system-key"}...)

// KeyManager defines the methods on the keymanager API end point.
type KeyManager interface {
	ListKeys(arg params.ListSSHKeys) (params.StringsResults, error)
	AddKeys(arg params.ModifyUserSSHKeys) (params.ErrorResults, error)
	DeleteKeys(arg params.ModifyUserSSHKeys) (params.ErrorResults, error)
	ImportKeys(arg params.ModifyUserSSHKeys) (params.ErrorResults, error)
}

// KeyManagerAPI implements the KeyUpdater interface and is the concrete
// implementation of the api end point.
type KeyManagerAPI struct {
	state      *state.State
	model      *state.Model
	resources  facade.Resources
	authorizer facade.Authorizer
	apiUser    names.UserTag
	check      *common.BlockChecker
}

var _ KeyManager = (*KeyManagerAPI)(nil)

func (api *KeyManagerAPI) checkCanRead(sshUser string) error {
	if err := api.checkCanWrite(sshUser); err == nil {
		return nil
	} else if err != apiservererrors.ErrPerm {
		return errors.Trace(err)
	}
	if sshUser == config.JujuSystemKey {
		// users cannot read the system key.
		return apiservererrors.ErrPerm
	}
	ok, err := common.HasPermission(
		api.state.UserPermission,
		api.apiUser,
		permission.ReadAccess,
		api.model.ModelTag(),
	)
	if err != nil {
		return errors.Trace(err)
	}
	if !ok {
		return apiservererrors.ErrPerm
	}
	return nil
}

func (api *KeyManagerAPI) checkCanWrite(sshUser string) error {
	if sshUser == config.JujuSystemKey {
		// users cannot modify the system key.
		return apiservererrors.ErrPerm
	}
	ok, err := common.HasModelAdmin(api.authorizer, api.state.ControllerTag(), names.NewModelTag(api.state.ModelUUID()))
	if err != nil {
		return errors.Trace(err)
	}
	if !ok {
		return apiservererrors.ErrPerm
	}
	return nil
}

// ListKeys returns the authorised ssh keys for the specified users.
func (api *KeyManagerAPI) ListKeys(arg params.ListSSHKeys) (params.StringsResults, error) {
	if len(arg.Entities.Entities) == 0 {
		return params.StringsResults{}, nil
	}
	results := make([]params.StringsResult, len(arg.Entities.Entities))

	// For now, authorised keys are global, common to all users.
	var keyInfo []string
	cfg, configErr := api.model.ModelConfig()
	if configErr == nil {
		keys := ssh.SplitAuthorisedKeys(cfg.AuthorizedKeys())
		keyInfo = parseKeys(keys, arg.Mode)
	}

	for i, entity := range arg.Entities.Entities {
		// NOTE: entity.Tag isn't a tag, but a username.
		if err := api.checkCanRead(entity.Tag); err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// All keys are global, no need to look up the user.
		if configErr == nil {
			results[i].Result = keyInfo
		}
		results[i].Error = apiservererrors.ServerError(configErr)
	}
	return params.StringsResults{Results: results}, nil
}

func parseKeys(keys []string, mode ssh.ListMode) (keyInfo []string) {
	for _, key := range keys {
		fingerprint, comment, err := ssh.KeyFingerprint(key)
		if err != nil {
			keyInfo = append(keyInfo, fmt.Sprintf("Invalid key: %v", key))
			continue
		}
		// Only including user added keys not internal ones.
		if internalComments.Contains(comment) {
			continue
		}
		if mode == ssh.FullKeys {
			keyInfo = append(keyInfo, key)
		} else {
			shortKey := fingerprint
			if comment != "" {
				shortKey += fmt.Sprintf(" (%s)", comment)
			}
			keyInfo = append(keyInfo, shortKey)
		}
	}
	return keyInfo
}

func (api *KeyManagerAPI) writeSSHKeys(sshKeys []string) error {
	// Write out the new keys.
	keyStr := strings.Join(sshKeys, "\n")
	attrs := map[string]interface{}{config.AuthorizedKeysKey: keyStr}
	// TODO(waigani) 2014-03-17 bug #1293324
	// Pass in validation to ensure SSH keys
	// have not changed underfoot
	err := api.model.UpdateModelConfig(attrs, nil)
	if err != nil {
		return fmt.Errorf("writing environ config: %v", err)
	}
	return nil
}

// currentKeyDataForAdd gathers data used when adding ssh keys.
func (api *KeyManagerAPI) currentKeyDataForAdd() (keys []string, fingerprints set.Strings, err error) {
	fingerprints = make(set.Strings)
	cfg, err := api.model.ModelConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("reading current key data: %v", err)
	}
	keys = ssh.SplitAuthorisedKeys(cfg.AuthorizedKeys())
	for _, key := range keys {
		fingerprint, _, err := ssh.KeyFingerprint(key)
		if err != nil {
			logger.Warningf("ignoring invalid ssh key %q: %v", key, err)
		}
		fingerprints.Add(fingerprint)
	}
	return keys, fingerprints, nil
}

// AddKeys adds new authorised ssh keys for the specified user.
func (api *KeyManagerAPI) AddKeys(arg params.ModifyUserSSHKeys) (params.ErrorResults, error) {
	if err := api.check.ChangeAllowed(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(arg.Keys)),
	}
	if len(arg.Keys) == 0 {
		return result, nil
	}

	if err := api.checkCanWrite(arg.User); err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(err)
	}

	// For now, authorised keys are global, common to all users.
	sshKeys, currentFingerprints, err := api.currentKeyDataForAdd()
	if err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(fmt.Errorf("reading current key data: %v", err))
	}

	// Ensure we are not going to add invalid or duplicate keys.
	result.Results = make([]params.ErrorResult, len(arg.Keys))
	for i, key := range arg.Keys {
		fingerprint, _, err := ssh.KeyFingerprint(key)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(fmt.Errorf("invalid ssh key: %s", key))
			continue
		}
		if currentFingerprints.Contains(fingerprint) {
			result.Results[i].Error = apiservererrors.ServerError(fmt.Errorf("duplicate ssh key: %s", key))
			continue
		}
		sshKeys = append(sshKeys, key)
	}
	err = api.writeSSHKeys(sshKeys)
	if err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(err)
	}
	return result, nil
}

type importedSSHKey struct {
	key         string
	fingerprint string
	err         error
}

// Override for testing
var RunSSHImportId = runSSHImportId

func runSSHImportId(keyId string) (string, error) {
	return utils.RunCommand("ssh-import-id", "-o", "-", keyId)
}

// runSSHKeyImport uses ssh-import-id to find the ssh keys for the specified key ids.
func runSSHKeyImport(keyIds []string) map[string][]importedSSHKey {
	importResults := make(map[string][]importedSSHKey, len(keyIds))
	for _, keyId := range keyIds {
		keyInfo := []importedSSHKey{}
		output, err := RunSSHImportId(keyId)
		if err != nil {
			keyInfo = append(keyInfo, importedSSHKey{err: err})
			importResults[keyId] = keyInfo
			continue
		}
		lines := strings.Split(output, "\n")
		hasKey := false
		for _, line := range lines {
			if !strings.HasPrefix(line, "ssh-") {
				continue
			}
			hasKey = true
			// ignore key comment (e.g., user@host)
			fingerprint, _, err := ssh.KeyFingerprint(line)
			keyInfo = append(keyInfo, importedSSHKey{
				key:         line,
				fingerprint: fingerprint,
				err:         errors.Annotatef(err, "invalid ssh key for %s", keyId),
			})
		}
		if !hasKey {
			keyInfo = append(keyInfo, importedSSHKey{
				err: errors.Errorf("invalid ssh key id: %s", keyId),
			})
		}
		importResults[keyId] = keyInfo
	}
	return importResults
}

// ImportKeys imports new authorised ssh keys from the specified key ids for the specified user.
func (api *KeyManagerAPI) ImportKeys(arg params.ModifyUserSSHKeys) (params.ErrorResults, error) {
	if err := api.check.ChangeAllowed(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(arg.Keys)),
	}
	if len(arg.Keys) == 0 {
		return result, nil
	}

	if err := api.checkCanWrite(arg.User); err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(err)
	}

	// For now, authorised keys are global, common to all users.
	sshKeys, currentFingerprints, err := api.currentKeyDataForAdd()
	if err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(fmt.Errorf("reading current key data: %v", err))
	}

	importedKeyInfo := runSSHKeyImport(arg.Keys)
	// Ensure we are not going to add invalid or duplicate keys.
	result.Results = make([]params.ErrorResult, len(importedKeyInfo))
	for i, key := range arg.Keys {
		compoundErr := ""
		for _, keyInfo := range importedKeyInfo[key] {
			if keyInfo.err != nil {
				compoundErr += fmt.Sprintf("%v\n", keyInfo.err)
				continue
			}
			if currentFingerprints.Contains(keyInfo.fingerprint) {
				compoundErr += fmt.Sprintf("%v\n", errors.Errorf("duplicate ssh key: %s", keyInfo.key))
				continue
			}
			sshKeys = append(sshKeys, keyInfo.key)
		}
		if compoundErr != "" {
			result.Results[i].Error = apiservererrors.ServerError(errors.New(strings.TrimSuffix(compoundErr, "\n")))
		}

	}
	err = api.writeSSHKeys(sshKeys)
	if err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(err)
	}
	return result, nil
}

// currentKeyDataForDelete gathers data used when deleting ssh keys.
func (api *KeyManagerAPI) currentKeyDataForDelete() (
	currentKeys []string, byFingerprint map[string]string, byComment map[string]string, err error) {

	cfg, err := api.model.ModelConfig()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reading current key data: %v", err)
	}
	// For now, authorised keys are global, common to all users.
	currentKeys = ssh.SplitAuthorisedKeys(cfg.AuthorizedKeys())

	// Make two maps that index keys by fingerprint and by comment for fast
	// lookup of keys to delete which may be given as either.
	byFingerprint = make(map[string]string)
	byComment = make(map[string]string)
	for _, key := range currentKeys {
		fingerprint, comment, err := ssh.KeyFingerprint(key)
		if err != nil {
			logger.Debugf("keeping unrecognised existing ssh key %q: %v", key, err)
			continue
		}
		byFingerprint[fingerprint] = key
		if comment != "" {
			byComment[comment] = key
		}
	}
	return currentKeys, byFingerprint, byComment, nil
}

// DeleteKeys deletes the authorised ssh keys for the specified user.
func (api *KeyManagerAPI) DeleteKeys(arg params.ModifyUserSSHKeys) (params.ErrorResults, error) {
	if err := api.check.ChangeAllowed(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(arg.Keys)),
	}
	if len(arg.Keys) == 0 {
		return result, nil
	}

	if err := api.checkCanWrite(arg.User); err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(err)
	}

	allKeys, byFingerprint, byComment, err := api.currentKeyDataForDelete()
	if err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(fmt.Errorf("reading current key data: %v", err))
	}

	// Record the keys to be deleted in the second pass.
	keysToDelete := make(set.Strings)

	// Find the keys corresponding to the specified key fingerprints or comments.
	for i, keyId := range arg.Keys {
		// Is given keyId a fingerprint?
		key, ok := byFingerprint[keyId]
		if ok {
			keysToDelete.Add(key)
			continue
		}
		// Not a fingerprint, is it a comment?
		key, ok = byComment[keyId]
		if ok {
			if internalComments.Contains(keyId) {
				result.Results[i].Error = apiservererrors.ServerError(fmt.Errorf("may not delete internal key: %s", keyId))
				continue
			}
			keysToDelete.Add(key)
			continue
		}
		result.Results[i].Error = apiservererrors.ServerError(fmt.Errorf("invalid ssh key: %s", keyId))
	}

	var keysToWrite []string

	// Add back only the keys that are not deleted, preserving the order.
	for _, key := range allKeys {
		if !keysToDelete.Contains(key) {
			keysToWrite = append(keysToWrite, key)
		}
	}

	if len(keysToWrite) == 0 {
		return params.ErrorResults{}, apiservererrors.ServerError(fmt.Errorf("cannot delete all keys"))
	}

	err = api.writeSSHKeys(keysToWrite)
	if err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(err)
	}
	return result, nil
}
