// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/juju/juju/state/api/params"
	"github.com/juju/utils"
)

// Backup requests a state-server backup file from the server and saves it to
// the local filesystem. It returns the name of the file created.
// The backup can take a long time to prepare and be a large file, depending
// on the system being backed up.
func (c *Client) Backup(backupFilePath string) (string, error) {
	// Prepare the upload request.
	url := fmt.Sprintf("%s/backup", c.st.serverRoot)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return "", fmt.Errorf("cannot create backup request: %v", err)
	}
	req.SetBasicAuth(c.st.tag, c.st.password)

	// Send the request.
	resp, err := utils.GetNonValidatingHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot fetch backup: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("cannot read backup response: %v", err)
		}
		var jsonResponse params.BackupResponse
		if err := json.Unmarshal(body, &jsonResponse); err != nil {
			return "", fmt.Errorf("cannot unmarshal backup response: %v", err)
		}

		return "", fmt.Errorf("error fetching backup: %v", jsonResponse.Error)
	}
	if backupFilePath == "" {
		backupFilePath = fmt.Sprintf("jujubackup-%s.tar.gz", "timestamp")
	}
	err = c.writeBackupFile(backupFilePath, resp.Body)
	if err != nil {
		return "", err
	}

	// XXXX what to do if the digest header is missing?
	// just log and return? (Seems hostile to delete it.)
	sha := resp.Header.Get("Digest")
	if sha == "" {
		logger.Warningf("SHA digest missing from response. Can't verify the backup file.")
		return backupFilePath, nil
	}
	return backupFilePath, nil
}

func (c *Client) writeBackupFile(backupFilePath string, body io.Reader) error {
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
