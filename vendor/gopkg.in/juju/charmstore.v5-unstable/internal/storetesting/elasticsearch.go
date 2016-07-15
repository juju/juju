// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storetesting // import "gopkg.in/juju/charmstore.v5-unstable/internal/storetesting"

import (
	"os"
	"time"

	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"gopkg.in/juju/charmstore.v5-unstable/elasticsearch"
)

// ElasticSearchSuite defines a test suite that connects to an
// elastic-search server. The address of the server depends on the value
// of the JUJU_TEST_ELASTICSEARCH environment variable, which can be
// "none" (do not start or connect to a server) or host:port holding the
// address and port of the server to connect to. If
// JUJU_TEST_ELASTICSEARCH is not specified then localhost:9200 will be
// used.
type ElasticSearchSuite struct {
	ES        *elasticsearch.Database
	indexes   []string
	TestIndex string
}

var jujuTestElasticSearch = os.Getenv("JUJU_TEST_ELASTICSEARCH")

func (s *ElasticSearchSuite) SetUpSuite(c *gc.C) {
	serverAddr := jujuTestElasticSearch
	switch serverAddr {
	case "none":
		c.Skip("elasticsearch disabled")
	case "":
		serverAddr = ":9200"
	}
	s.ES = &elasticsearch.Database{serverAddr}
}

func (s *ElasticSearchSuite) TearDownSuite(c *gc.C) {
}

func (s *ElasticSearchSuite) SetUpTest(c *gc.C) {
	s.TestIndex = s.NewIndex(c)
}

func (s *ElasticSearchSuite) TearDownTest(c *gc.C) {
	for _, index := range s.indexes {
		s.ES.DeleteIndex(index + "*")
		s.ES.DeleteDocument(".versions", "version", index)
	}
	s.indexes = nil
}

// NewIndex creates a new index name and ensures that it will be cleaned up at
// end of the test.
func (s *ElasticSearchSuite) NewIndex(c *gc.C) string {
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	id := time.Now().Format("20060102") + uuid.String()
	s.indexes = append(s.indexes, id)
	return id
}

// LoadESConfig loads a canned test configuration to the specified index
func (s *ElasticSearchSuite) LoadESConfig(index string, settings, mapping interface{}) error {
	if err := s.ES.PutIndex(index, settings); err != nil {
		return err
	}
	if err := s.ES.PutMapping(index, "entity", mapping); err != nil {
		return err
	}
	return nil
}
