// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/backup"
	backupAPI "github.com/juju/juju/state/backup/api"
	"github.com/juju/utils"
)

// for testing:
var (
	createEmptyFile  = backup.CreateEmptyFile
	writeBackup      = backup.WriteBackup
	newHTTPRequest   = backupAPI.NewAPIRequest
	checkAPIResponse = backupAPI.CheckAPIResponse
	extractDigest    = backupAPI.ExtractSHAFromDigestHeader
	extractFilename  = backupAPI.ExtractFilename
	sendHTTPRequest  = func(r *http.Request, c *http.Client) (*http.Response, error) {
		return c.Do(r)
	}
)

//---------------------------
// backup

type backupResult struct {
	filename string
	isTemp   bool
	header   *http.Header
	hash     string
	failure  *params.Error
}

func (r *backupResult) setFailure(msg string, cause error) {
	failure := params.Error{Message: msg}
	if cause != nil {
		failure.Code = params.ErrCode(cause)
		logger.Infof("backup client failure: %s (%v)", msg, cause)
	} else {
		logger.Infof("backup client failure: %s", msg)
	}
	r.failure = &failure
}

func (r *backupResult) fail(msg string, cause error) *backupResult {
	r.setFailure(msg, cause)
	return r
}

func (r *backupResult) checkFilenameHeader() error {
	_, err := extractFilename(r.header)
	if err != nil {
		return fmt.Errorf("could not extract filename from HTTP response: %v", err)
	}
	return nil
}

func (r *backupResult) syncBackupFilename() error {
	serverFilename, err := extractFilename(r.header)
	if err != nil {
		return fmt.Errorf("could not extract filename from HTTP response: %v", err)
	}

	target := filepath.Join(filepath.Base(r.filename), serverFilename)
	err = os.Rename(r.filename, target)
	if err != nil {
		return fmt.Errorf("could not move tempfile to new location: %v", err)
	}
	r.filename = target

	return nil
}

func (r *backupResult) cleanUp() {
	// Clean up any empty temp files.
	err := os.Remove(r.filename)
	if err != nil {
		logger.Infof("unable to clean up temp backup file: %v", err)
	}
}

func (c *Client) runBackup(filename string, excl bool) *backupResult {
	var file *os.File
	var err error
	result := backupResult{filename: filename}

	// Get an empty backup file ready *before* sending the request.
	file, result.filename, err = createEmptyFile(filename, 0600, excl)
	if err != nil {
		return result.fail("error while preparing backup file", err)
	}
	defer file.Close()
	result.isTemp = (filename != result.filename)
	absfilename, err := filepath.Abs(result.filename)
	if err == nil { // Otherwise we stick with the old filename.
		result.filename = absfilename
	}
	logger.Debugf("prepared empty backup file: %q", result.filename)

	// Prepare the upload request.
	req, err := c.newRawBackupRequest()
	if err != nil {
		return result.fail("error while preparing backup request", err)
	}

	// Send the request.
	resp, err := c.sendRawBackupRequest(req)
	if err != nil {
		return result.fail("failure sending backup request", err)
	}
	defer resp.Body.Close()
	result.header = &resp.Header

	// Check the response.
	apiErr := checkAPIResponse(resp)
	if apiErr != nil {
		return result.fail("backup request failed on server", apiErr)
	}

	// Save the backup.
	result.hash, err = writeBackup(file, resp.Body)
	if err != nil {
		return result.fail("could not save the backup", err)
	}

	return &result
}

// Backup requests a state-server backup file from the server and saves
// it to the local filesystem. It returns the name of the file created,
// along with the SHA-1 hash of the file and the expected hash (in that
// order).  The expected hash is reported by the server in the "Digest"
// header of the HTTP response.  If desired the two hashes can be
// compared to verify that the file is correct.
//
// If no filename is passed in, one is generated relative to the current
// directory with a format like "juju-backup-20140606-050109.tar.gz".
// The timestamp will closely match when the backup request was issued.
// If a filename ending with the path separator (e.g. /) is passed in,
// the generated filename will be relative to that dirname rather than
// to the current directory.
//
// Note that the backup can take a long time to prepare. The resulting
// file can be quite large file, depending on the system being backed up.
func (c *Client) Backup(backupFilePath string, excl bool) (
	filename string, hash string, expectedHash string, failure *params.Error,
) {
	var err error

	// Run backups!
	res := c.runBackup(backupFilePath, excl)
	if res.failure != nil {
		res.cleanUp()
		return "", "", "", res.failure
	}
	// We treat any error past this point as non-fatal.

	// Extract the SHA-1 hash.
	expectedHash, err = extractDigest(res.header)
	if err != nil {
		logger.Infof("could not extract digest from HTTP response: %v", err)
	}

	// Handle the filename from the server.
	if res.isTemp {
		err = res.syncBackupFilename()
	} else {
		// We always check the header, even if we aren't going to use it.
		err = res.checkFilenameHeader()
	}
	if err != nil {
		logger.Infof(err.Error())
	}

	// Log and return the result.
	logger.Infof("backup archive saved to %q", res.filename)
	return res.filename, res.hash, expectedHash, nil
}

//---------------------------
// helpers

func (c *Client) newRawBackupRequest() (*http.Request, error) {
	baseURL, err := url.Parse(c.st.serverRoot)
	if err != nil {
		return nil, fmt.Errorf("could not create base URL: %v", err)
	}
	uuid := c.EnvironmentUUID()
	req, err := newHTTPRequest(baseURL, uuid, c.st.tag, c.st.password)
	if err != nil {
		return nil, fmt.Errorf("could not create HTTP request: %v", err)
	}
	return req, nil
}

func (c *Client) getHTTPClient(secure bool) *http.Client {
	var httpclient *http.Client
	if secure {
		httpclient = utils.GetValidatingHTTPClient()
		tlsconfig := tls.Config{RootCAs: c.st.certPool, ServerName: "anything"}
		httpclient.Transport = utils.NewHttpTLSTransport(&tlsconfig)
	} else {
		httpclient = utils.GetValidatingHTTPClient()
	}
	return httpclient
}

func (c *Client) sendRawBackupRequest(req *http.Request) (*http.Response, error) {
	httpclient := c.getHTTPClient(true)
	resp, err := sendHTTPRequest(req, httpclient)
	if err != nil {
		return nil, fmt.Errorf("error when sending HTTP request: %v", err)
	}
	return resp, nil
}
