(command-juju-register)=
# `juju register`
> See also: [add-user](#add-user), [change-user-password](#change-user-password), [unregister](#unregister)

## Summary
Registers a controller.

## Usage
```juju register [options] <registration string>|<controller host name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--replace` | false | replace any existing controller |

## Examples

    juju register MFATA3JvZDAnExMxMDQuMTU0LjQyLjQ0OjE3MDcwExAxMC4xMjguMC4yOjE3MDcwBCBEFCaXerhNImkKKabuX5ULWf2Bp4AzPNJEbXVWgraLrAA=

    juju register --replace MFATA3JvZDAnExMxMDQuMTU0LjQyLjQ0OjE3MDcwExAxMC4xMjguMC4yOjE3MDcwBCBEFCaXerhNImkKKabuX5ULWf2Bp4AzPNJEbXVWgraLrAA=

    juju register public-controller.example.com


## Details

The register command adds details of a controller to the local system.
This is done either by completing the user registration process that
began with the `juju add-user` command, or by providing the DNS host
name of a public controller.

To complete the user registration process, you should have been provided
with a `base64`-encoded blob of data (the output of `juju add-user`)
which can be copied and pasted as the &lt;string&gt; argument to 'register'.
You will be prompted for a password, which, once set, causes the
registration string to be voided. In order to start using Juju the user
can now either add a model or wait for a model to be shared with them.
Some machine providers will require the user to be in possession of
certain credentials in order to add a model.

If a new controller has been spun up to replace an existing one, and you want
to start using that replacement controller instead of the original one,
use the `--replace` option to overwrite any existing controller details based
on either a name or UUID match.

When adding a controller at a public address, authentication via some
external third party (for example Ubuntu SSO) will be required, usually
by using a web browser.