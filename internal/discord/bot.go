package discord

import (
	"bytes"
	"context"
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

// BotConfig holds the configuration for the Discord bot.
type BotConfig struct {
	Token   string
	GuildID string
}

// Bot wraps a discordgo session with command routing.
type Bot struct {
	config   BotConfig
	session  *discordgo.Session
	router   *CommandRouter
	commands []SlashCommand
}

// NewBot validates config and creates a new Bot.
func NewBot(config BotConfig) (*Bot, error) {
	if config.Token == "" {
		return nil, fmt.Errorf("discord bot token is required")
	}
	return &Bot{config: config}, nil
}

// SetRouter sets the command router for handling slash commands.
func (b *Bot) SetRouter(router *CommandRouter) {
	b.router = router
}

// RegisterCommands stores commands for registration on Start.
func (b *Bot) RegisterCommands(cmds []SlashCommand) {
	b.commands = cmds
}

// Start connects to Discord, registers slash commands, and installs the
// interaction handler that routes commands to the CommandRouter.
func (b *Bot) Start(ctx context.Context) error {
	session, err := discordgo.New("Bot " + b.config.Token)
	if err != nil {
		return fmt.Errorf("discord session: %w", err)
	}
	b.session = session

	b.session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages

	// Install interaction handler
	b.session.AddHandler(b.handleInteraction)

	if err := b.session.Open(); err != nil {
		return fmt.Errorf("discord open: %w", err)
	}

	log.Printf("discord: connected as %s", b.session.State.User.Username)

	// Register slash commands
	if len(b.commands) > 0 {
		appCmds := toApplicationCommands(b.commands)
		for _, cmd := range appCmds {
			_, err := b.session.ApplicationCommandCreate(b.session.State.User.ID, b.config.GuildID, cmd)
			if err != nil {
				log.Printf("discord: failed to register command %q: %v", cmd.Name, err)
			}
		}
	}

	return nil
}

// Stop closes the Discord session.
func (b *Bot) Stop() error {
	if b.session != nil {
		return b.session.Close()
	}
	return nil
}

// handleInteraction routes InteractionCreate events to CommandRouter handlers.
func (b *Bot) handleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	if b.router == nil {
		return
	}

	data := i.ApplicationCommandData()
	ctx := context.Background()

	// Defer immediately to avoid Discord's 3s interaction timeout.
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		log.Printf("discord: failed to defer interaction: %v", err)
	}

	// Helper to extract string option
	strOpt := func(name string) string {
		for _, opt := range data.Options {
			if opt.Name == name {
				return opt.StringValue()
			}
		}
		return ""
	}

	// Helper to extract int option with default
	intOpt := func(name string, def int) int {
		for _, opt := range data.Options {
			if opt.Name == name {
				return int(opt.IntValue())
			}
		}
		return def
	}

	var resp CommandResponse

	switch data.Name {
	case "snap":
		resp = b.router.HandleSnap(ctx, strOpt("node"), strOpt("facing"), intOpt("quality", 80))
	case "locate":
		resp = b.router.HandleLocate(ctx, strOpt("node"))
	case "status":
		resp = b.router.HandleStatus(ctx, strOpt("node"))
	case "nodes":
		resp = b.router.HandleNodes()
	case "notify":
		resp = b.router.HandleNotify(ctx, strOpt("node"), strOpt("title"), strOpt("body"))
	case "devices":
		resp = b.router.HandleDevices()
	case "approve":
		resp = b.router.HandleApprove(strOpt("request"))
	case "reject":
		resp = b.router.HandleReject(strOpt("request"))
	case "revoke":
		resp = b.router.HandleRevoke(strOpt("device"), strOpt("role"))
	default:
		resp = CommandResponse{Message: fmt.Sprintf("Unknown command: %s", data.Name)}
	}

	// Send response as a follow-up (supports attachments).
	followup := &discordgo.WebhookParams{
		Content: resp.Message,
	}

	// If we have image data, attach it as a file.
	if len(resp.ImageData) > 0 {
		followup.Files = []*discordgo.File{
			{
				Name:        "snap.png",
				ContentType: "image/png",
				Reader:      bytes.NewReader(resp.ImageData),
			},
		}
	}

	if _, err := s.FollowupMessageCreate(i.Interaction, true, followup); err != nil {
		log.Printf("discord: failed to send follow-up: %v", err)
	}
}

// SlashCommand defines a Discord slash command with options.
type SlashCommand struct {
	Name        string
	Description string
	Options     []*discordgo.ApplicationCommandOption
}

// toApplicationCommands converts SlashCommands to discordgo format.
func toApplicationCommands(cmds []SlashCommand) []*discordgo.ApplicationCommand {
	out := make([]*discordgo.ApplicationCommand, len(cmds))
	for i, cmd := range cmds {
		out[i] = &discordgo.ApplicationCommand{
			Name:        cmd.Name,
			Description: cmd.Description,
			Options:     cmd.Options,
		}
	}
	return out
}
