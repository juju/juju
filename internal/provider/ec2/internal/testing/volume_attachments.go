// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// AttachVolume implements ec2.Client.
func (srv *Server) AttachVolume(ctx context.Context, in *ec2.AttachVolumeInput, opts ...func(*ec2.Options)) (*ec2.AttachVolumeOutput, error) {
	srv.volumeMutatingCalls.next()

	if err, ok := srv.apiCallErrors["AttachVolume"]; ok {
		return nil, err
	}

	attachment := types.VolumeAttachment{}

	volId := aws.ToString(in.VolumeId)
	vol, err := srv.volume(volId)
	if err != nil {
		return nil, err
	}
	if vol.State != "available" {
		return nil, apiError(" IncorrectState", "cannot attach volume that is not available: %v", volId)
	}
	attachment.VolumeId = in.VolumeId

	instId := aws.ToString(in.InstanceId)
	inst, err := srv.instance(instId)
	if err != nil {
		return nil, err
	}
	if inst.state != Running {
		return nil, apiError("IncorrectInstanceState", "cannot attach volume to instance %s as it is not running", instId)
	}
	attachment.InstanceId = in.InstanceId

	attachment.Device = in.Device

	volZone := aws.ToString(vol.AvailabilityZone)
	if volZone != inst.availZone {
		return nil, apiError(
			"InvalidVolume.ZoneMismatch",
			"volume availability zone %q must match instance zone %q", volZone, inst.availZone,
		)
	}

	attachId := volId
	if _, ok := srv.volumeAttachments[attachId]; ok {
		return nil, apiError("VolumeInUse", "Volume %s is already attached", aws.ToString(in.VolumeId))
	}
	v, err := srv.volume(attachId)
	if err != nil {
		return nil, err
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	va := &volumeAttachment{attachment}
	va.State = "attached"
	v.State = "in-use"
	srv.volumeAttachments[aws.ToString(va.VolumeId)] = va

	resp := &ec2.AttachVolumeOutput{}
	resp.VolumeId = va.VolumeId
	resp.InstanceId = va.InstanceId
	resp.Device = va.Device
	resp.State = "attached"
	resp.AttachTime = aws.Time(time.Now())
	return resp, nil
}

// DetachVolume implements ec2.Client.
func (srv *Server) DetachVolume(ctx context.Context, in *ec2.DetachVolumeInput, opts ...func(*ec2.Options)) (*ec2.DetachVolumeOutput, error) {
	srv.volumeMutatingCalls.next()

	if err, ok := srv.apiCallErrors["DetachVolume"]; ok {
		return nil, err
	}

	volId := aws.ToString(in.VolumeId)
	// Get attachment first so if not found, the expected error is returned.
	va, err := srv.volumeAttachment(volId)
	if err != nil {
		return nil, err
	}
	// Validate volume exists.
	v, err := srv.volume(volId)
	if err != nil {
		return nil, err
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	delete(srv.volumeAttachments, volId)
	v.State = "available"

	resp := &ec2.DetachVolumeOutput{}
	resp.VolumeId = va.VolumeId
	resp.InstanceId = va.InstanceId
	resp.Device = va.Device
	resp.State = "detaching"
	return resp, nil
}

type volumeAttachment struct {
	types.VolumeAttachment
}

func (srv *Server) volumeAttachment(id string) (*volumeAttachment, error) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	v, found := srv.volumeAttachments[id]
	if !found {
		return nil, apiError("InvalidAttachment.NotFound", "Volume attachment for volume %s not found", id)
	}
	return v, nil
}
