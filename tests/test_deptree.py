from mock import patch
import os
from unittest import TestCase

from deptree import (
    consolidate_deps,
    Dependency,
    get_args,
    main,
)
from utils import temp_dir


class DepTreeTestCase(TestCase):

    def test_get_args(self):
        args = get_args(['-d', '-v', '-i', 'foo', './bar', 'baz', 'qux'])
        self.assertTrue(args.verbose)
        self.assertTrue(args.dry_run)
        self.assertEqual(['foo'], args.ignore)
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
                f.write(conflict_dep.to_line())
                f.write(expected_deps['github/qux'].to_line())
                f.write(expected_deps['lp/qoh'].to_line())
            deps, conflicts = consolidate_deps([a_dep_file, b_dep_file])
        self.assertEqual(conflict_dep, conflicts[0][1])
        self.assertEqual([(b_dep_file, conflict_dep)], conflicts)
        self.assertEqual(expected_deps, deps)
