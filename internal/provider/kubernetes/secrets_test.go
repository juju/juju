// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	provider "github.com/juju/juju/internal/provider/kubernetes"
)

var _ = gc.Suite(&secretsSuite{})

type secretsSuite struct {
	fakeClientSuite
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

	out, err := s.broker.GetSecretToken("secret-1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.Equals, "token")

	result, err := s.mockSecrets.List(context.Background(), v1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Items, gc.HasLen, 1)
	c.Assert(result.Items[0].Name, gc.Equals, "secret-1")
}
