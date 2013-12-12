// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager

import (
	"strings"

	"fmt"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/utils/ssh"
)

// KeyManager defines the methods on the keymanager API end point.
type KeyManager interface {
	ListKeys(arg params.ListSSHKeys) (params.StringsResults, error)
	// soon
	// AddKeys()
	// DeleteKeys()
}

// KeyUpdaterAPI implements the KeyUpdater interface and is the concrete
// implementation of the api end point.
type KeyManagerAPI struct {
	state      *state.State
	resources  *common.Resources
	authorizer common.Authorizer
	getCanRead common.GetAuthFunc
}

var _ KeyManager = (*KeyManagerAPI)(nil)

// NewKeyUpdaterAPI creates a new server-side keyupdater API end point.
func NewKeyManagerAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*KeyManagerAPI, error) {
	// Only clients can access the key manager service.
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	// TODO(wallyworld) - replace stub with real canRead function
	// For now, only admins can read authorised ssh keys.
	getCanRead := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			return authorizer.GetAuthTag() == "user-admin"
		}, nil
	}
	return &KeyManagerAPI{state: st, resources: resources, authorizer: authorizer, getCanRead: getCanRead}, nil
}

// ListKeys returns the authorised ssh keys for the specified users.
func (api *KeyManagerAPI) ListKeys(arg params.ListSSHKeys) (params.StringsResults, error) {
	if len(arg.Entities.Entities) == 0 {
		return params.StringsResults{}, nil
	}
	results := make([]params.StringsResult, len(arg.Entities.Entities))

	// For now, authorised keys are global, common to all users.
	var keyInfo []string
	config, configErr := api.state.EnvironConfig()
	if configErr == nil {
		keysString := config.AuthorizedKeys()
		keyInfo = parseKeys(keysString, arg.Mode)
	}

	getCanRead, err := api.getCanRead()
	if err != nil {
		return params.StringsResults{}, err
	}
	for i, entity := range arg.Entities.Entities {
		if _, err := api.state.User(entity.Tag); err != nil {
			results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		if !getCanRead(entity.Tag) {
			results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		var err error
		if configErr == nil {
			results[i].Result = keyInfo
			err = nil
		} else {
			err = configErr
		}
		results[i].Error = common.ServerError(err)
	}
	return params.StringsResults{results}, nil
}

func parseKeys(text string, mode ssh.ListMode) (keyInfo []string) {
	keys := strings.Split(text, "\n")
	for _, key := range keys {
		if len(key) == 0 {
			continue
		}
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
