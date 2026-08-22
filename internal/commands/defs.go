// Package commands defines every slash command and its implementation.
// One concept per file; definitions live in defs.go as the single source
// of truth (Phase 5 generates docs from these).
package commands

import (
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/omit"
	"github.com/disgoorg/snowflake/v2"

	"github.com/disgoorg/disgolink/v4/lavalink"

	hexbot "hex-music-bot/internal/bot"
)

var commandDefs = []discord.ApplicationCommandCreate{
	discord.SlashCommandCreate{
		Name:        "play",
		Description: "Play a song, playlist or search query",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionString{
				Name:        "identifier",
				Description: "Song link, playlist link or search query",
				Required:    true,
			},
			discord.ApplicationCommandOptionString{
				Name:        "source",
				Description: "Where to search when identifier is not a link",
				Required:    false,
				Choices: []discord.ApplicationCommandOptionChoiceString{
					{Name: "YouTube", Value: string(lavalink.SearchTypeYouTube)},
					{Name: "YouTube Music", Value: string(lavalink.SearchTypeYouTubeMusic)},
					{Name: "SoundCloud", Value: string(lavalink.SearchTypeSoundCloud)},
					{Name: "Spotify", Value: "spsearch"},
					{Name: "Deezer", Value: "dzsearch"},
					{Name: "Apple Music", Value: "amsearch"},
				},
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "pause",
		Description: "Pause the current song",
	},
	discord.SlashCommandCreate{
		Name:        "resume",
		Description: "Resume the paused song",
	},
	discord.SlashCommandCreate{
		Name:        "skip",
		Description: "Skip ahead in the queue",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionInt{
				Name:        "amount",
				Description: "How many queued songs to skip ahead (default 1)",
				Required:    false,
				MinValue:    omit.Ptr(1),
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "previous",
		Description: "Play the previously finished song",
	},
	discord.SlashCommandCreate{
		Name:        "stop",
		Description: "Stop playback and clear the queue",
	},
	discord.SlashCommandCreate{
		Name:        "leave",
		Description: "Disconnect the bot from voice",
	},
	discord.SlashCommandCreate{
		Name:        "join",
		Description: "Join your current voice channel",
	},
	discord.SlashCommandCreate{
		Name:        "queue",
		Description: "Show the queued songs",
	},
	discord.SlashCommandCreate{
		Name:        "now-playing",
		Description: "Show the currently playing song",
	},
	discord.SlashCommandCreate{
		Name:        "shuffle",
		Description: "Shuffle the queue",
	},
	discord.SlashCommandCreate{
		Name:        "loop",
		Description: "Set the loop mode",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionString{
				Name:        "mode",
				Description: "What to loop",
				Required:    true,
				Choices: []discord.ApplicationCommandOptionChoiceString{
					{Name: "Off", Value: "off"},
					{Name: "This track", Value: "track"},
					{Name: "Whole queue", Value: "queue"},
				},
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "clear",
		Description: "Remove all queued songs",
	},
	discord.SlashCommandCreate{
		Name:        "volume",
		Description: "Set the player volume",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionInt{
				Name:        "level",
				Description: "Volume level (0-1000)",
				Required:    true,
				MinValue:    omit.Ptr(0),
				MaxValue:    omit.Ptr(1000),
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "seek",
		Description: "Seek to a position in the current song",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionInt{
				Name:        "position",
				Description: "Position to seek to",
				Required:    true,
				MinValue:    omit.Ptr(0),
			},
			discord.ApplicationCommandOptionInt{
				Name:        "unit",
				Description: "Unit of the position (default seconds)",
				Required:    false,
				Choices: []discord.ApplicationCommandOptionChoiceInt{
					{Name: "Milliseconds", Value: int(lavalink.Millisecond)},
					{Name: "Seconds", Value: int(lavalink.Second)},
					{Name: "Minutes", Value: int(lavalink.Minute)},
					{Name: "Hours", Value: int(lavalink.Hour)},
				},
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "remove",
		Description: "Remove one queued song by its position",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionInt{
				Name:        "position",
				Description: "Queue position to remove (see /queue)",
				Required:    true,
				MinValue:    omit.Ptr(1),
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "search",
		Description: "Search for a song and pick the right one",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionString{
				Name:         "identifier",
				Description:  "Search query (live suggestions appear as you type)",
				Required:     true,
				Autocomplete: true,
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "lyrics",
		Description: "Show synced lyrics for the current song",
	},
	discord.SlashCommandCreate{
		Name:        "history",
		Description: "Show recently played tracks",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionUser{
				Name:        "user",
				Description: "Show history for a specific user",
				Required:    false,
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "taste",
		Description: "Manage your taste profile for autoplay",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionString{
				Name:        "action",
				Description: "What to do",
				Required:    true,
				Choices: []discord.ApplicationCommandOptionChoiceString{
					{Name: "Add artist", Value: "add"},
					{Name: "Remove artist", Value: "remove"},
					{Name: "List my artists", Value: "list"},
				},
			},
			discord.ApplicationCommandOptionString{
				Name:        "artist",
				Description: "Artist name (for add/remove)",
				Required:    false,
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "playlist",
		Description: "Manage playlists",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionString{
				Name:        "action",
				Description: "Playlist action",
				Required:    true,
				Choices: []discord.ApplicationCommandOptionChoiceString{
					{Name: "Create playlist", Value: "create"},
					{Name: "List my playlists", Value: "list"},
					{Name: "Show playlist", Value: "show"},
					{Name: "Delete playlist", Value: "delete"},
					{Name: "Add track by title", Value: "add"},
				},
			},
			discord.ApplicationCommandOptionString{
				Name:        "name",
				Description: "Playlist name (for create)",
				Required:    false,
			},
			discord.ApplicationCommandOptionString{
				Name:        "code",
				Description: "Share code (for show/delete/add)",
				Required:    false,
			},
			discord.ApplicationCommandOptionString{
				Name:        "title",
				Description: "Track title (for add)",
				Required:    false,
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "autoplay",
		Description: "Toggle autoplay — keeps music going after queue drains",
	},
	discord.SlashCommandCreate{
		Name:        "247",
		Description: "Toggle 24/7 mode — survives restarts",
	},
	discord.SlashCommandCreate{
		Name:        "request-channel",
		Description: "Set this channel as the song-request channel",
	},
	discord.SlashCommandCreate{
		Name:        "filter",
		Description: "Apply an audio filter to playback",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionString{
				Name:        "preset",
				Description: "Filter preset to apply",
				Required:    true,
				Choices: []discord.ApplicationCommandOptionChoiceString{
					{Name: "Bassboost", Value: "bassboost"},
					{Name: "Nightcore", Value: "nightcore"},
					{Name: "Vaporwave", Value: "vaporwave"},
					{Name: "8D", Value: "8d"},
					{Name: "Tremolo", Value: "tremolo"},
					{Name: "Vibrato", Value: "vibrato"},
					{Name: "Karaoke", Value: "karaoke"},
					{Name: "Low Pass", Value: "lowpass"},
					{Name: "Reset All", Value: "reset"},
				},
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "forward",
		Description: "Skip forward by a time amount",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionString{
				Name:        "time",
				Description: "Amount to skip forward (e.g. 10, 1:30)",
				Required:    false,
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "rewind",
		Description: "Rewind by a time amount",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionString{
				Name:        "time",
				Description: "Amount to rewind (e.g. 10, 1:30)",
				Required:    false,
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "playtop",
		Description: "Add a song to play next (top of queue)",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionString{
				Name:        "identifier",
				Description: "Song link or search query",
				Required:    true,
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "playskip",
		Description: "Add a song and immediately skip to it",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionString{
				Name:        "identifier",
				Description: "Song link or search query",
				Required:    true,
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "skipto",
		Description: "Skip to a specific queue position, removing tracks before it",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionInt{
				Name:        "position",
				Description: "Queue position to skip to (see /queue)",
				Required:    true,
				MinValue:    omit.Ptr(1),
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "move",
		Description: "Move a queued song to a different position",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionInt{
				Name:        "from",
				Description: "Current queue position of the song",
				Required:    true,
				MinValue:    omit.Ptr(1),
			},
			discord.ApplicationCommandOptionInt{
				Name:        "to",
				Description: "New queue position for the song",
				Required:    true,
				MinValue:    omit.Ptr(1),
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "swap",
		Description: "Swap two queued songs by position",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionInt{
				Name:        "first",
				Description: "First queue position",
				Required:    true,
				MinValue:    omit.Ptr(1),
			},
			discord.ApplicationCommandOptionInt{
				Name:        "second",
				Description: "Second queue position",
				Required:    true,
				MinValue:    omit.Ptr(1),
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "removedupes",
		Description: "Remove duplicate songs from the queue",
	},
	discord.SlashCommandCreate{
		Name:        "settings",
		Description: "View all guild settings",
	},
	discord.SlashCommandCreate{
		Name:        "voteskip",
		Description: "Vote to skip the current song (when no DJ is set)",
	},
	discord.SlashCommandCreate{
		Name:        "debug",
		Description: "Show system and node diagnostics (owner only)",
	},
	discord.SlashCommandCreate{
		Name:        "ping",
		Description: "Check bot latency",
	},
	discord.SlashCommandCreate{
		Name:        "stats",
		Description: "Show playback statistics",
	},
}
// Register bulk-overwrites application commands. With guildID set, updates
// are instant in that guild; zero registers globally instead.
func Register(client *bot.Client, guildID snowflake.ID) error {
	var guildIDs []snowflake.ID
	if guildID != 0 {
		guildIDs = []snowflake.ID{guildID}
	}
	return handler.SyncCommands(client, commandDefs, guildIDs)
}

// All builds the command-name -> handler map. Every entry routes through
// the same *Bot, so buttons (Phase 2) can reuse identical paths.
func All(b *hexbot.Bot) map[string]hexbot.CommandHandler {
	run := func(fn func(*hexbot.Bot, *events.ApplicationCommandInteractionCreate, discord.SlashCommandInteractionData) error) hexbot.CommandHandler {
		return func(event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
			return fn(b, event, data)
		}
	}
	return map[string]hexbot.CommandHandler{
		"play":            run(Play),
		"pause":           run(Pause),
		"resume":          run(Resume),
		"skip":            run(Skip),
		"previous":        run(Previous),
		"stop":            run(Stop),
		"leave":           run(Leave),
		"join":            run(Join),
		"queue":           run(Queue),
		"now-playing":     run(NowPlaying),
		"lyrics":          run(Lyrics),
		"shuffle":         run(Shuffle),
		"loop":            run(Loop),
		"clear":           run(Clear),
		"volume":          run(Volume),
		"seek":            run(Seek),
		"remove":          run(Remove),
		"search":          run(Play),
		"history":         run(History),
		"taste":           run(Taste),
		"request-channel": run(SetRequestChannel),
		"filter":          run(Filter),
		"forward":         run(Forward),
		"rewind":          run(Rewind),
		"replay":          run(Replay),
		"playtop":         run(PlayTop),
		"playskip":        run(PlaySkip),
		"skipto":          run(SkipTo),
		"voteskip":        run(VoteSkip),
		"debug":           run(Debug),
		"ping":            run(Ping),
		"stats":           run(Stats),
	}
}
