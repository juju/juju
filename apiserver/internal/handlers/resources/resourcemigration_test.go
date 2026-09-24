// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/apiserverhttp"
	resourcesdownload "github.com/juju/juju/apiserver/internal/handlers/resources/download"
	"github.com/juju/juju/core/resource"
	charmresource "github.com/juju/juju/domain/deployment/charm/resource"
	domainresource "github.com/juju/juju/domain/resource"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/rpc/params"
)

const migrateResourcesPrefix = "/migrate/resources"

type resourcesUploadSuite struct {
	resourceServiceGetter *MockResourceServiceGetter
	resourceService       *MockResourceService

	modelServiceGetter *MockModelServiceGetter
	modelService       *MockModelService

	downloader *MockDownloader

	content        string
	origin         charmresource.Origin
	originStr      string
	revision       int
	revisionStr    string
	size           int64
	sizeStr        string
	fingerprint    charmresource.Fingerprint
	fingerprintStr string

	mux *apiserverhttp.Mux
	srv *httptest.Server
}

func TestResourcesUploadSuite(t *testing.T) {
	tc.Run(t, &resourcesUploadSuite{})
}

func (s *resourcesUploadSuite) SetUpTest(c *tc.C) {
	s.content = "resource-content"
	s.origin = charmresource.OriginStore
	s.originStr = s.origin.String()
	s.revision = 3
	s.revisionStr = strconv.Itoa(s.revision)
	s.size = int64(len(s.content))
	s.sizeStr = strconv.Itoa(int(s.size))
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(s.content))
	c.Assert(err, tc.ErrorIsNil)
	s.fingerprint = fp
	s.fingerprintStr = fp.String()

	s.mux = apiserverhttp.NewMux()
	s.srv = httptest.NewServer(s.mux)
}

func (s *resourcesUploadSuite) TearDownTest(c *tc.C) {
	s.srv.Close()
}

func (s *resourcesUploadSuite) TestStub(c *tc.C) {
	c.Skip("This suite is missing tests for the following scenarios:\n" +
		"- Sending a POST req requires authorization via unit or application only.\n" +
		"- Rejects an unknown model with http.StatusNotFound.\n")
}

// TestServeMethodNotSupported ensures that the handler rejects HTTP methods
// other than POST with a 405 Method Not Allowed response.
func (s *resourcesUploadSuite) TestServeMethodNotSupported(c *tc.C) {
	// Arrange
	handler := NewResourceMigrationUploadHandler(
		nil, // application service getter (unused for non-POST)
		nil, // resource service getter (unused for non-POST)
		nil, // downloader (unused for non-POST)
		loggertesting.WrapCheckLog(c),
	)
	unsupportedMethods := []string{
		http.MethodGet,
		http.MethodHead,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodConnect,
		http.MethodOptions,
		http.MethodTrace,
	}

	// This is a bit pathological, but we want to make sure that the handler
	// logic only actions on POST requests.
	for _, method := range unsupportedMethods {
		s.mux.AddHandler(method, migrateResourcesPrefix, handler)
		defer s.mux.RemoveHandler(method, migrateResourcesPrefix)
	}

	url := s.srv.URL + migrateResourcesPrefix
	for _, method := range unsupportedMethods {
		// Act
		request, err := http.NewRequest(method, url, nil)
		c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) unexpected error while building request. method: %s", method))
		response, err := http.DefaultClient.Do(request)
		c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) unexpected error while executing request. method: %s", method))
		defer response.Body.Close()

		// Assert
		c.Check(response.StatusCode, tc.Equals, http.StatusMethodNotAllowed,
			tc.Commentf("(Assert) unexpected status code. method: %s", method))
	}
}

// TestServeUploadModelNotImporting verifies that POST requests are rejected
// with 400 Bad Request when the model is not importing.
func (s *resourcesUploadSuite) TestServeUploadModelNotImporting(c *tc.C) {
	// Arrange
	defer s.setupHandlerWithImporting(c, false).Finish()
	query := url.Values{
		"name":        {"resource-name"},
		"application": {"app-name"},
		"timestamp":   {"not-placeholder"},
	}

	// Act
	response, err := http.Post(s.srv.URL+migrateResourcesPrefix+"?"+query.Encode(), "application/octet-stream", http.NoBody)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) unexpected error while executing request"))
	defer response.Body.Close()

	// Assert
	c.Check(response.StatusCode, tc.Equals, http.StatusBadRequest)
}

