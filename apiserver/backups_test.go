// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	corebackups "github.com/juju/juju/core/backups"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

type backupsDownloadSuite struct {
	backupDir string
	handler   http.Handler
}

func TestBackupsDownloadSuite(t *testing.T) {
	tc.Run(t, &backupsDownloadSuite{})
}

func (s *backupsDownloadSuite) SetUpTest(c *tc.C) {
	s.backupDir = c.MkDir()
	s.handler = &backupsDownloadHandler{
		resolveBackupDir: func(context.Context) (string, error) {
			return s.backupDir, nil
		},
		logger: loggertesting.WrapCheckLog(c),
	}
}

// stageArchive stages an archive for one-shot download under a fresh id
// and returns the id.
func (s *backupsDownloadSuite) stageArchive(c *tc.C, content string) string {
	id, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	path, err := corebackups.OneShotArchivePath(s.backupDir, id.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(os.MkdirAll(filepath.Dir(path), 0755), tc.ErrorIsNil)
	c.Assert(os.WriteFile(path, []byte(content), 0600), tc.ErrorIsNil)
	return id.String()
}

// downloadRequest serves a download request for the given id and returns
// the recorded response.
func (s *backupsDownloadSuite) downloadRequest(c *tc.C, id string, header http.Header) *httptest.ResponseRecorder {
	body, err := json.Marshal(params.BackupsDownloadArgs{ID: id})
	c.Assert(err, tc.ErrorIsNil)

	req := httptest.NewRequest(http.MethodGet, "/backups", strings.NewReader(string(body)))
	maps.Copy(req.Header, header)
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

// errorMessage extracts the message of the JSON error response.
func (s *backupsDownloadSuite) errorMessage(c *tc.C, rec *httptest.ResponseRecorder) string {
	var result params.ErrorResult
	err := json.Unmarshal(rec.Body.Bytes(), &result)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.NotNil)
	return result.Error.Message
}

func (s *backupsDownloadSuite) TestDownload(c *tc.C) {
	id := s.stageArchive(c, "archive data")

	rec := s.downloadRequest(c, id, nil)

	c.Check(rec.Code, tc.Equals, http.StatusOK)
	c.Check(rec.Header().Get("Content-Type"), tc.Equals, params.ContentTypeRaw)
	c.Check(rec.Header().Get("Content-Length"), tc.Equals, "12")
	c.Check(rec.Body.String(), tc.Equals, "archive data")

	// One-shot: the archive is removed once it has been fully served.
	path, err := corebackups.OneShotArchivePath(s.backupDir, id)
	c.Assert(err, tc.ErrorIsNil)
	_, err = os.Stat(path)
	c.Assert(err, tc.Satisfies, os.IsNotExist)
}

func (s *backupsDownloadSuite) TestDownloadAlreadyDownloaded(c *tc.C) {
	id := s.stageArchive(c, "archive data")

	rec := s.downloadRequest(c, id, nil)
	c.Check(rec.Code, tc.Equals, http.StatusOK)

	// A second download of the same id finds nothing.
	rec = s.downloadRequest(c, id, nil)
	c.Check(rec.Code, tc.Equals, http.StatusNotFound)
	c.Check(s.errorMessage(c, rec), tc.Matches, `backup ".*" not found`)
}

func (s *backupsDownloadSuite) TestDownloadUnknownID(c *tc.C) {
	id, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	rec := s.downloadRequest(c, id.String(), nil)

	c.Check(rec.Code, tc.Equals, http.StatusNotFound)
	c.Check(s.errorMessage(c, rec), tc.Matches, `backup ".*" not found`)
}

func (s *backupsDownloadSuite) TestDownloadInvalidID(c *tc.C) {
	for _, id := range []string{"", "not-a-uuid", "../../etc/passwd", "0-0-0-0-0"} {
		c.Logf("id %q", id)
		rec := s.downloadRequest(c, id, nil)

		c.Check(rec.Code, tc.Equals, http.StatusBadRequest)
		c.Check(s.errorMessage(c, rec), tc.Matches, "invalid backup id.*")
	}
}

func (s *backupsDownloadSuite) TestDownloadRangeRejected(c *tc.C) {
	id := s.stageArchive(c, "archive data")

	rec := s.downloadRequest(c, id, http.Header{"Range": []string{"bytes=0-4"}})

	c.Check(rec.Code, tc.Equals, http.StatusBadRequest)
	c.Check(s.errorMessage(c, rec), tc.Matches, "range requests are not supported.*")

	// A rejected request leaves the archive staged.
	path, err := corebackups.OneShotArchivePath(s.backupDir, id)
	c.Assert(err, tc.ErrorIsNil)
	_, err = os.Stat(path)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *backupsDownloadSuite) TestDownloadBadJSON(c *tc.C) {
	req := httptest.NewRequest(http.MethodGet, "/backups", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)

	c.Check(rec.Code, tc.Equals, http.StatusBadRequest)
	c.Check(s.errorMessage(c, rec), tc.Matches, "reading request body.*")
}

func (s *backupsDownloadSuite) TestDownloadBackupDirResolutionFailure(c *tc.C) {
	handler := &backupsDownloadHandler{
		resolveBackupDir: func(context.Context) (string, error) {
			return "", errors.New("boom")
		},
		logger: loggertesting.WrapCheckLog(c),
	}
	id := s.stageArchive(c, "archive data")

	body, err := json.Marshal(params.BackupsDownloadArgs{ID: id})
	c.Assert(err, tc.ErrorIsNil)
	req := httptest.NewRequest(http.MethodGet, "/backups", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	c.Check(rec.Code, tc.Equals, http.StatusInternalServerError)
	c.Check(s.errorMessage(c, rec), tc.Matches, "boom")
}

// failingWriter records the response headers and then fails after a
// limited number of bytes have been written.
type failingWriter struct {
	header http.Header
	writes int
	failAt int
}

func (w *failingWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}

func (w *failingWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes > w.failAt {
		return 0, errors.New("client went away")
	}
	return len(p), nil
}

func (w *failingWriter) WriteHeader(int) {}

func (s *backupsDownloadSuite) TestDownloadPartialTransferKeepsArchive(c *tc.C) {
	id := s.stageArchive(c, "archive data")

	path, err := corebackups.OneShotArchivePath(s.backupDir, id)
	c.Assert(err, tc.ErrorIsNil)
	content, err := os.ReadFile(path)
	c.Assert(err, tc.ErrorIsNil)

	body, err := json.Marshal(params.BackupsDownloadArgs{ID: id})
	c.Assert(err, tc.ErrorIsNil)
	req := httptest.NewRequest(http.MethodGet, "/backups", strings.NewReader(string(body)))
	w := &failingWriter{failAt: 0}
	s.handler.ServeHTTP(w, req)

	// The transfer failed part way, so the archive is left staged for a
	// retry within the retention window.
	staged, err := os.ReadFile(path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(staged, tc.DeepEquals, content)
}
