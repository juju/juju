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

var (
	createEmptyFile  = backup.CreateEmptyFile
	writeBackup      = backup.WriteBackup
	newHTTPRequest   = backupAPI.NewAPIRequest
	checkAPIResponse = backupAPI.CheckAPIResponse
	parseDigest      = backupAPI.ParseDigest
	extractFilename  = backupAPI.ExtractFilename
	sendHTTPRequest  = _sendHTTPRequest
)

// for testing:
func _sendHTTPRequest(req *http.Request, client *http.Client) (*http.Response, error) {
	return client.Do(req)
}

// Backup requests a state-server backup file from the server and saves
// it to the local filesystem. It returns the name of the file created,
// along with the SHA-1 hash of the file and the expected hash (in that
// order).  The expected hash is reported by the server in the "Digest"
// header of the HTTP response.  If desired the two hashes can be
// compared to verify that the file is correct.
//
// Note that the backup can take a long time to prepare. The resulting
// file can be quite large file, depending on the system being backed up.
func (c *Client) Backup(backupFilePath string, excl bool) (
	filename string, hash string, expectedHash string, failure *params.Error,
) {
	var file *os.File
	var err error
	closeAndCleanup := func() {
		file.Close()
		if failure != nil {
			// Make sure we clean up any empty temp files.
			logger.Debugf("cleaning up %s", filename)
			os.Remove(filename)
		}
	}

	// Get an empty backup file ready.
	file, filename, err = createEmptyFile(backupFilePath, excl)
	if err != nil {
		failure = c.newFailure("error while preparing backup file", err)
		return
	}
	defer closeAndCleanup()
	filename, err = filepath.Abs(filename)
	if err != nil {
		failure = c.newFailure("could not resolve filename", err)
		return
	}
	if backupFilePath == "" {
		logger.Debugf("saving to temp file: %q", filename)
	}

	// Prepare the upload request.
	req, err := c.newRawBackupRequest()
	if err != nil {
		failure = c.newFailure("error while preparing backup request", err)
		return
	}

	// Send the request.
	resp, err := c.sendRawBackupRequest(req)
	if err != nil {
		failure = c.newFailure("failure sending backup request", err)
		return
	}
	defer resp.Body.Close()

	// Check the response.
	apiErr := checkAPIResponse(resp)
	if apiErr != nil {
		failure = c.newFailure("backup request failed on server", apiErr)
		return
	}

	// Save the backup.
	hash, err = writeBackup(file, resp.Body)
	if err != nil {
		failure = c.newFailure("could not save the backup", err)
		return
	}

	// Extract the SHA-1 hash.
	expectedHash, err = parseDigest(resp.Header)
	if err != nil {
		// This is a non-fatal error.
		logger.Infof("could not extract digest from HTTP response: %v", err)
	}

	// Handle the filename from the server.
	serverFilename, err := extractFilename(resp.Header)
	if err != nil {
		logger.Infof("could not extract filename from HTTP response: %v", err)
	} else if backupFilePath == "" && serverFilename != "" {
		err := file.Sync()
		if err != nil {
			logger.Infof("could not flush tempfile: %v", err)
		} else {
			err := os.Rename(filename, serverFilename)
			if err != nil {
				logger.Infof("could not move tempfile to new location: %v", err)
			} else {
				filename = serverFilename
			}
		}
	}

	// Log and return the result.
	logger.Infof("backup archive saved to %q", filename)
	return filename, hash, expectedHash, nil
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

func (c *Client) newFailure(msg string, cause error) *params.Error {
	var failure *params.Error
	if cause != nil {
		failure = &params.Error{
			Code: params.ErrCode(cause),
		}
		logger.Infof("backup client failure: %s (%v)", msg, cause)
	} else {
		failure = &params.Error{}
		logger.Infof("backup client failure: %s", msg)
	}
	failure.Message = msg
	return failure
}
