from mock import (
    Mock,
    patch,
)
import os
from unittest import TestCase

from deptree import (
    Dependency,
    DependencyFile,
    get_args,
    main,
)
from utils import temp_dir


def DependencyTestCase(TestCase):

    def test_from_option(self):
        dep = Dependency.from_option('foo/bar:git:123abc0')
        self.assertEqual('foo/bar', dep.package)
        self.assertEqual('git', dep.vcs)
        self.assertEqual('123abc0', dep.revid)
        self.assertIs(None, dep.revno)

    def test_from_line(self):
        dep = Dependency.from_line('foo/bar\tgit\t123abc0')
        self.assertEqual('foo/bar', dep.package)
        self.assertEqual('git', dep.vcs)
        self.assertEqual('123abc0', dep.revid)
        self.assertIs(None, dep.revno)

    def test_from_string(self):
        dep = Dependency.from_string('foo/bar\tgit\t123abc0\t', '\t')
        self.assertEqual('foo/bar', dep.package)
        self.assertEqual('git', dep.vcs)
        self.assertEqual('123abc0', dep.revid)
        self.assertIs(None, dep.revno)
        dep = Dependency.from_string('foo/bar:git:123abc0', ':')
        self.assertEqual('foo/bar', dep.package)
        self.assertEqual('git', dep.vcs)
        self.assertEqual('123abc0', dep.revid)
        self.assertIs(None, dep.revno)

    def test_init(self):
        dep = Dependency('github/foo', 'git', 'rev123')
        self.assertEqual('github/foo', dep.package)
        self.assertEqual('git', dep.vcs)
        self.assertEqual('rev123', dep.revid)
        self.assertIs(None, dep.revno)
        # Revno is None when an empty string is passed
        dep = Dependency('github/foo', 'git', 'rev123', revno='')
        self.assertIs(None, dep.revno)

    def test_eq(self):
        a_dep = Dependency('github/foo', 'git', 'rev123')
        b_dep = Dependency('github/foo', 'git', 'rev123')
        self.assertTrue(a_dep == b_dep)
        c_dep = Dependency('github/bar', 'git', 'rev456')
        self.assertFalse(a_dep == c_dep)

    def test_ne(self):
        a_dep = Dependency('github/foo', 'git', 'rev123')
        b_dep = Dependency('github/foo', 'git', 'rev123')
        self.assertFalse(a_dep != b_dep)
        c_dep = Dependency('github/bar', 'git', 'rev456')
        self.assertTrue(a_dep != c_dep)

    def test_repr(self):
        self.assertEqual(
            '<Dependency github/foo git rev123>',
            repr(Dependency('github/foo', 'git', 'rev123')))
        self.assertEqual(
            '<Dependency github/foo git rev123 3>',
            repr(Dependency('github/foo', 'git', 'rev123', '3')))

    def test_str(self):
        self.assertEqual(
            'github/foo\tgit\trev123\t',
            str(Dependency('github/foo', 'git', 'rev123')))
        self.assertEqual(
            'github/foo\tgit\trev123\t3',
            str(Dependency('github/foo', 'git', 'rev123', '3')))

    def test_to_line(self):
        self.assertEqual(
            'github/foo\tgit\trev123\t\n',
            Dependency('github/foo', 'git', 'rev123').to_line())
        self.assertEqual(
            'github/foo\tgit\trev123\t3\n',
            Dependency('github/foo', 'git', 'rev123', '3').to_line())


