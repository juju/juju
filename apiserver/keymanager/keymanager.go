// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/set"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/utils/ssh"
)

var logger = loggo.GetLogger("juju.apiserver.keymanager")

func init() {
	common.RegisterStandardFacade("KeyManager", 0, NewKeyManagerAPI)
}

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
	state      *state.State
	resources  *common.Resources
	authorizer common.Authorizer
	canRead    func(string) bool
	canWrite   func(string) bool
	check      *common.BlockChecker
}

var _ KeyManager = (*KeyManagerAPI)(nil)

// NewKeyManagerAPI creates a new server-side keyupdater API end point.
func NewKeyManagerAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*KeyManagerAPI, error) {
	// Only clients and environment managers can access the key manager service.
	if !authorizer.AuthClient() && !authorizer.AuthEnvironManager() {
		return nil, common.ErrPerm
	}
	env, err := st.Environment()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// For gccgo interface comparisons, we need a Tag.
	owner := names.Tag(env.Owner())
	// TODO(wallyworld) - replace stub with real canRead function
	// For now, only admins can read authorised ssh keys.
	canRead := func(user string) bool {
		// Are we a machine agent operating as the system identity?
		if user == config.JujuSystemKey {
			_, ismachinetag := authorizer.GetAuthTag().(names.MachineTag)
			return ismachinetag
		}
		return authorizer.GetAuthTag() == owner
	}
	// TODO(wallyworld) - replace stub with real canWrite function
	// For now, only admins can write authorised ssh keys for users.
	// Machine agents can write the juju-system-key.
	canWrite := func(user string) bool {
		// Are we a machine agent writing the Juju system key.
		if user == config.JujuSystemKey {
			_, ismachinetag := authorizer.GetAuthTag().(names.MachineTag)
			return ismachinetag
		}
		// No point looking to see if the user exists as we are not
		// yet storing keys on the user.
		return authorizer.GetAuthTag() == owner
	}
	return &KeyManagerAPI{
		state:      st,
		resources:  resources,
		authorizer: authorizer,
		canRead:    canRead,
		canWrite:   canWrite,
		check:      common.NewBlockChecker(st),
	}, nil
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

	for i, entity := range arg.Entities.Entities {
		// NOTE: entity.Tag isn't a tag, but a username.
		if !api.canRead(entity.Tag) {
			results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		// All keys are global, no need to look up the user.
		if configErr == nil {
			results[i].Result = keyInfo
		}
		results[i].Error = common.ServerError(configErr)
	}
	return params.StringsResults{Results: results}, nil
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

func (api *KeyManagerAPI) writeSSHKeys(sshKeys []string) error {
	// Write out the new keys.
	keyStr := strings.Join(sshKeys, "\n")
	attrs := map[string]interface{}{config.AuthKeysConfig: keyStr}
	// TODO(waigani) 2014-03-17 bug #1293324
	// Pass in validation to ensure SSH keys
	// have not changed underfoot
	err := api.state.UpdateEnvironConfig(attrs, nil, nil)
	if err != nil {
		return fmt.Errorf("writing environ config: %v", err)
	}
	return nil
}

// currentKeyDataForAdd gathers data used when adding ssh keys.
func (api *KeyManagerAPI) currentKeyDataForAdd() (keys []string, fingerprints set.Strings, err error) {
	fingerprints = make(set.Strings)
	cfg, err := api.state.EnvironConfig()
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

	if !api.canWrite(arg.User) {
		return params.ErrorResults{}, common.ServerError(common.ErrPerm)
	}

	// For now, authorised keys are global, common to all users.
	sshKeys, currentFingerprints, err := api.currentKeyDataForAdd()
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
	err = api.writeSSHKeys(sshKeys)
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
func runSSHKeyImport(keyIds []string) map[string][]importedSSHKey {
	importResults := make(map[string][]importedSSHKey, len(keyIds))
	for _, keyId := range keyIds {
		keyInfo := []importedSSHKey{}
		output, err := RunSSHImportId(keyId)
		if err != nil {
			keyInfo = append(keyInfo, importedSSHKey{err: err})
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

	if !api.canWrite(arg.User) {
		return params.ErrorResults{}, common.ServerError(common.ErrPerm)
	}

	// For now, authorised keys are global, common to all users.
	sshKeys, currentFingerprints, err := api.currentKeyDataForAdd()
	if err != nil {
		return params.ErrorResults{}, common.ServerError(fmt.Errorf("reading current key data: %v", err))
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
			result.Results[i].Error = common.ServerError(errors.Errorf(strings.TrimSuffix(compoundErr, "\n")))
		}

	}
	err = api.writeSSHKeys(sshKeys)
	if err != nil {
		return params.ErrorResults{}, common.ServerError(err)
	}
	return result, nil
}

// currentKeyDataForDelete gathers data used when deleting ssh keys.
func (api *KeyManagerAPI) currentKeyDataForDelete() (
	keys map[string]string, invalidKeys []string, comments map[string]string, err error) {

	cfg, err := api.state.EnvironConfig()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reading current key data: %v", err)
	}
	// For now, authorised keys are global, common to all users.
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
	return keys, invalidKeys, comments, nil
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

	if !api.canWrite(arg.User) {
		return params.ErrorResults{}, common.ServerError(common.ErrPerm)
	}

	sshKeys, invalidKeys, keyComments, err := api.currentKeyDataForDelete()
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
		return params.ErrorResults{}, common.ServerError(fmt.Errorf("cannot delete all keys"))
	}

	err = api.writeSSHKeys(keysToWrite)
	if err != nil {
		return params.ErrorResults{}, common.ServerError(err)
	}
	return result, nil
}
