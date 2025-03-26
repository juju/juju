(command-juju-remove-application)=
# `juju remove-application`
> See also: [scale-application](#scale-application), [show-application](#show-application)

## Summary
Remove applications from the model.

## Usage
```juju remove-application [options] <application> [<application>...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--destroy-storage` | false | Destroy storage attached to application units |
| `--dry-run` | false | Print what this command would remove without removing |
| `--force` | false | Completely remove an application and all its dependencies |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-prompt` | false | Do not ask for confirmation. Overrides `mode` model config setting |
| `--no-wait` | false | Rush through application removal without waiting for each individual step to complete |

## Examples

    juju remove-application hadoop
    juju remove-application --force hadoop
    juju remove-application --force --no-wait hadoop
    juju remove-application -m test-model mariadb


## Details
Removing an application will terminate any relations that application has, remove
all units of the application, and in the case that this leaves machines with
no running applications, Juju will also remove the machine. For this reason,
you should retrieve any logs or data required from applications and units 
before removing them. Removing units which are co-located with units of
other charms or a Juju controller will not result in the removal of the
machine.

Sometimes, the removal of the application may fail as Juju encounters errors
and failures that need to be dealt with before an application can be removed.
For example, Juju will not remove an application if there are hook failures.
However, at times, there is a need to remove an application ignoring
all operational errors. In these rare cases, use --force option but note 
that --force will also remove all units of the application, its subordinates
and, potentially, machines without given them the opportunity to shutdown cleanly.

Application removal is a multi-step process. Under normal circumstances, Juju will not
proceed to the next step until the current step has finished. 
However, when using --force, users can also specify --no-wait to progress through steps 
without delay waiting for each step to complete.