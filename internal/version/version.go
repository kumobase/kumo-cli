// Package version holds the CLI version string, set at build time via
// -ldflags "-X github.com/kumobase/kumo-cli/internal/version.Version=vX.Y.Z".
package version

// Version is the kumo-cli version. Overridden at release build time.
var Version = "0.1.0-dev"
