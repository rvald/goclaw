package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rvald/goclaw/internal/discord"
	"github.com/rvald/goclaw/internal/gateway"
	"github.com/rvald/goclaw/internal/pairing"
)

const version = "0.1.0"

// Config holds all runtime configuration.
type Config struct {
	Port         int
	Bind         string // "loopback" or "lan"
	AuthToken    string
	DiscordToken string
	GuildID      string
	TickInterval time.Duration
	StateDir     string
}

func parseConfig() Config {
	cfg := Config{}

	flag.IntVar(&cfg.Port, "port", envInt("GOCLAW_PORT", 18789), "WebSocket server port")
	flag.StringVar(&cfg.Bind, "bind", envStr("GOCLAW_BIND", "loopback"), "Bind mode: loopback or lan")
	flag.StringVar(&cfg.AuthToken, "token", envStr("GOCLAW_TOKEN", ""), "Auth token for node connections")
	flag.StringVar(&cfg.DiscordToken, "discord-token", envStr("DISCORD_BOT_TOKEN", ""), "Discord bot token")
	flag.StringVar(&cfg.GuildID, "guild-id", envStr("DISCORD_GUILD_ID", ""), "Discord guild ID for command registration")
	flag.DurationVar(&cfg.TickInterval, "tick", 15*time.Second, "Tick keepalive interval")
	flag.StringVar(&cfg.StateDir, "state-dir", defaultStateDir(), "Directory for persistent state (pairing, etc.)")

	flag.Parse()
	return cfg
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

func run(cfg Config) error {
	// Signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Initialize pairing store + service (state dir created with 0700)
	pairingStore, err := pairing.NewStore(filepath.Join(cfg.StateDir, "pairing"))
	if err != nil {
		return fmt.Errorf("pairing store: %w", err)
	}
	pairingSvc := pairing.NewService(pairingStore)

	// Create gateway
	gw, err := gateway.New(gateway.GatewayConfig{
		Port:         cfg.Port,
		Bind:         cfg.Bind,
		AuthToken:    cfg.AuthToken,
		TickInterval: cfg.TickInterval,
		PairingSvc:   pairingSvc,
	})
	if err != nil {
		return fmt.Errorf("gateway init: %w", err)
	}

	// Discord bot (optional)
	var bot *discord.Bot
	if cfg.DiscordToken != "" {
		bot, err = discord.NewBot(discord.BotConfig{
			Token:   cfg.DiscordToken,
			GuildID: cfg.GuildID,
		})
		if err != nil {
			return fmt.Errorf("discord init: %w", err)
		}

		// Wire command router to the gateway's registry, invoker, and pairing
		router := discord.NewCommandRouter(gw.Invoker(), gw.Registry())
		router.WithPairing(pairingSvc, pairingStore)
		bot.SetRouter(router)
		bot.RegisterCommands(router.Commands())

		if err := bot.Start(ctx); err != nil {
			log.Printf("warning: discord failed to connect: %v", err)
			bot = nil // continue without Discord
		}
	}

	// Startup banner
	bindAddr := "127.0.0.1"
	if cfg.Bind == "lan" {
		bindAddr = "0.0.0.0"
	}
	authMode := "none"
	if cfg.AuthToken != "" {
		authMode = "token"
	}
	discordStatus := "disabled"
	if bot != nil {
		discordStatus = "connected"
	}

	fmt.Printf("\n")
	fmt.Printf("  goclaw v%s\n", version)
	fmt.Printf("  ws://%s:%d  auth=%s  bind=%s\n", bindAddr, cfg.Port, authMode, cfg.Bind)
	fmt.Printf("  discord: %s  pairing: enabled\n", discordStatus)
	fmt.Printf("  state: %s\n", cfg.StateDir)
	fmt.Printf("  health: http://%s:%d/health\n", bindAddr, cfg.Port)
	fmt.Printf("\n")

	// Run gateway (blocks until signal)
	go func() {
		<-ctx.Done()
		log.Println("shutting down...")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		if bot != nil {
			bot.Stop()
		}
		gw.Shutdown(shutdownCtx)
	}()

	return gw.Run(ctx)
}

func main() {
	cfg := parseConfig()
	if err := validateConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

// --- env helpers ---

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
