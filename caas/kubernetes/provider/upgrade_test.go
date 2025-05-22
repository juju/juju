// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"testing"

	"github.com/juju/tc"
	core "k8s.io/api/core/v1"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
)

type UpgraderSuite struct {
}

func TestUpgraderSuite(t *testing.T) {
	tc.Run(t, &UpgraderSuite{})
}

func (u *UpgraderSuite) TestUpgradePodTemplateSpec(c *tc.C) {
	tests := []struct {
		ExpectedPodTemplateSpec core.PodTemplateSpec
		PodTemplateSpec         core.PodTemplateSpec
		ImagePath               string
		Version                 semversion.Number
	}{
		{
			ExpectedPodTemplateSpec: core.PodTemplateSpec{
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Image: fmt.Sprintf("%s/%s:2.6.7", podcfg.JujudOCINamespace, podcfg.JujudOCIName),
						},
					},
				},
			},
			PodTemplateSpec: core.PodTemplateSpec{
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Image: fmt.Sprintf("%s/%s:2.6.6", podcfg.JujudOCINamespace, podcfg.JujudOCIName),
						},
					},
				},
			},
			Version: semversion.MustParse("2.6.7"),
		},
	}

	for _, test := range tests {
		containers, err := upgradePodTemplateSpec(test.PodTemplateSpec.Spec.Containers, test.ImagePath, test.Version)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(test.ExpectedPodTemplateSpec.Spec.Containers[0].Image, tc.Equals, containers[0].Image)
	}
}
