(command-juju-refresh)=
# `juju refresh`

```
Usage: juju refresh [options] <application>

Summary:
Refresh an application's charm.

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
--bind (= "")
    Configure application endpoint bindings to spaces
--channel (= "")
    Channel to use when getting the charm or bundle from the charm store or charm hub
--config  (= )
    Path to yaml-formatted application config
--force  (= false)
    Allow a charm to be refreshed which bypasses LXD profile allow list
--force-series  (= false)
    Refresh even if series of deployed applications are not supported by the new charm
--force-units  (= false)
    Refresh all units immediately, even if in error state
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
--path (= "")
    Refresh to a charm located at path
--resource  (= )
    Resource to be uploaded to the controller
--revision  (= -1)
    Explicit revision of current charm
--storage  (= )
    Charm storage constraints
--switch (= "")
    Crossgrade to a different charm

Details:
When no options are set, the application's charm will be refreshed to the latest
revision available in the repository from which it was originally deployed. An
explicit revision can be chosen with the --revision option.

A path will need to be supplied to allow an updated copy of the charm
to be located.

Deploying from a path is intended to suit the workflow of a charm author working
on a single client machine; use of this deployment method from multiple clients
is not supported and may lead to confusing behaviour. Each local charm gets
uploaded with the revision specified in the charm, if possible, otherwise it
gets a unique revision (highest in state + 1).

When deploying from a path, the --path option is used to specify the location from
which to load the updated charm. Note that the directory containing the charm must
match what was originally used to deploy the charm as a superficial check that the
updated charm is compatible.

Resources may be uploaded at upgrade time by specifying the --resource option.
Following the resource option should be name=filepath pair.  This option may be
repeated more than once to upload more than one resource.

  juju refresh foo --resource bar=/some/file.tgz --resource baz=./docs/cfg.xml

Where bar and baz are resources named in the metadata for the foo charm.

Storage constraints may be added or updated at upgrade time by specifying
the --storage option, with the same format as specified in "juju deploy".
If new required storage is added by the new charm revision, then you must
specify constraints or the defaults will be applied.

  juju refresh foo --storage cache=ssd,10G

Charm settings may be added or updated at upgrade time by specifying the
--config option, pointing to a YAML-encoded application config file.

  juju refresh foo --config config.yaml

If the new version of a charm does not explicitly support the application's series, the
upgrade is disallowed unless the --force-series option is used. This option should be
used with caution since using a charm on a machine running an unsupported series may
cause unexpected behavior.

The --switch option allows you to replace the charm with an entirely different one.
The new charm's URL and revision are inferred as they would be when running a
deploy command.

Please note that --switch is dangerous, because juju only has limited
information with which to determine compatibility; the operation will succeed,
regardless of potential havoc, so long as the following conditions hold:

- The new charm must declare all relations that the application is currently
  participating in.
- All config settings shared by the old and new charms must
  have the same types.
- Charms changing from CharmStore (cs: prefix) to CharmHub require a
  homogeneous architecture for applications.

The new charm may add new relations and configuration settings.

--switch and --path are mutually exclusive.

--path and --revision are mutually exclusive. The revision of the updated charm
is determined by the contents of the charm at the specified path.

--switch and --revision are mutually exclusive. To specify a given revision
number with --switch, give it in the charm URL, for instance "cs:wordpress-5"
would specify revision number 5 of the wordpress charm.

Use of the --force-units option is not generally recommended; units upgraded
while in an error state will not have refreshed hooks executed, and may cause
unexpected behavior.

--force option for LXD Profiles is not generally recommended when upgrading an
application; overriding profiles on the container may cause unexpected
behavior.

Aliases: upgrade-charm
```