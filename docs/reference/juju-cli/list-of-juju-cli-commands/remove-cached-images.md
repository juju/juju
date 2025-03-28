(command-juju-remove-cached-images)=
# `juju remove-cached-images`

```
Usage: juju remove-cached-images [options]

Summary:
Remove cached OS images.

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
    The architecture of the image to remove eg amd64
--kind (= "")
    The image kind to remove eg lxd
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
--series (= "")
    The series of the image to remove eg xenial

Details:
Remove cached os images in the Juju model.

Images are identified by:
  Kind         eg "lxd"
  Release       eg "xenial"
  Architecture eg "amd64"

Examples:
  # Remove cached lxd image for xenial amd64.
  juju remove-cached-images --kind lxd --series xenial --arch amd64
```