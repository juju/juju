// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	ociCore "github.com/oracle/oci-go-sdk/v65/core"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/storage"
)

func mibToGib(m uint64) uint64 {
	return (m + 1023) / 1024
}

type volumeSource struct {
	env        *Environ
	envName    string
	modelUUID  string
	storageAPI StorageClient
	computeAPI ComputeClient
	clock      clock.Clock
}

var _ storage.VolumeSource = (*volumeSource)(nil)

func (v *volumeSource) getVolumeStatus(resourceID *string) (string, error) {
	request := ociCore.GetVolumeRequest{
		VolumeId: resourceID,
	}

	response, err := v.storageAPI.GetVolume(context.Background(), request)
	if err != nil {
		if v.env.isNotFound(response.RawResponse) {
			return "", errors.NotFoundf("volume not found: %s", *resourceID)
		} else {
			return "", err
		}
	}
	return string(response.Volume.LifecycleState), nil
}

func (v *volumeSource) createVolume(ctx envcontext.ProviderCallContext, p storage.VolumeParams, instanceMap map[instance.Id]*ociInstance) (_ *storage.Volume, err error) {
	var details ociCore.CreateVolumeResponse
	defer func() {
		if err != nil && details.Id != nil {
			req := ociCore.DeleteVolumeRequest{
				VolumeId: details.Id,
			}
			response, nestedErr := v.storageAPI.DeleteVolume(context.Background(), req)
			if nestedErr != nil && !v.env.isNotFound(response.RawResponse) {
				logger.Warningf(ctx, "failed to cleanup volume: %s", *details.Id)
				return
			}
			nestedErr = v.env.waitForResourceStatus(
				v.getVolumeStatus, details.Id,
				string(ociCore.VolumeLifecycleStateTerminated),
				5*time.Minute)
			if nestedErr != nil && !errors.Is(nestedErr, errors.NotFound) {
				logger.Warningf(ctx, "failed to cleanup volume: %s", *details.Id)
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
	inst, ok := instanceMap[instanceId]
	if !ok {
		ociInstances, err := v.env.getOciInstances(ctx, instanceId)
		if err != nil {
			return nil, v.env.HandleCredentialError(ctx, err)
		}
		inst = ociInstances[0]
		instanceMap[instanceId] = inst
	}

	availabilityZone := inst.availabilityZone()
	name := p.Tag.String()

	volTags := map[string]string{}
	if p.ResourceTags != nil {
		volTags = p.ResourceTags
	}
	volTags[tags.JujuModel] = v.modelUUID

	size := int64(p.Size)
	requestDetails := ociCore.CreateVolumeDetails{
		AvailabilityDomain: &availabilityZone,
		CompartmentId:      v.env.ecfg().compartmentID(),
		DisplayName:        &name,
		SizeInMBs:          &size,
		FreeformTags:       volTags,
	}

	request := ociCore.CreateVolumeRequest{
		CreateVolumeDetails: requestDetails,
	}

	result, err := v.storageAPI.CreateVolume(context.Background(), request)
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

	volumeDetails, err := v.storageAPI.GetVolume(
		context.Background(), ociCore.GetVolumeRequest{VolumeId: result.Volume.Id})
	if err != nil {
		return nil, v.env.HandleCredentialError(ctx, err)
	}

	return &storage.Volume{Tag: p.Tag, VolumeInfo: makeVolumeInfo(volumeDetails.Volume)}, nil
}

func makeVolumeInfo(vol ociCore.Volume) storage.VolumeInfo {
	var size uint64
	if vol.SizeInMBs != nil {
		size = uint64(*vol.SizeInMBs)
	} else if vol.SizeInGBs != nil {
		size = uint64(*vol.SizeInGBs * 1024)
	}

	return storage.VolumeInfo{
		VolumeId:   *vol.Id,
		Size:       size,
		Persistent: true,
	}
}

func (v *volumeSource) CreateVolumes(ctx envcontext.ProviderCallContext, params []storage.VolumeParams) ([]storage.CreateVolumesResult, error) {
	logger.Debugf(ctx, "Creating volumes: %v", params)
	if params == nil {
		return []storage.CreateVolumesResult{}, nil
	}
	var credErr error

	results := make([]storage.CreateVolumesResult, len(params))
	instanceMap := map[instance.Id]*ociInstance{}
	for i, volume := range params {
		if credErr != nil {
			results[i].Error = errors.Trace(credErr)
			continue
		}
		vol, err := v.createVolume(ctx, volume, instanceMap)
		if err != nil {
			if denied, maybeCredErr := v.env.MaybeInvalidateCredentialError(ctx, err); denied {
				credErr = maybeCredErr
			}
			results[i].Error = errors.Trace(err)
			continue
		}
		results[i].Volume = vol
	}
	return results, nil
}

func (v *volumeSource) allVolumes() (map[string]ociCore.Volume, error) {
	result := map[string]ociCore.Volume{}
	volumes, err := v.storageAPI.ListVolumes(context.Background(), v.env.ecfg().compartmentID())
	if err != nil {
		return nil, err
	}

	for _, val := range volumes {
		if t, ok := val.FreeformTags[tags.JujuModel]; !ok {
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

func (v *volumeSource) ListVolumes(ctx envcontext.ProviderCallContext) ([]string, error) {
	var ids []string
	volumes, err := v.allVolumes()
	if err != nil {
		return nil, v.env.HandleCredentialError(ctx, err)
	}

	for k := range volumes {
		ids = append(ids, k)
	}
	return ids, nil
}

func (v *volumeSource) DescribeVolumes(ctx envcontext.ProviderCallContext, volIds []string) ([]storage.DescribeVolumesResult, error) {
	result := make([]storage.DescribeVolumesResult, len(volIds), len(volIds))

	allVolumes, err := v.allVolumes()
	if err != nil {
		return nil, v.env.HandleCredentialError(ctx, err)
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

func (v *volumeSource) DestroyVolumes(ctx envcontext.ProviderCallContext, volIds []string) ([]error, error) {
	volumes, err := v.allVolumes()
	if err != nil {
		return nil, v.env.HandleCredentialError(ctx, err)
	}

	var credErr error
	errs := make([]error, len(volIds))

	for idx, volId := range volIds {
		if credErr != nil {
			errs[idx] = errors.Trace(credErr)
			continue
		}
		volumeDetails, ok := volumes[volId]
		if !ok {
			errs[idx] = errors.NotFoundf("no such volume %s", volId)
			continue
		}
		request := ociCore.DeleteVolumeRequest{
			VolumeId: volumeDetails.Id,
		}

		response, err := v.storageAPI.DeleteVolume(context.Background(), request)
		if err != nil && !v.env.isNotFound(response.RawResponse) {
			if denied, maybeCredErr := v.env.MaybeInvalidateCredentialError(ctx, err); denied {
				credErr = maybeCredErr
			}
			errs[idx] = errors.Trace(err)
			continue
		}
		err = v.env.waitForResourceStatus(
			v.getVolumeStatus, volumeDetails.Id,
			string(ociCore.VolumeLifecycleStateTerminated),
			5*time.Minute)
		if err != nil && !errors.Is(err, errors.NotFound) {
			if denied, maybeCredErr := v.env.MaybeInvalidateCredentialError(ctx, err); denied {
				credErr = maybeCredErr
			}
			errs[idx] = errors.Trace(err)
		} else {
			errs[idx] = nil
		}
	}
	return errs, nil
}

func (v *volumeSource) ReleaseVolumes(ctx envcontext.ProviderCallContext, volIds []string) ([]error, error) {
	volumes, err := v.allVolumes()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var credErr error
	errs := make([]error, len(volIds))
	tagsToRemove := []string{
		tags.JujuModel,
		tags.JujuController,
	}
	for idx, volId := range volIds {
		if credErr != nil {
			errs[idx] = errors.Trace(credErr)
			continue
		}
		volumeDetails, ok := volumes[volId]
		if !ok {
			errs[idx] = errors.NotFoundf("no such volume %s", volId)
			continue
		}
		currentTags := volumeDetails.FreeformTags
		needsUpdate := false
		for _, tag := range tagsToRemove {
			if _, ok := currentTags[tag]; ok {
				needsUpdate = true
				currentTags[tag] = ""
			}
		}
		if needsUpdate {
			requestDetails := ociCore.UpdateVolumeDetails{
				FreeformTags: currentTags,
			}
			request := ociCore.UpdateVolumeRequest{
				UpdateVolumeDetails: requestDetails,
				VolumeId:            volumeDetails.Id,
			}

			_, err := v.storageAPI.UpdateVolume(context.Background(), request)
			if err != nil {
				if denied, maybeCredErr := v.env.MaybeInvalidateCredentialError(ctx, err); denied {
					credErr = maybeCredErr
				}
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
	instId := instanceId.String()

	attachments, err := v.computeAPI.ListVolumeAttachments(context.Background(), v.env.ecfg().compartmentID(), &instId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ret := make([]ociCore.IScsiVolumeAttachment, len(attachments))

	for idx, att := range attachments {
		// The oracle oci client will return a VolumeAttachment type, which is an
		// interface. This is due to the fact that they will at some point support
		// different attachment types. For the moment, there is only iSCSI, as stated
		// in the documentation, at the time of this writing:
		// https://docs.us-phoenix-1.oraclecloud.com/api/#/en/iaas/20160918/requests/AttachVolumeDetails
		//
		// So we need to cast it back to IScsiVolumeAttachment{} to be able to access
		// the connection info we need, and possibly chap secrets to be able to connect
		// to the volume.
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
	if attachment.Port == nil || attachment.Iqn == nil {
		return storage.AttachVolumesResult{}, errors.Errorf("invalid attachment info")
	}
	port := strconv.Itoa(*attachment.Port)
	planInfo := &storage.VolumeAttachmentPlanInfo{
		DeviceType: storage.DeviceTypeISCSI,
		DeviceAttributes: map[string]string{
			"iqn":     *attachment.Iqn,
			"address": *attachment.Ipv4,
			"port":    port,
		},
	}
	if attachment.ChapSecret != nil && attachment.ChapUsername != nil {
		planInfo.DeviceAttributes["chap-user"] = *attachment.ChapUsername
		planInfo.DeviceAttributes["chap-secret"] = *attachment.ChapSecret
	}
	result := storage.AttachVolumesResult{
		VolumeAttachment: &storage.VolumeAttachment{
			Volume:  param.Volume,
			Machine: param.Machine,
			VolumeAttachmentInfo: storage.VolumeAttachmentInfo{
				PlanInfo: planInfo,
			},
		},
	}
	return result, nil
}

func (v *volumeSource) attachVolume(ctx envcontext.ProviderCallContext, param storage.VolumeAttachmentParams) (_ storage.AttachVolumesResult, err error) {
	var details ociCore.AttachVolumeResponse
	defer func() {
		volAttach := details.VolumeAttachment
		if volAttach != nil && err != nil && volAttach.GetId() != nil {
			req := ociCore.DetachVolumeRequest{
				VolumeAttachmentId: volAttach.GetId(),
			}
			res, nestedErr := v.computeAPI.DetachVolume(context.Background(), req)
			if nestedErr != nil && !v.env.isNotFound(res.RawResponse) {
				logger.Warningf(ctx, "failed to cleanup volume attachment: %v", volAttach.GetId())
				return
			}
			nestedErr = v.env.waitForResourceStatus(
				v.getAttachmentStatus, volAttach.GetId(),
				string(ociCore.VolumeAttachmentLifecycleStateDetached),
				5*time.Minute)
			if nestedErr != nil && !errors.Is(nestedErr, errors.NotFound) {
				logger.Warningf(ctx, "failed to cleanup volume attachment: %v", volAttach.GetId())
				return
			}
		}
	}()

	instances, err := v.env.getOciInstances(ctx, param.InstanceId)
	if err != nil {
		return storage.AttachVolumesResult{}, v.env.HandleCredentialError(ctx, err)
	}
	if len(instances) != 1 {
		return storage.AttachVolumesResult{}, errors.Errorf("expected 1 instance, got %d", len(instances))
	}
	inst := instances[0]
	if inst.raw.LifecycleState == ociCore.InstanceLifecycleStateTerminated || inst.raw.LifecycleState == ociCore.InstanceLifecycleStateTerminating {
		return storage.AttachVolumesResult{}, errors.Errorf("invalid instance state for volume attachment: %s", inst.raw.LifecycleState)
	}

	if err := inst.waitForMachineStatus(
		ociCore.InstanceLifecycleStateRunning,
		5*time.Minute); err != nil {
		return storage.AttachVolumesResult{}, errors.Trace(err)
	}

	volumeAttachments, err := v.volumeAttachments(param.InstanceId)
	if err != nil {
		return storage.AttachVolumesResult{}, v.env.HandleCredentialError(ctx, err)
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

	details, err = v.computeAPI.AttachVolume(context.Background(), request)
	if err != nil {
		return storage.AttachVolumesResult{}, v.env.HandleCredentialError(ctx, err)
	}

	err = v.env.waitForResourceStatus(
		v.getAttachmentStatus, details.VolumeAttachment.GetId(),
		string(ociCore.VolumeAttachmentLifecycleStateAttached),
		5*time.Minute)
	if err != nil {
		return storage.AttachVolumesResult{}, v.env.HandleCredentialError(ctx, err)
	}

	detailsReq := ociCore.GetVolumeAttachmentRequest{
		VolumeAttachmentId: details.VolumeAttachment.GetId(),
	}

	response, err := v.computeAPI.GetVolumeAttachment(context.Background(), detailsReq)
	if err != nil {
		return storage.AttachVolumesResult{}, v.env.HandleCredentialError(ctx, err)
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

	response, err := v.computeAPI.GetVolumeAttachment(context.Background(), request)
	if err != nil {
		if v.env.isNotFound(response.RawResponse) {
			return "", errors.NotFoundf("volume attachment not found: %s", *resourceID)
		} else {
			return "", err
		}
	}
	return string(response.VolumeAttachment.GetLifecycleState()), nil
}

func (v *volumeSource) AttachVolumes(ctx envcontext.ProviderCallContext, params []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
	var instanceIds []instance.Id
	for _, val := range params {
		instanceIds = append(instanceIds, val.InstanceId)
	}
	if len(instanceIds) == 0 {
		return []storage.AttachVolumesResult{}, nil
	}
	ret := make([]storage.AttachVolumesResult, len(params))
	instancesAsMap, err := v.env.getOciInstancesAsMap(ctx, instanceIds...)
	if err != nil {
		if denied, maybeCredErr := v.env.MaybeInvalidateCredentialError(ctx, err); denied {
			// Exit out early to improve readability on handling credential
			// errors.
			for idx := range params {
				ret[idx].Error = maybeCredErr
			}
			return ret, nil
		}
		return []storage.AttachVolumesResult{}, errors.Trace(err)
	}

	for idx, volParam := range params {
		_, ok := instancesAsMap[volParam.InstanceId]
		if !ok {
			// this really should not happen, given how getOciInstancesAsMap()
			// works
			ret[idx].Error = errors.NotFoundf("instance %q was not found", volParam.InstanceId)
			continue
		}

		result, err := v.attachVolume(ctx, volParam)
		if err != nil {
			ret[idx].Error = v.env.HandleCredentialError(ctx, err)
		} else {
			ret[idx] = result
		}
	}
	return ret, nil
}

func (v *volumeSource) DetachVolumes(ctx envcontext.ProviderCallContext, params []storage.VolumeAttachmentParams) ([]error, error) {
	var credErr error
	ret := make([]error, len(params))
	instanceAttachmentMap := map[instance.Id][]ociCore.IScsiVolumeAttachment{}

	for idx, param := range params {
		if credErr != nil {
			ret[idx] = errors.Trace(credErr)
			continue
		}

		instAtt, ok := instanceAttachmentMap[param.InstanceId]
		if !ok {
			currentAttachments, err := v.volumeAttachments(param.InstanceId)
			if err != nil {
				if denied, maybeCredErr := v.env.MaybeInvalidateCredentialError(ctx, err); denied {
					credErr = maybeCredErr
				}
				ret[idx] = errors.Trace(err)
				continue
			}
			instAtt = currentAttachments
			instanceAttachmentMap[param.InstanceId] = instAtt
		}
		for _, attachment := range instAtt {
			if credErr != nil {
				ret[idx] = errors.Trace(credErr)
				continue
			}
			logger.Tracef(ctx, "volume ID is: %v", attachment.VolumeId)
			if attachment.VolumeId != nil && param.VolumeId == *attachment.VolumeId && attachment.LifecycleState != ociCore.VolumeAttachmentLifecycleStateDetached {
				if attachment.LifecycleState != ociCore.VolumeAttachmentLifecycleStateDetaching {
					request := ociCore.DetachVolumeRequest{
						VolumeAttachmentId: attachment.Id,
					}

					res, err := v.computeAPI.DetachVolume(context.Background(), request)
					if err != nil && !v.env.isNotFound(res.RawResponse) {
						if denied, maybeCredErr := v.env.MaybeInvalidateCredentialError(ctx, err); denied {
							credErr = maybeCredErr
						}
						ret[idx] = errors.Trace(err)
						break
					}
				}
				err := v.env.waitForResourceStatus(
					v.getAttachmentStatus, attachment.Id,
					string(ociCore.VolumeAttachmentLifecycleStateDetached),
					5*time.Minute)
				if err != nil && !errors.Is(err, errors.NotFound) {
					if denied, maybeCredErr := v.env.MaybeInvalidateCredentialError(ctx, err); denied {
						credErr = maybeCredErr
					}
					ret[idx] = errors.Trace(err)
					logger.Warningf(ctx, "failed to detach volume: %s", *attachment.Id)
				} else {
					ret[idx] = nil
				}
			}
		}
	}
	return ret, nil
}
