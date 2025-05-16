// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/testcharms/repo"
)

const defaultSeries = "bionic"
const localCharmRepo = "../../testcharms/charm-repo"

type DownloadSuite struct {
	baseSuite
}

func TestDownloadSuite(t *stdtesting.T) { tc.Run(t, &DownloadSuite{}) }
func (s *DownloadSuite) TestDownload(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tmpFile, close := s.expectTmpFile(c)
	defer close()

	fileSystem := NewMockFileSystem(ctrl)
	fileSystem.EXPECT().Create(tmpFile.Name()).Return(tmpFile, nil)

	archiveBytes := s.createCharmAchieve(c)

	httpClient := NewMockHTTPClient(ctrl)
	httpClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(r *http.Request) (*http.Response, error) {

		return &http.Response{
			StatusCode:    200,
			Body:          io.NopCloser(bytes.NewBuffer(archiveBytes)),
			ContentLength: int64(len(archiveBytes)),
		}, nil
	})

	serverURL, err := url.Parse("http://meshuggah.rocks")
	c.Assert(err, tc.ErrorIsNil)

	client := NewDownloadClient(httpClient, fileSystem, s.logger)
	digest, err := client.Download(c.Context(), serverURL, tmpFile.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(digest, tc.DeepEquals, &Digest{
		SHA256: "679e21d12ebfd206ba08dd7a3a23b81170d30c8c7cbc0ac2443beb6aac67dfdb",
		SHA384: "5821c48bdfc6d6ec87cfd4fc1e5f26898a3c983ccdbc46816fe6938493cfb003ca9642087666af9e1c0b7397b0a33c8a",
		Size:   int64(len(archiveBytes)),
	})
}

func (s *DownloadSuite) TestDownloadWithProgressBar(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tmpFile, close := s.expectTmpFile(c)
	defer close()

	fileSystem := NewMockFileSystem(ctrl)
	fileSystem.EXPECT().Create(tmpFile.Name()).Return(tmpFile, nil)

	httpClient := NewMockHTTPClient(ctrl)
	httpClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    200,
			Body:          io.NopCloser(strings.NewReader("hello world")),
			ContentLength: 11,
		}, nil
	})

	serverURL, err := url.Parse("http://meshuggah.rocks")
	c.Assert(err, tc.ErrorIsNil)

	pgBar := NewMockProgressBar(ctrl)
	pgBar.EXPECT().Write(gomock.Any()).MinTimes(1).DoAndReturn(func(p []byte) (int, error) {
		return len(p), nil
	})
	pgBar.EXPECT().Start("dummy", float64(11))
	pgBar.EXPECT().Finished()

	ctx := context.WithValue(c.Context(), DownloadNameKey, "dummy")

	client := NewDownloadClient(httpClient, fileSystem, s.logger)
	digest, err := client.Download(ctx, serverURL, tmpFile.Name(), WithProgressBar(pgBar))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(digest, tc.DeepEquals, &Digest{
		SHA256: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
		SHA384: "fdbd8e75a67f29f701a4e040385e2e23986303ea10239211af907fcbb83578b3e417cb71ce646efd0819dd8c088de1bd",
		Size:   11,
	})
}

func (s *DownloadSuite) TestDownloadWithDigest(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tmpFile, close := s.expectTmpFile(c)
	defer close()

	fileSystem := NewMockFileSystem(ctrl)
	fileSystem.EXPECT().Create(tmpFile.Name()).Return(tmpFile, nil)

	httpClient := NewMockHTTPClient(ctrl)
	httpClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    200,
			Body:          io.NopCloser(strings.NewReader("hello world")),
			ContentLength: 11,
		}, nil
	})

	serverURL, err := url.Parse("http://meshuggah.rocks")
	c.Assert(err, tc.ErrorIsNil)

	client := NewDownloadClient(httpClient, fileSystem, s.logger)
	digest, err := client.Download(c.Context(), serverURL, tmpFile.Name())
	c.Assert(err, tc.ErrorIsNil)

	expectedSHA256 := readSHA256(c, strings.NewReader("hello world"))
	expectedSHA384 := readSHA384(c, strings.NewReader("hello world"))

	c.Check(digest, tc.DeepEquals, &Digest{
		SHA256: expectedSHA256,
		SHA384: expectedSHA384,
		Size:   11,
	})
}

func (s *DownloadSuite) TestDownloadWithNotFoundStatusCode(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tmpFile, close := s.expectTmpFile(c)
	defer close()

	fileSystem := NewMockFileSystem(ctrl)
	fileSystem.EXPECT().Create(tmpFile.Name()).Return(tmpFile, nil)

	httpClient := NewMockHTTPClient(ctrl)
	httpClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 404,
			Body:       io.NopCloser(bytes.NewBufferString("")),
		}, nil
	})

	serverURL, err := url.Parse("http://meshuggah.rocks")
	c.Assert(err, tc.ErrorIsNil)

	client := NewDownloadClient(httpClient, fileSystem, s.logger)
	_, err = client.Download(c.Context(), serverURL, tmpFile.Name())
	c.Assert(err, tc.ErrorMatches, `cannot retrieve "http://meshuggah.rocks": archive not found`)
}

func (s *DownloadSuite) TestDownloadWithFailedStatusCode(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tmpFile, close := s.expectTmpFile(c)
	defer close()

	fileSystem := NewMockFileSystem(ctrl)
	fileSystem.EXPECT().Create(tmpFile.Name()).Return(tmpFile, nil)

	httpClient := NewMockHTTPClient(ctrl)
	httpClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			Status:     http.StatusText(http.StatusInternalServerError),
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString("")),
		}, nil
	})

	serverURL, err := url.Parse("http://meshuggah.rocks")
	c.Assert(err, tc.ErrorIsNil)

	client := NewDownloadClient(httpClient, fileSystem, s.logger)
	_, err = client.Download(c.Context(), serverURL, tmpFile.Name())
	c.Assert(err, tc.ErrorMatches, `cannot retrieve "http://meshuggah.rocks": unable to locate archive \(store API responded with status: Internal Server Error\)`)
}

func (s *DownloadSuite) createCharmAchieve(c *tc.C) []byte {
	tmpDir, err := os.MkdirTemp("", "charm")
	c.Assert(err, tc.ErrorIsNil)

	repo := repo.NewRepo(localCharmRepo, defaultSeries)
	charmPath := repo.CharmArchivePath(tmpDir, "dummy")

	path, err := os.ReadFile(charmPath)
	c.Assert(err, tc.ErrorIsNil)
	return path
}

func (s *DownloadSuite) expectTmpFile(c *tc.C) (*os.File, func()) {
	tmpFile, err := os.CreateTemp("", "charm")
	c.Assert(err, tc.ErrorIsNil)

	return tmpFile, func() {
		err := os.Remove(tmpFile.Name())
		c.Assert(err, tc.ErrorIsNil)
	}
}

func readSHA256(c *tc.C, reader io.Reader) string {
	hash := sha256.New()
	_, err := io.Copy(hash, reader)
	c.Assert(err, tc.ErrorIsNil)

	return hex.EncodeToString(hash.Sum(nil))
}

func readSHA384(c *tc.C, reader io.Reader) string {
	hash := sha512.New384()
	_, err := io.Copy(hash, reader)
	c.Assert(err, tc.ErrorIsNil)

	return hex.EncodeToString(hash.Sum(nil))
}
