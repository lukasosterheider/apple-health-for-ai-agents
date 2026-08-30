package main

import (
	"os"

	"github.com/lukasosterheider/apple-health-for-ai-agents/cli/internal/healthsync"
)

func main() { os.Exit(healthsync.Run(os.Args[1:], os.Stdout, os.Stderr)) }
