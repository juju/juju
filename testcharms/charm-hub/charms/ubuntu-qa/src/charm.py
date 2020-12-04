#!/usr/bin/env python3
# Copyright 2020 juju-qa@canonical.com
# See LICENSE file for licensing details.

import logging

from ops.charm import CharmBase
from ops.model import ActiveStatus
from ops.main import main
from ops.framework import StoredState
from datetime import datetime

logger = logging.getLogger(__name__)


class ApitestUbuntuQaCharm(CharmBase):
    _stored = StoredState()

    def __init__(self, *args):
        super().__init__(*args)
        self.framework.observe(self.on.config_changed, self._on_config_changed)
        self.framework.observe(self.on.fortune_action, self._on_fortune_action)
        self.framework.observe(self.on.update_status, self._on_update_status)
        self._stored.set_default(things=[])
        self._stored.set_default(status="")

    def _on_config_changed(self, _):
        current = self.model.config["thing"]
        if current not in self._stored.things:
            logger.info("found a new thing: %r", current)
            self._stored.things.append(current)
        status = self.model.config["status"]
        if status != self._stored.status:
            logger.info("found a new status: %r", status)
            self._stored.status = status
            self.unit.status = ActiveStatus(status)

    def _on_fortune_action(self, event):
        fail = event.params["fail"]
        if fail:
            event.fail(fail)
        else:
            event.set_results({"fortune": "A bug in the code is worth two in the documentation."})

    def _on_update_status(self, _):
        now = datetime.now()
        date_time = now.strftime("%m/%d/%Y, %H:%M:%S")
        details = "it is now: {0}".format(date_time)
        self.unit.status = ActiveStatus(details)


if __name__ == "__main__":
    main(ApitestUbuntuQaCharm)
