// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/response"
)

// StorageSnapshotParams
type StorageSnapshotParams struct {

	// Description is the description of the storage
	// snapshot
	Description string `json:"description,omitempty"`

	// Name is the name of the snapshot
	Name string `json:"name"`

	// Parent_volume_bootable field indicating
	// whether the parent storage volume is bootable
	Parent_volume_bootable string `json:"parent_volume_bootable"`

	// Property that describes:
	// If you don't specify a value, a remote snapshot is created.
	// Remote snapshots aren't stored in the
	// same location as the original storage volume. Instead,
	// they are reduced and stored in the associated Oracle
	// Storage Cloud Service instance. Remote snapshots are useful
	// if your domain spans multiple sites.
	// With remote snapshots, you can create a snapshot in one
	// site, then switch to another site and create a copy of the
	// storage volume on that site. However, creating a
	// remote snapshot and restoring a storage volume from a remote
	// snapshot can take quite a long time depending on the size
	// of the storage volume, as data is written to and from
	// the Oracle Storage Cloud Service instance.
	// Specify /oracle/private/storage/snapshot/collocated to
	// create a collocated snapshot. Colocated snapshots
	// are stored in the same physical location as the
	// original storage volume and each snapshot uses the
	// same amount of storage as the original volume.
	// Colocated snapshots and volumes from colocated snapshots can
	// be created very quickly. Colocated snapshots are useful
	// for quickly cloning storage volumes within a site.
	// However, you can't restore volumes across sites using colocated snapshots
	Property string `json:"property"`

	// Tags are strings that describe the
	// storage snapshot and help you identify it
	Tags []string `json:"tags,omitempty"`

	// Volume is the volume name you wish to create
	// the snapshot
	Volume string `json:"volume"`
}

// validate will validate the storage snapshot params
func (s StorageSnapshotParams) validate() (err error) {
	if s.Name == "" {
		return errors.New(
			"go-oracle-cloud: Empty storage snapshot name",
		)
	}

	if s.Volume == "" {
		return errors.New(
			"go-oracle-cloud: Empty storage snapshot volume name target",
		)
	}

	return nil
}

// CreateStorageSnapshot creates a storage volume snapshot.
// Creating a storage volume snapshot enables you to capture
// the current state of the storage volume.
// You can retain snapshots as a backup, or use
// them to create new, identical storage volumes.
// when it is attached to an instance or after detaching it.
// You can create a snapshot of a storage volume either
// If the storage volume is attached to an instance, then only
// data that has already been written to the storage volume will
// be captured in the snapshot. Data that is cached by the
// application or the operating system will be excluded from
// the snapshot. To create a snapshot of a bootable storage
// volume that is currently being used by an instance, you
// should delete the instance before you create the snapshot,
// to ensure the consistency of data. You can create
// the instance again later on, after the snapshot is created.
func (c *Client) CreateStorageSnapshot(
	p StorageSnapshotParams,
) (resp response.StorageSnapshot, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	url := c.endpoints["storagesnapshot"] + "/"

	if err = c.request(paramsRequest{
		verb: "POST",
		url:  url,
		resp: &resp,
		body: &p,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// DeleteStorageSnapshot deletes the specified storage volume snapshot
func (c *Client) DeleteStorageSnapshot(
	name string,
) (err error) {

	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New(
			"go-oracle-cloud: Empty storage snapshot name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["storagesnapshot"], name)

	if err = c.request(paramsRequest{
		verb: "DELETE",
		url:  url,
	}); err != nil {
		return err
	}

	return nil
}

// StorageSnapshotDetails retrieves details about the specified storage volume snapshot
func (c *Client) StorageSnapshotDetails(
	name string,
) (resp response.StorageSnapshot, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty storage snapshot name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["storagesnapshot"], name)

	if err = c.request(paramsRequest{
		verb: "GET",
		url:  url,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// AllStorageSnapshots retrieves details of the storage volume snapshots that
// are available in the specified container and match the specified query criteria
func (c *Client) AllStorageSnapshots(
	filter []Filter,
) (resp response.AllStorageSnapshots, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["storagesnapshot"], c.identify, c.username)

	if err = c.request(paramsRequest{
		verb: "GET",
		url:  url,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// AllStorageSnapshotNames retrieves the names of objects and subcontainers that you can access in the specified container
func (c *Client) AllStorageSnapshotNames() (resp response.DirectoryNames, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["storagesnapshot"], c.identify, c.username)

	if err = c.request(paramsRequest{
		directory: true,
		verb:      "GET",
		url:       url,
		resp:      &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
