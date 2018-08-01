// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// Package pubsub provides publish and subscribe functionality within a single process.
//
// A message as far as a hub is concerned is defined by a topic, and a data
// blob. All subscribers that match the published topic are notified, and have
// their callback function called with both the topic and the data blob.
//
// All subscribers get their own goroutine. This way slow consumers do not
// slow down the act of publishing, and slow consumers do not inferfere with
// other consumers. Subscribers are guaranteed to get the messages that match
// their topic matcher in the order that the messages were published to the
// hub.
//
// This package defines two types of hubs.
// * Simple hubs
// * Structured hubs
//
// Simple hubs just pass the datablob to the subscribers untouched.
// Structuctured hubs will serialize the datablob into a
// `map[string]interface{}` using the marshaller that was defined to create
// it. The subscription handler functions for structured hubs allow the
// handlers to define a structure for the datablob to be marshalled into.
//
// Hander functions for a structured hub can get all the published data available
// by defining a callback with the signature:
//   func (Topic, map[string]interface{})
//
// Or alternatively, define a struct type, and use that type as the second argument.
//   func (Topic, SomeStruct, error)
//
// The structured hub will try to serialize the published information into the
// struct specified. If there is an error marshalling, that error is passed to
// the callback as the error parameter.
package pubsub
