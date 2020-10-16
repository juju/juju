# Cleanup

Cleanup is called by the cleaner worker.  The cleaner worker watches the
cleanups collection and acts when something changes in the collection related
to the current model.  It will also act on a period time frame.

Cleanup will interate over the current docs in the collection and call a
method to handle the doc based on its kind.  If the method is successful,
the corresponding doc will be removed from the collection.  Otherwise the
method will be retried later.

Docs in the cleanups collection are handled in a specific order. E.g. cleaning
up a machine requires that it's units are gone, so clean up the units first.

## Model

Model cleanup starts with `juju destroy-model`, or `juju destroy-controller`.
Simplistically model destroyOps, causes the model is marked as dying and up to
5 docs are inserted into the cleanup collection:
  * cleanupModelsForDyingController, only for controller models
  * cleanupApplicationsForDyingModel,  only with non empty models
  * cleanupMachinesForDyingModel, only with non empty IAAS models
  * cleanupStorageForDyingModel, only with non empty models with storage
  * cleanupBranchesForDyingModel, for all models

## Docs in the Cleanup collection

### cleanupModelsForDyingController
### cleanupApplicationsForDyingModel

First it attempts to remove all remote applications from the model, then
attempts to remove all applications from the model.

Removing an application inserts a cleanupForceApplication into the cleanups
collection.

### cleanupMachinesForDyingModel

For all machines in the model, which are not managers, call 
DestroyWithContainers. If force is used, call ForceDestroy instead. Force is
not allowed for manual machines.  The first failure causes the method to exit.

DestroyWithContainers advances the machine's lifecycle to Dying, and inserts a
cleanupDyingMachine job into the cleanups collection.

Once a machine is dying, it's lifecycle takes over.

### cleanupStorageForDyingModel
### cleanupBranchesForDyingModel
### cleanupDyingMachine

For the dying machine, call cleanupDyingMachineResources.  After that, if
force is in use, and the machine hasn't been forcedestroyed yet, schedule
cleanupForceRemoveMachine by inserting the doc into the cleanups collection.

cleanupDyingMachineResources checks that the machine has no filesystem nor
volume attachments.  Then calls cleanupDyingEntityStorage, which detaches
all detachable storge and destroys storage which cannot be detached.

### cleanupForceRemoveMachine

### cleanupForceApplication

