// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitstate

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	corestorage "github.com/juju/juju/core/storage"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/secret"
	domainstorage "github.com/juju/juju/domain/storage"
)

type commitHookChangesArgSuite struct{}

func TestCommitHookChangesArgSuite(t *testing.T) {
	tc.Run(t, &commitHookChangesArgSuite{})
}

func (s *commitHookChangesArgSuite) TestValidateAndHasChangesNoChanges(c *tc.C) {
	hasChanges, err := CommitHookChangesArg{
		UnitName: unittesting.GenNewName(c, "testing/0"),
	}.ValidateAndHasChanges()

	c.Assert(err, tc.ErrorIsNil)
	c.Check(hasChanges, tc.Equals, false)
}

func (s *commitHookChangesArgSuite) TestValidateAndHasChangesCreateSecret(c *tc.C) {
	hasChanges, err := CommitHookChangesArg{
		UnitName:      unittesting.GenNewName(c, "testing/0"),
		SecretCreates: []CreateSecretArg{{CreateCharmSecretParams: secret.CreateCharmSecretParams{}}},
	}.ValidateAndHasChanges()

	c.Assert(err, tc.ErrorIsNil)
	c.Check(hasChanges, tc.Equals, true)
}

func (s *commitHookChangesArgSuite) TestValidateAndHasChangesAddStorage(c *tc.C) {
	hasChanges, err := CommitHookChangesArg{
		UnitName: unittesting.GenNewName(c, "testing/0"),
		AddStorage: []PreparedStorageAdd{{
			StorageName: corestorage.Name("data"),
			Storage: domainstorage.IAASUnitAddStorageArg{
				UnitAddStorageArg: domainstorage.UnitAddStorageArg{
					CountLessThanEqual: 1,
				},
			},
		}},
	}.ValidateAndHasChanges()

	c.Assert(err, tc.ErrorIsNil)
	c.Check(hasChanges, tc.Equals, true)
}

func (s *commitHookChangesArgSuite) TestValidateAndHasChangesErrorNoUnitName(c *tc.C) {
	hasChanges, err := CommitHookChangesArg{}.ValidateAndHasChanges()

	c.Check(err, tc.ErrorMatches, "invalid unit name: \"\"")
	c.Check(hasChanges, tc.Equals, false)
}

func (s *commitHookChangesArgSuite) TestValidateAndHasChangesErrorInvalidOpenPort(c *tc.C) {
	hasChanges, err := CommitHookChangesArg{
		UnitName: unittesting.GenNewName(c, "testing/0"),
		OpenPorts: map[string][]network.PortRange{
			"endpoint": {{Protocol: "failme"}},
		},
	}.ValidateAndHasChanges()

	c.Check(err, tc.ErrorMatches, ".*open port is invalid.*")
	c.Check(hasChanges, tc.Equals, true)
}

func (s *commitHookChangesArgSuite) TestValidateAndHasChangesErrorInvalidClosePort(c *tc.C) {
	hasChanges, err := CommitHookChangesArg{
		UnitName: unittesting.GenNewName(c, "testing/0"),
		ClosePorts: map[string][]network.PortRange{
			"endpoint": {{Protocol: "failme"}},
		},
	}.ValidateAndHasChanges()

	c.Check(err, tc.ErrorMatches, ".*close port is invalid.*")
	c.Check(hasChanges, tc.Equals, true)
}

func (s *commitHookChangesArgSuite) TestRequiresLeadershipTrueCreateAppSecret(c *tc.C) {
	requiresLeadership := CommitHookChangesArg{
		UnitName: unittesting.GenNewName(c, "testing/0"),
		SecretCreates: []CreateSecretArg{{
			CreateCharmSecretParams: secret.CreateCharmSecretParams{
				CharmOwner: secret.CharmSecretOwner{Kind: secret.ApplicationCharmSecretOwner},
			},
		}},
	}.RequiresLeadership()

	c.Check(requiresLeadership, tc.IsTrue)
}

