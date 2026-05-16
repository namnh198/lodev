package lodev

import (
	"context"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/Masterminds/sprig/v3"
	"github.com/moby/moby/api/types/container"
	"github.com/namnh198/lodev/pkg/dockerutil"
	"github.com/namnh198/lodev/pkg/nodeps"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/yarlson/tap"
)

// Regexp pattern to determine if a hostname is valid per RFC 1123.
var hostRegex = regexp.MustCompile(`^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$`)

// Regexp pattern to match Composer v1 versions: "1" or "1.x.y"
var composerV1Regex = regexp.MustCompile(`^1(\.\d+\.\d+)?$`)

// IsComposerV1 checks if the provided Composer version string matches the pattern for Composer v1.
func IsComposerV1(composerVer string) bool {
	return composerVer != "" && composerV1Regex.MatchString(composerVer)
}

// ValidateProjectName checks if the provided project name is valid per RFC 1123 for hostnames.
func ValidateProjectName(name string) error {
	if name == "" {
		return nil
	}

	match := hostRegex.MatchString(name)

	if !match {
		return fmt.Errorf("%s is not a valid project name. Please enter a project name in your configuration that will allow for a valid hostname.", name)
	}
	return nil
}

// ValidateProjectType checks if the given project type is valid (exists in the ProjectTypeMatrix)
func ValidateProjectType(projectType string) error {
	if _, ok := ProjectTypeMatrix[projectType]; !ok {
		return fmt.Errorf("Project type %s is not supported. We're supporting only: %v", projectType, GetProjectTypes())
	}

	return nil
}

// ValidateDocroot makes sure we have a usable docroot
// The docroot must remain inside the project root.
func ValidateDocroot(docroot string) error {
	switch {
	case filepath.IsAbs(docroot):
		return fmt.Errorf("docroot ('%s') cannot be an absolute path, it must be a relative path from the project root", docroot)
	case strings.HasPrefix(docroot, ".."):
		return fmt.Errorf("docroot ('%s') cannot begin with '..', it should be a relative path from project root but must remain inside the project", docroot)
	}
	return nil
}

// ValidatePHPVersion checks if the provided PHP version is valid.
func ValidatePHPVersion(phpVersion string) error {
	if phpVersion == "" {
		return nil
	}

	if !slices.Contains(nodeps.ValidPHPVersions, phpVersion) {
		return fmt.Errorf("%s is not a valid PHP version. Valid PHP versions are: %v", phpVersion, nodeps.ValidPHPVersions)
	}

	return nil
}

// ValidateWebserver checks if the provided webserver type is valid.
func ValidateWebserver(webserver string) error {
	if webserver == "" {
		return nil
	}

	if !slices.Contains(nodeps.ValidWebservers, webserver) {
		return fmt.Errorf("%s is not a valid webserver type. Valid webserver types are: %v", webserver, nodeps.ValidWebservers)
	}

	return nil
}

// SugggestPrompt returns a function that can be used as a suggestion provider for tap.Autocomplete.
func sugggestPrompt(list []string) func(string) []string {
	return func(input string) []string {
		if input == "" {
			return list
		}

		low := strings.ToLower(input)

		var out []string

		for _, s := range list {
			if strings.Contains(strings.ToLower(s), low) {
				out = append(out, s)
			}
		}

		return out
	}
}

// getTapSelectOptions converts a list of strings into a list of tap.SelectOption[string] with the same value and label.
func getSelectOptionsPrompt(list []string) []tap.SelectOption[string] {
	options := make([]tap.SelectOption[string], len(list))

	for i, item := range list {
		options[i] = tap.SelectOption[string]{
			Value: item,
			Label: item,
		}
	}

	return options
}

// AvailablePHPDocrootLocations returns an of default docroot locations to look for.
func AvailablePHPDocrootLocations() []string {
	return []string{
		"_www",
		"docroot",
		"htdocs",
		"html",
		"pub",
		"public",
		"web",
		"web/public",
		"webroot",
	}
}

// NormalizeProjectName normalizes a project name by converting it to lowercase and replacing dashes with underscores.
func NormalizeProjectName(name string) string {
	name = strings.ToLower(name)
	return strings.ReplaceAll(name, "-", "_")
}

