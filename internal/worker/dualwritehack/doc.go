// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This worker is where you put some ugly hacks to do eventually consistent
// dual writing to mongo from dqlite. Don't write tests here, this is a best
// effort to keep systems working while other systems are being rewritten as
// services.
package dualwritehack
