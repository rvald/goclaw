package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rvald/goclaw/internal/discord"
	"github.com/rvald/goclaw/internal/discovery"
	"github.com/rvald/goclaw/internal/gateway"
	"github.com/rvald/goclaw/internal/logger"
	"github.com/rvald/goclaw/internal/pairing"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the gateway server",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Setup config from flags
		cfg := Config{
			Port:         cfgPort,
			Bind:         cfgBind,
			AuthToken:    cfgAuthToken,
			DiscordToken: cfgDiscordToken,
			GuildID:      cfgGuildID,
			StateDir:     cfgStateDir,
			TickInterval: 15 * time.Second,
		}

		if err := validateConfig(cfg); err != nil {
			return err
		}

		// Configure logging
		logger.Setup(cfg.StateDir)

		return runServer(cfg)
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)

	// Local flags for server
	serverCmd.Flags().IntVar(&cfgPort, "port", envInt("GOCLAW_PORT", 18789), "WebSocket server port")
	serverCmd.Flags().StringVar(&cfgBind, "bind", envStr("GOCLAW_BIND", "loopback"), "Bind mode: loopback or lan")
	serverCmd.Flags().StringVar(&cfgAuthToken, "token", envStr("GOCLAW_TOKEN", ""), "Auth token for node connections")
	serverCmd.Flags().StringVar(&cfgDiscordToken, "discord-token", envStr("DISCORD_BOT_TOKEN", ""), "Discord bot token")
	serverCmd.Flags().StringVar(&cfgGuildID, "guild-id", envStr("DISCORD_GUILD_ID", ""), "Discord guild ID")
}

func runServer(cfg Config) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 1. Initialize Pairing State
	pairingStore, err := pairing.NewStore(filepath.Join(cfg.StateDir, "pairing"))
	if err != nil {
		return fmt.Errorf("pairing store: %w", err)
	}
	pairingSvc := pairing.NewService(pairingStore)

	// 2. Initialize Discovery (Bonjour)
	mdnsCfg := discovery.Config{
		InstanceName: "OpenClaw Gateway", // TODO: Make configurable or use hostname
		Port:         cfg.Port,
		LanHost:      "", // auto-detect
		Meta: discovery.Metadata{
			Role:        "gateway",
			Transport:   "gateway",
			GatewayPort: fmt.Sprintf("%d", cfg.Port),
			DisplayName: "OpenClaw Gateway",
		},
	}
	advertiser, err := discovery.NewAdvertiser(mdnsCfg)
	if err != nil {
		slog.Warn("failed to init bonjour", "error", err)
		// Don't fail hard, just warn
	} else {
		if err := advertiser.Start(); err != nil {
			slog.Warn("failed to start bonjour", "error", err)
		} else {
			slog.Info("bonjour advertising started")
			defer advertiser.Stop()
		}
	}

	// 3. Create Gateway
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

	// 4. Discord Bot
	var bot *discord.Bot
	if cfg.DiscordToken != "" {
		bot, err = discord.NewBot(discord.BotConfig{
			Token:   cfg.DiscordToken,
			GuildID: cfg.GuildID,
		})
		if err != nil {
			return fmt.Errorf("discord init: %w", err)
		}
		router := discord.NewCommandRouter(gw.Invoker(), gw.Registry())
		router.WithPairing(pairingSvc, pairingStore)
		bot.SetRouter(router)
		bot.RegisterCommands(router.Commands())

		if err := bot.Start(ctx); err != nil {
			slog.Warn("discord failed to connect", "error", err)
			bot = nil
		}
	}

	// Banner
	printBanner(cfg, bot != nil)

	// Run
	go func() {
		<-ctx.Done()
		slog.Info("shutting down...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		if bot != nil {
			bot.Stop()
		}
		if advertiser != nil {
			advertiser.Stop()
		}
		gw.Shutdown(shutdownCtx)
	}()

	return gw.Run(ctx)
}

func printBanner(cfg Config, discordConnected bool) {
	bindAddr := "127.0.0.1"
	if cfg.Bind == "lan" {
		bindAddr = "0.0.0.0"
	}
	authMode := "none"
	if cfg.AuthToken != "" {
		authMode = "token"
	}
	discordStatus := "disabled"
	if discordConnected {
		discordStatus = "connected"
	}

	fmt.Printf("\n")
	fmt.Printf("  goclaw v%s\n", version)
	fmt.Printf("  ws://%s:%d  auth=%s  bind=%s\n", bindAddr, cfg.Port, authMode, cfg.Bind)
	fmt.Printf("  discord: %s  pairing: enabled  bonjour: enabled\n", discordStatus)
	fmt.Printf("  state: %s\n", cfg.StateDir)
	fmt.Printf("  health: http://%s:%d/health\n", bindAddr, cfg.Port)
	fmt.Printf("\n")
}
