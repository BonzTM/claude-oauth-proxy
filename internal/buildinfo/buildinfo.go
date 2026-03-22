package buildinfo

import "fmt"

var (
	version     = "dev"
	commitShort = "unknown"
	builtAt     = "unknown"
)

func Version() string {
	return version
}

func Banner(binaryName string) string {
	if binaryName == "" {
		binaryName = "claude-oauth-proxy"
	}
	return fmt.Sprintf("%s %s (%s, %s)", binaryName, version, commitShort, builtAt)
}
