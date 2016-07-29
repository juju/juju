// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

/*
package mutex provides a named machine level mutex shareable between processes.
[godoc-link-here]

Mutexes have names, and the a mutex with the same name can only be locked by one
instance of the mutex at a time, even across process boundaries.

If a process dies while the mutex is held, the mutex is automatically released.

The linux implementation uses abstract domain sockets, windows uses a named
semaphore, and other platforms use flock on a temp file.

*/
package mutex