// FindContainerByType will find a container for this site denoted by the containerType if it is available.
func FindContainerByType(containerType string, appName string) (*container.Summary, error) {
	labels := dockerutil.GetDockerLodevLabels(appName, map[string]string{
		"com.docker.compose.service": containerType,
		"com.docker.compose.oneoff":  "False",
	})
	return dockerutil.FindContainerByLabels(labels)
}

// getLodevContainers will return a list of all containers that are managed by LODEV, identified by the "com.lodev.platform" label.
func getLodevContainers() ([]container.Summary, error) {
	labels := map[string]string{
		dockerutil.LabelPlatform: "lodev",
	}
	containers, err := dockerutil.FindContainersByLabels(labels)
	if err != nil {
		return nil, fmt.Errorf("failed to find containers by labels %v: %v", labels, err)
	}
	return containers, nil
}

// GetTimezone tries to find local timezone from the path, that can be
// either $TZ environment variable or /etc/localtime symlink
func GetTimezone(path string) (string, error) {
	// Use case-insensitive search for /zoneinfo/ in the file path.
	regex := regexp.MustCompile(`(?i)/.*?zoneinfo.*?/`)
	parts := regex.Split(strings.TrimSpace(path), 2)
	if len(parts) != 2 {
		// If this is not a path, but timezone, return it.
		_, err := time.LoadLocation(path)
		if err == nil {
			return path, nil
		}
		return "", fmt.Errorf("unable to read timezone from %s", path)
	}
	timezone := parts[1]
	// Remove leading prefixes if they exist.
	// https://stackoverflow.com/a/67888343/8097891
	for _, prefix := range []string{"posix/", "right/"} {
		timezone = strings.TrimPrefix(timezone, prefix)
	}
	if timezone == "" {
		return "", fmt.Errorf("unable to read timezone from %s", path)
	}
	_, err := time.LoadLocation(timezone)
	if err != nil {
		return "", fmt.Errorf("failed to load timezone '%s': %v", timezone, err)
	}
	return timezone, nil
}

// getTemplateFuncMap will return a map of useful template functions.
func getTemplateFuncMap() map[string]any {
	// Use sprig's template function map as a base
	m := sprig.FuncMap()

	// Add helpful utilities on top of it
	m["joinPath"] = path.Join
	m["templateCanUse"] = templateCanUse

	return m
}

// templateCanUse will return true if the given feature is available.
// This is used in YAML templates to determine whether to use a feature or not.
func templateCanUse(feature string) bool {
	// healthcheck.start_interval requires Docker Engine v25 or later
	// See https://github.com/docker/compose/pull/10939
	if feature == "healthcheck.start_interval" {
		if err := CheckDockerVersion(DockerVersionMatrix{APIVersion: "1.44", Version: "25.0"}); err == nil {
			return true
		}
	}
	return false
}

// GetLocalTimezone tries to find local timezone from $TZ or /etc/localtime symlink
func GetLocalTimezone() (string, error) {
	timezone := ""
	if os.Getenv("TZ") != "" {
		timezone = os.Getenv("TZ")
	} else {
		localtimeFile := filepath.Join("/etc", "localtime")
		var err error
		timezone, err = filepath.EvalSymlinks(localtimeFile)
		if err != nil {
			return "", fmt.Errorf("unable to read timezone from %s file: %v", localtimeFile, err)
		}
	}
	return GetTimezone(timezone)
}

// IsValidHostname checks if the provided string is a valid hostname.
// Pattern breakdown:
// ([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])      : A single label (1 to 63 chars), no leading/trailing hyphens
// (\.([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9]))* : Zero or more additional labels separated by dots
func IsValidHostname(hostname string) bool {
	// Enforce the maximum length of 253 characters per RFC rules.
	if len(hostname) == 0 || len(hostname) > 253 {
		return false
	}

	hostnameRegex := regexp.MustCompile(`^([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])(\.([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9]))*$`)
	return hostnameRegex.MatchString(hostname)
}

// CanUseHTTPS checks if we can use HTTPS for the router, which requires that the router is enabled and that mkcert is set up with a CA root.
func CanUseHTTPS() bool {
	if IsRouterDisabled() {
		return false
	}

	if GetCAROOT() == "" {
		return false
	}

	return true
}

