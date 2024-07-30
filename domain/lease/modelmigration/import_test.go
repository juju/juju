// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"errors"
	"time"

	"github.com/juju/description/v8"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/modelmigration"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type importSuite struct {
	testing.IsolationSuite

	coordinator *MockCoordinator
	service     *MockImportService
	txnRunner   *MockTxnRunner
}

var _ = gc.Suite(&importSuite{})

func (s *importSuite) TestRegisterImport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator, loggertesting.WrapCheckLog(c))
}

func (s *importSuite) TestSetup(c *gc.C) {
	defer s.setupMocks(c).Finish()

	op := &importOperation{
		logger: loggertesting.WrapCheckLog(c),
	}

	// We don't currently need the model DB, so for this instance we can just
	// pass nil.
	err := op.Setup(modelmigration.NewScope(nil, nil, nil))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestExecuteWithNoApplications(c *gc.C) {
	defer s.setupMocks(c).Finish()

	op := s.newImportOperation(c)

	s.expectNoLeases(c)

	err := op.Execute(context.Background(), description.NewModel(description.ModelArgs{}))
	c.Assert(err, jc.ErrorIsNil)

}

func (s *importSuite) TestExecuteWithApplications(c *gc.C) {
	defer s.setupMocks(c).Finish()

	op := s.newImportOperation(c)

	uuid := uuid.MustNewUUID().String()
	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			"uuid": uuid,
		},
	})
	model.AddApplication(description.ApplicationArgs{
		Tag:    names.NewApplicationTag("postgresql"),
		Leader: "postgresql/0",
	})

	// Expected lease.
	key := lease.Key{
		ModelUUID: uuid,
		Namespace: "postgresql",
		Lease:     lease.ApplicationLeadershipNamespace,
	}
	req := lease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	s.expectLease(c, key, req)

	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestExecuteWithMultipleApplications(c *gc.C) {
	defer s.setupMocks(c).Finish()

	op := s.newImportOperation(c)

	uuid := uuid.MustNewUUID().String()
	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			"uuid": uuid,
		},
	})
	model.AddApplication(description.ApplicationArgs{
		Tag:    names.NewApplicationTag("postgresql"),
		Leader: "postgresql/0",
	})
	model.AddApplication(description.ApplicationArgs{
		Tag:    names.NewApplicationTag("wordpress"),
		Leader: "wordpress/1",
	})

	// Expected lease.
	key := lease.Key{
		ModelUUID: uuid,
		Namespace: "postgresql",
		Lease:     lease.ApplicationLeadershipNamespace,
	}
	req := lease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	s.expectLease(c, key, req)

	key = lease.Key{
		ModelUUID: uuid,
		Namespace: "wordpress",
		Lease:     lease.ApplicationLeadershipNamespace,
	}
	req = lease.Request{
		Holder:   "wordpress/1",
		Duration: time.Minute,
	}

	s.expectLease(c, key, req)

	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestExecuteWithError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	op := s.newImportOperation(c)

	uuid := uuid.MustNewUUID().String()
	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			"uuid": uuid,
		},
	})
	model.AddApplication(description.ApplicationArgs{
		Tag:    names.NewApplicationTag("postgresql"),
		Leader: "postgresql/0",
	})

	s.service.EXPECT().ClaimLease(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("boom"))

	err := op.Execute(context.Background(), model)
	c.Assert(err, gc.ErrorMatches, `claiming lease for {"postgresql" "`+uuid+`" "application-leadership"}: boom`)
}

func (s *importSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockImportService(ctrl)
	s.txnRunner = NewMockTxnRunner(ctrl)

	return ctrl
}

func (s *importSuite) newImportOperation(c *gc.C) *importOperation {
	return &importOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}
}

func (s *importSuite) expectNoLeases(c *gc.C) {
	s.service.EXPECT().ClaimLease(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
}

func (s *importSuite) expectLease(c *gc.C, key lease.Key, req lease.Request) {
	s.service.EXPECT().ClaimLease(gomock.Any(), key, req).Return(nil)
}
