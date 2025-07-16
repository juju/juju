// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"fmt"
	"strings"

	"github.com/go-goose/goose/v5/neutron"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/version/v2"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
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

// UpgradeOperations is part of the environs.Upgrader interface.
// It returns a list of upgrade operations to execute.
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

func (t tagExistingSecurityGroupsStep) buildReplaceTagsWithRetry(ctx context.ProviderCallContext, neutronClient NetworkingNeutron, groupId string, tags []string) retry.CallArgs {
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

// Description is part of the environs.UpgradeStep interface.
func (t tagExistingSecurityGroupsStep) Description() string {
	return "Add tags to existing security groups"
}

// Run is part of the environs.UpgradeStep interface.
func (t tagExistingSecurityGroupsStep) Run(ctx context.ProviderCallContext) error {
	logger.Infof("starting upgrade step to tag existing security groups for controller: %s and model: %s", t.env.controllerUUID, t.env.modelUUID)

	// Get all security groups.
	neutronClient := t.env.neutron()
	query := neutron.ListSecurityGroupsV2Query{
		Tags: nil,
	}
	securityGroups, err := neutronClient.ListSecurityGroupsV2(query)
	if err != nil {
		handleCredentialError(err, ctx)
		return errors.Trace(err)
	}

	jujuGroupNamePrefix := fmt.Sprintf("juju-%s-%s", t.env.controllerUUID, t.env.modelUUID)
	for _, securityGroup := range securityGroups {
		if !strings.HasPrefix(securityGroup.Name, jujuGroupNamePrefix) {
			continue
		}

		// In addition to the new tags, we still include old tags so that we don't lose them.
		groupTags := append([]string{}, securityGroup.Tags...)
		groupTags = append(groupTags,
			fmt.Sprintf("%s=%s", tags.JujuController, t.env.controllerUUID),
			fmt.Sprintf("%s=%s", tags.JujuModel, t.env.modelUUID),
		)
		logger.Infof("adding tags %v for security group: %s", groupTags, securityGroup.Name)

		// Add the tags.
		err := retry.Call(t.buildReplaceTagsWithRetry(ctx, neutronClient, securityGroup.Id, groupTags))

		if err != nil {
			if retry.IsAttemptsExceeded(err) || retry.IsDurationExceeded(err) {
				return retry.LastError(err)
			}
			return errors.Trace(err)
		}
	}

	return nil
}
