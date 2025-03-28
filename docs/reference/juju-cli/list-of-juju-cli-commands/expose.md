(command-juju-expose)=
# `juju expose`

```
Usage: juju expose [options] <application name>

Summary:
Makes an application publicly available over the network.

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
--endpoints (= "")
    Expose only the ports that charms have opened for this comma-delimited list of endpoints
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
--to-cidrs (= "")
    A comma-delimited list of CIDRs that should be able to access the application ports once exposed
--to-spaces (= "")
    A comma-delimited list of spaces that should be able to access the application ports once exposed

Details:
Adjusts the firewall rules and any relevant security mechanisms of the
cloud to allow public access to the application.

If no additional options are specified, the command will, by default, allow
access from 0.0.0.0/0 to all ports opened by the application. For example, to
expose all ports opened by apache2, you can run:

juju expose apache2

The --endpoints option may be used to restrict the effect of this command to
the list of ports opened for a comma-delimited list of endpoints. For instance,
to only expose the ports opened by apache2 for the "www" endpoint, you can run:

juju expose apache2 --endpoints www

To make the selected set of ports accessible by specific CIDRs, the --to-cidrs
option may be used with a comma-delimited list of CIDR values. For example:

juju expose apache2 --to-cidrs 10.0.0.0/24,192.168.1.0/24

To make the selected set of ports accessible by specific spaces, the --to-spaces
option may be used with a comma-delimited list of space names. For example:

juju expose apache2 --to-spaces public

All of the above options can be combined together. In addition, multiple "juju
expose" invocations can be used to specify granular expose rules for different
endpoints. For example, to allow access to all opened apache ports from
0.0.0.0/0 but restrict access to any port opened for the "logs" endpoint to
CIDR 10.0.0.0/24 you can run:

juju expose apache2
juju expose apache2 --endpoints logs --to-cidrs 10.0.0.0/24

Each "juju expose" invocation always overwrites any previous expose rule for
the same endpoint name. For example, running the following commands instruct
juju to only allow access to ports opened for the "logs" endpoint from CIDR
192.168.0.0/24.

juju expose apache2 --endpoints logs --to-cidrs 10.0.0.0/24
juju expose apache2 --endpoints logs --to-cidrs 192.168.0.0/24

See also:
    unexpose
```