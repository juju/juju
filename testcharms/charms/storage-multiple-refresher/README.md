I created a charm [storage-multiple-refresher](https://charmhub.io/storage-multiple-refresher/) to use in the bash tests. Each revision’s changes are relative to the baseline. Here are the details of each revision.

- Revision 6: baseline with awesome-block multiple instance storage. Min count: 2, max count: 5. Min size 3G. Block type.
- Revision 7: relative to revision 6. Change awesome-block min count to 1, max count to 7, and min size to 2G.
- Revision 8: decrease awesome-fs min count to 1, increase awesome-fs max count to 8, and decrease min size to 1G.
