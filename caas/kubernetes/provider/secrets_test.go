// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/core/secrets"
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

	out, err := s.broker.GetSecretToken(context.Background(), "secret-1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.Equals, "token")

	result, err := s.mockSecrets.List(context.Background(), v1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Items, gc.HasLen, 1)
	c.Assert(result.Items[0].Name, gc.Equals, "secret-1")
}

func (s *secretsSuite) TestGetJujuSecret(c *gc.C) {
	secret := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "provider-id",
			Namespace: "test",
		},
		Type: core.SecretTypeOpaque,
		Data: map[string][]byte{
			"foo": []byte("bar"),
		},
	}
	_, err := s.mockSecrets.Create(context.Background(), secret, v1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	value, err := s.broker.GetJujuSecret(context.Background(), "provider-id")
	c.Assert(err, jc.ErrorIsNil)
	data, err := value.Values()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, jc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}

func (s *secretsSuite) TestDeleteJujuSecret(c *gc.C) {
	secret := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "provider-id",
			Namespace: "test",
		},
		Type: core.SecretTypeOpaque,
		Data: map[string][]byte{
			"foo": []byte("bar"),
		},
	}
	_, err := s.mockSecrets.Create(context.Background(), secret, v1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)
	another := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "another",
			Namespace: "test",
		},
		Type: core.SecretTypeOpaque,
		Data: map[string][]byte{
			"foo": []byte("bar2"),
		},
	}
	_, err = s.mockSecrets.Create(context.Background(), another, v1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	err = s.broker.DeleteJujuSecret(context.Background(), "provider-id")
	c.Assert(err, jc.ErrorIsNil)
	err = s.broker.DeleteJujuSecret(context.Background(), "provider-id")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	result, err := s.mockSecrets.List(context.Background(), v1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Items, gc.HasLen, 1)
	c.Assert(result.Items[0].Name, gc.Equals, "another")
}

func (s *secretsSuite) TestSaveJujuSecret(c *gc.C) {
	uri := secrets.NewURI()
	providerId, err := s.broker.SaveJujuSecret(context.Background(), uri.ID+"-666",
		secrets.NewSecretValue(map[string]string{
			"foo": "YmFy",
		}),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(providerId, gc.Equals, uri.ID+"-666")
	secret, err := s.mockSecrets.Get(context.Background(), providerId, v1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(secret.Type, gc.Equals, core.SecretTypeOpaque)
	c.Assert(secret.StringData, jc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}
