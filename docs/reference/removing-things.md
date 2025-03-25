(removing-things)=
# Removing things

This document clarifies the various Juju commands that can be used to remove things, as well as a couple of options that can be used to force a removal.

## Removal terms

There is a distinction between the similar sounding commands `unregister`, `detach`, `remove`, `destroy`, and `kill`. These commands are ordered such that their effect increases in severity:

*   `Unregister` means to decouple a resource from a logical entity for the client. The effect is local to the client only and does not affect the logical entity in any way.

*   `Detach` means to decouple a resource from a logical entity (such as an application). The resource will remain available and the underlying cloud resources used by it also remain in place.

*   `Remove` means to cleanly remove a single logical entity. This is a destructive process, meaning the entity will no longer be available via Juju, and any underlying cloud resources used by it will be freed (however, this can often be overridden on a case-by-case basis to leave the underlying cloud resources in place).

*   `Destroy` means to cleanly tear down a logical entity, along with everything within these entities. This is a very destructive process.

*   `Kill` means to forcibly tear down an unresponsive logical entity, along with everything within it. This is a very destructive process that does not guarantee associated resources are cleaned up.

<!--REPLACE THIS NOTE WITH SEE ALSO'S FROM THE SPECIFIC DOCS TO THIS DOC.-->
```{note}

These command terms/prefixes do not apply to all commands in a generic way. The explanations above are merely intended to convey how a command generally operates and what its severity level is. 

```

 
## Forcing removals

Juju object removal commands do not succeed when there are errors in the multiple steps that are required to remove the underlying object. For instance, a unit will not remove properly if it has a hook error, or a model cannot be removed if application units are in an error state. This is an intentionally conservative approach to the deletion of things.

However, this policy can also be a source of frustration for users in certain situations (i.e. "I don't care, I just want my model gone!"). Because of this, several commands have a `--force` option.

Furthermore, even when utilising the `--force` option, the process may take more time than an administrator is willing to accept (i.e. "Just go away as quickly as possible!").  Because of this, several commands that support the `--force` option have, in addition, support for a `--no-wait` option.

```{note}

The `--force` and `--no-wait` options should be regarded as tools to wield as a last resort. Using them introduces a chance of associated parts (e.g., relations) not being cleaned up, which can lead to future problems.

```

As of `v.2.6.1`, this is the state of affairs for those commands that support at least the `--force` option:

command | `--force` | `--no-wait`
---------------|---------------|---------------
`destroy-model` | yes | yes
`detach-storage` | yes | no
`remove-application` | yes | yes
`remove-machine` | yes | yes
`remove-offer` | yes | no
`remove-relation` | yes | no
`remove-storage` | yes | no
`remove-unit` | yes | yes

When a command has `--force` but not `--no-wait`, this means that the combination of those options simply does not apply.

