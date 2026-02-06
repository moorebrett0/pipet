package discord

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"

	"github.com/brettsmith/pipet/internal/pet"
	"github.com/brettsmith/pipet/internal/species"
)

// Bot wraps the Discord session and manages slash commands, messages, and presence.
type Bot struct {
	session   *discordgo.Session
	channelID string
	ownerIDs  map[string]bool

	allowSpectatorPet bool
	useThreads        bool

	petState *pet.PetState
	router   *Router

	mu     sync.Mutex
	cancel context.CancelFunc
}

// NewBot creates and configures a Discord bot (does not connect yet).
func NewBot(token, channelID string, ownerIDs []string, allowSpectatorPet, useThreads bool) (*Bot, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("invalid bot token: %w", err)
	}

	session.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentMessageContent |
		discordgo.IntentsGuilds

	owners := make(map[string]bool, len(ownerIDs))
	for _, id := range ownerIDs {
		owners[id] = true
	}

	return &Bot{
		session:           session,
		channelID:         channelID,
		ownerIDs:          owners,
		allowSpectatorPet: allowSpectatorPet,
		useThreads:        useThreads,
	}, nil
}

// SetRouter wires the router to handle messages and interactions.
func (b *Bot) SetRouter(r *Router) {
	b.router = r
	b.session.AddHandler(b.onMessageCreate)
	b.session.AddHandler(b.onInteractionCreate)
	b.session.AddHandler(b.onReady)
}

// Start opens the Discord connection and registers slash commands.
// Blocks until context is cancelled.
func (b *Bot) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	b.mu.Lock()
	b.cancel = cancel
	b.mu.Unlock()

	if err := b.session.Open(); err != nil {
		slog.Error("discord: failed to open session", "err", err)
		cancel()
		return
	}

	slog.Info("discord: connected", "user", b.session.State.User.Username)

	// Register slash commands
	b.registerCommands()

	// Wait for shutdown
	<-ctx.Done()
	slog.Info("discord: shutting down")
	b.session.Close()
}

// ChannelID returns the configured channel ID.
func (b *Bot) ChannelID() string {
	return b.channelID
}

// SendMessage sends a text message to a channel.
func (b *Bot) SendMessage(channelID, text string) {
	if text == "" {
		return
	}
	if _, err := b.session.ChannelMessageSend(channelID, text); err != nil {
		slog.Error("discord: send message failed", "err", err)
	}
}

// SendEmbed sends an embed to a channel.
func (b *Bot) SendEmbed(channelID string, embed *discordgo.MessageEmbed) {
	if _, err := b.session.ChannelMessageSendEmbed(channelID, embed); err != nil {
		slog.Error("discord: send embed failed", "err", err)
	}
}

// CreateThread creates a thread from a message and returns the thread channel ID.
func (b *Bot) CreateThread(channelID, messageID, name string) (string, error) {
	thread, err := b.session.MessageThreadStartComplex(channelID, messageID, &discordgo.ThreadStart{
		Name:                name,
		AutoArchiveDuration: 60,
	})
	if err != nil {
		return "", fmt.Errorf("create thread: %w", err)
	}
	return thread.ID, nil
}

// UpdatePresence sets the bot's Discord status based on pet mood.
func (b *Bot) UpdatePresence(mood string) {
	status, activity := moodToPresence(mood)
	err := b.session.UpdateStatusComplex(discordgo.UpdateStatusData{
		Status: status,
		Activities: []*discordgo.Activity{
			{
				Name: activity,
				Type: discordgo.ActivityTypeCustom,
			},
		},
	})
	if err != nil {
		slog.Debug("discord: update presence failed", "err", err)
	}
}

// IsOwner checks if a user ID is in the owner list.
func (b *Bot) IsOwner(userID string) bool {
	return b.ownerIDs[userID]
}

// SendIntroduction posts the pet's first message in the channel.
func (b *Bot) SendIntroduction(petState *pet.PetState) {
	snap := petState.Snapshot()
	sp := getSpecies(snap.SpeciesID)
	msg := fmt.Sprintf("%s hey everyone. i'm %s.\n   just hatched on a little pi zero.\n   %.0f°C in here. cozy.",
		sp.Emoji, snap.Name, snap.TempC)
	b.SendMessage(b.channelID, msg)
}

func (b *Bot) onReady(s *discordgo.Session, r *discordgo.Ready) {
	slog.Info("discord: ready", "user", r.User.Username, "guilds", len(r.Guilds))
}

// BotUserID returns the bot's own user ID.
func (b *Bot) BotUserID() string {
	if b.session.State != nil && b.session.State.User != nil {
		return b.session.State.User.ID
	}
	return ""
}

// IsMentioned checks if the bot was @mentioned in the message.
func (b *Bot) IsMentioned(m *discordgo.MessageCreate) bool {
	for _, u := range m.Mentions {
		if u.ID == b.BotUserID() {
			return true
		}
	}
	return false
}

// StripMention removes the bot's @mention from message text.
func (b *Bot) StripMention(text string) string {
	botID := b.BotUserID()
	// Discord mentions look like <@123456> or <@!123456>
	text = strings.ReplaceAll(text, "<@"+botID+">", "")
	text = strings.ReplaceAll(text, "<@!"+botID+">", "")
	return strings.TrimSpace(text)
}

func (b *Bot) onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore own messages only (not other bots)
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Only respond in the configured channel
	if m.ChannelID != b.channelID {
		return
	}

	if b.router != nil {
		b.router.HandleMessage(m)
	}
}

func (b *Bot) onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	if b.router != nil {
		b.router.HandleInteraction(i)
	}
}

func (b *Bot) registerCommands() {
	appID := b.session.State.User.ID
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "status",
			Description: "Check your pet's stats and mood",
		},
		{
			Name:        "pet",
			Description: "Give your pet some affection",
		},
		{
			Name:        "feed",
			Description: "Run cleanup/maintenance tasks on the Pi",
		},
		{
			Name:        "heal",
			Description: "Diagnose and fix resource issues on the Pi",
		},
		{
			Name:        "play",
			Description: "Ask your pet to do something fun",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "activity",
					Description: "What to do",
					Required:    false,
				},
			},
		},
		{
			Name:        "help",
			Description: "Show available commands",
		},
		{
			Name:        "revive",
			Description: "Bring your pet back to life",
		},
		{
			Name:        "mood",
			Description: "Check your pet's current mood",
		},
	}

	for _, cmd := range commands {
		if _, err := b.session.ApplicationCommandCreate(appID, "", cmd); err != nil {
			slog.Error("discord: failed to register command", "cmd", cmd.Name, "err", err)
		} else {
			slog.Info("discord: registered command", "cmd", cmd.Name)
		}
	}
}

func moodToPresence(mood string) (status, activity string) {
	switch mood {
	case "happy":
		return "online", "feeling great!"
	case "content":
		return "online", "just vibing"
	case "bored":
		return "idle", "anyone there?"
	case "hungry":
		return "idle", "getting hungry..."
	case "sleepy":
		return "idle", "zzz"
	case "anxious":
		return "dnd", "CPU is spiking..."
	case "sick":
		return "dnd", "need help..."
	case "dead":
		return "invisible", ""
	default:
		return "online", "just vibing"
	}
}

func getSpecies(id string) *species.Species {
	if sp, ok := species.Registry[id]; ok {
		return sp
	}
	return species.Registry["octopus"]
}
