// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"crypto/sha1"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"

	corebackups "github.com/juju/juju/core/backups"
	domainservicetesting "github.com/juju/juju/domain/services/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/rpc/params"
)

type backupsDownloadSuite struct {
	domainservicetesting.DomainServicesSuite
}

func TestBackupsDownloadSuite(t *stdtesting.T) {
	tc.Run(t, &backupsDownloadSuite{})
}

func (s *backupsDownloadSuite) handler(c *tc.C) *backupsDownloadHandler {
	return &backupsDownloadHandler{
		domainServicesGetter: s.ModelDomainServicesGetter(c),
		controllerModelUUID:  s.ControllerModelUUID,
		logger:               loggertesting.WrapCheckLog(c),
	}
}

// createArchive makes a real archive inside the controller model's
// backup-dir and returns its absolute path. The path is set as the model
// config backup-dir so the handler resolves it.
func (s *backupsDownloadSuite) createArchive(c *tc.C) (dir, filename string) {
	controllerServices := s.ControllerDomainServices(c)

	dir = c.MkDir()
	err := controllerServices.Config().UpdateModelConfig(
		c.Context(), map[string]any{"backup-dir": dir}, nil)
	c.Assert(err, tc.ErrorIsNil)

	payload := path.Join(dir, "blob")
	c.Assert(os.WriteFile(payload, []byte("blob content"), 0644), tc.ErrorIsNil)

	filename, err = corebackups.Create(corebackups.NewMetadata(time.Now()), corebackups.CreateArgs{
		DestinationDir: dir,
		FilesToBackUp:  []string{payload},
		Clock:          clock.WallClock,
	})
	c.Assert(err, tc.ErrorIsNil)
	return dir, filename
}

func (s *backupsDownloadSuite) get(c *tc.C, id string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(
		http.MethodGet, "/model/x/backups", strings.NewReader(`{"id":"`+id+`"}`))
	recorder := httptest.NewRecorder()
	s.handler(c).ServeHTTP(recorder, req)
	return recorder
}

// TestDownload serves the archive and removes it from disk afterwards.
func (s *backupsDownloadSuite) TestDownload(c *tc.C) {
	_, filename := s.createArchive(c)

	expected, err := os.ReadFile(filename)
	c.Assert(err, tc.ErrorIsNil)

	recorder := s.get(c, filename)
	c.Assert(recorder.Code, tc.Equals, http.StatusOK)

	// The raw content type and digest headers match the 3.6 download
	// response.
	c.Check(recorder.Header().Get("Content-Type"), tc.Equals, params.ContentTypeRaw)
	sum := sha1.Sum(expected)
	c.Check(recorder.Header().Get("Digest"), tc.Equals,
		params.EncodeChecksum(base64.StdEncoding.EncodeToString(sum[:])))

	// The archive bytes match the file contents read before the download.
	c.Check(recorder.Body.Len(), tc.Equals, len(expected))
	c.Check(recorder.Body.String(), tc.Equals, string(expected))

	// One-shot semantics: the archive is gone after serving.
	_, err = os.Stat(filename)
	c.Assert(os.IsNotExist(err), tc.IsTrue)
}

// TestDownloadInvalidID rejects ids that do not resolve to a backup
// archive under the backup dir.
func (s *backupsDownloadSuite) TestDownloadInvalidID(c *tc.C) {
	_, filename := s.createArchive(c)

	for _, id := range []string{
		"../any.tar.gz",
		"not-an-archive.tar.gz",
		path.Join("/missing", "juju-backup-empty.tar.gz"),
	} {
		recorder := s.get(c, id)
		c.Check(recorder.Code, tc.Equals, http.StatusBadRequest,
			tc.Commentf("id %q", id))
	}

	// A reject leaves a created archive untouched.
	_, err := os.Stat(filename)
	c.Assert(err, tc.ErrorIsNil)
}

// TestDownloadBadBody rejects non-JSON bodies.
func (s *backupsDownloadSuite) TestDownloadBadBody(c *tc.C) {
	req := httptest.NewRequest(
		http.MethodGet, "/model/x/backups", strings.NewReader(`not json`))
	recorder := httptest.NewRecorder()

	s.handler(c).ServeHTTP(recorder, req)
	c.Check(recorder.Code, tc.Equals, http.StatusBadRequest)
}
