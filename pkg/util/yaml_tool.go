package util

import (
	"fmt"
	"math"
	"reflect"

	"dario.cat/mergo"
	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
	"go.yaml.in/yaml/v4"
)

// LoadYAML loads a YAML file from the specified path into the provided output struct or map.
func LoadYAML(path string, out any) error {
	v := viper.NewWithOptions(viper.KeyDelimiter("."))
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return err
	}

	err := v.Unmarshal(out, func(dc *mapstructure.DecoderConfig) {
		dc.TagName = "yaml"
		dc.WeaklyTypedInput = true
		// Preserve float-to-string precision (e.g. YAML 8.0 → "8.0", not "8")
		dc.DecodeHook = floatToStringHookYAML()
	})

	if err != nil {
		return err
	}

	return nil
}

// MergeYAML  merges yaml files extraFiles into the baseFile, returning the result as a string
// Merging is *override* based, so later files can override contents of others
func MergeYAML(baseFile string, extraFiles ...string) (string, error) {
	resultMap := make(map[string]any)
	if err := LoadYAML(baseFile, &resultMap); err != nil {
		return "", err
	}

	for _, f := range extraFiles {
		m := make(map[string]any)
		if err := LoadYAML(f, &m); err != nil {
			return "", err
		}

		if err := mergo.Merge(&resultMap, m, mergo.WithOverride); err != nil {
			return "", err
		}
	}

	result, err := yaml.Marshal(resultMap)

	if err != nil {
		return "", err
	}

	return string(result), nil
}

// floatToStringHookYAML returns a mapstructure DecodeHookFunc that preserves
// decimal representation when converting floats to strings.
// YAML parses unquoted values like `8.0` as float64(8), but when that value
// targets a string field (e.g. database version), mapstructure's weak typing
// would format it as "8". This hook ensures "8.0" is preserved by detecting
// whole-number floats and appending ".0".
func floatToStringHookYAML() mapstructure.DecodeHookFunc {
	return func(from reflect.Type, to reflect.Type, data any) (any, error) {
		if from.Kind() == reflect.Float64 && to.Kind() == reflect.String {
			f := data.(float64)
			// If the float is a whole number (e.g. 8.0), format with one decimal
			// place to preserve the ".0" that was in the original YAML.
			// Non-whole floats (e.g. 10.11) are formatted normally.
			if f == math.Trunc(f) {
				return fmt.Sprintf("%.1f", f), nil
			}
			return fmt.Sprintf("%g", f), nil
		}
		return data, nil
	}
}
