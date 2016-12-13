"""Tests for assess_model_defaults module."""

from contextlib import contextmanager
from copy import deepcopy
import logging
import StringIO

from mock import (
    call,
    Mock,
    patch,
    )

from assess_model_defaults import (
    assemble_model_default,
    assess_model_defaults,
    assess_model_defaults_controller,
    assess_model_defaults_region,
    get_new_model_config,
    juju_assert_equal,
    main,
    parse_args,
    )
from tests import (
    parse_error,
    TestCase,
    )
from utility import (
    JujuAssertionError,
    )


class FakeJujuModelDefaults:

    def __init__(self, defaults=None, region=None):
        self.model_defaults = {}
        if defaults is not None:
            for (key, value) in defaults.items():
                self.model_defaults[key] = {'default': value}
        self.region = region

    def get_model_defaults(self, model_key, cloud=None, region=None):
        return {model_key: deepcopy(self.model_defaults[model_key])}

    def set_model_defaults(self, model_key, value, cloud=None, region=None):
        if cloud is None and region is None:
            self.model_defaults[model_key]['controller'] = value
        else:
            self.model_defaults[model_key].setdefault('regions', []).append({
                'name': region, 'value': value})

    def unset_model_defaults(self, model_key, cloud=None, region=None):
        key_defaults = self.model_defaults[model_key]
        if cloud is None and region is None:
            del key_defaults['controller']
        else:
            for index, element in enumerate(key_defaults['regions']):
                if element['name'] == region:
                    key_defaults['regions'].pop(index)
                    break
            else:
                raise KeyError(region)
            if not key_defaults['regions']:
                del key_defaults['regions']

    def get_region(self):
        return self.region


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
        mock_assess.assert_called_once_with(client, None)


class TestAssembleModelDefault(TestCase):

    def test_assemble_model_default(self):
        self.assertEqual({'test-mode': {'default': False}},
                         assemble_model_default('test-mode', False))
        self.assertEqual(
            {'test-mode': {'default': False, 'controller': True}},
            assemble_model_default('test-mode', False, True))
        self.assertEqual(
            {'test-mode': {'default': False, 'regions': [
                {'name': 'fakeregion', 'value': True},
                {'name': 'localhost', 'value': True}]}},
            assemble_model_default('test-mode', False, None,
                                   {'localhost': True, 'fakeregion': True}))
        self.assertEqual(
            {'automatically-retry-hooks':
                {'default': True, 'controller': False}},
            assemble_model_default('automatically-retry-hooks', True, False))


class TestAssert(TestCase):

    def test_juju_assert_equal(self):
        juju_assert_equal(0, 0, 'should-pass')
        with self.assertRaises(JujuAssertionError) as context:
            juju_assert_equal(0, 1, 'should-fail')
        self.assertEqual(context.exception.args, ('should-fail', 0, 1))


class TestGetNewModelConfig(TestCase):

    @contextmanager
    def wrap_new_model_test(self):
        (mock_client, mock_env, mock_model) = (Mock(), Mock(), Mock())
        mock_client.env.attach_mock(Mock(return_value=mock_env), 'clone')
        mock_client.attach_mock(Mock(return_value=mock_model), 'add_model')
        mock_model.attach_mock(Mock(return_value='<model_config>'),
                               'get_model_config')
        yield (mock_client, mock_env, mock_model)
        mock_client.add_model.assert_called_once_with(mock_env)
        mock_model.get_model_config.assert_called_once_with()
        mock_model.destroy_model.assert_called_once_with()

    def test_get_new_model_config_defaults(self):
        with self.wrap_new_model_test() as (client, env, model):
            config = get_new_model_config(client)
            client.env.clone.assert_called_once_with('temp-model')
            self.assertFalse(env.set_region.called)
            self.assertEqual('<model_config>', config)

    def test_get_new_model_config_region(self):
        with self.wrap_new_model_test() as (client, env, model):
            config = get_new_model_config(client, 'region')
            client.env.clone.assert_called_once_with('temp-model')
            env.set_region.assert_called_once_with('region')
            self.assertEqual('<model_config>', config)

    def test_get_new_model_config_model_name(self):
        with self.wrap_new_model_test() as (client, env, model):
            config = get_new_model_config(client, model_name='diff-name')
            client.env.clone.assert_called_once_with('diff-name')
            self.assertFalse(env.set_region.called)
            self.assertEqual('<model_config>', config)


class TestAssessModelDefaults(TestCase):

    def test_model_defaults_controller(self):
        client = Mock(wraps=FakeJujuModelDefaults({'some-key': 'black'}))
        with patch('assess_model_defaults.get_new_model_config',
                   return_value={'some-key': {'value': 'yellow'}},
                   autospec=True) as get_config_mock:
            assess_model_defaults_controller(client, 'some-key', 'yellow')
        get_config_mock.assert_called_once_with(client, None)
        self.assertEqual(4, client.get_model_defaults.call_count)
        client.get_model_defaults.assert_has_calls([call('some-key')] * 3)
        client.set_model_defaults.assert_called_once_with(
            'some-key', 'yellow', None, None)
        client.unset_model_defaults.assert_called_once_with(
            'some-key', None, None)

    def test_model_defaults_region(self):
        client = Mock(wraps=FakeJujuModelDefaults({'some-key': 'black'}))
        with patch('assess_model_defaults.get_new_model_config',
                   return_value={'some-key': {'value': 'yellow'}},
                   autospec=True) as get_config_mock:
            assess_model_defaults_region(
                client, 'some-key', 'yellow', 'localhost', 'localhost')
        get_config_mock.assert_called_once_with(client, None)
        self.assertEqual(4, client.get_model_defaults.call_count)
        client.get_model_defaults.assert_has_calls([call('some-key')] * 3)
        client.set_model_defaults.assert_called_once_with(
            'some-key', 'yellow', 'localhost', 'localhost')
        client.unset_model_defaults.assert_called_once_with(
            'some-key', 'localhost', 'localhost')

    def test_model_defaults(self):
        fake_client = FakeJujuModelDefaults(region='localhost')
        with patch('assess_model_defaults.assess_model_defaults_controller',
                   autospec=True) as assess_controller_mock:
            with patch('assess_model_defaults.assess_model_defaults_region',
                       autospec=True) as assess_region_mock:
                assess_model_defaults(fake_client, None)
        assess_controller_mock.assert_called_once_with(
            fake_client, 'automatically-retry-hooks', False)
        assess_region_mock.assert_called_once_with(
            fake_client, 'default-series', 'trusty', region='localhost')
