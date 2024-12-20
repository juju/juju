(commands)=
Commands and Sub-commands
=========================

The base `Command` interface is found in `cmd/cmd.go`.

Commands need to provide an `Info` method that returns an Info struct.

The info struct contains: name, args, purpose and a detailed description.
This information is used to provide the default help for the command.

In the same package, there is `CommandBase` whose purpose is to be composed
into new commands, and provides a default no-op SetFlags implementation, a
default Init method that checks for no extra args, and a default Help method.


Supercommands
=============

`Supercommand`s are commands that do many things, and have "sub-commands" that
provide this functionality.  Git and Bazaar are common examples of
"supercommands".  Subcommands must also provide the `Command` interface, and
are registered using the `Register` method.  The name and aliases are
registered with the supercommand.  If there is a duplicate name registered,
the whole thing panics.

Supercommands need to be created with the `NewSuperCommand` function in order
to provide a fully constructed object.

The 'help' subcommand
---------------------

All supercommand instances get a help command.  This provides the basic help
functionality to get all the registered commands, with the addition of also
being able to provide non-command help topics which can be added.

Help topics have a `name` which is what is matched from the command line, a
`short` one line description that is shown when `<cmd> help` is called,
and a `long` text that is output when the topic is requested.


Execution
=========

The `Main` method in the cmd package handles the execution of a command.

A new `gnuflag.FlagSet` is created and passed to the command in `SetFlags`.
This is for the command to register the flags that it knows how to handle.

The args are then parsed, and passed through to the `Init` method for the
command to decide what to do with the positional arguments.

The command is then `Run` and passed in an execution `Context` that defines
the standard input and output streams, and has the current working directory.
