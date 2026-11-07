// Command webimage prints, one per line, the docker build flags of lodev's base web image
// (label and build args), so the release workflow builds the image lodev would build:
//
//	args=(); while IFS= read -r a; do args+=("$a"); done < <(go run ./tools/webimage)
//	docker buildx build "${args[@]}" ... assets/webserver
package main

import (
	"fmt"
	"os"

	"lodev/assets"
	"lodev/internal/config"
	"lodev/internal/docker"
)

func flags() ([]string, error) {
	h, err := assets.Hash("webserver")
	if err != nil {
		return nil, err
	}
	node := config.DefaultNodeVersion
	return docker.PHPBuildFlags(config.BundledPHP, node, docker.ImageHash(h, node)), nil
}

func main() {
	f, err := flags()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, a := range f {
		fmt.Println(a)
	}
}
