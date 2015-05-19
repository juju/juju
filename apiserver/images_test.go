// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package apiserver_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apihttp "github.com/juju/juju/apiserver/http"
	"github.com/juju/juju/apiserver/params"
	containertesting "github.com/juju/juju/container/testing"
	"github.com/juju/juju/environs/jujutest"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/imagestorage"
	coretesting "github.com/juju/juju/testing"
)

type imageSuite struct {
	userAuthHttpSuite
	archiveContentType string
	imageData          string
	imageChecksum      string
}

var _ = gc.Suite(&imageSuite{})

func (s *imageSuite) SetUpSuite(c *gc.C) {
	s.userAuthHttpSuite.SetUpSuite(c)
	s.archiveContentType = "application/x-tar-gz"
	s.imageData = "abc"
	s.imageChecksum = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	testRoundTripper.RegisterForScheme("test")
}

func (s *imageSuite) TestDownloadMissingEnvUUIDPath(c *gc.C) {
	s.storeFakeImage(c, s.State, "lxc", "trusty", "amd64")

	s.envUUID = ""
	url := s.imageURL(c, "lxc", "trusty", "amd64")
	c.Assert(url.Path, jc.HasPrefix, "/environment//images")

	response, err := s.downloadRequest(c, url)
	c.Assert(err, jc.ErrorIsNil)
	s.testDownload(c, response)
}

func (s *imageSuite) TestDownloadEnvironmentPath(c *gc.C) {
	s.storeFakeImage(c, s.State, "lxc", "trusty", "amd64")

	url := s.imageURL(c, "lxc", "trusty", "amd64")
	c.Assert(url.Path, jc.HasPrefix, fmt.Sprintf("/environment/%s/", s.State.EnvironUUID()))

	response, err := s.downloadRequest(c, url)
	c.Assert(err, jc.ErrorIsNil)
	s.testDownload(c, response)
}

func (s *imageSuite) TestDownloadOtherEnvironmentPath(c *gc.C) {
	envState := s.setupOtherEnvironment(c)
	s.storeFakeImage(c, envState, "lxc", "trusty", "amd64")

	url := s.imageURL(c, "lxc", "trusty", "amd64")
	c.Assert(url.Path, jc.HasPrefix, fmt.Sprintf("/environment/%s/", envState.EnvironUUID()))

	response, err := s.downloadRequest(c, url)
	c.Assert(err, jc.ErrorIsNil)
	s.testDownload(c, response)
}

func (s *imageSuite) TestDownloadRejectsWrongEnvUUIDPath(c *gc.C) {
	s.envUUID = "dead-beef-123456"
	url := s.imageURL(c, "lxc", "trusty", "amd64")
	response, err := s.downloadRequest(c, url)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, response, http.StatusNotFound, `unknown environment: "dead-beef-123456"`)
}

// This provides the content for code accessing test:///... URLs. This allows
// us to set the responses for things like image queries.
var testRoundTripper = &jujutest.ProxyRoundTripper{}

type CountingRoundTripper struct {
	count int
	*jujutest.CannedRoundTripper
}

func (v *CountingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	v.count += 1
	return v.CannedRoundTripper.RoundTrip(req)
}

