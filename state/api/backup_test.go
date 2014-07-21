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
		if s.digest != "" {
			resp.Header = http.Header{}
			resp.Header.Set("digest", "SHA="+s.digest)
		}
		return &resp, nil
	}
	s.PatchValue(api.SendHTTPRequest, sender)

	s.client = s.APIState.Client()
	s.filename = filepath.Join(c.MkDir(), "backup.tar.gz")
}

func (s *backupSuite) setFailure(c *gc.C, msg string) {
	result := params.BackupResponse{Error: msg}
	var err error
	s.data, err = json.Marshal(result)
	c.Assert(err, gc.IsNil)
	s.statusCode = http.StatusInternalServerError
	s.digest = ""
	s.err = nil
}

func (s *backupSuite) setData(c *gc.C, data string) {
	s.data = []byte(data)
	s.statusCode = http.StatusOK
	hasher := sha1.New()
	hasher.Write(s.data)
	s.digest = fmt.Sprintf("%x", hasher.Sum(nil))
	s.err = nil
}

//---------------------------
// tests

func (s *backupSuite) TestBackupExplicitFilename(c *gc.C) {
	s.setData(c, "<compressed backup data>")
	filename, hash, expected, err := s.client.Backup(s.filename, false)

	c.Check(err, gc.IsNil)
	c.Check(filename, gc.Equals, s.filename)
	c.Check(hash, gc.Equals, "cfbcc716a37b2507ff1201bdab8fea98fef64c4f")
	c.Check(expected, gc.Equals, hash)
	backup.CheckArchive(c, filename, hash, s.data, nil)
}

func (s *backupSuite) TestBackupDefaultFilename(c *gc.C) {
	s.setData(c, "<compressed backup data>")
	filename, hash, expected, err := s.client.Backup("", false)
	defer os.Remove(filename)

	c.Check(err, gc.IsNil)
	c.Check(filepath.Base(filename), gc.Matches, `jujubackup-\d+-\d+.tar.gz`)
	c.Check(hash, gc.Equals, "cfbcc716a37b2507ff1201bdab8fea98fef64c4f")
	c.Check(expected, gc.Equals, hash)
	backup.CheckArchive(c, filename, hash, s.data, nil)
}

func (s *backupSuite) TestBackupFailureCreatingFile(c *gc.C) {
	create := func(filename string, excl bool) (*os.File, string, error) {
		return nil, "", fmt.Errorf("failed!")
	}
	s.PatchValue(api.CreateEmptyFile, create)
	_, _, _, err := s.client.Backup(s.filename, false)

	c.Check(err, gc.ErrorMatches, "error while preparing backup file")
	c.Check(params.ErrCode(err), gc.Equals, "")
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
	_, _, _, err := s.client.Backup(s.filename, false)

	c.Check(err, gc.ErrorMatches, "error while preparing backup request")
	c.Check(data.baseurl, gc.Matches, `https://localhost:\d+`)
	c.Check(data.uuid, gc.Matches, `\w+-\w+-\w+-\w+-\w+`)
	c.Check(data.tag, gc.Equals, "user-admin")
	c.Check(data.pw, gc.Equals, "dummy-secret")
}

func (s *backupSuite) TestBackupFailureSendingRequest(c *gc.C) {
	s.err = fmt.Errorf("failed!")
	_, _, _, err := s.client.Backup(s.filename, false)

	c.Check(err, gc.ErrorMatches, "failure sending backup request")
}

func (s *backupSuite) TestBackupRequestFailed(c *gc.C) {
	// We don't need to patch api.CheckAPIResponse here, so we don't.
	s.setFailure(c, "failed!")
	_, _, _, err := s.client.Backup(s.filename, false)

	c.Check(err, gc.ErrorMatches, "backup request failed on server")
}

func (s *backupSuite) TestBackupFailureWritingArchive(c *gc.C) {
	s.setData(c, "<compressed backup data>")
	write := func(archive io.Writer, infile io.Reader) (string, error) {
		return "", fmt.Errorf("failed!")
	}
	s.PatchValue(api.WriteBackup, write)
	_, _, _, err := s.client.Backup(s.filename, false)

	c.Check(err, gc.ErrorMatches, "could not save the backup")
}

func (s *backupSuite) TestBackupFailureParsingDigest(c *gc.C) {
	s.setData(c, "<compressed backup data>")
	parse := func(header http.Header) (string, error) {
		return "", fmt.Errorf("failed!")
	}
	s.PatchValue(api.ParseDigest, parse)
	filename, hash, expected, err := s.client.Backup(s.filename, false)

	c.Check(err, gc.IsNil)
	c.Check(filename, gc.Equals, s.filename)
	c.Check(hash, gc.Equals, "cfbcc716a37b2507ff1201bdab8fea98fef64c4f")
	c.Check(expected, gc.Equals, "")
	backup.CheckArchive(c, filename, hash, s.data, nil)
}

func (s *backupSuite) TestBackupFailureHandlingFilename(c *gc.C) {
	s.setData(c, "<compressed backup data>")
	extract := func(header http.Header) (string, error) {
		return "", fmt.Errorf("failed!")
	}
	s.PatchValue(api.ExtractFilename, extract)
	filename, hash, expected, err := s.client.Backup("", false)
	defer os.Remove(filename)

	c.Check(err, gc.IsNil)
	c.Check(filepath.Base(filename), gc.Matches, `jujubackup-\d+-\d+.tar.gz`)
	c.Check(hash, gc.Equals, "cfbcc716a37b2507ff1201bdab8fea98fef64c4f")
	c.Check(expected, gc.Equals, hash)
	backup.CheckArchive(c, filename, hash, s.data, nil)
}

func (s *backupSuite) TestBackupNoFilenameHeader(c *gc.C) {
	s.setData(c, "<compressed backup data>")
	extract := func(header http.Header) (string, error) {
		return "", nil
	}
	s.PatchValue(api.ExtractFilename, extract)
	filename, hash, expected, err := s.client.Backup("", false)
	defer os.Remove(filename)

	c.Check(err, gc.IsNil)
	c.Check(filepath.Base(filename), gc.Matches, `jujubackup-\d+-\d+.tar.gz`)
	c.Check(hash, gc.Equals, "cfbcc716a37b2507ff1201bdab8fea98fef64c4f")
	c.Check(expected, gc.Equals, hash)
	backup.CheckArchive(c, filename, hash, s.data, nil)
}
