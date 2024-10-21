## Remove old secret revision

If a revision has been orphaned by a new revision and we didn't remove it, this
can lead to leak of the resource. There is no collector running in the
background to clean up the orphaned resources. So, it is important to remove the
old revision once a new revision is created.

### Run the ./remove-old-secret-revisions.sh

The script will prompt you to ensure that you want to remove the old revision.
If there is an old revision it will be removed.

```bash
./remove-old-secret-revisions.sh
```