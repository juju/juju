// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	"os"
	"path"
	"strings"

	lxdapi "github.com/canonical/lxd/shared/api"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/lxdprofile"
	jujupaths "github.com/juju/juju/core/paths"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

// ReadLegacyCloudCredentials reads cloud credentials off disk for an old
// LXD controller, and returns them as a cloud.Credential with the
// certificate auth-type.
//
// If the credential files are missing from the filesystem, an error
// satisfying errors.IsNotFound will be returned.
func ReadLegacyCloudCredentials(readFile func(string) ([]byte, error)) (cloud.Credential, error) {
	var (
		jujuConfDir    = jujupaths.ConfDir(jujupaths.OSUnixLike)
		clientCertPath = path.Join(jujuConfDir, "lxd-client.crt")
		clientKeyPath  = path.Join(jujuConfDir, "lxd-client.key")
		serverCertPath = path.Join(jujuConfDir, "lxd-server.crt")
	)
	readFileString := func(path string) (string, error) {
		data, err := readFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				err = errors.NotFoundf("%s", path)
			}
			return "", errors.Trace(err)
		}
		return string(data), nil
	}
	clientCert, err := readFileString(clientCertPath)
	if err != nil {
		return cloud.Credential{}, errors.Annotate(err, "reading client certificate")
	}
	clientKey, err := readFileString(clientKeyPath)
	if err != nil {
		return cloud.Credential{}, errors.Annotate(err, "reading client key")
	}
	serverCert, err := readFileString(serverCertPath)
	if err != nil {
		return cloud.Credential{}, errors.Annotate(err, "reading server certificate")
	}
	return cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		credAttrServerCert: serverCert,
		credAttrClientCert: clientCert,
		credAttrClientKey:  clientKey,
	}), nil
}

// UpgradeOperations returns a list of UpgradeOperations for upgrading
// an Environ.
func (env *environ) UpgradeOperations(context.ProviderCallContext, environs.UpgradeOperationsParams) []environs.UpgradeOperation {
	return []environs.UpgradeOperation{
		{
			TargetVersion: ProviderVersion1,
			Steps: []environs.UpgradeStep{
				createProfilesStep{env},
			},
		},
	}
}

type createProfilesStep struct {
	env *environ
}

type profileAndInstance struct {
	profiles []string
	instance string
}

// Description returns a brief explanation what the upgrade step performs.
func (c createProfilesStep) Description() string {
	return "Create and assign LXD profiles to instances."
}

