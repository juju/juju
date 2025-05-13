// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreresource "github.com/juju/juju/core/resource"
	resourcetesting "github.com/juju/juju/core/resource/testing"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/rpc/params"
)

type BaseSuite struct {
	applicationService *MockApplicationService
	resourceService    *MockResourceService
	repository         *MockNewCharmRepository
	factory            func(context.Context, *charm.URL) (NewCharmRepository, error)
}

func (s *BaseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)
	s.resourceService = NewMockResourceService(ctrl)
	s.repository = NewMockNewCharmRepository(ctrl)
	s.factory = func(context.Context, *charm.URL) (NewCharmRepository, error) { return s.repository, nil }

	return ctrl
}

func (s *BaseSuite) newFacade(c *tc.C) *API {
	facade, err := NewResourcesAPI(s.applicationService, s.resourceService, s.factory,
		loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	return facade
}

func newResource(c *tc.C, name, username, data string) (coreresource.Resource, params.Resource) {
	opened := resourcetesting.NewResource(c, nil, name, "a-application", data)
	res := opened.Resource
	res.RetrievedBy = username
	if username == "" {
		res.Timestamp = time.Time{}
	}

	apiRes := params.Resource{
		CharmResource: params.CharmResource{
			Name:        name,
			Description: name + " description",
			Type:        "file",
			Path:        res.Path,
			Origin:      "upload",
			Revision:    0,
			Fingerprint: res.Fingerprint.Bytes(),
			Size:        res.Size,
		},
		UUID:            res.UUID.String(),
		ApplicationName: res.ApplicationName,
		Username:        username,
		Timestamp:       res.Timestamp,
	}

	return res, apiRes
}
