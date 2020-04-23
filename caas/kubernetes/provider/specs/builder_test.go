// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	appsv1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	apischema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/rest/fake"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	providermocks "github.com/juju/juju/caas/kubernetes/provider/mocks"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	k8sspecsmocks "github.com/juju/juju/caas/kubernetes/provider/specs/mocks"
	"github.com/juju/juju/cloud"
	k8sannotations "github.com/juju/juju/core/annotations"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/testing"
)

type builderSuite struct {
	testing.BaseSuite
	mockRestClients map[apischema.GroupVersion]*providermocks.MockRestClientInterface
	mockRestMapper  *k8sspecsmocks.MockRESTMapper

	builder k8sspecs.DeployerInterface

	namespace        string
	deploymentParams *caas.DeploymentParams
	labels           map[string]string
	annotations      k8sannotations.Annotation
}

var rawK8sSpec = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  labels:
    app: nginx
  namespace: ns-will-be-overwritten
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
        ports:
        - containerPort: 8000
          protocol: TCP
---
apiVersion: v1
kind: Service
metadata:
  name: mock
  labels:
    app: mock
spec:
  ports:
  - port: 99
    protocol: TCP
    targetPort: 9949
  selector:
    app: mock
`[1:]

var _ = gc.Suite(&builderSuite{})

type assertionRegisterRestClient func(c *providermocks.MockRestClientInterface)
type restClientSetUpAction struct {
	doAssert assertionRegisterRestClient
	gv       apischema.GroupVersion
}

func (s *builderSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.namespace = "test"
	s.deploymentParams = &caas.DeploymentParams{DeploymentType: caas.DeploymentStateless}
	s.mockRestClients = make(map[apischema.GroupVersion]*providermocks.MockRestClientInterface)
	s.labels = map[string]string{"juju-app": "test-app"}
	s.annotations = k8sannotations.New(
		map[string]string{
			"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
			"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
			"juju.io/controller":                       testing.ControllerTag.Id(),
		},
	)
}

func (s *builderSuite) TearDownTest(c *gc.C) {
	s.namespace = ""
	s.deploymentParams = nil
	s.mockRestClients = nil
	s.labels = nil
	s.annotations = nil
	s.BaseSuite.TearDownTest(c)
}

func (s *builderSuite) setupRestClients(c *gc.C, restClientAssertions ...restClientSetUpAction) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockRestMapper = k8sspecsmocks.NewMockRESTMapper(ctrl)

	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"username":              "fred",
		"password":              "secret",
		"ClientCertificateData": "cert-data",
		"ClientKeyData":         "cert-key",
	})
	cloudSpec := environs.CloudSpec{
		Endpoint:       "some-host",
		Credential:     &cred,
		CACertificates: []string{testing.CACert},
	}
	k8sRestConfig, err := provider.CloudSpecToK8sRestConfig(cloudSpec)
	c.Assert(err, jc.ErrorIsNil)

	for _, rca := range restClientAssertions {
		mockRestClient := providermocks.NewMockRestClientInterface(ctrl)
		rca.doAssert(mockRestClient)
		s.mockRestClients[rca.gv] = mockRestClient
	}

	s.builder = k8sspecs.NewDeployer(
		"test-app", s.namespace, *s.deploymentParams,
		k8sRestConfig,
		func(isNamespaced bool) map[string]string {
			return s.labels
		},
		s.annotations,
		func(cfg *rest.Config) (rest.Interface, error) {
			c.Assert(cfg.Username, gc.Equals, "fred")
			c.Assert(cfg.Password, gc.Equals, "secret")
			c.Assert(cfg.Host, gc.Equals, "some-host")
			c.Assert(cfg.TLSClientConfig, jc.DeepEquals, rest.TLSClientConfig{
				CertData: []byte("cert-data"),
				KeyData:  []byte("cert-key"),
				CAData:   []byte(testing.CACert),
			})
			return s.getRestClientForGroupVersion(c, *cfg.GroupVersion), nil
		},
		func(rest.Interface) meta.RESTMapper {
			return s.mockRestMapper
		},
	)
	return ctrl
}

func (s *builderSuite) getRestClientForGroupVersion(c *gc.C, gv apischema.GroupVersion) *providermocks.MockRestClientInterface {
	rC, ok := s.mockRestClients[gv]
	c.Assert(ok, jc.IsTrue)
	return rC
}

func (s *builderSuite) TestDeployCreated(c *gc.C) {
	gvApps := appsv1.SchemeGroupVersion
	gvCore := core.SchemeGroupVersion

	ctrl := s.setupRestClients(c,
		restClientSetUpAction{
			gv: gvApps, doAssert: func(restC *providermocks.MockRestClientInterface) {
				gvk := gvApps.WithKind("Deployment")
				restC.EXPECT().Patch(types.ApplyPatchType).DoAndReturn(
					func(t types.PatchType) *rest.Request {
						header := http.Header{}
						header.Set("Content-Type", string(t))
						return s.fakeRequest(
							c, gvk, nil, &http.Response{Header: header, StatusCode: http.StatusNotFound},
							"https://1.1.1.1/v1/namespaces/test/deployments/nginx-deployment?fieldManager=juju&force=true",
						)
					},
				)
				restC.EXPECT().Post().Return(
					s.fakeRequest(
						c, gvk, nil, &http.Response{StatusCode: http.StatusCreated},
						"https://1.1.1.1/v1/namespaces/test/deployments/nginx-deployment?fieldManager=juju&force=true",
					),
				)
			},
		},
		restClientSetUpAction{
			gv: gvCore, doAssert: func(restC *providermocks.MockRestClientInterface) {
				gvk := gvCore.WithKind("Service")
				restC.EXPECT().Patch(types.ApplyPatchType).DoAndReturn(
					func(t types.PatchType) *rest.Request {
						header := http.Header{}
						header.Set("Content-Type", string(t))
						return s.fakeRequest(
							c, gvk, nil, &http.Response{Header: header, StatusCode: http.StatusNotFound},
							"https://1.1.1.1/v1/namespaces/test/services/mock?fieldManager=juju&force=true",
						)
					},
				)
				restC.EXPECT().Post().Return(
					s.fakeRequest(
						c, gvk, nil, &http.Response{StatusCode: http.StatusCreated},
						"https://1.1.1.1/v1/namespaces/test/services/mock?fieldManager=juju&force=true",
					),
				)
			},
		},
	)
	defer ctrl.Finish()

	ctx, cancel := context.WithTimeout(context.Background(), testing.LongWait)
	defer cancel()

	gomock.InOrder(
		// Get mapping for all resources.
		s.mockRestMapper.EXPECT().
			RESTMapping(apischema.GroupKind{Kind: "Deployment", Group: "apps"}, "v1").
			DoAndReturn(
				func(gk apischema.GroupKind, version string) (*meta.RESTMapping, error) {
					return restMapping(meta.RESTScopeNameNamespace, gk, version), nil
				},
			),
		s.mockRestMapper.EXPECT().
			RESTMapping(apischema.GroupKind{Kind: "Service"}, "v1").
			DoAndReturn(
				func(gk apischema.GroupKind, version string) (*meta.RESTMapping, error) {
					return restMapping(meta.RESTScopeNameNamespace, gk, version), nil
				},
			),
	)

	c.Assert(s.builder.Deploy(ctx, rawK8sSpec, true), jc.ErrorIsNil)
}

func (s *builderSuite) TestDeployUpdated(c *gc.C) {
	gvApps := appsv1.SchemeGroupVersion
	gvCore := core.SchemeGroupVersion

	ctrl := s.setupRestClients(c,
		restClientSetUpAction{
			gv: gvApps, doAssert: func(restC *providermocks.MockRestClientInterface) {
				gvk := gvApps.WithKind("Deployment")
				restC.EXPECT().Patch(types.ApplyPatchType).DoAndReturn(
					func(t types.PatchType) *rest.Request {
						header := http.Header{}
						header.Set("Content-Type", string(t))
						return s.fakeRequest(
							c, gvk, nil, &http.Response{Header: header, StatusCode: http.StatusOK},
							"https://1.1.1.1/v1/namespaces/test/deployments/nginx-deployment?fieldManager=juju&force=true",
						)
					},
				)
			},
		},
		restClientSetUpAction{
			gv: gvCore, doAssert: func(restC *providermocks.MockRestClientInterface) {
				gvk := gvCore.WithKind("Service")
				restC.EXPECT().Patch(types.ApplyPatchType).DoAndReturn(
					func(t types.PatchType) *rest.Request {
						header := http.Header{}
						header.Set("Content-Type", string(t))
						return s.fakeRequest(
							c, gvk, nil, &http.Response{Header: header, StatusCode: http.StatusOK},
							"https://1.1.1.1/v1/namespaces/test/services/mock?fieldManager=juju&force=true",
						)
					},
				)
			},
		},
	)
	defer ctrl.Finish()

	ctx, cancel := context.WithTimeout(context.Background(), testing.LongWait)
	defer cancel()

	gomock.InOrder(
		// Get mapping for all resources.
		s.mockRestMapper.EXPECT().
			RESTMapping(apischema.GroupKind{Kind: "Deployment", Group: "apps"}, "v1").
			DoAndReturn(
				func(gk apischema.GroupKind, version string) (*meta.RESTMapping, error) {
					return restMapping(meta.RESTScopeNameNamespace, gk, version), nil
				},
			),
		s.mockRestMapper.EXPECT().
			RESTMapping(apischema.GroupKind{Kind: "Service"}, "v1").
			DoAndReturn(
				func(gk apischema.GroupKind, version string) (*meta.RESTMapping, error) {
					return restMapping(meta.RESTScopeNameNamespace, gk, version), nil
				},
			),
	)

	c.Assert(s.builder.Deploy(ctx, rawK8sSpec, true), jc.ErrorIsNil)
}

func (s *builderSuite) TestDeployDeploymentTypeMismatchFailed(c *gc.C) {
	gvApps := appsv1.SchemeGroupVersion
	gvCore := core.SchemeGroupVersion

	// Set wrong deployment type to daemon.
	s.deploymentParams = &caas.DeploymentParams{DeploymentType: caas.DeploymentDaemon}

	ctrl := s.setupRestClients(c,
		restClientSetUpAction{
			gv: gvApps, doAssert: func(restC *providermocks.MockRestClientInterface) {},
		},
		restClientSetUpAction{
			gv: gvCore, doAssert: func(restC *providermocks.MockRestClientInterface) {},
		},
	)

	defer ctrl.Finish()

	ctx, cancel := context.WithTimeout(context.Background(), testing.LongWait)
	defer cancel()

	gomock.InOrder(
		// Get mapping for all resources.
		s.mockRestMapper.EXPECT().
			RESTMapping(apischema.GroupKind{Kind: "Deployment", Group: "apps"}, "v1").
			DoAndReturn(
				func(gk apischema.GroupKind, version string) (*meta.RESTMapping, error) {
					return restMapping(meta.RESTScopeNameNamespace, gk, version), nil
				},
			),
		s.mockRestMapper.EXPECT().
			RESTMapping(apischema.GroupKind{Kind: "Service"}, "v1").
			DoAndReturn(
				func(gk apischema.GroupKind, version string) (*meta.RESTMapping, error) {
					return restMapping(meta.RESTScopeNameNamespace, gk, version), nil
				},
			),
	)
	c.Assert(s.builder.Deploy(ctx, rawK8sSpec, true), gc.ErrorMatches, `empty "daemonsets" resource definition not valid`)
}

func objBody(c *gc.C, object interface{}) io.ReadCloser {
	output, err := json.MarshalIndent(object, "", "")
	c.Assert(err, jc.ErrorIsNil)
	return ioutil.NopCloser(bytes.NewReader(output))
}

func (s *builderSuite) fakeRequest(
	c *gc.C,
	gvk apischema.GroupVersionKind,
	expectedErr error,
	expectedResponse *http.Response,
	expectedURL string,
) *rest.Request {
	fakeClient := fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
		if expectedErr != nil {
			return nil, expectedErr
		}
		if len(expectedResponse.Header) == 0 {
			expectedResponse.Header = http.Header{}
			expectedResponse.Header.Set("Content-Type", runtime.ContentTypeJSON)
		}
		if expectedResponse.Body == nil {
			expectedResponse.Body = objBody(c, &metav1.APIVersions{
				Versions: []string{gvk.Version},
				TypeMeta: metav1.TypeMeta{
					APIVersion: gvk.Version,
					Kind:       gvk.Kind,
				},
			})
		}
		// Check labels and annotations are set correctly.
		reqBody, err := req.GetBody()
		c.Assert(err, jc.ErrorIsNil)
		defer reqBody.Close()
		reqBodyData, err := ioutil.ReadAll(reqBody)
		c.Assert(err, jc.ErrorIsNil)
		reqObj, err := runtime.Decode(unstructured.UnstructuredJSONScheme, reqBodyData)
		c.Assert(err, jc.ErrorIsNil)

		metadataAccessor := meta.NewAccessor()
		labels, err := metadataAccessor.Labels(reqObj)
		c.Assert(err, jc.ErrorIsNil)
		labelsSet := k8slabels.Set(labels)
		for k, v := range s.labels {
			// Check all Juju's labels are set into the obj.
			c.Check(labelsSet.Get(k), gc.DeepEquals, v)
		}

		// Check all Juju's annotations are set into the obj.
		annotations, err := metadataAccessor.Annotations(reqObj)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(k8sannotations.New(annotations).HasAll(s.annotations), jc.IsTrue)

		// Check the namespace in raw spec has been reset from `ns-will-be-overwritten` to `test`.
		ns, err := metadataAccessor.Namespace(reqObj)
		c.Assert(ns, gc.DeepEquals, s.namespace)

		c.Assert(req.URL.String(), gc.DeepEquals, expectedURL)
		return expectedResponse, nil
	})

	return rest.NewRequestWithClient(
		&url.URL{Scheme: "https", Host: "1.1.1.1"},
		gvk.Version,
		rest.ClientContentConfig{
			GroupVersion: gvk.GroupVersion(),
			Negotiator:   runtime.NewClientNegotiator(scheme.Codecs.WithoutConversion(), gvk.GroupVersion()),
		},
		fakeClient,
	)
}

type scopeGetter struct {
	meta.RESTScope
	scope meta.RESTScopeName
}

func (sg scopeGetter) Name() meta.RESTScopeName {
	return sg.scope
}

func kindToResource(kind string) string {
	return strings.ToLower(kind) + "s"
}

func restMapping(scope meta.RESTScopeName, gk apischema.GroupKind, version string) *meta.RESTMapping {
	return &meta.RESTMapping{
		Resource: apischema.GroupVersionResource{
			Group:    gk.Group,
			Version:  version,
			Resource: kindToResource(gk.Kind),
		},
		GroupVersionKind: apischema.GroupVersionKind{
			Group:   gk.Group,
			Version: version,
			Kind:    gk.Kind,
		},
		Scope: scopeGetter{scope: scope},
	}
}
