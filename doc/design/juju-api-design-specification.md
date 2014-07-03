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

The *API server* subsystem implements the server side of the API. 

### Note: Order

1. `rpc.NewConn` with a codec return a connection
2. `connection.Serve` with a root object for the dispatching of requests
3. `connection.Start` to start serving
4. calling a named like the request *type* together with an instance ID 

## Data Design

## Component Design

### RPC

Core package for the API is [github.com/juju/juju/rpc](https://github.com/juju/juju/tree/master/rpc).
It provides a server type using WebSockets to receive requests in JSON, unmarshal 
them into Go structs and dispatch them using a global registry. This registry maps the 
name of the type and the version, both are parts of each request, to a factory method
with the signature

```
FactoryMethod(st *state.State,r *Resources, a Authorizer, id string) (interface{}, error)
```

The individual requests are then mapped to action methods of these request handler 
instances. Here the business logic is implemented.

Those methods must implement one of the following signatures:

- `RequestMethod()`
- `RequestMethod() ResponseType`
- `RequestMethod() (ResponseType, error)`
- `RequestMethod() error`
- `RequestMethod(ParameterType)`
- `RequestMethod(ParameterType) ResponseType`
- `RequestMethod(ParameterType) (ResponseType, error)`
- `RequestMethod(ParameterType) error`

Both, `ParameterType` and `ResponseType` have to be structs. Possible responses and
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
[github.com/juju/juju/state/apiserver](https://github.com/juju/juju/tree/master/state/apiserver) 
and its sub-packages. They are using the types in 
[github.com/juju/juju/state](https://github.com/juju/juju/tree/master/state) representing 
the Juju model.

### API

The according client logic used by the Juju CLI and the Juju daemon, which 
are also developed in Go, is located in 
[github.com/juju/juju/state/api](https://github.com/juju/juju/tree/master/state/api).

## Human Interface Design

### Overview of User Interface

### Screen Images

### Screen Objects and Actions

## Requirements Matrix

## Apendencies