func useTestImageData(files map[string]string) {
	if files != nil {
		testRoundTripper.Sub = &CountingRoundTripper{
			CannedRoundTripper: jujutest.NewCannedRoundTripper(files, nil),
		}
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
	url := s.imageURL(c, "lxc", "trusty", "amd64")
	response, err := s.downloadRequest(c, url)
	c.Assert(err, jc.ErrorIsNil)
	data := s.testDownload(c, response)

	metadata, cachedData := s.getImageFromStorage(c, s.State, "lxc", "trusty", "amd64")
	c.Assert(metadata.Size, gc.Equals, int64(len(s.imageData)))
	c.Assert(metadata.SHA256, gc.Equals, s.imageChecksum)
	c.Assert(metadata.SourceURL, gc.Equals, "test://cloud-images/trusty-released-amd64-root.tar.gz")
	c.Assert(string(data), gc.Equals, string(s.imageData))
	c.Assert(string(data), gc.Equals, string(cachedData))
}

func (s *imageSuite) TestDownloadFetchesAndCachesConcurrent(c *gc.C) {
	// Set up some image data for a fake server.
	testing.PatchExecutable(c, s, "ubuntu-cloudimg-query", containertesting.FakeLxcURLScript)
	useTestImageData(map[string]string{
		"/trusty-released-amd64-root.tar.gz": s.imageData,
		"/SHA256SUMS":                        s.imageChecksum + " *trusty-released-amd64-root.tar.gz",
	})
	defer func() {
		useTestImageData(nil)
	}()

	// Fetch the same image multiple times concurrently and ensure that
	// it is only downloaded from the external URL once.
	done := make(chan struct{})
	go func() {
		var wg sync.WaitGroup
		wg.Add(10)
		for i := 0; i < 10; i++ {
			go func() {
				defer wg.Done()
				url := s.imageURL(c, "lxc", "trusty", "amd64")
				response, err := s.downloadRequest(c, url)
				c.Assert(err, jc.ErrorIsNil)
				data := s.testDownload(c, response)
				c.Assert(string(data), gc.Equals, string(s.imageData))
			}()
		}
		wg.Wait()
		done <- struct{}{}
	}()
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for images to be fetced")
	}

	// Downloading an image is 2 requests - one for image, one for SA256.
	c.Assert(testRoundTripper.Sub.(*CountingRoundTripper).count, gc.Equals, 2)

	// Check that the image is correctly cached.
	metadata, cachedData := s.getImageFromStorage(c, s.State, "lxc", "trusty", "amd64")
	c.Assert(metadata.Size, gc.Equals, int64(len(s.imageData)))
	c.Assert(metadata.SHA256, gc.Equals, s.imageChecksum)
	c.Assert(metadata.SourceURL, gc.Equals, "test://cloud-images/trusty-released-amd64-root.tar.gz")
	c.Assert(s.imageData, gc.Equals, string(cachedData))
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

	resp, err := s.downloadRequest(c, s.imageURL(c, "lxc", "trusty", "amd64"))
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

	resp, err := s.downloadRequest(c, s.imageURL(c, "lxc", "trusty", "amd64"))
	defer resp.Body.Close()
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusInternalServerError, ".* cannot find sha256 checksum .*")
}

func (s *imageSuite) testDownload(c *gc.C, resp *http.Response) []byte {
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

func (s *imageSuite) downloadRequest(c *gc.C, url *url.URL) (*http.Response, error) {
	return s.sendRequest(c, "", "", "GET", url.String(), "", nil)
}

func (s *imageSuite) storeFakeImage(c *gc.C, st *state.State, kind, series, arch string) {
	storage := st.ImageStorage()
	metadata := &imagestorage.Metadata{
		EnvUUID:   st.EnvironUUID(),
		Kind:      kind,
		Series:    series,
		Arch:      arch,
		Size:      int64(len(s.imageData)),
		SHA256:    s.imageChecksum,
		SourceURL: "http://path",
	}
	err := storage.AddImage(strings.NewReader(s.imageData), metadata)
	c.Assert(err, gc.IsNil)
}

func (s *imageSuite) getImageFromStorage(c *gc.C, st *state.State, kind, series, arch string) (*imagestorage.Metadata, []byte) {
	storage := st.ImageStorage()
	metadata, r, err := storage.Image(kind, series, arch)
	c.Assert(err, gc.IsNil)
	data, err := ioutil.ReadAll(r)
	r.Close()
	c.Assert(err, gc.IsNil)
	return metadata, data
}

func (s *imageSuite) imageURL(c *gc.C, kind, series, arch string) *url.URL {
	uri := s.baseURL(c)
	uri.Path = fmt.Sprintf("/environment/%s/images/%s/%s/%s/trusty-released-amd64-root.tar.gz", s.envUUID, kind, series, arch)
	return uri
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