// TestServeUploadApplicationResourceNotFound verifies the handler's behavior
// when the application resource is not found.
func (s *resourcesUploadSuite) TestServeUploadApplicationResourceNotFound(c *tc.C) {
	// Arrange
	defer s.setupHandler(c).Finish()
	query := url.Values{
		"name":        {"resource-name"},
		"application": {"app-name"},
		"timestamp":   {"not-placeholder"},
	}
	s.resourceService.EXPECT().GetResourceUUIDByApplicationAndResourceName(
		gomock.Any(),
		"app-name",
		"resource-name",
	).Return("", errors.NotFound)

	// Act
	response, err := http.Post(s.srv.URL+migrateResourcesPrefix+"?"+query.Encode(), "application/octet-stream", nil)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) unexpected error while executing request"))
	defer response.Body.Close()

	// Assert
	c.Check(response.StatusCode, tc.Equals, http.StatusNotFound,
		tc.Commentf("(Assert) unexpected status code."))
}

// TestServeUploadApplicationStoreResourceError verifies error handling
// when storing a resource during an upload operation.
func (s *resourcesUploadSuite) TestServeUploadApplicationStoreResourceError(c *tc.C) {
	// Arrange
	defer s.setupHandler(c).Finish()
	query := url.Values{
		"name":        {"resource-name"},
		"application": {"app-name"},
		"timestamp":   {"not-placeholder"},
		"origin":      {"store"},
		"size":        {s.sizeStr},
		"fingerprint": {s.fingerprintStr},
		"revision":    {s.revisionStr},
	}
	s.resourceService.EXPECT().GetResourceUUIDByApplicationAndResourceName(
		gomock.Any(),
		"app-name",
		"resource-name",
	).Return("res-uuid", nil)
	s.downloader.EXPECT().Download(gomock.Any(), gomock.Any(), s.fingerprintStr, s.size).Return(http.NoBody, nil)
	s.resourceService.EXPECT().StoreResource(gomock.Any(), gomock.Any()).Return(
		resource.Resource{}, errors.New("cannot store resource"))
	s.resourceService.EXPECT().GetResource(gomock.Any(), resource.UUID("res-uuid")).Return(resource.Resource{
		ID: "res-uuid",
		Resource: charmresource.Resource{
			Origin:   s.origin,
			Revision: s.revision,
		},
	}, nil)

	// Act
	response, err := http.Post(s.srv.URL+migrateResourcesPrefix+"?"+query.Encode(), "application/octet-stream", nil)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) unexpected error while executing request"))
	defer response.Body.Close()

	// Assert
	c.Check(response.StatusCode, tc.Equals, http.StatusInternalServerError,
		tc.Commentf("(Assert) unexpected status code."))
}

// TestServeUploadApplicationGetResourceError validates the behavior
// when an error occurs while retrieving a resource.
func (s *resourcesUploadSuite) TestServeUploadApplicationGetResourceError(c *tc.C) {
	// Arrange
	defer s.setupHandler(c).Finish()
	query := url.Values{
		"name":        {"resource-name"},
		"application": {"app-name"},
		"timestamp":   {"not-placeholder"},
		"origin":      {s.originStr},
		"size":        {s.sizeStr},
		"fingerprint": {s.fingerprintStr},
		"revision":    {s.revisionStr},
	}
	s.resourceService.EXPECT().GetResourceUUIDByApplicationAndResourceName(
		gomock.Any(),
		"app-name",
		"resource-name",
	).Return("res-uuid", nil)
	s.resourceService.EXPECT().GetResource(gomock.Any(), gomock.Any()).Return(resource.Resource{}, errors.New(
		"cannot get resource"))

	// Act
	response, err := http.Post(s.srv.URL+migrateResourcesPrefix+"?"+query.Encode(), "application/octet-stream", nil)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) unexpected error while executing request"))
	defer response.Body.Close()

	// Assert
	c.Check(response.StatusCode, tc.Equals, http.StatusInternalServerError,
		tc.Commentf("(Assert) unexpected status code."))
}

