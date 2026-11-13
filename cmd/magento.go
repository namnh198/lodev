package cmd

import (
	"fmt"
	"regexp"

	"lodev/internal/config"
	"lodev/internal/docker"
	"lodev/internal/ui"
)

var magentoPrefixRe = regexp.MustCompile(`^[A-Za-z0-9_]*$`)

const magentoPrefixPHP = `$e = include "app/etc/env.php"; echo $e["db"]["table_prefix"] ?? "";`

// magentoSQL points a production dump at url: base URLs on every scope, no CDN static/media URLs,
// no cookie domain. db and prefix are checked identifiers; url comes from the validated config.
func magentoSQL(db, prefix, url string) []string {
	table := fmt.Sprintf("`%s`.`%score_config_data`", db, prefix)
	return []string{
		fmt.Sprintf("UPDATE %s SET value='%s' WHERE path IN ('web/unsecure/base_url','web/secure/base_url','web/unsecure/base_link_url','web/secure/base_link_url')", table, url),
		fmt.Sprintf("UPDATE %s SET value=NULL WHERE path IN ('web/unsecure/base_static_url','web/secure/base_static_url','web/unsecure/base_media_url','web/secure/base_media_url')", table),
		fmt.Sprintf("DELETE FROM %s WHERE path='web/cookie/cookie_domain'", table),
	}
}

// magentoAfterImport runs after db-import into a Magento project's own database.
func magentoAfterImport(p *config.Project, t dbTarget) error {
	webUp := containerRunning("lodev-" + p.Name + "-web")
	prefix := ""
	if webUp {
		user, _ := p.ContainerUser()
		out, err := dockerOutput(docker.ExecArgs(composeProject(p), composeFile(p), "web", user, p.WorkDir(), false,
			// lodev's php shows errors on stdout: a missing env.php must not read as the prefix.
			[]string{"php", "-d", "display_errors=stderr", "-r", magentoPrefixPHP})...)
		if err != nil {
			ui.Info("Magento: could not read app/etc/env.php (%v); assuming no table prefix", err)
			out = ""
		}
		prefix = out
	} else {
		ui.Info("Magento: lodev-%s-web is not running; assuming no table prefix", p.Name)
	}
	if !magentoPrefixRe.MatchString(prefix) {
		return fmt.Errorf("Magento: table prefix %q is not valid", prefix)
	}
	url := "https://" + p.Domains[0] + "/"
	for _, sql := range magentoSQL(t.db, prefix, url) {
		if err := dbExec(t.service, sql); err != nil {
			return fmt.Errorf("Magento: %w", err)
		}
	}
	ui.Info("Magento: base URLs → %s", url)
	if !webUp {
		ui.Info("Magento: run php bin/magento cache:flush after lodev start")
		return nil
	}
	return execInWeb(p, []string{"php", "bin/magento", "cache:flush"})
}
