package nodeps

// List PHP versions supported by LODEV
const (
	PHP56 = "5.6"
	PHP70 = "7.0"
	PHP71 = "7.1"
	PHP72 = "7.2"
	PHP73 = "7.3"
	PHP74 = "7.4"
	PHP80 = "8.0"
	PHP81 = "8.1"
	PHP82 = "8.2"
	PHP83 = "8.3"
	PHP84 = "8.4"
	PHP85 = "8.5"
)

// Valid PHP versions supported by LODEV
var ValidPHPVersions = []string{PHP56, PHP70, PHP71, PHP72, PHP73, PHP74, PHP80, PHP81, PHP82, PHP83, PHP84, PHP85}

var LegacyPHPVersions = []string{PHP56, PHP70, PHP71, PHP72, PHP73, PHP74, PHP80, PHP81}

// List NodeJS versions supported by LODEV
const (
	NodeJS18     = "18"
	NodeJS20     = "20"
	NodeJS24     = "24"
	NodeJS25     = "25"
	NodeJS26     = "26"
	NodeJSLTS    = "lts"
	NodeJSLatest = "latest"
)

// Valid NodeJS versions supported by LODEV
var ValidNodeJSVersions = []string{NodeJS18, NodeJS20, NodeJS24, NodeJS25, NodeJS26, NodeJSLTS, NodeJSLatest}

// List composer versions supported by LODEV
const (
	Composer22      = "2.2"
	Composer23      = "2.3"
	Composer24      = "2.4"
	Composer25      = "2.5"
	Composer26      = "2.6"
	Composer27      = "2.7"
	Composer28      = "2.8"
	Composer29      = "2.9"
	Composer2       = "2"
	ComposerLatest  = "latest"
	ComposerPreview = "preview"
	ComposerStable  = "stable"
)

// Valid composer versions supported by LODEV
var ValidComposerVersions = []string{Composer22, Composer23, Composer24, Composer25, Composer26, Composer27, Composer28, Composer29, Composer2, ComposerLatest, ComposerPreview, ComposerStable}

// List websever types supported by LODEV
const (
	WebserverApacheFPM = "apache-fpm"
	WebserverNginxFPM  = "nginx-fpm"
)

// Valid webservers supported by LODEV
var ValidWebservers = []string{WebserverNginxFPM, WebserverApacheFPM}

// Default versions for PHP, NodeJS, and Composer
const (
	DefaultPHPVersion      = PHP84
	DefaultNodeJSVersion   = NodeJSLTS
	DefaultComposerVersion = ComposerStable
	DefaultWebserver       = WebserverNginxFPM
)