// Run executes the upgrade logic to create profiles with the new naming scheme.
func (c createProfilesStep) Run(ctx context.ProviderCallContext) error {
	server := c.env.server()

	logger.Debugf("running rename profile step for model %q", c.env.uuid)

	// Create a model profile if it doesn't exist.
	if err := c.env.initProfile(); err != nil {
		return errors.Annotatef(err, "initializing profile %q", c.env.profileName())
	}

	instances, err := c.env.AllInstances(ctx)
	if err != nil {
		return errors.Annotatef(err, "getting all instances")
	}

	var profilesToAttach []profileAndInstance

	prefix := fmt.Sprintf("juju-%s", c.env.name)
	prefixTrailingHyphen := fmt.Sprintf("%s-", prefix)
	// Create charm profiles for each instance.
	for _, inst := range instances {
		instanceID := string(inst.Id())
		profiles, err := server.GetContainerProfiles(instanceID)
		if err != nil {
			return errors.Annotatef(err, "getting container profiles for instance %q", instanceID)
		}

		var newProfiles []string
		for _, profileName := range profiles {
			isCharmProfile := strings.HasPrefix(profileName, prefixTrailingHyphen) &&
				// A new model profile may have been created in a previous run (and crashed),
				// which we want to skip here.
				// Since the shortID suffix could consist only of numbers, we might
				// mistakenly treat it as a charm profile (e.g. `juju-model-017671`).
				// So we explicitly exclude it here.
				profileName != c.env.profileName() &&
				lxdprofile.IsValidName(profileName)
			if !isCharmProfile {
				continue
			}

			profile, _, err := server.GetProfile(profileName)
			if err != nil {
				return errors.Annotatef(err, "getting profile %q", profileName)
			}

			appAndRevName := profileName[len(prefixTrailingHyphen):]
			newProfileName := fmt.Sprintf("%s-%s", c.env.profileName(), appAndRevName)
			if exists, err := server.HasProfile(newProfileName); err != nil {
				return errors.Annotatef(err, "checking existence of profile %q", newProfileName)
			} else if !exists {
				if err = server.CreateProfile(lxdapi.ProfilesPost{
					ProfilePut: lxdapi.ProfilePut{
						Config:      profile.Config,
						Description: profile.Description,
						Devices:     profile.Devices,
					},
					Name: newProfileName,
				}); err != nil {
					return errors.Annotatef(err, "creating a new profile %q", newProfileName)
				}
				logger.Debugf("created new charm profile %q for model %q", newProfileName, c.env.uuid)
			}

			// We still append the new profile even if they have been created / exists
			// so that we can try to attach it later. We could face a situation where
			// in a previous run, profile creation succeeds but the upgrader worker crashes before
			// attaching it. In this case, we don't want to re-create the profile but rather
			// pick it and try attaching it.
			newProfiles = append(newProfiles, newProfileName)
		}

		// Add the model profile.
		newProfiles = append(newProfiles, c.env.profileName())
		profilesToAttach = append(profilesToAttach, profileAndInstance{
			profiles: newProfiles,
			instance: instanceID,
		})
	}

	profilesToDeleteSet := set.NewStrings()

	// Attach new profiles and delete old profiles.
	for _, attach := range profilesToAttach {
		currentProfiles, err := server.GetContainerProfiles(attach.instance)
		if err != nil {
			return errors.Annotatef(err, "getting profiles for instance %q", attach.instance)
		}

		currentProfilesSet := set.NewStrings(currentProfiles...)
		toAttachSet := set.NewStrings(attach.profiles...)
		toDeleteSet := currentProfilesSet.Difference(toAttachSet)
		profilesToDeleteSet = profilesToDeleteSet.Union(toDeleteSet)
		// This may happen if on a previous run, the upgrade step has attached the new profiles
		// but crashed and retries. It is safe to skip it.
		if toAttachSet.IsEmpty() {
			continue
		}
		// Keep existing profiles that are not prefixed with juju-<model>
		for _, currentProfile := range currentProfiles {
			if strings.Contains(currentProfile, prefix) {
				continue
			}
			toAttachSet.Add(currentProfile)
		}

		// So that it's deterministic. It makes it easier to assert the sorted values in unit tests.
		toAttach := toAttachSet.SortedValues()
		// It's safe for the API to re-attach profiles that are already attached.
		// So there is no need to exclude profiles that are already attached.
		err = server.UpdateContainerProfiles(attach.instance, toAttach)
		if err != nil {
			return errors.Annotatef(err, "updating instance %q with profiles %+v", attach.instance, toAttach)
		}
		logger.Debugf("attached profiles %v to instance %q in model %q", toAttach, attach.instance, c.env.uuid)
	}

	// Delete old profiles except "default".
	for _, oldProfile := range profilesToDeleteSet.SortedValues() {
		if oldProfile == "default" {
			continue
		}
		if !strings.HasPrefix(oldProfile, prefix) {
			continue
		}

		logger.Debugf("deleting profile %q in model %q", oldProfile, c.env.uuid)
		// In reality, it's difficult to prove the ownership of the profile because the old
		// naming scheme isn't unique. We attempt to delete it here. If it's used by another instance
		// then it will fail, and we are okay with this (it'll be kept).
		err := server.DeleteProfile(oldProfile)
		if err != nil {
			if strings.Contains(err.Error(), profileNotFound) {
				continue
			}

			if strings.Contains(err.Error(), profileCannotBeDeleted) {
				logger.Warningf("deleting old profile %q failed because it is most likely used by another instance", oldProfile)
				continue
			}

			logger.Errorf("deleting old profile %q due to %q did not succeed, not a fatal error", oldProfile, err.Error())
		}
	}

	logger.Debugf("finishing rename profile step for model %q", c.env.uuid)

	return nil
}
