# Juju API Design Specification

## Introduction

### Purpose

The *Juju API* is the central interface to control the functionality of Juju. It 
is used by the command line tools as well as by the Juju GUI. This document 
describes the API design for developers maintaining and extending the API.

### Scope

Juju's working model is based on the description of a wanted environment in a 
DBMS. Different workers observe the changes of this description and perform the 
needed actions so that the reality is matching. Goal of the API is to provide a 
secure, scalable, and high-available way to perform the needed changes or queries 
with different clients like the Juju CLI, which is written in Go, or the Juju 
GUI, which is written in JavaScript. Additionally the API provides multiple versions 
of its functionality for a predictable behavior in environments with a mixed set of 
clients.

### Overview

This document provides a detailed specification of 

- the basic components of the API, 
- the types of requests,
- the data format of requests and responses, 
- the marshaling and unmarshaling of requests and responses, 
- the dispatching of requests to their according business logic, and 
- the versioning of the API functionality.

### Reference Material

*TODO: Check if there are other documents or links to reference.*

### Definitions and Acronyms

Facade
: A registered type providing a grouped set of methods which can be addressed 
by API requests.
JSON
: JavaScript Object Notation
RPC
: Remote Procedure Calls
WebSocket
: A full-duplex TCP communication protocol standardized by the W3C.

## System Overview

The Juju API is based on a request/response model using websockets as communication
layer. Each client connects to the API server of the environment. In case of 
high-availability an environment provides multiple API servers and a client connects
to one of them.

The requests sent via this connection are encoded in JSON. They contain a type
responsible for anwering the request, the individual request method and possible
parameters. Additionally they contain a unique identifier which allows the caller
to identify responses to asynchronous requests.

The API handles two different major types of requests:

1. simple requests, possible returning a response, and
2. watcher requests, observing collections or individual entities and notifying on changes.

Even requests without a response return at least an empty resonse envelope. So
the coaller gets notified when the request has finished.

## System Architecture

Core part of the API architecture is the *RPC* subsystem providing the functionality
for communication and request dispatching. The latter isn't done statically. Instead
a registered root object is is responsible for this task. This way different implementations
can be used for tests and production. The subsystem also provides helpers to dynamically
resolve the request to a method of a responsible instance and call it with possible
request parameters. In case of a returned value the subsystem also marshals this and
send it back as an answer to the caller.

The *API server* subsystem implements the server side of the API. It provides a root
object for the *RPC* subsystem using a global registry. Here factory methods are registered
for request types and versions. 

When receiving a request the *RPC* uses a defined finder method of the root object 
interface to retrieve a *method caller* instance matching to the type, version and 
request information that are part of the request. In case of success the *RPC* executes 
a call of that method together with an optional ID of the typed instance and also
optional arguments. The root object of the *API server* returns a method caller 
which uses the factory method registered for type and version. This factory method
is called with the *state*, *resources* and an *authorizer* as arguments. The resources 
hold all the resources for a connection and will be cleaned up when the connection 
terminates. The authorizer represents a value that can be asked for authorization 
information on its associated authenticated entity. It allows an API 
implementation to ask questions about the client that is currently connected.

The result of calling the factory method is an initialized new instance of the request
type to handle the request itself. The *RPC* subsystem maps the request name, which is
part of the request, to one of the methods of this instance. There is a number of valid 
methods signatures, depending on the possible combination of calling parameters, responses, 
and errors (see description in *Component Design*). 

*TODO: Go API client description.*

## Data Design

### Message

Messages are encoded in JSON, the same format is used for requests and responses.
All fields but the request ID are allowed to be empty.

- **RequestId** (Number) holds the sequence number of the request.
- **Type** (String) holds the type of object to act on.
- **Version** (Number) holds the version of Type we will be acting on.
- **Id** (String) holds the ID of the object to act on.
- **Request** (String) holds the action to perform on the object.
- **Params** (JSON) holds an optional parameter as JSON structure.
- **Error** (String) holds the error, if any.
- **ErrorCode** (String) holds the code of the error, if any.
- **Response** (JSON) holds an optional response as JSON structure.

## Component Design

### RPC

Core package for the API is [rpc](https://github.com/juju/juju/tree/master/rpc).
It defines the `Codec` interface for the reading and writing of messages in an RPC
session and the `MethodFinder` interface to retrieve the method to call for a request.
The endpoint type `Conn` uses implementations of `Codec` and `MethodFinder`. This way
diferent implementations can be used. 

The standard `Codec` is implemented in [jsoncodec](https://github.com/juju/juju/tree/master/rpc/jsoncodec).
It uses WebSockets for communication and JSON for encoding. The standard `MethodFinder`
is implemented in [apiserver](https://github.com/juju/juju/tree/master/state/apiserver) (see
below).

The `MethodFinder` has to implement a method with the signature

```
FindMethod(typeName string, version int, methodName string) (rpcreflect.MethodCaller, error)
```

The returned `MethodCaller` is an interface implementing the method

```
Call(objId string, arg reflect.Value) (reflect.Value, error)
```

beside helpers returning the parameter and the result type. It executes the calling of the
request method on an initialized instance matching to type, version and request name
together with the request parameters and returning the request result. Those request
methods implement the business logic and must follow on of the following signatures
depending on the combination of parameter, result, and error.

- `RequestMethod()`
- `RequestMethod() ResultType`
- `RequestMethod() (ResultType, error)`
- `RequestMethod() error`
- `RequestMethod(ParameterType)`
- `RequestMethod(ParameterType) ResultType`
- `RequestMethod(ParameterType) (ResultType, error)`
- `RequestMethod(ParameterType) error`

Both, `ParameterType` and `ResultType` have to be structs. Possible results and
errors are marshaled again to JSON and wrapped into an envelope containing also the 
request identifier.

#### Example

If the client wants to change the addresses of a number of machines, it sends the
request:

```
{
	RequestId: 1234,
	Type: "Machiner",
	Version: 0,
	Request: "SetMachineAddresses",
	Params: {
		MachineAddresses: [
			{Tag: "machine-1", Addresses: ...},
			{Tag: "machine-2", Addresses: ...}
		]
	}
}
```

In this case the RPC will create and instance of `apiserver.MachinerAPI` in the
requested version. This type provides a method `SetMachineAddresses()` with
`params.SetMachinesAddresses` as argument, and `params.ErrorResults` and `error`
as results. The parameters will be unmarshalled to the argument type and
the method called with it.

### API Server

The business logic itself is implemented in 
[apiserver](https://github.com/juju/juju/tree/master/state/apiserver) 
and its sub-packages. They are using the types in 
[state](https://github.com/juju/juju/tree/master/state) representing 
the Juju model.

### API

The according client logic used by the Juju CLI and the Juju daemon, which 
are also developed in Go, is located in [api](https://github.com/juju/juju/tree/master/state/api).

## Human Interface Design

### Overview of User Interface

### Screen Images

### Screen Objects and Actions

## Requirements Matrix

## Apendencies
