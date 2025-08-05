> See also: [login](#login), [controllers](#controllers), [status](#status)

## Summary
Migrate a workload model to another controller.

## Usage
```juju migrate [options] <model-name> <target-controller-name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |

## Details

The 'migrate' command begins the migration of a workload model from
its current controller to a new controller. This is useful for load
balancing when a controller is too busy, or as a way to upgrade a
model's controller to a newer Juju version.

In order to start a migration, the target controller must be in the
juju client's local configuration cache. See the 'login' command
for details of how to do this.

The 'migrate' command only starts a model migration - it does not wait
for its completion. The progress of a migration can be tracked using
the 'status' command and by consulting the logs.

Once the migration is complete, the model's machine and unit agents
will be connected to the new controller. The model will no longer be
available at the source controller.

If the migration fails for some reason, the model is returned to its
original state where it is managed by the original
controller.




