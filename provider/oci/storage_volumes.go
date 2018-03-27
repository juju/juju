// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	providerCommon "github.com/juju/juju/provider/oci/common"
	"github.com/juju/juju/storage"

	ociCore "github.com/oracle/oci-go-sdk/core"
	// ociIdentity "github.com/oracle/oci-go-sdk/identity"
)

func mibToGib(m uint64) uint64 {
	return (m + 1023) / 1024
}

type volumeSource struct {
	env       *Environ
	envName   string
	modelUUID string
	api       providerCommon.ApiClient
	clock     clock.Clock
}

func newOciVolumeSource(env *Environ, name, uuid string, api providerCommon.ApiClient, clock clock.Clock) (*volumeSource, error) {
	if env == nil {
		return nil, errors.NotFoundf("environ")
	}

	if api == nil {
		return nil, errors.NotFoundf("storage client")
	}

	return &volumeSource{
		env:       env,
		envName:   name,
		modelUUID: uuid,
		api:       api,
		clock:     clock,
	}, nil
}

var _ storage.VolumeSource = (*volumeSource)(nil)

func (v *volumeSource) getVolumeStatus(resourceID *string) (string, error) {
	request := ociCore.GetVolumeRequest{
		VolumeId: resourceID,
	}

	response, err := v.api.GetVolume(context.Background(), request)
	if err != nil {
		if v.env.isNotFound(response.RawResponse) {
			return "", errors.NotFoundf("volume not found: %s", *resourceID)
		} else {
			return "", err
		}
	}
	return string(response.Volume.LifecycleState), nil
}

func (v *volumeSource) createVolume(p storage.VolumeParams, instanceMap map[instance.Id]*ociInstance) (_ *storage.Volume, err error) {
	var details ociCore.CreateVolumeResponse
	defer func() {
		if err != nil && details.Id != nil {
			req := ociCore.DeleteVolumeRequest{
				VolumeId: details.Id,
			}
			response, nestedErr := v.api.DeleteVolume(context.Background(), req)
			if nestedErr != nil && !v.env.isNotFound(response.RawResponse) {
				logger.Warningf("failed to cleanup volume: %s", *details.Id)
				return
			}
			nestedErr = v.env.waitForResourceStatus(
				v.getVolumeStatus, details.Id,
				string(ociCore.VolumeLifecycleStateTerminated),
				5*time.Minute)
			if nestedErr != nil && !errors.IsNotFound(nestedErr) {
				logger.Warningf("failed to cleanup volume: %s", *details.Id)
				return
			}
		}
	}()
	if err := v.ValidateVolumeParams(p); err != nil {
		return nil, errors.Trace(err)
	}
	if p.Attachment == nil {
		return nil, errors.Errorf("volume %s has no attachments", p.Tag.String())
	}
	instanceId := p.Attachment.InstanceId
	instance, ok := instanceMap[instanceId]
	if !ok {
		ociInstances, err := v.env.getOciInstances(instanceId)
		if err != nil {
			return nil, errors.Trace(err)
		}
		instance = ociInstances[0]
		instanceMap[instanceId] = instance
	}

	availabilityZone := instance.availabilityZone()
	name := p.Tag.String()
	volTags := p.ResourceTags
	volTags[tags.JujuModel] = v.modelUUID
	size := int(p.Size)
	requestDetails := ociCore.CreateVolumeDetails{
		AvailabilityDomain: &availabilityZone,
		CompartmentId:      v.env.ecfg().compartmentID(),
		DisplayName:        &name,
		SizeInMBs:          &size,
		FreeFormTags:       volTags,
	}

	request := ociCore.CreateVolumeRequest{
		CreateVolumeDetails: requestDetails,
	}

	result, err := v.api.CreateVolume(context.Background(), request)
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = v.env.waitForResourceStatus(
		v.getVolumeStatus, result.Volume.Id,
		string(ociCore.VolumeLifecycleStateAvailable),
		5*time.Minute)
	if err != nil {
		return nil, errors.Trace(err)
	}

	volumeDetails, err := v.api.GetVolume(
		context.Background(), ociCore.GetVolumeRequest{VolumeId: result.Volume.Id})
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &storage.Volume{p.Tag, makeVolumeInfo(volumeDetails.Volume)}, nil
}

