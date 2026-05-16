package cmd

import (
	"fmt"
	"strings"

	"github.com/namnh198/lodev/internal/lodev"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/spf13/cobra"
	"github.com/yarlson/tap"
)

var (
	serviceNamesArg []string
)

var ServiceCmd = &cobra.Command{
	Use:     "service",
	Aliases: []string{"addon", "add-on", "addons", "adds-on", "services"},
	Short:   "A collection of commands for managing LODEV services",
	Args:    cobra.ExactArgs(0),
	Example: `
lodev service
lodev service --add mysql,phpmyadmin,mailpit
lodev service --remove mailpit
`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Flag("update").Changed {
			return nil
		}
		if cmd.Flag("add").Changed && cmd.Flag("remove").Changed {
			return fmt.Errorf("cannot use --add and --remove flags together")
		}

		if cmd.Flag("add").Changed {
			if a := cmd.Flag("add").Value.String(); a == "" {
				return fmt.Errorf("--add flag requires a non-empty value")
			}
		}

		if cmd.Flag("remove").Changed {
			if a := cmd.Flag("remove").Value.String(); a == "" {
				return fmt.Errorf("--remove flag requires a non-empty value")
			}
		}

		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		var forceRefresh bool

		if cmd.Flags().NFlag() == 0 {
			renderTable([]string{"NAME", "LOCATION"}, lodev.DescribeLodevServices())
			return
		}

		// Handle the --update flag to refresh the service registry cache
		if cmd.Flag("update").Changed {
			tap.Intro("💻 Updating services registry")
			_, err := lodev.GetServiceList(true)
			if err != nil {
				tap.Cancel(fmt.Sprintf("Failed to update service registry: %v\n", err), tap.MessageOptions{})
			} else {
				util.SuccessMessage("Service registry updated successfully.", tap.MessageOptions{
					Hint: fmt.Sprintf("Saved at: %s", lodev.GetLodevServicePath("service_list.yaml")),
				})
			}
			return
		}

		var resultMessage string

		if cmd.Flag("add").Changed {
			tap.Intro("Add LODEV services")
			addedServices, err := lodev.AddServices(forceRefresh, serviceNamesArg...)
			if err != nil {
				tap.Cancel("Failed to add services.\nTry running 'lodev service --update' to refresh the service registry", tap.MessageOptions{
					Hint: fmt.Sprintf("%v", err),
				})
			}
			resultMessage = fmt.Sprintf("Successfully added services: %s", strings.Join(addedServices, ", "))
		}

		if cmd.Flag("remove").Changed {
			tap.Intro("Remove LODEV services")
			removedServices, err := lodev.RemoveServices(forceRefresh, serviceNamesArg...)

			if err != nil {
				tap.Cancel("Failed to remove services.\nTry running 'lodev service --update' to refresh the service registry", tap.MessageOptions{
					Hint: fmt.Sprintf("%v", err),
				})
			}
			resultMessage = fmt.Sprintf("Successfully removed services: %s", strings.Join(removedServices, ", "))
			forceRefresh = true
		}

		if ok, _ := cmd.Flags().GetBool("start"); !ok {
			tap.Outro(resultMessage, tap.MessageOptions{
				Hint: "You may use 'lodev services restart' to apply the changes",
			})
			return
		}

		err := lodev.StartLodevService(true, forceRefresh)
		if err != nil {
			tap.Cancel(fmt.Sprintf("Failed to start services: %v", err), tap.MessageOptions{})
		} else {
			tap.Outro(resultMessage, tap.MessageOptions{
				Hint: "Use `lodev service` to check the status of your services",
			})
		}
	},
}

func init() {
	validServiceNames, _ := getValidServiceNames()
	ServiceCmd.Flags().BoolP("start", "s", true, "Start the service immediately after adding/removing services")
	ServiceCmd.Flags().BoolP("update", "u", false, "Update the service registry by re-scan the services config directory")
	ServiceCmd.Flags().StringSliceVarP(&serviceNamesArg, "add", "a", []string{}, "Add(s) services by their name. Comma-separated list of repository names")
	completionFunc(ServiceCmd, "add", validServiceNames)
	ServiceCmd.Flags().StringSliceVarP(&serviceNamesArg, "remove", "r", []string{}, "Remove(s) services by their name. Comma-separated list of repository names")
	completionFunc(ServiceCmd, "remove", validServiceNames)
	RootCmd.AddCommand(ServiceCmd)
}

// getValidServiceNames retrieves the list of valid service names from the service registry.
func getValidServiceNames() ([]string, error) {
	serviceList, err := lodev.GetServiceList()
	if err != nil {
		return []string{}, err
	}
	var names []string
	for _, service := range serviceList.Services {
		names = append(names, service.Name)
	}
	return names, nil
}
