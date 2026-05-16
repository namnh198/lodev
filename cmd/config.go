package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/namnh198/lodev/internal/lodev"
	"github.com/namnh198/lodev/pkg/fileutil"
	"github.com/namnh198/lodev/pkg/nodeps"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/spf13/cobra"
)

var (
	// webEnvironmentGlobalArgs allows user to set value of environment in web container
	webEnvironmentGlobalArg string
)

var ConfigCmd = &cobra.Command{
	Use:     "config [flags]",
	Short:   "Modify Lodev configuration.",
	Example: "lodev config --project-tld=test\nlodev config --router-http-port=80 --router-https-port=443 --traefik-monitor-port=10999",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		err := lodev.ReadLodevConfig()
		if err != nil {
			util.Failed("Failed to read Lodev global config: %v", err)
		}

		dirty := false
		if cmd.Flag("web-environment").Changed {
			env := strings.TrimSpace(webEnvironmentGlobalArg)
			if env == "" || env == `""` || env == `''` {
				lodev.LodevConfig.WebEnvironment = []string{}
			} else {
				lodev.LodevConfig.WebEnvironment = strings.Split(env, ",")
			}
			dirty = true
		}

		if cmd.Flag("web-environment-add").Changed {
			env := strings.TrimSpace(webEnvironmentGlobalArg)
			if env == "" {
				lodev.LodevConfig.WebEnvironment = []string{}
			} else {
				envspl := strings.Split(env, ",")
				conc := append(lodev.LodevConfig.WebEnvironment, envspl...)
				// Convert to a hashmap to remove duplicate values.
				hashmap := make(map[string]string)
				for i := range conc {
					hashmap[conc[i]] = conc[i]
				}
				keys := []string{}
				for key := range hashmap {
					keys = append(keys, key)
				}
				lodev.LodevConfig.WebEnvironment = keys
				sort.Strings(lodev.LodevConfig.WebEnvironment)
			}
			dirty = true
		}

		if cmd.Flag("internet-detection-timeout-ms").Changed {
			val, _ := cmd.Flags().GetString("internet-detection-timeout-ms")
			lodev.LodevConfig.InternetWaitTimeout = val
			dirty = true
		}

		if cmd.Flag("container-wait-timeout").Changed {
			val, _ := cmd.Flags().GetString("container-wait-timeout")
			lodev.LodevConfig.ContainerWaitTimeout = val
			dirty = true
		}

		if cmd.Flag("project-tld").Changed {
			val, _ := cmd.Flags().GetString("project-tld")
			lodev.LodevConfig.ProjectTld = val
			dirty = true
		}

		if cmd.Flag("router-http-port").Changed {
			val, _ := cmd.Flags().GetString("router-http-port")
			lodev.LodevConfig.HttpPort = val
			dirty = true
		}

		if cmd.Flag("router-https-port").Changed {
			val, _ := cmd.Flags().GetString("router-https-port")
			lodev.LodevConfig.HttpsPort = val
			dirty = true
		}

		if cmd.Flag("traefik-monitor-port").Changed {
			val, _ := cmd.Flags().GetString("traefik-monitor-port")
			lodev.LodevConfig.TraefikMonitorPort = val
			dirty = true
		}

		if cmd.Flag("letsencrypt-email").Changed {
			val, _ := cmd.Flags().GetString("letsencrypt-email")
			lodev.LodevConfig.LetsEncryptEmail = val
			dirty = true
		}

		if cmd.Flag("use-letsencrypt").Changed {
			val, _ := cmd.Flags().GetBool("use-letsencrypt")
			lodev.LodevConfig.UseLetsEncrypt = val
			dirty = true
		}

		if cmd.Flag("use-docker-compose-from-path").Changed {
			val, _ := cmd.Flags().GetBool("use-docker-compose-from-path")
			lodev.LodevConfig.UseDockerComposeSystem = val
			dirty = true
		}

		if cmd.Flag("use-docker-buildx-from-system").Changed {
			val, _ := cmd.Flags().GetBool("use-docker-buildx-from-system")
			lodev.LodevConfig.UseDockerBuildxSystem = val
			dirty = true
		}

		// write lodev config
		if dirty {
			err = lodev.ValidateLodevConfig()
			if err != nil {
				util.Failed("Invalid configuration in %s: %v", lodev.GetLodevConfigDir(), err)
			}
			err = lodev.SaveLodevConfig()
			if err != nil {
				util.Failed("Failed to write Lodev global config: %v", err)
			}
			util.Success("Changed Lodev configuration. See the currently config:\n")
		}

		// print the currently config if no errors occur, even if no changes were made
		renderTableFromYAML(lodev.LodevConfig)
	},
}

