
# pubsub
    import "github.com/juju/pubsub"

Package pubsub provides publish and subscribe functionality within a single process.

A message as far as a hub is concerned is defined by a topic, and a data
blob. All subscribers that match the published topic are notified, and have
their callback function called with both the topic and the data blob.

All subscribers get their own goroutine. This way slow consumers do not
slow down the act of publishing, and slow consumers do not inferfere with
other consumers. Subscribers are guaranteed to get the messages that match
their topic matcher in the order that the messages were published to the
hub.

This package defines two types of hubs.
* Simple hubs
* Structured hubs

Simple hubs just pass the datablob to the subscribers untouched.
Structuctured hubs will serialize the datablob into a
`map[string]interface{}` using the marshaller that was defined to create
it. The subscription handler functions for structured hubs allow the
handlers to define a structure for the datablob to be marshalled into.

Hander functions for a structured hub can get all the published data available
by defining a callback with the signature:


	func (Topic, map[string]interface{})

Or alternatively, define a struct type, and use that type as the second argument.


	func (Topic, SomeStruct, error)

The structured hub will try to serialize the published information into the
struct specified. If there is an error marshalling, that error is passed to
the callback as the error parameter.





## Variables
``` go
var JSONMarshaller = &jsonMarshaller{}
```
JSONMarshaller simply wraps the json.Marshal and json.Unmarshal calls for the
Marshaller interface.



## type Marshaller
``` go
type Marshaller interface {
    // Marshal converts the argument into a byte streem that it can then Unmarshal.
    Marshal(interface{}) ([]byte, error)

    // Unmarshal attempts to convert the byte stream into type passed in as the
    // second arg.
    Unmarshal([]byte, interface{}) error
}
```
Marshaller defines the Marshal and Unmarshal methods used to serialize and
deserialize the structures used in Publish and Subscription handlers of the
structured hub.











## type Multiplexer
``` go
type Multiplexer interface {
    TopicMatcher
    Add(matcher TopicMatcher, handler interface{}) error
}
```
Multiplexer allows multiple subscriptions to be made sharing a single
message queue from the hub. This means that all the messages for the
various subscriptions are called back in the order that the messages were
published. If more than one handler is added to the Multiplexer that
matches any given topic, the handlers are called back one after the other
in the order that they were added.











## type RegexpMatcher
``` go
type RegexpMatcher regexp.Regexp
```
RegexpMatcher allows standard regular expressions to be used as
TopicMatcher values. RegexpMatches can be created using the short-hand
function MatchRegexp function that wraps regexp.MustCompile.











### func (\*RegexpMatcher) Match
``` go
func (m *RegexpMatcher) Match(topic Topic) bool
```
Match implements TopicMatcher.

The topic matches if the regular expression matches the topic.



## type SimpleHub
``` go
type SimpleHub struct {
    // contains filtered or unexported fields
}
```
SimpleHub provides the base functionality of dealing with subscribers,
and the notification of subscribers of events.









### func NewSimpleHub
``` go
func NewSimpleHub(config *SimpleHubConfig) *SimpleHub
```
NewSimpleHub returns a new SimpleHub instance.

A simple hub does not touch the data that is passed through to Publish.
This data is passed through to each Subscriber. Note that all subscribers
are notified in parallel, and that no modification should be done to the
data or data races will occur.




### func (\*SimpleHub) Publish
``` go
func (h *SimpleHub) Publish(topic Topic, data interface{}) <-chan struct{}
```
Publish will notifiy all the subscribers that are interested by calling
their handler function.

The data is passed through to each Subscriber untouched. Note that all
subscribers are notified in parallel, and that no modification should be
done to the data or data races will occur.

The channel return value is closed when all the subscribers have been
notified of the event.



### func (\*SimpleHub) Subscribe
``` go
func (h *SimpleHub) Subscribe(matcher TopicMatcher, handler func(Topic, interface{})) Unsubscriber
```
Subscribe takes a topic matcher, and a handler function. If the matcher
matches the published topic, the handler function is called with the
published Topic and the associated data.

The handler function will be called with all maching published events until
the Unsubscribe method on the Unsubscriber is called.



