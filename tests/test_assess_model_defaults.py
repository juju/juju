"""Tests for assess_model_defaults module."""

from copy import deepcopy
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
    juju_assert_equal,
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
from utility import (
    JujuAssertionError,
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

    def get_model_defaults(self, model_key, cloud=None, region=None):
        return ModelDefault(
            model_key, deepcopy(self.model_defaults[model_key]))

    def set_model_defaults(self, model_key, value, cloud=None, region=None):
        if cloud is None and region is None:
            self.model_defaults[model_key]['controller'] = value
        else:
            self.model_defaults[model_key].setdefault('regions', []).append({
                'name': region, 'value': value})

    def unset_model_defaults(self, model_key, cloud=None, region=None):
        if cloud is None and region is None:
            del self.model_defaults[model_key]['controller']
        else:
            for index, element in enumerate(
                    self.model_defaults[model_key]['regions']):
                if element['value'] == region:
                    self.model_defaults[model_key]['regions'].pop(index)
            if not self.model_defaults[model_key]['regions']:
                del self.model_defaults[model_key]['regions']


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


class TestAssert(TestCase):

    def test_juju_assert_equal(self):
        juju_assert_equal(0, 0, 'should-pass')
        with self.assertRaises(JujuAssertionError) as exc:
            juju_assert_equal(0, 1, 'should-fail')
        self.assertEqual(exc.exception.args, ('should-fail', 0, 1))


class TestAssessModelDefaults(TestCase):

    # Note: I paused this when I realized it would be extra once the
    # functions became client methods.
    @staticmethod
    def patch_all_model_defaults():
        def fake_get(client, key, cloud=None, region=None):
            return client.get_model_defaults(key, cloud, region)

        def fake_set(client, key, value, cloud=None, region=None):
            return client.set_model_defaults(key, value, cloud, region)

        def fake_unset(client, key, cloud=None, region=None):
            return client.unset_model_defaults(key, cloud, region)

        with patch('assess_model_defaults.get_model_defaults',
                   side_effect=lambda client, key, cloud=None, region=None:
                       client.get_model_defaults(key),
                   autospec=True) as get_mock:
            with patch('assess_model_defaults.set_model_defaults',
                       side_effect=lambda client, key, value,
                       cloud=None, region=None:
                           client.set_model_defaults(key, value),
                       autospec=True) as set_mock:
                with patch('assess_model_defaults.unset_model_defaults',
                           side_effect=lambda client, key,
                           cloud=None, region=None:
                               client.unset_model_defaults(key),
                           autospec=True) as unset_mock:
                    yield (get_mock, set_mock, unset_mock)

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
                           side_effect=lambda client, key:
                               client.unset_model_defaults(key),
                           autospec=True) as unset_mock:
                    assess_model_defaults_controller(
                        client, 'some-key', 'yellow')
        self.assertEqual(3, get_mock.call_count)
        set_mock.assert_called_once_with(client, 'some-key', 'yellow')
        unset_mock.assert_called_once_with(client, 'some-key')

    def test_model_defaults_region(self):
        client = FakeJujuModelDefaults({'some-key': 'black'})
        with patch('assess_model_defaults.get_model_defaults',
                   side_effect=lambda client, key, cloud=None, region=None:
                       client.get_model_defaults(key),
                   autospec=True) as get_mock:
            with patch('assess_model_defaults.set_model_defaults',
                       side_effect=lambda client, key, value,
                       cloud=None, region=None:
                           client.set_model_defaults(key, value),
                       autospec=True) as set_mock:
                with patch('assess_model_defaults.unset_model_defaults',
                           side_effect=lambda client, key,
                           cloud=None, region=None:
                               client.unset_model_defaults(key),
                           autospec=True) as unset_mock:
                    assess_model_defaults_controller(
                        client, 'some-key', 'yellow')
        self.assertEqual(3, get_mock.call_count)
        set_mock.assert_called_once_with(client, 'some-key', 'yellow')
        unset_mock.assert_called_once_with(client, 'some-key')

    def test_model_defaults(self):
        # Using fake_client means that deploy and get_status have plausible
        # results.  Wrapping it in a Mock causes every call to be recorded, and
        # allows assertions to be made about calls.  Mocks and the fake client
        # can also be used separately.
        fake_client = FakeJujuModelDefaults()
        with patch('assess_model_defaults.assess_model_defaults_controller',
                   autospec=True) as assess_controller_mock:
            with patch('assess_model_defaults.assess_model_defaults_region',
                       autospec=True) as assess_region_mock:
                assess_model_defaults(fake_client)
        assess_controller_mock.assert_called_once_with(
            fake_client, 'automatically-retry-hooks', False)
        assess_region_mock.assert_called_once_with(
            fake_client, 'default-series', 'trusty', 'localhost', 'localhost')
