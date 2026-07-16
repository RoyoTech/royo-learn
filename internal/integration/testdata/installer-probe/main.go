package main

import (
	"encoding/json"
	"fmt"
	"os"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 || os.Args[1] != "version" {
		os.Exit(2)
	}
	statePath := os.Getenv("ROYO_LEARN_PROBE_STATE")
	if statePath == "" {
		os.Exit(2)
	}
	if _, err := os.Stat(statePath); err == nil {
		fmt.Fprintln(os.Stderr, "simulated post-replacement failure")
		os.Exit(1)
	}
	if err := os.WriteFile(statePath, []byte("verified"), 0o600); err != nil {
		os.Exit(2)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"version": version})
}
