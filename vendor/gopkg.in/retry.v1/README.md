# retry
--
    import "gopkg.in/retry.v1"

Package retry provides a framework for retrying actions. It does not itself
invoke the action to be retried, but is intended to be used in a retry loop.

The basic usage is as follows:

    for a := someStrategy.Start(); a.Next(); {
    	try()
    }

See examples for details of suggested usage.

## Usage

#### type Attempt

```go
type Attempt struct {
}
```

Attempt represents a running retry attempt.

#### func  Start

```go
func Start(strategy Strategy, clk Clock) *Attempt
```
Start begins a new sequence of attempts for the given strategy using the given
Clock implementation for time keeping. If clk is nil, the time package will be
used to keep time.

#### func  StartWithCancel

```go
func StartWithCancel(strategy Strategy, clk Clock, stop <-chan struct{}) *Attempt
```
StartWithCancel is like Start except that if a value is received on stop while
waiting, the attempt will be aborted.

#### func (*Attempt) Count

```go
func (a *Attempt) Count() int
```
Count returns the current attempt count number, starting at 1. It returns 0 if
called before Next is called. When the loop has terminated, it holds the total
number of retries made.

#### func (*Attempt) More

```go
func (a *Attempt) More() bool
```
More reports whether there are more retry attempts to be made. It does not
sleep.

If More returns false, Next will return false. If More returns true, Next will
return true except when the attempt has been explicitly stopped via the stop
channel.

#### func (*Attempt) Next

```go
func (a *Attempt) Next() bool
```
Next reports whether another attempt should be made, waiting as necessary until
it's time for the attempt. It always returns true the first time it is called
unless a value is received on the stop channel - we are guaranteed to make at
least one attempt unless stopped.

#### func (*Attempt) Stopped

```go
func (a *Attempt) Stopped() bool
```
Stopped reports whether the attempt has terminated because a value was received
on the stop channel.

#### type Clock

```go
type Clock interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}
```

Clock represents a virtual clock interface that can be replaced for testing.

#### type Exponential

```go
type Exponential struct {
	// Initial holds the initial delay.
	Initial time.Duration
	// Factor holds the factor that the delay time will be multiplied
	// by on each iteration.
	Factor float64
	// MaxDelay holds the maximum delay between the start
	// of attempts. If this is zero, there is no maximum delay.
	MaxDelay time.Duration
}
```

Exponential represents an exponential backoff retry strategy. To limit the
number of attempts or their overall duration, wrap this in LimitCount or
LimitDuration.

#### func (Exponential) NewTimer

```go
func (r Exponential) NewTimer(now time.Time) Timer
```
NewTimer implements Strategy.NewTimer.

#### type Regular

```go
type Regular struct {
	// Total specifies the total duration of the attempt.
	Total time.Duration

	// Delay specifies the interval between the start of each try
	// in the burst. If an try takes longer than Delay, the
	// next try will happen immediately.
	Delay time.Duration

	// Min holds the minimum number of retries. It overrides Total.
	// To limit the maximum number of retries, use LimitCount.
	Min int
}
```

Regular represents a strategy that repeats at regular intervals.

#### func (Regular) NewTimer

```go
func (r Regular) NewTimer(now time.Time) Timer
```
NewTimer implements Strategy.NewTimer.

#### func (Regular) Start

```go
func (r Regular) Start(clk Clock) *Attempt
```
Start is short for Start(r, clk, nil)

#### type Strategy

```go
type Strategy interface {
	// NewTimer is called when the strategy is started - it is
	// called with the time that the strategy is started and returns
	// an object that is used to find out how long to sleep before
	// each retry attempt.
	NewTimer(now time.Time) Timer
}
```

Strategy is implemented by types that represent a retry strategy.

Note: You probably won't need to implement a new strategy - the existing types
and functions are intended to be sufficient for most purposes.

#### func  LimitCount

```go
func LimitCount(n int, strategy Strategy) Strategy
```
LimitCount limits the number of attempts that the given strategy will perform to
n. Note that all strategies will allow at least one attempt.

#### func  LimitTime

```go
func LimitTime(limit time.Duration, strategy Strategy) Strategy
```
LimitTime limits the given strategy such that no attempt will made after the
given duration has elapsed.

#### type Timer

```go
type Timer interface {
	// NextSleep is called with the time that Next or More has been
	// called and returns the length of time to sleep before the
	// next retry. If no more attempts should be made it should
	// return false, and the returned duration will be ignored.
	//
	// Note that NextSleep is called once after each iteration has
	// completed, assuming the retry loop is continuing.
	NextSleep(now time.Time) (time.Duration, bool)
}
```

Timer represents a source of timing events for a retry strategy.
