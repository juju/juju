// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package units3caller

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/dependency"
	dependencytesting "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	httprequest "gopkg.in/httprequest.v1"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/s3client"
)

type manifoldSuite struct {
	baseSuite
}

var _ = tc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
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
		NewClient: func(string, s3client.HTTPClient, logger.Logger) (objectstore.ReadSession, error) {
			return s.session, nil
		},
		Logger: s.logger,
	}
}

var expectedInputs = []string{"api-caller"}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.apiConn.EXPECT().RootHTTPClient().Return(&httprequest.Client{}, nil)

	w, err := Manifold(s.getConfig()).Start(context.Background(), s.newGetter())
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *manifoldSuite) TestOutput(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.apiConn.EXPECT().RootHTTPClient().Return(&httprequest.Client{}, nil)

	manifold := Manifold(s.getConfig())
	w, err := manifold.Start(context.Background(), s.newGetter())
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	var session objectstore.ReadSession
	err = manifold.Output(w, &session)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(session, tc.Equals, s.session)

	workertest.CleanKill(c, w)
}
