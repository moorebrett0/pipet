package discord

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/moorebrett0/pipet/internal/pet"
	"github.com/moorebrett0/pipet/internal/species"
)

// progressBar renders a visual bar like ████████░░ 78%
func progressBar(value float64, width int) string {
	filled := int(value / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	empty := width - filled
	return fmt.Sprintf("%s%s %.0f%%", strings.Repeat("\u2588", filled), strings.Repeat("\u2591", empty), value)
}

// moodColor returns a Discord embed color for the mood.
func moodColor(mood string) int {
	switch mood {
	case "happy":
		return 0x57F287 // green
	case "content":
		return 0x5865F2 // blurple
	case "bored":
		return 0xFEE75C // yellow
	case "hungry":
		return 0xEB459E // fuchsia
	case "sleepy":
		return 0x99AAB5 // grey
	case "anxious":
		return 0xED4245 // red
	case "sick":
		return 0xED4245 // red
	case "dead":
		return 0x23272A // dark
	default:
		return 0x5865F2
	}
}

// StatusEmbed builds a rich embed for /status.
func StatusEmbed(snap pet.Snapshot, sp *species.Species) *discordgo.MessageEmbed {
	alive := "alive"
	if !snap.IsAlive {
		alive = "DEAD"
	}

	stats := fmt.Sprintf(
		"happiness %s\nenergy    %s\nhunger    %s\nclean     %s\nbond      %s",
		progressBar(snap.Happiness, 10),
		progressBar(snap.Energy, 10),
		progressBar(snap.Hunger, 10),
		progressBar(snap.Cleanliness, 10),
		progressBar(snap.Bond, 10),
	)

	system := fmt.Sprintf(
		"\U0001F5A5 CPU %.1f%% | \U0001F321 %.1f\u00B0C\n\U0001F4BE %.0f%% mem | \U0001F4BF %.0f%% disk\n\u23F1 uptime %.1fd",
		snap.CPUPercent, snap.TempC,
		snap.MemPercent, snap.DiskPercent,
		snap.UptimeDays,
	)

	return &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s %s", sp.Emoji, snap.Name),
		Description: fmt.Sprintf("mood: %s %s | status: %s", moodEmoji(snap.Mood), snap.Mood, alive),
		Color:       moodColor(snap.Mood),
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Stats", Value: "```\n" + stats + "\n```", Inline: false},
			{Name: "System", Value: system, Inline: false},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("age: %.1f days", snap.AgeDays),
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}
}

func TemplateAffection(snap pet.Snapshot, sp *species.Species) string {
	parts := []string{sp.Body.Head, sp.Body.Back, sp.Body.Extra}
	part := parts[rand.Intn(len(parts))]
	return fmt.Sprintf("%s You scratch %s's %s. %s %s!",
		sp.Emoji, snap.Name, part, snap.Name, sp.Verbs.Happy)
}

func TemplateFeeding(snap pet.Snapshot, sp *species.Species) string {
	return fmt.Sprintf("%s %s %s! Hunger is now at %.0f%%.",
		sp.Emoji, snap.Name, sp.Verbs.Eat, snap.Hunger)
}

func TemplateIdleBehavior(snap pet.Snapshot, sp *species.Species) string {
	if len(sp.IdleBehaviors) == 0 {
		return ""
	}
	behavior := sp.IdleBehaviors[rand.Intn(len(sp.IdleBehaviors))]
	return fmt.Sprintf("%s %s %s.", sp.Emoji, snap.Name, behavior)
}

func TemplateMorningCheckIn(snap pet.Snapshot, sp *species.Species) string {
	return fmt.Sprintf("%s Good morning! %s %s\nMood: %s %s | Hunger: %.0f%%",
		sp.Emoji, snap.Name, sp.Verbs.Greet,
		moodEmoji(snap.Mood), snap.Mood, snap.Hunger)
}

func TemplateDistressAlert(snap pet.Snapshot, sp *species.Species, reason string) string {
	return fmt.Sprintf("\u26A0\uFE0F %s %s %s!\n%s",
		sp.Emoji, snap.Name, sp.Verbs.Distress, reason)
}

func TemplateBoredomMessage(snap pet.Snapshot, sp *species.Species) string {
	behavior := ""
	if len(sp.IdleBehaviors) > 0 {
		behavior = sp.IdleBehaviors[rand.Intn(len(sp.IdleBehaviors))]
	}
	return fmt.Sprintf("%s %s is getting bored... %s\nCome say hi!",
		sp.Emoji, snap.Name, behavior)
}

func TemplateDeathMessage(snap pet.Snapshot, sp *species.Species) string {
	return fmt.Sprintf("\U0001F480 %s has passed away...\nThe system was under too much stress. Use /revive to bring them back.",
		snap.Name)
}

func TemplateMilestone(snap pet.Snapshot, sp *species.Species, days int) string {
	return fmt.Sprintf("\U0001F389 %s %s is %d days old today! %s",
		sp.Emoji, snap.Name, days, sp.Verbs.Happy)
}

func TemplateHelp(snap pet.Snapshot, sp *species.Species) string {
	name := snap.Name
	if name == "" {
		name = "your pet"
	}
	return fmt.Sprintf("**PiPet Commands**\n\n"+
		"`/status` — See %s's stats and mood\n"+
		"`/pet` — Give %s some love\n"+
		"`/feed` — Run cleanup/maintenance\n"+
		"`/heal` — Diagnose and fix issues\n"+
		"`/play` — Ask %s to do something fun\n"+
		"`/mood` — Current mood\n"+
		"`/revive` — Bring %s back if they die\n"+
		"`/help` — This message\n\n"+
		"Or just talk to %s in this channel!", name, name, name, name, name)
}

func moodEmoji(mood string) string {
	switch mood {
	case "happy":
		return "\U0001F60A"
	case "content":
		return "\U0001F60C"
	case "bored":
		return "\U0001F612"
	case "hungry":
		return "\U0001F60B"
	case "sleepy":
		return "\U0001F634"
	case "anxious":
		return "\U0001F630"
	case "sick":
		return "\U0001F912"
	case "dead":
		return "\U0001F480"
	default:
		return "\U0001F610"
	}
}