// TestServeUploadApplicationWithPlaceholder tests the application's ability to
// handle uploading with placeholders correctly. It verifies that StoreResource is
// not called through not configuring related mock.
func (s *resourcesUploadSuite) TestServeUploadApplicationWithPlaceholder(c *tc.C) {
	// Arrange
	defer s.setupHandler(c).Finish()
	query := url.Values{
		"name":        {"resource-name"},
		"application": {"app-name"},
	}

	// Act
	response, err := http.Post(s.srv.URL+migrateResourcesPrefix+"?"+query.Encode(), "application/octet-stream", http.NoBody)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) unexpected error while executing request"))
	defer response.Body.Close()

	// Assert
	c.Check(response.StatusCode, tc.Equals, http.StatusOK,
		tc.Commentf("(Assert) unexpected status code."))
}

// TestServeUploadApplication tests the HTTP endpoint for uploading application
// resources, ensuring correct handling and storage. It verifies correct values
// are passed as argument for calling the different underlying services.
func (s *resourcesUploadSuite) TestServeUploadApplication(c *tc.C) {
	// Arrange
	now := time.Now().Truncate(time.Second).UTC()
	defer s.setupHandler(c).Finish()
	query := url.Values{
		"name":        {"resource-name"},
		"application": {"app-name"},
		"timestamp":   {"not-placeholder"},
		"origin":      {s.originStr},
		"size":        {s.sizeStr},
		"fingerprint": {s.fingerprintStr},
		"revision":    {s.revisionStr},
	}
	s.resourceService.EXPECT().GetResourceUUIDByApplicationAndResourceName(
		gomock.Any(),
		"app-name",
		"resource-name",
	).Return("res-uuid", nil)
	expectedResource := resource.Resource{
		ID: "res-uuid",
		Resource: charmresource.Resource{
			Origin:   s.origin,
			Revision: s.revision,
		},
		Timestamp: now,
	}
	s.downloader.EXPECT().Download(gomock.Any(), gomock.Any(), s.fingerprintStr, s.size).Return(http.NoBody, nil)
	s.resourceService.EXPECT().StoreResource(gomock.Any(), domainresource.StoreResourceArgs{
		ResourceUUID:    "res-uuid",
		Reader:          http.NoBody,
		RetrievedByType: resource.Application,
		Fingerprint:     s.fingerprint,
		Size:            s.size,
	}).Return(expectedResource, nil)
	s.resourceService.EXPECT().GetResource(gomock.Any(), resource.UUID("res-uuid")).Return(expectedResource, nil)

	// Act
	response, err := http.Post(s.srv.URL+migrateResourcesPrefix+"?"+query.Encode(), "application/octet-stream", http.NoBody)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) unexpected error while executing request"))
	defer response.Body.Close()

	// Assert
	var obtained params.ResourceUploadResult
	c.Check(response.StatusCode, tc.Equals, http.StatusOK,
		tc.Commentf("(Assert) unexpected status code."))
	body, err := io.ReadAll(response.Body)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) unexpected error while reading response body"))
	c.Assert(json.Unmarshal(body, &obtained), tc.ErrorIsNil,
		tc.Commentf("(Assert) unexpected error while unmarshalling response"))
	c.Check(obtained, tc.Equals, params.ResourceUploadResult{
		ID:        "res-uuid",
		Timestamp: now,
	})
}

