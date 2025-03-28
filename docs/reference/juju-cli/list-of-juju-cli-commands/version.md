(command-juju-version)=
# `juju version`

```
Usage: juju version [options]

Summary:
Print the Juju CLI client version.

Options:
--all  (= false)
    Prints all version information
--format  (= smart)
    Specify output format (json|smart|yaml)
-o, --output (= "")
    Specify an output file

Details:
Print only the Juju CLI client version.

To see the version of Juju running on a particular controller, use
  juju show-controller

To see the version of Juju running on a particular model, use
  juju show-model

See also:
    show-controller
    show-model
```