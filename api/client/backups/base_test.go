// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/core/backups"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type baseSuite struct {
	testhelpers.IsolationSuite

	facade    *mocks.MockFacadeCaller
	apiCaller *mocks.MockAPICallCloser
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.facade = mocks.NewMockFacadeCaller(ctrl)
	s.apiCaller = mocks.NewMockAPICallCloser(ctrl)

	return ctrl
}

func (s *baseSuite) newClient() *Client {
	return &Client{
		facade: s.facade,
		st:     s.apiCaller,
	}
}

func (s *baseSuite) checkMetadataResult(c *tc.C, result *params.BackupsMetadataResult, meta *backups.Metadata) {
	var finished, stored time.Time
	if meta.Finished != nil {
		finished = *meta.Finished
	}
	if meta.Stored() != nil {
		stored = *(meta.Stored())
	}

	c.Check(result.ID, tc.Equals, meta.ID())
	c.Check(result.Started, tc.Equals, meta.Started)
	c.Check(result.Finished, tc.Equals, finished)
	c.Check(result.Checksum, tc.Equals, meta.Checksum())
	c.Check(result.ChecksumFormat, tc.Equals, meta.ChecksumFormat())
	c.Check(result.Size, tc.Equals, meta.Size())
	c.Check(result.Stored, tc.Equals, stored)
	c.Check(result.Notes, tc.Equals, meta.Notes)

	c.Check(result.Model, tc.Equals, meta.Origin.Model)
	c.Check(result.Machine, tc.Equals, meta.Origin.Machine)
	c.Check(result.Hostname, tc.Equals, meta.Origin.Hostname)
	c.Check(result.Version, tc.Equals, meta.Origin.Version)
}
