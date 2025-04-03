(command-juju-cached-images)=
# `juju cached-images`

```
Usage: juju cached-images [options]

Summary:
Shows cached os images.

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
--arch (= "")
    The architecture of the image to list eg amd64
--format  (= yaml)
    Specify output format (json|yaml)
--kind (= "")
    The image kind to list eg lxd
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-o, --output (= "")
    Specify an output file
--series (= "")
    The series of the image to list eg xenial

Details:
List cached os images in the Juju model.

Images can be filtered on:
  Kind         eg "lxd"
  Release       eg "xenial"
  Architecture eg "amd64"
The filter attributes are optional.

Examples:
  # List all cached images.
  juju cached-images

  # List cached images for xenial.
  juju cached-images --series xenial

  # List all cached lxd images for xenial amd64.
  juju cached-images --kind lxd --series xenial --arch amd64

Aliases: list-cached-images
```