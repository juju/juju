I created a charm [storage-refresher](https://charmhub.io/storage-refresher) to use in the bash tests. Each revision’s changes are relative to the baseline. Here are the details of each revision.

- Revision 1: baseline with awesome-fs single instance storage with a minimum size of 3G
- Revision 2: increase awesome-fs minimum size to 5G
- Revision 3: decrease awesome-fs minimum size to 1G
- Revision 4: adds a new storage filesystem epic-fs
- Revision 5: removes existing storage awesome-fs
- Revision 6: changes existing storage awesome-fs to become multiple with min count 2 and max count 5
- Revision 7: changes existing storage awesome-fs to become multiple with min count 1 and max count 5
- Revision 8: changes existing storage awesome-fs to be a block
- Revision 9: same as Revision 1 (no changes)
