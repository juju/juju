// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package pubsub

import (
	"encoding/json"
	"reflect"

	"github.com/juju/errors"
)

// StructuredHub allows the hander functions to accept either structures
// or map[string]interface{}. The published structure does not need to match
// the structures of the subscribers. The structures are marshalled using the
// Marshaller defined in the StructuredHubConfig. If one is not specified, the
// marshalling is handled by the standard json library.
type StructuredHub struct {
	hub SimpleHub

	marshaller  Marshaller
	annotations map[string]interface{}
	postProcess func(map[string]interface{}) (map[string]interface{}, error)
}

// Marshaller defines the Marshal and Unmarshal methods used to serialize and
// deserialize the structures used in Publish and Subscription handlers of the
// structured hub.
type Marshaller interface {
	// Marshal converts the argument into a byte streem that it can then Unmarshal.
	Marshal(interface{}) ([]byte, error)

	// Unmarshal attempts to convert the byte stream into type passed in as the
	// second arg.
	Unmarshal([]byte, interface{}) error
}

// StructuredHubConfig is the argument struct for NewStructuredHub.
type StructuredHubConfig struct {
	// Logger allows specifying a logging implementation for debug
	// and trace level messages emitted from the hub.
	Logger Logger

	// Marshaller defines how the structured hub will convert from structures to
	// a map[string]interface{} and back. If this is not specified, the
	// `JSONMarshaller` is used.
	Marshaller Marshaller

	// Annotations are added to each message that is published if and only if
	// the values are not already set.
	Annotations map[string]interface{}

	// PostProcess allows the caller to modify the resulting
	// map[string]interface{}. This is useful when a dynamic value, such as a
	// timestamp is added to the map, or when other type conversions are
	// necessary across all the values in the map.
	PostProcess func(map[string]interface{}) (map[string]interface{}, error)
}

// JSONMarshaller simply wraps the json.Marshal and json.Unmarshal calls for the
// Marshaller interface.
var JSONMarshaller = &jsonMarshaller{}

type jsonMarshaller struct{}

func (*jsonMarshaller) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (*jsonMarshaller) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// NewStructuredHub returns a new StructuredHub instance.
func NewStructuredHub(config *StructuredHubConfig) *StructuredHub {
	if config == nil {
		config = new(StructuredHubConfig)
	}
	logger := config.Logger
	if logger == nil {
		logger = noOpLogger{}
	}
	if config.Marshaller == nil {
		config.Marshaller = JSONMarshaller
	}
	return &StructuredHub{
		hub: SimpleHub{
			logger: logger,
		},
		marshaller:  config.Marshaller,
		annotations: config.Annotations,
		postProcess: config.PostProcess,
	}
}

// Publish will notifiy all the subscribers that are interested by calling
// their handler function.
//
// The data is serialized out using the marshaller and then back into  a
// map[string]interface{}. If there is an error marshalling the data, Publish
// fails with an error.  The resulting map is then updated with any
// annotations provided. The annotated values are only set if the specified
// field is missing or empty. After the annotations are set, the PostProcess
// function is called if one was specified. The resulting map is then passed
// to each of the subscribers.
//
// Subscribers are notified in parallel, and that no
// modification should be done to the data or data races will occur.
//
// The channel return value is closed when all the subscribers have been
// notified of the event.
func (h *StructuredHub) Publish(topic string, data interface{}) (<-chan struct{}, error) {
	if data == nil {
		data = make(map[string]interface{})
	}
	asMap, err := h.toStringMap(data)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for key, defaultValue := range h.annotations {
		if value, exists := asMap[key]; !exists || value == reflect.Zero(reflect.TypeOf(value)).Interface() {
			asMap[key] = defaultValue
		}
	}
	if h.postProcess != nil {
		asMap, err = h.postProcess(asMap)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	h.hub.logger.Tracef("publish %q: %#v", topic, asMap)
	return h.hub.Publish(topic, asMap), nil
}

func (h *StructuredHub) toStringMap(data interface{}) (map[string]interface{}, error) {
	var result map[string]interface{}
	resultType := reflect.TypeOf(result)
	dataType := reflect.TypeOf(data)
	if dataType.AssignableTo(resultType) {
		cast, ok := data.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("%T assignable to map[string]interface{} but isn't one?", data)
		}
		return cast, nil
	}
	bytes, err := h.marshaller.Marshal(data)
	if err != nil {
		return nil, errors.Annotate(err, "marshalling")
	}
	err = h.marshaller.Unmarshal(bytes, &result)
	if err != nil {
		return nil, errors.Annotate(err, "unmarshalling")
	}
	return result, nil
}

// Subscribe takes a topic with a handler function. If the topic is the same
// as the published topic, the handler function is called with the published
// topic and the associated data.
//
// The function return value is a function that will unsubscribe the caller
// from the hub, for this subscription.
//
// The hander function must have the signature:
//   `func(string, map[string]interface{})`
// or
//   `func(string, SomeStruct, error)`
// where `SomeStruct` is any structure.
//
// If the hander function does not match one of these signatures, the Subscribe
// function returns an error.
//
// The map[string]interface{} from the
// Publish call is unmarshalled into the `SomeStruct` structure. If there is
// an error unmarshalling the handler is called with a zerod structure and an
// error with the marshalling error.
func (h *StructuredHub) Subscribe(topic string, handler interface{}) (func(), error) {
	return h.SubscribeMatch(equalTopic(topic), handler)
}

// SubscribeMatch takes a function that determins whether the topic matches,
// and a handler function. If the matcher matches the published topic, the
// handler function is called with the published topic and the associated
// data.
//
// All other aspects of the function are the same as the `Subscribe` method.
func (h *StructuredHub) SubscribeMatch(matcher func(string) bool, handler interface{}) (func(), error) {
	if matcher == nil {
		return nil, errors.NotValidf("missing matcher")
	}
	callback, err := newStructuredCallback(h.hub.logger, h.marshaller, handler)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return h.hub.SubscribeMatch(matcher, callback.handler), nil
}
