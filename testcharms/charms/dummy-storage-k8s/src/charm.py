#!/usr/bin/env python3
"""Minimal k8s sidecar charm used to test Juju k8s storage features."""

import ops


class DummyStorageK8sCharm(ops.CharmBase):
    """Minimal k8s sidecar charm with a filesystem storage volume."""

    def __init__(self, framework: ops.Framework):
        super().__init__(framework)
        framework.observe(self.on["workload"].pebble_ready, self._on_pebble_ready)

    def _on_pebble_ready(self, _: ops.PebbleReadyEvent) -> None:
        self.unit.status = ops.ActiveStatus()


if __name__ == "__main__":
    ops.main(DummyStorageK8sCharm)
