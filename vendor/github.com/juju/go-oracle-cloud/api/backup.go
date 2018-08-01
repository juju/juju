// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/response"
)

// BackupParams used as params to CreateBackup function
type BackupParams struct {

	// BackupConfigurationName multi-part name of the backup configuration.
	BackupConfigurationName string `json:"backupConfigurationName"`

	// Description of the Backup
	Description string `json:"description,omitempty"`

	// Name of the backup
	Name string `json:"name"`
}

// validate validates the backup params
func (b BackupParams) validate() (err error) {

	if b.BackupConfigurationName == "" {
		return errors.New(
			"go-oracle-cloud: Empty backup name",
		)
	}

	if b.Name == "" {
		return errors.New(
			"go-oracle-cloud: Empty backup name",
		)
	}

	return nil
}

// CreateBackup schedules a backup immediately using the
// specified backup configuration. The storage
// volume that you have specified in the backup configuration
// is backed up immediately, irrespective of the
// status of enabled in the specified backup configuration.
func (c *Client) CreateBackup(
	p BackupParams,
) (resp response.Backup, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	url := c.endpoints["backup"] + "/"

	if err = c.request(paramsRequest{
		url:  url,
		verb: "POST",
		body: &p,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// DeleteBackup delete a backup and it's associated snapshot.
// In progress backups may not be deleted
func (c *Client) DeleteBackup(
	name string,
) (err error) {

	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New(
			"go-oracle-cloud: Empty backup name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["backup"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}

	return nil
}

// AllBackups retrieves details of the backups
// that are available and match the specified query criteria
func (c *Client) AllBackups(
	filter []Filter,
) (resp response.AllBackups, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["backup"], c.identify, c.username)

	if err = c.request(paramsRequest{
		url:    url,
		verb:   "GET",
		resp:   &resp,
		filter: filter,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// BackupDetails retrives the backup specified by
// the provided multi-part object name.
func (c *Client) BackupDetails(
	name string,
) (resp response.Backup, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty backup name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["backup"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
