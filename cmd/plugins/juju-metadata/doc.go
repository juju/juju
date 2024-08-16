// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
//
// Package main in juju-metadata provides cli commands for managing
// image and agent binary metadata.
//
// Metadata plugin tools are separated into 2 categories:
// 1. Tools that create simplestreams content related to agents and images for juju
//    to use in the future. The location of the simple stream must be provided to
//    juju via command flags or model config.
//        * generate-agent-binaries
//        * generate-image
//        * sign
//        * validate-agent-binaries
//        * validate-images
// 2. Tools that upload image identifiers to a running controller as simplestreams
//    content for juju to use.
//        * add-image
//        * delete-image
//        * list-image
//
// <metadata-source-directory>
// |-tools
//     |-streams
//         |-v1
//            |-index.(s)json
//            |-product-foo-stream-agents.(s)json
//            |-product-bar-stream-agents.(s)json
//     |-released
//          |-tools-abc.tar.gz
//          |-tools-def.tar.gz
//          |-tools-xyz.tar.gz
//     |-proposed
//     |-testing
//    |-devel
// |-images
//     |-streams
//         |-v1
//            |-index.json
//            |-product-bar-imagemetadata.(s)json

package main
