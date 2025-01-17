(juju_agent)=
# `juju_agent`

```{toctree}
:hidden:

list-of-juju-introspect-macros/index

```

## Summary

Juju agent selector for introspect command. 

## Usage

```text
juju-agent [command]                                  
```

## Details

The juju-agent is one of the shell functions that 
is exported by the juju-introspect shell script. 
This subcommand is simply a selector for the  
correct agent to use when passing another subcommand
to juju instrospect.

By default, juju-agent will transfer the subcommand  
to the machine agent. 
If the machine agent is not running (i.e. if the 
agent name is null), then the it tries to transfer 
to the controller agent. If the controller agent is 
not running, then it tries to transfer to the application 
agent. And finally, if the application agent is not 
running, then it tries to transfer to the unit agent.

E.g:

    juju_agent metrics

Note: this example is equivalent to running juju_metrics.
