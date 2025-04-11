// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	schematesting "github.com/juju/juju/domain/schema/testing"
	domainsequence "github.com/juju/juju/domain/sequence"
	sequenceerrors "github.com/juju/juju/domain/sequence/errors"
)

type stateSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestGetSequencesForExportNoRows(c *gc.C) {
	state := NewState(s.TxnRunnerFactory())

	seq, err := state.GetSequencesForExport(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(seq, gc.HasLen, 0)
}

func (s *stateSuite) TestGetSequencesForExport(c *gc.C) {
	state := NewState(s.TxnRunnerFactory())

	var seqValue uint64
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		seqValue, err = NextValue(ctx, state, tx, domainsequence.StaticNamespace("foo"))
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	seq, err := state.GetSequencesForExport(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(seq, gc.DeepEquals, map[string]uint64{
		"foo": seqValue,
	})
}

func (s *stateSuite) TestGetSequencesForExportMultiple(c *gc.C) {
	state := NewState(s.TxnRunnerFactory())

	var seqValue uint64
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		for i := 0; i < 10; i++ {
			var err error
			if seqValue, err = NextValue(ctx, state, tx, domainsequence.StaticNamespace("foo")); err != nil {
				return err
			}
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(seqValue, gc.Equals, uint64(9))

	seq, err := state.GetSequencesForExport(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(seq, gc.DeepEquals, map[string]uint64{
		"foo": seqValue,
	})
}

func (s *stateSuite) TestGetSequencesForExportPrefixNamespace(c *gc.C) {
	state := NewState(s.TxnRunnerFactory())

	var seqValue uint64
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		seqValue, err = NextValue(ctx, state, tx, domainsequence.MakePrefixNamespace(domainsequence.StaticNamespace("foo"), "bar"))
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	seq, err := state.GetSequencesForExport(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(seq, gc.DeepEquals, map[string]uint64{
		"foo_bar": seqValue,
	})
}

func (s *stateSuite) TestImportSequences(c *gc.C) {
	state := NewState(s.TxnRunnerFactory())

	seq, err := state.GetSequencesForExport(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(seq, gc.HasLen, 0)

	err = state.ImportSequences(context.Background(), map[string]uint64{
		"foo":     1,
		"foo_bar": 2,
	})
	c.Assert(err, jc.ErrorIsNil)

	seq, err = state.GetSequencesForExport(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(seq, gc.DeepEquals, map[string]uint64{
		"foo":     1,
		"foo_bar": 2,
	})
}

func (s *stateSuite) TestImportSequencesTwice(c *gc.C) {
	state := NewState(s.TxnRunnerFactory())

	seq, err := state.GetSequencesForExport(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(seq, gc.HasLen, 0)

	err = state.ImportSequences(context.Background(), map[string]uint64{
		"foo":     1,
		"foo_bar": 2,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = state.ImportSequences(context.Background(), map[string]uint64{
		"foo":     1,
		"foo_bar": 2,
	})
	c.Assert(err, jc.ErrorIs, sequenceerrors.DuplicateNamespaceSequence)
}

func (s *stateSuite) TestRemoveAllSequences(c *gc.C) {
	state := NewState(s.TxnRunnerFactory())

	seq, err := state.GetSequencesForExport(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(seq, gc.HasLen, 0)

	err = state.ImportSequences(context.Background(), map[string]uint64{
		"foo":     1,
		"foo_bar": 2,
	})
	c.Assert(err, jc.ErrorIsNil)

	seq, err = state.GetSequencesForExport(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(seq, gc.DeepEquals, map[string]uint64{
		"foo":     1,
		"foo_bar": 2,
	})

	err = state.RemoveAllSequences(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	seq, err = state.GetSequencesForExport(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(seq, gc.HasLen, 0)
}
