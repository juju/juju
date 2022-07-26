package logger

// LogTailer allows for retrieval of Juju's logs.
// It first returns any matching already recorded logs and
// then waits for additional matching logs as they appear.
type LogTailer interface {
	// Logs returns the channel through which the LogTailer returns Juju logs.
	// It will be closed when the tailer stops.
	Logs() <-chan *LogRecord

	// Dying returns a channel which will be closed as the LogTailer stops.
	Dying() <-chan struct{}

	// Stop is used to request that the LogTailer stops.
	// It blocks until the LogTailer has stopped.
	Stop() error

	// Err returns the error that caused the LogTailer to stopped.
	// If it hasn't stopped or stopped without error nil will be returned.
	Err() error
}
