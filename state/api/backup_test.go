// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/api"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/backup"
)

var _ = gc.Suite(&backupSuite{})

type backupSuite struct {
	clientSuite
	client   *api.Client
	filename string

	statusCode int
	data       []byte
	digest     string
	err        error
}

func (s *backupSuite) SetUpTest(c *gc.C) {
	s.clientSuite.SetUpTest(c)

	sender := func(req *http.Request, _ *http.Client) (*http.Response, error) {
		if s.err != nil {
			return nil, s.err
		}

		statusCode := s.statusCode
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		resp := http.Response{
			StatusCode: statusCode,
			Body:       ioutil.NopCloser(bytes.NewBuffer(s.data)),
		}
		resp.Header = http.Header{}
		if s.digest != "" {
			resp.Header.Set("digest", "SHA="+s.digest)
		}
		if s.filename != "" {
			resp.Header.Set("Content-Disposition",
				fmt.Sprintf(`attachment; filename="%s"`, s.filename))
		}
		return &resp, nil
	}
	s.PatchValue(api.SendHTTPRequest, sender)

	s.client = s.APIState.Client()
}

func (s *backupSuite) setFailure(c *gc.C, msg string) {
	result := params.BackupResponse{Error: msg}
	var err error
	s.data, err = json.Marshal(result)
	c.Assert(err, gc.IsNil)
	s.statusCode = http.StatusInternalServerError
	s.digest = ""
	s.filename = ""
	s.err = nil
}

func (s *backupSuite) setData(c *gc.C, data string) {
	s.data = []byte(data)
	s.statusCode = http.StatusOK
	hasher := sha1.New()
	hasher.Write(s.data)
	s.digest = fmt.Sprintf("%x", hasher.Sum(nil))
	s.filename = fmt.Sprintf(backup.FilenameTemplate, "20140101-000000")
	s.err = nil
}

//---------------------------
// tests

func (s *backupSuite) TestBackupSuccess(c *gc.C) {
	s.setData(c, "<compressed backup data>")
	archive, err := os.Create(filepath.Join(c.MkDir(), "backup.tar.gz"))
	c.Assert(err, gc.IsNil)
	res := s.client.Backup(archive)

	c.Check(res.Failure(), gc.IsNil)
	c.Check(res.FilenameFromServer(), gc.Matches, `jujubackup-\d+-\d+.tar.gz`)
	hash := res.WrittenHash()
	c.Check(hash, gc.Equals, "cfbcc716a37b2507ff1201bdab8fea98fef64c4f")
	c.Check(res.HashFromServer(), gc.Equals, hash)
	backup.CheckArchive(c, archive.Name(), hash, s.data, nil)
}

func (s *backupSuite) TestBackupFailureCreatingRequest(c *gc.C) {
	data := struct {
		baseurl string
		uuid    string
		tag     string
		pw      string
	}{}
	newreq := func(URL *url.URL, uuid, tag, pw string) (*http.Request, error) {
		data.baseurl = URL.String()
		data.uuid = uuid
		data.tag = tag
		data.pw = pw
		return nil, fmt.Errorf("failed!")
	}
	s.PatchValue(api.NewHTTPRequest, newreq)
	res := s.client.Backup(nil)

	err := res.Failure()
	c.Check(err, gc.ErrorMatches, "error while preparing backup request")
	c.Check(data.baseurl, gc.Matches, `https://localhost:\d+`)
	c.Check(data.uuid, gc.Matches, `\w+-\w+-\w+-\w+-\w+`)
	c.Check(data.tag, gc.Equals, "user-admin")
	c.Check(data.pw, gc.Equals, "dummy-secret")
}

func (s *backupSuite) TestBackupFailureSendingRequest(c *gc.C) {
	s.err = fmt.Errorf("failed!")
	res := s.client.Backup(nil)

	c.Check(res.Failure(), gc.ErrorMatches, "failure sending backup request")
}

func (s *backupSuite) TestBackupRequestFailed(c *gc.C) {
	// We don't need to patch api.CheckAPIResponse here, so we don't.
	s.setFailure(c, "failed!")
	res := s.client.Backup(nil)

	c.Check(res.Failure(), gc.ErrorMatches, "backup request failed on server")
}

func (s *backupSuite) TestBackupFailureWritingArchive(c *gc.C) {
	s.setData(c, "<compressed backup data>")
	write := func(archive io.Writer, infile io.Reader) (string, error) {
		return "", fmt.Errorf("failed!")
	}
	s.PatchValue(api.WriteBackup, write)
	archive, err := os.Create(filepath.Join(c.MkDir(), "backup.tar.gz"))
	c.Assert(err, gc.IsNil)
	res := s.client.Backup(archive)

	c.Check(res.Failure(), gc.ErrorMatches, "could not save the backup")
}

func (s *backupSuite) TestBackupFailureParsingDigest(c *gc.C) {
	s.setData(c, "<compressed backup data>")
	parse := func(header *http.Header) (string, error) {
		return "", fmt.Errorf("failed!")
	}
	s.PatchValue(api.ExtractDigest, parse)
	archive, err := os.Create(filepath.Join(c.MkDir(), "backup.tar.gz"))
	c.Assert(err, gc.IsNil)
	res := s.client.Backup(archive)

	c.Check(res.Failure(), gc.IsNil)
	c.Check(res.FilenameFromServer(), gc.Matches, `jujubackup-\d+-\d+.tar.gz`)
	hash := res.WrittenHash()
	c.Check(hash, gc.Equals, "cfbcc716a37b2507ff1201bdab8fea98fef64c4f")
	c.Check(res.HashFromServer(), gc.Equals, "")
	backup.CheckArchive(c, archive.Name(), hash, s.data, nil)
}

func (s *backupSuite) TestBackupFailureHandlingFilename(c *gc.C) {
	s.setData(c, "<compressed backup data>")
	extract := func(header *http.Header) (string, error) {
		return "", fmt.Errorf("failed!")
	}
	s.PatchValue(api.ExtractFilename, extract)
	archive, err := os.Create(filepath.Join(c.MkDir(), "backup.tar.gz"))
	c.Assert(err, gc.IsNil)
	res := s.client.Backup(archive)

	c.Check(res.Failure(), gc.IsNil)
	c.Check(res.FilenameFromServer(), gc.Equals, "")
	hash := res.WrittenHash()
	c.Check(hash, gc.Equals, "cfbcc716a37b2507ff1201bdab8fea98fef64c4f")
	c.Check(res.HashFromServer(), gc.Equals, hash)
	backup.CheckArchive(c, archive.Name(), hash, s.data, nil)
}
