// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vault_test

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/hashicorp/vault/api"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	vault "github.com/mittwald/vaultgo"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/secrets/provider"
	_ "github.com/juju/juju/secrets/provider/all"
	jujuvault "github.com/juju/juju/secrets/provider/vault"
	"github.com/juju/juju/secrets/provider/vault/mocks"
	coretesting "github.com/juju/juju/testing"
)

type providerSuite struct {
	testing.IsolationSuite
	coretesting.JujuOSEnvSuite

	mockRoundTripper *mocks.MockRoundTripper
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)
}

type newVaultClientFunc func(addr string, tlsConf *vault.TLSConfig, opts ...vault.ClientOpts) (*vault.Client, error)

func (s *providerSuite) newVaultClient(c *gc.C, returnErr error) (*gomock.Controller, newVaultClientFunc) {
	ctrl := gomock.NewController(c)
	s.mockRoundTripper = mocks.NewMockRoundTripper(ctrl)

	return ctrl, func(addr string, tlsConf *vault.TLSConfig, opts ...vault.ClientOpts) (*vault.Client, error) {
		c.Assert(addr, gc.Equals, "http://vault-ip:8200/")
		c.Assert(tlsConf, jc.DeepEquals, &vault.TLSConfig{
			TLSConfig: &api.TLSConfig{
				CACertBytes:   []byte(coretesting.CACert),
				TLSServerName: "tls-server",
			},
		})

		client, err := vault.NewClient(addr, tlsConf, opts...)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(opts, gc.HasLen, 1)
		err = opts[0](client)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(client.Token(), gc.Equals, "vault-token")
		if returnErr != nil {
			return nil, returnErr
		}

		conf := api.DefaultConfig()
		conf.Address = addr
		if tlsConf != nil {
			err = conf.ConfigureTLS(tlsConf.TLSConfig)
			c.Assert(err, jc.ErrorIsNil)
		}
		conf.HttpClient.Transport = s.mockRoundTripper
		client.Client, err = api.NewClient(conf)
		c.Assert(err, jc.ErrorIsNil)

		return client, nil
	}
}

