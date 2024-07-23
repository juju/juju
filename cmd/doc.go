// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package cmd provides the structs and methods for building Juju commands.
// Commands need to provide an `Info` method that returns an Info struct.
//
// The info struct contains: Name, Args, Purpose and a detailed description
// (Doc).
// This information is used to provide the default help for the command.
//
// In the same package, there is `CommandBase` whose purpose is to be composed
// into new commands, and provides a default no-op SetFlags implementation, a
// default Init method that checks for no extra args, and a default Help method.
//
// `Supercommand`s are commands that do many things, and have "sub-commands" that
// provide this functionality.  Git and Bazaar are common examples of
// "supercommands".  Subcommands must also provide the `Command` interface, and
// are registered using the `Register` method.  The name and aliases are
// registered with the supercommand.  If there is a duplicate name registered,
// the whole thing panics.
// For the time being, the only supercommand that we have is `wait-for`
// (https://juju.is/docs/juju/juju-wait-for) and the supercommand pattern is
// not going to be adopted anymore in the near future. Example:
// juju wait-for unit mysql/0
//
// Supercommands need to be created with the `NewSuperCommand` function in order
// to provide a fully constructed object.
//
// All supercommand instances get a help command. Every supercommand gets a
// help command which provides the basic help functionality to its respective
// subcommands.
//
// Help topics have a `name` which is what is matched from the command line, a
// `short` one line description that is shown when `<cmd> help` is called,
// and a `long` text that is output when the topic is requested.
package cmd
