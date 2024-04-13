// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/provider/caas/specs"
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

	spec.Roles[0].Rules = []specs.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"get", "watch", "list"},
		},
	}
	c.Assert(spec.Validate(), jc.ErrorIsNil)
}

func (s *typesSuite) TestPrimeServiceAccountSpecV3Validate(c *gc.C) {
	spec := specs.PrimeServiceAccountSpecV3{}
	c.Assert(spec.Validate(), gc.ErrorMatches, `invalid primary service account: roles is required`)

	spec = specs.PrimeServiceAccountSpecV3{
		ServiceAccountSpecV3: specs.ServiceAccountSpecV3{
			Roles: []specs.Role{
				{
					Global: true,
					Name:   "foo",
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
	c.Assert(spec.Validate(), gc.ErrorMatches, `invalid primary service account: either all or none of the roles should have a name set`)
}
