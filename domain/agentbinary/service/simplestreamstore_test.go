// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

//const gzipContentType = "application/gzip"

type simplestreamStoreSuite struct {
	testhelpers.IsolationSuite
}

func TestSimplestreamStoreSuite(t *testing.T) {
	tc.Run(t, &simplestreamStoreSuite{})
}

func (*simplestreamStoreSuite) TestStub(c *tc.C) {
	c.Skip("re implement after 4.0 release")
}

//func (s *simplestreamStoreSuite) setupMocks(c *tc.C) *gomock.Controller {
//	ctrl := gomock.NewController(c)
//	s.mockProviderForAgentBinaryFinder = NewMockProviderForAgentBinaryFinder(ctrl)
//	s.mockHTTPClient = NewMockHTTPClient(ctrl)
//	return ctrl
//}
//
//func (s *simplestreamStoreSuite) TestGetAgentBinaryWithSHA256(c *tc.C) {
//	ctrl := s.setupMocks(c)
//	defer ctrl.Finish()
//
//	semVersionBinary := semversion.MustParseBinary("4.0.1-ubuntu-amd64")
//
//	ver := agentbinary.Version{
//		Number: semVersionBinary.Number,
//		Arch:   semVersionBinary.Arch,
//	}
//
//	toolURL := "testURL"
//	var buf bytes.Buffer
//	gz := gzip.NewWriter(&buf)
//	payload := []byte("hello-agent-binary")
//	_, err := gz.Write(payload)
//	c.Assert(err, tc.IsNil)
//	c.Assert(gz.Close(), tc.IsNil) // flush gzip footer
//
//	agentBinaryFilter := func(
//		_ context.Context,
//		_ envtools.SimplestreamsFetcher,
//		_ environs.BootstrapEnviron,
//		majorVersion,
//		minorVersion int,
//		streams []string,
//		filter coretools.Filter,
//	) (coretools.List, error) {
//		c.Assert(majorVersion, tc.Equals, 4)
//		c.Assert(minorVersion, tc.Equals, 0)
//		c.Assert(streams, tc.DeepEquals, []string{"testing", "devel", "proposed", "released"})
//		c.Assert(filter, tc.DeepEquals, coretools.Filter{
//			Number: semversion.Number{Major: majorVersion, Minor: minorVersion, Patch: 1},
//			Arch:   "amd64",
//		},
//		)
//		return coretools.List{
//			{
//				Version: semVersionBinary,
//				URL:     toolURL,
//				Size:    int64(len(payload)),
//			},
//		}, nil
//	}
//
//	req, err := http.NewRequestWithContext(c.Context(), http.MethodGet, toolURL, nil)
//	c.Assert(err, tc.IsNil)
//	req.Header.Set(headerAccept, gzipContentType)
//	gzipReader := io.NopCloser(bytes.NewReader(buf.Bytes()))
//
//	s.mockHTTPClient.EXPECT().Do(req).Return(&http.Response{
//		StatusCode: http.StatusOK,
//		Header: http.Header{
//			headerContentType: []string{gzipContentType},
//		},
//		Body: gzipReader,
//	}, nil)
//
//	simpleStreamStore := NewSimpleStreamAgentBinaryStore(func(context.Context) (ProviderForAgentBinaryFinder, error) {
//		return s.mockProviderForAgentBinaryFinder, nil
//	}, agentBinaryFilter, s.mockHTTPClient)
//
//	data, size, _, err := simpleStreamStore.GetAgentBinaryWithSHA256(c.Context(), ver, domainagentbinary.AgentStreamTesting)
//	c.Assert(err, tc.ErrorIsNil)
//	c.Assert(data, tc.DeepEquals, gzipReader)
//	c.Assert(size, tc.Equals, int64(len(payload)))
//	c.Assert(data.Close(), tc.ErrorIsNil)
//}
//
//func (s *simplestreamStoreSuite) TestGetAgentBinaryWithSHA256NotFound(c *tc.C) {
//	ctrl := s.setupMocks(c)
//	defer ctrl.Finish()
//
//	semVersionBinary := semversion.MustParseBinary("4.0.1-ubuntu-amd64")
//
//	ver := agentbinary.Version{
//		Number: semVersionBinary.Number,
//		Arch:   semVersionBinary.Arch,
//	}
//
//	toolURL := "testURL"
//	var buf bytes.Buffer
//	gz := gzip.NewWriter(&buf)
//	payload := []byte("hello-agent-binary")
//	_, err := gz.Write(payload)
//	c.Assert(err, tc.IsNil)
//	c.Assert(gz.Close(), tc.IsNil) // flush gzip footer
//	agentBinaryFilter := func(
//		_ context.Context,
//		_ envtools.SimplestreamsFetcher,
//		_ environs.BootstrapEnviron,
//		majorVersion,
//		minorVersion int,
//		streams []string,
//		filter coretools.Filter,
//	) (coretools.List, error) {
//		c.Assert(majorVersion, tc.Equals, 4)
//		c.Assert(minorVersion, tc.Equals, 0)
//		c.Assert(streams, tc.DeepEquals, []string{"testing", "devel", "proposed", "released"})
//		c.Assert(filter, tc.DeepEquals, coretools.Filter{
//			Number: semversion.Number{Major: majorVersion, Minor: minorVersion, Patch: 1},
//			Arch:   "amd64",
//		})
//		return coretools.List{
//			{
//				Version: semVersionBinary,
//				URL:     toolURL,
//				Size:    int64(len(payload)),
//			},
//		}, nil
//	}
//
//	req, err := http.NewRequestWithContext(c.Context(), http.MethodGet, toolURL, nil)
//	c.Assert(err, tc.IsNil)
//	req.Header.Set(headerAccept, gzipContentType)
//	gzipReader := io.NopCloser(bytes.NewReader(buf.Bytes()))
//	s.mockHTTPClient.EXPECT().Do(req).Return(&http.Response{
//		StatusCode: http.StatusNotFound,
//		Body:       gzipReader,
//	}, nil)
//
//	simpleStreamStore := NewSimpleStreamAgentBinaryStore(func(context.Context) (ProviderForAgentBinaryFinder, error) {
//		return s.mockProviderForAgentBinaryFinder, nil
//	}, agentBinaryFilter, s.mockHTTPClient)
//
//	data, size, _, err := simpleStreamStore.GetAgentBinaryWithSHA256(c.Context(), ver, domainagentbinary.AgentStreamTesting)
//	c.Assert(err, tc.ErrorIs, domainagenterrors.NotFound)
//	c.Assert(data, tc.IsNil)
//	c.Assert(size, tc.Equals, int64(0))
//}
//
//func (s *simplestreamStoreSuite) TestGetAgentBinaryWithSHA256NotAcceptable(c *tc.C) {
//	ctrl := s.setupMocks(c)
//	defer ctrl.Finish()
//
//	semVersionBinary := semversion.MustParseBinary("4.0.1-ubuntu-amd64")
//
//	ver := agentbinary.Version{
//		Number: semVersionBinary.Number,
//		Arch:   semVersionBinary.Arch,
//	}
//
//	toolURL := "testURL"
//	var buf bytes.Buffer
//	gz := gzip.NewWriter(&buf)
//	payload := []byte("hello-agent-binary")
//	_, err := gz.Write(payload)
//	c.Assert(err, tc.IsNil)
//	c.Assert(gz.Close(), tc.IsNil) // flush gzip footer
//
//	agentBinaryFilter := func(
//		_ context.Context,
//		_ envtools.SimplestreamsFetcher,
//		_ environs.BootstrapEnviron,
//		majorVersion,
//		minorVersion int,
//		streams []string,
//		filter coretools.Filter,
//	) (coretools.List, error) {
//		c.Assert(majorVersion, tc.Equals, 4)
//		c.Assert(minorVersion, tc.Equals, 0)
//		c.Assert(streams, tc.DeepEquals, []string{"proposed", "released"})
//		c.Assert(filter, tc.DeepEquals, coretools.Filter{
//			Number: semversion.Number{Major: majorVersion, Minor: minorVersion, Patch: 1},
//			Arch:   "amd64",
//		})
//		return coretools.List{
//			{
//				Version: semVersionBinary,
//				URL:     toolURL,
//			},
//		}, nil
//	}
//
//	req, err := http.NewRequestWithContext(c.Context(), http.MethodGet, toolURL, nil)
//	c.Assert(err, tc.IsNil)
//	req.Header.Set(headerAccept, gzipContentType)
//	gzipReader := io.NopCloser(bytes.NewReader(buf.Bytes()))
//
//	s.mockHTTPClient.EXPECT().Do(req).Return(&http.Response{
//		StatusCode: http.StatusNotAcceptable,
//		Header: http.Header{
//			headerContentType: []string{gzipContentType},
//		},
//		Body: gzipReader,
//	}, nil)
//
//	simpleStreamStore := NewSimpleStreamAgentBinaryStore(func(context.Context) (ProviderForAgentBinaryFinder, error) {
//		return s.mockProviderForAgentBinaryFinder, nil
//	}, agentBinaryFilter, s.mockHTTPClient)
//
//	data, size, _, err := simpleStreamStore.GetAgentBinaryWithSHA256(c.Context(), ver, domainagentbinary.AgentStreamProposed)
//	c.Assert(err, tc.ErrorMatches, `simplestreams url "testURL" does not support expected content type "application/gzip"`)
//	c.Assert(data, tc.IsNil)
//	c.Assert(size, tc.Equals, int64(0))
//}
//
