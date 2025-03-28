(command-juju-config)=
# `juju config`

```
Usage: juju config [options] <application name> [--branch <branch-name>] [--reset <key[,key]>] [<attribute-key>][=<value>] ...]

Summary:
Gets, sets, or resets configuration for a deployed application.

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
--file  (= )
    path to yaml-formatted application config
--format  (= yaml)
    Specify output format (json|yaml)
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-o, --output (= "")
    Specify an output file
--reset  (= )
    Reset the provided comma delimited keys

Details:
If no config key is specified, all configuration items (keys, values, metadata)
for the application will be printed out.

The entire set of available config settings and their current values can be
listed by running "juju config <application name>". For example, to obtain the
config settings for apache2 you can run:

juju config apache2

When listing config settings, this command will, by default, format its output
as a yaml document. To obtain the output formatted as json, the --format json
flag can be specified. For example:

juju config apache2 --format json

The settings list output includes the name of the charm used to deploy the
application and a listing of the application-specific configuration settings.
See `juju status` for the set of deployed applications.

To obtain the configuration value for a specific setting, simply specify its
name as an argument, e.g. "juju config apache2 servername". In this case, the
command will ignore any provided --format option and will instead output the
value as plain text. This allows external scripts to use the output of a "juju
config <application name> <setting name>" invocation as an input to an
expression or a function.

To set the value of one or more settings, provide each one as a key/value pair
argument to the command invocation. For instance:

juju config apache2 servername=example.com lb_balancer_timeout=60

A single setting value may be set via file.  The following example uses
a file "/tmp/servername" with contents "example.com":

juju config apache2 servername=@/tmp/servername

By default, any configuration changes will be applied to the currently active
branch. A specific branch can be targeted using the --branch option. Changes
can be immediately be applied to the model by specifying --branch=master. For
example:

juju config apache2 --branch=master servername=example.com
juju config apache2 --branch test-branch servername=staging.example.com

Rather than specifying each setting name/value inline, the --file flag option
may be used to provide a list of settings to be updated as a yaml file. The
yaml file contents must include a single top-level key with the application's
name followed by a dictionary of key/value pairs that correspond to the names
and values of the settings to be set. For instance, to configure apache2,
the following yaml file can be used:

apache2:
  servername: "example.com"
  lb_balancer_timeout: 60

If the above yaml document is stored in a file called config.yaml, the
following command can be used to apply the config changes:

juju config apache2 --file config.yaml

Finally, the --reset flag can be used to revert one or more configuration
settings back to their default value as defined in the charm metadata:

juju config apache2 --reset servername
juju config apache2 --reset servername,lb_balancer_timeout

See also:
    deploy
    status
```