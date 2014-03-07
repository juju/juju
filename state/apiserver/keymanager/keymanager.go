// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager

import (
	stderrors "errors"
	"fmt"
	"strings"

	"github.com/juju/loggo"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/juju-core/utils/ssh"
)

var logger = loggo.GetLogger("juju.state.apiserver.keymanager")

// KeyManager defines the methods on the keymanager API end point.
type KeyManager interface {
	ListKeys(arg params.ListSSHKeys) (params.StringsResults, error)
	AddKeys(arg params.ModifyUserSSHKeys) (params.ErrorResults, error)
	DeleteKeys(arg params.ModifyUserSSHKeys) (params.ErrorResults, error)
	ImportKeys(arg params.ModifyUserSSHKeys) (params.ErrorResults, error)
}

// KeyUpdaterAPI implements the KeyUpdater interface and is the concrete
// implementation of the api end point.
type KeyManagerAPI struct {
	state       *state.State
	resources   *common.Resources
	authorizer  common.Authorizer
	getCanRead  common.GetAuthFunc
	getCanWrite common.GetAuthFunc
}

var _ KeyManager = (*KeyManagerAPI)(nil)

// NewKeyUpdaterAPI creates a new server-side keyupdater API end point.
func NewKeyManagerAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*KeyManagerAPI, error) {
	// Only clients and environment managers can access the key manager service.
	if !authorizer.AuthClient() && !authorizer.AuthEnvironManager() {
		return nil, common.ErrPerm
	}
	// TODO(wallyworld) - replace stub with real canRead function
	// For now, only admins can read authorised ssh keys.
	getCanRead := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			return authorizer.GetAuthTag() == "user-admin"
		}, nil
	}
	// TODO(wallyworld) - replace stub with real canWrite function
	// For now, only admins can write authorised ssh keys for users.
	// Machine agents can write the juju-system-key.
	getCanWrite := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			// Are we a machine agent writing the Juju system key.
			if tag == config.JujuSystemKey {
				_, _, err := names.ParseTag(authorizer.GetAuthTag(), names.MachineTagKind)
				return err == nil
			}
			// Are we writing the auth key for a user.
			if _, err := st.User(tag); err != nil {
				return false
			}
			return authorizer.GetAuthTag() == "user-admin"
		}, nil
	}
	return &KeyManagerAPI{
		state: st, resources: resources, authorizer: authorizer, getCanRead: getCanRead, getCanWrite: getCanWrite}, nil
}

