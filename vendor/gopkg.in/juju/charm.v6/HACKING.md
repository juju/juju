# HACKING

See README for information about gopkg.in

## Developing

If you are to develop on a versioned branch, use gopkg.in.

    go get -u -v -t gopkg.in/juju/charm.v2/...

gopkg.in names the local branch master.  To submit a pull request, push to
your github branch using a refspec which reflects the version tag you are using.

    git push git@github.com:jrwren/charm +master:v2
