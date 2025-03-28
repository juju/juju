(command-juju-remove-relation)=
# `juju remove-relation`

```
Usage: juju remove-relation [options] <application1>[:<relation name1>] <application2>[:<relation name2>] | <relation-id>

Summary:
Removes an existing relation between two applications.

Global Options:
--debug  (= false)
    equivalent to --show-log --logging-config=<root>=DEBUG
-h, --help  (= false)
    Show help on a command or other topic.
--logging-config (= "")
    specify log levels for modules
--quiet  (= false)
    show no informational output
--show-log  (= false)
    if set, write the log file to stderr
--verbose  (= false)
    show more verbose output

Command Options:
-B, --no-browser-login  (= false)
    Do not use web browser for authentication
--force  (= false)
    Force remove a relation
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>

Details:
An existing relation between the two specified applications will be removed.
This should not result in either of the applications entering an error state,
but may result in either or both of the applications being unable to continue
normal operation. In the case that there is more than one relation between
two applications it is necessary to specify which is to be removed (see
examples). Relations will automatically be removed when using the`juju
remove-application` command.

The relation is specified using the relation endpoint names, eg
  mysql wordpress, or
  mediawiki:db mariadb:db

It is also possible to specify the relation ID, if known. This is useful to
terminate a relation originating from a different model, where only the ID is known.

Sometimes, the removal of the relation may fail as Juju encounters errors
and failures that need to be dealt with before a relation can be removed.
However, at times, there is a need to remove a relation ignoring
all operational errors. In these rare cases, use --force option but note
that --force will remove a relation without giving it the opportunity to be removed cleanly.

Examples:
    juju remove-relation mysql wordpress
    juju remove-relation 4
    juju remove-relation 4 --force

In the case of multiple relations, the relation name should be specified
at least once - the following examples will all have the same effect:

    juju remove-relation mediawiki:db mariadb:db
    juju remove-relation mediawiki mariadb:db
    juju remove-relation mediawiki:db mariadb

See also:
    add-relation
    remove-application
```