from unittest import TestCase

from update_lxc_cache import (
    LxcCache,
    main,
    parse_args,
    )


class UpdateLxcCacheTestCase(TestCase):

    def test_parse_args(self):
        args = parse_args(['user@host', 'trusty', 'ppc64el', './workspace'])
        self.assertEqual('./workspace', args.workspace)
        self.assertEqual('trusty', args.release)
        self.assertEqual('ppc64el', args.arch)
        self.assertEqual('ubuntu', args.dist)
        self.assertEqual('default', args.variant)
        self.assertFalse(args.verbose)
        self.assertFalse(args.dry_run)
