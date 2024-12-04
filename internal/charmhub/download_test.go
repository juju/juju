// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testcharms/repo"
)

const defaultSeries = "bionic"
const localCharmRepo = "../../testcharms/charm-repo"

type DownloadSuite struct {
	baseSuite
}

var _ = gc.Suite(&DownloadSuite{})

func (s *DownloadSuite) TestDownload(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)

	client := NewDownloadClient(httpClient, fileSystem, s.logger)
	digest, err := client.Download(context.Background(), serverURL, tmpFile.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(digest, gc.DeepEquals, &Digest{
		SHA256: "",
		SHA384: "",
		Size:   int64(len(archiveBytes)),
	})
}

func (s *DownloadSuite) TestDownloadWithProgressBar(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)

	pgBar := NewMockProgressBar(ctrl)
	pgBar.EXPECT().Write(gomock.Any()).MinTimes(1).DoAndReturn(func(p []byte) (int, error) {
		return len(p), nil
	})
	pgBar.EXPECT().Start("dummy", float64(11))
	pgBar.EXPECT().Finished()

	ctx := context.WithValue(context.Background(), DownloadNameKey, "dummy")

	client := NewDownloadClient(httpClient, fileSystem, s.logger)
	digest, err := client.Download(ctx, serverURL, tmpFile.Name(), WithProgressBar(pgBar))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(digest, gc.DeepEquals, &Digest{
		SHA256: "",
		SHA384: "",
		Size:   11,
	})
}

func (s *DownloadSuite) TestDownloadWithSHA256Digest(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)

	client := NewDownloadClient(httpClient, fileSystem, s.logger)
	digest, err := client.Download(context.Background(), serverURL, tmpFile.Name())
	c.Assert(err, jc.ErrorIsNil)

	expectedSHA256 := readSHA256(c, strings.NewReader("hello world"))
	expectedSHA384 := readSHA384(c, strings.NewReader("hello world"))

	c.Check(digest, gc.DeepEquals, &Digest{
		SHA256: expectedSHA256,
		SHA384: expectedSHA384,
		Size:   11,
	})
}

func (s *DownloadSuite) TestDownloadAndRead(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)

	client := NewDownloadClient(httpClient, fileSystem, s.logger)
	_, digest, err := client.DownloadAndRead(context.Background(), serverURL, tmpFile.Name())
	c.Assert(err, jc.ErrorIsNil)

	expectedSHA256 := readSHA256(c, bytes.NewBuffer(archiveBytes))
	expectedSHA384 := readSHA384(c, bytes.NewBuffer(archiveBytes))

	c.Check(digest, gc.DeepEquals, &Digest{
		SHA256: expectedSHA256,
		SHA384: expectedSHA384,
		Size:   int64(len(archiveBytes)),
	})
}

func (s *DownloadSuite) TestDownloadAndReadWithNotFoundStatusCode(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)

	client := NewDownloadClient(httpClient, fileSystem, s.logger)
	_, _, err = client.DownloadAndRead(context.Background(), serverURL, tmpFile.Name())
	c.Assert(err, gc.ErrorMatches, `cannot retrieve "http://meshuggah.rocks": archive not found`)
}

func (s *DownloadSuite) TestDownloadAndReadWithFailedStatusCode(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)

	client := NewDownloadClient(httpClient, fileSystem, s.logger)
	_, _, err = client.DownloadAndRead(context.Background(), serverURL, tmpFile.Name())
	c.Assert(err, gc.ErrorMatches, `cannot retrieve "http://meshuggah.rocks": unable to locate archive \(store API responded with status: Internal Server Error\)`)
}

func (s *DownloadSuite) createCharmAchieve(c *gc.C) []byte {
	tmpDir, err := os.MkdirTemp("", "charm")
	c.Assert(err, jc.ErrorIsNil)

	repo := repo.NewRepo(localCharmRepo, defaultSeries)
	charmPath := repo.CharmArchivePath(tmpDir, "dummy")

	path, err := os.ReadFile(charmPath)
	c.Assert(err, jc.ErrorIsNil)
	return path
}

func (s *DownloadSuite) expectTmpFile(c *gc.C) (*os.File, func()) {
	tmpFile, err := os.CreateTemp("", "charm")
	c.Assert(err, jc.ErrorIsNil)

	return tmpFile, func() {
		err := os.Remove(tmpFile.Name())
		c.Assert(err, jc.ErrorIsNil)
	}
}

func readSHA256(c *gc.C, reader io.Reader) string {
	hash := sha256.New()
	_, err := io.Copy(hash, reader)
	c.Assert(err, jc.ErrorIsNil)

	return fmt.Sprintf("%x", hash.Sum(nil))
}

func readSHA384(c *gc.C, reader io.Reader) string {
	hash := sha512.New384()
	_, err := io.Copy(hash, reader)
	c.Assert(err, jc.ErrorIsNil)

	return hex.EncodeToString(hash.Sum(nil))
}
