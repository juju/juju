// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/api/keymanager"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/utils/ssh"
)

func ensureSystemSSHKey(context Context) error {
	privateKey, publicKey, err := readOrMakeSystemIdentity(context)
	if err != nil {
		return errors.Trace(err)
	}
	if publicKey == "" {
		// privateKey was read from disk, so it exists.
		return nil
	}
	if err := updateAuthorizedKeys(context, publicKey); err != nil {
		return errors.Trace(err)
	}
	if err := writeSystemIdentity(context, privateKey); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Somewhere in the 1.20 cycle the system-identity was added to the
// state serving info collection in the database, however no migration
// step was added to take the identity file from disk and put it into the
// new value in the database.
func ensureSystemSSHKeyRedux(context Context) error {
	// If there is a system-identity in the database already, we don't need to
	// do anything.
	stateInfo, err := context.State().StateServingInfo()
	if err != nil {
		logger.Errorf("failed to read state serving info: %v", err)
		return errors.Trace(err)
	}
	if stateInfo.SystemIdentity != "" {
		logger.Infof("state serving info has a system identity already, all good")
		// We are good. One exists already.
		// Make sure that the agent thinks that it is the same.
		return updateSystemIdentityInAgentConfig(context, stateInfo.SystemIdentity)
	}

	privateKey, publicKey, err := readOrMakeSystemIdentity(context)
	if err != nil {
		logger.Errorf("failed to read or make system identity: %v", err)
		return errors.Trace(err)
	}

	if err := state.SetSystemIdentity(context.State(), privateKey); err != nil {
		if errors.Cause(err) == txn.ErrAborted {
			logger.Errorf("someone else has set system identity already")
			// Another state server upgrading concurrently has updated
			// the system identity so it is no longer empty. So discard
			// anything that was created, reread the system info and write
			// out the file.  We also assume that the other upgrade has
			// updated the authorized keys already.
			stateInfo, err := context.State().StateServingInfo()
			if err != nil {
				logger.Errorf("failed to read state serving info: %v", err)
				return errors.Trace(err)
			}
			if stateInfo.SystemIdentity == "" {
				logger.Errorf("but the transaction said it would be there...")
				return errors.New("system identity is not set")
			}
			if err := writeSystemIdentity(context, stateInfo.SystemIdentity); err != nil {
				logger.Errorf("failed to write the system identity file: %v", err)
				return errors.Trace(err)
			}
			return updateSystemIdentityInAgentConfig(context, stateInfo.SystemIdentity)
		}

		logger.Errorf("failed to set system identity: %v", err)
		return errors.Annotate(err, "cannot set state serving info")
	}

	if publicKey != "" {
		if err := writeSystemIdentity(context, privateKey); err != nil {
			return errors.Trace(err)
		}
	}
	return updateSystemIdentityInAgentConfig(context, privateKey)
}

// updateAuthorizedKeysForSystemIdentity makes sure that the authorized keys
// list is up to date with the system identity.  Due to changes in the way
// upgrades are done in 1.22, this part, which uses the API had to be split
// from the first part which used the state connection.
func updateAuthorizedKeysForSystemIdentity(context Context) error {
	agentInfo, ok := context.AgentConfig().StateServingInfo()
	if !ok {
		return errors.New("missing state serving info for the agent")
	}
	publicKey, err := ssh.PublicKey([]byte(agentInfo.SystemIdentity), config.JujuSystemKey)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(updateAuthorizedKeys(context, publicKey))
}

func updateAuthorizedKeys(context Context, publicKey string) error {
	// Look for an existing authorized key.
	logger.Infof("setting new authorized key for %q", publicKey)
	keyManager := keymanager.NewClient(context.APIState())

	result, err := keyManager.ListKeys(ssh.FullKeys, config.JujuSystemKey)
	if err != nil {
		return errors.Trace(err)
	}
	if result[0].Error != nil {
		return errors.Trace(result[0].Error)
	}
	keys := result[0].Result

	// Loop through the keys. If we find a key that matches the publicKey
	// then we are good, and done.  If the comment on the key is for the system identity
	// but it is not the same, remove it.
	var keysToRemove []string
	for _, key := range keys {
		// The list of keys returned don't have carriage returns, but the
		// publicKey does, so add one one before testing for equality.
		if (key + "\n") == publicKey {
			logger.Infof("system identity key already in authorized list")
			return nil
		}

		fingerprint, comment, err := ssh.KeyFingerprint(key)
		if err != nil {
			// Log the error, but it doesn't stop us doing what we need to do.
			logger.Errorf("bad key in authorized keys: %v", err)
		} else if comment == config.JujuSystemKey {
			keysToRemove = append(keysToRemove, fingerprint)
		}
	}
	if keysToRemove != nil {
		logger.Infof("removing %d keys", len(keysToRemove))
		results, err := keyManager.DeleteKeys(config.JujuSystemKey, keysToRemove...)
		if err != nil {
			// Log the error but continue.
			logger.Errorf("failed to remove keys: %v", err)
		} else {
			for _, err := range results {
				if err.Error != nil {
					// Log the error but continue.
					logger.Errorf("failed to remove key: %v", err.Error)
				}
			}
		}
	}

	errResults, err := keyManager.AddKeys(config.JujuSystemKey, publicKey)
	if err != nil {
		return errors.Annotate(err, "failed to update authorised keys with new system key")
	}
	if err := errResults[0].Error; err != nil {
		return errors.Annotate(err, "failed to update authorised keys with new system key")
	}
	return nil
}

func updateSystemIdentityInAgentConfig(context Context, systemIdentity string) error {
	agentInfo, ok := context.AgentConfig().StateServingInfo()
	if !ok {
		return errors.New("missing state serving info for the agent")
	}
	if agentInfo.SystemIdentity != systemIdentity {
		agentInfo.SystemIdentity = systemIdentity
		context.AgentConfig().SetStateServingInfo(agentInfo)
	}
	return nil
}

func readOrMakeSystemIdentity(context Context) (privateKey, publicKey string, err error) {
	identityFile := context.AgentConfig().SystemIdentityPath()
	// Don't generate a key unless we have to.
	keyExists, err := systemKeyExists(identityFile)
	if err != nil {
		return "", "", errors.Annotate(err, "failed to check system key exists")
	}
	if keyExists {
		logger.Infof("key exists, reading contents")

		// Read the contents.
		contents, err := ioutil.ReadFile(identityFile)
		if err != nil {
			return "", "", errors.Trace(err)
		}
		// If we are just reading the private key,
		return string(contents), "", nil
	}

	logger.Infof("generating new key")
	privateKey, publicKey, err = ssh.GenerateKey(config.JujuSystemKey)
	if err != nil {
		return "", "", errors.Annotate(err, "failed to create system key")
	}
	return privateKey, publicKey, nil
}

func writeSystemIdentity(context Context, privateKey string) error {
	identityFile := context.AgentConfig().SystemIdentityPath()
	logger.Infof("writing system identity to %q", identityFile)
	if err := ioutil.WriteFile(identityFile, []byte(privateKey), 0600); err != nil {
		return errors.Annotate(err, "failed to write identity file")
	}
	return nil
}

func systemKeyExists(identityFile string) (bool, error) {
	_, err := os.Stat(identityFile)
	if err == nil {
		logger.Infof("identity file %q exists", identityFile)
		return true, nil
	}
	if !os.IsNotExist(err) {
		logger.Infof("error looking for identity file %q: %v", identityFile, err)
		return false, err
	}
	logger.Infof("identity file %q does not exist", identityFile)
	return false, nil
}
