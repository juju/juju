// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package pubsub

import (
	"reflect"

	"github.com/juju/errors"
)

type structuredCallback struct {
	logger     Logger
	marshaller Marshaller
	callback   reflect.Value
	dataType   reflect.Type
}

func newStructuredCallback(logger Logger, marshaller Marshaller, handler interface{}) (*structuredCallback, error) {
	rt, err := checkStructuredHandler(handler)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Tracef("new structured callback, return type %v", rt)
	return &structuredCallback{
		logger:     logger,
		marshaller: marshaller,
		callback:   reflect.ValueOf(handler),
		dataType:   rt,
	}, nil
}

func (s *structuredCallback) handler(topic string, data interface{}) {
	// The data is always map[string]interface{}.
	asMap := data.(map[string]interface{})
	s.logger.Tracef("convert map to %v", s.dataType)
	value, includeErr, err := toHanderType(s.marshaller, s.dataType, asMap)
	args := []reflect.Value{reflect.ValueOf(topic), value}
	if includeErr {
		// NOTE: you can't just use reflect.ValueOf(err) as that doesn't work
		// with nil errors. reflect.ValueOf(nil) isn't a valid value. So we need
		// to make  sure that we get the type of the parameter correct, which is
		// the error interface.
		errValue := reflect.Indirect(reflect.ValueOf(&err))
		args = append(args, errValue)
	}

	s.callback.Call(args)
}

func toHanderType(marshaller Marshaller, rt reflect.Type, data map[string]interface{}) (reflect.Value, bool, error) {
	mapType := reflect.TypeOf(data)
	if mapType == rt {
		return reflect.ValueOf(data), false, nil
	}
	sv := reflect.New(rt) // returns a Value containing *StructType
	bytes, err := marshaller.Marshal(data)
	if err != nil {
		return reflect.Indirect(sv), true, errors.Annotate(err, "marshalling data")
	}
	err = marshaller.Unmarshal(bytes, sv.Interface())
	if err != nil {
		return reflect.Indirect(sv), true, errors.Annotate(err, "unmarshalling data")
	}
	return reflect.Indirect(sv), true, nil
}

// checkStructuredHandler makes sure that the handler is a function that takes
// a Topic, a structure, and an error, or Topic and map[string]interface{}.
// Returns the reflect.Type for the structure.
func checkStructuredHandler(handler interface{}) (reflect.Type, error) {
	if handler == nil {
		return nil, errors.NotValidf("nil handler")
	}
	mapType := reflect.TypeOf(map[string]interface{}{})
	t := reflect.TypeOf(handler)
	if t.Kind() != reflect.Func {
		return nil, errors.NotValidf("handler of type %T", handler)
	}
	if t.NumOut() != 0 {
		return nil, errors.NotValidf("expected no return values, got %d, incorrect handler signature", t.NumOut())
	}
	if t.NumIn() < 2 || t.NumIn() > 3 {
		return nil, errors.NotValidf("expected 2 or 3 args, got %d, incorrect handler signature", t.NumIn())
	}

	var topic string
	var topicType = reflect.TypeOf(topic)
	if t.In(0) != topicType {
		return nil, errors.NotValidf("first arg should be a string, incorrect handler signature")
	}

	arg2 := t.In(1)
	if arg2 == mapType {
		// Special case the map case.
		if t.NumIn() == 3 {
			return nil, errors.NotValidf("data type of map[string]interface{} expects only 2 args, got 3, incorrect handler signature")
		}
		return arg2, nil
	}

	if arg2.Kind() != reflect.Struct {
		return nil, errors.NotValidf("second arg should be a structure or map[string]interface{} for data, incorrect handler signature")
	}
	if t.NumIn() != 3 {
		return nil, errors.NotValidf("structure handlers need an error arg, incorrect handler signature")
	}

	arg3 := t.In(2)
	if arg3.Kind() != reflect.Interface || arg3.Name() != "error" {
		return nil, errors.NotValidf("third arg should be error for deserialization errors, incorrect handler signature")
	}
	return arg2, nil
}
