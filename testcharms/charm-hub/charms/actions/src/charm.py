#!/usr/bin/env python3
# Copyright 2024 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.
import logging

import ops

logger = logging.getLogger(__name__)


class ActionsTestCharm(ops.CharmBase):

    def __init__(self, *args):
        super().__init__(*args)
        self.framework.observe(self.on.start, self._on_start)
        self.framework.observe(self.on.fortune_action, self._on_fortune_action)
        self.framework.observe(self.on.list_my_params_action, self._on_list_my_params_action)

    def _on_start(self, event: ops.StartEvent):
        self.unit.status = ops.ActiveStatus()

    def _on_fortune_action(self, event: ops.ActionEvent):
        fail = event.params["fail"]
        length = event.params["length"]
        if fail:
            event.fail(fail)
        else:
            if length == "short":
                event.set_results({"fortune": "A bug in the code is worth two in the documentation."})
            elif length == "long":
                event.set_results({"fortune": "Any fool can write code that a computer can understand. Good programmers write code that humans can understand"})

    def _on_list_my_params_action(self, event: ops.ActionEvent):
        event.set_results(event.params)

if __name__ == "__main__":  # pragma: nocover
    ops.main(ActionsTestCharm)  # type: ignore

