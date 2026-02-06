package pet

// DetermineMood returns a mood string based on priority-ordered rules.
// Priority: Dead > Sick > Anxious > Sleepy > Hungry > Bored > Happy > Content
func DetermineMood(s Snapshot) string {
	if !s.IsAlive {
		return "dead"
	}

	// Sick: memory critical (>90%)
	if s.MemPercent > 90 {
		return "sick"
	}

	// Anxious: temperature high (>70°C)
	if s.TempC > 70 {
		return "anxious"
	}

	// Sleepy: very low energy
	if s.Energy < 20 {
		return "sleepy"
	}

	// Hungry: high hunger
	if s.Hunger > 70 {
		return "hungry"
	}

	// Bored: low happiness
	if s.Happiness < 30 {
		return "bored"
	}

	// Happy: high happiness and good stats
	if s.Happiness > 70 && s.Hunger < 40 && s.Energy > 40 {
		return "happy"
	}

	return "content"
}
