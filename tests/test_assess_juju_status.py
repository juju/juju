"""Tests for assess_juju_status module."""

import logging

from mock import (
    Mock,
    patch,
    )
from assess_juju_status import (
    assess_juju_status,
    verify_application_status,
    parse_args,
    main,
    )
from jujupy import fake_juju_client
from tests import (
    TestCase,
    )
from jujupy.client import (
    Status,
    )
from assess_min_version import (
    JujuAssertionError
)


class TestParseArgs(TestCase):

    def test_common_args(self):
        args = parse_args(["an-env", "/bin/juju", "/tmp/logs", "an-env-mod"])
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual("/tmp/logs", args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual(False, args.debug)

    def test_optional_args(self):
        args = parse_args(["an-env", "/bin/juju", "/tmp/logs", "an-env-mod",
                           "--charm-app", "dummy-source",
                           "--series", "foo"])
        self.assertEqual("dummy-source", args.charm_app)
        self.assertEquals("foo", args.series)


class TestMain(TestCase):

    def test_main(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        client = Mock(spec=["is_jes_enabled"])
        with patch("assess_juju_status.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_juju_status.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch('deploy_stack.client_from_config',
                           return_value=client) as mock_cfc:
                    with patch("assess_juju_status.assess_juju_status",
                               autospec=True) as mock_assess:
                        main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_cfc.assert_called_once_with('an-env', "/bin/juju", debug=False,
                                         soft_deadline=None)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client, "dummy-source", "xenial")


class TestAssess(TestCase):

    def test_juju_status(self):
        fake_client = Mock(wraps=fake_juju_client())
        fake_client.bootstrap()
        assess_juju_status(fake_client, 'dummy-source', 'xenial')
        fake_client.deploy.assert_called_once_with(
            'dummy-source')
        fake_client.wait_for_started.assert_called_once_with()
        self.assertEqual(
            1, fake_client.get_status().get_service_unit_count('dummy-source'))

    def test_juju_status_application_status(self):
        fake_client = Mock(wraps=fake_juju_client())
        fake_client.bootstrap()
        assess_juju_status(fake_client, 'dummy-source', 'xenial')
        self.assertIn("verified juju application status successfully",
                      self.log_stream.getvalue())

    def test_juju_status_application_successfully(self):
        fake_client = Mock(wraps=fake_juju_client())
        app_status = Status({
            'applications': {
                'fakejob': {
                    'units': {
                        'fakejob/0': {
                            'juju-status': {
                                'current': 'idle',
                                'since': 'DD MM YYYY hh:mm:ss',
                                'version': '2.0.0',
                            },
                        },
                    },
                }
            },
        }, '')
        fake_client.bootstrap()
        assess_juju_status(fake_client, 'dummy-source', 'xenial')
        with patch.object(fake_client, 'get_status', autospec=True) as ags:
            ags.return_value = app_status
            verify_application_status(fake_client, "fakejob")
            self.assertIn("verified juju application status successfully",
                          self.log_stream.getvalue())

    def test_juju_status_application_status_not_found(self):
        fake_client = Mock(wraps=fake_juju_client())
        app_status = Status({
            'applications': {
                'fakejob': {
                    'units': {
                        'fakejob/0': {
                            'juju-status': {
                            },
                        },
                    },
                }
            },
        }, '')
        fake_client.bootstrap()
        assess_juju_status(fake_client, 'dummy-source', 'xenial')
        with patch.object(fake_client, 'get_status', autospec=True) as ags:
            ags.return_value = app_status
            with self.assertRaisesRegexp(
                    JujuAssertionError, "application status not found"):
                verify_application_status(fake_client, "fakejob")
