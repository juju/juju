// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	core "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider"
)

var _ = tc.Suite(&secretsSuite{})

type secretsSuite struct {
	fakeClientSuite
}

func (s *secretsSuite) TestProcessSecretData(c *tc.C) {
	o, err := provider.ProcessSecretData(
		map[string]string{
			"username": "YWRtaW4=",
			"password": "MWYyZDFlMmU2N2Rm",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(o, tc.DeepEquals, map[string][]byte{
		"username": []byte("admin"),
		"password": []byte("1f2d1e2e67df"),
	})
}

func (s *secretsSuite) TestGetSecretToken(c *tc.C) {
	secret := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
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
	_, err := s.mockSecrets.Create(context.Background(), secret, v1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	out, err := s.broker.GetSecretToken(context.Background(), "secret-1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, tc.Equals, "token")

	result, err := s.mockSecrets.List(context.Background(), v1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Items, tc.HasLen, 1)
	c.Assert(result.Items[0].Name, tc.Equals, "secret-1")
}
