package pet

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// PetState holds the mutable state of the pet, protected by a mutex.
type PetState struct {
	mu sync.RWMutex

	// Identity (set during onboarding, never change)
	Name      string `json:"name"`
	SpeciesID string `json:"species_id"`

	// Stats (0–100)
	Hunger      float64 `json:"hunger"`      // 0=full, 100=starving
	Happiness   float64 `json:"happiness"`   // 0=miserable, 100=ecstatic
	Energy      float64 `json:"energy"`      // 0=exhausted, 100=energized
	Cleanliness float64 `json:"cleanliness"` // 0=filthy, 100=spotless
	Bond        float64 `json:"bond"`        // 0=stranger, 100=soulmates

	// Lifecycle
	BornAt          time.Time `json:"born_at"`
	LastInteraction time.Time `json:"last_interaction"`
	LastFed         time.Time `json:"last_fed"`
	IsAlive         bool      `json:"is_alive"`

	// System stats (written by monitor, read by mood/templates)
	CPUPercent  float64 `json:"cpu_percent"`
	MemPercent  float64 `json:"mem_percent"`
	DiskPercent float64 `json:"disk_percent"`
	TempC       float64 `json:"temp_c"`
	UptimeDays  float64 `json:"uptime_days"`
}

// Snapshot is a read-only copy of PetState for use outside the lock.
type Snapshot struct {
	Name      string
	SpeciesID string

	Hunger      float64
	Happiness   float64
	Energy      float64
	Cleanliness float64
	Bond        float64

	BornAt          time.Time
	LastInteraction time.Time
	LastFed         time.Time
	IsAlive         bool

	CPUPercent  float64
	MemPercent  float64
	DiskPercent float64
	TempC       float64
	UptimeDays  float64

	Mood string
	AgeDays float64
}

// NewPetState creates a new pet with starting stats.
func NewPetState(name, speciesID string) *PetState {
	now := time.Now()
	return &PetState{
		Name:            name,
		SpeciesID:       speciesID,
		Hunger:          20,
		Happiness:       80,
		Energy:          80,
		Cleanliness:     80,
		Bond:            10,
		BornAt:          now,
		LastInteraction: now,
		LastFed:         now,
		IsAlive:         true,
	}
}

// Snapshot copies fields under RLock and computes derived values.
func (s *PetState) Snapshot() Snapshot {
	s.mu.RLock()
	snap := Snapshot{
		Name:            s.Name,
		SpeciesID:       s.SpeciesID,
		Hunger:          s.Hunger,
		Happiness:       s.Happiness,
		Energy:          s.Energy,
		Cleanliness:     s.Cleanliness,
		Bond:            s.Bond,
		BornAt:          s.BornAt,
		LastInteraction: s.LastInteraction,
		LastFed:         s.LastFed,
		IsAlive:         s.IsAlive,
		CPUPercent:      s.CPUPercent,
		MemPercent:      s.MemPercent,
		DiskPercent:     s.DiskPercent,
		TempC:           s.TempC,
		UptimeDays:      s.UptimeDays,
	}
	s.mu.RUnlock()

	snap.Mood = DetermineMood(snap)
	snap.AgeDays = time.Since(snap.BornAt).Hours() / 24
	return snap
}

// IsOnboarded returns true if the pet has been set up.
func (s *PetState) IsOnboarded() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Name != "" && s.SpeciesID != ""
}

// SetIdentity sets name and species during onboarding.
func (s *PetState) SetIdentity(name, speciesID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Name = name
	s.SpeciesID = speciesID
	now := time.Now()
	s.BornAt = now
	s.LastInteraction = now
	s.LastFed = now
	s.IsAlive = true
	s.Hunger = 20
	s.Happiness = 80
	s.Energy = 80
	s.Cleanliness = 80
	s.Bond = 10
}

// bumpBond increases bond on interaction (diminishing returns at high levels).
func (s *PetState) bumpBond() {
	gain := 2.0
	if s.Bond > 50 {
		gain = 1.0
	}
	if s.Bond > 80 {
		gain = 0.5
	}
	s.Bond = clamp(s.Bond + gain)
}

// Feed decreases hunger and records feeding time.
func (s *PetState) Feed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Hunger = clamp(s.Hunger - 30)
	s.Happiness = clamp(s.Happiness + 5)
	s.LastFed = time.Now()
	s.LastInteraction = time.Now()
	s.bumpBond()
}

// Play increases happiness and decreases energy.
func (s *PetState) Play() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Happiness = clamp(s.Happiness + 20)
	s.Energy = clamp(s.Energy - 10)
	s.Hunger = clamp(s.Hunger + 5)
	s.LastInteraction = time.Now()
	s.bumpBond()
}

// Pet increases happiness slightly (affection).
func (s *PetState) Pet() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Happiness = clamp(s.Happiness + 10)
	s.LastInteraction = time.Now()
	s.bumpBond()
}

// TouchInteraction records that the user interacted without stat changes.
func (s *PetState) TouchInteraction() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastInteraction = time.Now()
	s.bumpBond()
}

// ApplySystemStats maps system metrics to pet stats.
func (s *PetState) ApplySystemStats(cpu, mem, disk, tempC, uptimeDays float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.CPUPercent = cpu
	s.MemPercent = mem
	s.DiskPercent = disk
	s.TempC = tempC
	s.UptimeDays = uptimeDays

	// Map system → pet stats
	s.Hunger = clamp(cpu)                          // CPU % → hunger
	s.Cleanliness = clamp(100 - disk)              // disk usage → cleanliness
	s.Energy = clamp(100 - (uptimeDays * 14))      // uptime → energy drain

	// Happiness decays per hour since last interaction
	hoursSince := time.Since(s.LastInteraction).Hours()
	s.Happiness = clamp(s.Happiness - hoursSince*0.1) // gentle decay per update cycle

	// Bond decays slowly without interaction (0.5/hour)
	s.Bond = clamp(s.Bond - hoursSince*0.05)

	// Death: sustained critical state
	if s.Hunger >= 95 && s.MemPercent >= 95 && s.Energy <= 5 {
		s.IsAlive = false
	}
}

// Kill marks the pet as dead.
func (s *PetState) Kill() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IsAlive = false
}

// Revive resets the pet to alive with decent stats.
func (s *PetState) Revive() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IsAlive = true
	s.Hunger = 20
	s.Happiness = 50
	s.Energy = 50
	s.Cleanliness = 50
	s.Bond = clamp(s.Bond * 0.5) // bond persists partially through death
	s.LastInteraction = time.Now()
}

// Save writes the state to disk atomically (write tmp, then rename).
func (s *PetState) Save(path string) error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write tmp state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}

// Load reads state from disk. Returns a new empty state if file doesn't exist.
func Load(path string) (*PetState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &PetState{}, nil
		}
		return nil, fmt.Errorf("read state: %w", err)
	}

	var state PetState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}
	return &state, nil
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
