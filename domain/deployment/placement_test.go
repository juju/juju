// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployment

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/internal/testhelpers"
)

type PlacementSuite struct {
	testhelpers.IsolationSuite
}

func TestPlacementSuite(t *testing.T) {
	tc.Run(t, &PlacementSuite{})
}

func (s *PlacementSuite) TestPlacement(c *tc.C) {
	const modelUUID = "32c5aaae-6713-4cd7-83a4-d1256e9c97d0"

	tests := []struct {
		input     *instance.Placement
		modelUUID string
		output    Placement
		err       *string
	}{
		{
			input:     nil,
			modelUUID: modelUUID,
			output: Placement{
				Type: PlacementTypeUnset,
			},
		},
		{
			input: &instance.Placement{
				Scope:     instance.MachineScope,
				Directive: "0",
			},
			modelUUID: modelUUID,
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
			modelUUID: modelUUID,
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
			modelUUID: modelUUID,
			err:       new(`container type "kvm" not supported`),
		},
		{
			input: &instance.Placement{
				Scope:     instance.MachineScope,
				Directive: "0/lxd",
			},
			modelUUID: modelUUID,
			err:       new(`placement directive "0/lxd" is not in the form of <parent>/<scope>/<child>`),
		},
		{
			input: &instance.Placement{
				Scope:     instance.MachineScope,
				Directive: "0/lxd/0/0",
			},
			modelUUID: modelUUID,
			err:       new(`placement directive "0/lxd/0/0" is not in the form of <parent>/<scope>/<child>`),
		},
		{
			input: &instance.Placement{
				Scope: string(instance.LXD),
			},
			modelUUID: modelUUID,
			output: Placement{
				Type:      PlacementTypeContainer,
				Container: ContainerTypeLXD,
			},
		},
		{
			input: &instance.Placement{
				Scope: string(instance.NONE),
			},
			modelUUID: modelUUID,
			output:    Placement{},
			err:       new(`invalid container type "none"`),
		},
		{
			input: &instance.Placement{
				Scope:     "lxd",
				Directive: "0",
			},
			modelUUID: modelUUID,
			output: Placement{
				Type:      PlacementTypeContainer,
				Container: ContainerTypeLXD,
				Directive: "0",
			},
		},
		{
			// A container type scope must still fall through to
			// container parsing even when modelUUID is non-empty.
			input: &instance.Placement{
				Scope:     "lxd",
				Directive: "0",
			},
			modelUUID: modelUUID,
			output: Placement{
				Type:      PlacementTypeContainer,
				Container: ContainerTypeLXD,
				Directive: "0",
			},
		},
		{
			// A non-container, non-model scope with a set
			// modelUUID must fall through to container parsing and
			// error, not be misclassified as a provider placement.
			input: &instance.Placement{
				Scope: "not-a-real-scope",
			},
			modelUUID: modelUUID,
			err:       new(`invalid container type "not-a-real-scope"`),
		},
		{
			input: &instance.Placement{
				Scope:     instance.ModelScope,
				Directive: "zone=us-east-1a",
			},
			modelUUID: modelUUID,
			output: Placement{
				Type:      PlacementTypeProvider,
				Directive: "zone=us-east-1a",
			},
		},
		{
			input: &instance.Placement{
				Scope:     modelUUID,
				Directive: "zone=us-east-1a",
			},
			modelUUID: modelUUID,
			output: Placement{
				Type:      PlacementTypeProvider,
				Directive: "zone=us-east-1a",
			},
		},
		{
			// An empty scope with a non-empty modelUUID must
			// fall through to the container type path and error.
			input: &instance.Placement{
				Scope: "",
			},
			modelUUID: modelUUID,
			err:       new(`invalid container type ""`),
		},
		{
			// A real model UUID scope with a different modelUUID
			// must not be misclassified as a provider placement.
			// It should fall through to container parsing and error.
			input: &instance.Placement{
				Scope:     modelUUID,
				Directive: "zone=us-east-1a",
			},
			modelUUID: "00000000-0000-0000-0000-000000000000",
			err:       new(`invalid container type "32c5aaae-6713-4cd7-83a4-d1256e9c97d0"`),
		},
		{
			// The literal "model-uuid" placeholder scope works
			// as provider placement.
			input: &instance.Placement{
				Scope:     instance.ModelScope,
				Directive: "subnet=subnet-123",
			},
			modelUUID: modelUUID,
			output: Placement{
				Type:      PlacementTypeProvider,
				Directive: "subnet=subnet-123",
			},
		},
		{
			// A real model UUID scope with an empty directive is
			// still a valid provider placement.
			input: &instance.Placement{
				Scope:     modelUUID,
				Directive: "",
			},
			modelUUID: modelUUID,
			output: Placement{
				Type:      PlacementTypeProvider,
				Directive: "",
			},
		},
	}
	for _, test := range tests {
		c.Logf("input: %v", test.input)

		result, err := ParsePlacement(test.input, test.modelUUID)
		if test.err != nil {
			c.Assert(err, tc.ErrorMatches, *test.err)
		} else {
			c.Assert(err, tc.ErrorIsNil)
		}
		c.Check(result, tc.Equals, test.output)
	}
}
