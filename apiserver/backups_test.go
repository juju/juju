// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/juju/tc"

	corebackups "github.com/juju/juju/core/backups"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/rpc/params"
)

type backupSuite struct {
	backupDir string
	handler   *backupHandler

	// createArchiveStub scripts the archive creation; cleanupRan
	// records that the returned cleanup ran.
	createErr   error
	cleanupRan  bool
	archiveData string
}

func TestBackupSuite(t *testing.T) {
	tc.Run(t, &backupSuite{})
}

func (s *backupSuite) SetUpTest(c *tc.C) {
	s.backupDir = c.MkDir()
	s.cleanupRan = false
	s.createErr = nil
	s.archiveData = "archive data"
	s.handler = &backupHandler{
		createArchive: s.createArchive,
		logger:        loggertesting.WrapCheckLog(c),
	}
}

// createArchive stubs archive creation: it writes a real archive into
// a temporary directory under the backup dir and returns a cleanup
// that removes it, mirroring the production creator.
func (s *backupSuite) createArchive(ctx context.Context, notes string) (*corebackups.Metadata, string, func(), error) {
	if s.createErr != nil {
		return nil, "", nil, s.createErr
	}
	tmpDir, err := os.MkdirTemp(s.backupDir, "create-")
	if err != nil {
		return nil, "", nil, err
	}
	archivePath := filepath.Join(tmpDir, "juju-backup-test.tar.gz")
	if err := os.WriteFile(archivePath, []byte(s.archiveData), 0600); err != nil {
		return nil, "", nil, err
	}
	meta := corebackups.NewMetadata(time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC))
	meta.Notes = notes
	cleanup := func() {
		s.cleanupRan = true
		_ = os.RemoveAll(tmpDir)
	}
	return meta, archivePath, cleanup, nil
}

// postRequest serves a create request with the given notes and returns
// the recorded response.
func (s *backupSuite) postRequest(c *tc.C, notes string, header http.Header) *httptest.ResponseRecorder {
	body, err := json.Marshal(params.BackupsCreateArgs{Notes: notes})
	c.Assert(err, tc.ErrorIsNil)

	req := httptest.NewRequest(http.MethodPost, "/backup", strings.NewReader(string(body)))
	maps.Copy(req.Header, header)
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

// errorMessage extracts the message of the JSON error response.
func (s *backupSuite) errorMessage(c *tc.C, rec *httptest.ResponseRecorder) string {
	var result params.ErrorResult
	err := json.Unmarshal(rec.Body.Bytes(), &result)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.NotNil)
	return result.Error.Message
}

func (s *backupSuite) TestCreateAndServe(c *tc.C) {
	rec := s.postRequest(c, "my notes", nil)

	c.Check(rec.Code, tc.Equals, http.StatusOK)
	c.Check(rec.Header().Get("Content-Type"), tc.Equals, params.ContentTypeRaw)
	c.Check(rec.Header().Get("Content-Length"), tc.Equals, "12")
	c.Check(rec.Body.String(), tc.Equals, "archive data")

	// The metadata travels in the response header.
	var result params.BackupsMetadataResult
	err := json.Unmarshal([]byte(rec.Header().Get(params.BackupMetadataHeader)), &result)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Notes, tc.Equals, "my notes")
	c.Check(result.Filename, tc.Equals, "juju-backup-test.tar.gz")
	c.Check(result.Started, tc.Equals, time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC))

	// The archive is removed once the request ends.
	c.Check(s.cleanupRan, tc.IsTrue)
	entries, err := os.ReadDir(s.backupDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entries, tc.HasLen, 0)
}

func (s *backupSuite) TestCreateFailure(c *tc.C) {
	s.createErr = errors.New("cannot export controller")

	rec := s.postRequest(c, "", nil)

	c.Check(rec.Code, tc.Equals, http.StatusInternalServerError)
	c.Check(s.errorMessage(c, rec), tc.Matches, "cannot export controller")
}

func (s *backupSuite) TestCreateBadJSON(c *tc.C) {
	req := httptest.NewRequest(http.MethodPost, "/backup", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)

	c.Check(rec.Code, tc.Equals, http.StatusBadRequest)
	c.Check(s.errorMessage(c, rec), tc.Matches, "reading request body.*")
}

// TestGetRejected verifies that pre-4.1 clients, which download created
// backups with a GET, get a clear upgrade error.
func (s *backupSuite) TestGetRejected(c *tc.C) {
	body, err := json.Marshal(params.BackupsDownloadArgs{ID: "juju-backup-20260101-000000.tar.gz"})
	c.Assert(err, tc.ErrorIsNil)
	req := httptest.NewRequest(http.MethodGet, "/backups", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)

	c.Check(s.errorMessage(c, rec), tc.Matches,
		"downloading backups from clients older than 4.1 is not supported.*")
}

func (s *backupSuite) TestMethodNotAllowed(c *tc.C) {
	req := httptest.NewRequest(http.MethodPut, "/backups", nil)
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)

	c.Check(rec.Code, tc.Equals, http.StatusMethodNotAllowed)
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

// TestInterruptedTransferRemovesArchive verifies that a transfer that
// breaks part way through still removes the archive: nothing is left
// on the controller, however the request ends.
func (s *backupSuite) TestInterruptedTransferRemovesArchive(c *tc.C) {
	body, err := json.Marshal(params.BackupsCreateArgs{})
	c.Assert(err, tc.ErrorIsNil)
	req := httptest.NewRequest(http.MethodPost, "/backup", strings.NewReader(string(body)))
	w := &failingWriter{failAt: 0}
	s.handler.ServeHTTP(w, req)

	c.Check(s.cleanupRan, tc.IsTrue)
	entries, err := os.ReadDir(s.backupDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entries, tc.HasLen, 0)
}