// TestServeUploadApplicationRetrievedByUser tests that the RetrievedBy and
// RetrievedByType values are correctly determined for a user retriever.
func (s *resourcesUploadSuite) TestServeUploadApplicationRetrievedByUser(c *tc.C) {
	// Arrange
	now := time.Now().Truncate(time.Second).UTC()
	defer s.setupHandler(c).Finish()
	query := url.Values{
		"name":        {"resource-name"},
		"application": {"app-name"},
		"timestamp":   {"not-placeholder"},
		"user":        {"username"},
		"origin":      {"upload"},
		"size":        {s.sizeStr},
		"fingerprint": {s.fingerprintStr},
	}
	s.resourceService.EXPECT().GetResourceUUIDByApplicationAndResourceName(
		gomock.Any(),
		"app-name",
		"resource-name",
	).Return("res-uuid", nil)
	expectedRes := resource.Resource{
		ID: "res-uuid",
		Resource: charmresource.Resource{
			Origin:   charmresource.OriginUpload,
			Revision: -1,
		},
		Timestamp: now,
	}
	s.downloader.EXPECT().Download(gomock.Any(), gomock.Any(), s.fingerprintStr, s.size).Return(http.NoBody, nil)
	s.resourceService.EXPECT().StoreResource(gomock.Any(), domainresource.StoreResourceArgs{
		ResourceUUID:    "res-uuid",
		Reader:          http.NoBody,
		RetrievedByType: resource.User,
		RetrievedBy:     "username",
		Fingerprint:     s.fingerprint,
		Size:            s.size,
	}).Return(expectedRes, nil)
	s.resourceService.EXPECT().GetResource(gomock.Any(), resource.UUID("res-uuid")).Return(expectedRes, nil)

	// Act
	response, err := http.Post(s.srv.URL+migrateResourcesPrefix+"?"+query.Encode(), "application/octet-stream", http.NoBody)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) unexpected error while executing request"))
	defer response.Body.Close()
}

// TestServeUploadApplicationRetrievedByApplication tests that the RetrievedBy
// and RetrievedByType values are correctly determined for an application
// retriever.
func (s *resourcesUploadSuite) TestServeUploadApplicationRetrievedByApplication(c *tc.C) {
	// Arrange
	now := time.Now().Truncate(time.Second).UTC()
	defer s.setupHandler(c).Finish()
	query := url.Values{
		"name":        {"resource-name"},
		"application": {"app-name"},
		"timestamp":   {"not-placeholder"},
		"user":        {"app-name"},
		"origin":      {"store"},
		"size":        {s.sizeStr},
		"fingerprint": {s.fingerprintStr},
		"revision":    {s.revisionStr},
	}
	s.resourceService.EXPECT().GetResourceUUIDByApplicationAndResourceName(
		gomock.Any(),
		"app-name",
		"resource-name",
	).Return("res-uuid", nil)
	expectedRes := resource.Resource{
		ID: "res-uuid",
		Resource: charmresource.Resource{
			Origin:   s.origin,
			Revision: s.revision,
		},
		Timestamp: now,
	}
	s.downloader.EXPECT().Download(gomock.Any(), gomock.Any(), s.fingerprintStr, s.size).Return(http.NoBody, nil)
	s.resourceService.EXPECT().StoreResource(gomock.Any(), domainresource.StoreResourceArgs{
		ResourceUUID:    "res-uuid",
		Reader:          http.NoBody,
		RetrievedByType: resource.Application,
		RetrievedBy:     "app-name",
		Fingerprint:     s.fingerprint,
		Size:            s.size,
	}).Return(expectedRes, nil)
	s.resourceService.EXPECT().GetResource(gomock.Any(), resource.UUID("res-uuid")).Return(expectedRes, nil)

	// Act
	response, err := http.Post(s.srv.URL+migrateResourcesPrefix+"?"+query.Encode(), "application/octet-stream", http.NoBody)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) unexpected error while executing request"))
	defer response.Body.Close()
}

// TestServeUploadApplicationRetrievedByUnit tests that the RetrievedBy and
// RetrievedByType values are correctly determined for a unit retriever.
func (s *resourcesUploadSuite) TestServeUploadApplicationRetrievedByUnit(c *tc.C) {
	// Arrange
	now := time.Now().Truncate(time.Second).UTC()
	defer s.setupHandler(c).Finish()
	query := url.Values{
		"name":        {"resource-name"},
		"application": {"app-name"},
		"timestamp":   {"not-placeholder"},
		"user":        {"app-name/0"},
		"origin":      {"store"},
		"size":        {s.sizeStr},
		"fingerprint": {s.fingerprintStr},
		"revision":    {s.revisionStr},
	}
	s.resourceService.EXPECT().GetResourceUUIDByApplicationAndResourceName(
		gomock.Any(),
		"app-name",
		"resource-name",
	).Return("res-uuid", nil)
	expectedRes := resource.Resource{
		ID: "res-uuid",
		Resource: charmresource.Resource{
			Origin:   s.origin,
			Revision: s.revision,
		},
		Timestamp: now,
	}
	s.downloader.EXPECT().Download(gomock.Any(), gomock.Any(), s.fingerprintStr, s.size).Return(http.NoBody, nil)
	s.resourceService.EXPECT().StoreResource(gomock.Any(), domainresource.StoreResourceArgs{
		ResourceUUID:    "res-uuid",
		Reader:          http.NoBody,
		RetrievedByType: resource.Unit,
		RetrievedBy:     "app-name/0",
		Fingerprint:     s.fingerprint,
		Size:            s.size,
	}).Return(expectedRes, nil)
	s.resourceService.EXPECT().GetResource(gomock.Any(), resource.UUID("res-uuid")).Return(expectedRes, nil)

	// Act
	response, err := http.Post(s.srv.URL+migrateResourcesPrefix+"?"+query.Encode(), "application/octet-stream", http.NoBody)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) unexpected error while executing request"))
	defer response.Body.Close()
}

