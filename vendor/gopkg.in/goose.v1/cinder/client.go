// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package cinder

import (
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Basic returns a basic Cinder client which will handle authorization
// of requests, and routing to the correct endpoint.
func Basic(endpoint *url.URL, tenantId string, token TokenFn) *Client {
	return NewClient(tenantId, endpoint,
		SetAuthHeaderFn(token, http.DefaultClient.Do),
	)
}

// TokenFn represents a function signature which returns the user's
// authorization token when called.
type TokenFn func() string

// SetAuthHeaderFn returns a RequestHandlerFn which sets the
// authentication headers for a given request.
func SetAuthHeaderFn(token TokenFn, wrappedHandler RequestHandlerFn) RequestHandlerFn {
	return func(req *http.Request) (*http.Response, error) {
		req.Header.Set("X-Auth-Token", token())
		return wrappedHandler(req)
	}
}

// NewClient is the most flexible way to instantiate a Cinder
// Client. The handleRequest function will be responsible for
// modifying requests and dispatching them as needed. For an example
// of how to utilize this method, see the Basic function.
func NewClient(tenantId string, endpoint *url.URL, handleRequest RequestHandlerFn) *Client {
	if endpoint == nil {
		return nil
	}
	// Ensure the cinder endpoint has a trailing slash on the path.
	path := endpoint.Path
	if len(path) != 0 && path[len(path)-1:] != "/" {
		changedEndpoint := *endpoint
		changedEndpoint.Path += "/"
		endpoint = &changedEndpoint
	}
	return &Client{tenantId, endpoint, handleRequest}
}

// Client is a Cinder client.
type Client struct {
	tenantId      string
	endpoint      *url.URL
	handleRequest RequestHandlerFn
}

// GetSnapshot shows information for a specified snapshot.
func (c *Client) GetSnapshot(snapshotId string) (*GetSnapshotResults, error) {
	return getSnapshot(
		c,
		GetSnapshotParams{TenantId: c.tenantId, SnapshotId: snapshotId},
	)
}

// UpdateSnapshot updates a specified snapshot.
func (c *Client) UpdateSnapshot(snapshotId string, args UpdateSnapshotSnapshotParams) (*UpdateSnapshotResults, error) {
	return updateSnapshot(c, UpdateSnapshotParams{
		TenantId:   c.tenantId,
		SnapshotId: snapshotId,
		Snapshot:   args,
	})
}

// DeleteSnapshot deletes a specified snapshot.
func (c *Client) DeleteSnapshot(snapshotId string) error {
	_, err := deleteSnapshot(
		c,
		DeleteSnapshotParams{TenantId: c.tenantId, SnapshotId: snapshotId},
	)
	return err
}

// VersionDetails shows details for Block Storage API v2.
func (c *Client) VersionDetails() (*VersionDetailsResults, error) {
	return versionDetails(c, VersionDetailsParams{})
}

// ListExtensionsCinderV2 lists Block Storage API extensions.
func (c *Client) ListExtensionsCinderV2() (*ListExtensionsCinderV2Results, error) {
	return listExtensionsCinderV2(c, ListExtensionsCinderV2Params{})
}

// GetVolumesSimple lists summary information for all Block Storage
// volumes that the tenant who submits the request can access.
func (c *Client) GetVolumesSimple() (*GetVolumesSimpleResults, error) {
	return getVolumesSimple(c, GetVolumesSimpleParams{TenantId: c.tenantId})
}

// UpdateVolumeType updates a volume type.
func (c *Client) UpdateVolumeType(volumeTypeId, volumeType string) (*UpdateVolumeTypeResults, error) {
	return updateVolumeType(c, UpdateVolumeTypeParams{
		TenantId:     c.tenantId,
		VolumeTypeId: volumeTypeId,
		VolumeType:   volumeType,
	})
}

// DeleteVolumeType deletes a specified volume type.
func (c *Client) DeleteVolumeType(volumeTypeId string) error {
	_, err := deleteVolumeType(
		c,
		DeleteVolumeTypeParams{TenantId: c.tenantId, VolumeTypeId: volumeTypeId},
	)
	return err
}

// GetVolumesDetail lists detailed information for all Block Storage
// volumes that the tenant who submits the request can access.
func (c *Client) GetVolumesDetail() (*GetVolumesDetailResults, error) {
	return getVolumesDetail(c, GetVolumesDetailParams{TenantId: c.tenantId})
}

// GetVolume lists information about the volume with the given
// volumeId.
func (c *Client) GetVolume(volumeId string) (*GetVolumeResults, error) {
	return getVolume(c, GetVolumeParams{TenantId: c.tenantId, VolumeId: volumeId})
}

// CreateVolumeType creates a volume type.
func (c *Client) CreateVolumeType(args CreateVolumeTypeVolumeTypeParams) (*CreateVolumeTypeResults, error) {
	return createVolumeType(
		c,
		CreateVolumeTypeParams{TenantId: c.tenantId, VolumeType: args},
	)
}

// GetVolumeType shows information about a specified volume type.
func (c *Client) GetVolumeType(volumeTypeId string) (*GetVolumeTypeResults, error) {
	return getVolumeType(
		c,
		GetVolumeTypeParams{TenantId: c.tenantId, VolumeTypeId: volumeTypeId},
	)
}

// ListVersion lists information about all Block Storage API versions.
func (c *Client) ListVersions() (*ListVersionsResults, error) {
	return listVersions(c, ListVersionsParams{})
}

// UpdateVolumeTypeExtraSpecs updates the extra specifications
// assigned to a volume type.
func (c *Client) UpdateVolumeTypeExtraSpecs(volumeTypeId, volumeType, extraSpecs string) (*UpdateVolumeTypeExtraSpecsResults, error) {
	return updateVolumeTypeExtraSpecs(c, UpdateVolumeTypeExtraSpecsParams{
		TenantId:     c.tenantId,
		VolumeTypeId: volumeTypeId,
		VolumeType:   volumeType,
		ExtraSpecs:   extraSpecs,
	})
}

// GetSnapshotsSimple lists summary information for all Block Storage
// snapshots that the tenant who submits the request can access.
func (c *Client) GetSnapshotsSimple() (*GetSnapshotsSimpleResults, error) {
	return getSnapshotsSimple(c, GetSnapshotsSimpleParams{TenantId: c.tenantId})
}

// ShowSnapshotMetadata shows the metadata for a specified snapshot.
func (c *Client) ShowSnapshotMetadata(snapshotId string) (*ShowSnapshotMetadataResults, error) {
	return showSnapshotMetadata(
		c,
		ShowSnapshotMetadataParams{TenantId: c.tenantId, SnapshotId: snapshotId},
	)
}

// CreateSnapshot creates a snapshot, which is a point-in-time
// complete copy of a volume. You can create a volume from the
// snapshot.
func (c *Client) CreateSnapshot(args CreateSnapshotSnapshotParams) (*CreateSnapshotResults, error) {
	return createSnapshot(c, CreateSnapshotParams{TenantId: c.tenantId, Snapshot: args})
}

// GetSnapshotsDetail lists detailed information for all Block Storage
// snapshots that the tenant who submits the request can access.
func (c *Client) GetSnapshotsDetail() (*GetSnapshotsDetailResults, error) {
	return getSnapshotsDetail(c, GetSnapshotsDetailParams{TenantId: c.tenantId})
}

// UpdateSnapshotMetadata updates the metadata for a specified
// snapshot.
func (c *Client) UpdateSnapshotMetadata(snapshotId, key string) (*UpdateSnapshotMetadataResults, error) {
	return updateSnapshotMetadata(c, UpdateSnapshotMetadataParams{
		TenantId:   c.tenantId,
		SnapshotId: snapshotId,
		Metadata: UpdateSnapshotMetadataMetadataParams{
			Key: key,
		},
	})
}

// CreateVolume creates a volume. To create a bootable volume, include
// the image ID and set the bootable flag to true in the request body.
//
// Preconditions:
//
// - The user must have enough volume storage quota remaining to create
//   a volume of size requested.
//
// Asynchronous Postconditions:
//
// - With correct permissions, you can see the volume status as
//   available through API calls.
// - With correct access, you can see the created volume in the
//   storage system that OpenStack Block Storage manages.
//
// Troubleshooting:
//
// - If volume status remains creating or shows another error status,
//   the request failed. Ensure you meet the preconditions then
//   investigate the storage backend.
// - Volume is not created in the storage system which OpenStack Block Storage manages.
// - The storage node needs enough free storage space to match the
//   specified size of the volume creation request.
func (c *Client) CreateVolume(args CreateVolumeVolumeParams) (*CreateVolumeResults, error) {
	return createVolume(c, CreateVolumeParams{TenantId: c.tenantId, Volume: args})
}

// UpdateVolume updates a volume.
func (c *Client) UpdateVolume(volumeId string, args UpdateVolumeVolumeParams) (*UpdateVolumeResults, error) {
	return updateVolume(c, UpdateVolumeParams{TenantId: c.tenantId, VolumeId: volumeId, Volume: args})
}

// DeleteVolume flags a volume for deletion. The volume managed by
// OpenStack Block Storage is not deleted from the storage system.
func (c *Client) DeleteVolume(volumeId string) error {
	_, err := deleteVolume(
		c,
		DeleteVolumeParams{TenantId: c.tenantId, VolumeId: volumeId},
	)
	return err
}

// GetVolumeTypes lists volume types.
func (c *Client) GetVolumeTypes() (*GetVolumeTypesResults, error) {
	return getVolumeTypes(c, GetVolumeTypesParams{TenantId: c.tenantId})
}

type predicateFn func() (bool, error)

func notifier(predicate predicateFn, numAttempts int, waitDur time.Duration) <-chan error {
	notifierChan := make(chan error)
	go func() {
		defer close(notifierChan)
		for attemptNum := 0; attemptNum < numAttempts; attemptNum++ {
			if ok, err := predicate(); err != nil {
				notifierChan <- err
				return
			} else if ok {
				return
			}

			time.Sleep(waitDur)
		}
		notifierChan <- fmt.Errorf("too many attempts")
	}()
	return notifierChan
}

// VolumeStatusNotifier will check a volume's status to determine
// whether it matches the given status. After a check, it waits for
// "waitDur" before attempting again. If the status does not match
// after "numAttempts", an error is returned.
func (c *Client) VolumeStatusNotifier(volId, status string, numAttempts int, waitDur time.Duration) <-chan error {
	statusMatches := func() (bool, error) {
		volInfo, err := c.GetVolume(volId)
		if err != nil {
			return false, err
		}
		return volInfo.Volume.Status == status, nil
	}
	return notifier(statusMatches, numAttempts, waitDur)
}

// SnapshotStatusNotifier will check a volume's status to determine
// whether it matches the given status. After a check, it waits for
// "waitDur" before attempting again. If the status does not match
// after "numAttempts", an error is returned.
func (c *Client) SnapshotStatusNotifier(snapId, status string, numAttempts int, waitDur time.Duration) <-chan error {
	statusMatches := func() (bool, error) {
		snapInfo, err := c.GetSnapshot(snapId)
		if err != nil {
			return false, err
		}
		return snapInfo.Snapshot.Status == status, nil
	}
	return notifier(statusMatches, numAttempts, waitDur)
}
