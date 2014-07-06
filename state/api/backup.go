// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/juju/juju/state/backup"
)

var getBackupHash = backup.GetHashDefault

// Backup requests a state-server backup file from the server and saves it to
// the local filesystem. It returns the name of the file created.
// The backup can take a long time to prepare and be a large file, depending
// on the system being backed up.
func (c *Client) Backup(backupFilePath string, validate bool) (string, error) {
	if backupFilePath == "" {
		backupFilePath = backup.DefaultFilename()
	}

	// Open the backup file.
	file, err := os.Create(backupFilePath)
	if err != nil {
		return "", fmt.Errorf("error creating backup file: %v", err)
	}
	defer file.Close()

	// Send the request.
	resp, err := c.sendHTTPRequest("POST", "backup")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Write out the archive.
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return "", fmt.Errorf("error writing the backup file: %v", err)
	}

	// Validate the result.
	if validate {
		err = validateBackupHash(backupFilePath, resp)
		if err != nil {
			return backupFilePath, err
		}
	}

	return backupFilePath, nil
}

func validateBackupHash(backupFilePath string, resp *http.Response) error {
	// Get the expected hash.
	prefix := "SHA="
	digest := resp.Header.Get("Digest")
	if digest == "" {
		msg := "could not verify backup file: SHA digest missing from response"
		return fmt.Errorf(msg)
	}
	if !strings.HasPrefix(digest, prefix) {
		msg := "could not verify backup file: unrecognized Digest header (expected \"%s\")"
		return fmt.Errorf(msg, prefix)
	}
	expected := digest[len(prefix):]

	// Get the actual hash.
	actual, err := getBackupHash(backupFilePath)
	if err != nil {
		return fmt.Errorf("could not verify backup file: %v", err)
	}

	// Compare the hashes.
	if actual != expected {
		return fmt.Errorf("archive hash did not match value from server: %s != %s",
			actual, expected)
	}
	return nil
}
