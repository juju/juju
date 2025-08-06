// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	gc "gopkg.in/check.v1"
	"k8s.io/client-go/kubernetes"
)

type resourceSuite struct {
	coreClient kubernetes.Interface
}

func (s *resourceSuite) SetUpTest(c *gc.C) {
	s.coreClient = newCombinedClientSet()
}

func (s *resourceSuite) TearDownTest(c *gc.C) {
	s.coreClient = nil
}
