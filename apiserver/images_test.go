// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	apihttp "github.com/juju/juju/apiserver/http"
	"github.com/juju/juju/apiserver/params"
	containertesting "github.com/juju/juju/container/testing"
	"github.com/juju/juju/environs/jujutest"
	"github.com/juju/juju/state/imagestorage"
)

type imageSuite struct {
	authHttpSuite
	archiveContentType string
	imageData          string
	imageChecksum      string
}

var _ = gc.Suite(&imageSuite{})

func (s *imageSuite) SetUpSuite(c *gc.C) {
	s.authHttpSuite.SetUpSuite(c)
	s.archiveContentType = "application/x-tar-gz"
	s.imageData = "abc"
	s.imageChecksum = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	testRoundTripper.RegisterForScheme("test")

}

func (s *imageSuite) TestDownload(c *gc.C) {
	environ, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	s.storeFakeImage(c, environ.UUID(), "lxc", "trusty", "amd64")
	s.testDownload(c, "lxc", "trusty", "amd64", environ.UUID())
}

func (s *imageSuite) TestDownloadRejectsWrongEnvUUIDPath(c *gc.C) {
	resp, err := s.downloadRequest(c, "dead-beef-123456", "lxc", "trusty", "amd64")
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusNotFound, `unknown environment: "dead-beef-123456"`)
}

// This provides the content for code accessing test:///... URLs. This allows
// us to set the responses for things like image queries.
var testRoundTripper = &jujutest.ProxyRoundTripper{}

func useTestImageData(files map[string]string) {
	if files != nil {
		testRoundTripper.Sub = jujutest.NewCannedRoundTripper(files, nil)
	} else {
		testRoundTripper.Sub = nil
	}
}

func (s *imageSuite) TestDownloadFetchesAndCaches(c *gc.C) {
	// Set up some image data for a fake server.
	testing.PatchExecutable(c, s, "ubuntu-cloudimg-query", containertesting.FakeLxcURLScript)
	useTestImageData(map[string]string{
		"/trusty-released-amd64-root.tar.gz": s.imageData,
		"/SHA256SUMS":                        s.imageChecksum + " *trusty-released-amd64-root.tar.gz",
	})
	defer func() {
		useTestImageData(nil)
	}()

	// The image is not in imagestorage, so the download request causes
	// the API server to search for the image on cloud-images, fetches it,
	// and then cache it in imagestorage.
	environ, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	data := s.testDownload(c, "lxc", "trusty", "amd64", environ.UUID())

	metadata, cachedData := s.getImageFromStorage(c, "lxc", "trusty", "amd64")
	c.Assert(metadata.Size, gc.Equals, int64(len(s.imageData)))
	c.Assert(metadata.SHA256, gc.Equals, s.imageChecksum)
	c.Assert(string(data), gc.Equals, string(s.imageData))
	c.Assert(string(data), gc.Equals, string(cachedData))
}

func (s *imageSuite) TestDownloadFetchChecksumMismatch(c *gc.C) {
	// Set up some image data for a fake server.
	testing.PatchExecutable(c, s, "ubuntu-cloudimg-query", containertesting.FakeLxcURLScript)
	useTestImageData(map[string]string{
		"/trusty-released-amd64-root.tar.gz": s.imageData,
		"/SHA256SUMS":                        "different-checksum *trusty-released-amd64-root.tar.gz",
	})
	defer func() {
		useTestImageData(nil)
	}()

	resp, err := s.downloadRequest(c, "", "lxc", "trusty", "amd64")
	defer resp.Body.Close()
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusInternalServerError, ".* download checksum mismatch .*")
}

func (s *imageSuite) TestDownloadFetchNoSHA256File(c *gc.C) {
	// Set up some image data for a fake server.
	testing.PatchExecutable(c, s, "ubuntu-cloudimg-query", containertesting.FakeLxcURLScript)
	useTestImageData(map[string]string{
		"/trusty-released-amd64-root.tar.gz": s.imageData,
	})
	defer func() {
		useTestImageData(nil)
	}()

	resp, err := s.downloadRequest(c, "", "lxc", "trusty", "amd64")
	defer resp.Body.Close()
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusInternalServerError, ".* cannot find sha256 checksum .*")
}

func (s *imageSuite) testDownload(c *gc.C, kind, series, arch, uuid string) []byte {
	resp, err := s.downloadRequest(c, uuid, kind, series, arch)
	c.Assert(err, gc.IsNil)
	c.Check(resp.StatusCode, gc.Equals, http.StatusOK)
	c.Check(resp.Header.Get("Digest"), gc.Equals, string(apihttp.DigestSHA)+"="+s.imageChecksum)
	c.Check(resp.Header.Get("Content-Type"), gc.Equals, s.archiveContentType)
	c.Check(resp.Header.Get("Content-Length"), gc.Equals, fmt.Sprintf("%v", len(s.imageData)))

	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, gc.IsNil)

	c.Assert(data, gc.HasLen, len(s.imageData))

	hash := sha256.New()
	hash.Write(data)
	c.Assert(fmt.Sprintf("%x", hash.Sum(nil)), gc.Equals, s.imageChecksum)
	return data
}

func (s *imageSuite) downloadRequest(c *gc.C, uuid, kind, series, arch string) (*http.Response, error) {
	url := s.imageURL(c, "")
	url.Path = fmt.Sprintf("/environment/%s/images/%s/%s/%s/trusty-released-amd64-root.tar.gz", uuid, kind, series, arch)
	return s.sendRequest(c, "", "", "GET", url.String(), "", nil)
}

func (s *imageSuite) storeFakeImage(c *gc.C, uuid, kind, series, arch string) {
	storage := s.State.ImageStorage()
	metadata := &imagestorage.Metadata{
		EnvUUID: uuid,
		Kind:    kind,
		Series:  series,
		Arch:    arch,
		Size:    int64(len(s.imageData)),
		SHA256:  s.imageChecksum,
	}
	err := storage.AddImage(strings.NewReader(s.imageData), metadata)
	c.Assert(err, gc.IsNil)
}

func (s *imageSuite) getImageFromStorage(c *gc.C, kind, series, arch string) (*imagestorage.Metadata, []byte) {
	storage := s.State.ImageStorage()
	metadata, r, err := storage.Image(kind, series, arch)
	c.Assert(err, gc.IsNil)
	data, err := ioutil.ReadAll(r)
	r.Close()
	c.Assert(err, gc.IsNil)
	return metadata, data
}

func (s *imageSuite) imageURL(c *gc.C, query string) *url.URL {
	uri := s.baseURL(c)
	uri.Path += "/images"
	uri.RawQuery = query
	return uri
}

func (s *imageSuite) imageURI(c *gc.C, query string) string {
	if query != "" && query[0] == '?' {
		query = query[1:]
	}
	return s.imageURL(c, query).String()
}

func (s *imageSuite) assertErrorResponse(c *gc.C, resp *http.Response, expCode int, expError string) {
	body := assertResponse(c, resp, expCode, "application/json")
	err := jsonImageResponse(c, body).Error
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, expError)
}

func jsonImageResponse(c *gc.C, body []byte) (jsonResponse params.ErrorResult) {
	err := json.Unmarshal(body, &jsonResponse)
	c.Assert(err, gc.IsNil)
	return
}
