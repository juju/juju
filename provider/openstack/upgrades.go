// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"fmt"
	"github.com/go-goose/goose/v5/neutron"
	"github.com/juju/errors"
	"github.com/juju/juju/environs/context"
	"github.com/juju/retry"
	"github.com/juju/version/v2"
	"regexp"
	"time"

	"github.com/juju/juju/environs"
)

// PreparePrechecker is part of the environs.JujuUpgradePrechecker
// interface. It is called to give an Environ a chance to perform
// interactive operations that are required for prechecking
// an upgrade.
func (e *Environ) PreparePrechecker() error {
	return authenticateClient(e.client())
}

// PrecheckUpgradeOperations is part of the environs.JujuUpgradePrechecker
// interface.  It returns a slice of PrecheckJujuUpgradeOperation to be
// used to determine if a controller can be safely upgraded.
func (env *Environ) PrecheckUpgradeOperations() []environs.PrecheckJujuUpgradeOperation {
	return []environs.PrecheckJujuUpgradeOperation{{
		TargetVersion: version.MustParse("2.8.0"), // should be 2.8
		Steps: []environs.PrecheckJujuUpgradeStep{
			verifyNeutronEnabledStep{env},
		},
	}}
}

type verifyNeutronEnabledStep struct {
	env *Environ
}

func (verifyNeutronEnabledStep) Description() string {
	return "Verify Neutron OpenStack service enabled"
}

// Run is part of the environs.PrecheckJujuUpgradeStep interface.
func (step verifyNeutronEnabledStep) Run() error {
	if step.env.supportsNeutron() {
		return nil
	}
	return errors.NotFoundf("OpenStack Neutron service")
}

func (env *Environ) UpgradeOperations(context.ProviderCallContext, environs.UpgradeOperationsParams) []environs.UpgradeOperation {
	return []environs.UpgradeOperation{
		{
			TargetVersion: providerVersion1,
			Steps: []environs.UpgradeStep{
				tagExistingSecurityGroupsStep{env},
			},
		},
	}
}

// tagExistingSecurityGroupsStep implements environs.UpgradeStep interface.
// It is used to add tags to existing security groups.
type tagExistingSecurityGroupsStep struct {
	env *Environ
}

func (t tagExistingSecurityGroupsStep) replaceTagsWithRetry(ctx context.ProviderCallContext, neutronClient NetworkingNeutron, groupId string, tags []string) retry.CallArgs {
	retryStrategy := shortRetryStrategy
	retryStrategy.IsFatalError = func(err error) bool {
		return !errors.IsNotFound(err)
	}
	retryStrategy.Func = func() error {
		tagsErr := neutronClient.ReplaceAllTags("security-groups", groupId, tags)
		if tagsErr != nil {
			handleCredentialError(tagsErr, ctx)
			return tagsErr
		}
		return nil
	}

	return retryStrategy
}

func (t tagExistingSecurityGroupsStep) extractUuids(name string, re *regexp.Regexp) (string, string) {
	matches := re.FindStringSubmatch(name)

	if len(matches) < 3 {
		return "", ""
	}

	return matches[1], matches[2]
}

// Description is part of the environs.UpgradeStep interface.
func (t tagExistingSecurityGroupsStep) Description() string {
	return "Add tags to existing security groups"
}

// Run is part of the environs.UpgradeStep interface.
func (t tagExistingSecurityGroupsStep) Run(ctx context.ProviderCallContext) error {
	// TODO(@adisazhar123): skip if not a controller
	logger.Infof("starting upgrade step to tag existing security groups")

	// get all security groups
	neutronClient := t.env.neutron()
	query := neutron.ListSecurityGroupsV2Query{}
	securityGroups, err := neutronClient.ListSecurityGroupsV2(query)
	if err != nil {
		handleCredentialError(err, ctx)
		return errors.Trace(err)
	}

	// The regex below covers the following patterns:
	//	(1) juju-<controller-uuid>-<model-uuid>
	//	(2) juju-<controller-uuid>-<model-uuid>-<machine-id>
	//	(3) juju-<controller-uuid>-<model-uuid>-global
	uuidPattern := `[0-9a-fA-F]{8}\b-[0-9a-fA-F]{4}\b-[0-9a-fA-F]{4}\b-[0-9a-fA-F]{4}\b-[0-9a-fA-F]{12}`
	secGroupNamePattern := fmt.Sprintf(`^juju-(%s)-(%s)(?:-(?:\d+|global))?$`, uuidPattern, uuidPattern)
	re, err := regexp.Compile(secGroupNamePattern)

	if err != nil {
		return errors.Trace(err)
	}

	for _, securityGroup := range securityGroups {
		// Extract the UUIDs from the security group so we can tag it below.
		controllerUUID, modelUUID := t.extractUuids(securityGroup.Name, re)
		if controllerUUID == "" && modelUUID == "" {
			continue
		}

		// In addition to the new tags, we still include old tags so that we don't lose them.
		tags := append([]string{}, securityGroup.Tags...)
		tags = append(tags, "juju-controller="+controllerUUID, "juju-model="+modelUUID)
		logger.Infof("adding tags %v for security group: %s", tags, securityGroup.Name)

		// Add the tags.
		err := retry.Call(t.replaceTagsWithRetry(ctx, neutronClient, securityGroup.Id, tags))

		if err != nil {
			if retry.IsAttemptsExceeded(err) || retry.IsDurationExceeded(err) {
				return retry.LastError(err)
			}
			return errors.Trace(err)
		}

		time.Sleep(time.Second)
	}

	return nil
}
