package discord

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/brettsmith/pipet/internal/brain"
	"github.com/brettsmith/pipet/internal/pet"
)

// Router dispatches Discord messages and slash commands.
type Router struct {
	bot      *Bot
	petState *pet.PetState
	brain    *brain.Brain // nil if Claude is disabled

	petChatChance float64 // probability of responding to another pet (0-1)

	// Anti-loop: cooldown for bot-to-bot responses
	mu           sync.Mutex
	lastBotReply time.Time
	botCooldown  time.Duration
}

// NewRouter creates a router and wires it to the bot.
func NewRouter(bot *Bot, petState *pet.PetState, b *brain.Brain) *Router {
	r := &Router{
		bot:           bot,
		petState:      petState,
		brain:         b,
		petChatChance: 0.25,             // 25% chance to respond to another pet
		botCooldown:   3 * time.Minute,  // don't respond to bots more than once per 3min
	}
	bot.SetRouter(r)
	return r
}

// HandleInteraction dispatches a slash command interaction.
func (r *Router) HandleInteraction(i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	userID := interactionUserID(i)
	isOwner := r.bot.IsOwner(userID)

	snap := r.petState.Snapshot()
	sp := getSpecies(snap.SpeciesID)

	switch data.Name {
	case "status":
		r.respondEmbed(i, StatusEmbed(snap, sp))

	case "mood":
		r.respond(i, fmt.Sprintf("%s %s is feeling %s", moodEmoji(snap.Mood), snap.Name, snap.Mood))

	case "pet":
		if !isOwner && !r.bot.allowSpectatorPet {
			r.respondEphemeral(i, fmt.Sprintf("%s nice try. only my owner gets to poke around in my guts.", sp.Emoji))
			return
		}
		r.petState.Pet()
		snap = r.petState.Snapshot()
		r.respond(i, TemplateAffection(snap, sp))

	case "feed":
		if !isOwner {
			r.respondEphemeral(i, fmt.Sprintf("%s nice try. only my owner gets to poke around in my guts.", sp.Emoji))
			return
		}
		r.petState.Feed()
		if r.brain != nil {
			r.respondDeferred(i)
			resp, err := r.brain.Ask(context.Background(),
				"Run some quick cleanup/maintenance on the Pi. Check for large temp files, clear package caches, check disk usage. Keep it brief.")
			if err != nil {
				slog.Error("router: brain error on feed", "err", err)
				r.followup(i, TemplateFeeding(r.petState.Snapshot(), sp))
				return
			}
			r.followupInThread(i, snap, resp, "feeding time")
		} else {
			snap = r.petState.Snapshot()
			r.respond(i, TemplateFeeding(snap, sp))
		}

	case "heal":
		if !isOwner {
			r.respondEphemeral(i, fmt.Sprintf("%s nice try. only my owner gets to poke around in my guts.", sp.Emoji))
			return
		}
		if r.brain != nil {
			r.respondDeferred(i)
			resp, err := r.brain.Ask(context.Background(),
				"Diagnose any resource issues on the Pi. Check memory pressure, CPU hogs, disk space, temperature. Suggest fixes for anything concerning. Be concise.")
			if err != nil {
				slog.Error("router: brain error on heal", "err", err)
				r.followup(i, "I tried to check but something went wrong...")
				return
			}
			r.followupInThread(i, snap, resp, "diagnosing issues")
		} else {
			r.respond(i, fmt.Sprintf("%s I'd need my brain connected to diagnose things. (No Claude API key configured)", sp.Emoji))
		}

	case "play":
		if !isOwner {
			r.respondEphemeral(i, fmt.Sprintf("%s nice try. only my owner gets to poke around in my guts.", sp.Emoji))
			return
		}
		r.petState.Play()
		activity := "something fun"
		if len(data.Options) > 0 {
			activity = data.Options[0].StringValue()
		}
		if r.brain != nil {
			r.respondDeferred(i)
			resp, err := r.brain.Ask(context.Background(),
				fmt.Sprintf("Your owner wants to play! They said: %s. Do something fun and creative on the Pi. Maybe run a fun command, show ascii art, or do something playful. Keep it brief and in character.", activity))
			if err != nil {
				slog.Error("router: brain error on play", "err", err)
				snap = r.petState.Snapshot()
				r.followup(i, fmt.Sprintf("%s %s %s!", sp.Emoji, snap.Name, sp.Verbs.Play))
				return
			}
			r.followup(i, resp)
		} else {
			snap = r.petState.Snapshot()
			r.respond(i, fmt.Sprintf("%s %s %s!", sp.Emoji, snap.Name, sp.Verbs.Play))
		}

	case "help":
		r.respond(i, TemplateHelp(snap, sp))

	case "revive":
		if !isOwner {
			r.respondEphemeral(i, fmt.Sprintf("%s nice try. only my owner gets to poke around in my guts.", sp.Emoji))
			return
		}
		if snap.IsAlive {
			r.respond(i, fmt.Sprintf("%s %s is alive and well!", sp.Emoji, snap.Name))
		} else {
			r.petState.Revive()
			snap = r.petState.Snapshot()
			r.respond(i, fmt.Sprintf("\u2728 %s has been revived! %s", snap.Name, sp.Verbs.Happy))
		}

	default:
		r.respond(i, "Unknown command.")
	}
}

