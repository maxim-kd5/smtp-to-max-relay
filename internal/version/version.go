package version

import "strings"

const baseVersion = "0.2"

// BuildNumber is injected at build time via ldflags.
var BuildNumber = "0"
var BuildSuffix = ""

func BotVersion() string {
	build := strings.TrimSpace(BuildNumber)
	if build == "" {
		build = "0"
	}

	version := baseVersion + "." + build
	suffix := strings.TrimSpace(BuildSuffix)
	if suffix != "" {
		version += "-" + suffix
	}

	return version
}