func makeVolumeInfo(vol ociCore.Volume) storage.VolumeInfo {
	return storage.VolumeInfo{
		VolumeId:   *vol.Id,
		Size:       uint64(*vol.SizeInMBs),
		Persistent: true,
	}
}

func (v *volumeSource) CreateVolumes(params []storage.VolumeParams) ([]storage.CreateVolumesResult, error) {
	logger.Debugf("Creating volumes: %v", params)
	if params == nil {
		return []storage.CreateVolumesResult{}, nil
	}
	results := make([]storage.CreateVolumesResult, len(params))
	instanceMap := map[instance.Id]*ociInstance{}
	for i, volume := range params {
		vol, err := v.createVolume(volume, instanceMap)
		if err != nil {
			results[i].Error = errors.Trace(err)
			continue
		}
		results[i].Volume = vol
	}
	return results, nil
}

func (v *volumeSource) allVolumes() (map[string]ociCore.Volume, error) {
	result := map[string]ociCore.Volume{}
	request := ociCore.ListVolumesRequest{
		CompartmentId: v.env.ecfg().compartmentID(),
	}
	response, err := v.api.ListVolumes(context.Background(), request)
	if err != nil {
		return nil, err
	}

	for _, val := range response.Items {
		if t, ok := val.FreeFormTags[tags.JujuModel]; !ok {
			continue
		} else {
			if t != "" && t != v.modelUUID {
				continue
			}
		}
		result[*val.Id] = val
	}
	return result, nil
}

func (v *volumeSource) ListVolumes() ([]string, error) {
	ids := []string{}
	volumes, err := v.allVolumes()
	if err != nil {
		return nil, err
	}

	for k, _ := range volumes {
		ids = append(ids, k)
	}
	return ids, nil
}

func (v *volumeSource) DescribeVolumes(volIds []string) ([]storage.DescribeVolumesResult, error) {
	result := make([]storage.DescribeVolumesResult, len(volIds), len(volIds))

	allVolumes, err := v.allVolumes()
	if err != nil {
		return nil, errors.Trace(err)
	}

	for i, val := range volIds {
		if volume, ok := allVolumes[val]; ok {
			volumeInfo := makeVolumeInfo(volume)
			result[i].VolumeInfo = &volumeInfo
		} else {
			result[i].Error = errors.NotFoundf("%s", volume)
		}
	}
	return result, nil
}

func (v *volumeSource) DestroyVolumes(volIds []string) ([]error, error) {
	volumes, err := v.allVolumes()
	if err != nil {
		return nil, errors.Trace(err)
	}

	errs := make([]error, len(volIds))

	for idx, volId := range volIds {
		volumeDetails, ok := volumes[volId]
		if !ok {
			errs[idx] = errors.NotFoundf("no such volume %s", volId)
			continue
		}
		request := ociCore.DeleteVolumeRequest{
			VolumeId: volumeDetails.Id,
		}

		response, err := v.api.DeleteVolume(context.Background(), request)
		if err != nil && !v.env.isNotFound(response.RawResponse) {
			errs[idx] = errors.Trace(err)
			continue
		}
		err = v.env.waitForResourceStatus(
			v.getVolumeStatus, volumeDetails.Id,
			string(ociCore.VolumeLifecycleStateTerminated),
			5*time.Minute)
		if err != nil && !errors.IsNotFound(err) {
			errs[idx] = errors.Trace(err)
		} else {
			errs[idx] = nil
		}
	}
	return errs, nil
}

