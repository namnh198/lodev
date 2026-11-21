// Package dotlodev_assets holds the files lodev scaffolds into a project's .lodev, at the paths
// they take there. Only one webserver folder (nginx or apache) is copied: the project's.
package dotlodev_assets

import "embed"

//go:embed all:commands all:web-images all:php all:hooks all:homeadditional all:nginx all:apache example.compose.yml.example
var FS embed.FS
