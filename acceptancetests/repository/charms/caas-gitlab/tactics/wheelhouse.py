import logging

from charmtools.build.tactics import ExactMatch, Tactic
from charmtools import utils

log = logging.getLogger(__name__)


class WheelhouseTactic(ExactMatch, Tactic):
    """
    This tactic matches the file `wheelhouse.txt` and is called to process
    that file.
    """
    kind = "dynamic"
    FILENAME = 'wheelhouse.txt'

    def __init__(self, *args, **kwargs):
        super(WheelhouseTactic, self).__init__(*args, **kwargs)
        self.tracked = []
        self.previous = []

    @property
    def dest(self):
        return self.target.directory / 'lib'

    def __str__(self):
        return "Building wheelhouse in {}".format(self.dest)

    def combine(self, existing):
        """
        Maintain a list of the tactic instances for each previous layer's
        `wheelhouse.txt` file, in order.
        """
        self.previous = existing.previous + [existing]
        return self

    def __call__(self):
        """
        Process the wheelhouse.txt file.

        This gets called once on the tactic instance for the highest level
        layer which contains a `wheelhouse.txt` file (e.g., the charm layer).
        It then iterates the instances representing all of the previous
        layers, in order, so that higher layers take precedence over base
        layers.

        For each layer, its `wheelhouse.txt` is pip installed into a temp
        directory and all files that end up in that temp dir are recorded
        for tracking ownership in the build reporting, and then the temp
        dir's contents are copied into the charm's `lib/` dir.

        The installation happens separately for each layer's `wheelhouse.txt`
        (rather than just combining them into a single `wheelhouse.txt`)
        because files created by a given layer's `wheelhouse.txt` should be
        signed by that layer in the report to properly detect changes, etc.
        """
        # recursively process previous layers, depth-first
        for tactic in self.previous:
            tactic()
        # process this layer
        self.dest.mkdir_p()
        with utils.tempdir(chdir=False) as temp_dir:
            # install into a temp dir first to track new and updated files
            utils.Process(
                ('pip3', 'install', '-t', str(temp_dir), '-r', self.entity)
            ).exit_on_error()()
            # clear out cached compiled files (there shouldn't really be a
            # reason to include these in the charms; they'll just be
            # recompiled on first run)
            for path in temp_dir.walk():
                if path.isdir() and (
                    path.basename() == '__pycache__' or
                    path.basename().endswith('.dist-info')
                ):
                    path.rmtree()
                elif path.isfile() and path.basename().endswith('.pyc'):
                    path.remove()
            # track all the files that were created by this layer
            self.tracked.extend([self.dest / file.relpath(temp_dir)
                                 for file in temp_dir.walkfiles()])
            # copy everything over from temp_dir to charm's /lib
            temp_dir.merge_tree(self.dest)

    def sign(self):
        """return sign in the form {relpath: (origin layer, SHA256)}

        This is how the report of changed files is created.
        """
        sigs = {}
        # recursively have all previous layers sign their files, depth-first
        for tactic in self.previous:
            sigs.update(tactic.sign())
        # sign ownership of all files this layer created or updated
        for d in self.tracked:
            relpath = d.relpath(self.target.directory)
            sigs[relpath] = (self.layer.url, "dynamic", utils.sign(d))
        return sigs
