// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !go1.6

package main

// This line intentionally does not compile.  This file will only be compiled if
// you are compiling with a version of Go that is lower than the one we support.
var requiredGoVersion = This_project_requires_Go_1_6