func (v *volumeSource) ReleaseVolumes(volIds []string) ([]error, error) {
	volumes, err := v.allVolumes()
	if err != nil {
		return nil, errors.Trace(err)
	}

	errs := make([]error, len(volIds))
	tagsToRemove := []string{
		tags.JujuModel,
		tags.JujuController,
	}
	for idx, volId := range volIds {
		volumeDetails, ok := volumes[volId]
		if !ok {
			errs[idx] = errors.NotFoundf("no such volume %s", volId)
			continue
		}
		currentTags := volumeDetails.FreeFormTags
		needsUpdate := false
		for _, tag := range tagsToRemove {
			if _, ok := currentTags[tag]; ok {
				needsUpdate = true
				currentTags[tag] = ""
			}
		}
		if needsUpdate {
			requestDetails := ociCore.UpdateVolumeDetails{
				FreeFormTags: currentTags,
			}
			request := ociCore.UpdateVolumeRequest{
				UpdateVolumeDetails: requestDetails,
				VolumeId:            volumeDetails.Id,
			}

			_, err := v.api.UpdateVolume(context.Background(), request)
			if err != nil {
				errs[idx] = errors.Trace(err)
			} else {
				errs[idx] = nil
			}
		}
	}
	return errs, nil
}

func (v *volumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	size := mibToGib(params.Size)
	if size < minVolumeSizeInGB || size > maxVolumeSizeInGB {
		return errors.Errorf(
			"invalid volume size %d. Valid range is %d - %d (GiB)", size, minVolumeSizeInGB, maxVolumeSizeInGB)
	}
	return nil
}

func (v *volumeSource) volumeAttachments(instanceId instance.Id) ([]ociCore.IScsiVolumeAttachment, error) {
	instId := string(instanceId)
	request := ociCore.ListVolumeAttachmentsRequest{
		CompartmentId: v.env.ecfg().compartmentID(),
		InstanceId:    &instId,
	}
	result, err := v.api.ListVolumeAttachments(context.Background(), request)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ret := make([]ociCore.IScsiVolumeAttachment, len(result.Items))

	for idx, att := range result.Items {
		// The oracle oci client will return a VolumeAttachment type, which is an
		// interface. This is due to the fact that they will at some point support
		// different attachment types. For the moment, there is only iSCSI, as stated
		// in the documentation, at the time of this writing:
		// https://docs.us-phoenix-1.oraclecloud.com/api/#/en/iaas/20160918/requests/AttachVolumeDetails
		//
		// So we need to cast it back to IScsiVolumeAttachment{} to be able to access
		// the connection info we need, and possibly chap secrets to be able to connect
		// to the volume.
		// NOTE: the current client errors out when trying to list attachments. I had to hack
		// it to return IScsiVolumeAttachment{} instead of VolumeAttachment{}.
		// TODO: Remove hack once client is fixed
		baseType, ok := att.(ociCore.IScsiVolumeAttachment)
		if !ok {
			return nil, errors.Errorf("invalid attachment type. Expected iscsi")
		}

		if baseType.LifecycleState == ociCore.VolumeAttachmentLifecycleStateDetached {
			continue
		}
		ret[idx] = baseType
	}
	return ret, nil
}

func makeVolumeAttachmentResult(attachment ociCore.IScsiVolumeAttachment, param storage.VolumeAttachmentParams) (storage.AttachVolumesResult, error) {
	if attachment.Port == nil {
		return storage.AttachVolumesResult{}, errors.Errorf("invalid port")
	}
	port := strconv.Itoa(*attachment.Port)
	plugInfo := &storage.VolumeAttachmentPlugInfo{
		DeviceType: storage.DiskTypeISCSI,
		DeviceAttributes: map[string]string{
			"iqn":     *attachment.Iqn,
			"address": *attachment.Ipv4,
			"port":    port,
		},
	}
	if attachment.ChapSecret != nil && attachment.ChapUsername != nil {
		plugInfo.DeviceAttributes["chap-user"] = *attachment.ChapUsername
		plugInfo.DeviceAttributes["chap-secret"] = *attachment.ChapSecret
	}
	result := storage.AttachVolumesResult{
		VolumeAttachment: &storage.VolumeAttachment{
			param.Volume,
			param.Machine,
			storage.VolumeAttachmentInfo{
				PlugInfo: plugInfo,
			},
		},
	}
	return result, nil
}

