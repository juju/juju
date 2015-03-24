from mock import patch
import json
from unittest import TestCase

from utils import (
    get_random_hex_string,
    temp_dir,
)
from validate_streams import (
    check_devel_not_stable,
    check_expected_changes,
    check_expected_unchanged,
    check_agents_content,
    compare_agents,
    find_agents,
    main,
    parse_args,
    reconcile_aliases,
)


def make_agent_data(version='1.20.7', release='trusty',
                    arch='amd64', sha256=None, path=None):
    name = '{}-{}-{}'.format(version, release, arch)
    if not path:
        path = "releases/juju-{}.tgz".format(name)
    if not sha256:
        sha256 = get_random_hex_string()
    tool = {
        "release": "{}".format(release),
        "version": "{}".format(version),
        "arch": "{}".format(arch),
        "size": 8234578,
        "path": path,
        "ftype": "tar.gz",
        "sha256": sha256
    }
    return name, tool


def make_agents_data(release='trusty', arch='amd64',
                     versions=['1.20.7'], sha256='valid_sum'):
    return dict(make_agent_data(v, release, arch, sha256) for v in versions)


def make_product_data(release='trusty', arch='amd64', versions=['1.20.7']):
    name = 'com.ubuntu.juju:{}:{}'.format(release, arch)
    items = make_agents_data(release, arch, versions)
    product = {
        "version": "{}".format(versions[0]),
        "arch": "{}".format(arch),
        "versions": {
            "20140919": {
                "items": items
            }
        }
    }
    return name, product


def make_products_data(versions):
    products = {}
    for release, arch in (('trusty', 'amd64'), ('trusty', 'i386')):
        name, product = make_product_data(release, arch, versions)
        products[name] = product
    stream = {
        "products": products,
        "updated": "Fri, 19 Sep 2014 13:25:28 -0400",
        "format": "products:1.0",
        "content_id": "com.ubuntu.juju:released:agents"
    }
    return stream


