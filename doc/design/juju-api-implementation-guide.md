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
core packages of the API are implemented. They provide a communication between client
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

The access to the business logic provided by the API is done through *facades*. Those
are grouping *connected functionalities*, so that everything a worker or a client 
needs can be provided optimally by only one factory each. The facade is specified
in the `Type` field of each request. Each facade is represented by a package which 
implements and registers a *factory* for each version it supports. When a request
for a given type is received the factory for the requested version is used to create
a facade instance. This type implements a set of *methods* for the handling of the
requests as described in the [specification document](juju-api-design-specification.md).

All API facade packages are located inside the package 
[apiserver](https://github.com/juju/juju/tree/master/apiserver). Here you'll
e.g. find the package [agent](https://github.com/juju/juju/tree/master/apiserver/agent)
implementing the API interfaces used by the *machine agent*. As a first step it registers
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

***Remark***

> Most of the current facade packages so far only implement the initial version and so
> don't contain the postfix `V0`. They will be refactored step-by-step when adding a
> `V1`.

### Scenario

The folling description uses a fictional API for monitoring purposes. So a monitoring worker
on a machine can store vital data in state while other functions exist to retrieve them. The
initial version 0 provides the function `WriteCPU()` and `WriteDisk()`, in version 1 the 
function `WriteRAM()` is added while the arguments of `WriteCPU()` are changed. In version 2
`WriteCPU()`is then dropped in favor of `WriteLoad()`.

### Server

#### Implementation

The implementation of version 0 of the monitoring API is done in a file named `monitoring_v0.go`.
Here the type `MonitoringAPIV0` is defined and its factory function registered like decribed
above. Also the initial functions are implemented here:

```
func (api *MonitoringAPIV0) WriteCPU(args params.CPUMonitors) (params.Errors, error) {
        ...
}

func (api *MonitoringAPIV0) WriteDisk(args params.DiskMonitors) (params.Errors, error) {
        ...
}
```

When implementing version 1 we want to reuse the already written code. So in a new file
named `monitoring_v1.go` in the same package the new type is defined by embedding the version
0:

```
type MonitoringAPIV1 struct {
        MonitoringAPIV0
}
```

This way the new version already provides the functions of version 0 and can be registered
like its predecessor. Now the new function can be added:

```
func (api *MonitoringAPIV1) WriteRAM(args params.RAMMonitors) (params.Errors, error) {
        ...
}
```

But beside adding a new functionality we also have to change our CPU monitoring functions
as descibed. It now expects different arguments, so those have to be versioned too. The
way Go embedds functions and the RPC mechanism resolves function calls we now can overload
the initial version by defining the function new on the version 1:

```
func (api *MonitoringAPIV1) WriteCPU(args params.CPUMonitorsV1) (params.Errors, error) {
        ...
}
```

As said above the version 2 renames a function. Technically this means dropping an existing
one and adding a new one. The latter is no problem, it's like the adding of a new function
shown above, even if the functionality stays the same. Dropping a function is the more
complicated part and larger changes have to be done. First a private base type containing
the fields of version 0 has to be implemented in the file `monitoring.go`:

```
type monitoringAPIBase struct {
        ...
}
```

Now in `monitoring_v0.go` the code of the API functions has to be moved into versioned private
functions of the newly created base. This base now has to be embedded and the versioned moved
code be called:

```
func (api *monitoringAPIBase) writeCPUV0(args params.CPUMonitors) (params.Errors, error) {
        ...
}

type MonitoringAPIV0 struct {
        monitoringAPIBase
}

func (api *MonitoringAPIV0) WriteCPU(args params.CPUMonitors) (params.Errors, error) {
        return api.writeCPUV0(args)
}
```

Now also in the version 1 file move the code into according version functions of the base,
embed it, and call it from inside the public functions. Also remove the embedding of the
version 0. Instead the functions have to be implemented on the type itself and call the
embedded code like in version 0. Thankfully this is a quick task.

The coding of version 2 is now done the same way. First the new load monitoring function
is implemented as private base function. Then the according public function added on the
version 2 type. Again all exported functions are added like in version 1, only the removed
function will be left out:

```
func (api *monitoringAPIBase) writeLoadV2(args params.LoadMonitors) (params.Errors, error) {
        ...
}

type MonitoringAPIV2 struct {
        monitoringAPIBase
}

func (api *MonitoringAPIV2) WriteLoad(args params.LoadMonitors) (params.Errors, error) {
        return api.writeLoadV2(args)
}
```

This way new or changed logic is implemented in their versioned files and can easily be
reused, changed, or dropped.

#### Testing

The testing of versioned APIs differs from most other unit tests. Each provided function
of each version of a facade has to be tested, but while some tests don't differ between 
versions because the function didn't changed others have to be reimplemented. So the idea
of organizing the tests and reuse the code is very similar to the solution for the
implementation described above.

Once again the tests are split into one base file and one file per version. The base file
`monitoring_test.go`contains a test suite with no pubic test functions. It's the container 
for test variables and private prepared test functions. As long as there are no changes 
needed it also may contain the definitions of `SetUpSuite()`/`TearDownSuite()` and 
`SetUpTest()`/`TearDownTest()`. Otherwise those have to be implemented as private versioned
methods like the test methods themselve.

```
type baseSuite struct {
        ...
}

func (s *baseSuite) SetUpTest(c *gc.C) {
        ...
}
```

Now in `monitoring_v0_test.go` the tests for the version 0 of the provided API functions
have to be implemented. Additionally types to wrap the factory functions as well as 
interfaces containing only one of the API functions to test have to be declared. Both are
used to inject the real versions later.

```
func factoryV0 func(st *state.State, resources *common.Resources, auth common.Authorizer) (interface{}, error)

func (s *baseSuite) testNewMonitorSucceedsV0(c *gc.C, factory factoryV0) {
        ...
        api, err := factory(s.State, s.resources, s.authorizer)
        c.Assert(err, gc.IsNil)
        ...
}

func (s *baseSuite) testNewMonitorFailsV0(c *gc.C, factory factoryV0) {
        ...
}

type writeCPUV0 interface {
        WriteCPU(args params.CPUMonitors) (params.Errors, error)
}

func (s *baseSuite) testWriteCPUSucceedsV0(c *gc.C, api writeCPUV0) {
        ...
        results, err := api.WriteCPU(args)
        c.Assert(err, gc.IsNil)
        ...
}
```

As long as the functionality of the versions doesn't change those tests can
be reused in future test. First the have to be integrated into the test suite
for version 0 in the same file. First we need a factory for this version:

```
func factoryWrapperV0(st *state.State, resources *common.Resources, auth common.Authorizer) (interface{}, error) {
	return monitoring.NewMonitorAPIV0(st, resources, auth)
}
```

Now the versioned suite itself can be implemented:

```
type monitoringSuiteV0 struct {
        baseSuite
}

var _ = gc.Suite(&monitoringSuiteV0{})

func (s *monitoringSuiteV0) TestNewMonitorSucceeds(c *gc.C) {
        s.testNewMonitorSucceedsV0(c, factoryWrapperV0)
}

func (s *monitoringSuiteV0) TestWriteCPUSucceeds(c *gc.C) {
        s.testWriteCPUSucceedsV0(c, s.newAPI(c))
}
```

Here `newAPI()` is a little but useful helper:

```
func (s *monitoringSuiteV0) newAPI(c *gc.C) *monitoring.MonitoringAPIV0 {
	api, err := monitoring.NewMonitorAPIV0(s.State, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
	return api
}
```

When now implementing the version 1 of the monitoring suite the factory wrapper
has to be reimplemented to return an instance of version 1 as well as the
private tests for the changed or added functions have to be written. The interface 
for the `WriteCPU()` function needs a new version too because its signature changed.

```
func factoryWrapperV1(st *state.State, resources *common.Resources, auth common.Authorizer) (interface{}, error) {
	return monitoring.NewMonitorAPIV1(st, resources, auth)
}

func (s *baseSuite) testWriteCPUSucceedsV1(c *gc.C, api writeCPUV1) {
        ...
}

func (s *baseSuite) testWriteRAMFailsV1(c *gc.C, api writeRAMV1) {
        ...
}
```

Now like already in version 0 the suite for version 1 can be implemented. It mostly
looks like its predecessor and the tests for `WriteDisk()` can reuse the version 0
tests of the base suite. The tests for `WriteRAM()` have to use the new base tests
instead while the tests for `WriteRAM()` are added. Also the little `newAPI()` helper
now has to return an instance of the version 1 API.

```
func (s *monitoringSuiteV1) TestWriteCPUSucceeds(c *gc.C) {
	s.testWriteCPUSucceedsV1(c, s.newAPI(c))
}

func (s *monitoringSuiteV1) TestWriteDiskSucceeds(c *gc.C) {
	s.testWriteDiskSucceedsV0(c, s.newAPI(c))
}

func (s *monitoringSuiteV1) TestWriteRAMFails(c *gc.C) {
	s.testWriteRAMFailsV1(c, s.newAPI(c))
}
```

Finally for version 2 the steps again are similar:

- Create the file `monitoring_v2_test.go`
- Add a factory wrapper returning the version 2 of the API
- Add an interface for the new `WriteLoad()` function
- Implement the private base tests for this new function
- Add the `monitoringSuiteV2`
- Implement `newAPI()` returning a version 2 API instance
- Add all known test methods but those for `WriteCPU()` tests
- Add tests for the new `WriteLoad()` using their according base tests

This way test suites can easily grow version by version. The code is distributed
to the versioned tests in a natural way, adding, changing, and removing is no
problem. 

**Take care:** Don't forget tests for existing functions when implementing the
suite for a new version!

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
