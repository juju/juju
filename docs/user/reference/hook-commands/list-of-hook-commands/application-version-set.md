(hook-command-application-version-set)=
# `application-version-set`
## Summary
Specify which version of the application is deployed.

## Usage
``` application-version-set [options] <new-version>```

## Examples

    application-version-set 1.1.10


## Details

application-version-set tells Juju which version of the application
software is running. This could be a package version number or some
other useful identifier, such as a Git hash, that indicates the
version of the deployed software. (It shouldn't be confused with the
charm revision.) The version set will be displayed in "juju status"
output for the application.