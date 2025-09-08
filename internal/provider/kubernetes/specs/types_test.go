// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	k8sspces "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/caas/kubernetes/provider/specs/mocks"
	"github.com/juju/juju/caas/specs"
	"github.com/juju/juju/testing"
)

type typesSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&typesSuite{})

func (s *typesSuite) TestParsePodSpec(c *gc.C) {

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	converter := mocks.NewMockPodSpecConverter(ctrl)
	getParser := func(specVersion specs.Version) (k8sspces.ParserType, error) {
		return func(string) (k8sspces.PodSpecConverter, error) {
			return converter, nil
		}, nil
	}

	minSpecs := &specs.PodSpec{}
	minSpecs.Version = specs.CurrentVersion
	minSpecs.Containers = []specs.ContainerSpec{
		{
			Name:  "gitlab-helper",
			Image: "gitlab-helper/latest",
			Ports: []specs.ContainerPort{
				{ContainerPort: 8080, Protocol: "TCP"},
			},
		},
	}

	gomock.InOrder(
		converter.EXPECT().Validate().Return(nil),
		converter.EXPECT().ToLatest().Return(minSpecs),
	)

	out, err := k8sspces.ParsePodSpecForTest("", getParser)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(minSpecs, jc.DeepEquals, out)
}

func (s *typesSuite) TestK8sContainersValidate(c *gc.C) {
	cs := &k8sspces.K8sContainers{}
	c.Assert(cs.Validate(), gc.ErrorMatches, `require at least one container spec`)

	c1 := k8sspces.K8sContainer{}
	c1.Name = "c1"
	c1.Image = "gitlab"
	cs = &k8sspces.K8sContainers{
		Containers: []k8sspces.K8sContainer{c1},
	}
	c.Assert(cs.Validate(), jc.ErrorIsNil)
}
