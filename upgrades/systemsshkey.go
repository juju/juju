// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/keymanager"
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
		return nil
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
			return nil
		}

		logger.Errorf("failed to set system identity: %v", err)
		return errors.Annotate(err, "cannot set state serving info")
	}

	// If the public key is empty then the system identity was read
	// from the file on disk already, so we are done.
	if publicKey == "" {
		logger.Infof("publicKey is empty, so we have read it from disk, so should be in auth keys already")
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

func updateAuthorizedKeys(context Context, publicKey string) error {
	logger.Infof("setting new authorized key for %q", publicKey)

	keyManager := keymanager.NewClient(context.APIState())
	errResults, err := keyManager.AddKeys(config.JujuSystemKey, publicKey)
	apiErr := err
	if apiErr == nil {
		apiErr = errResults[0].Error
	}
	if err != nil || errResults[0].Error != nil {
		return errors.Annotate(apiErr, "failed to update authorised keys with new system key")
	}
	return nil
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
		return true, nil
	}
	if !os.IsNotExist(err) {
		return false, err
	}
	return false, nil
}
