// Licensed under the LGPLv3, see LICENCE file for details.
// Copyright 2017 Canonical Ltd.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
)

// BackupConfigurationParams type used to feed up
// the CreateBackupConfiguration function with params.
type BackupConfigurationParams struct {

	// Description of this Backup Configuration
	Description string `json:"description,omitempty"`

	// BackupRetentionCount represents how many backups to retain
	// Minimum Value: 1
	BackupRetentionCount uint32 `json:"backupRetentionCount"`

	// Enabled when true, backups will automatically
	// be generated based on the interval.
	Enabled bool `json:"enabled"`

	// Name is the name of the backup configuration
	Name string `json:"name"`

	// VolumeUri the complete URI of the storage volume
	// that you want to backup.
	VolumeUri string `json:"volumeUri"`

	// Interval represents the interval in the backup configuration.
	// There are two kinds of Intervals. Each Interval has its own JSON format.
	// Your Interval field should look like one of the following:
	//
	// "interval":{
	//    "Hourly":{
	//     "hourlyInterval":2
	//	  }
	//  }
	//
	//
	// {
	//   "DailyWeekly": {
	//	   "daysOfWeek":["MONDAY"],
	//	   "timeOfDay":"03:15",
	// 	   "userTimeZone":"America/Los_Angeles"
	//    }
	// }
	// Days of the week is any day of the week
	// fully capitalized (MONDAY, TUESDAY, etc).
	// The user time zone is any IANA user timezone.
	// For example user time zones see List of IANA time zones.
	//
	Interval common.Interval `json:"interval"`
}

// validate will validate the backup configuration params passed
func (c BackupConfigurationParams) validate() (err error) {
	if c.Name == "" {
		return errors.New(
			"go-oracle-cloud: Empty backup configuration name",
		)
	}

	if err = c.Interval.Validate(); err != nil {
		return err
	}

	return nil
}

// CreateBackupConfiguration creates a new backup configuration.
// Requires authorization to create backup configurations as well
// as appropriate authorization to create snapshots from the target volume.
func (c *Client) CreateBackupConfiguration(
	p BackupConfigurationParams,
) (resp response.BackupConfiguration, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	url := c.endpoints["backupconfiguration"] + "/"

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

// DeleteBackupConfiguration deletes a backup configuration.
// In order to delete the configuration all backups and restores
// related to the configuration must already be deleted.
// If disabling a backup configuration is desired, consider setting enabled to false.
func (c *Client) DeleteBackupConfiguration(name string) (err error) {
	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New(
			"go-oracle-cloud: Empty backup configuration name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["backupconfiguration"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}

	return nil
}

// BackupConfigurationDetails retrieves details of the specified
// backup configuration. You can use this request to verify whether
// the CreateBackupConfiguration and UpdateBackupConfiguration
// requests were completed successfully.
func (c *Client) BackupConfigurationDetails(
	name string,
) (resp response.BackupConfiguration, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty backup configuration name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["backupconfiguration"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// AllBackupConfigurations retrieves details for all backup
// configuration objects the current user has permission to access
func (c *Client) AllBackupConfigurations(filter []Filter) (resp []response.BackupConfiguration, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := c.endpoints["backupconfiguration"] + "/"

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

// UpdateBackupConfiguration
// Modify an existing backup configuration.
// All fields, including unmodifiable fields, must be provided
// for this operation. The following fields are unmodifiable:
// volumeName, runAsUser, name.
func (c *Client) UpdateBackupConfiguration(
	p BackupConfigurationParams,
	currentName string,
) (resp response.BackupConfiguration, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if currentName == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty backup configuration current name",
		)
	}

	if p.Name == "" {
		p.Name = currentName
	}

	url := fmt.Sprintf("%s%s", c.endpoints["backupconfiguration"], currentName)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "PUT",
		body: &p,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
