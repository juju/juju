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

This documents covers how those factories and methods have to be implemented and
maintained. The goal here is a clean organization following common paterns while
keeping the compatability to older versions.

### Overview

This document provides guide on

- how the code has to be organized into packages and types,
- how the versioning has to be implemented, and
- which patterns to follow when implementing API types and methods.

## Organization

## Versioning

## Patterns
