// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/charm/hooks"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
	"github.com/juju/juju/internal/worker/uniter/storage"
	"github.com/juju/juju/rpc/params"
)

type attachmentsSuite struct {
	testing.BaseSuite
	mockStateOpsSuite

	modelType model.ModelType
}

type caasAttachmentsSuite struct {
	attachmentsSuite
}

type iaasAttachmentsSuite struct {
	attachmentsSuite
}

var _ = tc.Suite(&caasAttachmentsSuite{})
var _ = tc.Suite(&iaasAttachmentsSuite{})

func (s *attachmentsSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.storSt = storage.NewState()
}

func (s *caasAttachmentsSuite) SetUpTest(c *tc.C) {
	s.modelType = model.CAAS
	s.attachmentsSuite.SetUpTest(c)
}

func (s *iaasAttachmentsSuite) SetUpTest(c *tc.C) {
	s.modelType = model.IAAS
	s.attachmentsSuite.SetUpTest(c)
}

func (s *attachmentsSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctlr := s.mockStateOpsSuite.setupMocks(c)
	s.expectState(c)
	return ctlr
}

func (s *attachmentsSuite) TestNewAttachments(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})
	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachmentId, error) {
			c.Assert(u, tc.Equals, unitTag)
			return nil, nil
		},
	}

	_, err := storage.NewAttachments(context.Background(), st, unitTag, s.mockStateOps, abort)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *attachmentsSuite) assertNewAttachments(c *tc.C, storageTag names.StorageTag) *storage.Attachments {
	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	// Simulate remote State returning a single Alive storage attachment.

	attachmentIds := []params.StorageAttachmentId{{
		StorageTag: storageTag.String(),
		UnitTag:    unitTag.String(),
	}}
	attachment := params.StorageAttachment{
		StorageTag: storageTag.String(),
		UnitTag:    unitTag.String(),
		Life:       life.Alive,
		Kind:       params.StorageKindBlock,
		Location:   "/dev/sdb",
	}

	storSt := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachmentId, error) {
			c.Assert(u, tc.Equals, unitTag)
			return attachmentIds, nil
		},
		storageAttachment: func(s names.StorageTag, u names.UnitTag) (params.StorageAttachment, error) {
			c.Assert(s, tc.Equals, storageTag)
			return attachment, nil
		},
	}

	att, err := storage.NewAttachments(context.Background(), storSt, unitTag, s.mockStateOps, abort)
	c.Assert(err, tc.ErrorIsNil)
	return att
}

func (s *attachmentsSuite) TestNewAttachmentsInitHavePending(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageTag := names.NewStorageTag("data/0")

	// No initial storage State, so no storagers will be started.
	att := s.assertNewAttachments(c, storageTag)
	c.Assert(att.Pending(), tc.Equals, 1)
	err := att.ValidateHook(hook.Info{
		Kind:      hooks.StorageAttached,
		StorageId: storageTag.Id(),
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *attachmentsSuite) TestNewAttachmentsInit(c *tc.C) {
	defer s.mockStateOpsSuite.setupMocks(c).Finish()
	storageTag := names.NewStorageTag("data/0")
	s.storSt.Attach(storageTag.Id())
	s.expectSetState(c, "")
	// Setup a storage tag which should be ignored by init.
	s.storSt.Attach("data/3")
	err := s.storSt.Detach("data/3")
	c.Assert(err, tc.ErrorIsNil)
	s.expectState(c)

	att := s.assertNewAttachments(c, storageTag)
	c.Assert(att.Pending(), tc.Equals, 0)
}

func (s *attachmentsSuite) TestAttachmentsUpdateShortCircuitDeath(c *tc.C) {
	defer s.setupMocks(c).Finish()

	abort := make(chan struct{})

	unitTag := names.NewUnitTag("mysql/0")
	removed := names.NewSet()
	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachmentId, error) {
			return nil, nil
		},
		remove: func(s names.StorageTag, u names.UnitTag) error {
			c.Assert(u, tc.Equals, unitTag)
			removed.Add(s)
			return nil
		},
	}

	att, err := storage.NewAttachments(context.Background(), st, unitTag, s.mockStateOps, abort)
	c.Assert(err, tc.ErrorIsNil)
	r := storage.NewResolver(loggertesting.WrapCheckLog(c), att, s.modelType)

	// First make sure we create a storage-attached hook operation for
	// data/0. We do this to show that until the hook is *committed*,
	// we will still short-circuit removal.
	localState := resolver.LocalState{State: operation.State{
		Kind: operation.Continue,
	}}
	storageTag0 := names.NewStorageTag("data/0")
	storageTag1 := names.NewStorageTag("data/1")
	_, err = r.NextOp(context.Background(), localState, remotestate.Snapshot{
		Life: life.Alive,
		Storage: map[names.StorageTag]remotestate.StorageSnapshot{
			storageTag0: {
				Life:     life.Alive,
				Kind:     params.StorageKindBlock,
				Location: "/dev/sdb",
				Attached: true,
			},
		},
	}, &mockOperations{})
	c.Assert(err, tc.ErrorIsNil)

	for _, storageTag := range []names.StorageTag{storageTag0, storageTag1} {
		_, err = r.NextOp(context.Background(), localState, remotestate.Snapshot{
			Life: life.Alive,
			Storage: map[names.StorageTag]remotestate.StorageSnapshot{
				storageTag: {Life: life.Dying},
			},
		}, nil)
		c.Assert(err, tc.Equals, resolver.ErrNoOperation)
	}
	c.Assert(removed.SortedValues(), tc.DeepEquals, []names.Tag{
		storageTag0, storageTag1,
	})
}

