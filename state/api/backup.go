// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/backup"
)

// Backup requests a state-server backup file from the server and saves it to
// the local filesystem. It returns the name of the file created.
// The backup can take a long time to prepare and be a large file, depending
// on the system being backed up.
func (c *Client) Backup(backupFilePath string, validate bool) (string, error) {
	if backupFilePath == "" {
		formattedDate := time.Now().Format(backup.TimestampFormat)
		backupFilePath = fmt.Sprintf(backup.FilenameTemplate, formattedDate)
	}

	// Send the request.
	var errorResult params.BackupResponse
	resp, err := c.sendRawRPC("backup", &errorResult)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Write out the archive.
	err = writeBackupFile(backupFilePath, resp.Body)
	if err != nil {
		return "", err
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

func writeBackupFile(backupFilePath string, body io.Reader) error {
	file, err := os.Create(backupFilePath)
	if err != nil {
		return fmt.Errorf("Error creating backup file: %v", err)
	}
	defer file.Close()
	_, err = io.Copy(file, body)
	if err != nil {
		return fmt.Errorf("Error writing the backup file: %v", err)
	}
	return nil
}

func validateBackupHash(backupFilePath string, resp *http.Response) error {
	// Get the expected hash.
	prefix := "SHA="
	digest := resp.Header.Get("Digest")
	if !strings.HasPrefix(digest, prefix) {
		msg := "SHA digest missing from response. Can't verify backup file."
		return fmt.Errorf(msg)
	}
	expected := digest[len(prefix):]

	// Get the actual hash.
	tarball, err := os.Open(backupFilePath)
	if err != nil {
		return fmt.Errorf("could not open backup file: %s", backupFilePath)
	}
	defer tarball.Close()

	actual, err := backup.GetHash(tarball)
	if err != nil {
		return err
	}

	// Compare the hashes.
	if actual != expected {
		return fmt.Errorf("archive hash did not match value from server: %s != %s",
			actual, expected)
	}
	return nil
}
