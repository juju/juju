#!/usr/bin/env python3
# Copyright 2025 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.

"""Charm the application."""

import logging

import ops
import requests

logger = logging.getLogger(__name__)


class ResourceK8SCharm(ops.CharmBase):
    """Charm the application."""

    def __init__(self, framework: ops.Framework):
        super().__init__(framework)
        framework.observe(self.on["app-container"].pebble_ready, self._on_pebble_ready)
        self.framework.observe(self.on.update_status, self._on_update_status)

    def _on_pebble_ready(self, event: ops.PebbleReadyEvent):
        """Handle pebble-ready event."""
        container = self.unit.get_container("app-container")

        # The resource whoami server service should be defined in
        # /var/lib/pebble/default/layers on resource container with the option
        # "startup" set to "enabled". autostart() will then start this service.
        container.autostart()

        self.unit.status = ops.ActiveStatus("resource name server started")
        self._get_whoami()

    def _on_update_status(self, _):
        self._get_whoami()

    def _get_whoami(self):
        # The oci image resource should be running its whoami server at
        # localhost:8080.
        response = requests.get("http://localhost:8080")
        if response.status_code == 200:
            self.unit.status = ops.ActiveStatus("Resource container whoami server: " + str(response.text))
        else:
            response.raise_for_status()


if __name__ == "__main__":
    ops.main(ResourceK8SCharm)