// TestServeUploadApplicationFingerprintMismatch verifies that a file resource
// blob whose hash does not match the claimed fingerprint is rejected with a
// 400 and never stored.
func (s *resourcesUploadSuite) TestServeUploadApplicationFingerprintMismatch(c *tc.C) {
	// Arrange
	defer s.setupHandlerWithDownloader(c,
		resourcesdownload.NewDownloader(loggertesting.WrapCheckLog(c), resourcesdownload.DefaultFileSystem())).Finish()
	query := url.Values{
		"name":        {"resource-name"},
		"application": {"app-name"},
		"timestamp":   {"not-placeholder"},
		"origin":      {s.originStr},
		"size":        {s.sizeStr},
		"fingerprint": {strings.Repeat("a", 96)},
		"revision":    {s.revisionStr},
	}
	s.resourceService.EXPECT().GetResourceUUIDByApplicationAndResourceName(
		gomock.Any(),
		"app-name",
		"resource-name",
	).Return("res-uuid", nil)
	s.resourceService.EXPECT().GetResource(gomock.Any(), resource.UUID("res-uuid")).Return(resource.Resource{
		ID: "res-uuid",
		Resource: charmresource.Resource{
			Origin:   s.origin,
			Revision: s.revision,
			Type:     charmresource.TypeFile,
		},
	}, nil)
	// StoreResource is deliberately not expected: the upload must be rejected.

	// Act
	response, err := http.Post(s.srv.URL+migrateResourcesPrefix+"?"+query.Encode(),
		"application/octet-stream", strings.NewReader(s.content))
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) unexpected error while executing request"))
	defer response.Body.Close()

	// Assert
	c.Check(response.StatusCode, tc.Equals, http.StatusBadRequest,
		tc.Commentf("(Assert) unexpected status code."))
	body, err := io.ReadAll(response.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(body), tc.Matches, ".*unexpected hash.*")
}

// TestServeUploadApplicationSizeMismatch verifies that a file resource blob
// whose size does not match the claimed size is rejected with a 400 and never
// stored.
func (s *resourcesUploadSuite) TestServeUploadApplicationSizeMismatch(c *tc.C) {
	// Arrange
	defer s.setupHandlerWithDownloader(c,
		resourcesdownload.NewDownloader(loggertesting.WrapCheckLog(c), resourcesdownload.DefaultFileSystem())).Finish()
	query := url.Values{
		"name":        {"resource-name"},
		"application": {"app-name"},
		"timestamp":   {"not-placeholder"},
		"origin":      {s.originStr},
		"size":        {strconv.Itoa(int(s.size + 5))},
		"fingerprint": {s.fingerprintStr},
		"revision":    {s.revisionStr},
	}
	s.resourceService.EXPECT().GetResourceUUIDByApplicationAndResourceName(
		gomock.Any(),
		"app-name",
		"resource-name",
	).Return("res-uuid", nil)
	s.resourceService.EXPECT().GetResource(gomock.Any(), resource.UUID("res-uuid")).Return(resource.Resource{
		ID: "res-uuid",
		Resource: charmresource.Resource{
			Origin:   s.origin,
			Revision: s.revision,
			Type:     charmresource.TypeFile,
		},
	}, nil)
	// StoreResource is deliberately not expected: the upload must be rejected.

	// Act
	response, err := http.Post(s.srv.URL+migrateResourcesPrefix+"?"+query.Encode(),
		"application/octet-stream", strings.NewReader(s.content))
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) unexpected error while executing request"))
	defer response.Body.Close()

	// Assert
	c.Check(response.StatusCode, tc.Equals, http.StatusBadRequest,
		tc.Commentf("(Assert) unexpected status code."))
	body, err := io.ReadAll(response.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(body), tc.Matches, ".*unexpected size.*")
}