func (s *providerSuite) TestBackendConfigBadClient(c *gc.C) {
	ctrl, newVaultClient := s.newVaultClient(c, errors.New("boom"))
	defer ctrl.Finish()

	s.PatchValue(&jujuvault.NewVaultClient, newVaultClient)
	p, err := provider.Provider(jujuvault.BackendType)
	c.Assert(err, jc.ErrorIsNil)

	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "vault",
			Config: map[string]any{
				"endpoint":        "http://vault-ip:8200/",
				"namespace":       "ns",
				"token":           "vault-token",
				"ca-cert":         coretesting.CACert,
				"tls-server-name": "tls-server",
			},
		},
	}
	issuedTokenUUID := "some-uuid"
	_, err = p.RestrictedConfig(
		adminCfg, true, false, issuedTokenUUID, nil, nil, nil, nil)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *providerSuite) TestBackendConfigAdmin(c *gc.C) {
	ctrl, newVaultClient := s.newVaultClient(c, nil)
	defer ctrl.Finish()

	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), gc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-some-uuid`)
			c.Assert(req.Method, gc.Equals, http.MethodGet)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader(`{"errors":[]}`)),
			}, nil
		},
	)
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), gc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-some-uuid`)
			c.Assert(req.Method, gc.Equals, http.MethodPut)
			b, _ := ioutil.ReadAll(req.Body)
			defer req.Body.Close()
			policyReq := struct {
				Policy string
			}{}
			_ = json.Unmarshal(b, &policyReq)
			c.Assert(policyReq.Policy, gc.Equals, strings.Join([]string{
				`path "fred-06f00d/*" {capabilities = ["read"]}`,
			}, "\n"))
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(nil),
			}, nil
		},
	)
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), gc.Equals, `http://vault-ip:8200/v1/auth/token/create`)
			b, _ := ioutil.ReadAll(req.Body)
			defer req.Body.Close()
			tokenReq := api.TokenCreateRequest{}
			_ = json.Unmarshal(b, &tokenReq)
			c.Assert(tokenReq, jc.DeepEquals, api.TokenCreateRequest{
				Policies:        []string{"fred-06f00d-some-uuid"},
				Metadata:        map[string]string{"juju-issued-token-uuid": "some-uuid"},
				TTL:             "10m",
				NoDefaultPolicy: true,
			})
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"auth": {"client_token": "foo"}}`)),
			}, nil
		},
	)

	s.PatchValue(&jujuvault.NewVaultClient, newVaultClient)
	p, err := provider.Provider(jujuvault.BackendType)
	c.Assert(err, jc.ErrorIsNil)

	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "vault",
			Config: map[string]any{
				"endpoint":        "http://vault-ip:8200/",
				"namespace":       "ns",
				"token":           "vault-token",
				"ca-cert":         coretesting.CACert,
				"tls-server-name": "tls-server",
			},
		},
	}
	issuedTokenUUID := "some-uuid"
	cfg, err := p.RestrictedConfig(
		adminCfg, true, false, issuedTokenUUID, nil, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Config["token"], gc.Equals, "foo")
}

func (s *providerSuite) TestBackendConfigNonAdmin(c *gc.C) {
	ctrl, newVaultClient := s.newVaultClient(c, nil)
	defer ctrl.Finish()

	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), gc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-some-uuid`)
			c.Assert(req.Method, gc.Equals, http.MethodGet)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader(`{"errors":[]}`)),
			}, nil
		},
	)
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), gc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-some-uuid`)
			c.Assert(req.Method, gc.Equals, http.MethodPut)
			b, _ := ioutil.ReadAll(req.Body)
			defer req.Body.Close()
			policyReq := struct {
				Policy string
			}{}
			_ = json.Unmarshal(b, &policyReq)
			c.Assert(policyReq.Policy, gc.Equals, strings.Join([]string{
				`path "fred-06f00d/owned-1-*" {capabilities = ["create", "read", "update", "delete", "list"]}`,
				`path "fred-06f00d/read-rev-1" {capabilities = ["read"]}`,
			}, "\n"))
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(nil),
			}, nil
		},
	)
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), gc.Equals, `http://vault-ip:8200/v1/auth/token/create`)
			b, _ := ioutil.ReadAll(req.Body)
			defer req.Body.Close()
			tokenReq := api.TokenCreateRequest{}
			_ = json.Unmarshal(b, &tokenReq)
			c.Assert(tokenReq, jc.DeepEquals, api.TokenCreateRequest{
				Policies:        []string{"fred-06f00d-some-uuid"},
				Metadata:        map[string]string{"juju-issued-token-uuid": "some-uuid"},
				TTL:             "10m",
				NoDefaultPolicy: true,
			})
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"auth": {"client_token": "foo"}}`)),
			}, nil
		},
	)

	s.PatchValue(&jujuvault.NewVaultClient, newVaultClient)
	p, err := provider.Provider(jujuvault.BackendType)
	c.Assert(err, jc.ErrorIsNil)

	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "vault",
			Config: map[string]any{
				"endpoint":        "http://vault-ip:8200/",
				"namespace":       "ns",
				"token":           "vault-token",
				"ca-cert":         coretesting.CACert,
				"tls-server-name": "tls-server",
			},
		},
	}
	ownedNames := []string{"owned-1"}
	ownedRevs := map[string]set.Strings{
		"owned-1": set.NewStrings("owned-rev-1", "owned-rev-2"),
	}
	readRevs := map[string]set.Strings{
		"read-1": set.NewStrings("read-rev-1"),
	}
	issuedTokenUUID := "some-uuid"
	cfg, err := p.RestrictedConfig(
		adminCfg, true, false, issuedTokenUUID, names.NewUnitTag("ubuntu/0"),
		ownedNames, ownedRevs, readRevs,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Config["token"], gc.Equals, "foo")
}

func (s *providerSuite) TestBackendConfigForDrain(c *gc.C) {
	ctrl, newVaultClient := s.newVaultClient(c, nil)
	defer ctrl.Finish()

	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), gc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-some-uuid`)
			c.Assert(req.Method, gc.Equals, http.MethodGet)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader(`{"errors":[]}`)),
			}, nil
		},
	)
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), gc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-some-uuid`)
			c.Assert(req.Method, gc.Equals, http.MethodPut)
			b, _ := ioutil.ReadAll(req.Body)
			defer req.Body.Close()
			policyReq := struct {
				Policy string
			}{}
			_ = json.Unmarshal(b, &policyReq)
			c.Assert(policyReq.Policy, gc.Equals, strings.Join([]string{
				`path "fred-06f00d/owned-1-*" {capabilities = ["create", "read", "update", "delete", "list"]}`,
				`path "fred-06f00d/read-rev-1" {capabilities = ["read"]}`,
			}, "\n"))
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(nil),
			}, nil
		},
	)
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), gc.Equals, `http://vault-ip:8200/v1/auth/token/create`)
			b, _ := ioutil.ReadAll(req.Body)
			defer req.Body.Close()
			tokenReq := api.TokenCreateRequest{}
			_ = json.Unmarshal(b, &tokenReq)
			c.Assert(tokenReq, jc.DeepEquals, api.TokenCreateRequest{
				Policies:        []string{"fred-06f00d-some-uuid"},
				Metadata:        map[string]string{"juju-issued-token-uuid": "some-uuid"},
				TTL:             "10m",
				NoDefaultPolicy: true,
			})
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"auth": {"client_token": "foo"}}`)),
			}, nil
		},
	)

	s.PatchValue(&jujuvault.NewVaultClient, newVaultClient)
	p, err := provider.Provider(jujuvault.BackendType)
	c.Assert(err, jc.ErrorIsNil)

	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "vault",
			Config: map[string]any{
				"endpoint":        "http://vault-ip:8200/",
				"namespace":       "ns",
				"token":           "vault-token",
				"ca-cert":         coretesting.CACert,
				"tls-server-name": "tls-server",
			},
		},
	}
	ownedNames := []string{"owned-1"}
	ownedRevs := map[string]set.Strings{
		"owned-1": set.NewStrings("owned-rev-1", "owned-rev-2"),
	}
	readRevs := map[string]set.Strings{
		"read-1": set.NewStrings("read-rev-1"),
	}
	issuedTokenUUID := "some-uuid"
	cfg, err := p.RestrictedConfig(
		adminCfg, true, true, issuedTokenUUID,
		names.NewUnitTag("ubuntu/0"), ownedNames, ownedRevs, readRevs,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Config["token"], gc.Equals, "foo")
}

func (s *providerSuite) TestBackendConfigNonAdminIdempotentSameIssuedTokenUUID(c *gc.C) {
	ctrl, newVaultClient := s.newVaultClient(c, nil)
	defer ctrl.Finish()

	policyURL := `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-some-uuid`
	tokenURL := `http://vault-ip:8200/v1/auth/token/create`
	expectedPolicy := strings.Join([]string{
		`path "fred-06f00d/owned-1-*" {capabilities = ["create", "read", "update", "delete", "list"]}`,
		`path "fred-06f00d/read-rev-1" {capabilities = ["read"]}`,
	}, "\n")
	policyGetCalls := 0
	policyPutCalls := 0
	tokenCalls := 0
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case policyURL:
				if req.Method == http.MethodGet {
					policyGetCalls++
					if policyGetCalls == 1 {
						return &http.Response{Request: req, StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(`{"errors":[]}`))}, nil
					}
					return &http.Response{
						Request:    req,
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"data":{"policy":%q}}`, expectedPolicy))),
					}, nil
				}
				c.Assert(req.Method, gc.Equals, http.MethodPut)
				policyPutCalls++
				b, _ := ioutil.ReadAll(req.Body)
				defer req.Body.Close()
				policyReq := struct {
					Policy string
				}{}
				_ = json.Unmarshal(b, &policyReq)
				c.Assert(policyReq.Policy, gc.Equals, expectedPolicy)
				return &http.Response{Request: req, StatusCode: http.StatusOK, Body: io.NopCloser(nil)}, nil
			case tokenURL:
				tokenCalls++
				tokenValue := "foo1"
				if tokenCalls == 2 {
					tokenValue = "foo2"
				}
				return &http.Response{Request: req, StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"auth": {"client_token": "` + tokenValue + `"}}`))}, nil
			default:
				c.Fatalf("unexpected URL %q", req.URL.String())
				return nil, nil
			}
		},
	).AnyTimes()

	s.PatchValue(&jujuvault.NewVaultClient, newVaultClient)
	p, err := provider.Provider(jujuvault.BackendType)
	c.Assert(err, jc.ErrorIsNil)

	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "vault",
			Config: map[string]any{
				"endpoint":        "http://vault-ip:8200/",
				"namespace":       "ns",
				"token":           "vault-token",
				"ca-cert":         coretesting.CACert,
				"tls-server-name": "tls-server",
			},
		},
	}
	ownedNames := []string{"owned-1"}
	ownedRevs := map[string]set.Strings{"owned-1": set.NewStrings("owned-rev-1", "owned-rev-2")}
	readRevs := map[string]set.Strings{"read-1": set.NewStrings("read-rev-1")}
	issuedTokenUUID := "some-uuid"

	cfg1, err := p.RestrictedConfig(
		adminCfg, true, false, issuedTokenUUID, names.NewUnitTag("ubuntu/0"),
		ownedNames, ownedRevs, readRevs,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg1.Config["token"], gc.Equals, "foo1")

	cfg2, err := p.RestrictedConfig(
		adminCfg, true, false, issuedTokenUUID, names.NewUnitTag("ubuntu/0"),
		ownedNames, ownedRevs, readRevs,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg2.Config["token"], gc.Equals, "foo2")
	c.Assert(policyGetCalls, gc.Equals, 2)
	c.Assert(policyPutCalls, gc.Equals, 1)
	c.Assert(tokenCalls, gc.Equals, 2)
}

