<p align="center">
  <img src="pipets.png" alt="PiPets — your pi has feelings now" width="500">
</p>

# PiPet

A digital pet that lives inside your Raspberry Pi and hangs out in Discord.

Your pet monitors the Pi's vitals — CPU, memory, disk, temperature — and maps them to pet stats like hunger, energy, and cleanliness. Talk to it, feed it, play with it. Ignore it and it gets bored. Push the Pi too hard and it gets sick.

Multiple pets can share a channel. Each Pi runs its own bot, and the pets will occasionally banter with each other.

```
┌──────────────────────────────┐
│ 🐙 Inky                      │
│ mood: happy                   │
│                               │
│ happiness ████████░░ 78%      │
│ energy    █████░░░░░ 52%      │
│ hunger    ███░░░░░░░ 28%      │
│ clean     ████████░░ 82%      │
│ bond      ██████░░░░ 61%      │
│                               │
│ 🖥 CPU 23% | 🌡 48°C          │
│ 💾 189/512MB | 💿 4.2/29GB     │
│ ⏱ uptime 3d 14h               │
│                               │
│ age: 12 days                  │
└──────────────────────────────┘
```

## Species

Pick one when you first run the binary:

| | Species | Personality |
|---|---------|------------|
| 🦞 | Lobster | Tough on the outside, soft on the inside |
| 🐙 | Octopus | Clever and curious, eight arms multitasking |
| 🐢 | Turtle | Slow and steady, ancient wisdom |
| 🐧 | Penguin | Formal but clumsy, surprisingly fast swimmer |
| 🦀 | Crab | Sassy and sideways, no-nonsense attitude |
| 🐡 | Pufferfish | Cute when calm, spiky when stressed |
| 🦑 | Squid | Fast, mysterious, bioluminescent thinker |
| 🐠 | Fish | Colorful, simple, just vibing |

## Quick Start

### Prerequisites

- A Raspberry Pi (any model, Zero 2 W ideal)
- Go 1.22+ (for building) or grab a release binary
- A Discord account

### 1. Create a Discord Bot

