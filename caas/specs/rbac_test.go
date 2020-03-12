// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	// jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas/specs"
	// "github.com/juju/juju/testing"
)

func (s *typesSuite) TestServiceAccountSpecV2Validate(c *gc.C) {
	spec := specs.ServiceAccountSpecV2{}
	c.Assert(spec.Validate(), gc.ErrorMatches, `rules is required`)

	spec = specs.ServiceAccountSpecV2{
		Global: true,
		Rules: []specs.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}
	c.Assert(spec.ToLatest(), gc.DeepEquals, &specs.PrimeServiceAccountSpecV3{
		ServiceAccountSpecV3: specs.ServiceAccountSpecV3{
			Roles: []specs.Role{
				{
					Global: true,
					Rules: []specs.PolicyRule{
						{
							APIGroups: []string{""},
							Resources: []string{"pods"},
							Verbs:     []string{"get", "watch", "list"},
						},
					},
				},
			},
		},
	})
}

func (s *typesSuite) TestServiceAccountSpecV3Validate(c *gc.C) {
	spec := specs.ServiceAccountSpecV3{}
	c.Assert(spec.Validate(), gc.ErrorMatches, `roles is required`)

	spec = specs.ServiceAccountSpecV3{
		Roles: []specs.Role{
			{
				Name: "role1",
			},
		},
	}
	c.Assert(spec.Validate(), gc.ErrorMatches, `rules is required`)
}

func (s *typesSuite) TestPrimeServiceAccountSpecV3Validate(c *gc.C) {
	spec := specs.PrimeServiceAccountSpecV3{}
	c.Assert(spec.Validate(), gc.ErrorMatches, `roles is required`)

	spec = specs.PrimeServiceAccountSpecV3{
		ServiceAccountSpecV3: specs.ServiceAccountSpecV3{
			Roles: []specs.Role{
				{
					Global: true,
					Rules: []specs.PolicyRule{
						{
							APIGroups: []string{""},
							Resources: []string{"pods"},
							Verbs:     []string{"get", "watch", "list"},
						},
					},
				},
				{
					Global: true,
					Rules: []specs.PolicyRule{
						{
							APIGroups: []string{""},
							Resources: []string{"pods"},
							Verbs:     []string{"get", "watch", "list"},
						},
					},
				},
			},
		},
	}
	c.Assert(spec.Validate(), gc.ErrorMatches, `the prime service can only have one role or cluster role`)
}

func (s *typesSuite) TestPrimeServiceAccountSpecV3SetName(c *gc.C) {
	spec := specs.PrimeServiceAccountSpecV3{
		ServiceAccountSpecV3: specs.ServiceAccountSpecV3{
			Roles: []specs.Role{
				{
					Global: true,
					Rules: []specs.PolicyRule{
						{
							APIGroups: []string{""},
							Resources: []string{"pods"},
							Verbs:     []string{"get", "watch", "list"},
						},
					},
				},
			},
		},
	}
	spec.SetName("mariadb-k8s")
	c.Assert(spec.GetName(), gc.DeepEquals, "mariadb-k8s")
	c.Assert(spec.Roles[0].Name, gc.DeepEquals, "mariadb-k8s")
}
