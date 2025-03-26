// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"

	"github.com/juju/juju/internal/cloudconfig/podcfg"
	"github.com/juju/juju/internal/version"
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
		containers, err := upgradePodTemplateSpec(test.PodTemplateSpec.Spec.Containers, test.ImagePath, test.Version)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(test.ExpectedPodTemplateSpec.Spec.Containers[0].Image, gc.Equals, containers[0].Image)
	}
}
