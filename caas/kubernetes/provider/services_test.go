// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
)

type servicesSuite struct {
	client *fake.Clientset
}

var _ = gc.Suite(&servicesSuite{})

func (s *servicesSuite) SetUpTest(c *gc.C) {
	s.client = fake.NewSimpleClientset()
}

func (s *servicesSuite) TestFindServiceForApplication(c *gc.C) {
	_, err := s.client.CoreV1().Services("test").Create(
		context.TODO(),
		&core.Service{
			ObjectMeta: meta.ObjectMeta{
				Name: "wallyworld",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "wallyworld",
					"app.kubernetes.io/managed-by": "juju",
				},
			},
		},
		meta.CreateOptions{},
	)

	c.Assert(err, jc.ErrorIsNil)

	svc, err := findServiceForApplication(
		context.TODO(),
		s.client.CoreV1().Services("test"),
		"wallyworld",
		constants.LabelVersion1,
	)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(svc.Name, gc.Equals, "wallyworld")
}

func (s *servicesSuite) TestFindServiceForApplicationWithEndpoints(c *gc.C) {
	_, err := s.client.CoreV1().Services("test").Create(
		context.TODO(),
		&core.Service{
			ObjectMeta: meta.ObjectMeta{
				Name: "wallyworld",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "wallyworld",
					"app.kubernetes.io/managed-by": "juju",
				},
			},
		},
		meta.CreateOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.client.CoreV1().Services("test").Create(
		context.TODO(),
		&core.Service{
			ObjectMeta: meta.ObjectMeta{
				Name: "wallyworld-endpoints",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "wallyworld",
					"app.kubernetes.io/managed-by": "juju",
				},
			},
		},
		meta.CreateOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)

	svc, err := findServiceForApplication(
		context.TODO(),
		s.client.CoreV1().Services("test"),
		"wallyworld",
		constants.LabelVersion1,
	)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(svc.Name, gc.Equals, "wallyworld")
}

func (s *servicesSuite) TestFindServiceForApplicationWithMultiple(c *gc.C) {
	_, err := s.client.CoreV1().Services("test").Create(
		context.TODO(),
		&core.Service{
			ObjectMeta: meta.ObjectMeta{
				Name: "wallyworld",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "wallyworld",
					"app.kubernetes.io/managed-by": "juju",
				},
			},
		},
		meta.CreateOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.client.CoreV1().Services("test").Create(
		context.TODO(),
		&core.Service{
			ObjectMeta: meta.ObjectMeta{
				Name: "wallyworld-v2",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "wallyworld",
					"app.kubernetes.io/managed-by": "juju",
				},
			},
		},
		meta.CreateOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)

	_, err = findServiceForApplication(
		context.TODO(),
		s.client.CoreV1().Services("test"),
		"wallyworld",
		constants.LabelVersion1,
	)

	c.Assert(errors.Is(err, errors.NotValid), jc.IsTrue)
}

func (s *servicesSuite) TestFindServiceForApplicationMissing(c *gc.C) {
	_, err := findServiceForApplication(
		context.TODO(),
		s.client.CoreV1().Services("test"),
		"wallyworld",
		constants.LabelVersion1,
	)

	c.Assert(errors.Is(err, errors.NotFound), jc.IsTrue)
}
