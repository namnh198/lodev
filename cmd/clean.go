package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"lodev/internal/config"
	"lodev/internal/registry"
	"lodev/internal/ui"
	"lodev/internal/update"
)

type imageInfo struct{ Ref, ID, Size string }

// withTag gives a bare image name docker's implied :latest, so refs compare as docker images lists them.
func withTag(ref string) string {
	if i := strings.LastIndex(ref, "/"); !strings.Contains(ref[i+1:], ":") {
		return ref + ":latest"
	}
	return ref
}

// cleanKeep is what this lodev still runs on: the current PHP images, OpenSearch image, mkcert and
// the registered projects' images, plus the registry image a stable release pulled. Nightly and dev
// builds build the base image here, so no registry image is theirs.
func cleanKeep(g *config.Global, reg registry.Registry) map[string]bool {
	keep := map[string]bool{"lodev/opensearch:" + g.Versions["opensearch"]: true, "lodev/mkcert:latest": true}
	if update.Stable(config.ImageTag) {
		keep[config.RegistryImage()] = true
	}
	for _, v := range config.SupportedPHP {
		keep[withTag(config.PHPImage(v))] = true
	}
	for name := range reg {
		keep[withTag(config.ProjectImage(name))] = true
	}
	return keep
}

// cleanCandidates are lodev's images (its own and the registry's) no container uses (by ref or by
// ID, as `docker ps -a` shows them) and lodev does not keep.
func cleanCandidates(imgs []imageInfo, used, keep map[string]bool) []imageInfo {
	var out []imageInfo
	for _, img := range imgs {
		if !strings.HasPrefix(img.Ref, "lodev/") && !strings.HasPrefix(img.Ref, config.RegistryRepo+":") || keep[img.Ref] {
			continue
		}
		inUse := false
		for u := range used {
			if withTag(u) == img.Ref || strings.HasPrefix(img.ID, u) || strings.HasPrefix(u, img.ID) {
				inUse = true
				break
			}
		}
		if !inUse {
			out = append(out, img)
		}
	}
	return out
}

func newCleanCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "clean",
		Short: "Remove lodev images no container uses, and Docker build cache older than 7 days",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			dry, _ := c.Flags().GetBool("dry-run")
			yes, _ := c.Flags().GetBool("yes")
			keepCache, _ := c.Flags().GetBool("keep-cache")
			g, err := config.LoadGlobal()
			if err != nil {
				return err
			}
			reg, err := registry.Load()
			if err != nil {
				return err
			}
			list, err := dockerOutput("images", "--filter", "reference=lodev/*", "--filter", "reference="+config.RegistryRepo, "--format", "{{.Repository}}:{{.Tag}}\t{{.ID}}\t{{.Size}}")
			if err != nil {
				return err
			}
			var imgs []imageInfo
			for _, l := range strings.Split(list, "\n") {
				if f := strings.Split(l, "\t"); len(f) == 3 {
					imgs = append(imgs, imageInfo{f[0], f[1], f[2]})
				}
			}
			ps, err := dockerOutput("ps", "-a", "--format", "{{.Image}}")
			if err != nil {
				return err
			}
			used := map[string]bool{}
			for _, u := range strings.Fields(ps) {
				used[u] = true
			}
			cands := cleanCandidates(imgs, used, cleanKeep(g, reg))
			out := c.OutOrStdout()
			if len(cands) > 0 {
				rows := make([][]string, len(cands))
				for i, img := range cands {
					rows[i] = []string{img.Ref, img.Size}
				}
				printTable(out, []string{"IMAGE", "SIZE"}, rows)
			}
			if !keepCache {
				df, _ := dockerOutput("system", "df", "--format", "{{.Type}}\t{{.Reclaimable}}")
				cache := "?"
				for _, l := range strings.Split(df, "\n") {
					if t, v, ok := strings.Cut(l, "\t"); ok && t == "Build Cache" {
						cache = v
					}
				}
				fmt.Fprintf(out, "Build cache older than 7 days (of %s reclaimable): Docker's build cache, shared by every project on this machine\n", cache)
			}
			if len(cands) == 0 && keepCache {
				fmt.Fprintln(out, "Nothing to clean.")
				return nil
			}
			if dry || !confirm("Delete these?", yes) {
				return nil
			}
			removed := 0
			for _, img := range cands {
				if _, err := dockerOutput("rmi", img.Ref); err != nil {
					ui.Warn("could not remove %s: %v", img.Ref, err)
					continue
				}
				removed++
			}
			ui.Success("Removed %d image(s)", removed)
			if keepCache {
				return nil
			}
			// docker prints the space it reclaimed.
			return dockerRun("builder", "prune", "-f", "--filter", "until=168h")
		},
	}
	c.Flags().Bool("dry-run", false, "only list what would be removed")
	c.Flags().Bool("yes", false, "do not ask before removing")
	c.Flags().Bool("keep-cache", false, "leave Docker's build cache alone")
	return c
}

func init() { rootCmd.AddCommand(newCleanCmd()) }
