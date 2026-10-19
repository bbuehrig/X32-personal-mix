package x32

// IconEmoji returns a display emoji for an X32 icon index (1–74).
// Index 1 is "blank" on the desk — empty string so the UI shows a transparent slot.
func IconEmoji(id int) string {
	if id < 1 || id > len(iconEmojis) {
		return ""
	}
	if id == 1 {
		return "" // blank
	}
	return iconEmojis[id-1]
}

// IconName returns a short English key for an X32 icon index.
func IconName(id int) string {
	if id < 1 || id > len(iconNames) {
		return "unknown"
	}
	return iconNames[id-1]
}

var iconNames = []string{
	"blank", "kick-back", "kick-front", "snare-top", "snare-bottom",
	"tom-high", "tom-medium", "tom", "hihat", "crash",
	"drums", "bell", "congas-1", "congas-2", "tambourine",
	"xylophone", "elec-bass", "acou-bass-1", "acou-bass-2", "elec-guit-1",
	"elec-guit-2", "elec-guit-3", "acou-guit", "amp-1", "amp-2",
	"amp-3", "acou-piano", "organ", "elec-key-1", "elec-key-2",
	"synth-1", "synth-2", "synth-3", "synth-4", "trumpet",
	"trombone", "sax", "clarinet", "violin", "cello",
	"male-singer", "female-singer", "choir", "hand-sign", "talk-a",
	"talk-b", "mic-1", "c-mic-left", "c-mic-right", "mic-2",
	"wireless-mic", "table-mic", "in-ear", "xlr", "trs",
	"trs-left", "trs-right", "cinch-left", "cinch-right", "tape",
	"fx", "pc", "wedge", "speaker-right", "speaker-left",
	"speaker-array", "speaker-stand", "rack", "controls", "faders",
	"routing-main", "routing-bus", "routing-dispatch", "smiley",
}

var iconEmojis = []string{
	"", "🥁", "🥁", "🥁", "🥁",
	"🥁", "🥁", "🥁", "🎶", "💥",
	"🥁", "🔔", "🪘", "🪘", "🪇",
	"🎹", "🎸", "🎸", "🎸", "🎸",
	"🎸", "🎸", "🎸", "🔊", "🔊",
	"🔊", "🎹", "🎹", "⌨️", "⌨️",
	"🎛️", "🎛️", "🎛️", "🎛️", "🎺",
	"🎺", "🎷", "🎵", "🎻", "🎻",
	"🎤", "🎤", "👥", "🤘", "🗣️",
	"🗣️", "🎙️", "🎙️", "🎙️", "🎙️",
	"📡", "🎙️", "🎧", "🔌", "🔌",
	"🔌", "🔌", "🔌", "🔌", "📼",
	"✨", "💻", "📢", "🔈", "🔈",
	"🔊", "🗼", "🗄️", "🎚️", "🎚️",
	"🔀", "🔀", "🔀", "😊",
}
