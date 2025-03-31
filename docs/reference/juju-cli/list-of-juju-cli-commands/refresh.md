(command-juju-refresh)=
# `juju refresh`
> See also: [deploy](#deploy)

## Summary
Refresh an application's charm.

## Usage
```juju refresh [options] <application>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--base` |  | Select a different base than what is currently running. |
| `--bind` |  | Configure application endpoint bindings to spaces |
| `--channel` |  | Channel to use when getting the charm from Charmhub |
| `--config` |  | Either a path to yaml-formatted application config file or a key=value pair  |
| `--force` | false | Allow a charm to be refreshed which bypasses LXD profile allow list |
| `--force-base`, `--force-series` | false | Refresh even if the base of the deployed application is not supported by the new charm |
| `--force-units` | false | Refresh all units immediately, even if in error state |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--path` |  | Refresh to a charm located at path |
| `--resource` |  | Resource to be uploaded to the controller |
| `--revision` | -1 | Explicit revision of current charm |
| `--storage` |  | Charm storage directives |
| `--switch` |  | Crossgrade to a different charm |
| `--trust` | unset | Allows charm to run hooks that require access credentials |

## Examples

To refresh the storage constraints for the application foo:

	juju refresh foo --storage cache=ssd,10G

To refresh the application config from a file for application foo:

	juju refresh foo --config config.yaml

To refresh the resources for application foo:

	juju refresh foo --resource bar=/some/file.tgz --resource baz=./docs/cfg.xml


## Details

When no options are set, the application's charm will be refreshed to the latest
revision available in the repository from which it was originally deployed. An
explicit revision can be chosen with the --revision option.

Refreshing a local packaged charm will require a path to be supplied to allow an
updated copy of the charm.

Deploying from a path is intended to suit the workflow of a charm author working
on a single client machine; use of this deployment method from multiple clients
is not supported and may lead to confusing behaviour. Each local packaged charm
gets uploaded with the revision specified in the charm, if possible, otherwise
it gets a unique revision (highest in state + 1).

When deploying from a path, the --path option is used to specify the location
of the packaged charm. Note that the charm must match what was originally used
to deploy the charm as a superficial check that the updated charm is compatible.

Resources may be uploaded at upgrade time by specifying the --resource option.
Following the resource option should be name=filepath pair.  This option may be
repeated more than once to upload more than one resource.

  juju refresh foo --resource bar=/some/file.tgz --resource baz=./docs/cfg.xml

Where bar and baz are resources named in the metadata for the foo charm.

Storage directives may be added or updated at upgrade time by specifying
the --storage option, with the same format as specified in "juju deploy".
If new required storage is added by the new charm revision, then you must
specify directives or the defaults will be applied.

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

The new charm may add new relations and configuration settings.

The new charm may also need to be granted access to trusted credentials.
Use --trust to grant such access.
Or use --trust=false to revoke such access.

--switch and --path are mutually exclusive.

--path and --revision are mutually exclusive. The revision of the updated charm
is determined by the contents of the charm at the specified path.

--switch and --revision are mutually exclusive.

Use of the --force-units option is not generally recommended; units upgraded
while in an error state will not have upgrade-charm hooks executed, and may
cause unexpected behavior.

--force option for LXD Profiles is not generally recommended when upgrading an
application; overriding profiles on the container may cause unexpected
behavior.