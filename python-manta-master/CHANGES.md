# python-manta Changelog

## 2.1.1 (not yet released)

- Fix test suite breakage.


## 2.1.0

- [issue #18] Add MANTA_NO_AUTH support for running python-manta in a Manta
  job or mlogin session. For example, this means that you can now much more
  easily use python-manta in a manta job by using `pip install manta` in
  your init command. E.g. using the node-manta `mjob` command:

        mfind /bob/stor/datafiles -to \
            | mjob create --init 'pip install manta' \
                -s /bob/stor/scripts/my-processor.py \
                -m '/assets/bob/stor/scripts/my-processor.py'

  Now "my-processor.py" can `import manta` to read and write to Manta inside the
  job.

- [issue #16] A start at some encoding handling fixes in `mantash` for paths
  with non-ascii characters.

- [issue #15] Fix tab completion in `mantash` to escape shell special
  characters.

- 'mantash sign MANTA-PATH' wrapper around msign from the node-manta tools.

- 'mantash json' simple command to JSON-pretty-print a smallish JSON file.

- 'mantash head' and 'mantash tail' first cuts.

- Fix mantash bash completion to append '/' for directories (both for manta dirs
  and local dirs as appropriate for the command).

- Add 'mantash login ...' command. This calls out to the awesome
  [`mlogin`](https://apidocs.joyent.com/manta/mlogin.html) tool from the [Manta
  Node.js SDK (aka node-manta)](https://github.com/joyent/node-manta), hence
  this requires that you have node-manta setup and on your PATH.

- Sort 'mantash find ...' output.

- Add support to 'mantash' to remember user areas visited for future
  tab completion. For example, if you are "bob" and you've visited
  "/sue/public/..." then "cd /s<TAB>" will complete to "cd /sue/" for you.


## 2.0.1

- Packaging tweaks, improved README, clean rev for pypi.


## 2.0.0

- Rework the packaging for `pip install manta` et al to work.
  Drop siloed 3rd party deps. This work inspired and mostly
  copied from [deserat's](https://github.com/deserat) pull #10.


## 1.5.1

- [issue #12] Fixes for `MantaClient.put_object`.


## 1.5.0

- [issue #9, MANTA-1593] Proper URI encoding (RFC3986).


## 1.4.0

- [MANTA-1478] Properly handle paging through ListDirectory results. Before
  this, `<MantaClient>.ls(...)` and `mantash ls` would not return all
  entries in a directory with greater than ~256 entries.

- Add 'mantash open MANTA-PATH' to open a file in Manta in your browser.

- Rename 'mantash gzcat ...' to 'mantash zcat ...' as it should have been.

- [issue #8] A start at Python 3 support. This is **incomplete**. Paramiko
  doesn't support Python 3 so we are stuck. Update to httplib2 0.8.


## 1.3.1

- Fix parsing of DSA pubkeys when generating a pubkey fingerprint
  for `MANTA_KEY_ID=<path to ssh private key>` usage.


## 1.3.0

- [MANTA-1299] Backward incompatible change. Update to the new http-signature
  signing scheme. <http://tools.ietf.org/html/draft-cavage-http-signatures-00>

- Backward incompatible change. Update `manta.MantaClient` "user" field to
  "account" and `mantash` top-level options to mimic (as much as possible)
  the [node-manta](https://github.com/joyent/node-manta)
  CLI tools (e.g. `mls -h`).


## 1.2.2

- [pull #5] Add `-h` support for human readable sizes to `mantash ls`.
  (by Bill Pijewski).

- `mantash gzcat PATHS...`

- [issue #4] Fix possible infinite loop on `ls /:user/jobs`.


## 1.2.1

- Fix `job` handling of PATHS: relative paths within a dir, e.g.:

        $ job a-dir/a-file.txt ^ cat

  resulted bogus manta object paths (keys) being added to the job.

- Mantash `find` takes multiple DIRS, e.g. `find foo bar -type o`.

- Mantash `ls` fixes.

- Get tab-completion of paths to handle '~' properly.

- `cd -` support in mantash shell.

- Fix `mantash find OBJECT-PATH` (as opposed to find on a *DIR*).


## 1.2.0

- [issue #1] Drop the binary Crypto build for sunos-py27 (32-bit) and give
  instructions to install PyCrypto with pkgin et al on SmartOS.

- Improve tab-completion in the mantash interactive shell. It should now
  properly do manta path (e.g. `ls`), manta dir (e.g. `cd`), local path (e.g.
  `lls`), or local dir completion (e.g. `lcd`) depending on the command being
  used.

  Note that `get` does manta path completion and `put` does local path
  completion even though that is the inappropriate context for the *last* arg
  to those commands. The problem is that both `get` and `put` can accept
  multiple source paths if the last arg is a target *directory*. There is no
  good way to recognize when the last arg is being tab-completed.

- 'DST-PATH' in `mantash get SRC-PATH DST-PATH` now defaults to the cwd.


## 1.1.0

- Add `manta.CLISigner` for signing appropriate for an CLI tool. It will
  use an ssh-agent if available and fallback to using a given SSH key
  (looking at key files in "~/.ssh/" as appropriate). Switch `mantash`
  to use this for signing.

- Fix a bug where the '-type' argument to 'mantash find' always returned empty
  results.

- Add 'mantash -C DIRECTORY' to start in the given directory. E.g.:

        mantash -C /trent/public find .

- Add support for MANTASH_PS1 envvar for a fancy prompt. A subset of the
  Bash codes are supported (see "PROMPTING" section in `man bash and
  the `_update_prompt` method in "bin/mantash"). The default is `[\m\w]$ `,
  but you might like:

        export MANTASH_PS1='\e[90m[\u@\h \e[34m\w\e[90m]$\e[0m '

- `mantash --version`


## 1.0.0

First release.