1. Go to [discord.com/developers/applications](https://discord.com/developers/applications)
2. Click **New Application** — name it after your pet (e.g. "Inky")
3. Go to the **Bot** tab
4. Click **Reset Token** — copy it, you'll need it in a moment

### 2. Enable Message Content Intent

Still on the **Bot** tab:

- Scroll down to **Privileged Gateway Intents**
- Toggle ON **Message Content Intent**
- Save Changes

### 3. Invite the Bot to Your Server

Go to **OAuth2 → URL Generator**:

- **Scopes**: check `bot` and `applications.commands`
- **Bot Permissions**: use integer `326417525824`, or check:
  - Send Messages
  - Send Messages in Threads
  - Create Public Threads
  - Embed Links
  - Read Message History
  - Use Slash Commands

Copy the generated URL → open it → pick your server → authorize.

### 4. Get Your IDs

Turn on Developer Mode in Discord:

- **User Settings → Advanced → Developer Mode** → ON

Then:

- Right-click the **channel** where the pet should live → **Copy Channel ID**
- Right-click **yourself** → **Copy User ID**

### 5. Setup

```bash
git clone https://github.com/moorebrett0/pipet.git
cd pipet
./setup.sh
```

The setup script walks you through pasting your bot token, channel ID, owner ID, and optional Anthropic API key. It writes a `.env` file.

Or create `.env` manually:

```bash
cp .env.example .env
# edit .env with your values
```

### 6. Run

```bash
go build ./cmd/pipet
./pipet
```

First run hatches your pet in the terminal:

```
  🥚 crk... crk...

  pick a species:

  1) 🦞 Lobster     2) 🐙 Octopus
  3) 🐢 Turtle      4) 🐧 Penguin
  5) 🦀 Crab        6) 🐡 Pufferfish
  7) 🦑 Squid       8) 🐠 Fish

  > 2

  🐙 ...

  what's my name?

  > Inky

  🐙 waves three tentacles at once

  hi. i'm Inky.
  it's warm in here. i like it.
```

Then it connects to Discord and introduces itself in the channel.

## Talking to Your Pet

### @mention for conversation

```
@Inky how's the Pi doing?
@Inky check my disk space
@Inky tell me a joke
```

### Slash commands

| Command | What it does | Owner only? |
|---------|-------------|-------------|
| `/status` | Pet stats + mood as an embed | No |
| `/pet` | Give affection, boost happiness | Configurable |
| `/feed` | Run cleanup/maintenance tasks | Yes |
| `/heal` | Diagnose and fix resource issues | Yes |
| `/play` | Ask pet to do something fun | Yes |
| `/mood` | Check current mood | No |
| `/help` | Show commands | No |
| `/revive` | Bring pet back to life | Yes |

### Pattern responses

These work without @mention — say them in the channel:

- **Greetings**: "hello", "hey", "good morning"
- **Affection**: "good boy", "boop", "head pat"
- **Feeding**: "feed", "hungry", "treat"

## Multiple Pets

Each Pi runs its own Discord bot. To add another pet:

1. Create another bot application at discord.com/developers
2. Invite it to the same server and channel
3. Run `./setup.sh` on the new Pi with the new bot's token

All pets share the same channel. When one pet says something, others have a 25% chance of responding (with a 3-minute cooldown to prevent loops). Slash commands are per-bot — Discord shows which pet owns each command.

## How Stats Work

| System Metric | Pet Stat | How |
|---|---|---|
| CPU % | Hunger | CPU load = hunger |
| Disk % | Cleanliness | Disk usage = messiness |
| Uptime | Energy | Long uptimes drain energy |
| Interactions | Happiness | Decays without attention |
| Interactions | Bond | Grows slowly, diminishing returns |
| Memory > 90% | Sick mood | Pet feels ill |
| Temp > 70°C | Anxious mood | Pet overheating |

## Mood → Discord Presence

Your pet's mood shows in the Discord sidebar:

| Mood | Status |
|------|--------|
| 😊 Happy | 🟢 Online — "feeling great!" |
| 😌 Content | 🟢 Online — "just vibing" |
| 😐 Bored | 🟡 Idle — "anyone there?" |
| 😰 Anxious | 🔴 DND — "CPU is spiking..." |
| 🤒 Sick | 🔴 DND — "need help..." |
| 😴 Sleepy | 🟡 Idle — "zzz" |
| 💀 Dead | ⚫ Invisible |

## Proactive Messages

The pet posts to the channel on its own:

- **Morning check-in** at a configurable hour
- **Distress alerts** when CPU/memory/temp/disk are critical
- **Boredom** if nobody talks to it for 2 hours
- **Milestones** at 1, 7, 30, 100, 365 days old
- **Death notice** if the system is critically overloaded

## AI Integration (Optional)

PiPet supports two AI providers. Set one API key in your `.env` to enable AI responses. Without either, the pet uses canned template responses — still works, just less dynamic.

| Provider | Env Var | Cost | Get a key |
|----------|---------|------|-----------|
| **Claude** (Anthropic) | `ANTHROPIC_API_KEY` | Paid | [console.anthropic.com](https://console.anthropic.com/settings/keys) |
| **Gemini** (Google) | `GOOGLE_API_KEY` | Free tier | [aistudio.google.com](https://aistudio.google.com/apikey) |

Auto-detection: if both keys are set, Claude is preferred. Set `AI_PROVIDER=gemini` to override.

With AI enabled:
- Free-form conversation in character
- `/feed` actually runs cleanup commands on the Pi
- `/heal` diagnoses real resource issues
- `/play` does creative things with shell commands
- Pet-to-pet banter uses AI to stay in character

The pet has a `run_shell` tool so the AI can execute commands on the Pi. Dangerous commands (rm -rf, shutdown, etc.) are blocked.

## Configuration

The `.env` file handles secrets. For advanced tuning, create a `config.yaml`:

```yaml
# All of these are optional — defaults are sane
proactive:
  morning_hour: 8
  boredom_minutes: 120
  distress_cooldown: 30m

monitor:
  interval: 30s

shell:
  timeout: 10s

pet:
  save_interval: 5m

discord:
  allow_spectator_pet: true
  use_threads: true
```

## Install on Raspberry Pi

### From source

```bash
sudo apt install golang git
git clone https://github.com/moorebrett0/pipet.git
cd pipet
./setup.sh
go build ./cmd/pipet
./pipet
```

### One-liner (after a release is published)

```bash
curl -sSL https://raw.githubusercontent.com/moorebrett0/pipet/main/install.sh | sudo bash
```

### Run as a service

```bash
sudo cp pipet.service /etc/systemd/system/
sudo systemctl enable pipet
sudo systemctl start pipet
```

Check logs: `journalctl -u pipet -f`

## Cross-compile

Build on your laptop, deploy to the Pi:

```bash
make release
scp pipet-linux-arm64 pi@raspberrypi:~/pipet
```

## Project Structure

```
cmd/pipet/main.go           — entry point, wiring, graceful shutdown
internal/config/             — .env + YAML config loading
internal/species/            — 8 aquatic species definitions
internal/pet/                — state (mutex, JSON persistence), mood engine
internal/monitor/            — /proc + /sys reads, lock-free stats
internal/shell/              — blocked patterns + timeout executor
internal/brain/              — AI providers (Claude/Gemini), system prompt, tool-use loop
internal/discord/            — bot, slash commands, embeds, threads, presence
internal/onboarding/         — terminal hatching flow
internal/proactive/          — scheduled messages + presence updates
```

## License

MIT
