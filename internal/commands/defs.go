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
		"playlist":        run(PlaylistGroup),
		"autoplay":        run(ToggleAutoplay),
		"247":             run(Set247),
		"request-channel": run(SetRequestChannel),
	}
}
