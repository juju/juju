# Introduction

Juju Actions provide a way for a charm author to build custom features
into their charm that can be invoked given the name of the Action and
optional arguments.


# High Level

## API Methods - Client facing


    GetActionDefinitions

    AddActions
    GetActions
    CancelActions

    WatchActions


## API Methods - Jujuc facing

    GetActions
    StartActions
    FinishActions

    WatchActions
    




# Details

In general, most of the API calls are bulk calls, and both in and out
parameters can be expected to have arrays of data.



## Client Facing

The Client Facing API is expected to be called from places like a
Command Line Interface (CLI), or a visual interface (GUI).


### GetActionDefinitions

`GetActionDefinitions` takes a charm or service name and uses it to
resolve the list of Actions that have been defined in that Charm.

The result will be an array of `ActionDefinitions` that will include
name and parameter information for each Action defined.


### AddActions

`AddActions` accepts an array of Action requests and queues each
Action up for the specified Unit or Service, along with any associated
parameters.

The results will include unique UUID values associated with each queued
Action.



### GetActions

`GetActions` uses the passed in array of Actions to build a filtered
list of actions that have been queued, using the optional Action UUID
field, or the Unit, or the Service name fields as a filter to list any
Actions matching the filters.

The results will show a list of queued actions and their current status,
including output for actions that are complete or that have failed.



### CancelActions

`CancelActions` takes a list of Action UUID's and attempts to cancel the
corresponding actions from the pending queue.

The results will be an array with results in each array slot
corresponding to the Action in the request.

If the cancellation is successful the result will indicate that the
Action was successfully cancelled, otherwise the result may indicate the
reason the cancellation failed, along with the current status of that
Action.



### WatchActions

`WatchActions` will return a watcher delegate that can be used to
iteratively query `Next()` to retrieve watcher events.





## Jujuc facing

The jujuc facing API is an internal API and is expected to be called
from the Uniter on the Service Unit.


### GetActions

`GetActions` would retrieve a list of Actions that are relevant to the
calling Service or Unit.



### StartActions

`StartActions` encapsulates a bulk call to notify the State server that
the specified (by UUID) Actions have been picked up to execute by the
calling Service or Unit.



### FinishActions

`FinishActions` is a bulk call to notify the State server that the
specified Actions have completed. The request will include details
specifying whether the Action was successful or not, and any associated
output.



### WatchActions

`WatchActions` provides a watcher that notifies when new Actions have
been queued.