func init() {
	ConfigCmd.Flags().StringVarP(&webEnvironmentGlobalArg, "web-environment", "", "", `Set the environment variables in the web container: --web-environment="TYPO3_CONTEXT=Development,SOMEENV=someval"`)
	ConfigCmd.Flags().StringVarP(&webEnvironmentGlobalArg, "web-environment-add", "", "", `Append environment variables to the web container: --web-environment-add="TYPO3_CONTEXT=Development,SOMEENV=someval"`)
	ConfigCmd.Flags().String("internet-detection-timeout-ms", nodeps.InternetWaitTimeout, "Increase timeout when checking internet timeout (ms)")
	ConfigCmd.Flags().String("container-wait-timeout", nodeps.ContainerWaitTimeout, "Increase timeout when waiting for a container to be ready (s)")
	ConfigCmd.Flags().String("project-tld", nodeps.ProjectTld, "Set the default top-level domain to be used for all projects, can be overridden by project configuration")
	completionFunc(ConfigCmd, "project-tld", []string{nodeps.ProjectTld})
	ConfigCmd.Flags().String("router-http-port", nodeps.HttpPort, "The default router HTTP port for all projects. Can be changed if there are port (80) conflicts")
	completionFunc(ConfigCmd, "router-http-port", []string{nodeps.HttpPort})
	ConfigCmd.Flags().String("router-https-port", nodeps.HttpsPort, "The default router HTTPS port for all projects. Can be changed if there are port (443) conflicts")
	completionFunc(ConfigCmd, "router-https-port", []string{nodeps.HttpsPort})
	ConfigCmd.Flags().String("traefik-monitor-port", nodeps.TraefikMonitorPort, "Can be used to change the Traefik monitor port in case of port conflicts, for example 'lodev config --traefik-monitor-port=11999'")
	completionFunc(ConfigCmd, "traefik-monitor-port", []string{nodeps.TraefikMonitorPort})
	ConfigCmd.Flags().Bool("performance-mode-reset", false, "Reset performance mode to the default value (detect by OS)")
	ConfigCmd.Flags().String("letsencrypt-email", "", "Email associated with Let's Encrypt, 'lodev config --letsencrypt-email=me@example.com'")
	ConfigCmd.Flags().Bool("use-letsencrypt", false, "Enables experimental Let's Encrypt integration")
	completionFunc(ConfigCmd, "use-letsencrypt", []string{"true", "false"})
	ConfigCmd.Flags().Bool("use-docker-compose-from-path", false, fmt.Sprintf("If true, use docker-compose from path instead of private %s (used only in development testing)", fileutil.ShortHomeJoin(lodev.GetLodevBinPath("docker-compose"))))
	ConfigCmd.Flags().MarkHidden("use-docker-compose-from-path")
	ConfigCmd.Flags().String("required-docker-compose-version", "", "Override default docker-compose version (used only in development testing)")
	ConfigCmd.Flags().String("required-docker-buildx-version", "", "Override default docker-buildx version (used only in development testing)")
	ConfigCmd.Flags().Bool("use-docker-buildx-from-system", false, fmt.Sprintf("If true, use docker-buildx from system instead of private %s (used only in development testing)", fileutil.ShortHomeJoin(lodev.GetLodevBinPath("docker-buildx"))))
	ConfigCmd.Flags().MarkHidden("use-docker-buildx-from-system")

	RootCmd.AddCommand(ConfigCmd)
}