func (s *commitHookChangesArgSuite) TestRequiresLeadershipFalseCreateUnitSecret(c *tc.C) {
	requiresLeadership := CommitHookChangesArg{
		UnitName: unittesting.GenNewName(c, "testing/0"),
		SecretCreates: []CreateSecretArg{{
			CreateCharmSecretParams: secret.CreateCharmSecretParams{
				CharmOwner: secret.CharmSecretOwner{Kind: secret.UnitCharmSecretOwner},
			},
		}},
	}.RequiresLeadership()

	c.Check(requiresLeadership, tc.IsFalse)
}

func (s *commitHookChangesArgSuite) TestRequiresLeadershipTrueDeleteAppSecret(c *tc.C) {
	requiresLeadership := CommitHookChangesArg{
		UnitName: unittesting.GenNewName(c, "testing/0"),
		SecretDeletes: []DeleteSecretArg{{
			OwnerKind: secret.ApplicationCharmSecretOwner,
		}},
	}.RequiresLeadership()

	c.Check(requiresLeadership, tc.IsTrue)
}

func (s *commitHookChangesArgSuite) TestRequiresLeadershipFalseDeleteUnitSecret(c *tc.C) {
	requiresLeadership := CommitHookChangesArg{
		UnitName: unittesting.GenNewName(c, "testing/0"),
		SecretDeletes: []DeleteSecretArg{{
			OwnerKind: secret.UnitCharmSecretOwner,
		}},
	}.RequiresLeadership()

	c.Check(requiresLeadership, tc.IsFalse)
}

func (s *commitHookChangesArgSuite) TestRequiresLeadershipTrueApplicationSettings(c *tc.C) {
	requiresLeadership := CommitHookChangesArg{
		UnitName: unittesting.GenNewName(c, "testing/0"),
		RelationSettings: []RelationSettings{{
			ApplicationSettings: map[string]string{"key": "value"},
		}},
	}.RequiresLeadership()

	c.Check(requiresLeadership, tc.IsTrue)
}

func (s *commitHookChangesArgSuite) TestRequiresLeadershipFalseUnitSettings(c *tc.C) {
	requiresLeadership := CommitHookChangesArg{
		UnitName: unittesting.GenNewName(c, "testing/0"),
		RelationSettings: []RelationSettings{{
			Settings: map[string]string{"key": "value"},
		}},
	}.RequiresLeadership()

	c.Check(requiresLeadership, tc.IsFalse)
}

func (s *commitHookChangesArgSuite) TestRequiresLeadershipFalseOpenPorts(c *tc.C) {
	requiresLeadership := CommitHookChangesArg{
		UnitName: unittesting.GenNewName(c, "testing/0"),
		OpenPorts: map[string][]network.PortRange{
			"endpoint": {{Protocol: "failme"}},
		},
	}.RequiresLeadership()

	c.Check(requiresLeadership, tc.IsFalse)
}

func (s *commitHookChangesArgSuite) TestRequiresLeadershipTrueRevokeAppSecret(c *tc.C) {
	requiresLeadership := CommitHookChangesArg{
		UnitName: unittesting.GenNewName(c, "testing/0"),
		SecretRevokes: []RevokeSecretArg{{
			OwnerKind: secret.ApplicationCharmSecretOwner,
		}},
	}.RequiresLeadership()

	c.Check(requiresLeadership, tc.IsTrue)
}

func (s *commitHookChangesArgSuite) TestRequiresLeadershipFalseRevokeUnitSecret(c *tc.C) {
	requiresLeadership := CommitHookChangesArg{
		UnitName: unittesting.GenNewName(c, "testing/0"),
		SecretRevokes: []RevokeSecretArg{{
			OwnerKind: secret.UnitCharmSecretOwner,
		}},
	}.RequiresLeadership()

	c.Check(requiresLeadership, tc.IsFalse)
}