## type SimpleHubConfig
``` go
type SimpleHubConfig struct {
    // LogModule allows for overriding the default logging module.
    // The default value is "pubsub.simple".
    LogModule string
}
```
SimpleHubConfig is the argument struct for NewSimpleHub.











## type StructuredHub
``` go
type StructuredHub struct {
    // contains filtered or unexported fields
}
```
StructuredHub allows the hander functions to accept either structures
or map[string]interface{}. The published structure does not need to match
the structures of the subscribers. The structures are marshalled using the
Marshaller defined in the StructuredHubConfig. If one is not specified, the
marshalling is handled by the standard json library.









### func NewStructuredHub
``` go
func NewStructuredHub(config *StructuredHubConfig) *StructuredHub
```
NewStructuredHub returns a new StructuredHub instance.




### func (\*StructuredHub) NewMultiplexer
``` go
func (h *StructuredHub) NewMultiplexer() (Unsubscriber, Multiplexer, error)
```
NewMultiplexer creates a new multiplexer for the hub and subscribes it.
Unsubscribing the multiplexer stops calls for all handlers added.
Only structured hubs support multiplexer.



### func (\*StructuredHub) Publish
``` go
func (h *StructuredHub) Publish(topic Topic, data interface{}) (<-chan struct{}, error)
```
Publish will notifiy all the subscribers that are interested by calling
their handler function.

The data is serialized out using the marshaller and then back into  a
map[string]interface{}. If there is an error marshalling the data, Publish
fails with an error.  The resulting map is then updated with any
annotations provided. The annotated values are only set if the specified
field is missing or empty. After the annotations are set, the PostProcess
function is called if one was specified. The resulting map is then passed
to each of the subscribers.

Subscribers are notified in parallel, and that no
modification should be done to the data or data races will occur.

The channel return value is closed when all the subscribers have been
notified of the event.



### func (\*StructuredHub) Subscribe
``` go
func (h *StructuredHub) Subscribe(matcher TopicMatcher, handler interface{}) (Unsubscriber, error)
```
Subscribe takes a topic matcher, and a handler function. If the matcher
matches the published topic, the handler function is called with the
published Topic and the associated data.

The handler function will be called with all maching published events until
the Unsubscribe method on the Unsubscriber is called.

The hander function must have the signature:


	`func(Topic, map[string]interface{})`

or


	`func(Topic, SomeStruct, error)`

where `SomeStruct` is any structure. The map[string]interface{} from the
Publish call is unmarshalled into the `SomeStruct` structure. If there is
an error unmarshalling the handler is called with a zerod structure and an
error with the marshalling error.



## type StructuredHubConfig
``` go
type StructuredHubConfig struct {
    // LogModule allows for overriding the default logging module.
    // The default value is "pubsub.structured".
    LogModule string

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
```
StructuredHubConfig is the argument struct for NewStructuredHub.











## type Topic
``` go
type Topic string
```
Topic represents a message that can be subscribed to.











### func (Topic) Match
``` go
func (t Topic) Match(topic Topic) bool
```
Match implements TopicMatcher. One topic matches another if they
are equal.



## type TopicMatcher
``` go
type TopicMatcher interface {
    Match(Topic) bool
}
```
TopicMatcher defines the Match method that is used to determine
if the subscriber should be notified about a particular message.





``` go
var MatchAll TopicMatcher = (*allMatcher)(nil)
```
MatchAll is a topic matcher that matches all topics.





### func MatchRegexp
``` go
func MatchRegexp(expression string) TopicMatcher
```
MatchRegexp expects a valid regular expression. If the expression
passed in is not valid, the function panics. The expected use of this
is to be able to do something like:


	hub.Subscribe(pubsub.MatchRegex("prefix.*suffix"), handler)




## type Unsubscriber
``` go
type Unsubscriber interface {
    Unsubscribe()
}
```
Unsubscriber provides a way to stop receiving handler callbacks.
Unsubscribing from a hub will also mark any pending notifications as done,
and the handler will not be called for them.

















- - -
Generated by [godoc2md](http://godoc.org/github.com/davecheney/godoc2md)