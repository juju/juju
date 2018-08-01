// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
)

// StorageAttachmentParams
type StorageAttachmentParams struct {
	// Index number for the volume.
	// The allowed range is 1-10.
	// The index determines the device name by which the
	// volume is exposed to the instance. Index 0 is allocated
	// to the temporary boot disk, /dev/xvda An attachment with
	// index 1 is exposed to the instance as /dev/xvdb,
	// an attachment with index 2 is exposed as /dev/xvdc, and so on.
	Index common.Index `json:"index"`

	// Instance_name multipart name of the instance
	// to which you want to attach the volume
	Instance_name string `json:"instance_name"`

	// Storage_volume_name is the name of the storage volume
	Storage_volume_name string `json:"storage_volume_name"`
}

// validate validates the given storage attachment params
func (s StorageAttachmentParams) validate() (err error) {
	if s.Instance_name == "" {
		return errors.New(
			"go-oracle-cloud: Empty storage attachment instance name",
		)
	}

	if err = s.Index.Validate(); err != nil {
		return err
	}

	if s.Storage_volume_name == "" {
		return errors.New(
			"go-oracle-cloud: Empty storage attachment volume name",
		)
	}

	return nil
}

// CreateStorageAttachment creates an attachment of a storage volume
// to an instance a storage attachment is an association between
// Note that, after attaching the volume, you must create a file
// system and mount the file system on the instance.<Paste>
func (c *Client) CreateStorageAttachment(
	p StorageAttachmentParams,
) (resp response.StorageAttachment, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	url := c.endpoints["storageattachment"] + "/"

	if err = c.request(paramsRequest{
		verb: "POST",
		url:  url,
		body: &p,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// DeleteStorageAttachment deletes the specified storage
// attachment. No response is returned.
func (c *Client) DeleteStorageAttachment(
	name string,
) (err error) {

	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New(
			"go-oracle-cloud: Empty storage attachment instance name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["storageattachment"], name)

	if err = c.request(paramsRequest{
		verb: "DELETE",
		url:  url,
	}); err != nil {
		return err
	}

	return nil
}

// retrieves details of the specified storage attachment
func (c *Client) StorageAttachmentDetails(
	name string,
) (resp response.StorageAttachment, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty storage attachment instance name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["storageattachment"], name)

	if err = c.request(paramsRequest{
		verb: "GET",
		url:  url,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

func (c *Client) AllStorageAttachments(
	filter []Filter,
) (resp response.AllStorageAttachments, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["storageattachment"], c.identify, c.username)

	if err = c.request(paramsRequest{
		verb:   "GET",
		url:    url,
		resp:   &resp,
		filter: filter,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
