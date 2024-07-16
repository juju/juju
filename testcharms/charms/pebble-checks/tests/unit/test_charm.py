import unittest
import unittest.mock

import ops
import ops.pebble
import ops.testing
from charm import PebbleChecksCharm, MockCheckInfo


class TestCharm(unittest.TestCase):
    def setUp(self):
        self.harness = ops.testing.Harness(PebbleChecksCharm)
        self.addCleanup(self.harness.cleanup)
        self.harness.set_can_connect("ubuntu", True)
        self.harness.begin()

    # TODO(CHARMTECH-165): update to use pebble-check-failed once ops updates:
    #                      https://github.com/canonical/operator/pull/1281
    def _pebble_emit_check(self, status, container_name, check_name):
        import os

        os.environ["JUJU_PEBBLE_CHECK_NAME"] = check_name
        event = ops.WorkloadEvent(
            None,
            self.harness.model.unit.get_container(container_name),
        )
        event.info = MockCheckInfo(event.workload, check_name)
        if status == "failed":
            self.harness.charm._on_check_failed(event)
        elif status == "recovered":
            self.harness.charm._on_check_recovered(event)

    def test_check_failed(self):
        # TODO: Remove this when ops has native support (see note above).
        import os
        import enum

        os.environ["JUJU_DISPATCH_PATH"] = "hooks/ubuntu-pebble-check-failed"
        self.addCleanup(os.environ.__delitem__, "JUJU_DISPATCH_PATH")
        self.addCleanup(os.environ.__delitem__, "JUJU_PEBBLE_CHECK_NAME")
        self.addCleanup(setattr, ops.pebble, "NoticeType", ops.pebble.NoticeType)

        class NoticeType(enum.Enum):
            CUSTOM = "custom"
            # Harness.pebble_notify() only permits "custom".
            CHANGE_UPDATE = "custom"

        ops.pebble.NoticeType = NoticeType

        def get_change(_: ops.pebble.Client, change_id: str):
            return ops.pebble.Change.from_dict(
                {
                    "id": change_id,
                    "kind": ops.pebble.ChangeKind.PERFORM_CHECK.value,
                    "summary": "",
                    "status": ops.pebble.ChangeStatus.ERROR.value,
                    "ready": False,
                    "spawn-time": "2021-02-10T04:36:22.118970777Z",
                }
            )

        with unittest.mock.patch.object(
            ops.testing._TestingPebbleClient, "get_change", get_change
        ):
            self.harness.pebble_notify(
                "ubuntu",
                "123",
                type=ops.pebble.NoticeType.CHANGE_UPDATE,
                data={"kind": "perform-check", "check-name": "exec-check"},
            )
        self._pebble_emit_check("failed", "ubuntu", "exec-check")
        self.assertEqual(
            self.harness.model.unit.status,
            ops.MaintenanceStatus("check failed: exec-check"),
        )

    def test_check_recovered(self):
        # TODO: Remove this when ops has native support (see note above).
        import os
        import enum

        os.environ["JUJU_DISPATCH_PATH"] = "hooks/ubuntu-pebble-check-recovered"
        self.addCleanup(os.environ.__delitem__, "JUJU_DISPATCH_PATH")
        self.addCleanup(os.environ.__delitem__, "JUJU_PEBBLE_CHECK_NAME")
        self.addCleanup(setattr, ops.pebble, "NoticeType", ops.pebble.NoticeType)

        class NoticeType(enum.Enum):
            CUSTOM = "custom"
            # Harness.pebble_notify() only permits "custom".
            CHANGE_UPDATE = "custom"

        ops.pebble.NoticeType = NoticeType

        def get_change(_: ops.pebble.Client, change_id: str):
            return ops.pebble.Change.from_dict(
                {
                    "id": change_id,
                    "kind": ops.pebble.ChangeKind.RECOVER_CHECK.value,
                    "summary": "",
                    "status": ops.pebble.ChangeStatus.DONE.value,
                    "ready": False,
                    "spawn-time": "2021-02-10T04:36:23.118970777Z",
                }
            )

        with unittest.mock.patch.object(
            ops.testing._TestingPebbleClient, "get_change", get_change
        ):
            self.harness.pebble_notify(
                "ubuntu",
                "123",
                type=ops.pebble.NoticeType.CHANGE_UPDATE,
                data={"kind": "recover-check", "check-name": "exec-check"},
            )
        self._pebble_emit_check("recovered", "ubuntu", "exec-check")
        self.assertEqual(
            self.harness.model.unit.status,
            ops.ActiveStatus("check recovered: exec-check"),
        )
