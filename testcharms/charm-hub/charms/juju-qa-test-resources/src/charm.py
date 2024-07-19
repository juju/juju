#!/usr/bin/env python3

import logging
import os
import stat
import subprocess

import ops

# Log messages can be retrieved using juju debug-log
logger = logging.getLogger(__name__)

class JujuQaTestResourcesCharm(ops.CharmBase):
    """Charm the service."""

    def __init__(self, *args):
        super().__init__(*args)
        self.framework.observe(self.on.install, self._on_install)

    def _on_install(self, event: ops.InstallEvent):
        resource_path = self.model.resources.fetch("runnable")
        os.chmod(resource_path, stat.S_IRUSR | stat.S_IWUSR | stat.S_IXUSR | stat.S_IRGRP | stat.S_IXGRP | stat.S_IROTH | stat.S_IXOTH)
        res = subprocess.run(resource_path, capture_output=True, text=True)
        logger.info("runnable=%s", res.stdout)
        self.unit.status = ops.ActiveStatus(res.stdout)

if __name__ == "__main__":  # pragma: nocover
    ops.main(JujuQaTestResourcesCharm)  # type: ignore
