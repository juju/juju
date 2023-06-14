// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

// EntitiesWatcher delivers strings that can be parsed as tags, but since it
// doesn't actually produce tags today we may as well make it compatible with
// StringsWatcher so we can use it with a StringsHandler. In an ideal world
// we'd have something like `type EntitiesChannel <-chan []names.Tag` instead.
type EntitiesWatcher = StringsWatcher
