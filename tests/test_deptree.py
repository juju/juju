from mock import patch
import os
from unittest import TestCase

from deptree import (
    consolidate_deps,
    Dependency,
    get_args,
    main,
    write_tmp_tsv,
)
from utils import temp_dir


def DependencyTestCase(TestCase):

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


class DepTreeTestCase(TestCase):

    def test_get_args(self):
        args = get_args(['-d', '-v', '-i', 'foo', './bar', 'baz', 'qux'])
        self.assertTrue(args.verbose)
        self.assertTrue(args.dry_run)
        self.assertEqual(['foo'], args.include)
        self.assertEqual('./bar', args.srcdir)
        self.assertEqual(['baz', 'qux'], args.dep_files)

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
            deps, conflicts = consolidate_deps([a_dep_file, b_dep_file])
        self.assertEqual([(b_dep_file, conflict_dep)], conflicts)
        self.assertEqual(expected_deps, deps)

    def test_write_tmp_tsv(self):
        a_dep = Dependency('github/foo', 'git', 'rev123', None)
        b_dep = Dependency('github/bar', 'git', 'rev456', None)
        consolidated_deps = {
            a_dep.package: a_dep,
            b_dep.package: b_dep,
        }
        tmp_tsv = write_tmp_tsv(consolidated_deps)
        self.assertTrue(os.path.isfile(tmp_tsv))
        self.addCleanup(os.unlink, tmp_tsv)
        with open(tmp_tsv) as f:
            content = f.read()
        expected = ''.join([b_dep.to_line(), a_dep.to_line()])
        self.assertEqual(expected, content)

    def test_main(self):
        with patch('deptree.consolidate_deps',
                   return_value=[{}, []]) as cd_mock:
            main(['foo', 'bar', 'baz'])
            self.assertEqual((['bar', 'baz'], ), cd_mock.call_args[0])