func (v *volumeSource) attachVolume(param storage.VolumeAttachmentParams) (_ storage.AttachVolumesResult, err error) {
	var details ociCore.AttachVolumeResponse
	defer func() {
		volAttach := details.VolumeAttachment
		if err != nil && volAttach.GetId() != nil {
			req := ociCore.DetachVolumeRequest{
				VolumeAttachmentId: volAttach.GetId(),
			}
			_, nestedErr := v.api.DetachVolume(context.Background(), req)
			if nestedErr != nil {
				logger.Warningf("failed to cleanup volume attachment: %v", volAttach.GetId())
				return
			}
			nestedErr = v.env.waitForResourceStatus(
				v.getAttachmentStatus, volAttach.GetId(),
				string(ociCore.VolumeAttachmentLifecycleStateDetached),
				5*time.Minute)
			if nestedErr != nil && !errors.IsNotFound(nestedErr) {
				logger.Warningf("failed to cleanup volume attachment: %v", volAttach.GetId())
				return
			}
		}
	}()

	instances, err := v.env.getOciInstances(param.InstanceId)
	if err != nil {
		return storage.AttachVolumesResult{}, errors.Trace(err)
	}
	if len(instances) != 1 {
		return storage.AttachVolumesResult{}, errors.Errorf("expected 1 instance, got %d", len(instances))
	}
	instance := instances[0]
	if instance.raw.LifecycleState == ociCore.InstanceLifecycleStateTerminated || instance.raw.LifecycleState == ociCore.InstanceLifecycleStateTerminating {
		return storage.AttachVolumesResult{}, errors.Errorf("invalid instance state for volume attachment: %s", instance.Status())
	}

	if err := instance.waitForMachineStatus(
		ociCore.InstanceLifecycleStateRunning,
		5*time.Minute); err != nil {

		return storage.AttachVolumesResult{}, errors.Trace(err)
	}

	volumeAttachments, err := v.volumeAttachments(param.InstanceId)
	if err != nil {
		return storage.AttachVolumesResult{}, errors.Trace(err)
	}

	for _, val := range volumeAttachments {
		if val.VolumeId == nil || val.InstanceId == nil {
			continue
		}
		if *val.VolumeId == param.VolumeId && *val.InstanceId == string(param.InstanceId) {
			// Volume already attached. Return info.
			return makeVolumeAttachmentResult(val, param)
		}
	}

	instID := string(param.InstanceId)
	useChap := true
	displayName := fmt.Sprintf("%s_%s", instID, param.VolumeId)
	attachDetails := ociCore.AttachIScsiVolumeDetails{
		InstanceId:  &instID,
		VolumeId:    &param.VolumeId,
		UseChap:     &useChap,
		DisplayName: &displayName,
	}
	request := ociCore.AttachVolumeRequest{
		AttachVolumeDetails: attachDetails,
	}

	details, err = v.api.AttachVolume(context.Background(), request)
	if err != nil {
		return storage.AttachVolumesResult{}, errors.Trace(err)
	}

	err = v.env.waitForResourceStatus(
		v.getAttachmentStatus, details.VolumeAttachment.GetId(),
		string(ociCore.VolumeAttachmentLifecycleStateAttached),
		5*time.Minute)
	if err != nil {
		return storage.AttachVolumesResult{}, errors.Trace(err)
	}

	detailsReq := ociCore.GetVolumeAttachmentRequest{
		VolumeAttachmentId: details.VolumeAttachment.GetId(),
	}

	response, err := v.api.GetVolumeAttachment(context.Background(), detailsReq)
	if err != nil {
		return storage.AttachVolumesResult{}, errors.Trace(err)
	}

	baseType, ok := response.VolumeAttachment.(ociCore.IScsiVolumeAttachment)
	if !ok {
		return storage.AttachVolumesResult{}, errors.Errorf("invalid attachment type. Expected iscsi")
	}

	return makeVolumeAttachmentResult(baseType, param)
}

