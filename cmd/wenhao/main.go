// Command wenhao is a config- and plugin-driven coding agent CLI.
package main

import (
	"os"

	"wenhao/internal/cli"

	// Blank imports wire compile-time built-ins into their registries.
	_ "wenhao/internal/provider/anthropic"
	_ "wenhao/internal/provider/openai"
	_ "wenhao/internal/tool/builtin"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(cli.Run(os.Args[1:], version))
}
