// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/rpc/params"
)

var _ = gc.Suite(&AddPendingResourcesSuite{})

type AddPendingResourcesSuite struct {
	BaseSuite
}

func (s *AddPendingResourcesSuite) TestNoURL(c *gc.C) {
	defer s.setUpTest(c).Finish()
	_, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	id1 := "some-unique-ID"
	aTag := names.NewApplicationTag("a-application")
	s.backend.EXPECT().AddPendingResource(aTag.Id(), "", resourceMatcher{c: c}).Return(id1, nil)
	facade := s.newFacade(c)

	result, err := facade.AddPendingResources(context.Background(), params.AddPendingResourcesArgsV2{
		Entity: params.Entity{
			Tag: aTag.String(),
		},
		Resources: []params.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result, jc.DeepEquals, params.AddPendingResourcesResult{
		PendingIDs: []string{
			id1,
		},
	})
}

func (s *AddPendingResourcesSuite) TestWithURLUpToDate(c *gc.C) {
	defer s.setUpTest(c).Finish()
	res1, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	res1.Origin = charmresource.OriginStore
	res1.Revision = 3
	apiRes1.Origin = charmresource.OriginStore.String()
	apiRes1.Revision = 3
	id1 := "some-unique-ID"
	aTag := names.NewApplicationTag("a-application")
	s.backend.EXPECT().AddPendingResource(aTag.Id(), gomock.Any(), resourceMatcher{c: c}).Return(id1, nil)
	res := []charmresource.Resource{
		res1.Resource,
	}
	s.factory.EXPECT().ResolveResources(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil)
	facade := s.newFacade(c)

	result, err := facade.AddPendingResources(context.Background(), params.AddPendingResourcesArgsV2{
		Entity: params.Entity{
			Tag: aTag.String(),
		},
		URL: "ch:amd64/jammy/spam-5",
		Resources: []params.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	c.Check(result, jc.DeepEquals, params.AddPendingResourcesResult{
		PendingIDs: []string{
			id1,
		},
	})
}

func (s *AddPendingResourcesSuite) TestWithURLMismatchComplete(c *gc.C) {
	defer s.setUpTest(c).Finish()
	res1, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	res1.Origin = charmresource.OriginStore
	res1.Revision = 3
	apiRes1.Origin = charmresource.OriginStore.String()
	apiRes1.Revision = 3
	id1 := "some-unique-ID"
	aTag := names.NewApplicationTag("a-application")
	s.backend.EXPECT().AddPendingResource(aTag.Id(), gomock.Any(), resourceMatcher{c: c}).Return(id1, nil)
	curl := charm.MustParseURL("ch:amd64/jammy/spam-5")
	charmID := corecharm.CharmID{
		URL:    curl,
		Origin: corecharm.Origin{Channel: &charm.Channel{}},
	}
	expected := []charmresource.Resource{res1.Resource}
	s.factory.EXPECT().ResolveResources(gomock.Any(), expected, charmID).Return(expected, nil)

	facade := s.newFacade(c)
	result, err := facade.AddPendingResources(context.Background(), params.AddPendingResourcesArgsV2{
		Entity: params.Entity{
			Tag: aTag.String(),
		},
		URL: curl.String(),
		Resources: []params.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	c.Check(result, jc.DeepEquals, params.AddPendingResourcesResult{
		PendingIDs: []string{
			id1,
		},
	})
}

func (s *AddPendingResourcesSuite) TestWithURLMismatchIncomplete(c *gc.C) {
	defer s.setUpTest(c).Finish()
	res1, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	res1.Origin = charmresource.OriginStore
	res1.Revision = 2
	apiRes1.Origin = charmresource.OriginStore.String()
	apiRes1.Revision = 3
	apiRes1.Fingerprint = nil
	apiRes1.Size = 0
	id1 := "some-unique-ID"
	aTag := names.NewApplicationTag("a-application")
	s.backend.EXPECT().AddPendingResource(aTag.Id(), gomock.Any(), resourceMatcher{c: c}).Return(id1, nil)
	expected := []charmresource.Resource{{
		Meta:        res1.Meta,
		Origin:      charmresource.OriginStore,
		Revision:    3,
		Fingerprint: res1.Fingerprint,
		Size:        res1.Size,
	}}
	res2 := []charmresource.Resource{{
		Meta:     res1.Meta,
		Origin:   charmresource.OriginStore,
		Revision: 3,
	}}
	curl := charm.MustParseURL("ch:amd64/jammy/spam-5")
	charmID := corecharm.CharmID{
		URL:    curl,
		Origin: corecharm.Origin{Channel: &charm.Channel{}},
	}
	s.factory.EXPECT().ResolveResources(gomock.Any(), res2, charmID).Return(expected, nil)

	facade := s.newFacade(c)
	result, err := facade.AddPendingResources(context.Background(), params.AddPendingResourcesArgsV2{
		Entity: params.Entity{
			Tag: aTag.String(),
		},
		URL: curl.String(),
		Resources: []params.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result, jc.DeepEquals, params.AddPendingResourcesResult{
		PendingIDs: []string{
			id1,
		},
	})
}

func (s *AddPendingResourcesSuite) TestWithURLNoRevision(c *gc.C) {
	defer s.setUpTest(c).Finish()
	res1, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	res1.Origin = charmresource.OriginStore
	res1.Revision = 3
	res1.Size = 10
	apiRes1.Origin = charmresource.OriginStore.String()
	apiRes1.Revision = -1
	apiRes1.Size = 0
	apiRes1.Fingerprint = nil
	resNoRev := []charmresource.Resource{{
		Meta:     res1.Meta,
		Origin:   charmresource.OriginStore,
		Revision: -1,
	}}
	id1 := "some-unique-ID"
	aTag := names.NewApplicationTag("a-application")
	s.backend.EXPECT().AddPendingResource(aTag.Id(), gomock.Any(), resourceMatcher{c: c}).Return(id1, nil)
	csRes := res1 // a copy
	csRes.Revision = 3
	csRes.Size = 10
	expected := []charmresource.Resource{csRes.Resource}
	curl := charm.MustParseURL("ch:amd64/jammy/spam-5")
	charmID := corecharm.CharmID{
		URL:    curl,
		Origin: corecharm.Origin{Channel: &charm.Channel{}},
	}
	s.factory.EXPECT().ResolveResources(gomock.Any(), resNoRev, charmID).Return(expected, nil)

	facade := s.newFacade(c)
	result, err := facade.AddPendingResources(context.Background(), params.AddPendingResourcesArgsV2{
		Entity: params.Entity{
			Tag: aTag.String(),
		},
		URL: curl.String(),
		Resources: []params.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	c.Check(result, jc.DeepEquals, params.AddPendingResourcesResult{
		PendingIDs: []string{
			id1,
		},
	})
}

func (s *AddPendingResourcesSuite) TestWithURLUpload(c *gc.C) {
	defer s.setUpTest(c).Finish()
	res1, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	res1.Origin = charmresource.OriginUpload
	res1.Revision = 0
	apiRes1.Origin = charmresource.OriginUpload.String()
	apiRes1.Revision = 0
	id1 := "some-unique-ID"
	aTag := names.NewApplicationTag("a-application")
	s.backend.EXPECT().AddPendingResource(aTag.Id(), gomock.Any(), resourceMatcher{c: c}).Return(id1, nil)
	csRes := res1 // a copy
	csRes.Origin = charmresource.OriginStore
	csRes.Revision = 3
	expected := []charmresource.Resource{csRes.Resource}
	curl := charm.MustParseURL("ch:amd64/jammy/spam-5")
	charmID := corecharm.CharmID{
		URL:    curl,
		Origin: corecharm.Origin{Channel: &charm.Channel{}},
	}
	s.factory.EXPECT().ResolveResources(gomock.Any(), []charmresource.Resource{res1.Resource}, charmID).Return(expected, nil)

	facade := s.newFacade(c)
	result, err := facade.AddPendingResources(context.Background(), params.AddPendingResourcesArgsV2{
		Entity: params.Entity{
			Tag: aTag.String(),
		},
		URL: curl.String(),
		Resources: []params.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	c.Check(result, jc.DeepEquals, params.AddPendingResourcesResult{
		PendingIDs: []string{
			id1,
		},
	})
}

func (s *AddPendingResourcesSuite) TestUnknownResource(c *gc.C) {
	defer s.setUpTest(c).Finish()
	res1, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	res1.Origin = charmresource.OriginStore
	res1.Revision = 3
	apiRes1.Origin = charmresource.OriginStore.String()
	apiRes1.Revision = 3
	id1 := "some-unique-ID"
	aTag := names.NewApplicationTag("a-application")
	s.backend.EXPECT().AddPendingResource(aTag.Id(), gomock.Any(), resourceMatcher{c: c}).Return(id1, nil)
	expected := []charmresource.Resource{res1.Resource}
	curl := charm.MustParseURL("ch:amd64/jammy/spam-5")
	charmID := corecharm.CharmID{
		URL:    curl,
		Origin: corecharm.Origin{Channel: &charm.Channel{}},
	}
	s.factory.EXPECT().ResolveResources(gomock.Any(), expected, charmID).Return(expected, nil)

	facade := s.newFacade(c)
	result, err := facade.AddPendingResources(context.Background(), params.AddPendingResourcesArgsV2{
		Entity: params.Entity{
			Tag: aTag.String(),
		},
		URL: curl.String(),
		Resources: []params.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result, jc.DeepEquals, params.AddPendingResourcesResult{
		PendingIDs: []string{
			id1,
		},
	})
}

func (s *AddPendingResourcesSuite) TestDataStoreError(c *gc.C) {
	defer s.setUpTest(c).Finish()
	_, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	failure := errors.New("<failure>")
	s.backend.EXPECT().AddPendingResource(gomock.Any(), gomock.Any(), gomock.Any()).Return("", failure)

	facade := s.newFacade(c)
	result, err := facade.AddPendingResources(context.Background(), params.AddPendingResourcesArgsV2{
		Entity: params.Entity{
			Tag: "application-a-application",
		},
		Resources: []params.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result, jc.DeepEquals, params.AddPendingResourcesResult{
		ErrorResult: params.ErrorResult{Error: &params.Error{
			Message: `while adding pending resource info for "spam": <failure>`,
		}},
	})
}

type resourceMatcher struct {
	c *gc.C
}

func (m resourceMatcher) Matches(x interface{}) bool {
	in, ok := x.(charmresource.Resource)
	if !ok {
		m.c.Fatal("wrong type")
		return false
	}
	if err := in.Validate(); err != nil {
		m.c.Logf("from Validate: %s", err)
		return false
	}
	return true
}

func (m resourceMatcher) String() string {
	return "match resource"
}