// TestServeUploadApplicationOversizedBody verifies that a body longer than the
// claimed size is rejected with a 400 and never stored: the read is bounded at
// the claimed size, so the validation fails instead of the blob being buffered
// in full.
func (s *resourcesUploadSuite) TestServeUploadApplicationOversizedBody(c *tc.C) {
	// Arrange
	defer s.setupHandlerWithDownloader(c,
		resourcesdownload.NewDownloader(loggertesting.WrapCheckLog(c), resourcesdownload.DefaultFileSystem())).Finish()
	query := url.Values{
		"name":        {"resource-name"},
		"application": {"app-name"},
		"timestamp":   {"not-placeholder"},
		"origin":      {s.originStr},
		"size":        {s.sizeStr},
		"fingerprint": {s.fingerprintStr},
		"revision":    {s.revisionStr},
	}
	s.resourceService.EXPECT().GetResourceUUIDByApplicationAndResourceName(
		gomock.Any(),
		"app-name",
		"resource-name",
	).Return("res-uuid", nil)
	s.resourceService.EXPECT().GetResource(gomock.Any(), resource.UUID("res-uuid")).Return(resource.Resource{
		ID: "res-uuid",
		Resource: charmresource.Resource{
			Origin:   s.origin,
			Revision: s.revision,
			Type:     charmresource.TypeFile,
		},
	}, nil)
	// StoreResource is deliberately not expected: the upload must be rejected.

	// Act: the body is longer than the claimed size.
	response, err := http.Post(s.srv.URL+migrateResourcesPrefix+"?"+query.Encode(),
		"application/octet-stream", strings.NewReader(s.content+" and much more content"))
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) unexpected error while executing request"))
	defer response.Body.Close()

	// Assert
	c.Check(response.StatusCode, tc.Equals, http.StatusBadRequest,
		tc.Commentf("(Assert) unexpected status code."))
	body, err := io.ReadAll(response.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(body), tc.Matches, ".*validating resource blob.*")
}

// TestServeUploadApplicationValidBlob verifies that a file resource blob
// matching the claimed size and fingerprint is stored with the claimed values.
func (s *resourcesUploadSuite) TestServeUploadApplicationValidBlob(c *tc.C) {
	// Arrange
	now := time.Now().Truncate(time.Second).UTC()
	defer s.setupHandlerWithDownloader(c,
		resourcesdownload.NewDownloader(loggertesting.WrapCheckLog(c), resourcesdownload.DefaultFileSystem())).Finish()
	query := url.Values{
		"name":        {"resource-name"},
		"application": {"app-name"},
		"timestamp":   {"not-placeholder"},
		"origin":      {s.originStr},
		"size":        {s.sizeStr},
		"fingerprint": {s.fingerprintStr},
		"revision":    {s.revisionStr},
	}
	s.resourceService.EXPECT().GetResourceUUIDByApplicationAndResourceName(
		gomock.Any(),
		"app-name",
		"resource-name",
	).Return("res-uuid", nil)
	expectedRes := resource.Resource{
		ID: "res-uuid",
		Resource: charmresource.Resource{
			Origin:   s.origin,
			Revision: s.revision,
			Type:     charmresource.TypeFile,
		},
		Timestamp: now,
	}
	s.resourceService.EXPECT().GetResource(gomock.Any(), resource.UUID("res-uuid")).Return(expectedRes, nil)
	var storedArgs domainresource.StoreResourceArgs
	var storedContent string
	s.resourceService.EXPECT().StoreResource(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, args domainresource.StoreResourceArgs) (resource.Resource, error) {
			stored, rErr := io.ReadAll(args.Reader)
			c.Assert(rErr, tc.ErrorIsNil)
			storedContent = string(stored)
			storedArgs = args
			return expectedRes, nil
		})

	// Act
	response, err := http.Post(s.srv.URL+migrateResourcesPrefix+"?"+query.Encode(),
		"application/octet-stream", strings.NewReader(s.content))
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) unexpected error while executing request"))
	defer response.Body.Close()

	// Assert
	c.Check(response.StatusCode, tc.Equals, http.StatusOK,
		tc.Commentf("(Assert) unexpected status code."))
	c.Check(storedContent, tc.Equals, s.content)
	c.Check(storedArgs.Size, tc.Equals, s.size)
	c.Check(storedArgs.Fingerprint.String(), tc.Equals, s.fingerprint.String())
}

