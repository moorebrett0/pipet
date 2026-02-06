package onboarding

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/brettsmith/pipet/internal/pet"
	"github.com/brettsmith/pipet/internal/species"
)

// Run performs interactive terminal onboarding. Returns true if onboarding completed.
func Run(petState *pet.PetState) bool {
	if petState.IsOnboarded() {
		return false
	}

	reader := bufio.NewReader(os.Stdin)

	// Hatching animation
	fmt.Println()
	printSlow("  \U0001F95A crk... crk...", 80)
	fmt.Println()
	time.Sleep(500 * time.Millisecond)

	fmt.Println("  pick a species:")
	fmt.Println()

	// Display species grid (2 columns)
	for i := 0; i < len(species.OrderedIDs); i += 2 {
		left := species.OrderedIDs[i]
		leftSp := species.Registry[left]
		col1 := fmt.Sprintf("  %d) %s %-12s", i+1, leftSp.Emoji, leftSp.Name)

		if i+1 < len(species.OrderedIDs) {
			right := species.OrderedIDs[i+1]
			rightSp := species.Registry[right]
			fmt.Printf("%s%d) %s %s\n", col1, i+2, rightSp.Emoji, rightSp.Name)
		} else {
			fmt.Println(col1)
		}
	}

	// Species selection
	fmt.Println()
	var selectedID string
	for {
		fmt.Print("  > ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		// Try as number first
		if num, err := strconv.Atoi(input); err == nil && num >= 1 && num <= len(species.OrderedIDs) {
			selectedID = species.OrderedIDs[num-1]
			break
		}

		// Try as name
		lower := strings.ToLower(input)
		for _, id := range species.OrderedIDs {
			if id == lower {
				selectedID = id
				break
			}
		}
		if selectedID != "" {
			break
		}

		fmt.Println("  hmm, pick a number 1-8 or type the species name")
	}

	sp := species.Registry[selectedID]
	fmt.Println()
	fmt.Printf("  %s ...\n", sp.Emoji)
	fmt.Println()
	time.Sleep(300 * time.Millisecond)

	// Name selection
	fmt.Println("  what's my name?")
	fmt.Println()

	var name string
	for {
		fmt.Print("  > ")
		input, _ := reader.ReadString('\n')
		name = strings.TrimSpace(input)

		if name != "" && len(name) <= 32 {
			break
		}
		fmt.Println("  pick a name (1-32 characters)")
	}

	// Set identity
	petState.SetIdentity(name, selectedID)

	// Hatching reveal
	fmt.Println()
	fmt.Printf("  %s %s\n", sp.Emoji, sp.Verbs.Greet)
	fmt.Println()
	printSlow(fmt.Sprintf("  hi. i'm %s.", name), 50)
	printSlow("  it's warm in here. i like it.", 50)
	fmt.Println()

	return true
}

// PrintStartup prints the startup checklist after onboarding.
func PrintStartup(name string, aiEnabled, discordConnected bool) {
	fmt.Println("  starting up...")

	checks := []struct {
		label string
		ok    bool
	}{
		{"monitor running", true},
		{"ai connected", aiEnabled},
		{"discord connected", discordConnected},
		{"state saved", true},
	}

	for _, c := range checks {
		time.Sleep(200 * time.Millisecond)
		mark := "\u2713"
		if !c.ok {
			mark = "\u2717"
		}
		fmt.Printf("  %s %s\n", mark, c.label)
	}

	fmt.Println()
	printSlow(fmt.Sprintf("  %s is alive. don't forget about me.", name), 40)
	fmt.Println()
}

func printSlow(text string, delayMs int) {
	for _, ch := range text {
		fmt.Print(string(ch))
		time.Sleep(time.Duration(delayMs) * time.Millisecond)
	}
	fmt.Println()
}
