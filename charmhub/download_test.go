// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"net/url"
	os "os"

	gomock "github.com/golang/mock/gomock"
	charmrepotesting "github.com/juju/charmrepo/v6/testing"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
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

	tmpFile, err := ioutil.TempFile("", "charm")
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		err := os.Remove(tmpFile.Name())
		c.Assert(err, jc.ErrorIsNil)
	}()

	fileSystem := NewMockFileSystem(ctrl)
	fileSystem.EXPECT().Create(tmpFile.Name()).Return(tmpFile, nil)

	transport := NewMockTransport(ctrl)
	transport.EXPECT().Do(gomock.Any()).DoAndReturn(func(r *http.Request) (*http.Response, error) {
		archiveBytes := s.createCharmArchieve(c)

		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBuffer(archiveBytes)),
		}, nil
	})

	serverURL, err := url.Parse("http://meshuggah.rocks")
	c.Assert(err, jc.ErrorIsNil)

	client := NewDownloadClient(transport, fileSystem, &FakeLogger{})
	_, err = client.DownloadAndRead(context.TODO(), serverURL, tmpFile.Name())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DownloadSuite) TestDownloadAndReadWithNotFoundStatusCode(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tmpFile, err := ioutil.TempFile("", "charm")
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		err := os.Remove(tmpFile.Name())
		c.Assert(err, jc.ErrorIsNil)
	}()

	fileSystem := NewMockFileSystem(ctrl)
	fileSystem.EXPECT().Create(tmpFile.Name()).Return(tmpFile, nil)

	transport := NewMockTransport(ctrl)
	transport.EXPECT().Do(gomock.Any()).DoAndReturn(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 404,
			Body:       ioutil.NopCloser(bytes.NewBufferString("")),
		}, nil
	})

	serverURL, err := url.Parse("http://meshuggah.rocks")
	c.Assert(err, jc.ErrorIsNil)

	client := NewDownloadClient(transport, fileSystem, &FakeLogger{})
	_, err = client.DownloadAndRead(context.TODO(), serverURL, tmpFile.Name())
	c.Assert(err, gc.ErrorMatches, `cannot retrieve "http://meshuggah.rocks": archive not found`)
}

func (s *DownloadSuite) TestDownloadAndReadWithFailedStatusCode(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tmpFile, err := ioutil.TempFile("", "charm")
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		err := os.Remove(tmpFile.Name())
		c.Assert(err, jc.ErrorIsNil)
	}()

	fileSystem := NewMockFileSystem(ctrl)
	fileSystem.EXPECT().Create(tmpFile.Name()).Return(tmpFile, nil)

	transport := NewMockTransport(ctrl)
	transport.EXPECT().Do(gomock.Any()).DoAndReturn(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			Status:     http.StatusText(http.StatusInternalServerError),
			StatusCode: http.StatusInternalServerError,
			Body:       ioutil.NopCloser(bytes.NewBufferString("")),
		}, nil
	})

	serverURL, err := url.Parse("http://meshuggah.rocks")
	c.Assert(err, jc.ErrorIsNil)

	client := NewDownloadClient(transport, fileSystem, &FakeLogger{})
	_, err = client.DownloadAndRead(context.TODO(), serverURL, tmpFile.Name())
	c.Assert(err, gc.ErrorMatches, `cannot retrieve "http://meshuggah.rocks": unable to locate archive \(store API responded with status: Internal Server Error\)`)
}

func (s *DownloadSuite) createCharmArchieve(c *gc.C) []byte {
	tmpDir, err := ioutil.TempDir("", "charm")
	c.Assert(err, jc.ErrorIsNil)

	repo := charmrepotesting.NewRepo(localCharmRepo, defaultSeries)
	charmPath := repo.CharmArchivePath(tmpDir, "dummy")

	path, err := ioutil.ReadFile(charmPath)
	c.Assert(err, jc.ErrorIsNil)
	return path
}