func (v *volumeSource) getAttachmentStatus(resourceID *string) (string, error) {
	request := ociCore.GetVolumeAttachmentRequest{
		VolumeAttachmentId: resourceID,
	}

	response, err := v.api.GetVolumeAttachment(context.Background(), request)
	if err != nil {
		if v.env.isNotFound(response.RawResponse) {
			return "", errors.NotFoundf("volume attachment not found: %s", *resourceID)
		} else {
			return "", err
		}
	}
	return string(response.VolumeAttachment.GetLifecycleState()), nil
}

func (v *volumeSource) AttachVolumes(params []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
	instanceIds := []instance.Id{}
	for _, val := range params {
		instanceIds = append(instanceIds, val.InstanceId)
	}
	if len(instanceIds) == 0 {
		return []storage.AttachVolumesResult{}, nil
	}
	instancesAsMap, err := v.env.getOciInstancesAsMap(instanceIds...)
	if err != nil {
		return []storage.AttachVolumesResult{}, errors.Trace(err)
	}

	ret := make([]storage.AttachVolumesResult, len(params))
	for idx, volParam := range params {
		_, ok := instancesAsMap[volParam.InstanceId]
		if !ok {
			// this really should not happen, given how getOciInstancesAsMap()
			// works
			ret[idx].Error = errors.NotFoundf("instance %q was not found", volParam.InstanceId)
			continue
		}

		result, err := v.attachVolume(volParam)
		if err != nil {
			ret[idx].Error = errors.Trace(err)
		} else {
			ret[idx] = result
		}
	}
	return ret, nil
}

func (v *volumeSource) DetachVolumes(params []storage.VolumeAttachmentParams) ([]error, error) {
	ret := make([]error, len(params))
	instanceAttachmentMap := map[instance.Id][]ociCore.IScsiVolumeAttachment{}
	for idx, param := range params {
		instAtt, ok := instanceAttachmentMap[param.InstanceId]
		if !ok {
			currentAttachments, err := v.volumeAttachments(param.InstanceId)
			if err != nil {
				ret[idx] = errors.Trace(err)
				continue
			}
			instAtt = currentAttachments
			instanceAttachmentMap[param.InstanceId] = instAtt
		}
		for _, attachment := range instAtt {
			logger.Tracef("volume ID is: %v", attachment.VolumeId)
			if attachment.VolumeId != nil && param.VolumeId == *attachment.VolumeId && attachment.LifecycleState != ociCore.VolumeAttachmentLifecycleStateDetached {
				if attachment.LifecycleState != ociCore.VolumeAttachmentLifecycleStateDetaching {
					request := ociCore.DetachVolumeRequest{
						VolumeAttachmentId: attachment.Id,
					}

					_, err := v.api.DetachVolume(context.Background(), request)
					if err != nil {
						ret[idx] = errors.Trace(err)
						break
					}
				}
				err := v.env.waitForResourceStatus(
					v.getAttachmentStatus, attachment.Id,
					string(ociCore.VolumeAttachmentLifecycleStateDetached),
					5*time.Minute)
				if err != nil && !errors.IsNotFound(err) {
					ret[idx] = errors.Trace(err)
					logger.Warningf("failed to detach volume: %s", *attachment.Id)
				} else {
					ret[idx] = nil
				}
			}
		}
	}
	return ret, nil
}
