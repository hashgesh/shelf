// Command shelf browses Conan 2 packages stored in JFrog Artifactory.
package main

import (
	"fmt"
	"os"
)

// Build metadata, injected at link time via -ldflags "-X main.version=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("shelf %s (commit %s, built %s)\n", version, commit, date)
		return
	}
	fmt.Fprintln(os.Stderr, "shelf: henüz uygulanmadı")
	os.Exit(1)
}
