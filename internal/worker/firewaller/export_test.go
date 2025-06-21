package firewaller

func FlushMachine(fw *Firewaller) error {
	return fw.flushMachine(&machineData{})
}

func SetNeedsToFlushModel(fw *Firewaller, needsToFlushModel bool) {
	fw.needsToFlushModel = needsToFlushModel
}
