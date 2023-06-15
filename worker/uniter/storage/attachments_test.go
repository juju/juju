// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v11/hooks"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
	"github.com/juju/juju/worker/uniter/storage"
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

var _ = gc.Suite(&caasAttachmentsSuite{})
var _ = gc.Suite(&iaasAttachmentsSuite{})

func (s *attachmentsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.storSt = storage.NewState()
}

func (s *caasAttachmentsSuite) SetUpTest(c *gc.C) {
	s.modelType = model.CAAS
	s.attachmentsSuite.SetUpTest(c)
}

func (s *iaasAttachmentsSuite) SetUpTest(c *gc.C) {
	s.modelType = model.IAAS
	s.attachmentsSuite.SetUpTest(c)
}

func (s *attachmentsSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctlr := s.mockStateOpsSuite.setupMocks(c)
	s.expectState(c)
	return ctlr
}

func (s *attachmentsSuite) TestNewAttachments(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})
	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachmentId, error) {
			c.Assert(u, gc.Equals, unitTag)
			return nil, nil
		},
	}

	_, err := storage.NewAttachments(st, unitTag, s.mockStateOps, abort)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *attachmentsSuite) assertNewAttachments(c *gc.C, storageTag names.StorageTag) *storage.Attachments {
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
			c.Assert(u, gc.Equals, unitTag)
			return attachmentIds, nil
		},
		storageAttachment: func(s names.StorageTag, u names.UnitTag) (params.StorageAttachment, error) {
			c.Assert(s, gc.Equals, storageTag)
			return attachment, nil
		},
	}

	att, err := storage.NewAttachments(storSt, unitTag, s.mockStateOps, abort)
	c.Assert(err, jc.ErrorIsNil)
	return att
}