// ListKeys returns the authorised ssh keys for the specified users.
func (api *KeyManagerAPI) ListKeys(arg params.ListSSHKeys) (params.StringsResults, error) {
	if len(arg.Entities.Entities) == 0 {
		return params.StringsResults{}, nil
	}
	results := make([]params.StringsResult, len(arg.Entities.Entities))

	// For now, authorised keys are global, common to all users.
	var keyInfo []string
	cfg, configErr := api.state.EnvironConfig()
	if configErr == nil {
		keys := ssh.SplitAuthorisedKeys(cfg.AuthorizedKeys())
		keyInfo = parseKeys(keys, arg.Mode)
	}

	canRead, err := api.getCanRead()
	if err != nil {
		return params.StringsResults{}, err
	}
	for i, entity := range arg.Entities.Entities {
		if !canRead(entity.Tag) {
			results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		if _, err := api.state.User(entity.Tag); err != nil {
			if errors.IsNotFoundError(err) {
				results[i].Error = common.ServerError(common.ErrPerm)
			} else {
				results[i].Error = common.ServerError(err)
			}
			continue
		}
		var err error
		if configErr == nil {
			results[i].Result = keyInfo
		} else {
			err = configErr
		}
		results[i].Error = common.ServerError(err)
	}
	return params.StringsResults{results}, nil
}

func parseKeys(keys []string, mode ssh.ListMode) (keyInfo []string) {
	for _, key := range keys {
		fingerprint, comment, err := ssh.KeyFingerprint(key)
		if err != nil {
			keyInfo = append(keyInfo, fmt.Sprintf("Invalid key: %v", key))
		} else {
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
	}
	return keyInfo
}

func (api *KeyManagerAPI) writeSSHKeys(currentConfig *config.Config, sshKeys []string) error {
	// Write out the new keys.
	keyStr := strings.Join(sshKeys, "\n")
	newConfig, err := currentConfig.Apply(map[string]interface{}{config.AuthKeysConfig: keyStr})
	if err != nil {
		return fmt.Errorf("creating new environ config: %v", err)
	}
	err = api.state.SetEnvironConfig(newConfig, currentConfig)
	if err != nil {
		return fmt.Errorf("writing environ config: %v", err)
	}
	return nil
}

// currentKeyDataForAdd gathers data used when adding ssh keys.
func (api *KeyManagerAPI) currentKeyDataForAdd() (keys []string, fingerprints *set.Strings, cfg *config.Config, err error) {
	fp := set.NewStrings()
	fingerprints = &fp
	cfg, err = api.state.EnvironConfig()
	if err != nil {
		return nil, nil, nil, err
	}
	keys = ssh.SplitAuthorisedKeys(cfg.AuthorizedKeys())
	for _, key := range keys {
		fingerprint, _, err := ssh.KeyFingerprint(key)
		if err != nil {
			logger.Warningf("ignoring invalid ssh key %q: %v", key, err)
		}
		fingerprints.Add(fingerprint)
	}
	return keys, fingerprints, cfg, nil
}

// AddKeys adds new authorised ssh keys for the specified user.
func (api *KeyManagerAPI) AddKeys(arg params.ModifyUserSSHKeys) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(arg.Keys)),
	}
	if len(arg.Keys) == 0 {
		return result, nil
	}

	canWrite, err := api.getCanWrite()
	if err != nil {
		return params.ErrorResults{}, common.ServerError(err)
	}
	if !canWrite(arg.User) {
		return params.ErrorResults{}, common.ServerError(common.ErrPerm)
	}

	// For now, authorised keys are global, common to all users.
	sshKeys, currentFingerprints, currentConfig, err := api.currentKeyDataForAdd()
	if err != nil {
		return params.ErrorResults{}, common.ServerError(fmt.Errorf("reading current key data: %v", err))
	}

	// Ensure we are not going to add invalid or duplicate keys.
	result.Results = make([]params.ErrorResult, len(arg.Keys))
	for i, key := range arg.Keys {
		fingerprint, _, err := ssh.KeyFingerprint(key)
		if err != nil {
			result.Results[i].Error = common.ServerError(fmt.Errorf("invalid ssh key: %s", key))
			continue
		}
		if currentFingerprints.Contains(fingerprint) {
			result.Results[i].Error = common.ServerError(fmt.Errorf("duplicate ssh key: %s", key))
			continue
		}
		sshKeys = append(sshKeys, key)
	}
	err = api.writeSSHKeys(currentConfig, sshKeys)
	if err != nil {
		return params.ErrorResults{}, common.ServerError(err)
	}
	return result, nil
}

type importedSSHKey struct {
	key         string
	fingerprint string
	err         error
}

//  Override for testing
var RunSSHImportId = runSSHImportId

func runSSHImportId(keyId string) (string, error) {
	return utils.RunCommand("ssh-import-id", "-o", "-", keyId)
}

// runSSHKeyImport uses ssh-import-id to find the ssh keys for the specified key ids.
func runSSHKeyImport(keyIds []string) []importedSSHKey {
	keyInfo := make([]importedSSHKey, len(keyIds))
	for i, keyId := range keyIds {
		output, err := RunSSHImportId(keyId)
		if err != nil {
			keyInfo[i].err = err
			continue
		}
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if !strings.HasPrefix(line, "ssh-") {
				continue
			}
			keyInfo[i].fingerprint, _, keyInfo[i].err = ssh.KeyFingerprint(line)
			if err == nil {
				keyInfo[i].key = line
			}
		}
		if keyInfo[i].key == "" {
			keyInfo[i].err = fmt.Errorf("invalid ssh key id: %s", keyId)
		}
	}
	return keyInfo
}

