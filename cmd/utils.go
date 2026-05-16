package cmd

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yarlson/tap"
)

// completionFunc returns a Cobra completion function with static values.
func completionFunc(cmd *cobra.Command, flag string, values []string) {
	cmd.RegisterFlagCompletionFunc(flag, func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return values, cobra.ShellCompDirectiveNoFileComp
	})
}

// renderTableFromYAML renders a table of configuration values from a struct using reflection.
func renderTableFromYAML(vars any) {
	v := reflect.ValueOf(vars)
	typeOfVal := v.Type()

	configTBHeader := []string{"Field", "Value"}
	configTBRows := make([][]string, 0, v.NumField())

	for i := 0; i < v.NumField(); i++ {
		tag := typeOfVal.Field(i).Tag.Get("yaml")
		parts := strings.Split(tag, ",")
		tag = parts[0]
		fieldValue := v.Field(i).Interface()
		if tag != "build info" && tag != "web_environment" && tag != "router" {
			key := strings.ReplaceAll(tag, "_", "-")
			value := fmt.Sprintf("%v", fieldValue)
			if key != "-" {
				configTBRows = append(configTBRows, []string{key, value})
			}
		}
	}

	keys := make([]string, 0, v.NumField())
	valMap := map[string]string{}
	for i := 0; i < v.NumField(); i++ {
		tag := typeOfVal.Field(i).Tag.Get("yaml")
		parts := strings.Split(tag, ",")
		tag = parts[0]
		fieldValue := v.Field(i).Interface()
		tagWithDashes := strings.ReplaceAll(tag, "_", "-")
		valMap[tagWithDashes] = fmt.Sprintf("%v", fieldValue)
		keys = append(keys, tagWithDashes)
	}
	sort.Strings(keys)
	renderTable(configTBHeader, configTBRows)
}

func renderTable(tbHeaders []string, tbRows [][]string) {
	if len(tbRows) == 0 {
		tap.Outro("No data available.", tap.MessageOptions{})
		return
	}

	tap.Table(tbHeaders, tbRows, tap.TableOptions{
		ShowBorders:   true,
		IncludePrefix: false,
		HeaderStyle:   tap.TableStyleBold,
		HeaderColor:   tap.TableColorCyan,
		FormatBorder:  tap.CyanBorder,
	})
}
