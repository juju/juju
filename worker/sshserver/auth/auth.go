// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auth

import (
	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jujussh "github.com/juju/utils/v3/ssh"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/state"
)

// Logger holds the methods required to log messages.
type Logger interface {
	Errorf(string, ...interface{})
}

// Authenticator is the struct to authorize users' ssh connections.
type Authenticator struct {
	statePool *state.StatePool
	logger    Logger
}

// NewAuthenticator create an Authorizer.
func NewAuthenticator(sp *state.StatePool, logger Logger) (Authenticator, error) {
	if sp == nil {
		return Authenticator{}, errors.Errorf("StatePool can't be nil.")
	}
	if logger == nil {
		return Authenticator{}, errors.Errorf("Logger can't be nil.")
	}
	return Authenticator{
		statePool: sp,
		logger:    logger,
	}, nil
}

// authorizedKeysPerModel collects the authorized keys given a model uuid.
func (sm Authenticator) authorizedKeysPerModel(uuid string) ([]string, error) {
	model, p, err := sm.statePool.GetModel(uuid)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer p.Release()
	cfg, err := model.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}
	keys := jujussh.SplitAuthorisedKeys(cfg.AuthorizedKeys())
	return keys, nil
}

// PublicKeyAuthentication extracts the models a user has access to, get all the authorized keys and search for a match.
// If it's found it returns true, in case of errors or no-match returns false.
func (sm Authenticator) PublicKeyAuthentication(userTag names.UserTag, publicKey ssh.PublicKey) bool {
	systemState, err := sm.statePool.SystemState()
	if err != nil {
		sm.logger.Errorf("failed to get system state: %v", err)
		return false
	}
	modelUUIDs, err := systemState.ModelUUIDsForUser(userTag)
	if err != nil {
		sm.logger.Errorf("failed to get model uuids for user: %v", err)
		return false
	}
	for _, uuid := range modelUUIDs {
		authKeys, err := sm.authorizedKeysPerModel(uuid)
		if err != nil {
			sm.logger.Errorf("failed to get authorized key for model: %v", err)
			return false
		}
		for _, authKey := range authKeys {
			pubKey, _, _, _, err := gossh.ParseAuthorizedKey([]byte(authKey))
			if err != nil {
				sm.logger.Errorf("failed to parse user public key: %v", err)
				continue
			}
			if ssh.KeysEqual(publicKey, pubKey) {
				return true
			}
		}
	}
	sm.logger.Errorf("failed to find a matching public key")
	return false
}
