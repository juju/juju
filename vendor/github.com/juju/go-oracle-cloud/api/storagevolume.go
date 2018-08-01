// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
)

// StorageVolumeParams used for StorageVolume functions type
// for providing parameters
type StorageVolumeParams struct {

	// Bootable is a a boolean field that indicates whether
	// the storage volume can be used as the boot
	// disk for an instance.
	// If you set the value to true, then you must
	// specify values for the following parameters imagelist
	// The machine image that you want to extract
	// on to the storage volume that you're creating.
	// imagelist_entry field (optional)
	// for the version of the image list entry
	// that you want to extract. The default value is 1.
	Bootable bool `json:"bootable"`

	// Description is the description of the
	// storage volume
	Description string `json:"description"`

	// Imagelist is the name of machineimage
	// to extract onto this volume when created.<Paste>
	Imagelist string `json:"imagelist"`

	// Imagelist_entry is a pecific imagelist entry version to extract.
	Imagelist_entry int `json:"imagelist_entry"`

	// Name is the name of the storage volume
	Name string `json:"name"`

	// Properties contains the storage-pool properties
	// For storage volumes that require low latency and high IOPS,
	// such as for storing database files, specify common.LatencyPool
	// For all other storage volumes, specify common.DefaultPool
	Properties []common.StoragePool `json:"properties"`

	// Size is the the size of this storage volume.
	// Use one of the following abbreviations for the unit of measurement:
	// B or b for bytes
	// K or k for kilobytes
	// M or m for megabytes
	// G or g for gigabytes
	// T or t for terabytes
	// For example, to create a volume of size 10 gigabytes,
	// you can specify 10G, or 10240M, or 10485760K, and so on.
	// The allowed range is from 1 GB to 2 TB, in increments of 1 GB.
	// If you are creating a bootable storage volume, ensure
	// that the size of the storage volume is greater
	// than the size of the machine image that you want
	// to extract on to the storage volume.
	// If you are creating this storage volume from a storage snapshot,
	// ensure that the size of the storage volume that you create
	// is greater than the size of the storage snapshot.
	Size common.StorageSize `json:"size"`

	// Snapshot multipart name of the storage volume snapshot if
	// this storage volume is a clone.
	Snapshot string `json:"snapshot"`

	// Snapshot_account of the parent snapshot
	// from which the storage volume is restored
	Snapshot_account string `json:"snapshot_account"`

	// Snapshot_id is the dd of the parent snapshot
	// from which the storage volume is restored or cloned
	Snapshot_id string `json:"snapshot_id"`

	// Tags are strings that you can use to tag the storage volume.
	Tags []string `json:"tags,omitempty"`
}

func (s StorageVolumeParams) validate() (err error) {
	if s.Name == "" {
		return errors.New(
			"go-oracle-cloud: Empty storage volume name",
		)
	}

	if s.Properties == nil || len(s.Properties) == 0 {
		return errors.New(
			"go-oracle-cloud: Empty storage volume properties",
		)
	}

	if s.Size == "" {
		return errors.New(
			"go-oracle-cloud: Empty storage volume size",
		)
	}

	if s.Bootable {
		if s.Imagelist == "" {
			return errors.New(
				"go-oracle-cloud: Empty storage volume imagelist",
			)
		}

	}
	return nil
}

// CreateStorageVolume creates a storage volume
// After creating storage volumes you can attach them to instances
func (c *Client) CreateStorageVolume(
	p StorageVolumeParams,
) (resp response.StorageVolume, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	url := c.endpoints["storagevolume"] + "/"

	if err = c.request(paramsRequest{
		url:  url,
		verb: "POST",
		resp: &resp,
		body: &p,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// DeleteStorageVolume deletes the specified storage volume
func (c *Client) DeleteStorageVolume(
	name string,
) (err error) {

	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New(
			"go-oracle-cloud Empty storage volume name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["storagevolume"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}

	return nil
}

// retrieves details about the specified storage volume
func (c *Client) StorageVolumeDetails(
	name string,
) (resp response.StorageVolume, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud Empty storage volume name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["storagevolume"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// AllStorageVolumes retrieves details of the storage volumes
// that are available in the specified container and match
// the specified query criteria
func (c *Client) AllStorageVolumes(
	filter []Filter,
) (resp response.AllStorageVolumes, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["storagevolume"], c.identify, c.username)

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

// UpdateStorageVolume pdates a storage volume
// Although you have to pass values for several parameters,
// you can only increase the size of the storage volume
// and modify the values for the tags and description parameters.
// You can update an existing storage volume to increase
// the capacity dynamically, even when the volume is attached
// to an instance. You must specify all the required fields,
// although these fields won't be updated.
func (c *Client) UpdateStorageVolume(
	p StorageVolumeParams,
	currentName string,
) (resp response.StorageVolume, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	if currentName == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty storage volume current name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["storagevolume"], currentName)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "PUT",
		resp: &resp,
		body: &p,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
