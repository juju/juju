// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func (srv *Server) createBlockDeviceMappingsOnRun(mappings []types.BlockDeviceMapping) []types.InstanceBlockDeviceMapping {
	results := make([]types.InstanceBlockDeviceMapping, 0, len(mappings))
	for _, mapping := range mappings {
		if aws.ToString(mapping.VirtualName) != "" {
			// ephemeral block devices are attached, but do not
			// show up in block device mappings in responses.
			continue
		}
		results = append(results, types.InstanceBlockDeviceMapping{
			DeviceName: mapping.DeviceName,
			Ebs: &types.EbsInstanceBlockDevice{
				VolumeId:            aws.String(fmt.Sprintf("vol-%v", srv.volumeId.next())),
				AttachTime:          aws.Time(time.Now()),
				Status:              "attached",
				DeleteOnTermination: mapping.Ebs.DeleteOnTermination,
			},
		})
	}
	return results
}