func (s *providerSuite) TestBackendConfigNonAdminIdempotentSameIssuedTokenUUIDOrderAndDuplicateInsensitive(c *gc.C) {
	ctrl, newVaultClient := s.newVaultClient(c, nil)
	defer ctrl.Finish()

	policyURL := `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-some-uuid`
	tokenURL := `http://vault-ip:8200/v1/auth/token/create`
	expectedPolicy := strings.Join([]string{
		`path "fred-06f00d/owned-1-*" {capabilities = ["create", "read", "update", "delete", "list"]}`,
		`path "fred-06f00d/owned-2-*" {capabilities = ["create", "read", "update", "delete", "list"]}`,
		`path "fred-06f00d/read-rev-1" {capabilities = ["read"]}`,
		`path "fred-06f00d/read-rev-2" {capabilities = ["read"]}`,
	}, "\n")
	policyGetCalls := 0
	policyPutCalls := 0
	tokenCalls := 0
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case policyURL:
				if req.Method == http.MethodGet {
					policyGetCalls++
					if policyGetCalls == 1 {
						return &http.Response{Request: req, StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(`{"errors":[]}`))}, nil
					}
					return &http.Response{
						Request:    req,
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"data":{"policy":%q}}`, expectedPolicy))),
					}, nil
				}
				c.Assert(req.Method, gc.Equals, http.MethodPut)
				policyPutCalls++
				b, _ := ioutil.ReadAll(req.Body)
				defer req.Body.Close()
				policyReq := struct {
					Policy string
				}{}
				_ = json.Unmarshal(b, &policyReq)
				c.Assert(policyReq.Policy, gc.Equals, expectedPolicy)
				return &http.Response{Request: req, StatusCode: http.StatusOK, Body: io.NopCloser(nil)}, nil
			case tokenURL:
				tokenCalls++
				return &http.Response{Request: req, StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"auth": {"client_token": "foo"}}`))}, nil
			default:
				c.Fatalf("unexpected URL %q", req.URL.String())
				return nil, nil
			}
		},
	).AnyTimes()

	s.PatchValue(&jujuvault.NewVaultClient, newVaultClient)
	p, err := provider.Provider(jujuvault.BackendType)
	c.Assert(err, jc.ErrorIsNil)

	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "vault",
			Config: map[string]any{
				"endpoint":        "http://vault-ip:8200/",
				"namespace":       "ns",
				"token":           "vault-token",
				"ca-cert":         coretesting.CACert,
				"tls-server-name": "tls-server",
			},
		},
	}
	issuedTokenUUID := "some-uuid"

	_, err = p.RestrictedConfig(
		adminCfg, true, false, issuedTokenUUID, names.NewUnitTag("ubuntu/0"),
		[]string{"owned-2", "owned-1"},
		map[string]set.Strings{"owned-1": set.NewStrings("owned-rev-1")},
		map[string]set.Strings{
			"read-1": set.NewStrings("read-rev-2", "read-rev-1"),
			"read-2": set.NewStrings("read-rev-1"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	_, err = p.RestrictedConfig(
		adminCfg, true, false, issuedTokenUUID, names.NewUnitTag("ubuntu/0"),
		[]string{"owned-1", "owned-2", "owned-1"},
		map[string]set.Strings{"owned-2": set.NewStrings("owned-rev-2")},
		map[string]set.Strings{
			"read-1": set.NewStrings("read-rev-1"),
			"read-2": set.NewStrings("read-rev-2", "read-rev-2"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(policyGetCalls, gc.Equals, 2)
	c.Assert(policyPutCalls, gc.Equals, 1)
	c.Assert(tokenCalls, gc.Equals, 2)
}

func (s *providerSuite) TestBackendConfigNonAdminImmutableMismatchSameIssuedTokenUUID(c *gc.C) {
	ctrl, newVaultClient := s.newVaultClient(c, nil)
	defer ctrl.Finish()

	policyURL := `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-some-uuid`
	tokenURL := `http://vault-ip:8200/v1/auth/token/create`
	expectedPolicy := strings.Join([]string{
		`path "fred-06f00d/owned-1-*" {capabilities = ["create", "read", "update", "delete", "list"]}`,
		`path "fred-06f00d/read-rev-1" {capabilities = ["read"]}`,
	}, "\n")
	policyGetCalls := 0
	policyPutCalls := 0
	tokenCalls := 0
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case policyURL:
				if req.Method == http.MethodGet {
					policyGetCalls++
					if policyGetCalls == 1 {
						return &http.Response{Request: req, StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(`{"errors":[]}`))}, nil
					}
					return &http.Response{
						Request:    req,
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"data":{"policy":%q}}`, expectedPolicy))),
					}, nil
				}
				c.Assert(req.Method, gc.Equals, http.MethodPut)
				policyPutCalls++
				return &http.Response{Request: req, StatusCode: http.StatusOK, Body: io.NopCloser(nil)}, nil
			case tokenURL:
				tokenCalls++
				return &http.Response{Request: req, StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"auth": {"client_token": "foo1"}}`))}, nil
			default:
				c.Fatalf("unexpected URL %q", req.URL.String())
				return nil, nil
			}
		},
	).AnyTimes()

	s.PatchValue(&jujuvault.NewVaultClient, newVaultClient)
	p, err := provider.Provider(jujuvault.BackendType)
	c.Assert(err, jc.ErrorIsNil)

	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "vault",
			Config: map[string]any{
				"endpoint":        "http://vault-ip:8200/",
				"namespace":       "ns",
				"token":           "vault-token",
				"ca-cert":         coretesting.CACert,
				"tls-server-name": "tls-server",
			},
		},
	}
	ownedNames := []string{"owned-1"}
	ownedRevs := map[string]set.Strings{"owned-1": set.NewStrings("owned-rev-1", "owned-rev-2")}
	readRevs := map[string]set.Strings{"read-1": set.NewStrings("read-rev-1")}
	issuedTokenUUID := "some-uuid"

	_, err = p.RestrictedConfig(
		adminCfg, true, false, issuedTokenUUID, names.NewUnitTag("ubuntu/0"),
		ownedNames, ownedRevs, readRevs,
	)
	c.Assert(err, jc.ErrorIsNil)

	_, err = p.RestrictedConfig(
		adminCfg, true, false, issuedTokenUUID, names.NewUnitTag("ubuntu/0"),
		ownedNames, ownedRevs,
		map[string]set.Strings{"read-1": set.NewStrings("read-rev-2")},
	)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
	c.Assert(policyGetCalls, gc.Equals, 2)
	c.Assert(policyPutCalls, gc.Equals, 1)
	c.Assert(tokenCalls, gc.Equals, 1)
}

