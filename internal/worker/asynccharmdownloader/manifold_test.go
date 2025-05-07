// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package asynccharmdownloader

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type ManifoldConfigSuite struct {
	testhelpers.IsolationSuite
	config ManifoldConfig
}

var _ = tc.Suite(&ManifoldConfigSuite{})

func (s *ManifoldConfigSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = validConfig(c)
}

func validConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		DomainServicesName:     "domain-services",
		HTTPClientName:         "http-client",
		NewDownloader:          NewDownloader,
		NewHTTPClient:          NewHTTPClient,
		NewAsyncDownloadWorker: NewAsyncDownloadWorker,
		Logger:                 loggertesting.WrapCheckLog(c),
		Clock:                  clock.WallClock,
	}
}

func (s *ManifoldConfigSuite) TestValid(c *tc.C) {
	c.Check(s.config.Validate(), tc.ErrorIsNil)
}

func (s *ManifoldConfigSuite) TestMissingDomainServicesName(c *tc.C) {
	s.config.DomainServicesName = ""
	s.checkNotValid(c, "empty DomainServicesName not valid")
}

func (s *ManifoldConfigSuite) TestMissingHTTPClientName(c *tc.C) {
	s.config.HTTPClientName = ""
	s.checkNotValid(c, "empty HTTPClientName not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewDownloader(c *tc.C) {
	s.config.NewDownloader = nil
	s.checkNotValid(c, "nil NewDownloader not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewHTTPClient(c *tc.C) {
	s.config.NewHTTPClient = nil
	s.checkNotValid(c, "nil NewHTTPClient not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewAsyncDownloadWorker(c *tc.C) {
	s.config.NewAsyncDownloadWorker = nil
	s.checkNotValid(c, "nil NewAsyncDownloadWorker not valid")
}

func (s *ManifoldConfigSuite) TestMissingLogger(c *tc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *ManifoldConfigSuite) TestMissingClock(c *tc.C) {
	s.config.Clock = nil
	s.checkNotValid(c, "nil Clock not valid")
}

func (s *ManifoldConfigSuite) checkNotValid(c *tc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}
