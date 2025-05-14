// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
)

type servicesSuite struct {
	client *fake.Clientset
}

var _ = tc.Suite(&servicesSuite{})

func (s *servicesSuite) SetUpTest(c *tc.C) {
	s.client = fake.NewSimpleClientset()
}

func (s *servicesSuite) TestFindServiceForApplication(c *tc.C) {
	_, err := s.client.CoreV1().Services("test").Create(
		c.Context(),
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

	c.Assert(err, tc.ErrorIsNil)

	svc, err := findServiceForApplication(
		c.Context(),
		s.client.CoreV1().Services("test"),
		"wallyworld",
		constants.LabelVersion1,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(svc.Name, tc.Equals, "wallyworld")
}

func (s *servicesSuite) TestFindServiceForApplicationWithEndpoints(c *tc.C) {
	_, err := s.client.CoreV1().Services("test").Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.client.CoreV1().Services("test").Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)

	svc, err := findServiceForApplication(
		c.Context(),
		s.client.CoreV1().Services("test"),
		"wallyworld",
		constants.LabelVersion1,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(svc.Name, tc.Equals, "wallyworld")
}

func (s *servicesSuite) TestFindServiceForApplicationWithMultiple(c *tc.C) {
	_, err := s.client.CoreV1().Services("test").Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.client.CoreV1().Services("test").Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)

	_, err = findServiceForApplication(
		c.Context(),
		s.client.CoreV1().Services("test"),
		"wallyworld",
		constants.LabelVersion1,
	)

	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *servicesSuite) TestFindServiceForApplicationMissing(c *tc.C) {
	_, err := findServiceForApplication(
		c.Context(),
		s.client.CoreV1().Services("test"),
		"wallyworld",
		constants.LabelVersion1,
	)

	c.Assert(err, tc.ErrorIs, errors.NotFound)
}
