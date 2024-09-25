# Catacomb vs Tomb

Status: Accepted

Summary: Catacombs and tombs are used to manage the lifecycle of a worker. Catacombs are used when the worker needs to manage and tie its lifecycle to other workers. Tombs are used when the worker needs to manage its own lifecycle independently.

## Issue

When writing workers, it is important to manage the lifecycle of the worker. This includes starting and stopping the worker, and ensuring that the worker is stopped when the parent process is stopped. Understanding when to use a catacomb or a tomb is important to ensure that the worker is stopped correctly.

## Decision

When writing a worker, the decision to use a catacomb or a tomb should be based on the requirements of the worker. The following guidelines should be followed:

  - Use a catacomb when the worker needs to manage and tie its lifecycle to other workers. This includes watchers and sub workers that need to be stopped when the parent worker is stopped.
  - Use a tomb when the worker needs to manage its own lifecycle independently. This includes workers that are not tied to other workers and need to be stopped when the parent process is stopped.

The reason not to use a catacomb for all workers, is two fold:

  1. By using a tomb, you're making it clear that the worker is not tied to any other workers. This makes it clear to the reader, that the worker is independent.
  2. There is a slight overhead in using a catacomb, as it needs to manage the lifecycle of other workers. This overhead is not needed if the worker is independent.
