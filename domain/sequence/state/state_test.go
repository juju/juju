// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
	domainsequence "github.com/juju/juju/domain/sequence"
	sequenceerrors "github.com/juju/juju/domain/sequence/errors"
)

type stateSuite struct {
	schematesting.ModelSuite
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestGetSequencesForExportNoRows(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())

	seq, err := state.GetSequencesForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(seq, tc.HasLen, 0)
}

func (s *stateSuite) TestGetSequencesForExport(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())

	var seqValue uint64
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		seqValue, err = NextValue(ctx, state, tx, domainsequence.StaticNamespace("foo"))
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	seq, err := state.GetSequencesForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(seq, tc.DeepEquals, map[string]uint64{
		"foo": seqValue,
	})
}

func (s *stateSuite) TestGetSequencesForExportMultiple(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())

	var seqValue uint64
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		for range 10 {
			var err error
			if seqValue, err = NextValue(ctx, state, tx, domainsequence.StaticNamespace("foo")); err != nil {
				return err
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(seqValue, tc.Equals, uint64(9))

	seq, err := state.GetSequencesForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(seq, tc.DeepEquals, map[string]uint64{
		"foo": seqValue,
	})
}

func (s *stateSuite) TestGetSequencesForExportPrefixNamespace(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())

	var seqValue uint64
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		seqValue, err = NextValue(ctx, state, tx, domainsequence.MakePrefixNamespace(domainsequence.StaticNamespace("foo"), "bar"))
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	seq, err := state.GetSequencesForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(seq, tc.DeepEquals, map[string]uint64{
		"foo_bar": seqValue,
	})
}

func (s *stateSuite) TestImportSequences(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())

	seq, err := state.GetSequencesForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(seq, tc.HasLen, 0)

	err = state.ImportSequences(c.Context(), map[string]uint64{
		"foo":     1,
		"foo_bar": 2,
	})
	c.Assert(err, tc.ErrorIsNil)

	seq, err = state.GetSequencesForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(seq, tc.DeepEquals, map[string]uint64{
		"foo":     1,
		"foo_bar": 2,
	})
}

// TestImportedSequencesContinue checks that allocation resumes after persisted
// imports, including zero and independently named prefix namespaces.
func (s *stateSuite) TestImportedSequencesContinue(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	err := state.ImportSequences(c.Context(), map[string]uint64{"foo": 0, "foo_bar": 7})
	c.Assert(err, tc.ErrorIsNil)

	var single uint64
	var batch []uint64
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		single, err = NextValue(ctx, state, tx, domainsequence.StaticNamespace("foo"))
		if err != nil {
			return err
		}
		batch, err = NextNValues(ctx, state, tx, 3,
			domainsequence.MakePrefixNamespace(domainsequence.StaticNamespace("foo"), "bar"))
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(single, tc.Equals, uint64(1))
	c.Check(batch, tc.DeepEquals, []uint64{8, 9, 10})

	stored, err := state.GetSequencesForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(stored, tc.DeepEquals, map[string]uint64{"foo": 1, "foo_bar": 10})
}

func (s *stateSuite) TestImportSequencesTwice(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())

	seq, err := state.GetSequencesForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(seq, tc.HasLen, 0)

	err = state.ImportSequences(c.Context(), map[string]uint64{
		"foo":     1,
		"foo_bar": 2,
	})
	c.Assert(err, tc.ErrorIsNil)

	err = state.ImportSequences(c.Context(), map[string]uint64{
		"foo":     1,
		"foo_bar": 2,
	})
	c.Assert(err, tc.ErrorIs, sequenceerrors.DuplicateNamespaceSequence)
}