class DependencyFileTestCase(TestCase):

    def test_init(self):
        with patch('deptree.DependencyFile.consolidate_deps', autospec=True,
                   return_value=({}, [])) as cd_mock:
            dep_file = DependencyFile(['foo.tsv', 'bar.tsv'])
        cd_mock.assert_called_once_with(dep_file)
        self.assertEqual(['foo.tsv', 'bar.tsv'], dep_file.dep_files)
        self.assertIsNone(dep_file.tmp_tsv)
        self.assertEqual({}, dep_file.deps)
        self.assertEqual([], dep_file.conflicts)

    def test_consolidate_deps(self):
        expected_deps = {
            'github/foo': Dependency('github/foo', 'git', 'rev123', None),
            'github/bar': Dependency('github/bar', 'git', 'rev456', None),
            'github/baz': Dependency('github/baz', 'git', 'rev789', None),
            'github/qux': Dependency('github/qux', 'git', 'revdef', None),
            'lp/qoh': Dependency('lp/qoh', 'bzr', 'rev789', '3')
        }
        conflict_dep = Dependency('github/baz', 'git', 'revabc', None)
        with temp_dir() as base_dir:
            a_dep_file = '%s/a.tsv' % base_dir
            with open(a_dep_file, 'w') as f:
                f.write(expected_deps['github/foo'].to_line())
                f.write(expected_deps['github/bar'].to_line())
                f.write(expected_deps['github/baz'].to_line())
            b_dep_file = '%s/b.tsv' % base_dir
            with open(b_dep_file, 'w') as f:
                f.write(expected_deps['github/foo'].to_line())
                f.write(conflict_dep.to_line())
                f.write(expected_deps['github/qux'].to_line())
                f.write(expected_deps['lp/qoh'].to_line())
            dep_file = DependencyFile([a_dep_file, b_dep_file])
            deps, conflicts = dep_file.consolidate_deps()
        self.assertEqual([(b_dep_file, conflict_dep)], conflicts)
        self.assertEqual(expected_deps, deps)

    def test_include_deps(self):
        deps = {
            'github/foo': Dependency('github/foo', 'git', 'rev123', None),
            'github/bar': Dependency('github/bar', 'git', 'rev456', None),
        }
        dep_file = DependencyFile([])
        dep_file.deps = deps
        include = [
            Dependency('github/bar', 'git', 'redefined', None),
            Dependency('github/baz', 'git', 'added', None),
        ]
        redefined, added = dep_file.include_deps(include)
        self.assertEqual([include[0]], redefined)
        self.assertEqual([include[1]], added)
        self.assertEqual(include[0], dep_file.deps['github/bar'])
        self.assertEqual(include[1], dep_file.deps['github/baz'])

    def test_write_tmp_tsv(self):
        a_dep = Dependency('github/foo', 'git', 'rev123', None)
        b_dep = Dependency('github/bar', 'git', 'rev456', None)
        consolidated_deps = {
            a_dep.package: a_dep,
            b_dep.package: b_dep,
        }
        dep_file = DependencyFile([])
        dep_file.deps = consolidated_deps
        tmp_tsv = dep_file.write_tmp_tsv()
        self.assertEqual(tmp_tsv, dep_file.tmp_tsv)
        self.assertTrue(os.path.isfile(tmp_tsv))
        self.addCleanup(os.unlink, tmp_tsv)
        with open(tmp_tsv) as f:
            content = f.read()
        expected = ''.join([b_dep.to_line(), a_dep.to_line()])
        self.assertEqual(expected, content)

    def test_delete_tmp_tsv(self):
        dep_file = DependencyFile([])
        self.assertFalse(dep_file.delete_tmp_tsv())
        with temp_dir() as base_dir:
            dep_path = '%s/a.tsv' % base_dir
            with open(dep_path, 'w') as f:
                f.write('foo')
            dep_file.tmp_tsv = dep_path
            self.assertTrue(dep_file.delete_tmp_tsv())
            self.assertFalse(os.path.isfile(dep_path))
        self.assertIsNone(dep_file.tmp_tsv)

    def test_pin_deps(self):
        dep_file = DependencyFile([])
        with patch('subprocess.check_output') as co_mock:
            def write_tmp_tsv():
                dep_file.tmp_tsv = '/tmp/baz.tsv'
            with patch.object(dep_file, 'write_tmp_tsv',
                              side_effect=write_tmp_tsv,
                              autospec=True) as wt_mock:
                with patch.object(dep_file, 'delete_tmp_tsv',
                                  autospec=True) as dt_mock:
                    dep_file.pin_deps()
        wt_mock.assert_called_once_with()
        co_mock.assert_called_once_with(['godeps', '-u', '/tmp/baz.tsv'])
        dt_mock.assert_called_once_with()


class DepTreeTestCase(TestCase):

    def test_get_args(self):
        args = get_args(
            ['-d', '-v', '-i', 'foo:git:rev', 'baz', 'qux'])
        self.assertTrue(args.verbose)
        self.assertTrue(args.dry_run)
        self.assertEqual([Dependency('foo', 'git', 'rev')], args.include)
        self.assertEqual(['baz', 'qux'], args.dep_files)

    def test_main(self):
        df_mock = Mock(spec=DependencyFile)
        df_mock.consolidate_deps.return_value = ({}, [])
        df_mock.include_deps.return_value = ([], [])
        df_mock.pin_deps.return_value = None
        with patch('deptree.DependencyFile',
                   autospec=True, return_value=df_mock) as init_mock:
            main(['bar', 'baz'])
        init_mock.assert_called_once_with(['bar', 'baz'])
        df_mock.include_deps.assert_called_once_with([])
        df_mock.pin_deps.assert_called_once_with()
