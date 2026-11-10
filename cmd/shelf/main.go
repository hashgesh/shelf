// Command shelf browses Conan 2 packages stored in JFrog Artifactory.
package main

import (
	"os"

	"github.com/hashgesh/shelf/internal/cli"
)

// Build metadata, injected at link time via -ldflags "-X main.version=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(cli.Execute(cli.BuildInfo{Version: version, Commit: commit, Date: date}))
}
