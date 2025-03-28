(command-juju-add-ssh-key)=
# `juju add-ssh-key`

```
Usage: juju add-ssh-key [options] <ssh key> ...

Summary:
Adds a public SSH key to a model.

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
Juju maintains a per-model cache of public SSH keys which it copies to
each unit (including units already deployed). By default this includes the
key of the user who created the model (assuming it is stored in the
default location ~/.ssh/). Additional keys may be added with this command,
quoting the entire public key as an argument.

Examples:
    juju add-ssh-key "ssh-rsa qYfS5LieM79HIOr535ret6xy
    AAAAB3NzaC1yc2EAAAADAQA6fgBAAABAQCygc6Rc9XgHdhQqTJ
    Wsoj+I3xGrOtk21xYtKijnhkGqItAHmrE5+VH6PY1rVIUXhpTg
    pSkJsHLmhE29OhIpt6yr8vQSOChqYfS5LieM79HIOJEgJEzIqC
    52rCYXLvr/BVkd6yr4IoM1vpb/n6u9o8v1a0VUGfc/J6tQAcPR
    ExzjZUVsfjj8HdLtcFq4JLYC41miiJtHw4b3qYu7qm3vh4eCiK
    1LqLncXnBCJfjj0pADXaL5OQ9dmD3aCbi8KFyOEs3UumPosgmh
    VCAfjjHObWHwNQ/ZU2KrX1/lv/+lBChx2tJliqQpyYMiA3nrtS
    jfqQgZfjVF5vz8LESQbGc6+vLcXZ9KQpuYDt joe@ubuntu"

For ease of use it is possible to use shell substitution to pass the key
to the command:

juju add-ssh-key "$(cat ~/mykey.pub)"

See also:
    ssh-keys
    remove-ssh-key
    import-ssh-key

```