# Juju API Implementation Guide

## Status

*Work in Progress*

## Contents

1. [Introduction](#introduction)
2. [Organization](#organization)
3. [Versioning](#versioning)
4. [Patterns](#patterns)

## Introduction

### Purpose

The *Juju API* is the central interface to control the functionality of Juju. It 
is used by the command line tools as well as by the Juju GUI. This document 
describes how functionality has to be added, modified, or removed.

### Scope

The [Juju API Design Specification](juju-api-design-specificaion.md) shows how the
core packages of the API are implemented. They provide a comunication between client
and server using WebSockets and a JSON marshalling. Additionally the API server
allows to register facades constructors for types and versions. Those are used to
dispatch the requests to the responsible methods.

This documents covers how those *factories*, *facades*, and *methods* have to be 
implemented and maintained. The goal here is a clean organization following common 
paterns while keeping the compatability to older versions.

### Overview

This document provides guide on

- how the code has to be organized into packages and types,
- how the versioning has to be implemented, and
- which patterns to follow when implementing API types and methods.

## Organization

The business logic of the API is grouped by the types of entities it's acting on.
This type is specified in the `Type` field of each request. Each type is represented
by a package which implements and registers a *factory* for each version it supports.
When a request for a given type is received the factory for the requested version is
used to create a *facade*. The facade is an instance of a type implementing a set of
*methods* for the handling of the requests as described in the specification document.

All API type packages are located inside the package 
[apiserver](https://github.com/juju/juju/tree/master/state/apiserver). Here you'll
e.g. find the package [agent](https://github.com/juju/juju/tree/master/state/apiserver/agent)
implementing the API interfaces used by the machine agent. As a first step it registers
the factory for version 0 of the facade with

```
func init() {
	common.RegisterStandardFacade("Agent", 0, NewAPI)
}
```

This initial step has to be done for each factory/facade pair in each package. In case 
of multiple versions all have to be registered here. So when the next one is added the 
init function additionally contains the line

```
common.RegisterStandardFacade("Agent", 1, NewAPIv1)
```

All factories implement the signature

```
func(st *state.State, resources *common.Resources, auth common.Authorizer) (facade interface{}, err error)
```

to initialize and return their individual facade. So e.g. In icase of the type *Agent* 
it is an instance of `agent.API`, in case of the *Machiner* an instance of `machine.MachinerAPI`.
Those types then implement the methods following according to the allowed signatures
like described in the specification so that the RPC package can distribute the requests
to them.

## Versioning

When implementing a new version of a facade, there are a couple of use cases:

1. Changing the meaning of an existing API without changing its signature (empty strings 
   passed to `Set` are considered to mean set the value to empty, rather than meaning revert 
   to default).
2. Changing the signature of an existing API (adding new parameters, removing parameters, 
   changing the return signature).
3. Adding a new API.
4. Removing an existing API (such as during a rename operation).

For (1), (2), and (3) it is possible to take the existing implementation, and just embed it 
into the new version:

```
type ClientV2 struct {
	ClientV1
}

func (c *ClientV2) ChangedMeaning(args params.Args) (r, error){
}

func (c *ClientV2) ChangeSig(args params.ArgsV2) (r2, error) {
}

func (c *ClientV2) NewMethod(args params.NewArgs) (rNew,...) {
}
```

The embedding rules for Go mean that if you re-use the name of a method, the embedded method is 
automatically hidden. However for (4) it is hard to hide a method that is exposed on an embedded 
struct. So here create a base class that has all the common functionality, and embed that into 
both versions of the facade. And then on the original version just copy the original implementation, 
and do not copy the implementation to the new version.

```
type ClientBase struct {
}

// RENAME all methods that were func (*Client) Foo to func (*ClientBase) Foo

type ClientV0 struct {
	ClientBase
}

func (*ClientV0) OnlyInV0(args params.Args) (r, error) {
}


type ClientV1 struct {
	ClientBase
}
```

## Patterns

### Bulk Requests

Many of the API requests are intended to change a property of an entity inside state. But
while this may be correct in the one or other case many operation especially in larger
environments need to change many entities of one type during one operation. In case of an
API method implementation accepting only the needed parameters for one change this would
lead to an unacceptable I/O overhead as well as the loosing of the transactional context.

To avoid these problems most API methods should be implemented to accept *bulk requests*.
Instead of parameters for only one change the API method takes a list of parameter sets.
Additionally if the request returns a response this has to be capable to transport all
individual responses in a way that the caller is able to identify to which parameter set
they belong.

As an example take the *Machiner* and here

```
func (api *MachinerAPI) SetMachineAddresses(args params.SetMachinesAddresses) (params.ErrorResults, error)
```

Here `SetMachinesAdresses`is a struct containing a slice of `MachineAddresses`. The
method iterates over this slice and sets the addresses for the individual machines.
The result of this operation is of type `error`and may be nil. All errors are
collected in the order of the passed machine addresses and returned as `ErrorResults`.
So the client is able to check if and where errors occured. In case of only one machine
address to change it's no problem to put one this one machine address into the parameter
set.
