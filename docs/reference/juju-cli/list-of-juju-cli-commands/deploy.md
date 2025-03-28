(command-juju-deploy)=
# `juju deploy`

```
Usage: juju deploy [options] <charm or bundle> [<application name>]

Summary:
Deploys a new application or bundle.

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
--attach-storage  (= )
    Existing storage to attach to the deployed unit (not available on k8s models)
--bind (= "")
    Configure application endpoint bindings to spaces
--channel (= "")
    Channel to use when deploying a charm or bundle from the charm store, or charm hub
--config  (= )
    Either a path to yaml-formatted application config file or a key=value pair
--constraints (= "")
    Set application constraints
--device  (= )
    Charm device constraints
--dry-run  (= false)
    Just show what the deploy would do
--force  (= false)
    Allow a charm/bundle to be deployed which bypasses checks such as supported series or LXD profile allow list
--increase-budget  (= 0)
    increase model budget allocation by this amount
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
--map-machines (= "")
    Specify the existing machines to use for bundle deployments
-n, --num-units  (= 1)
    Number of application units to deploy for principal charms
--overlay  (= )
    Bundles to overlay on the primary bundle, applied in order
--plan (= "")
    plan to deploy charm under
--resource  (= )
    Resource to be uploaded to the controller
--revision  (= -1)
    The revision to deploy
--series (= "")
    The series on which to deploy
--storage  (= )
    Charm storage constraints
--to (= "")
    The machine and/or container to deploy the unit in (bypasses constraints)
--trust  (= false)
    Allows charm to run hooks that require access credentials

Details:
A charm or bundle can be referred to by its simple name and a series, revision,
or channel can optionally be specified:

  juju deploy postgresql
  juju deploy ch:postgresql --series bionic
  juju deploy ch:postgresql --channel edge
  juju deploy ch:ubuntu --revision 17 --channel edge

All the above deployments use remote charms found in Charm Hub, denoted by the
'ch:' prefix.  Remote charms with no prefix will be deployed from Charm Hub.

If a channel is specified, it will be used as the source for looking up the
charm or bundle from Charm Hub. When used in a bundle deployment context,
the specified channel is only used for retrieving the bundle and is ignored when
looking up the charms referenced by the bundle. However, each charm within a
bundle is allowed to explicitly specify the channel used to look it up.

If a revision is specified, a channel must also be specified for Charm Hub charms
and bundles.  The charm will be deployed with revision.  The channel will be used
when refreshing the application in the future.

A local charm may be deployed by giving the path to its directory:

  juju deploy /path/to/charm
  juju deploy /path/to/charm --series bionic

You will need to be explicit if there is an ambiguity between a local and a
remote charm:

  juju deploy ./pig
  juju deploy ch:pig

An error is emitted if the determined series is not supported by the charm. Use
the '--force' option to override this check:

  juju deploy charm --series bionic --force

A bundle can be expressed similarly to a charm:

  juju deploy mediawiki-single
  juju deploy mediawiki-single --series focal
  juju deploy ch:mediawiki-single

A local bundle may be deployed by specifying the path to its YAML file:

  juju deploy /path/to/bundle.yaml

The final charm/machine series is determined using an order of precedence (most
preferred to least):

 - the '--series' command option
 - the series stated in the charm URL
 - for a bundle, the series stated in each charm URL (in the bundle file)
 - for a bundle, the series given at the top level (in the bundle file)
 - the 'default-series' model key
 - the top-most series specified in the charm's metadata file
   (this sets the charm's 'preferred series' in the Charm Store)

An 'application name' provides an alternate name for the application. It works
only for charms; it is silently ignored for bundles (although the same can be
done at the bundle file level). Such a name must consist only of lower-case
letters (a-z), numbers (0-9), and single hyphens (-). The name must begin with
a letter and not have a group of all numbers follow a hyphen:

  Valid:   myappname, custom-app, app2-scat-23skidoo
  Invalid: myAppName, custom--app, app2-scat-23, areacode-555-info

Use the '--constraints' option to specify hardware requirements for new machines.
These become the application's default constraints (i.e. they are used if the
application is later scaled out with the `add-unit` command). To overcome this
behaviour use the `set-constraints` command to change the application's default
constraints or add a machine (`add-machine`) with a certain constraint and then
target that machine with `add-unit` by using the '--to' option.

Use the '--device' option to specify GPU device requirements (with Kubernetes).
The below format is used for this option's value, where the 'label' is named in
the charm metadata file:

  <label>=[<count>,]<device-class>|<vendor/type>[,<attributes>]

Use the '--config' option to specify application configuration values. This
option accepts either a path to a YAML-formatted file or a key=value pair. A
file should be of this format:

  <charm name>:
	<option name>: <option value>
	...

For example, to deploy 'mediawiki' with file 'mycfg.yaml' that contains:

  mediawiki:
	name: my media wiki
	admins: me:pwdOne
	debug: true

use

  juju deploy mediawiki --config mycfg.yaml

Key=value pairs can also be passed directly in the command. For example, to
declare the 'name' key:

  juju deploy mediawiki --config name='my media wiki'

To define multiple keys:

  juju deploy mediawiki --config name='my media wiki' --config debug=true

If a key gets defined multiple times the last value will override any earlier
values. For example,

  juju deploy mediawiki --config name='my media wiki' --config mycfg.yaml

Similar to the 'juju config' command, if the value begins with an '@' character,
it will be treated as a path to a config file and its contents will be assigned
to the specified key. For example,

  juju deploy mediawiki --config name='@wiki-name.txt"

will set the 'name' key to the contents of file 'wiki-name.txt'.

If mycfg.yaml contains a value for 'name', it will override the earlier 'my
media wiki' value. The same applies to single value options. For example,

  juju deploy mediawiki --config name='a media wiki' --config name='my wiki'

the value of 'my wiki' will be used.

Use the '--resource' option to upload resources needed by the charm. This
option may be repeated if multiple resources are needed:

  juju deploy foo --resource bar=/some/file.tgz --resource baz=./docs/cfg.xml

Where 'bar' and 'baz' are named in the metadata file for charm 'foo'.

Use the '--to' option to deploy to an existing machine or container by
specifying a "placement directive". The `status` command should be used for
guidance on how to refer to machines. A few placement directives are
provider-dependent (e.g.: 'zone').

In more complex scenarios, "network spaces" are used to partition the cloud
networking layer into sets of subnets. Instances hosting units inside the same
space can communicate with each other without any firewalls. Traffic crossing
space boundaries could be subject to firewall and access restrictions. Using
spaces as deployment targets, rather than their individual subnets, allows Juju
to perform automatic distribution of units across availability zones to support
high availability for applications. Spaces help isolate applications and their
units, both for security purposes and to manage both traffic segregation and
congestion.

When deploying an application or adding machines, the 'spaces' constraint can
be used to define a comma-delimited list of required and forbidden spaces (the
latter prefixed with '^', similar to the 'tags' constraint).

When deploying bundles, machines specified in the bundle are added to the model
as new machines. Use the '--map-machines=existing' option to make use of any
existing machines. To map particular existing machines to machines defined in
the bundle, multiple comma separated values of the form 'bundle-id=existing-id'
can be passed. For example, for a bundle that specifies machines 1, 2, and 3;
and a model that has existing machines 1, 2, 3, and 4, the below deployment
would have existing machines 1 and 2 assigned to machines 1 and 2 defined in
the bundle and have existing machine 4 assigned to machine 3 defined in the
bundle.

  juju deploy mybundle --map-machines=existing,3=4

Only top level machines can be mapped in this way, just as only top level
machines can be defined in the machines section of the bundle.

When charms that include LXD profiles are deployed the profiles are validated
for security purposes by allowing only certain configurations and devices. Use
the '--force' option to bypass this check. Doing so is not recommended as it
can lead to unexpected behaviour.

Further reading: https://juju.is/docs/olm/manage-applications

Examples:

Deploy to a new machine:

    juju deploy apache2

Deploy to machine 23:

    juju deploy mysql --to 23

Deploy to a new LXD container on a new machine:

    juju deploy mysql --to lxd

Deploy to a new LXD container on machine 25:

    juju deploy mysql --to lxd:25

Deploy to LXD container 3 on machine 24:

    juju deploy mysql --to 24/lxd/3

Deploy 2 units, one on machine 3 and one to a new LXD container on machine 5:

    juju deploy mysql -n 2 --to 3,lxd:5

Deploy 3 units, one on machine 3 and the remaining two on new machines:

    juju deploy mysql -n 3 --to 3

Deploy to a machine with at least 8 GiB of memory:

    juju deploy postgresql --constraints mem=8G

Deploy to a specific availability zone (provider-dependent):

    juju deploy mysql --to zone=us-east-1a

Deploy to a specific MAAS node:

    juju deploy mysql --to host.maas

Deploy to a machine that is in the 'dmz' network space but not in either the
'cms' nor the 'database' spaces:

    juju deploy haproxy -n 2 --constraints spaces=dmz,^cms,^database

Deploy a k8s charm that requires a single Nvidia GPU:

    juju deploy mycharm --device miner=1,nvidia.com/gpu

Deploy a k8s charm that requires two Nvidia GPUs that have an
attribute of 'gpu=nvidia-tesla-p100':

    juju deploy mycharm --device \
       twingpu=2,nvidia.com/gpu,gpu=nvidia-tesla-p100

See also:
    add-relation
    add-unit
    config
    expose
    get-constraints
    refresh
    set-constraints
    spaces
```