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
and server using WebSockets and a JSON marshalling. To expose methods as RPC, the API
server uses a registry of Facades, their public name, and their version to determine
how requests are dispatched to the associated method.

This document covers how those *factories*, *facades*, and *methods* have to be
implemented and maintained. The goal here is a clean organization following common
paterns while preserving compatability to older versions.

### Overview

This document provides guidance on

- how the code should be organized into packages and types,
- how the versioning should be implemented, and
- recommended patterns for implementing new API types and methods.

## Organization

Access to the business logic provided by the API is done through *facades*. Those
are grouping *connected functionality*, so that everything a worker or a client
needs can be provided by a single facade. The facade is specified in the
`Type` field of each request. Each facade is represented by a package which
implements and registers a *factory* for each version it supports. When a request
for a given type is received the factory for the requested version is used to create
a facade instance. This type implements a set of *methods* for handling of the
requests as described in the [specification document](juju-api-design-specification.md).

All API facade packages are located inside the package
[apiserver](https://github.com/juju/juju/tree/master/apiserver). For example, here you'll
find the package [agent](https://github.com/juju/juju/tree/master/apiserver/agent), which
implements the API interface used by Machine and Unit agents.

You can see that in the `init()` function it registers the factory for version 0 of the
facade with:

```
func init() {
	common.RegisterStandardFacade("Agent", 0, NewAgentAPIV0)
}
```

This initial step has to be done for each factory/facade pair in each package. In case
When there are multiple versions, all are registered here. So when the next version is
added, the `init()` function will include the line:

```
common.RegisterStandardFacade("Agent", 1, NewAgentAPIV1)
```

The signature for factories inside the `apiserver` package is:

```
func(st *state.State, resources *common.Resources, auth common.Authorizer) (facade interface{}, err error)
```

Real implementations here don't return their API facades as empty interface but as
a typed instance:

```
func NewAgentAPIV0(st *state.State, resources *common.Resources, auth common.Authorizer) (*AgentAPIV0, error)
```

The usage of `common.RegisterStandardFacade()` takes care of wrapping this function before
registering the facade. The concrete facade types implement their methods according to the
allowed signatures described in the specification so that the RPC package can distribute
the requests to them.

## Versioning

***Remark***

> Most of the current facade packages so far only implement the initial version and so
> don't contain the postfix `V0`. These versions represent the API that has been released
> with Juju 1.18. They will be refactored step-by-step when adding a `V1`.

### Scenario

The folling description uses a fictional API for monitoring purposes. So a monitoring worker
on a machine can store vital data in state while other functions exist to retrieve them. We
start with version 1 here because the API didn't exist in Juju 1.18.

The initial version provides the function `WriteCPU()` and `WriteDisk()`, in version 2 the
function `WriteRAM()` is added while the arguments of `WriteCPU()` are changed. In version 3
`WriteCPU()`is then dropped in favor of `WriteLoad()`.

### Server

#### Implementation

The implementation of version 1 of the monitoring API is done in a file named `monitoring_v1.go`.
Here the type `MonitoringAPIV1` is defined and its factory function registered like decribed
above. Also the initial functions are implemented here:

```
func (api *MonitoringAPIV1) WriteCPU(args params.CPUMeasurings) (params.ErrorResults, error) {
        ...
}

func (api *MonitoringAPIV1) WriteDisk(args params.DiskMeasurings) (params.ErrorResults, error) {
        ...
}
```

When implementing version 1 we want to reuse the already written code. So in a new file
named `monitoring_v2.go` in the same package the new type is defined by embedding the version
1:

```
type MonitoringAPIV2 struct {
        MonitoringAPIV1
}
```

This way the new version already provides the functions of version 1 and can be registered
like its predecessor. Now the new function can be added:

```
func (api *MonitoringAPIV2) WriteRAM(args params.RAMMeasurings) (params.ErrorResults, error) {
        ...
}
```

But beside adding a new functionality we also have to change our CPU monitoring functions
as descibed. It now expects different arguments, so those have to be versioned too. The
way Go embedds functions and the RPC mechanism resolves function calls we now can overload
the initial version by defining the function new on the version 2:

```
func (api *MonitoringAPIV2) WriteCPU(args params.CPUMeasuringsV2) (params.ErrorResults, error) {
        ...
}
```

As said above the version 3 renames a function. Technically this means dropping an existing
one and adding a new one. The latter is no problem, it's like the adding of a new function
shown above, even if the functionality stays the same. Dropping a function is the more
complicated part and larger changes have to be done. First a private base type containing
the fields of version 1 has to be implemented in the file `monitoring.go`:

```
type monitoringAPIBase struct {
        ...
}
```

Now in `monitoring_v1.go` the code of the API functions has to be moved into versioned private
functions of the newly created base. This base now has to be embedded and the versioned moved
code be called:

```
func (api *monitoringAPIBase) writeCPUV1(args params.CPUMeasurings) (params.ErrorResults, error) {
        ...
}

type MonitoringAPIV1 struct {
        monitoringAPIBase
}

func (api *MonitoringAPIV1) WriteCPU(args params.CPUMeasurings) (params.ErrorResults, error) {
        return api.writeCPUV1(args)
}
```

Now also in the version 2 file move the code into according version functions of the base,
embed it, and call it from inside the public functions. Also remove the embedding of the
version 1. Instead the functions have to be implemented on the type itself and call the
embedded code like in version 1. Thankfully this is a quick task.

The coding of version 3 is now done the same way. First the new load monitoring function
is implemented as private base function. Then the according public function added on the
version 3 type. Again all exported functions are added like in version 2, only the removed
function will be left out:

```
func (api *monitoringAPIBase) writeLoadV3(args params.LoadMeasurings) (params.ErrorResults, error) {
        ...
}

type MonitoringAPIV3 struct {
        monitoringAPIBase
}

func (api *MonitoringAPIV3) WriteLoad(args params.LoadMeasurings) (params.ErrorResults, error) {
        return api.writeLoadV3(args)
}
```

This way new or changed logic is implemented in their versioned files and can easily be
reused, changed, or dropped.

#### Testing

The testing of versioned APIs differs from most other unit tests. Each provided function
of each version of a facade has to be tested, but while some tests don't differ between
versions because the function didn't change others have to be reimplemented. So the idea
of organizing the tests and reuse the code is very similar to the solution for the
implementation described above.

Once again the tests are split into one base file and one file per version. The base file
`monitoring_test.go`contains a test suite with no pubic test functions. It's the container
for test variables and private prepared test functions. As long as there are no changes
needed it also may contain the definitions of `SetUpSuite()`/`TearDownSuite()` and
`SetUpTest()`/`TearDownTest()`. Otherwise those have to be implemented as private versioned
methods like the test methods themselves.

```
type baseSuite struct {
        ...
}

func (s *baseSuite) SetUpTest(c *gc.C) {
        ...
}
```

Now in `monitoring_v1_test.go` the tests for version 1 of the provided API functions
have to be implemented. Additionally, types to wrap the factory functions as well as
interfaces containing only one of the API functions to test have to be declared. Both are
used to inject the real versions later.

```
func factoryV1 func(st *state.State, resources *common.Resources, auth common.Authorizer) (interface{}, error)

func (s *baseSuite) testNewMonitorSucceedsV1(c *gc.C, factory factoryV1) {
        ...
        api, err := factory(s.State, s.resources, s.authorizer)
        c.Assert(err, jc.ErrorIsNil)
        ...
}

func (s *baseSuite) testNewMonitorFailsV1(c *gc.C, factory factoryV1) {
        ...
}

type writeCPUV1 interface {
        WriteCPU(args params.CPUMeasurings) (params.ErrorResults, error)
}

func (s *baseSuite) testWriteCPUSucceedsV1(c *gc.C, api writeCPUV1) {
        ...
        results, err := api.WriteCPU(args)
        c.Assert(err, jc.ErrorIsNil)
        ...
}
```

As long as the functionality of the versions doesn't change those tests can
be reused in future test. First the have to be integrated into the test suite
for version 1 in the same file. First we need a factory for this version:

```
func factoryWrapperV1(st *state.State, resources *common.Resources, auth common.Authorizer) (interface{}, error) {
	return monitoring.NewMonitorAPIV0(st, resources, auth)
}
```

Now the versioned suite itself can be implemented:

```
type monitoringSuiteV1 struct {
        baseSuite
}

var _ = gc.Suite(&monitoringSuiteV1{})

func (s *monitoringSuiteV1) TestNewMonitorSucceeds(c *gc.C) {
        s.testNewMonitorSucceedsV0(c, factoryWrapperV1)
}

func (s *monitoringSuiteV1) TestWriteCPUSucceeds(c *gc.C) {
        s.testWriteCPUSucceedsV1(c, s.newAPI(c))
}
```

Here `newAPI()` is a little but useful helper:

```
func (s *monitoringSuiteV1) newAPI(c *gc.C) *monitoring.MonitoringAPIV1 {
	api, err := monitoring.NewMonitorAPIV1(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	return api
}
```

When now implementing the version 2 of the monitoring suite the factory wrapper
has to be reimplemented to return an instance of version 2 as well as the
private tests for the changed or added functions have to be written. The interface
for the `WriteCPU()` function needs a new version too because its signature changed.

```
func factoryWrapperV2(st *state.State, resources *common.Resources, auth common.Authorizer) (interface{}, error) {
	return monitoring.NewMonitorAPIV2(st, resources, auth)
}

func (s *baseSuite) testWriteCPUSucceedsV2(c *gc.C, api writeCPUV2) {
        ...
}

func (s *baseSuite) testWriteRAMFailsV2(c *gc.C, api writeRAMV2) {
        ...
}
```

Now like already in version 1 the suite for version 1 can be implemented. It mostly
looks like its predecessor and the tests for `WriteDisk()` can reuse the version 1
tests of the base suite. The tests for `WriteRAM()` have to use the new base tests
instead while the tests for `WriteRAM()` are added. Also the little `newAPI()` helper
now has to return an instance of the version 2 API.

```
func (s *monitoringSuiteV2) TestWriteCPUSucceeds(c *gc.C) {
	s.testWriteCPUSucceedsV2(c, s.newAPI(c))
}

func (s *monitoringSuiteV2) TestWriteDiskSucceeds(c *gc.C) {
	s.testWriteDiskSucceedsV1(c, s.newAPI(c))
}

func (s *monitoringSuiteV2) TestWriteRAMFails(c *gc.C) {
	s.testWriteRAMFailsV2(c, s.newAPI(c))
}
```

Finally for version 3 the steps again are similar:

- Create the file `monitoring_v3_test.go`
- Add a factory wrapper returning the version 3 of the API
- Add an interface for the new `WriteLoad()` function
- Implement the private base tests for this new function
- Add the `monitoringSuiteV3`
- Implement `newAPI()` returning a version 3 API instance
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

When developing an API function always have in mind that it may not only be interesting
to use it for a single operation. Sometimes it's useful to perform the same operation
for a number of entities, e.g. instead of retrieving the information of one machine it
could make sense to read them at once. Here you surely could perform the function for
each instance individually, but this also creates a large overhead.

Another aspect of a too narrow design of an API function is when it could possibly used
in other facades too. It only depends on a small number of parameters controlling its
behavior, e.g. like the authorization in case of the `LifeGetter` in `apiserver/common`.

So even if there's only one use case regarding only a single operation for an API function
is known during implementation *always* design it to operate as *bulk request* for a larger
number of operations. Additionally check if the same logic could be reused in different
facades by exchanging only few parameters, like e.g. the authentication.

As an example take the monitoring API of above. Surely all functions could be implemented
for only one machine:

```
// Package "params".
type CPUMeasuring struct {
	Id     string `json:"id"`
	Time   int    `json:"time"`
        User   int    `json:"user"`
        System int    `json:"system"`
        Nice   int    `json:"nice"`
        Idle   int    `json:"idle"`
}

// Package "monitoring".
func (api *MonitoringAPIV0) WriteCPU(arg params.CPUMeasuring) (params.ErrorResult, error) {
        ...
}
```

So in case of a temporary not available API server all enqueued measurings would have to
be written value by value, call by call (*OK, maybe not the best example, but think of
the enabling of the monitoring for 100 machines at once.*). To change that simply create
a wrapper type containing a slice of the interesting types:

```
type CPUMeasurings struct {
	Measurings []CPUMeasuring `json:"measurings"`
}
```

Now use this as argument as well as the according bulk return type:

```
func (api *MonitoringAPIV0) WriteCPU(args params.CPUMeasurings) (params.ErrorResults, error) {
        ...
}
```

In case of writing only one value it is still no problem. But this way we win the possibility
to also do bulk requests.

When defining the arguments and the result also *always* take care for an explicit serialication
like shown above. The naming scheme for arguments and the according results is:

```
type Thing struct { ... }

type Things struct {
        Things []Thing `json:"things"`
}

type ThingResult struct { ... }

type ThingResults struct {
        Results []ThingResult `json:"results"`
}
```

As we're talking about items of our cloud world the arguments as well as the result *must* have
nouns as identifiers, no combination of verb and noun according to the function. So in case of
the scenerio example above a type named `WriteDisk` would be a bad name for reusage. The name
`DiskMeasuring` instead also makes sense for a reusage when retrieving those values from state.
