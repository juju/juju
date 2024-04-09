// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/utils/v4"
	"github.com/juju/utils/v4/ssh"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
)

// Logger is the minimal logging interface required by the KeyManagerAPI.
type Logger interface {
	Debugf(string, ...any)
	Warningf(string, ...any)
}

// jujuKeyCommentIdentifiers is the set of identifiers in use by Juju system
// keys that may be stored within model config.
var jujuKeyCommentIdentifiers = set.NewStrings("juju-client-key", config.JujuSystemKey)

// KeyManagerAPI provides api endpoints for manipulating ssh keys
type KeyManagerAPI struct {
	model                    Model
	authorizer               facade.Authorizer
	check                    BlockChecker
	configSchemaSourceGetter config.ConfigSchemaSourceGetter

	controllerTag names.ControllerTag
	logger        Logger
}

func (api *KeyManagerAPI) checkCanRead(sshUser string) error {
	if err := api.checkCanWrite(sshUser); err == nil {
		return nil
	} else if err != apiservererrors.ErrPerm {
		return errors.Trace(err)
	}
	err := api.authorizer.HasPermission(permission.ReadAccess, api.model.ModelTag())
	return err
}

func (api *KeyManagerAPI) checkCanWrite(sshUser string) error {
	ok, err := common.HasModelAdmin(api.authorizer, api.controllerTag, api.model.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !ok {
		return apiservererrors.ErrPerm
	}
	return nil
}

// ListKeys returns the authorised ssh keys for the specified users.
func (api *KeyManagerAPI) ListKeys(ctx context.Context, arg params.ListSSHKeys) (params.StringsResults, error) {
	if len(arg.Entities.Entities) == 0 {
		return params.StringsResults{}, nil
	}

	// For now, authorised keys are global, common to all users.
	cfg, err := api.model.ModelConfig(ctx)
	if err != nil {
		// Return error embedded in results for compatibility.
		// TODO: Change this to a call-error on next facade bump
		results := transform.Slice(arg.Entities.Entities, func(_ params.Entity) params.StringsResult {
			return params.StringsResult{Error: apiservererrors.ServerError(err)}
		})
		return params.StringsResults{Results: results}, nil
	}
	keys := ssh.SplitAuthorisedKeys(cfg.AuthorizedKeys())
	keyInfo := parseKeys(keys, arg.Mode)

	results := transform.Slice(arg.Entities.Entities, func(entity params.Entity) params.StringsResult {
		// NOTE: entity.Tag isn't a tag, but a username.
		if err := api.checkCanRead(entity.Tag); err != nil {
			return params.StringsResult{Error: apiservererrors.ServerError(err)}
		}
		// All keys are global, no need to look up the user.
		return params.StringsResult{Result: keyInfo}
	})
	return params.StringsResults{Results: results}, nil
}

func parseKeys(keys []string, mode ssh.ListMode) (keyInfo []string) {
	for _, key := range keys {
		fingerprint, comment, err := ssh.KeyFingerprint(key)
		if err != nil {
			keyInfo = append(keyInfo, fmt.Sprintf("Invalid key: %v", key))
			continue
		}
		if jujuKeyCommentIdentifiers.Contains(comment) {
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
	err := api.model.UpdateModelConfig(api.configSchemaSourceGetter, attrs, nil)
	if err != nil {
		return fmt.Errorf("writing environ config: %v", err)
	}
	return nil
}

// currentKeyDataForAdd gathers data used when adding ssh keys.
func (api *KeyManagerAPI) currentKeyDataForAdd(ctx context.Context) (keys []string, fingerprints set.Strings, err error) {
	fingerprints = make(set.Strings)
	cfg, err := api.model.ModelConfig(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("reading current key data: %v", err)
	}
	keys = ssh.SplitAuthorisedKeys(cfg.AuthorizedKeys())
	for _, key := range keys {
		fingerprint, _, err := ssh.KeyFingerprint(key)
		if err != nil {
			api.logger.Warningf("ignoring invalid ssh key %q: %v", key, err)
			continue
		}
		fingerprints.Add(fingerprint)
	}
	return keys, fingerprints, nil
}

// AddKeys adds new authorised ssh keys for the specified user.
func (api *KeyManagerAPI) AddKeys(ctx context.Context, arg params.ModifyUserSSHKeys) (params.ErrorResults, error) {
	if err := api.checkCanWrite(arg.User); err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(err)
	}
	if err := api.check.ChangeAllowed(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	if len(arg.Keys) == 0 {
		return params.ErrorResults{}, nil
	}

	// For now, authorised keys are global, common to all users.
	sshKeys, currentFingerprints, err := api.currentKeyDataForAdd(ctx)
	if err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(fmt.Errorf("reading current key data: %v", err))
	}

	// Ensure we are not going to add invalid or duplicate keys.
	results := transform.Slice(arg.Keys, func(key string) params.ErrorResult {
		fingerprint, comment, err := ssh.KeyFingerprint(key)
		if err != nil {
			return params.ErrorResult{Error: apiservererrors.ServerError(fmt.Errorf("invalid ssh key: %s", key))}
		}
		if jujuKeyCommentIdentifiers.Contains(comment) {
			return params.ErrorResult{Error: apiservererrors.ServerError(fmt.Errorf("may not add key with comment %s: %s", comment, key))}
		}
		if currentFingerprints.Contains(fingerprint) {
			return params.ErrorResult{Error: apiservererrors.ServerError(fmt.Errorf("duplicate ssh key: %s", key))}
		}
		currentFingerprints.Add(fingerprint)
		sshKeys = append(sshKeys, key)
		return params.ErrorResult{}
	})

	err = api.writeSSHKeys(sshKeys)
	if err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(err)
	}
	return params.ErrorResults{Results: results}, nil
}

type importedSSHKey struct {
	key         string
	fingerprint string
	comment     string
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
			fingerprint, comment, err := ssh.KeyFingerprint(line)
			keyInfo = append(keyInfo, importedSSHKey{
				key:         line,
				fingerprint: fingerprint,
				comment:     comment,
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
func (api *KeyManagerAPI) ImportKeys(ctx context.Context, arg params.ModifyUserSSHKeys) (params.ErrorResults, error) {
	if err := api.checkCanWrite(arg.User); err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(err)
	}
	if err := api.check.ChangeAllowed(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	if len(arg.Keys) == 0 {
		return params.ErrorResults{}, nil
	}

	// For now, authorised keys are global, common to all users.
	sshKeys, currentFingerprints, err := api.currentKeyDataForAdd(ctx)
	if err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(fmt.Errorf("reading current key data: %v", err))
	}

	importedKeyInfo := runSSHKeyImport(arg.Keys)

	// Ensure we are not going to add invalid or duplicate keys.
	results := transform.Slice(arg.Keys, func(key string) params.ErrorResult {
		compoundErr := ""
		for _, keyInfo := range importedKeyInfo[key] {
			if keyInfo.err != nil {
				compoundErr += fmt.Sprintf("%v\n", keyInfo.err)
				continue
			}
			if jujuKeyCommentIdentifiers.Contains(keyInfo.comment) {
				compoundErr += fmt.Sprintf("%v\n", errors.Errorf("may not add key with comment %s: %s", keyInfo.comment, keyInfo.key))
				continue
			}
			if currentFingerprints.Contains(keyInfo.fingerprint) {
				compoundErr += fmt.Sprintf("%v\n", errors.Errorf("duplicate ssh key: %s", keyInfo.key))
				continue
			}
			sshKeys = append(sshKeys, keyInfo.key)
		}
		if compoundErr != "" {
			return params.ErrorResult{Error: apiservererrors.ServerError(errors.Errorf(strings.TrimSuffix(compoundErr, "\n")))}
		}
		return params.ErrorResult{}
	})

	err = api.writeSSHKeys(sshKeys)
	if err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(err)
	}
	return params.ErrorResults{Results: results}, nil
}

type keyDataForDelete struct {
	allKeys       []string
	byFingerprint map[string]string
	byComment     map[string]string
	invalidKeys   map[string]string
}

// currentKeyDataForDelete gathers data used when deleting ssh keys.
func (api *KeyManagerAPI) currentKeyDataForDelete(ctx context.Context) (keyDataForDelete, error) {

	cfg, err := api.model.ModelConfig(ctx)
	if err != nil {
		return keyDataForDelete{}, fmt.Errorf("reading current key data: %v", err)
	}
	// For now, authorised keys are global, common to all users.
	currentKeys := ssh.SplitAuthorisedKeys(cfg.AuthorizedKeys())

	// Make two maps that index keys by fingerprint and by comment for fast
	// lookup of keys to delete which may be given as either.
	byFingerprint := make(map[string]string)
	byComment := make(map[string]string)
	invalidKeys := make(map[string]string)
	for _, key := range currentKeys {
		fingerprint, comment, err := ssh.KeyFingerprint(key)
		if err != nil {
			api.logger.Debugf("invalid existing ssh key %q: %v", key, err)
			invalidKeys[key] = key
			continue
		}
		byFingerprint[fingerprint] = key
		if comment != "" {
			byComment[comment] = key
		}
	}
	data := keyDataForDelete{
		allKeys:       currentKeys,
		byFingerprint: byFingerprint,
		byComment:     byComment,
		invalidKeys:   invalidKeys,
	}
	return data, nil
}

// DeleteKeys deletes the authorised ssh keys for the specified user.
func (api *KeyManagerAPI) DeleteKeys(ctx context.Context, arg params.ModifyUserSSHKeys) (params.ErrorResults, error) {
	if err := api.checkCanWrite(arg.User); err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(err)
	}
	if err := api.check.RemoveAllowed(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	if len(arg.Keys) == 0 {
		return params.ErrorResults{}, nil
	}

	keyData, err := api.currentKeyDataForDelete(ctx)
	if err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(fmt.Errorf("reading current key data: %v", err))
	}

	// Record the keys to be deleted in the second pass.
	keysToDelete := make(set.Strings)

	results := transform.Slice(arg.Keys, func(keyId string) params.ErrorResult {
		// Is given keyId a fingerprint?
		key, ok := keyData.byFingerprint[keyId]
		if ok {
			keysToDelete.Add(key)
			return params.ErrorResult{}
		}
		// Not a fingerprint, is it a comment?
		key, ok = keyData.byComment[keyId]
		if ok {
			if jujuKeyCommentIdentifiers.Contains(keyId) {
				return params.ErrorResult{Error: apiservererrors.ServerError(fmt.Errorf("may not delete internal key: %s", keyId))}
			}
			keysToDelete.Add(key)
			return params.ErrorResult{}
		}
		// Allow invalid keys to be deleted by writing out key verbatim.
		key, ok = keyData.invalidKeys[keyId]
		if ok {
			keysToDelete.Add(key)
			return params.ErrorResult{}
		}
		return params.ErrorResult{Error: apiservererrors.ServerError(fmt.Errorf("key not found: %s", keyId))}
	})

	var keysToWrite []string

	// Add back only the keys that are not deleted, preserving the order.
	for _, key := range keyData.allKeys {
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
	return params.ErrorResults{Results: results}, nil
}
