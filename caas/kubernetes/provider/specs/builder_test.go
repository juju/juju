// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	"context"
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	// core "k8s.io/api/core/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/caas/kubernetes/provider/mocks"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/testing"
	// "github.com/juju/juju/caas/kubernetes/provider"
)

type builderSuite struct {
	testing.BaseSuite
	mockRestClient *mocks.MockRestClientInterface
}

var _ = gc.Suite(&builderSuite{})

func (s *builderSuite) setupExecClient(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockRestClient = mocks.NewMockRestClientInterface(ctrl)
	return ctrl
}

func (s *builderSuite) TestDeploy(c *gc.C) {
	ctrl := s.setupExecClient(c)
	defer ctrl.Finish()

	var rawK8sSpec = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  labels:
    app: nginx
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
        - containerPort: 80
`[1:]

	builder := k8sspecs.NewDeployer(
		"test", &rest.Config{}, func(*rest.Config) (rest.Interface, error) {
			return s.mockRestClient, nil
		}, rawK8sSpec,
	)
	s.mockRestClient.EXPECT().Get().Return(nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Assert(builder.Deploy(ctx, true), jc.ErrorIsNil)
}
