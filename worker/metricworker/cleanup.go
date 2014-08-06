package metricworker

import (
	"time"

	"github.com/juju/loggo"

	"github.com/juju/juju/state/api/metricsmanager"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.metricworker.cleanup")

type MetricCleanupWorker struct {
	work worker.Worker
}

func NewCleanup(client *metricsmanager.Client, notify chan struct{}) worker.Worker {
	f := func(stopCh <-chan struct{}) error {
		err := client.CleanupOldMetrics()
		if err != nil {
			logger.Errorf("failed to cleanup %v", err)
			return nil // We failed this time, but we'll retry later
		}
		if notify != nil {
			notify <- struct{}{}
		}
		return nil
	}
	return worker.NewPeriodicWorker(f, time.Second)
}
