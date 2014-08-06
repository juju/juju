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

## Patterns