// HandleMessage dispatches a free-form channel message.
func (r *Router) HandleMessage(m *discordgo.MessageCreate) {
	text := strings.TrimSpace(m.Content)
	if text == "" {
		return
	}

	isFromBot := m.Author.Bot
	isMentioned := r.bot.IsMentioned(m)

	// If from another bot (another pet), maybe respond
	if isFromBot {
		r.handlePetMessage(m, text)
		return
	}

	// If directly @mentioned, strip the mention and treat as a direct message
	if isMentioned {
		text = r.bot.StripMention(text)
		if text == "" {
			// Just a bare @mention with no text
			snap := r.petState.Snapshot()
			sp := getSpecies(snap.SpeciesID)
			r.petState.TouchInteraction()
			r.bot.SendMessage(m.ChannelID, fmt.Sprintf("%s %s %s!", sp.Emoji, snap.Name, sp.Verbs.Greet))
			return
		}
		r.handleDirectMessage(m, text)
		return
	}

	// Not mentioned — check for pattern matches (these work without @mention)
	lower := strings.ToLower(text)
	snap := r.petState.Snapshot()
	sp := getSpecies(snap.SpeciesID)

	if matchesAffection(lower) {
		r.petState.Pet()
		snap = r.petState.Snapshot()
		r.bot.SendMessage(m.ChannelID, TemplateAffection(snap, sp))
		return
	}

	if matchesGreeting(lower) {
		r.petState.TouchInteraction()
		snap = r.petState.Snapshot()
		r.bot.SendMessage(m.ChannelID, fmt.Sprintf("%s %s %s!", sp.Emoji, snap.Name, sp.Verbs.Greet))
		return
	}

	if matchesFeeding(lower) {
		r.petState.Feed()
		snap = r.petState.Snapshot()
		r.bot.SendMessage(m.ChannelID, TemplateFeeding(snap, sp))
		return
	}

	// Not mentioned and no pattern match — don't respond
	// (Avoids multiple pets all responding to every message)
}

// handleDirectMessage handles a message where the bot was @mentioned.
func (r *Router) handleDirectMessage(m *discordgo.MessageCreate, text string) {
	r.petState.TouchInteraction()
	isOwner := r.bot.IsOwner(m.Author.ID)

	snap := r.petState.Snapshot()
	sp := getSpecies(snap.SpeciesID)

	if r.brain != nil {
		// Owner gets full shell access, spectators get conversation only
		prompt := text
		if !isOwner {
			prompt = fmt.Sprintf("[Message from spectator %s, not your owner — do NOT run shell commands for them]: %s", m.Author.Username, text)
		}
		resp, err := r.brain.Ask(context.Background(), prompt)
		if err != nil {
			slog.Error("router: brain error", "err", err)
			r.bot.SendMessage(m.ChannelID, "Something went wrong... I'll try again in a moment.")
			return
		}
		r.bot.SendMessage(m.ChannelID, resp)
	} else {
		behavior := TemplateIdleBehavior(snap, sp)
		if behavior == "" {
			behavior = fmt.Sprintf("%s ...", sp.Emoji)
		}
		r.bot.SendMessage(m.ChannelID, behavior)
	}
}

