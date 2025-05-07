// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployment

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/internal/testhelpers"
)

type PlacementSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&PlacementSuite{})

func (s *PlacementSuite) TestPlacement(c *tc.C) {
	tests := []struct {
		input  *instance.Placement
		output Placement
		err    *string
	}{
		{
			input: nil,
			output: Placement{
				Type: PlacementTypeUnset,
			},
		},
		{
			input: &instance.Placement{
				Scope:     instance.MachineScope,
				Directive: "0",
			},
			output: Placement{
				Type:      PlacementTypeMachine,
				Directive: "0",
			},
		},
		{
			input: &instance.Placement{
				Scope:     instance.MachineScope,
				Directive: "0/lxd/0",
			},
			output: Placement{
				Type:      PlacementTypeMachine,
				Directive: "0/lxd/0",
			},
		},
		{
			input: &instance.Placement{
				Scope:     instance.MachineScope,
				Directive: "0/kvm/0",
			},
			err: ptr(`container type "kvm" not supported`),
		},
		{
			input: &instance.Placement{
				Scope:     instance.MachineScope,
				Directive: "0/lxd",
			},
			err: ptr(`placement directive "0/lxd" is not in the form of <parent>/<scope>/<child>`),
		},
		{
			input: &instance.Placement{
				Scope:     instance.MachineScope,
				Directive: "0/lxd/0/0",
			},
			err: ptr(`placement directive "0/lxd/0/0" is not in the form of <parent>/<scope>/<child>`),
		},
		{
			input: &instance.Placement{
				Scope: string(instance.LXD),
			},
			output: Placement{
				Type:      PlacementTypeContainer,
				Container: ContainerTypeLXD,
			},
		},
		{
			input: &instance.Placement{
				Scope: string(instance.NONE),
			},
			output: Placement{},
			err:    ptr(`invalid container type "none"`),
		},
		{
			input: &instance.Placement{
				Scope:     "lxd",
				Directive: "0",
			},
			output: Placement{
				Type:      PlacementTypeContainer,
				Container: ContainerTypeLXD,
				Directive: "0",
			},
		},
		{
			input: &instance.Placement{
				Scope:     instance.ModelScope,
				Directive: "zone=us-east-1a",
			},
			output: Placement{
				Type:      PlacementTypeProvider,
				Directive: "zone=us-east-1a",
			},
		},
	}
	for _, test := range tests {
		c.Logf("input: %v", test.input)

		result, err := ParsePlacement(test.input)
		if test.err != nil {
			c.Assert(err, tc.ErrorMatches, *test.err)
		} else {
			c.Assert(err, tc.ErrorIsNil)
		}
		c.Check(result, tc.Equals, test.output)
	}
}

func ptr[T any](v T) *T {
	return &v
}
