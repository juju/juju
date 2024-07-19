// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/charm"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type manifestSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&manifestSuite{})

var manifestTestCases = [...]struct {
	name   string
	input  []charmManifest
	output charm.Manifest
}{
	{
		name:   "empty",
		input:  []charmManifest{},
		output: charm.Manifest{},
	},
	{
		name: "decode base",
		input: []charmManifest{
			{
				Index:        0,
				OS:           "ubuntu",
				Track:        "latest",
				Risk:         "edge",
				Architecture: "amd64",
			},
		},
		output: charm.Manifest{
			Bases: []charm.Base{
				{
					Name: "ubuntu",
					Channel: charm.Channel{
						Track: "latest",
						Risk:  charm.RiskEdge,
					},
					Architectures: []string{"amd64"},
				},
			},
		},
	},
	{
		name: "decode bases",
		input: []charmManifest{
			{
				Index:        0,
				OS:           "ubuntu",
				Track:        "latest",
				Risk:         "edge",
				Architecture: "amd64",
			},
			{
				Index:        0,
				OS:           "ubuntu",
				Track:        "latest",
				Risk:         "edge",
				Architecture: "arm64",
			},
		},
		output: charm.Manifest{
			Bases: []charm.Base{
				{
					Name: "ubuntu",
					Channel: charm.Channel{
						Track: "latest",
						Risk:  charm.RiskEdge,
					},
					Architectures: []string{"amd64", "arm64"},
				},
			},
		},
	},
}

func (s *manifestSuite) TestDecodeManifest(c *gc.C) {
	for _, tc := range manifestTestCases {
		c.Logf("Running test case %q", tc.name)

		decoded, err := decodeManifest(tc.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(decoded, gc.DeepEquals, tc.output)
	}
}

type manifestStateSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&manifestStateSuite{})

func (s *manifestStateSuite) TestManifestOS(c *gc.C) {
	type osType struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}

	stmt := sqlair.MustPrepare(`
SELECT os.* AS &osType.* FROM os ORDER BY id;
`, osType{})

	var results []osType
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt).GetAll(&results)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)

	m := []string{
		"ubuntu",
	}

	for i, value := range m {
		c.Logf("result %d: %#v", i, value)
		result, err := encodeManifestOS(value)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(result, gc.DeepEquals, results[i].ID)
	}
}

func (s *manifestStateSuite) TestManifestArchitecture(c *gc.C) {
	type archType struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}

	stmt := sqlair.MustPrepare(`
SELECT architecture.* AS &archType.* FROM architecture ORDER BY id;
`, archType{})

	var results []archType
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt).GetAll(&results)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 5)

	m := []string{
		"amd64",
		"arm64",
		"ppc64el",
		"s390x",
		"riscv64",
	}

	for i, value := range m {
		c.Logf("result %d: %#v", i, value)
		result, err := encodeManifestArchitecture(value)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(result, gc.DeepEquals, results[i].ID)
	}
}
