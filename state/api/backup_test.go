// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"fmt"
	//	"io/ioutil"
	"net/http"
	"os"
	//	"path/filepath"

	gc "launchpad.net/gocheck"

	//	simpleTesting "github.com/juju/juju/rpc/simple/testing"
	"github.com/juju/juju/state/api"
	"github.com/juju/juju/state/apiserver"
	"github.com/juju/juju/state/backup"
	"github.com/juju/juju/testing"
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
			Body:       &ioutil.NopCloser{bytes.NewBuffer(s.data)},
		}
		if s.digest != "" {
			resp.Header.Set("digest", "SHA="+s.digest)
		}
		return resp, nil
	}
	s.PatchValue(api.SendHTTPRequest, sender)

	s.client = s.APIState.Client()
	s.filename = filepath.Join(c.MkDir(), "backup.tar.gz")
}

func (s *backupSuite) setFailed(c *gc.C, msg string) {
	result := params.BackupResponse{Error: msg}
	s.data, err = json.Marshal(result)
	c.Assert(err, gc.IsNil)
	s.statusCode = http.StatusInternalServerError
	s.digest = ""
	s.err = nil
}

func (s *backupSuite) setData(c *gc.C, data string) {
	s.data = []byte(data)
	c.Assert(err, gc.IsNil)
	s.statusCode = http.StatusOK
	//    s.digest = ...
	s.err = nil
}

func (s *backupSuite) checkArchive(c *gc.C, filename, hash string) {
	// 1) the filename is created on disk
	// 2) The content of the filename is not nil
	// 3) It is a valid tarball
	// 4) The hash matches expectations (though presumably that's already covered by other code)
	// 5) we could assert that some of the filenames in the tarball match what we expect to be in a backup.
	s.Error("not finished")
}

//---------------------------
// tests

func (s *backupSuite) TestBackup(c *gc.C) {
	s.setData(c, "<compressed backup data>")
	filename, hash, expected, err := s.client.Backup(self.filename, false)

	c.Check(err, gc.IsNil)
	c.Check(filename, gc.Equals, self.filename)
	c.Check(hash, gc.Equals, "")
	c.Check(expected, gc.Equals, "")
	s.checkArchive(c, filename, hash)
}

func (s *backupSuite) TestBackupDefaultFilename(c *gc.C) {
	s.setData(c, "<compressed backup data>")
	filename, hash, expected, err := s.client.Backup("", false)
	defer os.Remove(filename)

	c.Check(err, gc.IsNil)
	c.Check(filename, gc.Equals, self.filename)
	c.Check(hash, gc.Equals, "")
	c.Check(expected, gc.Equals, "")
	s.checkArchive(c, filename, hash)
}

func (s *backupSuite) TestBackupFailureCreatingFile(c *gc.C) {
	create := func(filename string) (*os.File, string, error) {
		return nil, "", fmt.Errorf("failed!")
	}
	s.PatchValue(api.CreateEmptyFile, create)
	_, _, _, err := s.client.Backup(self.filename, false)

	c.Check(err, gc.ErrorMatches, "error while preparing backup file")
	c.Check(params.ErrCode(err), gc.Equals, "")
}

func (s *backupSuite) TestBackupFailureCreatingRequest(c *gc.C) {
	data := struct {
		URL  *url.URL
		uuid string
		tag  string
		pw   string
	}{}
	newreq := func(URL *url.URL, uuid, tag, pw string) (*http.Request, error) {
		data.URL = URL
		data.uuid = uuid
		data.tag = tag
		data.pw = pw
		return nil, fmt.Errorf("failed!")
	}
	s.PatchValue(api.NewAPIRequest, newreq)
	_, _, _, err := s.client.Backup(self.filename, false)

	c.Check(err, gc.ErrorMatches, "error while preparing backup request")
	c.Check(data.URL.String(), gc.Equals, "")
	c.Check(data.uuid, gc.Equals, "")
	c.Check(data.uuidtag, gc.Equals, "")
	c.Check(data.uuidpw, gc.Equals, "")
}

func (s *backupSuite) TestBackupFailureSendingRequest(c *gc.C) {
	s.err = fmt.Errorf("failed!")
	_, _, _, err := s.client.Backup(self.filename, false)

	c.Check(err, gc.ErrorMatches, "failure sending backup request")
}

func (s *backupSuite) TestBackupRequestFailed(c *gc.C) {
	// We don't need to patch api.CheckAPIResponse here, so we don't.
	s.setFailure("failed!")
	_, _, _, err := s.client.Backup(self.filename, false)

	c.Check(err, gc.ErrorMatches, "backup request failed on server")
}

func (s *backupSuite) TestBackupFailureWritingArchive(c *gc.C) {
	write := func(archive *os.File, infile io.Reader) (string, error) {
	}
	s.PatchValue(api.WriteBackup, write)
	_, _, _, err := s.client.Backup(self.filename, false)

	c.Check(err, gc.ErrorMatches, "could not save the backup")
}

func (s *backupSuite) TestBackupFailureParsingDigest(c *gc.C) {
	parse := func(header http.Header) (*os.File, string, error) {
	}
	s.PatchValue(api.ParseDigest, parse)
	filename, hash, expected, err := s.client.Backup(self.filename, false)

	c.Check(err, gc.IsNil)
	c.Check(filename, gc.Equals, self.filename)
	c.Check(hash, gc.Equals, "")
	c.Check(expected, gc.Equals, "")
	s.checkArchive(c, filename, hash)
}
