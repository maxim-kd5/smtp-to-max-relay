package version

import "strings"

const baseVersion = "0.2"

// BuildNumber is injected at build time via ldflags.
var BuildNumber = "0"

func BotVersion() string {
	build := strings.TrimSpace(BuildNumber)
	if build == "" {
		build = "0"
	}
	return baseVersion + "." + build
}