// ImportKeys imports new authorised ssh keys from the specified key ids for the specified user.
func (api *KeyManagerAPI) ImportKeys(arg params.ModifyUserSSHKeys) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(arg.Keys)),
	}
	if len(arg.Keys) == 0 {
		return result, nil
	}

	canWrite, err := api.getCanWrite()
	if err != nil {
		return params.ErrorResults{}, common.ServerError(err)
	}
	if !canWrite(arg.User) {
		return params.ErrorResults{}, common.ServerError(common.ErrPerm)
	}

	// For now, authorised keys are global, common to all users.
	sshKeys, currentFingerprints, currentConfig, err := api.currentKeyDataForAdd()
	if err != nil {
		return params.ErrorResults{}, common.ServerError(fmt.Errorf("reading current key data: %v", err))
	}

	importedKeyInfo := runSSHKeyImport(arg.Keys)
	// Ensure we are not going to add invalid or duplicate keys.
	result.Results = make([]params.ErrorResult, len(importedKeyInfo))
	for i, keyInfo := range importedKeyInfo {
		if keyInfo.err != nil {
			result.Results[i].Error = common.ServerError(keyInfo.err)
			continue
		}
		if currentFingerprints.Contains(keyInfo.fingerprint) {
			result.Results[i].Error = common.ServerError(fmt.Errorf("duplicate ssh key: %s", keyInfo.key))
			continue
		}
		sshKeys = append(sshKeys, keyInfo.key)
	}
	err = api.writeSSHKeys(currentConfig, sshKeys)
	if err != nil {
		return params.ErrorResults{}, common.ServerError(err)
	}
	return result, nil
}

// currentKeyDataForAdd gathers data used when deleting ssh keys.
func (api *KeyManagerAPI) currentKeyDataForDelete() (
	keys map[string]string, invalidKeys []string, comments map[string]string, cfg *config.Config, err error) {

	// For now, authorised keys are global, common to all users.
	cfg, err = api.state.EnvironConfig()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("reading current key data: %v", err)
	}
	existingSSHKeys := ssh.SplitAuthorisedKeys(cfg.AuthorizedKeys())

	// Build up a map of keys indexed by fingerprint, and fingerprints indexed by comment
	// so we can easily get the key represented by each keyId, which may be either a fingerprint
	// or comment.
	keys = make(map[string]string)
	comments = make(map[string]string)
	for _, key := range existingSSHKeys {
		fingerprint, comment, err := ssh.KeyFingerprint(key)
		if err != nil {
			logger.Debugf("keeping unrecognised existing ssh key %q: %v", key, err)
			invalidKeys = append(invalidKeys, key)
			continue
		}
		keys[fingerprint] = key
		if comment != "" {
			comments[comment] = fingerprint
		}
	}
	return keys, invalidKeys, comments, cfg, nil
}

// DeleteKeys deletes the authorised ssh keys for the specified user.
func (api *KeyManagerAPI) DeleteKeys(arg params.ModifyUserSSHKeys) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(arg.Keys)),
	}
	if len(arg.Keys) == 0 {
		return result, nil
	}

	canWrite, err := api.getCanWrite()
	if err != nil {
		return params.ErrorResults{}, common.ServerError(err)
	}
	if !canWrite(arg.User) {
		return params.ErrorResults{}, common.ServerError(common.ErrPerm)
	}

	sshKeys, invalidKeys, keyComments, currentConfig, err := api.currentKeyDataForDelete()
	if err != nil {
		return params.ErrorResults{}, common.ServerError(fmt.Errorf("reading current key data: %v", err))
	}

	// We keep all existing invalid keys.
	keysToWrite := invalidKeys

	// Find the keys corresponding to the specified key fingerprints or comments.
	for i, keyId := range arg.Keys {
		// assume keyId may be a fingerprint
		fingerprint := keyId
		_, ok := sshKeys[keyId]
		if !ok {
			// keyId is a comment
			fingerprint, ok = keyComments[keyId]
		}
		if !ok {
			result.Results[i].Error = common.ServerError(fmt.Errorf("invalid ssh key: %s", keyId))
		}
		// We found the key to delete so remove it from those we wish to keep.
		delete(sshKeys, fingerprint)
	}
	for _, key := range sshKeys {
		keysToWrite = append(keysToWrite, key)
	}
	if len(keysToWrite) == 0 {
		return params.ErrorResults{}, common.ServerError(stderrors.New("cannot delete all keys"))
	}

	err = api.writeSSHKeys(currentConfig, keysToWrite)
	if err != nil {
		return params.ErrorResults{}, common.ServerError(err)
	}
	return result, nil
}
