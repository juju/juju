package parallel

import (
	"errors"
	"launchpad.net/tomb"
	"sync"
)

var (
	ErrStopped = errors.New("try was stopped")
	ErrClosed  = errors.New("try was closed")
)

type Try struct {
	tomb          tomb.Tomb
	closeMutex    sync.Mutex
	close         chan struct{}
	limiter       chan struct{}
	start         chan func()
	result        chan result
	moreImportant func(err0, err1 error) bool
	maxParallel   int
	endResult     interface{}
}

// NewTry returns an object that runs functions concurrently until one
// succeeds. The result of the first function that returns without an
// error is available from the Result method. If maxParallel is
// positive, it limits the nunber of concurrently running functions.
//
// The function moreImportant(err0, err1) returns whether err0 is
// considered more important than err1, and is used to determine the
// error return (see the Result method). It may be nil, in which case
// the first error always wins.
func NewTry(maxParallel int, moreImportant func(err0, err1 error) bool) *Try {
	if moreImportant == nil {
		moreImportant = neverMoreImportant
	}
	t := &Try{
		moreImportant: moreImportant,
		maxParallel:   maxParallel,
		close:         make(chan struct{}, 1),
		result:        make(chan result),
		start:         make(chan func()),
	}
	if t.maxParallel > 0 {
		t.limiter = make(chan struct{}, t.maxParallel)
		for i := 0; i < t.maxParallel; i++ {
			t.limiter <- struct{}{}
		}
	}
	go func() {
		defer t.tomb.Done()
		val, err := t.loop()
		t.endResult = val
		t.tomb.Kill(err)
	}()
	return t
}

func neverMoreImportant(err0, err1 error) bool {
	return false
}

type result struct {
	val interface{}
	err error
}

func (t *Try) loop() (interface{}, error) {
	var err error
	closed := false
	nrunning := 0
	for {
		select {
		case f := <-t.start:
			nrunning++
			go t.tryProc(f)
		case r := <-t.result:
			if r.err == nil {
				return r.val, r.err
			}
			if err == nil || t.moreImportant(r.err, err) {
				err = r.err
			}
			nrunning--
			if closed && nrunning == 0 {
				return nil, err
			}
		case <-t.tomb.Dying():
			if err == nil {
				return nil, ErrStopped
			}
			return nil, err
		case <-t.close:
			closed = true
			if nrunning == 0 {
				return nil, err
			}
		}
	}
}

func (t *Try) tryProc(try func()) {
	if t.limiter == nil {
		try()
		return
	}
	select {
	case <-t.limiter:
	case <-t.tomb.Dying():
		return
	}
	try()
	t.limiter <- struct{}{}
}

// Start starts running the given function and returns immediately. It
// returns ErrClosed if the Try has been closed, and ErrStopped if the try
// is finishing.
//
// The function should listen on the stop channel and return if
// it receives a value, though this is advisory only - the Try does not wait
// for all started functions to return before completing.
func (t *Try) Start(try func(stop <-chan struct{}) (interface{}, error)) error {
	dying := t.tomb.Dying()
	f := func() {
		val, err := try(dying)
		select {
		case t.result <- result{val, err}:
		case <-dying:
		}
	}
	select {
	case t.start <- f:
		return nil
	case <-dying:
		return ErrStopped
	case <-t.close:
		return ErrClosed
	}
}

// Close closes the Try. No more functions will be started
// if Start is called, and the Try will terminate when all
// outstanding functions have completed (or earlier
// if one succeeds)
func (c *Try) Close() {
	c.closeMutex.Lock()
	defer c.closeMutex.Unlock()
	select {
	case <-c.close:
	default:
		close(c.close)
	}
}

func (t *Try) Dead() <-chan struct{} {
	return t.tomb.Dead()
}

// Wait waits for the Try to complete and returns the same
// error returned by Result.
func (t *Try) Wait() error {
	return t.tomb.Wait()
}

// Result waits for the Try to complete and returns the result of the
// first successful function started by Start.
//
// If no function succeeded, the most important error, as determined by
// moreImportant, is returned. If there were no errors, ErrStopped is
// returned.
func (t *Try) Result() (interface{}, error) {
	err := t.tomb.Wait()
	return t.endResult, err
}

// Kill stops the try and all its currently executing functions.
func (t *Try) Kill() {
	t.tomb.Kill(nil)
}
