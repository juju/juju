// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
)

// DummyUpgradeCAASController implements UpgradeCAASControllerBroker for the
// purpose of testing.
type dummyUpgradeCAASController struct {
	client *fake.Clientset
}

type ControllerUpgraderSuite struct {
	broker *dummyUpgradeCAASController
}

var _ = gc.Suite(&ControllerUpgraderSuite{})

func (d *dummyUpgradeCAASController) Client() kubernetes.Interface {
	return d.client
}

func (d *dummyUpgradeCAASController) IsLegacyLabels() bool {
	return false
}

func (d *dummyUpgradeCAASController) Namespace() string {
	return "test"
}

func (s *ControllerUpgraderSuite) SetUpTest(c *gc.C) {
	s.broker = &dummyUpgradeCAASController{
		client: fake.NewSimpleClientset(),
	}
}

func (s *ControllerUpgraderSuite) TestControllerUpgrade(c *gc.C) {
	var (
		appName      = k8sconstants.JujuControllerStackName
		oldImagePath = fmt.Sprintf("%s/%s:9.9.8", podcfg.JujudOCINamespace, podcfg.JujudOCIName)
		newImagePath = fmt.Sprintf("%s/%s:9.9.9", podcfg.JujudOCINamespace, podcfg.JujudOCIName)
	)
	_, err := s.broker.Client().AppsV1().StatefulSets(s.broker.Namespace()).Create(context.Background(),
		&apps.StatefulSet{
			ObjectMeta: meta.ObjectMeta{
				Name: appName,
			},
			Spec: apps.StatefulSetSpec{
				Selector: &meta.LabelSelector{
					MatchLabels: map[string]string{
						"match-label": "true",
					},
				},
				Template: core.PodTemplateSpec{
					Spec: core.PodSpec{
						Containers: []core.Container{
							{
								Name:  "jujud",
								Image: oldImagePath,
							},
						},
					},
				},
			},
		}, meta.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(controllerUpgrade(context.Background(), appName, semversion.MustParse("9.9.9"), s.broker), jc.ErrorIsNil)

	ss, err := s.broker.Client().AppsV1().StatefulSets(s.broker.Namespace()).
		Get(context.Background(), appName, meta.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ss.Spec.Template.Spec.Containers[0].Image, gc.Equals, newImagePath)

	c.Assert(ss.Annotations[utils.AnnotationVersionKey(false)], gc.Equals, semversion.MustParse("9.9.9").String())
	c.Assert(ss.Spec.Template.Annotations[utils.AnnotationVersionKey(false)], gc.Equals, semversion.MustParse("9.9.9").String())
}

func (s *ControllerUpgraderSuite) TestControllerDoesNotExist(c *gc.C) {
	var (
		appName = k8sconstants.JujuControllerStackName
	)
	err := controllerUpgrade(context.Background(), appName, semversion.MustParse("9.9.9"), s.broker)
	c.Assert(err, gc.NotNil)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}
