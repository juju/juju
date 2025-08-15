// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	lxdapi "github.com/canonical/lxd/shared/api"
	"github.com/juju/collections/set"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"os"
	"path"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	jujupaths "github.com/juju/juju/core/paths"
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

func (c createProfilesStep) Description() string {
	return "Create and assign LXD profiles to instances."
}

func (c createProfilesStep) Run(ctx context.ProviderCallContext) error {
	prefix := fmt.Sprintf("juju-%s", c.env.name)
	server := c.env.server()

	logger.Debugf("running create profile step for model %q", c.env.uuid)

	// Create a model profile.
	if exists, err := server.HasProfile(c.env.profileName()); err != nil {
		return errors.Annotatef(err, "check for existing profile %q", c.env.profileName())
	} else if !exists {
		if err := server.CreateProfileWithConfig(c.env.profileName(), c.env.profileCfg()); err != nil {
			return errors.Annotatef(err, "create profile %q with config %+v", c.env.provider, c.env.profileCfg())
		}
		logger.Debugf("created new profile %q for model %q", c.env.profileName(), c.env.uuid)
	}

	instances, err := c.env.AllInstances(ctx)
	if err != nil {
		return errors.Annotatef(err, "get all instances")
	}

	var profilesToAttach []profileAndInstance

	// Create charm profiles for each instance.
	for _, inst := range instances {
		instanceID := string(inst.Id())

		profiles, err := server.GetContainerProfiles(instanceID)
		if err != nil {
			return errors.Annotatef(err, "get container profiles for instance %q", instanceID)
		}

		var newProfiles []string
		for _, profileName := range profiles {
			isCharmProfile := strings.HasPrefix(profileName, prefix) && lxdprofile.IsValidName(profileName)
			if !isCharmProfile {
				continue
			}

			profile, _, err := server.GetProfile(profileName)
			if err != nil {
				return errors.Annotatef(err, "get profile %q", profileName)
			}

			appAndRevName := profileName[len(prefix)+1:]
			newProfileName := fmt.Sprintf("%s-%s", c.env.profileName(), appAndRevName)
			if exists, err := server.HasProfile(newProfileName); err != nil {
				return err
			} else if !exists {
				if err = server.CreateProfile(lxdapi.ProfilesPost{
					ProfilePut: lxdapi.ProfilePut{
						Config:      profile.Config,
						Description: profile.Description,
						Devices:     profile.Devices,
					},
					Name: newProfileName,
				}); err != nil {
					return errors.Annotatef(err, "create a new profile %q", newProfiles)
				}
				logger.Debugf("created new charm profile %q for model %q", newProfiles, c.env.uuid)
			}

			// We still append the new profile even if they have been created / exists
			// so that we can try to attach it later. We could face a situation where
			// in a previous run, profile creation succeeds but the upgrader worker crashes before
			// attaching it. In this case, we don't want to re-create the profile but rather
			// pick it and try attaching it.
			newProfiles = append(newProfiles, newProfileName)
		}

		newProfiles = append(newProfiles, c.env.profileName())
		profilesToAttach = append(profilesToAttach, profileAndInstance{
			profiles: newProfiles,
			instance: instanceID,
		})
	}

	// Attach new profiles and delete old profiles.
	for _, attach := range profilesToAttach {
		currentProfiles, err := server.GetContainerProfiles(attach.instance)
		if err != nil {
			return errors.Annotatef(err, "get profiles for instance %q", attach.instance)
		}

		currentProfilesSet := set.NewStrings(currentProfiles...)
		newProfilesSet := set.NewStrings(attach.profiles...)
		// Grab the new currentProfiles that has not been attached to the container.
		toAttachSet := newProfilesSet.Difference(currentProfilesSet)

		// This may happen if on a previous run, the upgrade step has attached the new profiles
		// but crashed and retries. It is safe to skip it.
		if toAttachSet.IsEmpty() {
			continue
		}
		toAttachSet.Add("default")
		toAttach := toAttachSet.Values()
		err = server.UpdateContainerProfiles(attach.instance, toAttach)
		if err != nil {
			return errors.Annotatef(err, "updating instance %q with profiles %+v", attach.instance, toAttach)
		}
		logger.Debugf("attached profiles %v to instance %q in model %q", toAttach, attach.instance, c.env.uuid)

		// Delete old profiles except "default".
		profilesToDelete := currentProfilesSet.Difference(newProfilesSet)
		for _, oldProfile := range profilesToDelete.Values() {
			if oldProfile != "default" && strings.HasPrefix(oldProfile, prefix) {
				logger.Debugf("deleting profile %q in model %q", oldProfile, c.env.uuid)
				// In reality, it's difficult to prove the ownership of the profile because the old
				// naming scheme isn't unique. We attempt to delete it here. If it's used by another instance
				// then it will fail, and we are okay with this (it'll be kept).
				err := server.DeleteProfile(oldProfile)
				if err != nil {
					logger.Errorf("failed to delete old profile %q due to %q, not a fatal error", oldProfile, err.Error())
				}
			}
		}
	}

	logger.Debugf("finishing create profile step for model %q", c.env.uuid)

	return nil
}
