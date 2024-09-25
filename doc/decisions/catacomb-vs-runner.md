# catacomb.Catacomb vs worker.Runner

Status: Accepted

Summary: Catacombs and runners are used to manage the lifecycle of multiple workers. Catacombs are used when the worker needs to manage and tie its lifecycle to other workers. Runners extend that capability by allowing additional management of the workers.

## Issue

When writing workers, there might be a point when you need to manage a pool of workers. If the pool of workers need to restarted based on a certain error, then that can add a lot of complexity to the worker that just uses a catacomb.

## Decision

When writing a worker that needs to manage a pool of workers, the decision to use a catacomb or a runner should be based on the requirements of the worker. The following guidelines should be followed:

  - Use a catacomb when the worker needs to manage and tie its lifecycle to other workers. This includes watchers and sub workers that need to be stopped when the parent worker is stopped.
  - Use a runner when the worker needs to manage a pool of workers. This includes workers that need to be restarted based on a certain error, or workers that need to be managed in a specific way.
