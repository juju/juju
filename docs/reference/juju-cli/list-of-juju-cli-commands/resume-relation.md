(command-juju-resume-relation)=
# `juju resume-relation`

```
Usage: juju resume-relation [options] <relation-id>[,<relation-id>]

Summary:
Resumes a suspended relation to an application offer.

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
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>

Details:
A relation between an application in another model and an offer in this model will be resumed.
The relation-joined and relation-changed hooks will be run for the relation, and the relation
status will be set to joined. The relation is specified using its id.

Examples:
    juju resume-relation 123
    juju resume-relation 123 456

See also:
    add-relation
    offers
    remove-relation
    suspend-relation
```