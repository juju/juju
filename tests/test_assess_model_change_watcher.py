#!/usr/bin/env python3

import logging
from io import BytesIO, StringIO, TextIOWrapper
import sys

from mock import (
    Mock,
    patch,
    )

from assess_model_change_watcher import (
    assess_model_change_watcher,
    _get_message,
    is_config_change_in_event,
    is_in_dict,
    listen_to_watcher,
    main,
    parse_args,
    )
from fakejuju import fake_juju_client
from tests import (
    parse_error,
    TestCase,
    )


class TestParseArgs(TestCase):

    def xxx_test_common_args(self):
        args = parse_args(["an-env", "/bin/juju", "/tmp/logs", "an-env-mod"])
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual("/tmp/logs", args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual(False, args.debug)

    def xxx_test_help(self):
        if isinstance(sys.stdout, TextIOWrapper):
            fake_stdout = StringIO()
        else:
            fake_stdout = BytesIO()
        with parse_error(self) as fake_stderr:
            with patch("sys.stdout", fake_stdout):
                parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())


class TestMain(TestCase):

    def xxx_test_main(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        env = object()
        client = Mock(spec=["is_jes_enabled"])
        with patch("assess_model_change_watcher.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_model_change_watcher.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch('deploy_stack.client_from_config',
                           return_value=client) as mock_cfc:
                    with patch("assess_model_change_watcher.assess_model_change_watcher",
                               autospec=True) as mock_assess:
                        main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_cfc.assert_called_once_with('an-env', "/bin/juju", debug=False,
                                         soft_deadline=None)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client, None)


class TestAssess(TestCase):

    watcher = [
        "application",
        "change",
        {
            "owner-tag": "",
            "status": {
                "version": "",
                "message": "Token is bar",
                "current": "active",
                "since": "2016-12-01T02:38:03.078457034Z"
            },
            "min-units": 0,
            "life": "alive",
            "charm-url": "local:trusty/dummy-source-0",
            "exposed": False,
            "name": "dummy-source",
            "model-uuid": "0b9a4bd8-4980-43cd-8764-cc3ead6ac364",
            "constraints": {},
            "subordinate": False,
            "config": {
                "token": "1234asdf"
            }
        }
    ]

    def xxx_test_model_change_watcher(self):
        # Using fake_client means that deploy and get_status have plausible
        # results.  Wrapping it in a Mock causes every call to be recorded, and
        # allows assertions to be made about calls.  Mocks and the fake client
        # can also be used separately.
        fake_client = Mock(wraps=fake_juju_client())
        fake_client.bootstrap()
        assess_model_change_watcher(fake_client, 'angsty')
        fake_client.deploy.assert_called_once_with('dummy-source')
        fake_client.wait_for_started.assert_called_once_with()
        self.assertEqual(
            1, fake_client.get_status().get_service_unit_count('dummy-source'))
        self.assertNotIn("TODO", self.log_stream.getvalue())

    def test_get_message(self):
        event = _get_message(self.watcher)
        self.assertEqual(event, self.watcher[2])

    def test_is_config_in_event(self):
        self.assertIsTrue(is_config_change_in_event(self.watcher))

    def test_is_config_in_event_false(self):
        watcher =["application", "change", {"foo": "bar"}]
        self.assertIsFalse(is_config_change_in_event(watcher))

    def test_listen_to_watcher(self):
        fake_watcher = FakeWatcher()
        future = Mock()
        with patch("assess_model_change_watcher.watcher", autospec=True,
                   return_value=fake_watcher) as w_mock:
            listen_to_watcher(is_config_change_in_event, None, future)


class FakeDelta:
    deltas = [TestAssess().watcher]

class FakeWatcher:

    @staticmethod
    def AllWatcher(self):
        return self

    def connect(self, conn):
        pass

    def Next(self):
        data = [FakeDelta()]
        yield data

