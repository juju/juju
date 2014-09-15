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
[apiserver](https://github.com/juju/juju/tree/master/apiserver). Here you'll
e.g. find the package [agent](https://github.com/juju/juju/tree/master/apiserver/agent)
implementing the API interfaces used by the machine agent. As a first step it registers
the factory for version 0 of the facade with

```
func init() {
	common.RegisterStandardFacade("Agent", 0, NewAgentAPIV0)
}
```

This initial step has to be done for each factory/facade pair in each package. In case 
of multiple versions all have to be registered here. So when the next one is added the 
init function additionally contains the line

```
common.RegisterStandardFacade("Agent", 1, NewAgentAPIV1)
```

All factories implement the signature

```
func(st *state.State, resources *common.Resources, auth common.Authorizer) (facade interface{}, err error)
```

to initialize and return their individual facade. So e.g. in case of the type *Agent* 
it is an instance of `agent.AgentAPIV0` etc, in case of the *Machiner* an instance of 
`machine.MachinerAPI` etc. Those types then implement the methods following according to 
the allowed signatures like described in the specification so that the RPC package can 
distribute the requests to them.

## Versioning

### Server

#### Implementation

When implementing a new version of a facade, there are a couple of use cases. Depending on
the changes most of them start by embedding the latest version of the facade into the new one.

```
type FantasticAPIV1 struct {
        FantasticAPIV0
}
```

When needed additional field can be added here. Note the convention of naming the facades
by their deployment name, here `Fantastic`, the term `API` to signal where talking about the
API and not an often same named worker counterpart, and `V0`, `V1`, etc to separate the
different versions.

The easiest use case is the adding of a new method without influencing existing ones. In
this case it simply can be added to the new created type:

```
func (api *FantasticAPIV1) MyNewMethod(args params.FooArgs) (params.FooResults, error) {
        ...
}
```

The second one is the change of the signature of a method by changing the arguments and/or
the result. The change looks like this:

```
func (api *FantasticAPIV1) MyOldMethod(args params.BarArgsV1) (params.BarResultsV1, error) {
        ...
}
```

This change also effects the client side. In case of a newer version here the client has to
be able to handle a lower version on the server. This is the reason to also use versioning
at the arguments and results. A bit different is the change of the meaning of an existing
method. The reimplementation on the new facade type is pretty simple:

```
func (api *FantasticAPIV1) MyChangedMethod(args params.BazArgs) (params.BazArgs, error) {
        ...
}
```

Here the more important change is on the client side to explicitely check the best available
API version and to act accordingly. Also the design of those changes has to take care that
the client cannot expect the newest version on the server.

Last but not least API methods may be removed. This cannot be done directly by not implementing
when choosing the embedding pattern shown above. Here the best way is to move all common logic
into a base type, only the method to remove is missing:

```
type FantasticAPIBase struct {
        ...
}

func (api *FantasticAPIBase) MyBaseMethod(args params.YaddaArgs) (params.YaddaResults, error) {
        ...
}
```

Now this base API has to be embedded into the individual version. The method to be removed will
here only be implemented in those versions where it exists:

```
// FantasticAPIV0 provides MyRemovedMethod.
type FantasticAPIV0 struct {
        FantasticAPIBase
}

func (api *FantasticAPIV0) MyRemovedMethod(args params.OldArgs) (params.OldResults, error) {
        ...
}

// FantasticAPIV1 does not provide MyRemovedMethod anymore.
type FantasticAPIV1 struct {
        FantasticAPIBase
}
```

***Alternative approach to discuss***

> The method described above will get more difficult in newer versions when more methods
> have to be removed. Here it may be better to have a hidden private implementation of
> API versions. The public versions contain these private ones as a field, their public
> methods in the best case simply pass a call to the counterpart of the private type. A
> removed method is in this case simply not implemented on the public type anymore. Also
> signature changes may be easier. While the real change--without any semantic change, that's
> a different use case--is done in private type the public wrappers do everything to make
> it compatible for their versions.

#### Testing

Testing the API needs a different strategy than most other unit tests. We have to assure
that the versions we provide are robust over the time. So each time we run the tests of a
facade we have to do it for all existing versions. But the API changes over time, so the 
tests have to be versioned too. Last but not least the tests have to be maintainable while 
they are growing and changing.

The approach to do so are *per-method suites*. Major idea is to group the tests for each
individual method of the API into a suite starting with a version 0 here. As long as the
methods are compatible over the versions the tests for new facades can use the existing
suites. But when they are changing the used suites have to be changed too. This way we
get a matrix.

Take this scenario:

| Method | Change  | Vsn | 0            | 1            | 2          |
|--------|---------|:---:|--------------|--------------|------------|
| Foo    | Stable  | 0   | FooSuiteV0   | FooSuiteV0   | FooSuiteV0 |
| Bar    | Added   | 1   |              | BarSuiteV1   | BarSuiteV1 |
| Baz    | Changed | 2   | BazSuiteV0   | BazSuiteV0   | BazSuiteV2 |
| Yadda  | Removed | 2   | YaddaSuiteV0 | YaddaSuiteV0 |            |

Here the `FooSuiteV0` has to be run for all three facade versions. The `BarSuiteV1` instead
only for the facades `V1`and `V2`. The `BazSuiteV0` is valid for the facades `V0` and `V1`
while a new `BazSuiteV2` has to be chosen for facade `V2`. Last but not least the `YaddaSuiteV0`.
It lives for two versions and is not called anymore for `V2` and higher.

Now the facades have to be injected into the suites. Instead of adding a field for the facade
itself we add a factory field with the signature

```
type Factory func(*state.State, *common.Resources, common.Authorizer) (interface{}, error)
```

Additionally we we implement functions with this type for each facade version, using the 
according real constructor function, like here for version 0:

```
func newFantasticAPIV0(
        st *state.State, resources *Resources, authorizer Authorizer,
) (interface{}, error) {
	return fantastic.NewFantasticAPIV0(st, resources, authorizer)
}
```

When registering a suite for a version this factory can be injected to be used inside the
tests. Here the factory can be used to instantiate the API it needs:

```
// Test method Baz for version 0 and 1.
func (s *BazSuiteV0) TestBazOK(c *gc.C) {
        api, err := s.factory(s.State, s.resources, s.authorizer)
        c.Assert(err, gc.IsNil)
        fantasticAPI, ok := api.(fantasticAPIV0)
        c.Assert(ok, gc.Equal, true)
        ...
}

// Test method Baz for version 2.
func (s *BazSuiteV2) TestBazOK(c *gc.C) {
        api, err := s.factory(s.State, s.resources, s.authorizer)
        c.Assert(err, gc.IsNil)
        fantasticAPI, ok := api.(fantasticAPIV2)
        c.Assert(ok, gc.Equal, true)
        ...
}
```

The suite may contain more tests for the method `Baz()`, e.g. error situations, while
the according other suites cover the other methods. When registering the suites the
factories for the different versions have to be injected:

```
var (
         _ = gc.Suite(&FooSuiteV0{
                baseFantasticAPISuite{
                        factory: newFantasticAPIV0,
                },
         })
         _ = gc.Suite(&FooSuiteV0{
                baseFantasticAPISuite{
                        factory: newFantasticAPIV1,
                },
         })
         _ = gc.Suite(&FooSuiteV0{
                baseFantasticAPISuite{
                        factory: newFantasticAPIV2,
                },
         })

         _ = gc.Suite(&BarSuiteV1{
                baseFantasticAPISuite{
                        factory: newFantasticAPIV1,
                },
         })
         _ = gc.Suite(&BarSuiteV1{
                baseFantasticAPISuite{
                        factory: newFantasticAPIV2,
                },
         })

        ...
)
```

In this example we see how the `FooSuiteV0` now runs the unchanged tests for `Foo()`
for all three versions of the API while the `BarSuiteV1` tests `Bar()` only for 
the versions 1 and 2.

***Alternative approach to discuss***

> Instead of manual registrations the `init()` function can be used together with
> a table defining the combination of suites and factories.

### Client

#### Implementation

TBD.

#### Testing

TBD.

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
func (api *MachinerAPIV0) SetMachineAddresses(args params.SetMachinesAddresses) (params.ErrorResults, error) {
        ...
}
```

Here `SetMachinesAdresses`is a struct containing a slice of `MachineAddresses`. The
method iterates over this slice and sets the addresses for the individual machines.
The result of this operation is of type `error`and may be nil. All errors are
collected in the order of the passed machine addresses and returned as `ErrorResults`.
So the client is able to check if and where errors occured. In case of only one machine
address to change it's no problem to put one this one machine address into the parameter
set.
