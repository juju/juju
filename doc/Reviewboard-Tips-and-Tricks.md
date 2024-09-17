### List of current Reviewboard Admins

* alexisb
* cmars
* frobware
* fwereade
* katco
* perrito666
* natefinch
* thumper
* jam
* sinzui
* wallyworld

## Troubleshooting

### My review page is broken / looks very wrong

Sometimes something goes wrong and a review breaks reviewboard for on reason or another. When this happens, the easiest
thing to do is just delete the review and recreate it. However, to perma-delete the review (which is required to get
reviewboard to recreate it), you need someone with admin rights (see above). That person needs to open the review page
and in the menu list labelled close, choose "delete permanently".

Then you can either push a new revision to your branch, or use the rbt command line to get reviewboard to pick up the
branch again.