var (
	IsInternetActiveAlreadyChecked = false // IsInternetActiveAlreadyChecked flags whether it's been checked
	IsInternetActiveResult         = false // IsInternetActiveResult is the result of the check
	IsInternetActiveErr            error   // IsInternetActiveErr is the error encountered during the check, if any
	IsInternetActiveNetResolver    interface {
		LookupIP(ctx context.Context, network, host string) ([]net.IP, error)
	} = net.DefaultResolver // IsInternetActiveNetResolver wraps the standard DNS resolver.
)

// IsInternetActive checks to see if we have a viable
// internet connection. It tries a quick DNS query.
// This requires that the named record be query-able.
// This check will only be made once per command run.
func IsInternetActive() bool {
	// if this was already checked, return the result
	if IsInternetActiveAlreadyChecked {
		return IsInternetActiveResult
	}

	internetTimeout, _ := strconv.Atoi(LodevConfig.InternetWaitTimeout)
	timeout := time.Duration(internetTimeout) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Test by using Cloudflare's one.one.one.one DNS.
	testHostname := "one.one.one.one"
	addrs, err := IsInternetActiveNetResolver.LookupIP(ctx, "ip4", testHostname)

	// Internet is active (active == true) if both err and ctx.Err() were nil
	active := err == nil && ctx.Err() == nil
	// Remember the result to not call this twice
	IsInternetActiveAlreadyChecked = true
	IsInternetActiveResult = active
	if !active {
		IsInternetActiveErr = fmt.Errorf("unable to resolve testHostname=%s, addrs=%v, internet_detection_timeout_ms=%dms: %w", testHostname, addrs, timeout.Milliseconds(), err)
		if ctx.Err() != nil {
			IsInternetActiveErr = fmt.Errorf("%w; context error: %v", IsInternetActiveErr, ctx.Err())
		}
	}

	return active
}

// CheckLodevVersionConstraint validates if the given constraint matches the current LODEV version.
// If the version constraint includes pre-releases, it will normalize the constraint before checking.
// Returns an error if the version doesn't meet the constraint or if the constraint is invalid.
func CheckLodevVersionConstraint(constraint string, errorPrefix string, errorSuffix string) error {
	normalizedConstraint := constraint
	if strings.Contains(nodeps.LodevVersion, "-") {
		// Pre-releases need '-0' added for validation
		normalizedConstraint = normalizeConstraint(constraint)
	}
	util.Debug("Comparing constraint '%s' against version '%s'", normalizedConstraint, nodeps.LodevVersion)
	if errorPrefix == "" {
		errorPrefix = "error"
	}
	c, err := semver.NewConstraint(normalizedConstraint)
	if err != nil {
		return fmt.Errorf("%s: the '%s' constraint is not valid. See https://github.com/Masterminds/semver#checking-version-constraints for valid constraints format", errorPrefix, constraint)
	}
	// Make sure we do this check with valid released versions
	v, err := semver.NewVersion(nodeps.LodevVersion)
	if err == nil && !c.Check(v) {
		return fmt.Errorf("%s: your LODEV version '%s' doesn't meet the constraint '%s'. Please update to a LODEV version that meets this constraint %s", errorPrefix, nodeps.LodevVersion, constraint, strings.TrimSpace(errorSuffix))
	}

	return nil
}

// normalizeConstraint adds '-0' to version expressions that don't contain a prerelease identifier
// See https://github.com/Masterminds/semver#working-with-prerelease-versions
func normalizeConstraint(constraint string) string {
	// remove all commas, so we can split by spaces
	constraintNoCommas := strings.ReplaceAll(constraint, ",", " ")
	// Split the constraint into tokens based on spaces
	tokens := strings.Fields(constraintNoCommas)
	for i, token := range tokens {
		last := token[len(token)-1]
		// If the token represents a version number (ends with a digit or is a wildcard)
		// and doesn't contain a suffix '-0', append '-0'
		if !strings.HasSuffix(token, "-0") && strings.Contains("0123456789xX*", string(last)) {
			tokens[i] = token + "-0"
		}
	}
	return strings.Join(tokens, " ")
}
