// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider"
)

var _ = gc.Suite(&secretsSuite{})

type secretsSuite struct {
	BaseSuite
}

func (s *secretsSuite) TestProcessSecretData(c *gc.C) {
	o, err := provider.ProcessSecretData(
		map[string]string{
			"username": "YWRtaW4=",
			"password": "MWYyZDFlMmU2N2Rm",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(o, gc.DeepEquals, map[string][]byte{
		"username": []byte("admin"),
		"password": []byte("1f2d1e2e67df"),
	})
}

func (s *secretsSuite) TestGetSecretToken(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	secret := &core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name: "secret-1",
			Annotations: map[string]string{
				core.ServiceAccountNameKey: "secret-1",
			},
		},
		Type: core.SecretTypeServiceAccountToken,
		Data: map[string][]byte{
			core.ServiceAccountTokenKey: []byte("token"),
		},
	}

	gomock.InOrder(
		s.mockSecrets.EXPECT().Get(gomock.Any(), "secret-1", meta.GetOptions{}).
			Return(secret, nil),
	)

	out, err := s.broker.GetSecretToken("secret-1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.Equals, "token")
}
