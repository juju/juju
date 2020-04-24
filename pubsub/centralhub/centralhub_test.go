// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package centralhub_test

import (
	"time"

	"github.com/juju/names/v4"
	"github.com/juju/pubsub"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/pubsub/centralhub"
	"github.com/juju/juju/testing"
)

type CentralHubSuite struct{}

var _ = gc.Suite(&CentralHubSuite{})

func (*CentralHubSuite) waitForSubscribers(c *gc.C, done <-chan struct{}) {
	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("subscribers not finished")
	}
}

func (s *CentralHubSuite) TestSetsOrigin(c *gc.C) {
	hub := centralhub.New(names.NewControllerAgentTag("42"))
	topic := "testing"
	var called bool
	unsub, err := hub.SubscribeMatch(pubsub.MatchAll, func(t string, data map[string]interface{}) {
		c.Check(t, gc.Equals, topic)
		expected := map[string]interface{}{
			"key":    "value",
			"origin": "controller-42",
		}
		c.Check(data, jc.DeepEquals, expected)
		called = true
	})

	c.Assert(err, jc.ErrorIsNil)
	defer unsub()

	done, err := hub.Publish(topic, map[string]interface{}{"key": "value"})
	c.Assert(err, jc.ErrorIsNil)
	s.waitForSubscribers(c, done)
	c.Assert(called, jc.IsTrue)
}

type IntStruct struct {
	Key int `json:"key"`
}

func (s *CentralHubSuite) TestYAMLMarshalling(c *gc.C) {
	hub := centralhub.New(names.NewMachineTag("42"))
	topic := "testing"
	var called bool
	unsub, err := hub.SubscribeMatch(pubsub.MatchAll, func(t string, data map[string]interface{}) {
		c.Check(t, gc.Equals, topic)
		expected := map[string]interface{}{
			"key":    1234,
			"origin": "machine-42",
		}
		c.Check(data, jc.DeepEquals, expected)
		called = true
	})

	c.Assert(err, jc.ErrorIsNil)
	defer unsub()

	// With the default JSON marshalling, integers are marshalled to floats into the map.
	done, err := hub.Publish(topic, IntStruct{1234})
	c.Assert(err, jc.ErrorIsNil)
	s.waitForSubscribers(c, done)
	c.Assert(called, jc.IsTrue)
}

type NestedStruct struct {
	Key    string    `yaml:"key"`
	Nested IntStruct `yaml:"nested"`
}

func (s *CentralHubSuite) TestPostProcessingMaps(c *gc.C) {
	// Due to the need to send the resulting maps over the API, nested structs
	// need to be map[string]interface{} not map[interface{}]interface{},
	// which is what the YAML marshaller will give us.
	hub := centralhub.New(names.NewMachineTag("42"))
	topic := "testing"
	var called bool
	unsub, err := hub.SubscribeMatch(pubsub.MatchAll, func(t string, data map[string]interface{}) {
		c.Check(t, gc.Equals, topic)
		expected := map[string]interface{}{
			"key": "value",
			"nested": map[string]interface{}{
				"key": 1234,
			},
			"origin": "machine-42",
		}
		c.Check(data, jc.DeepEquals, expected)
		called = true
	})

	c.Assert(err, jc.ErrorIsNil)
	defer unsub()

	// With the default JSON marshalling, integers are marshalled to floats into the map.
	done, err := hub.Publish(topic, NestedStruct{
		Key:    "value",
		Nested: IntStruct{1234}})
	c.Assert(err, jc.ErrorIsNil)
	s.waitForSubscribers(c, done)
	c.Assert(called, jc.IsTrue)
}
