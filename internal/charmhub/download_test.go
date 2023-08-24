// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testcharms/repo"
)

const defaultSeries = "bionic"
const localCharmRepo = "../testcharms/charm-repo"

type DownloadSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&DownloadSuite{})

func (s *DownloadSuite) TestDownloadAndRead(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tmpFile, err := os.CreateTemp("", "charm")
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		err := os.Remove(tmpFile.Name())
		c.Assert(err, jc.ErrorIsNil)
	}()

	fileSystem := NewMockFileSystem(ctrl)
	fileSystem.EXPECT().Create(tmpFile.Name()).Return(tmpFile, nil)

	httpClient := NewMockHTTPClient(ctrl)
	httpClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(r *http.Request) (*http.Response, error) {
		archiveBytes := s.createCharmArchieve(c)

		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBuffer(archiveBytes)),
		}, nil
	})

	serverURL, err := url.Parse("http://meshuggah.rocks")
	c.Assert(err, jc.ErrorIsNil)

	client := newDownloadClient(httpClient, fileSystem, &FakeLogger{})
	_, err = client.DownloadAndRead(context.Background(), serverURL, tmpFile.Name())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DownloadSuite) TestDownloadAndReadWithNotFoundStatusCode(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tmpFile, err := os.CreateTemp("", "charm")
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		err := os.Remove(tmpFile.Name())
		c.Assert(err, jc.ErrorIsNil)
	}()

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

	client := newDownloadClient(httpClient, fileSystem, &FakeLogger{})
	_, err = client.DownloadAndRead(context.Background(), serverURL, tmpFile.Name())
	c.Assert(err, gc.ErrorMatches, `cannot retrieve "http://meshuggah.rocks": archive not found`)
}

func (s *DownloadSuite) TestDownloadAndReadWithFailedStatusCode(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tmpFile, err := os.CreateTemp("", "charm")
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		err := os.Remove(tmpFile.Name())
		c.Assert(err, jc.ErrorIsNil)
	}()

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

	client := newDownloadClient(httpClient, fileSystem, &FakeLogger{})
	_, err = client.DownloadAndRead(context.Background(), serverURL, tmpFile.Name())
	c.Assert(err, gc.ErrorMatches, `cannot retrieve "http://meshuggah.rocks": unable to locate archive \(store API responded with status: Internal Server Error\)`)
}

func (s *DownloadSuite) createCharmArchieve(c *gc.C) []byte {
	tmpDir, err := os.MkdirTemp("", "charm")
	c.Assert(err, jc.ErrorIsNil)

	repo := repo.NewRepo(localCharmRepo, defaultSeries)
	charmPath := repo.CharmArchivePath(tmpDir, "dummy")

	path, err := os.ReadFile(charmPath)
	c.Assert(err, jc.ErrorIsNil)
	return path
}
