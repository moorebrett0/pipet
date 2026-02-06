package proactive

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/brettsmith/pipet/internal/discord"
	"github.com/brettsmith/pipet/internal/pet"
	"github.com/brettsmith/pipet/internal/species"
)

// MessageSender can send messages and update presence.
type MessageSender interface {
	SendMessage(channelID, text string)
	UpdatePresence(mood string)
	ChannelID() string
}

// Scheduler sends proactive messages based on pet state and time.
type Scheduler struct {
	sender   MessageSender
	petState *pet.PetState

	checkInterval    time.Duration
	morningHour      int
	boredomMinutes   int
	distressCooldown time.Duration

	mu            sync.Mutex
	lastMorning   time.Time
	lastDistress  time.Time
	lastBoredom   time.Time
	lastDeath     time.Time
	lastMilestone int
	lastMood      string
}

// Config for the proactive scheduler.
type Config struct {
	CheckInterval    time.Duration
	MorningHour      int
	BoredomMinutes   int
	DistressCooldown time.Duration
}

// New creates a proactive scheduler.
func New(sender MessageSender, petState *pet.PetState, cfg Config) *Scheduler {
	return &Scheduler{
		sender:           sender,
		petState:         petState,
		checkInterval:    cfg.CheckInterval,
		morningHour:      cfg.MorningHour,
		boredomMinutes:   cfg.BoredomMinutes,
		distressCooldown: cfg.DistressCooldown,
	}
}

// Run starts the tick loop. Blocks until context is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.check()
		}
	}
}

func (s *Scheduler) check() {
	if !s.petState.IsOnboarded() {
		return
	}

	snap := s.petState.Snapshot()
	sp := getSpecies(snap.SpeciesID)
	channelID := s.sender.ChannelID()

	// Always update presence when mood changes
	if snap.Mood != s.lastMood {
		s.lastMood = snap.Mood
		s.sender.UpdatePresence(snap.Mood)
	}

	if channelID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// Death notice
	if !snap.IsAlive && (s.lastDeath.IsZero() || now.Sub(s.lastDeath) > 24*time.Hour) {
		s.lastDeath = now
		s.sender.SendMessage(channelID, discord.TemplateDeathMessage(snap, sp))
		return
	}

	if !snap.IsAlive {
		return
	}

	// Morning check-in
	if now.Hour() == s.morningHour && now.Sub(s.lastMorning) > 20*time.Hour {
		s.lastMorning = now
		s.sender.SendMessage(channelID, discord.TemplateMorningCheckIn(snap, sp))
		return
	}

	// Distress alerts
	if reason := checkDistress(snap); reason != "" && now.Sub(s.lastDistress) > s.distressCooldown {
		s.lastDistress = now
		s.sender.SendMessage(channelID, discord.TemplateDistressAlert(snap, sp, reason))
		return
	}

	// Boredom
	boredomThreshold := time.Duration(s.boredomMinutes) * time.Minute
	if time.Since(snap.LastInteraction) > boredomThreshold && now.Sub(s.lastBoredom) > boredomThreshold {
		s.lastBoredom = now
		s.sender.SendMessage(channelID, discord.TemplateBoredomMessage(snap, sp))
		return
	}

	// Age milestones
	milestones := []int{1, 7, 30, 100, 365}
	ageDays := int(math.Floor(snap.AgeDays))
	for _, m := range milestones {
		if ageDays >= m && s.lastMilestone < m {
			s.lastMilestone = m
			s.sender.SendMessage(channelID, discord.TemplateMilestone(snap, sp, m))
			return
		}
	}
}

func checkDistress(snap pet.Snapshot) string {
	if snap.MemPercent > 90 {
		return "Memory usage is critical! I'm not feeling well..."
	}
	if snap.TempC > 75 {
		return "It's getting really hot in here! The Pi is overheating!"
	}
	if snap.CPUPercent > 90 {
		return "The CPU is maxed out! I can barely think..."
	}
	if snap.DiskPercent > 95 {
		return "Disk is almost full! I'm running out of space..."
	}
	return ""
}

func getSpecies(id string) *species.Species {
	if sp, ok := species.Registry[id]; ok {
		return sp
	}
	return species.Registry["octopus"]
}