// TestServeUploadApplicationContainerImageSkipsValidation is a regression test
// for the Juju 3.6 export bug (see commit 432c96a1c2): container image
// resources are stored without blob validation, even when the claimed size and
// fingerprint do not match the blob bytes.
func (s *resourcesUploadSuite) TestServeUploadApplicationContainerImageSkipsValidation(c *tc.C) {
	// Arrange
	now := time.Now().Truncate(time.Second).UTC()
	defer s.setupHandler(c).Finish()
	// Claims deliberately do not match the body, simulating a resource
	// exported from Juju 3.6.
	query := url.Values{
		"name":        {"resource-name"},
		"application": {"app-name"},
		"timestamp":   {"not-placeholder"},
		"origin":      {s.originStr},
		"size":        {"9999"},
		"fingerprint": {strings.Repeat("a", 96)},
		"revision":    {s.revisionStr},
	}
	s.resourceService.EXPECT().GetResourceUUIDByApplicationAndResourceName(
		gomock.Any(),
		"app-name",
		"resource-name",
	).Return("res-uuid", nil)
	expectedRes := resource.Resource{
		ID: "res-uuid",
		Resource: charmresource.Resource{
			Origin:   s.origin,
			Revision: s.revision,
			Type:     charmresource.TypeContainerImage,
		},
		Timestamp: now,
	}
	s.resourceService.EXPECT().GetResource(gomock.Any(), resource.UUID("res-uuid")).Return(expectedRes, nil)
	// The downloader is deliberately not expected: container image resources
	// skip blob validation.
	var storedArgs domainresource.StoreResourceArgs
	var storedContent string
	s.resourceService.EXPECT().StoreResource(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, args domainresource.StoreResourceArgs) (resource.Resource, error) {
			stored, rErr := io.ReadAll(args.Reader)
			c.Assert(rErr, tc.ErrorIsNil)
			storedContent = string(stored)
			storedArgs = args
			return expectedRes, nil
		})
	body := `{"ImageName":"ghcr.io/foo/bar@sha256:deadbeef"}`

	// Act
	response, err := http.Post(s.srv.URL+migrateResourcesPrefix+"?"+query.Encode(),
		"application/octet-stream", strings.NewReader(body))
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) unexpected error while executing request"))
	defer response.Body.Close()

	// Assert
	c.Check(response.StatusCode, tc.Equals, http.StatusOK,
		tc.Commentf("(Assert) unexpected status code."))
	c.Check(storedContent, tc.Equals, body)
	// The wrong claims are passed through untouched; the container image
	// resource store re-derives them from the parsed metadata.
	c.Check(storedArgs.Size, tc.Equals, int64(9999))
	c.Check(storedArgs.Fingerprint.String(), tc.Equals, strings.Repeat("a", 96))
}

// TestServeUploadUnitWithPlaceholder tests the upload functionality for a unit
// with a placeholder in the resource upload service. It is basically the same
// test than the one with application, with one call to SetUnitResource.
func (s *resourcesUploadSuite) TestServeUploadUnitWithPlaceholder(c *tc.C) {
	// Arrange
	defer s.setupHandler(c).Finish()
	query := url.Values{
		"name": {"resource-name"},
		"unit": {"testunit-id"},
	}

	// Act
	response, err := http.Post(s.srv.URL+migrateResourcesPrefix+"?"+query.Encode(), "application/octet-stream", http.NoBody)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) unexpected error while executing request"))
	defer response.Body.Close()

	// Assert
	c.Check(response.StatusCode, tc.Equals, http.StatusOK,
		tc.Commentf("(Assert) unexpected status code."))
}

