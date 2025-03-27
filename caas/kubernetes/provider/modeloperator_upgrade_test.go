// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/cloudconfig/podcfg"
)

type dummyUpgradeCAASModel struct {
	client *fake.Clientset
}

type modelUpgraderSuite struct {
	broker *dummyUpgradeCAASModel
}

var _ = gc.Suite(&modelUpgraderSuite{})

func (d *dummyUpgradeCAASModel) Client() kubernetes.Interface {
	return d.client
}

func (d *dummyUpgradeCAASModel) LabelVersion() constants.LabelVersion {
	return constants.LabelVersion1
}

func (d *dummyUpgradeCAASModel) Namespace() string {
	return "test"
}

func (s *modelUpgraderSuite) SetUpTest(c *gc.C) {
	s.broker = &dummyUpgradeCAASModel{
		client: fake.NewSimpleClientset(),
	}
}

func (s *modelUpgraderSuite) TestModelOperatorUpgrade(c *gc.C) {
	var (
		operatorName = modelOperatorName
		oldImagePath = fmt.Sprintf("%s/%s:9.9.8", podcfg.JujudOCINamespace, podcfg.JujudOCIName)
		newImagePath = fmt.Sprintf("%s/%s:9.9.9", podcfg.JujudOCINamespace, podcfg.JujudOCIName)
	)

	_, err := s.broker.Client().AppsV1().Deployments(s.broker.Namespace()).Create(context.TODO(),
		&apps.Deployment{
			ObjectMeta: meta.ObjectMeta{
				Name: operatorName,
			},
			Spec: apps.DeploymentSpec{
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
		}, v1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(modelOperatorUpgrade(operatorName, version.MustParse("9.9.9"), s.broker), jc.ErrorIsNil)
	de, err := s.broker.Client().AppsV1().Deployments(s.broker.Namespace()).
		Get(context.TODO(), operatorName, meta.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(de.Spec.Template.Spec.Containers[0].Image, gc.Equals, newImagePath)

	c.Assert(de.Annotations[utils.AnnotationVersionKey(1)], gc.Equals, version.MustParse("9.9.9").String())
	c.Assert(de.Spec.Template.Annotations[utils.AnnotationVersionKey(1)], gc.Equals, version.MustParse("9.9.9").String())
}
