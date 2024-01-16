// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package units3caller

import (
	context "context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/dependency"
	dependencytesting "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"
	httprequest "gopkg.in/httprequest.v1"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/s3client"
)

type manifoldSuite struct {
	baseSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg = s.getConfig()
	cfg.APICallerName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewClient = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"api-caller": s.apiConn,
	}
	return dependencytesting.StubGetter(resources)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		APICallerName: "api-caller",
		NewClient: func(string, s3client.HTTPClient, s3client.Logger) (objectstore.ReadSession, error) {
			return s.session, nil
		},
		Logger: s.logger,
	}
}

var expectedInputs = []string{"api-caller"}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.apiConn.EXPECT().RootHTTPClient().Return(&httprequest.Client{}, nil)

	w, err := Manifold(s.getConfig()).Start(context.Background(), s.newGetter())
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *manifoldSuite) TestOutput(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.apiConn.EXPECT().RootHTTPClient().Return(&httprequest.Client{}, nil)

	manifold := Manifold(s.getConfig())
	w, err := manifold.Start(context.Background(), s.newGetter())
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	var session objectstore.ReadSession
	err = manifold.Output(w, &session)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(session, gc.Equals, s.session)

	workertest.CleanKill(c, w)
}
