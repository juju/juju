(hook-command-resource-get)=
# `resource-get`
## Summary
Get the path to the locally cached resource file.

## Usage
``` resource-get [options] <resource name>```

## Examples

    # resource-get software
    /var/lib/juju/agents/unit-resources-example-0/resources/software/software.zip


## Details

"resource-get" is used while a hook is running to get the local path
to the file for the identified resource. This file is an fs-local copy,
unique to the unit for which the hook is running. It is downloaded from
the controller, if necessary.

If "resource-get" for a resource has not been run before (for the unit)
then the resource is downloaded from the controller at the revision
associated with the unit's application. That file is stored in the unit's
local cache. If "resource-get" *has* been run before then each
subsequent run syncs the resource with the controller. This ensures
that the revision of the unit-local copy of the resource matches the
revision of the resource associated with the unit's application.

Either way, the path provided by "resource-get" references the
up-to-date file for the resource. Note that the resource may get
updated on the controller for the application at any time, meaning the
cached copy *may* be out of date at any time after you call
"resource-get". Consequently, the command should be run at every
point where it is critical that the resource be up to date.

The "upgrade-charm" hook is useful for keeping your charm's resources
on a unit up to date.  Run "resource-get" there for each of your
charm's resources to do so. The hook fires whenever the the file for
one of the application's resources changes on the controller (in addition
to when the charm itself changes). That means it happens in response
to "juju upgrade-charm" as well as to "juju push-resource".

Note that the "upgrade-charm" hook does not run when the unit is
started up. So be sure to run "resource-get" for your resources in the
"install" hook (or "config-changed", etc.).

Note that "resource-get" only provides an FS path to the resource file.
It does not provide any information about the resource (e.g. revision).

Further details:
resource-get fetches a resource from the Juju controller or Charmhub.
The command returns a local path to the file for a named resource.

If resource-get has not been run for the named resource previously, then the
resource is downloaded from the controller at the revision associated with
the unit’s application. That file is stored in the unit’s local cache.
If resource-get has been run before then each subsequent run synchronizes the
resource with the controller. This ensures that the revision of the unit-local
copy of the resource matches the revision of the resource associated with the
unit’s application.

The path provided by resource-get references the up-to-date file for the resource.
Note that the resource may get updated on the controller for the application at
any time, meaning the cached copy may be out of date at any time after
resource-get is called. Consequently, the command should be run at every point
where it is critical for the resource be up to date.