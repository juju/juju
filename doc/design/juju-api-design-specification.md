# Juju API Design Specification

## Status

*Work in Progress*

## Contents

1. [Introduction](#introduction)
2. [Requirements](#requirements)
3. [System Overview](#system-overview)
4. [System Architecture](#system-architecture)
5. [Data Design](#data-design)
6. [Component Design](#component-design)

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

*TODO: Check if there are documents or links to reference.*

### Definitions and Acronyms

- **Facade** A registered and versioned type which is addressed by API 
  requests. It creates an instance providing a set of methods for one 
  entity or a cohesive functionality.
- **JSON** JavaScript Object Notation
- **RPC** Remote Procedure Calls
- **WebSocket** A full-duplex TCP communication protocol standardized by
  the W3C ([see RfC 6455](http://tools.ietf.org/html/rfc6455)).

## Requirements

- As a user of Trusty, I want to be able to bootstrap an environment today, and be 
  able to trust that in 2 years I will still be able to access/control/update that 
  environment even when I’ve upgraded my juju client tools.
- As a group with heterogeneous clients, we want to be able to bootstrap a new 
  environment, and trust that all clients are still able to connect and manage the 
  environment.
- As a user with a new Juju client, I want to be able to bootstrap a new environment 
  and have access to all the latest features for that environment. (HA, User accounts, 
  etc.)
- As an Agent, I want to be able to communicate with the API server to be able to perform 
  my regular tasks, and be able to upgrade to stay in sync with the desired agent version. 
  It is expected that we won’t always be able to stay in perfect synchronization, 
  especially in an environment with heterogeneous architectures and platforms.
- As a developer, I want to be able to make a change to an existing named API in such 
  a fashion that new clients are able to make use of the new functionality, but older 
  clients can still use the API in a compatible fashion.

## System Overview

### WebSockets

The major functionality of the Juju API is based on a request/response model 
using WebSockets as communication layer. Each client connects to the API server 
of the environment. In case of high-availability an environment provides multiple 
API servers and a client connects to one of them.

The requests sent via this connection are encoded in JSON. They contain a type
responsible for anwering the request, the individual request method and possible
parameters. Additionally they contain a unique identifier which allows the caller
to identify responses to asynchronous requests.

This part of the API handles two different major types of requests:

1. simple requests, which may return a response, and
2. watcher requests for the observation of changes to collections or individual 
   entities.

Watcher requests are also request/response calls. But they create server-side
resources which respond to future `Next()` calls. Those retrieve already happened
changes or wait for the next ones and return them to the caller.

Another handler using WebSockets for delivering a stream of data is the
debug log handler. It opens the `all-machines.log` and continuously streams
the data to the client.

### HTTP Requests

Beside the communication using WebSockets there are several parts of the
API using standard HTTP requests. Individual handlers registered for the
according paths care for those requests.

The first one is the charm handler, which supports HTTP POST to add a local 
charm to the store provider and HTTP GET to retrieve or list charm files. The
second one is the tools handler, which supports HTTP POST for tje upload of
tools to the API server. Last but not least the API provides a backup handler
which allows to use the storage for the backup of files via HTTP POST.

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

API messages are encoded in JSON. The type `rpc.Request` represents
a remote procedure call to be performed, absent its parameters. Those
can be found in the type `rpc.Call`, which represents an active
remote procedue call and embeds the request as well as other important
parts of the request.

#### Request

- **RequestId** (Number) holds the sequence number of the request.
- **Type** (String) holds the type of object to act on.
- **Version** (Number) holds the version of Type we will be acting on.
- **Id** (String) is the ID of a watched object; future implementations
  pass one ore more IDs as parameters.
- **Request** (String) holds the action to perform on the object.
- **Params** (JSON) holds the parameters as JSON structure, each request
  implementation out to accept bulk requests.

#### Response

- **RequestId** (Number) holds the sequence number of the request.
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

Both `ParameterType` and `ResultType` have to be structs. Possible results and
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

The API server is implemented in the package [apiserver](https://github.com/juju/juju/tree/master/state/apiserver)
and its sub-packages. It is started inside the *machine agent* by calling `apiserver.NewServer()`.
The returned `Server` instance holds the server side of the API. After starting
the server several handlers are registered. One of them is the API server
by registering `Server.apiHandler()` as handler function. It receives the
HTTP request and starts the individual WebSockets connection. Inside of
`Server.serveConn()` it uses the WebSocket to etablish the RPC using 
the JSON codec. 

In case of a valid environment UUID a new server state server instance is
created using `apiserver.initialRoot` with an `apiserver.srvAdmin` for the
login process. If this is successful the root is changed to `apiserver.srvRoot`
for the real API request handling. It implements `FindMethod()` which is needed
by the RPC to dispatch request to the according methods like described above.

#### Facade Registration

The registry for facades is implemented in the package
[apiserver/common](https://github.com/juju/juju/tree/master/state/apiserver/common). It provides
a function for the registration of facade constructor functions together with
their name and a version number. They are called in an `init()` function in
their respective packages.

```
func init() {
	common.RegisterStandardFacade("Machiner", 0, NewMachinerAPI)
	common.RegisterStandardFacade("Machiner", 1, NewMachinerAPIv1)
}
```

### API Client

The according client logic used by the Juju CLI and the Juju daemon, which are also
developed in Go, is located in [api](https://github.com/juju/juju/tree/master/state/api).

Clients connect with `api.Open()` which returns a `api.State` instance as entry point.
This instance can perform low level RPC calls with the method `Call()`. It also provide
the methods `AllFacadeVersion()` to retrieve a map of all facades with slices of all
their versions. More useful is the method `BestFacadeVersion()` to retrieve the best
available version for a named facade. So in case of no matching version a client can
react with an error instead of performing an invalid call.

*TODO: Describe how individual client calls are implemented.*

### Watching

*TODO: Describe watching using the API.*
