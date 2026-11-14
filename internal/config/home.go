package config

import (
	"os"
	"path/filepath"
)

// Home is lodev's state directory: $LODEV_HOME, else ~/.lodev.
func Home() string {
	if h := os.Getenv("LODEV_HOME"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".lodev")
}

// TraefikDir holds the route files Traefik watches.
func TraefikDir() string { return filepath.Join(Home(), "traefik") }
