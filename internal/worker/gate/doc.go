// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package gate provides a mechanism by which independent workers can wait for
// one another to finish a task, without introducing explicit dependencies
// between those workers.
package gate