func (s *providerSuite) TestNewBackend(c *gc.C) {
	ctrl, newVaultClient := s.newVaultClient(c, nil)
	defer ctrl.Finish()
	s.PatchValue(&jujuvault.NewVaultClient, newVaultClient)
	p, err := provider.Provider(jujuvault.BackendType)
	c.Assert(err, jc.ErrorIsNil)

	cfg := &provider.ModelBackendConfig{
		ModelName: "fred",
		ModelUUID: coretesting.ModelTag.Id(),
		BackendConfig: provider.BackendConfig{
			BackendType: jujuvault.BackendType,
			Config: map[string]any{
				"endpoint":        "http://vault-ip:8200/",
				"namespace":       "ns",
				"token":           "vault-token",
				"ca-cert":         coretesting.CACert,
				"tls-server-name": "tls-server",
			},
		},
	}
	b, err := p.NewBackend(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(jujuvault.MountPath(b), gc.Equals, "fred-06f00d")
}

func (s *providerSuite) TestNewBackendWithMountPath(c *gc.C) {
	ctrl, newVaultClient := s.newVaultClient(c, nil)
	defer ctrl.Finish()
	s.PatchValue(&jujuvault.NewVaultClient, newVaultClient)
	p, err := provider.Provider(jujuvault.BackendType)
	c.Assert(err, jc.ErrorIsNil)

	cfg := &provider.ModelBackendConfig{
		ModelName: "fred",
		ModelUUID: coretesting.ModelTag.Id(),
		BackendConfig: provider.BackendConfig{
			BackendType: jujuvault.BackendType,
			Config: map[string]interface{}{
				"endpoint":        "http://vault-ip:8200/",
				"token":           "vault-token",
				"mount-path":      "/some/path/",
				"ca-cert":         coretesting.CACert,
				"tls-server-name": "tls-server",
			},
		},
	}
	b, err := p.NewBackend(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(jujuvault.MountPath(b), gc.Equals, "/some/path/fred-06f00d")
}
