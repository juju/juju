// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"

	"github.com/juju/juju/cloudconfig/podcfg"
)

type UpgraderSuite struct {
}

var _ = gc.Suite(&UpgraderSuite{})

func (u *UpgraderSuite) TestUpgradePodTemplateSpec(c *gc.C) {
	tests := []struct {
		ExpectedPodTemplateSpec core.PodTemplateSpec
		PodTemplateSpec         core.PodTemplateSpec
		ImagePath               string
		Version                 version.Number
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
			Version: version.MustParse("2.6.7"),
		},
	}

	for _, test := range tests {
		podTempl, err := upgradePodTemplateSpec(&test.PodTemplateSpec, test.ImagePath, test.Version)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(test.ExpectedPodTemplateSpec.Spec.Containers[0].Image, gc.Equals, podTempl.Spec.Containers[0].Image)
	}
}
