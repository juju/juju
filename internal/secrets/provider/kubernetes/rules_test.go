// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	stdtesting "testing"

	"github.com/juju/tc"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/juju/juju/internal/testhelpers"
)

type rulesSuite struct {
	testhelpers.IsolationSuite
}

func TestRulesSuite(t *stdtesting.T) { tc.Run(t, &rulesSuite{}) }
func (s *rulesSuite) TestRulesForSecretAccessNew(c *tc.C) {
	owned := []string{"owned-secret-1"}
	read := []string{"read-secret-1"}
	newPolicies := rulesForSecretAccess("test", false, nil, owned, read, nil)
	c.Assert(newPolicies, tc.DeepEquals, []rbacv1.PolicyRule{
		{
			Verbs:     []string{"create", "patch"},
			APIGroups: []string{"*"},
			Resources: []string{"secrets"},
		},
		{
			Verbs:         []string{"get", "list"},
			APIGroups:     []string{"*"},
			Resources:     []string{"namespaces"},
			ResourceNames: []string{"test"},
		},
		{
			Verbs:         []string{"*"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"owned-secret-1"},
		},
		{
			Verbs:         []string{"get"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"read-secret-1"},
		},
	})
}

func (s *rulesSuite) TestRulesForSecretAccessControllerModelNew(c *tc.C) {
	owned := []string{"owned-secret-1"}
	read := []string{"read-secret-1"}
	newPolicies := rulesForSecretAccess("test", true, nil, owned, read, nil)
	c.Assert(newPolicies, tc.DeepEquals, []rbacv1.PolicyRule{
		{
			Verbs:     []string{"create", "patch"},
			APIGroups: []string{"*"},
			Resources: []string{"secrets"},
		},
		{
			Verbs:     []string{"get", "list"},
			APIGroups: []string{"*"},
			Resources: []string{"namespaces"},
		},
		{
			Verbs:         []string{"*"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"owned-secret-1"},
		},
		{
			Verbs:         []string{"get"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"read-secret-1"},
		},
	})
}

func (s *rulesSuite) TestRulesForSecretAccessUpdate(c *tc.C) {
	existing := []rbacv1.PolicyRule{
		{
			Verbs:     []string{"create", "patch"},
			APIGroups: []string{"*"},
			Resources: []string{"secrets"},
		},
		{
			Verbs:         []string{"get", "list"},
			APIGroups:     []string{"*"},
			Resources:     []string{"namespaces"},
			ResourceNames: []string{"test"},
		},
		{
			Verbs:         []string{"*"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"owned-secret-1"},
		},
		{
			Verbs:         []string{"*"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"removed-owned-secret"},
		},
		{
			Verbs:         []string{"get"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"read-secret-1"},
		},
		{
			Verbs:         []string{"get"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"removed-read-secret"},
		},
	}

	owned := []string{"owned-secret-1", "owned-secret-2"}
	read := []string{"read-secret-1", "read-secret-2"}
	removed := []string{"removed-owned-secret", "removed-read-secret"}

	newPolicies := rulesForSecretAccess("test", false, existing, owned, read, removed)
	c.Assert(newPolicies, tc.DeepEquals, []rbacv1.PolicyRule{
		{
			Verbs:     []string{"create", "patch"},
			APIGroups: []string{"*"},
			Resources: []string{"secrets"},
		},
		{
			Verbs:         []string{"get", "list"},
			APIGroups:     []string{"*"},
			Resources:     []string{"namespaces"},
			ResourceNames: []string{"test"},
		},
		{
			Verbs:         []string{"*"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"owned-secret-1"},
		},
		{
			Verbs:         []string{"*"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"owned-secret-2"},
		},
		{
			Verbs:         []string{"get"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"read-secret-1"},
		},
		{
			Verbs:         []string{"get"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"read-secret-2"},
		},
	})
}

func (s *rulesSuite) TestRulesForSecretAccessControllerModelUpdate(c *tc.C) {
	existing := []rbacv1.PolicyRule{
		{
			Verbs:     []string{"create", "patch"},
			APIGroups: []string{"*"},
			Resources: []string{"secrets"},
		},
		{
			Verbs:     []string{"get", "list"},
			APIGroups: []string{"*"},
			Resources: []string{"namespaces"},
		},
		{
			Verbs:         []string{"*"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"owned-secret-1"},
		},
		{
			Verbs:         []string{"*"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"removed-owned-secret"},
		},
		{
			Verbs:         []string{"get"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"read-secret-1"},
		},
		{
			Verbs:         []string{"get"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"removed-read-secret"},
		},
	}

	owned := []string{"owned-secret-1", "owned-secret-2"}
	read := []string{"read-secret-1", "read-secret-2"}
	removed := []string{"removed-owned-secret", "removed-read-secret"}

	newPolicies := rulesForSecretAccess("test", true, existing, owned, read, removed)
	c.Assert(newPolicies, tc.DeepEquals, []rbacv1.PolicyRule{
		{
			Verbs:     []string{"create", "patch"},
			APIGroups: []string{"*"},
			Resources: []string{"secrets"},
		},
		{
			Verbs:     []string{"get", "list"},
			APIGroups: []string{"*"},
			Resources: []string{"namespaces"},
		},
		{
			Verbs:         []string{"*"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"owned-secret-1"},
		},
		{
			Verbs:         []string{"*"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"owned-secret-2"},
		},
		{
			Verbs:         []string{"get"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"read-secret-1"},
		},
		{
			Verbs:         []string{"get"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"read-secret-2"},
		},
	})
}