func (s *attachmentsSuite) TestAttachmentsStorage(c *tc.C) {
	s.testAttachmentsStorage(c, operation.State{Kind: operation.Continue})
}

func (s *caasAttachmentsSuite) TestAttachmentsStorageStarted(c *tc.C) {
	opState := operation.State{
		Kind:      operation.RunHook,
		Step:      operation.Queued,
		Installed: true,
		Started:   true,
	}
	s.testAttachmentsStorage(c, opState)
}

func (s *attachmentsSuite) testAttachmentsStorage(c *tc.C, opState operation.State) {
	defer s.setupMocks(c).Finish()

	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachmentId, error) {
			return nil, nil
		},
	}

	att, err := storage.NewAttachments(context.Background(), st, unitTag, s.mockStateOps, abort)
	c.Assert(err, tc.ErrorIsNil)
	r := storage.NewResolver(loggertesting.WrapCheckLog(c), att, s.modelType)

	storageTag := names.NewStorageTag("data/0")

	// Inform the resolver of an attachment.
	localState := resolver.LocalState{State: opState}
	op, err := r.NextOp(context.Background(), localState, remotestate.Snapshot{
		Life: life.Alive,
		Storage: map[names.StorageTag]remotestate.StorageSnapshot{
			storageTag: {
				Kind:     params.StorageKindBlock,
				Life:     life.Alive,
				Location: "/dev/sdb",
				Attached: true,
			},
		},
	}, &mockOperations{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run hook storage-attached")
}

func (s *caasAttachmentsSuite) TestAttachmentsStorageNotStarted(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachmentId, error) {
			return nil, nil
		},
	}

	att, err := storage.NewAttachments(context.Background(), st, unitTag, s.mockStateOps, abort)
	c.Assert(err, tc.ErrorIsNil)
	r := storage.NewResolver(loggertesting.WrapCheckLog(c), att, s.modelType)

	storageTag := names.NewStorageTag("data/0")

	// Inform the resolver of an attachment.
	localState := resolver.LocalState{State: operation.State{
		Kind:      operation.RunHook,
		Step:      operation.Queued,
		Installed: true,
		Started:   false,
	}}
	_, err = r.NextOp(context.Background(), localState, remotestate.Snapshot{
		Life: life.Alive,
		Storage: map[names.StorageTag]remotestate.StorageSnapshot{
			storageTag: {
				Kind:     params.StorageKindBlock,
				Life:     life.Alive,
				Location: "/dev/sdb",
				Attached: true,
			},
		},
	}, &mockOperations{})
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
}