class ValidateStreams(TestCase):

    def test_parge_args(self):
        # The purpose, release, old json and new json are required.
        required = ['--added', '1.20.9', 'proposed', 'old/json', 'new/json']
        args = parse_args(required)
        self.assertEqual('proposed', args.purpose)
        self.assertEqual('old/json', args.old_json)
        self.assertEqual('new/json', args.new_json)
        self.assertEqual('1.20.9', args.added)
        self.assertIsNone(None, args.removed)
        self.assertIsNone(None, args.ignored)
        # A bad release version can be removed.
        args = parse_args(['--removed', 'bad'] + required)
        self.assertEqual('bad', args.removed)
        # A version can be ignored.
        args = parse_args(['--ignored', '1.18'] + required)
        self.assertEqual('1.18', args.ignored)

    def test_find_agents(self):
        products = make_products_data(['1.20.7', '1.20.8'])
        with temp_dir() as wd:
            file_path = '{}/json'.format(wd)
            with open(file_path, 'w') as f:
                f.write(json.dumps(products))
            agents = find_agents(file_path)
        expected = [
            '1.20.7-trusty-i386', '1.20.7-trusty-amd64',
            '1.20.8-trusty-amd64', '1.20.8-trusty-i386']
        self.assertEqual(expected, agents.keys())

    def test_check_devel_not_stable(self):
        # devel agents cannot ever got to proposed and release.
        old_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_agents = make_agents_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.21-alpha1'])
        # Devel versions can go to testing
        message = check_devel_not_stable(old_agents, new_agents, 'testing')
        self.assertEqual([], message)
        # Devel versions can go to devel
        message = check_devel_not_stable(old_agents, new_agents, 'devel')
        self.assertEqual([], message)
        # Devel versions cannot be proposed.
        message = check_devel_not_stable(old_agents, new_agents, 'proposed')
        self.assertEqual(
            ["Devel versions in proposed stream: "
             "['1.21-alpha1-trusty-amd64']"],
            message)
        # Devel versions cannot be release.
        message = check_devel_not_stable(old_agents, new_agents, 'release')
        self.assertEqual(
            ["Devel versions in release stream: ['1.21-alpha1-trusty-amd64']"],
            message)

    def test_check_agents_content(self):
        old_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_agents = make_agents_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_agents['1.20.7-trusty-amd64']['sha256'] = 'bad_sum'
        message = check_agents_content(old_agents, new_agents)
        self.assertEqual(
            (['Tool 1.20.7-trusty-amd64 sha256 changed from '
              'valid_sum to bad_sum']),
            message)

    def test_compare_agents_identical(self):
        old_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        message = compare_agents(
            old_agents, new_agents, 'proposed', added=None, removed=None)
        self.assertIs(None, message)

    def test_check_expected_changes_with_no_changes(self):
        new_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        errors = check_expected_changes(new_agents, added=None, removed=None)
        self.assertEqual([], errors)

    def test_check_expected_changes_with_changes(self):
        new_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.9'])
        errors = check_expected_changes(
            new_agents, added='1.20.9', removed='1.20.8')
        self.assertEqual([], errors)

    def test_check_expected_changes_with_found_errors(self):
        new_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        errors = check_expected_changes(
            new_agents, added=None, removed='1.20.8')
        self.assertEqual(
            ["1.20.8 agents were not removed: ['1.20.8-trusty-amd64']"],
            errors)

    def test_check_expected_changes_with_missing_errors(self):
        new_agents = make_agents_data('trusty', 'amd64', ['1.20.7'])
        errors = check_expected_changes(
            new_agents, added='1.20.8', removed=None)
        self.assertEqual(['1.20.8 agents were not added'], errors)

    def test_check_expected_unchanged_without_changes(self):
        old_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        errors = check_expected_unchanged(
            old_agents, new_agents, added=None, removed=None)
        self.assertEqual([], errors)

    def test_check_expected_unchanged_without_changes_and_added_removed(self):
        old_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.9'])
        errors = check_expected_unchanged(
            old_agents, new_agents, added='1.20.9', removed='1.20.8')
        self.assertEqual([], errors)

    def test_check_expected_unchanged_with_missing_errors(self):
        old_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.9'])
        errors = check_expected_unchanged(
            old_agents, new_agents, added='1.20.9', removed=None)
        self.assertEqual(
            ["These agents are missing: ['1.20.8-trusty-amd64']"],
            errors)

    def test_check_expected_unchanged_with_found_errors(self):
        old_agents = make_agents_data('trusty', 'amd64', ['1.20.7'])
        new_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.9'])
        errors = check_expected_unchanged(
            old_agents, new_agents, added=None, removed=None)
        self.assertEqual(
            ["These unknown agents were found: ['1.20.9-trusty-amd64']"],
            errors)

    def test_check_expected_unchanged_with_ignored(self):
        old_agents = make_agents_data('trusty', 'amd64', ['1.21-b1'])
        new_agents = make_agents_data('trusty', 'amd64', ['1.21-b1', '1.20.1'])
        errors = check_expected_unchanged(
            old_agents, new_agents, added=None, removed=None, ignored='1.20.')
        self.assertEqual([], errors)

    def test_check_expected_unchanged_calls_reconcile_aliases(self):
        old_agents = make_agents_data('trusty', 'amd64', ['1.20.7'])
        new_agents = make_agents_data('trusty', 'amd64', ['1.18.1', '1.20.7'])
        with patch('validate_streams.reconcile_aliases') as mock:
            check_expected_unchanged(old_agents, new_agents)
        args, kwargs = mock.call_args
        self.assertEqual((set(['1.18.1-trusty-amd64']), new_agents), args)

    def test_reconcile_aliases_without_found_errors(self):
        new_agents = make_agents_data('trusty', 'ppc64el', ['1.20.7'])
        found_errors = set()
        reconcile_aliases(found_errors, new_agents)
        self.assertEqual(set(), found_errors)

    def test_reconcile_aliases_with_unaliased_found_errors_remain(self):
        # Unaliased found_errors are left in the set.
        new_agents = make_agents_data('trusty', 'ppc64el', ['1.20.7'])
        extra_agent_name, extra_agent = make_agent_data(
            '1.18.1', 'trusty', 'ppc64', sha256=None)
        new_agents[extra_agent_name] = extra_agent
        found_errors = set([extra_agent_name])
        reconcile_aliases(found_errors, new_agents)
        self.assertEqual(set(['1.18.1-trusty-ppc64']), found_errors)

    def test_reconcile_aliases_with_ppc64_aliases_are_removed(self):
        # Aliases in found_errors are are removed from set.
        new_agents = make_agents_data(
            'trusty', 'ppc64el', ['1.18.1', '1.20.7'], sha256=None)
        ppc64el_agent = new_agents['1.18.1-trusty-ppc64el']
        ppc64_agent_name, ppc64_agent = make_agent_data(
            '1.18.1', 'trusty', 'ppc64',
            ppc64el_agent['sha256'], ppc64el_agent['path'])
        new_agents[ppc64_agent_name] = ppc64_agent
        found_errors = set(['1.18.1-trusty-ppc64'])
        reconcile_aliases(found_errors, new_agents)
        self.assertEqual(set(), found_errors)

    def test_compare_agents_changed_tool(self):
        old_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_agents = make_agents_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.20.9'])
        new_agents['1.20.7-trusty-amd64']['sha256'] = 'bad_sum'
        errors = compare_agents(
            old_agents, new_agents, 'proposed', '1.20.9', removed=None)
        self.assertEqual(
            ['Tool 1.20.7-trusty-amd64 sha256 changed from '
             'valid_sum to bad_sum'],
            errors)

    def test_compare_agents_called_check_expected_agents_called(self):
        old_agents = make_agents_data('trusty', 'amd64', ['1.20.7'])
        new_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        with patch("validate_streams.check_expected_changes",
                   return_value=(['foo'])) as cec_mock:
            with patch("validate_streams.check_expected_unchanged",
                       return_value=(['bar'])) as ceu_mock:
                errors = compare_agents(
                    old_agents, new_agents, 'proposed', '1.20.9',
                    removed=None, ignored=None)
                cec_mock.assert_called_with(new_agents, '1.20.9', None)
                ceu_mock.assert_called_with(
                    old_agents, new_agents, '1.20.9', None, None)
        self.assertEqual(['foo', 'bar'], errors)

    def test_compare_agents_added_devel_version(self):
        # devel agents cannot ever got to proposed and release.
        old_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_agents = make_agents_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.21-alpha1'])
        # Devel versions can go to devel
        message = compare_agents(
            old_agents, new_agents, 'devel', '1.21-alpha1', removed=None)
        self.assertIs(None, message)
        # Devel versions cannot be proposed.
        errors = compare_agents(
            old_agents, new_agents, 'proposed', '1.21-alpha1', removed=None)
        expected = (
            "Devel versions in proposed stream: ['1.21-alpha1-trusty-amd64']")
        self.assertEqual([expected], errors)
        # Devel versions cannot be release.
        errors = compare_agents(
            old_agents, new_agents, 'release', '1.21-alpha1', removed=None)
        expected = (
            "Devel versions in release stream: ['1.21-alpha1-trusty-amd64']")
        self.assertEqual([expected], errors)

    def test_main_without_errrors(self):
        def fake_fa(name):
            return name
        with patch('validate_streams.find_agents',
                   autospec=True, side_effect=fake_fa) as fa_mock:
            with patch('validate_streams.compare_agents',
                       autospec=True, return_value=None) as ca_mock:
                returncode = main(
                    ['script', '--added', '1.2.3', 'released', 'old', 'new'])
        self.assertEqual(0, returncode)
        fa_mock.assert_any_call('old')
        fa_mock.assert_any_call('new')
        ca_mock.assert_called_with(
            'old', 'new', 'released', '1.2.3', None, None)

    def test_main_with_errrors(self):
        with patch('validate_streams.find_agents', autospec=True):
            with patch('validate_streams.compare_agents',
                       autospec=True, return_value=['error']):
                returncode = main(
                    ['script', '--added', '1.2.3', 'released', 'old', 'new'])
        self.assertEqual(1, returncode)
