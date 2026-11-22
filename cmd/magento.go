package cmd

import (
	"fmt"
	"regexp"
	"strings"

	"lodev/internal/config"
	"lodev/internal/docker"
	"lodev/internal/ui"
)

var magentoPrefixRe = regexp.MustCompile(`^[A-Za-z0-9_]*$`)

const magentoPrefixPHP = `$e = include "app/etc/env.php"; echo $e["db"]["table_prefix"] ?? "";`

// magentoEngineRe is the search engines whose catalog/search/<engine>_* settings lodev's OpenSearch answers.
var magentoEngineRe = regexp.MustCompile(`^(opensearch|elasticsearch[0-9]*)$`)

// magentoSQL points a production dump at url: base URLs on every scope, no CDN static/media URLs,
// no cookie domain, mail to Mailpit, and (engine not "") search on the shared OpenSearch under
// index. db, prefix, engine and index are checked identifiers; url comes from the validated config.
func magentoSQL(db, prefix, url, engine, index string) []string {
	table := fmt.Sprintf("`%s`.`%score_config_data`", db, prefix)
	insert := func(kv ...string) string {
		var rows []string
		for i := 0; i < len(kv); i += 2 {
			rows = append(rows, fmt.Sprintf("('default',0,'%s','%s')", kv[i], kv[i+1]))
		}
		return fmt.Sprintf("INSERT INTO %s (scope,scope_id,path,value) VALUES %s", table, strings.Join(rows, ","))
	}
	sql := []string{
		fmt.Sprintf("UPDATE %s SET value='%s' WHERE path IN ('web/unsecure/base_url','web/secure/base_url','web/unsecure/base_link_url','web/secure/base_link_url')", table, url),
		fmt.Sprintf("UPDATE %s SET value=NULL WHERE path IN ('web/unsecure/base_static_url','web/secure/base_static_url','web/unsecure/base_media_url','web/secure/base_media_url')", table),
		fmt.Sprintf("DELETE FROM %s WHERE path='web/cookie/cookie_domain'", table),
		// Every scope: a store-level production SMTP host would mail real customers.
		fmt.Sprintf("DELETE FROM %s WHERE path LIKE 'system/smtp/%%'", table),
		insert("system/smtp/disable", "0", "system/smtp/transport", "smtp", "system/smtp/host", "mailpit", "system/smtp/port", "1025", "system/smtp/auth", "none"),
	}
	if engine == "" {
		return sql
	}
	s := "catalog/search/" + engine
	return append(sql,
		fmt.Sprintf("DELETE FROM %s WHERE path IN ('%s_server_hostname','%[2]s_server_port','%[2]s_enable_auth','%[2]s_index_prefix')", table, s),
		insert(s+"_server_hostname", "opensearch", s+"_server_port", "9200", s+"_enable_auth", "0", s+"_index_prefix", index))
}

// magentoAfterImport runs after db-import into a Magento project's own database.
func magentoAfterImport(p *config.Project, t dbTarget) error {
	webUp := containerRunning(p.WebContainer())
	prefix := ""
	if webUp {
		user, _ := p.ContainerUser()
		out, err := dockerOutput(docker.ExecArgs(p.ComposeProject(), composeFile(p), "web", user, p.WorkDir(), false,
			// lodev's php shows errors on stdout: a missing env.php must not read as the prefix.
			[]string{"php", "-d", "display_errors=stderr", "-r", magentoPrefixPHP})...)
		if err != nil {
			ui.Info("Magento: could not read app/etc/env.php (%v); assuming no table prefix", err)
			out = ""
		}
		prefix = out
	} else {
		ui.Info("Magento: %s is not running; assuming no table prefix", p.WebContainer())
	}
	if !magentoPrefixRe.MatchString(prefix) {
		return fmt.Errorf("Magento: table prefix %q is not valid", prefix)
	}
	engine, err := dockerOutput("exec", config.ServiceContainer(t.service), t.service, "-uroot", "-proot", "-N", "-e",
		fmt.Sprintf("SELECT value FROM `%s`.`%score_config_data` WHERE scope='default' AND scope_id=0 AND path='catalog/search/engine'", t.db, prefix))
	if err != nil {
		return fmt.Errorf("Magento: %w", err)
	}
	if engine == "" || engine == "NULL" {
		engine = "opensearch" // Magento's default when the dump does not set one
	}
	if !magentoEngineRe.MatchString(engine) {
		ui.Info("Magento: search engine %q: set its host yourself", engine)
		engine = ""
	}
	url := "https://" + p.Domains[0] + "/"
	for _, sql := range magentoSQL(t.db, prefix, url, engine, p.Name) {
		if err := dbExec(t.service, sql); err != nil {
			return fmt.Errorf("Magento: %w", err)
		}
	}
	ui.Info("Magento: base URLs → %s, mail → Mailpit", url)
	if engine != "" {
		ui.Info("Magento: %s → opensearch:9200, index prefix %s", engine, p.Name)
	}
	if !webUp {
		ui.Info("Magento: run php bin/magento cache:flush after lodev start")
		return nil
	}
	return execInWeb(p, []string{"php", "bin/magento", "cache:flush"})
}
