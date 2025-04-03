(hook-command)=
# Hook command

> See also: {ref}`Hook <hook>`

```{toctree}
:hidden:
hook-command/list-of-hook-commands/index
```


In Juju, a **hook tool  (or 'hook command')** is a Bash script located in `/var/lib/juju/tools/unit-<app name>-<unit ID>` that a charm uses to communicate with its Juju unit agent in response to a {ref}`hook <hook>`.

In the Juju ecosystem, in [Ops](https://ops.readthedocs.io/en/latest/), hook tools are accessed through Ops constructs, specifically, those constructs designed to be used in the definition of the event handlers associated with the Ops events that translate Juju {ref}`hooks <hook>`. For example, when your charm calls `ops.Unit.is_leader`, in the background this calls `~/hooks/unit-name/leader-get`; its output is wrapped and returned as a Python `True/False` value.

In Juju, you can use hook commands for troubleshooting.

````{dropdown} Example: Use relation-get to change relation data

```text
# Get the relation ID

$ juju show-unit synapse/0

...
  - relation-id: 7
    endpoint: synapse-peers
    related-endpoint: synapse-peers
   application-data:
      secret-id: secret://1234
    local-unit:
      in-scope: true


# Check the output:
$ juju exec --unit synapse/0 "relation-get -r 7 --app secret-id synapse/0"
secret://1234

# Change the data:
juju exec --unit synapse/0 "relation-set -r 7 --app secret-id=something-else"

# Check the output again to verify the change.
```

````
