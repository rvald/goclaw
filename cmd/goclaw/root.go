package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	// Persistent flags
	cfgStateDir string
	
	// Server flags (now persistent or specific to server cmd, 
	// but often useful to have global config)
	cfgPort         int
	cfgBind         string
	cfgAuthToken    string
	cfgDiscordToken string
	cfgGuildID      string
)

var rootCmd = &cobra.Command{
	Use:   "goclaw",
	Short: "OpenClaw Gateway",
	Long:  `Go implementation of the OpenClaw Gateway.`,
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgStateDir, "state-dir", defaultStateDir(), "Directory for persistent state")
	
	// Server-specific flags (can be global if other commands need them, 
	// but ideally 'nodes' command only needs state-dir)
	// For backward compatibility/ease, we can keep some global if needed, 
	// but let's stick to clean separation.
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// defaultStateDir returns XDG_STATE_HOME/goclaw or ~/.local/state/goclaw.
func defaultStateDir() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "goclaw")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".goclaw", "state")
	}
	return filepath.Join(home, ".local", "state", "goclaw")
}
