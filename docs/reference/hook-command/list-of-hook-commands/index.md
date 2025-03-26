(list-of-hook-commands)=
<!---
This stub is the index file for the hook command reference, the contents table below is
populated by the code in conf.py on make doc-build
-->
# List of hook commands

```{important}

This list replicates the output of `juju help hook-tool` and of `juju help-tool <name of hook tool>`.

```

<!--Units deployed with Juju have a suite of tooling available to them, called ‘hook tools’. These commands provide the charm developer with a consistent interface to take action on the unit's behalf, such as opening ports, obtaining configuration, even determining which unit is the leader in a cluster. The listed hook-tools are available in any hook running on the unit, and are only available within ‘hook context’.-->



```{toctree}
:titlesonly:
:glob:

*

```
