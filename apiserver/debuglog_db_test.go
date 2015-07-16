// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import gc "gopkg.in/check.v1"

// debugLogDBSuite runs the common debuglog API tests when the db-log
// feature flag is enabled. These tests are inherited from
// debugLogBaseSuite.
type debugLogDBSuite struct {
	debugLogBaseSuite
}

var _ = gc.Suite(&debugLogDBSuite{})

func (s *debugLogDBSuite) SetUpSuite(c *gc.C) {
	s.SetInitialFeatureFlags("db-log")
	s.debugLogBaseSuite.SetUpSuite(c)
}

// See debuglog_db_internal_test.go for DB specific unit tests and the
// featuretests package for an end-to-end integration test.