func (s *attachmentsSuite) TestNewAttachmentsInitHavePending(c *gc.C) {
	defer s.setupMocks(c).Finish()

	storageTag := names.NewStorageTag("data/0")

	// No initial storage State, so no storagers will be started.
	att := s.assertNewAttachments(c, storageTag)
	c.Assert(att.Pending(), gc.Equals, 1)
	err := att.ValidateHook(hook.Info{
		Kind:      hooks.StorageAttached,
		StorageId: storageTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *attachmentsSuite) TestNewAttachmentsInit(c *gc.C) {
	defer s.mockStateOpsSuite.setupMocks(c).Finish()
	storageTag := names.NewStorageTag("data/0")
	s.storSt.Attach(storageTag.Id())
	s.expectSetState(c, "")
	// Setup a storage tag which should be ignored by init.
	s.storSt.Attach("data/3")
	err := s.storSt.Detach("data/3")
	c.Assert(err, jc.ErrorIsNil)
	s.expectState(c)

	att := s.assertNewAttachments(c, storageTag)
	c.Assert(att.Pending(), gc.Equals, 0)
}

func (s *attachmentsSuite) TestAttachmentsUpdateShortCircuitDeath(c *gc.C) {
	defer s.setupMocks(c).Finish()

	abort := make(chan struct{})

	unitTag := names.NewUnitTag("mysql/0")
	removed := names.NewSet()
	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachmentId, error) {
			return nil, nil
		},
		remove: func(s names.StorageTag, u names.UnitTag) error {
			c.Assert(u, gc.Equals, unitTag)
			removed.Add(s)
			return nil
		},
	}

	att, err := storage.NewAttachments(st, unitTag, s.mockStateOps, abort)
	c.Assert(err, jc.ErrorIsNil)
	r := storage.NewResolver(loggo.GetLogger("test"), att, s.modelType)

	// First make sure we create a storage-attached hook operation for
	// data/0. We do this to show that until the hook is *committed*,
	// we will still short-circuit removal.
	localState := resolver.LocalState{State: operation.State{
		Kind: operation.Continue,
	}}
	storageTag0 := names.NewStorageTag("data/0")
	storageTag1 := names.NewStorageTag("data/1")
	_, err = r.NextOp(localState, remotestate.Snapshot{
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
	c.Assert(err, jc.ErrorIsNil)

	for _, storageTag := range []names.StorageTag{storageTag0, storageTag1} {
		_, err = r.NextOp(localState, remotestate.Snapshot{
			Life: life.Alive,
			Storage: map[names.StorageTag]remotestate.StorageSnapshot{
				storageTag: {Life: life.Dying},
			},
		}, nil)
		c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	}
	c.Assert(removed.SortedValues(), jc.DeepEquals, []names.Tag{
		storageTag0, storageTag1,
	})
}

func (s *attachmentsSuite) TestAttachmentsStorage(c *gc.C) {
	s.testAttachmentsStorage(c, operation.State{Kind: operation.Continue})
}

func (s *caasAttachmentsSuite) TestAttachmentsStorageStarted(c *gc.C) {
	opState := operation.State{
		Kind:      operation.RunHook,
		Step:      operation.Queued,
		Installed: true,
		Started:   true,
	}
	s.testAttachmentsStorage(c, opState)
}

func (s *attachmentsSuite) testAttachmentsStorage(c *gc.C, opState operation.State) {
	defer s.setupMocks(c).Finish()

	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachmentId, error) {
			return nil, nil
		},
	}

	att, err := storage.NewAttachments(st, unitTag, s.mockStateOps, abort)
	c.Assert(err, jc.ErrorIsNil)
	r := storage.NewResolver(loggo.GetLogger("test"), att, s.modelType)

	storageTag := names.NewStorageTag("data/0")

	// Inform the resolver of an attachment.
	localState := resolver.LocalState{State: opState}
	op, err := r.NextOp(localState, remotestate.Snapshot{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run hook storage-attached")
}

func (s *caasAttachmentsSuite) TestAttachmentsStorageNotStarted(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachmentId, error) {
			return nil, nil
		},
	}

	att, err := storage.NewAttachments(st, unitTag, s.mockStateOps, abort)
	c.Assert(err, jc.ErrorIsNil)
	r := storage.NewResolver(loggo.GetLogger("test"), att, s.modelType)

	storageTag := names.NewStorageTag("data/0")

	// Inform the resolver of an attachment.
	localState := resolver.LocalState{State: operation.State{
		Kind:      operation.RunHook,
		Step:      operation.Queued,
		Installed: true,
		Started:   false,
	}}
	_, err = r.NextOp(localState, remotestate.Snapshot{
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
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

func (s *attachmentsSuite) TestAttachmentsCommitHook(c *gc.C) {
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
			c.Assert(s, gc.Equals, storageTag)
			return nil
		},
	}

	att, err := storage.NewAttachments(st, unitTag, s.mockStateOps, abort)
	c.Assert(err, jc.ErrorIsNil)
	r := storage.NewResolver(loggo.GetLogger("test"), att, s.modelType)

	// Inform the resolver of an attachment.
	localState := resolver.LocalState{State: operation.State{
		Kind: operation.Continue,
	}}
	_, err = r.NextOp(localState, remotestate.Snapshot{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(att.Pending(), gc.Equals, 1)

	s.storSt.Attach(storageTag.Id())
	s.expectSetState(c, "")
	err = att.CommitHook(hook.Info{
		Kind:      hooks.StorageAttached,
		StorageId: storageTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.storSt.Detach(storageTag.Id())
	c.Assert(err, jc.ErrorIsNil)
	s.expectSetState(c, "")
	c.Assert(removed, jc.IsFalse)
	err = att.CommitHook(hook.Info{
		Kind:      hooks.StorageDetaching,
		StorageId: storageTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(removed, jc.IsTrue)
}

func (s *attachmentsSuite) TestAttachmentsSetDying(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	storageTag := names.NewStorageTag("data/0")
	var destroyed, removed bool
	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachmentId, error) {
			c.Assert(u, gc.Equals, unitTag)
			return []params.StorageAttachmentId{{
				StorageTag: storageTag.String(),
				UnitTag:    unitTag.String(),
			}}, nil
		},
		storageAttachment: func(s names.StorageTag, u names.UnitTag) (params.StorageAttachment, error) {
			c.Assert(u, gc.Equals, unitTag)
			c.Assert(s, gc.Equals, storageTag)
			return params.StorageAttachment{}, &params.Error{
				Message: "not provisioned",
				Code:    params.CodeNotProvisioned,
			}
		},
		destroyUnitStorageAttachments: func(u names.UnitTag) error {
			c.Assert(u, gc.Equals, unitTag)
			destroyed = true
			return nil
		},
		remove: func(s names.StorageTag, u names.UnitTag) error {
			c.Assert(removed, jc.IsFalse)
			c.Assert(s, gc.Equals, storageTag)
			c.Assert(u, gc.Equals, unitTag)
			removed = true
			return nil
		},
	}

	att, err := storage.NewAttachments(st, unitTag, s.mockStateOps, abort)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(att.Pending(), gc.Equals, 1)
	r := storage.NewResolver(loggo.GetLogger("test"), att, s.modelType)

	// Inform the resolver that the unit is Dying. The storage is still
	// Alive, and is now provisioned, but will be destroyed and removed
	// by the resolver.
	localState := resolver.LocalState{State: operation.State{
		Kind: operation.Continue,
	}}
	_, err = r.NextOp(localState, remotestate.Snapshot{
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
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	c.Assert(destroyed, jc.IsTrue)
	c.Assert(att.Pending(), gc.Equals, 0)
	c.Assert(removed, jc.IsTrue)
}

func (s *attachmentsSuite) TestAttachmentsWaitPending(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	storageTag := names.NewStorageTag("data/0")
	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachmentId, error) {
			return nil, nil
		},
	}

	att, err := storage.NewAttachments(st, unitTag, s.mockStateOps, abort)
	c.Assert(err, jc.ErrorIsNil)
	r := storage.NewResolver(loggo.GetLogger("test"), att, s.modelType)

	nextOp := func(installed bool) error {
		localState := resolver.LocalState{State: operation.State{
			Installed: installed,
			Kind:      operation.Continue,
		}}
		_, err := r.NextOp(localState, remotestate.Snapshot{
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
	c.Assert(att.Pending(), gc.Equals, 1)

	if s.modelType == model.IAAS {
		c.Assert(err, gc.Equals, resolver.ErrWaiting)
	} else {
		c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	}

	err = nextOp(true /* workload installed */)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}
