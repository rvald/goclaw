package main

import (
	"fmt"
	"os"
	"time"
)

const version = "0.1.0"

// Config holds runtime configuration (used by server command)
type Config struct {
	Port         int
	Bind         string
	AuthToken    string
	DiscordToken string
	GuildID      string
	TickInterval time.Duration
	StateDir     string
}

func validateConfig(cfg Config) error {
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return fmt.Errorf("invalid port: %d (must be 1-65535)", cfg.Port)
	}
	if cfg.Bind != "loopback" && cfg.Bind != "lan" {
		return fmt.Errorf("invalid bind mode: %q (must be \"loopback\" or \"lan\")", cfg.Bind)
	}
	if cfg.Bind == "lan" && cfg.AuthToken == "" {
		return fmt.Errorf("refusing to start: --bind lan requires --token to prevent unauthenticated access")
	}
	return nil
}

// Env helpers
func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return fallback
	}
	return n
}

// defaultStateDir is now also in root.go, 
// ensuring we don't have dupes or conflicts if we merge files.
// Since we split files, we can keep util functions in a utils.go or duplicate for now.
// For simplicity in this refactor step, I'll keep them here or move them.
// Actually, `root.go` has `defaultStateDir` and `main` function.
// `server.go` has `serverCmd`.
// `main.go` should just likely be `root.go` content if I wanted one file, 
// but Go allows multiple files in package main.
//
// The previous `root.go` write actually contained `func main()`.
// So `cmd/goclaw/main.go` should probably be DELETED or merged.
// 
// I will keep `main.go` as the entrypoint that calls Execute, 
// and `root.go` as the definition.
// 
// Let's correct the file distribution:
// 1. root.go: vars, rootCmd, Execute() (public func or just vars)
// 2. main.go: func main() { rootCmd.Execute() }
// 3. server.go: serverCmd logic