func (s *attachmentsSuite) TestAttachmentsCommitHook(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	var removed bool
	storageTag := names.NewStorageTag("data/0")
	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachmentId, error) {
			return nil, nil
		},
		remove: func(s names.StorageTag, u names.UnitTag) error {
			removed = true
			c.Assert(s, tc.Equals, storageTag)
			return nil
		},
	}

	att, err := storage.NewAttachments(context.Background(), st, unitTag, s.mockStateOps, abort)
	c.Assert(err, tc.ErrorIsNil)
	r := storage.NewResolver(loggertesting.WrapCheckLog(c), att, s.modelType)

	// Inform the resolver of an attachment.
	localState := resolver.LocalState{State: operation.State{
		Kind: operation.Continue,
	}}
	_, err = r.NextOp(context.Background(), localState, remotestate.Snapshot{
		Life: life.Alive,
		Storage: map[names.StorageTag]remotestate.StorageSnapshot{
			storageTag: {
				Kind:     params.StorageKindBlock,
				Life:     life.Alive,
				Location: "/dev/sdb",
				Attached: true,
			},
		},
	}, &mockOperations{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(att.Pending(), tc.Equals, 1)

	s.storSt.Attach(storageTag.Id())
	s.expectSetState(c, "")
	err = att.CommitHook(context.Background(), hook.Info{
		Kind:      hooks.StorageAttached,
		StorageId: storageTag.Id(),
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.storSt.Detach(storageTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	s.expectSetState(c, "")
	c.Assert(removed, tc.IsFalse)
	err = att.CommitHook(context.Background(), hook.Info{
		Kind:      hooks.StorageDetaching,
		StorageId: storageTag.Id(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(removed, tc.IsTrue)
}

func (s *attachmentsSuite) TestAttachmentsSetDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	storageTag := names.NewStorageTag("data/0")
	var destroyed, removed bool
	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachmentId, error) {
			c.Assert(u, tc.Equals, unitTag)
			return []params.StorageAttachmentId{{
				StorageTag: storageTag.String(),
				UnitTag:    unitTag.String(),
			}}, nil
		},
		storageAttachment: func(s names.StorageTag, u names.UnitTag) (params.StorageAttachment, error) {
			c.Assert(u, tc.Equals, unitTag)
			c.Assert(s, tc.Equals, storageTag)
			return params.StorageAttachment{}, &params.Error{
				Message: "not provisioned",
				Code:    params.CodeNotProvisioned,
			}
		},
		destroyUnitStorageAttachments: func(u names.UnitTag) error {
			c.Assert(u, tc.Equals, unitTag)
			destroyed = true
			return nil
		},
		remove: func(s names.StorageTag, u names.UnitTag) error {
			c.Assert(removed, tc.IsFalse)
			c.Assert(s, tc.Equals, storageTag)
			c.Assert(u, tc.Equals, unitTag)
			removed = true
			return nil
		},
	}

	att, err := storage.NewAttachments(context.Background(), st, unitTag, s.mockStateOps, abort)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(att.Pending(), tc.Equals, 1)
	r := storage.NewResolver(loggertesting.WrapCheckLog(c), att, s.modelType)

	// Inform the resolver that the unit is Dying. The storage is still
	// Alive, and is now provisioned, but will be destroyed and removed
	// by the resolver.
	localState := resolver.LocalState{State: operation.State{
		Kind: operation.Continue,
	}}
	_, err = r.NextOp(context.Background(), localState, remotestate.Snapshot{
		Life: life.Dying,
		Storage: map[names.StorageTag]remotestate.StorageSnapshot{
			storageTag: {
				Kind:     params.StorageKindBlock,
				Life:     life.Alive,
				Location: "/dev/sdb",
				Attached: true,
			},
		},
	}, &mockOperations{})
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
	c.Assert(destroyed, tc.IsTrue)
	c.Assert(att.Pending(), tc.Equals, 0)
	c.Assert(removed, tc.IsTrue)
}

func (s *attachmentsSuite) TestAttachmentsWaitPending(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	storageTag := names.NewStorageTag("data/0")
	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachmentId, error) {
			return nil, nil
		},
	}

	att, err := storage.NewAttachments(context.Background(), st, unitTag, s.mockStateOps, abort)
	c.Assert(err, tc.ErrorIsNil)
	r := storage.NewResolver(loggertesting.WrapCheckLog(c), att, s.modelType)

	nextOp := func(installed bool) error {
		localState := resolver.LocalState{State: operation.State{
			Installed: installed,
			Kind:      operation.Continue,
		}}
		_, err := r.NextOp(context.Background(), localState, remotestate.Snapshot{
			Life: life.Alive,
			Storage: map[names.StorageTag]remotestate.StorageSnapshot{
				storageTag: {
					Life:     life.Alive,
					Attached: false,
				},
			},
		}, &mockOperations{})
		return err
	}

	// Inform the resolver of a new, unprovisioned storage attachment.
	// For IAAS models, before install, we should wait for its completion;
	// after install, we should not.
	err = nextOp(false /* workload not installed */)
	c.Assert(att.Pending(), tc.Equals, 1)

	if s.modelType == model.IAAS {
		c.Assert(err, tc.Equals, resolver.ErrWaiting)
	} else {
		c.Assert(err, tc.Equals, resolver.ErrNoOperation)
	}

	err = nextOp(true /* workload installed */)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
}
