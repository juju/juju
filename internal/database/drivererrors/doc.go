// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// DriverErrors package is the internal errors package, which provides extensive
// error handling for the dqlite database. It is not expected that the domain
// package will need to interact with this package directly, instead it should
// use the domain and database package to handle errors. Those packages are
// expected to build on top of this package to provide a more user friendly
// error handling experience.

package drivererrors
