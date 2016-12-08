"""Tests for assess_model_defaults module."""

import logging
import StringIO

from mock import (
    Mock,
    patch,
    )
import yaml

from assess_model_defaults import (
    assess_model_defaults,
    assess_model_defaults_controller,
    get_model_defaults,
    main,
    ModelDefault,
    parse_args,
    set_model_defaults,
    unset_model_defaults,
    )
from fakejuju import fake_juju_client
from tests import (
    parse_error,
    TestCase,
    )


class FakeJujuModelDefaults:

    def __init__(self, defaults=None):
        self.model_defaults = {}
        if defaults is not None:
            for (key, value) in defaults.items():
                self.model_defaults[key] = {'default': value}

    test_mode_example = {
        'test-mode': {
            'default': False,  # Was false, not sure why it was unquoted.
            'controller': 'true',
            'regions': [{'name': 'localhost', 'value': 'true'}]
            }
        }

    def get_model_defaults(self, model_key):
        return ModelDefault(model_key, self.model_defaults[model_key])

    def set_model_defaults(self, model_key, value):
        self.model_defaults[model_key]['controller'] = value

    def unset_model_defaults(self, model_key):
        del self.model_defaults[model_key]


class TestParseArgs(TestCase):

    def test_common_args(self):
        args = parse_args(["an-env", "/bin/juju", "/tmp/logs", "an-env-mod"])
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual("/tmp/logs", args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual(False, args.debug)

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch("sys.stdout", fake_stdout):
                parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())
        self.assertNotIn("TODO", fake_stdout.getvalue())


class TestMain(TestCase):

    def test_main(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        client = Mock(spec=["is_jes_enabled"])
        with patch("assess_model_defaults.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_model_defaults.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch('deploy_stack.client_from_config',
                           return_value=client) as mock_cfc:
                    with patch("assess_model_defaults.assess_model_defaults",
                               autospec=True) as mock_assess:
                        main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_cfc.assert_called_once_with('an-env', "/bin/juju", debug=False,
                                         soft_deadline=None)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client)


class TestJujuWrappers(TestCase):

    def test_get_model_defaults(self):
        raw_yaml = yaml.safe_dump({'some-key': {'default': 'black'}})
        client = fake_juju_client()
        with patch.object(client, 'get_juju_output', autospec=True,
                          return_value=raw_yaml) as output_mock:
            retval = get_model_defaults(client, 'some-key')
        self.assertEqual(ModelDefault('some-key', {'default': 'black'}),
                         retval)
        output_mock.assert_called_once_with(
            'model-defaults', '--format', 'yaml', 'some-key')

    def test_set_model_defaults(self):
        client = fake_juju_client()
        with patch.object(client, 'juju', autospec=True) as juju_mock:
            set_model_defaults(client, 'some-key', 'white')
        juju_mock.assert_called_once_with(
            'model-defaults', ('some-key=white',))

    def test_unset_model_defaults(self):
        client = fake_juju_client()
        with patch.object(client, 'juju', autospec=True) as juju_mock:
            unset_model_defaults(client, 'some-key')
        juju_mock.assert_called_once_with(
            'model-defaults', ('--reset', 'some-key'))


class TestAssessModelDefaults(TestCase):

    def test_model_defaults_controller(self):
        client = FakeJujuModelDefaults({'some-key': 'black'})
        with patch('assess_model_defaults.get_model_defaults',
                   side_effect=lambda client, key:
                       client.get_model_defaults(key),
                   autospec=True) as get_mock:
            with patch('assess_model_defaults.set_model_defaults',
                       side_effect=lambda client, key, value:
                           client.set_model_defaults(key, value),
                       autospec=True) as set_mock:
                with patch('assess_model_defaults.unset_model_defaults',
                           autospec=True) as unset_mock:
                    assess_model_defaults_controller(
                        client, 'some-key', 'yellow')
        self.assertEqual(3, get_mock.call_count)
        set_mock.assert_called_once_with(client, 'some-key', 'yellow')
        unset_mock.assert_called_once_with(client, 'some-key')

    # def test_model_defaults_region(self):
    #     Add cloud & region arguments to the fake.

    def test_model_defaults(self):
        # Using fake_client means that deploy and get_status have plausible
        # results.  Wrapping it in a Mock causes every call to be recorded, and
        # allows assertions to be made about calls.  Mocks and the fake client
        # can also be used separately.
        fake_client = Mock(wraps=fake_juju_client())
        fake_client.bootstrap()
        assess_model_defaults(fake_client)
        fake_client.deploy.assert_called_once_with(
            'local:trusty/my-charm')
        fake_client.wait_for_started.assert_called_once_with()
        self.assertEqual(
            1, fake_client.get_status().get_service_unit_count('my-charm'))
        self.assertNotIn("TODO", self.log_stream.getvalue())