// TestServeUploadUnit tests the process of uploading a resource unit and
// verifies correct resource handling and response. It is basically the same
// test than the one with application, with one call to SetUnitResource.
func (s *resourcesUploadSuite) TestServeUploadUnit(c *tc.C) {
	// Arrange
	defer s.setupHandler(c).Finish()
	query := url.Values{
		"name":      {"resource-name"},
		"unit":      {"testunit-id"},
		"timestamp": {"not-placeholder"},
	}

	// Act
	response, err := http.Post(s.srv.URL+migrateResourcesPrefix+"?"+query.Encode(), "application/octet-stream", http.NoBody)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Act) unexpected error while executing request"))
	defer response.Body.Close()

	// Assert
	c.Check(response.StatusCode, tc.Equals, http.StatusOK,
		tc.Commentf("(Assert) unexpected status code."))
}

// setupHandler configures the resources migration upload HTTP handler for the
// common case where the model is importing, initialises mocks and registers it
// to the mux. It provides cleanup logic.
func (s *resourcesUploadSuite) setupHandler(c *tc.C) Finisher {
	return s.setupHandlerWithImporting(c, true)
}

// setupHandlerWithImporting configures the handler indicating whether the model
// is currently importing (migration in progress). When importing is false, the
// request is expected to be rejected before hitting the resource service.
func (s *resourcesUploadSuite) setupHandlerWithImporting(c *tc.C, importing bool) Finisher {
	finish := s.setupMocks(c).Finish
	s.expectApplicationService(importing)
	if importing {
		s.expectResourceService()
	}

	registered := s.registerHandlerWithDownloader(c, s.downloader)
	return &finisherWrapper{
		finish: func() {
			registered.Finish()
			finish()
		},
	}
}

// setupHandlerWithDownloader configures mocks for an importing model and
// registers the handler with the given downloader. Use it for tests that need
// a downloader other than the suite mock.
func (s *resourcesUploadSuite) setupHandlerWithDownloader(c *tc.C, downloading Downloader) Finisher {
	finish := s.setupMocks(c).Finish
	s.expectApplicationService(true)
	s.expectResourceService()

	registered := s.registerHandlerWithDownloader(c, downloading)
	return &finisherWrapper{
		finish: func() {
			registered.Finish()
			finish()
		},
	}
}

// registerHandlerWithDownloader registers the resources migration upload HTTP
// handler with the given downloader. Mocks must have been initialised first.
func (s *resourcesUploadSuite) registerHandlerWithDownloader(c *tc.C, downloading Downloader) Finisher {
	handler := NewResourceMigrationUploadHandler(
		s.modelServiceGetter,
		s.resourceServiceGetter,
		downloading,
		loggertesting.WrapCheckLog(c),
	)

	err := s.mux.AddHandler("POST", migrateResourcesPrefix, handler)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) unexpected error while adding handler"))

	return &finisherWrapper{
		finish: func() {
			s.mux.RemoveHandler("POST", migrateResourcesPrefix)
		},
	}
}

// expectResourceService prepare mocks for resource service
func (s *resourcesUploadSuite) expectResourceService() {
	s.resourceServiceGetter.EXPECT().Resource(gomock.Any()).Return(s.resourceService, nil)
}

// expectApplicationService prepare mocks for application service
func (s *resourcesUploadSuite) expectApplicationService(importing bool) {
	s.modelServiceGetter.EXPECT().Model(gomock.Any()).Return(s.modelService, nil)
	s.modelService.EXPECT().IsImportingModel(gomock.Any()).Return(importing, nil)
}

// setupMocks initializes mock services and returns a gomock.Controller
// for managing mock lifecycle.
func (s *resourcesUploadSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.resourceServiceGetter = NewMockResourceServiceGetter(ctrl)
	s.resourceService = NewMockResourceService(ctrl)
	s.modelServiceGetter = NewMockModelServiceGetter(ctrl)
	s.modelService = NewMockModelService(ctrl)
	s.downloader = NewMockDownloader(ctrl)

	return ctrl
}

type Finisher interface {
	Finish()
}

type finisherWrapper struct {
	finish func()
}

func (f *finisherWrapper) Finish() {
	f.finish()
}
