// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/juju/collections/set"
)

// CreateVolume implements ec2.Client.
func (srv *Server) CreateVolume(ctx context.Context, in *ec2.CreateVolumeInput, opts ...func(*ec2.Options)) (*ec2.CreateVolumeOutput, error) {
	srv.volumeMutatingCalls.next()

	if err, ok := srv.apiCallErrors["CreateVolume"]; ok {
		return nil, err
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	volume := srv.newVolume("magnetic", 1, in.TagSpecifications)
	volume.AvailabilityZone = in.AvailabilityZone
	if in.VolumeType != "" {
		volume.VolumeType = in.VolumeType
	}
	if in.Size != nil {
		volume.Size = in.Size
	}
	volume.Encrypted = in.Encrypted
	volume.Iops = in.Iops
	volume.KmsKeyId = in.KmsKeyId
	volume.Throughput = in.Throughput

	return &ec2.CreateVolumeOutput{
		Attachments:      nil,
		AvailabilityZone: volume.AvailabilityZone,
		Encrypted:        volume.Encrypted,
		Iops:             volume.Iops,
		Size:             volume.Size,
		State:            volume.State,
		Tags:             volume.Tags,
		VolumeId:         volume.VolumeId,
		VolumeType:       volume.VolumeType,
		KmsKeyId:         volume.KmsKeyId,
		Throughput:       volume.Throughput,
	}, nil
}

// DeleteVolume implements ec2.Client.
func (srv *Server) DeleteVolume(ctx context.Context, in *ec2.DeleteVolumeInput, opts ...func(*ec2.Options)) (*ec2.DeleteVolumeOutput, error) {
	srv.volumeMutatingCalls.next()

	if err, ok := srv.apiCallErrors["DeleteVolume"]; ok {
		return nil, err
	}

	v, err := srv.volume(aws.ToString(in.VolumeId))
	if err != nil {
		return nil, err
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()

	volId := aws.ToString(v.VolumeId)
	if _, ok := srv.volumeAttachments[volId]; ok {
		return nil, apiError("VolumeInUse", "Volume %s is attached", volId)
	}
	delete(srv.volumes, volId)
	return &ec2.DeleteVolumeOutput{}, nil
}

// DescribeVolumes implements ec2.Client.
func (srv *Server) DescribeVolumes(ctx context.Context, in *ec2.DescribeVolumesInput, opts ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error) {
	if err, ok := srv.apiCallErrors["DescribeVolumes"]; ok {
		return nil, err
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()

	var f ec2filter
	idSet := set.NewStrings()
	if in != nil {
		f = in.Filters
		idSet = set.NewStrings(in.VolumeIds...)
	}

	result := &ec2.DescribeVolumesOutput{}
	for _, v := range srv.volumes {
		volId := aws.ToString(v.VolumeId)
		ok, err := f.ok(v)
		if ok && (len(idSet) == 0 || idSet.Contains(volId)) {
			vol := v.Volume
			if va, ok := srv.volumeAttachments[volId]; ok {
				vol.Attachments = []types.VolumeAttachment{va.VolumeAttachment}
			}
			result.Volumes = append(result.Volumes, vol)
		} else if err != nil {
			return nil, apiError("InvalidParameterValue", "describe Volumes: %v", err)
		}
	}
	if f, ok := srv.apiCallModifiers["DescribeVolumes"]; ok {
		if len(f) > 0 {
			f[0](result)
			srv.apiCallModifiers["DescribeVolumes"] = f[1:]
		}
	}
	return result, nil
}

// SetCreateRootDisks records whether or not the server should create
// root disks for each instance created. It defaults to false.
func (srv *Server) SetCreateRootDisks(create bool) {
	srv.mu.Lock()
	srv.createRootDisks = create
	srv.mu.Unlock()
}

func (srv *Server) newVolume(
	volumeType types.VolumeType,
	size int32,
	tagSpecs []types.TagSpecification,
) *volume {
	// Create a volume and volume attachment too.
	volume := &volume{}
	volume.VolumeId = aws.String(fmt.Sprintf("vol-%d", srv.volumeId.next()))
	volume.State = "available"
	volume.CreateTime = aws.Time(time.Now())
	volume.VolumeType = volumeType
	volume.Size = aws.Int32(size)
	volume.Tags = tagSpecForType(types.ResourceTypeVolume, tagSpecs).Tags
	srv.volumes[aws.ToString(volume.VolumeId)] = volume
	return volume
}

type volume struct {
	types.Volume
}

func (v *volume) matchAttr(attr, value string) (ok bool, err error) {
	if strings.HasPrefix(attr, "attachment.") {
		return false, fmt.Errorf("%q filter is not implemented", attr)
	}
	if strings.HasPrefix(attr, "tag:") {
		key := attr[len("tag:"):]
		return matchTag(v.Tags, key, value), nil
	}
	switch attr {
	case "volume-type":
		return string(v.VolumeType) == value, nil
	case "status":
		return string(v.State) == value, nil
	case "volume-id":
		return aws.ToString(v.VolumeId) == value, nil
	case "size":
		size, err := strconv.Atoi(value)
		if err != nil {
			return false, err
		}
		return aws.ToInt32(v.Size) == int32(size), nil
	case "availability-zone":
		return aws.ToString(v.AvailabilityZone) == value, nil
	case "snapshot-id":
		return aws.ToString(v.SnapshotId) == value, nil
	case "encrypted":
		encrypted, err := strconv.ParseBool(value)
		if err != nil {
			return false, err
		}
		return aws.ToBool(v.Encrypted) == encrypted, nil
	case "tag", "tag-key", "tag-value", "create-time":
		return false, fmt.Errorf("%q filter is not implemented", attr)
	}
	return false, fmt.Errorf("unknown attribute %q", attr)
}

func (srv *Server) volume(id string) (*volume, error) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	v, found := srv.volumes[id]
	if !found {
		return nil, apiError("InvalidVolume.NotFound", "Volume %s not found", id)
	}
	return v, nil
}
