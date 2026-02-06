# PiPet Addendum: Discord Instead of Telegram

## The Swap
Replace Telegram with Discord as the sole messaging layer. Same architecture, different client.

### Dependency change
Remove: `github.com/go-telegram-bot-api/telegram-bot-api/v5`
Add: `github.com/bwmarrin/discordgo`

## Bot Setup
User creates a Discord application at discord.com/developers, creates a bot, grabs the token. Bot needs these gateway intents:
- `GUILD_MESSAGES` — read/send in channels
- `MESSAGE_CONTENT` — read what people say (privileged intent, must be toggled on in dev portal)
- `GUILDS` — know what server it's in

Bot permissions integer: `326417525824` (Send Messages, Send Messages in Threads, Create Public Threads, Embed Links, Attach Files, Read Message History, Use Slash Commands)

## How It Works

### Dedicated channel
Pet lives in a single channel (configured by ID). All proactive messages (morning check-in, alerts, boredom, milestones) go here. Anyone in the channel can watch.

### Slash commands
Register these on startup:

| Command | What it does | LLM? |
|---------|-------------|------|
| `/status` | Pet stats + mood as an embed | No |
| `/pet` | Give affection, boost happiness | No (local brain) |
| `/feed` | Run cleanup/maintenance tasks | Yes (cloud) |
| `/heal` | Diagnose and fix resource issues | Yes (cloud) |
| `/play` | Ask pet to do something fun | Yes (cloud) |

### Free-form messages
Any regular message in the pet's channel is treated as conversation. Goes through the classifier (local brain) → routed to local or cloud as before. Bot responds in the same channel.

### Threads for noisy stuff
When the pet runs diagnostics or multi-step shell commands, it creates a thread:

```
🐙 let me look into that...
  └─ 🧵 "Inky diagnosing memory issue" (thread)
       → running ps aux --sort=-%mem
       → found: chromium-browser using 280MB
       → want me to kill it?
```

Keeps the main channel clean. Spectators see the thread was created but don't get spammed with shell output.

### Embeds for status
Status responses use Discord embeds for a clean look:

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

## Owner vs Spectator

### Owner (allowed_users list)
- Can use all slash commands
- Can run shell commands via conversation
- Can feed, heal, play

### Spectators (everyone else in channel)
- Can watch everything
- Can `/status` to check on the pet
- Can `/pet` to give affection (happiness boost, everyone can pet!)
- Cannot run commands, feed, or heal — bot politely declines:
  ```
  🦀 nice try. only my owner gets to poke around in my guts.
  ```

## Config changes

```yaml
discord:
  bot_token: ""
  channel_id: ""         # channel the pet lives in
  owner_ids:             # users who can run commands
    - "123456789"
  allow_spectator_pet: true  # let anyone /pet for affection
  use_threads: true          # diagnostic output in threads
```

Removes the old `telegram` config block entirely.

## Proactive messages
Same as before but posted to the Discord channel. Pet uses the bot's presence status too:

| Mood | Discord Status |
|------|---------------|
| 😊 Happy | 🟢 Online — "feeling great!" |
| 😌 Content | 🟢 Online — "just vibing" |
| 😐 Bored | 🟡 Idle — "anyone there?" |
| 😰 Anxious | 🔴 DND — "CPU is spiking..." |
| 🤒 Sick | 🔴 DND — "need help..." |
| 😴 Sleepy | 🟡 Idle — "zzz" |
| 💀 Dead | ⚫ Invisible |

This is a nice free touch — you can see your pet's mood from the Discord sidebar without even opening the channel.

## Onboarding
Hatching happens in the terminal on the Pi itself when someone first runs the binary:

```
$ ./pipet

  🥚 crk... crk...

  pick a species:

  1) 🦞 lobster    2) 🐙 octopus
  3) 🐢 turtle     4) 🐧 penguin
  5) 🦀 crab       6) 🐡 pufferfish
  7) 🦑 squid      8) 🐠 fish

  > 3

  🐢 ...

  what's my name?

  > Sheldon

  🐢 *slowly pokes head out*

  hi. i'm Sheldon.
  it's warm in here. i like it.

  starting up...
  ✓ monitor running
  ✓ local brain loaded (smollm2-135m)
  ✓ discord connected
  ✓ state saved

  Sheldon is alive. don't forget about me.
```

Then in the Discord channel, the pet introduces itself:

```
🐢 hey everyone. i'm Sheldon.
   just hatched on a little pi zero.
   48°C in here. cozy.
```

That's it. The hatching is yours. The introduction is public.
