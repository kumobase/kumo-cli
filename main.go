// Command kumo is the command-line interface for the Kumo platform
// (https://kumo.run). It is a thin wrapper over the github.com/kumobase/kumo-go
// SDK.
package main

import "github.com/kumobase/kumo-cli/internal/cli"

func main() {
	cli.Execute()
}
