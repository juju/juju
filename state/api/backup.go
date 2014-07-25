// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/backup"
	backupAPI "github.com/juju/juju/state/backup/api"
	"github.com/juju/utils"
)

// for testing:
var (
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

type BackupResult struct {
	filename    string
	serverHash  string
	writtenHash string
	failure     *params.Error
}

func NewBackupResult(
	filename, serverHash, writtenHash string, failure *params.Error,
) *BackupResult {
	res := BackupResult{
		filename:    filename,
		serverHash:  serverHash,
		writtenHash: writtenHash,
		failure:     failure,
	}
	return &res
}

func (r *BackupResult) FilenameFromServer() string {
	return r.filename
}

func (r *BackupResult) HashFromServer() string {
	return r.serverHash
}

func (r *BackupResult) WrittenHash() string {
	return r.writtenHash
}

func (r *BackupResult) Failure() *params.Error {
	return r.failure
}

func (r *BackupResult) VerifyHash() bool {
	return r.writtenHash == r.serverHash
}

func (r *BackupResult) setFailure(msg string, cause error) {
	failure := params.Error{Message: msg}
	if cause != nil {
		failure.Code = params.ErrCode(cause)
		logger.Infof("backup client failure: %s (%v)", msg, cause)
	} else {
		logger.Infof("backup client failure: %s", msg)
	}
	r.failure = &failure
}

func (r *BackupResult) handleHeader(header *http.Header) {
	// Errors here are non-fatal.

	filename, err := extractFilename(header)
	if err != nil {
		logger.Infof("could not extract filename from HTTP response: %v", err)
	} else {
		r.filename = filename
	}

	serverHash, err := extractDigest(header)
	if err != nil {
		logger.Infof("could not extract digest from HTTP response: %v", err)
	} else {
		r.serverHash = serverHash
	}
}

// Backup requests a state-server backup file from the server and writes
// it to the provided file. It returns a BackupResult struct, populated
// with the results received from the server (or error information if
// the request failed).
//
// Note that the backup can take a long time to prepare. The resulting
// file can be quite large file, depending on the system being backed up.
func (c *Client) Backup(archive io.Writer) *BackupResult {
	var err error
	result := BackupResult{}
	fail := func(msg string, cause error) *BackupResult {
		result.setFailure(msg, cause)
		return &result
	}

	// Prepare the upload request.
	req, err := c.newRawBackupRequest()
	if err != nil {
		return fail("error while preparing backup request", err)
	}

	// Send the request.
	resp, err := c.sendRawBackupRequest(req)
	if err != nil {
		return fail("failure sending backup request", err)
	}
	defer resp.Body.Close()

	// Check the response.
	apiErr := checkAPIResponse(resp)
	if apiErr != nil {
		return fail("backup request failed on server", apiErr)
	}
	result.handleHeader(&resp.Header)

	// Save the backup.
	result.writtenHash, err = writeBackup(archive, resp.Body)
	if err != nil {
		return fail("could not save the backup", err)
	}

	return &result
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