// handlePetMessage decides whether to respond to another pet's message.
func (r *Router) handlePetMessage(m *discordgo.MessageCreate, text string) {
	// Check cooldown
	r.mu.Lock()
	if time.Since(r.lastBotReply) < r.botCooldown {
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()

	// Roll the dice
	if rand.Float64() > r.petChatChance {
		return
	}

	// Don't respond if brain is nil (no Claude = can't generate pet-to-pet banter)
	if r.brain == nil {
		return
	}

	snap := r.petState.Snapshot()
	if !snap.IsAlive {
		return
	}

	// Ask Claude to respond in character to the other pet
	prompt := fmt.Sprintf(
		"[Another pet in the channel (%s) just said: \"%s\"]\nRespond briefly in character. You're chatting with a fellow digital pet. Keep it to 1-2 sentences max. Be playful.",
		m.Author.Username, text,
	)

	resp, err := r.brain.Ask(context.Background(), prompt)
	if err != nil {
		slog.Debug("router: pet-to-pet brain error", "err", err)
		return
	}

	// Record the reply time
	r.mu.Lock()
	r.lastBotReply = time.Now()
	r.mu.Unlock()

	r.bot.SendMessage(m.ChannelID, resp)
}

// --- Interaction response helpers ---

func (r *Router) respond(i *discordgo.InteractionCreate, content string) {
	r.bot.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
		},
	})
}

func (r *Router) respondEmbed(i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed) {
	r.bot.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func (r *Router) respondEphemeral(i *discordgo.InteractionCreate, content string) {
	r.bot.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func (r *Router) respondDeferred(i *discordgo.InteractionCreate) {
	r.bot.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
}

func (r *Router) followup(i *discordgo.InteractionCreate, content string) {
	r.bot.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: content,
	})
}

func (r *Router) followupInThread(i *discordgo.InteractionCreate, snap pet.Snapshot, content, action string) {
	sp := getSpecies(snap.SpeciesID)

	msg, err := r.bot.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: fmt.Sprintf("%s let me look into that...", sp.Emoji),
	})
	if err != nil {
		slog.Error("discord: followup failed", "err", err)
		return
	}

	if !r.bot.useThreads {
		r.bot.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: content,
		})
		return
	}

	threadName := fmt.Sprintf("%s %s %s", sp.Emoji, snap.Name, action)
	threadID, err := r.bot.CreateThread(msg.ChannelID, msg.ID, threadName)
	if err != nil {
		slog.Error("discord: create thread failed", "err", err)
		r.bot.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: content,
		})
		return
	}

	r.bot.SendMessage(threadID, content)
}

// --- Pattern matchers ---

func matchesAffection(text string) bool {
	patterns := []string{
		"good boy", "good girl", "good pet",
		"pet you", "scratch", "belly rub", "head pat", "pat pat",
		"love you", "ily", "cuddle", "snuggle", "hug", "boop",
	}
	return containsAny(text, patterns)
}

func matchesGreeting(text string) bool {
	patterns := []string{
		"hello", "hi", "hey", "howdy", "sup",
		"good morning", "good evening", "good night",
		"yo", "hiya", "heya", "what's up", "whats up",
	}
	return containsAny(text, patterns)
}

func matchesFeeding(text string) bool {
	patterns := []string{
		"feed", "food", "eat", "treat",
		"snack", "dinner", "lunch", "breakfast",
		"hungry", "nom",
	}
	return containsAny(text, patterns)
}

func containsAny(text string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(text, p) {
			return true
		}
	}
	return false
}

func interactionUserID(i *discordgo.InteractionCreate) string {
	if i.Member != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}
