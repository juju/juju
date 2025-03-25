(juju-introspect)=
# `juju-introspect`


```{toctree}
:hidden:

list-of-juju-introspect-macros/index

```



## Summary

Introspect Juju agents running on this machine.


## Usage

```text
juju-introspect [options] (--listen=...|<path> [key=value [...]])
```

### Options

```text
--agent (= "")
    agent to introspect (defaults to machine agent)
--data-dir (= "/var/lib/juju")
    Juju base data directory
--listen (= "")
    address on which to expose the introspection socket
--post  (= false)
    perform a POST action rather than a GET
--verbose  (= false)
    show query path and args
```

## Details


Introspect Juju agents running on this machine.

The juju-introspect command can be used to expose
the agent's introspection socket via HTTP, using
the --listen flag. e.g.

    juju-introspect --listen=:6060

Otherwise, a single positional argument is required,
which is the path to query. e.g.

    juju-introspect /debug/pprof/heap?debug=1

By default, juju-introspect operates on the
machine agent. If you wish to introspect a
unit agent on the machine, you can specify the
agent using --agent. e.g.

    juju-introspect --agent=unit-mysql-0 metrics
