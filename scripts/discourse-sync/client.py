import pydiscourse

class Client(pydiscourse.DiscourseClient):
    """Extends pydiscourse.DiscourseClient to support additional API methods."""

    def lock_post(self, post_id: int, locked: bool, **kwargs):
        """Lock a post from being edited

        https://docs.discourse.org/#tag/Posts/operation/lockPost

        Args:
            post_id: the ID of the post to lock
            locked: True to lock, False to unlock
        """
        if locked:
            kwargs["locked"] = "true"
        else:
            kwargs["locked"] = "false"
        return self._put(f"posts/{post_id}/locked.json", **kwargs)
