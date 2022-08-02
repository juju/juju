#!/usr/bin/env python3

import logging
import traceback

from ops.charm import CharmBase
from ops.main import main
from ops.model import ActiveStatus

logger = logging.getLogger(__name__)


class RefresherCharm(CharmBase):
    """Charm the service."""

    def __init__(self, *args):
        super().__init__(*args)
        self.framework.observe(self.on.upgrade_charm, self._on_upgrade_charm)
        self.framework.observe(self.on.install, self._on_install_charm)

    def _on_install_charm(self, event):
        self.app.status = ActiveStatus("installed")
        self.unit.status = ActiveStatus("ready")

    def _on_upgrade_charm(self, event):
        logger.info("Running upgrade hook")
        self.app.status = ActiveStatus("upgrade hook ran v2")
        self.unit.status = ActiveStatus("upgrade hook ran v2")

if __name__ == "__main__":
    main(RefresherCharm)
