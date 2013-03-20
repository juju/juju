package store

import (
	"time"
)

func TimeToStamp(t time.Time) int32 {
	return timeToStamp(t)
}
