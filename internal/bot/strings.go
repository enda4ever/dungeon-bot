package bot

import (
	"fmt"
	"log"
	"strings"
	"text/template"
	"unicode"
	"unicode/utf8"
)

type MsgData struct {
	Player   string
	Pronouns Pronouns
	Room     string
	Text     string
	Call     string
	Passed   bool
	Cry      string
	Gold     int
	Items    string
	Loot     string
	Rooms    int
	Count    int
	Escaped  bool
	Thread   string
	Link     string
	Name     string
	Channel  string
	Reason   string
	Lines    string
	Place    string
	Pockets  []string
	Escapes  int
	Fallen   int
}

var templateFuncs = template.FuncMap{"cap": capFirst, "fancy": fancy}

func msg(name, body string) *template.Template {
	return template.Must(template.New(name).Funcs(templateFuncs).Parse(body))
}

var doubleStruckCapitals = map[rune]rune{'C': 'ℂ', 'H': 'ℍ', 'N': 'ℕ', 'P': 'ℙ', 'Q': 'ℚ', 'R': 'ℝ', 'Z': 'ℤ'}

func fancy(v any) string {
	var sb strings.Builder
	for _, r := range fmt.Sprint(v) {
		switch {
		case r >= 'a' && r <= 'z':
			sb.WriteRune(0x1D552 + r - 'a')
		case r >= 'A' && r <= 'Z':
			if c, ok := doubleStruckCapitals[r]; ok {
				sb.WriteRune(c)
			} else {
				sb.WriteRune(0x1D538 + r - 'A')
			}
		case r >= '0' && r <= '9':
			sb.WriteRune(0x1D7D8 + r - '0')
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func capFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

type pronounForms struct {
	they, them, their, theyHave, was string
}

var pronounSets = map[string]pronounForms{
	"she":  {"she", "her", "her", "she's", "was"},
	"he":   {"he", "him", "his", "he's", "was"},
	"they": {"they", "them", "their", "they've", "were"},
}

func render(tmpl *template.Template, data MsgData) string {
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		log.Printf("strings: template %q failed: %v", tmpl.Name(), err)
		return ""
	}
	return strings.TrimSpace(sb.String())
}

func join(sep string, parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}

var EnteredMsg = msg("entered", "```"+`
ᛝᛝᛝ 𝕕𝕦𝕟𝕘𝕖𝕠𝕟 𝕘𝕒𝕥𝕖𝕤 ᛝᛝᛝ
`+"```"+`
{{.Player}} has been thrown in the dungeon!`)

var DiedHeaderMsg = msg("died_header", "```"+`
ᛝᛝᛝ 𝕗𝕒𝕝𝕝𝕖𝕟 𝕡𝕖𝕒𝕤𝕒𝕟𝕥 ᛝᛝᛝ
`+"```")

var EscapedHeaderMsg = msg("escaped_header", "```"+`
ᛝᛝᛝ 𝕕𝕦𝕟𝕘𝕖𝕠𝕟 𝕓𝕣𝕖𝕒𝕜 ᛝᛝᛝ
`+"```")

var HistoryMsg = msg("history", "```"+`
ᛝᛝᛝ 𝕙𝕦𝕞𝕓𝕝𝕖 𝕤𝕥𝕠𝕔𝕜𝕡𝕚𝕝𝕖 ᛝᛝᛝ
`+"```"+`
Beyond the dungeon gates, the |x|umble |x|orde grows stronger.

{{.Escapes}} {{if eq .Escapes 1}}rebel has{{else}}rebels have{{end}} successfully escaped their four walls, and together assembled{{if .Pockets}}:{{range .Pockets}}
- {{fancy .}}{{end}}{{else}} nothing.{{end}}

{{.Fallen}} {{if eq .Fallen 1}}rebel has{{else}}rebels have{{end}} crawled into darkness.
Their loot was lost, but their courage inspires the horde.`)

var ThreadOpenMsg = msg("thread_open", "```"+`
ᛝᛝᛝ 𝕪𝕠𝕦𝕣 𝕔𝕖𝕝𝕝 ᛝᛝᛝ
`+"```"+`
You must escape the dungeon.

A coin has been tossed down to you, which bears the mark of the |x|umble |x|orde.
You discover an opening in the corner of your cell, and enter the secret passage...
⠀`)

var RoomMsg = msg("room", `{{.Text}}`)

var UntitledRoomMsg = msg("untitled_room", `**{{.Room}}**`)

var ChapelTitle = msg("chapel_title", "```"+`
ᛝᛝᛝ 𝕥𝕙𝕖 𝕔𝕙𝕒𝕡𝕖𝕝 ᛝᛝᛝ
`+"```")

var CryptTitle = msg("crypt_title", "```"+`
ᛝᛝᛝ 𝕥𝕙𝕖 𝕔𝕣𝕪𝕡𝕥 ᛝᛝᛝ
`+"```")

var DiningHallTitle = msg("dining_hall_title", "```"+`
ᛝᛝᛝ 𝕥𝕙𝕖 𝕕𝕚𝕟𝕚𝕟𝕘 𝕙𝕒𝕝𝕝 ᛝᛝᛝ
`+"```")

var GardenTitle = msg("garden_title", "```"+`
ᛝᛝᛝ 𝕥𝕙𝕖 𝕘𝕒𝕣𝕕𝕖𝕟 ᛝᛝᛝ
`+"```")

var LibraryTitle = msg("library_title", "```"+`
ᛝᛝᛝ 𝕥𝕙𝕖 𝕝𝕚𝕓𝕣𝕒𝕣𝕪 ᛝᛝᛝ
`+"```")

var BedroomTitle = msg("bedroom_title", "```"+`
ᛝᛝᛝ 𝕥𝕙𝕖 𝕓𝕖𝕕𝕣𝕠𝕠𝕞 ᛝᛝᛝ
`+"```")

var FloodedChamberTitle = msg("flooded_chamber_title", "```"+`
ᛝᛝᛝ 𝕥𝕙𝕖 𝕗𝕝𝕠𝕠𝕕𝕖𝕕 𝕔𝕙𝕒𝕞𝕓𝕖𝕣 ᛝᛝᛝ
`+"```")

var KitchenTitle = msg("kitchen_title", "```"+`
ᛝᛝᛝ 𝕥𝕙𝕖 𝕜𝕚𝕥𝕔𝕙𝕖𝕟 ᛝᛝᛝ
`+"```")

var WorkshopTitle = msg("workshop_title", "```"+`
ᛝᛝᛝ 𝕥𝕙𝕖 𝕨𝕠𝕣𝕜𝕤𝕙𝕠𝕡 ᛝᛝᛝ
`+"```")

var EmptyChamberTitle = msg("empty_chamber_title", "```"+`
ᛝᛝᛝ 𝕥𝕙𝕖 𝕖𝕞𝕡𝕥𝕪 𝕔𝕙𝕒𝕞𝕓𝕖𝕣 ᛝᛝᛝ
`+"```")

var FountainTitle = msg("fountain_title", "```"+`
ᛝᛝᛝ 𝕥𝕙𝕖 𝕗𝕠𝕦𝕟𝕥𝕒𝕚𝕟 ᛝᛝᛝ
`+"```")

var StairsTitle = msg("stairs_title", "```"+`
ᛝᛝᛝ 𝕥𝕙𝕖 𝕤𝕥𝕒𝕚𝕣𝕤 ᛝᛝᛝ
`+"```")

var CeilingCollapseTitle = msg("ceiling_collapse_title", "```"+`
ᛝᛝᛝ 𝕥𝕙𝕖 𝕔𝕖𝕚𝕝𝕚𝕟𝕘 𝕔𝕠𝕝𝕝𝕒𝕡𝕤𝕖 ᛝᛝᛝ
`+"```")

var FloorCollapseTitle = msg("floor_collapse_title", "```"+`
ᛝᛝᛝ 𝕥𝕙𝕖 𝕗𝕝𝕠𝕠𝕣 𝕔𝕠𝕝𝕝𝕒𝕡𝕤𝕖 ᛝᛝᛝ
`+"```")

var SupplyRoomTitle = msg("supply_room_title", "```"+`
ᛝᛝᛝ 𝕥𝕙𝕖 𝕤𝕦𝕡𝕡𝕝𝕪 𝕣𝕠𝕠𝕞 ᛝᛝᛝ
`+"```")

var BarracksTitle = msg("barracks_title", "```"+`
ᛝᛝᛝ 𝕥𝕙𝕖 𝕓𝕒𝕣𝕣𝕒𝕔𝕜𝕤 ᛝᛝᛝ
`+"```")

var FloodTitle = msg("flood_title", "```"+`
ᛝᛝᛝ 𝕥𝕙𝕖 𝕗𝕝𝕠𝕠𝕕 ᛝᛝᛝ
`+"```")

var ArrowTrapTitle = msg("arrow_trap_title", "```"+`
ᛝᛝᛝ 𝕥𝕙𝕖 𝕒𝕣𝕣𝕠𝕨 𝕥𝕣𝕒𝕡 ᛝᛝᛝ
`+"```")

var TrapDoorTitle = msg("trap_door_title", "```"+`
ᛝᛝᛝ 𝕥𝕙𝕖 𝕥𝕣𝕒𝕡 𝕕𝕠𝕠𝕣 ᛝᛝᛝ
`+"```")

var RoomTitles = map[string]*template.Template{
	"chapel":           ChapelTitle,
	"crypt":            CryptTitle,
	"dining-hall":      DiningHallTitle,
	"garden":           GardenTitle,
	"workshop":         WorkshopTitle,
	"empty-chamber":    EmptyChamberTitle,
	"fountain":         FountainTitle,
	"library":          LibraryTitle,
	"bedroom":          BedroomTitle,
	"flooded-chamber":  FloodedChamberTitle,
	"kitchen":          KitchenTitle,
	"stairs":           StairsTitle,
	"ceiling-collapse": CeilingCollapseTitle,
	"floor-collapse":   FloorCollapseTitle,
	"supply-room":      SupplyRoomTitle,
	"barracks":         BarracksTitle,
	"flood":            FloodTitle,
	"arrow-trap":       ArrowTrapTitle,
	"trap-door":        TrapDoorTitle,
}

var CallMsg = msg("call", "You flip the humble coin.\nCall `/heads` or `/tails`...")


var OutcomeMsg = msg("outcome", `{{.Cry}} {{.Text}}`)

var (
	PassCries = []string{"Huzzah!", "Helm yeah!", "Indeed!", "Victorious!"}
	FailCries = []string{"Curses!", "Misfortune!", "Alas!", "Oh Dear!"}
)

var OnwardsMsg = msg("onwards", `Onwards!`)

var DiedMsg = msg("died", `The dungeon claims another.`)

var EscapedMsg = msg("escaped", `{{.Player}} has escaped the dungeon. The |x|umble |x|orde grows stronger.`)

var CollectMsg = msg("collect", `You collect: {{fancy .Loot}}`)

var EscapeSpoilsMsg = msg("escape_spoils", `{{.Player}} escaped the dungeon{{if or .Gold .Items}} with:{{if .Gold}}
Gold: {{fancy .Gold}}{{end}}{{if .Items}}
Items: {{fancy .Items}}{{end}}{{else}}.{{end}}`)

var DroppedMsg = msg("dropped", `{{if or .Gold .Items}}Dropped:{{if .Gold}}
Gold: {{fancy .Gold}}{{end}}{{if .Items}}
Items: {{fancy .Items}}{{end}}{{end}}`)

var PocketsMsg = msg("pockets", `{{if .Pockets}}Lost to the guards:{{range .Pockets}}
- {{fancy .}}{{end}}{{else}}{{cap .Pronouns.They}} died with dust in {{.Pronouns.Their}} pockets.{{end}}`)

var ObituaryMsg = msg("obituary", `{{.Player}} perished{{if .Place}} {{.Place}}{{end}}. {{.Text}}`)

var PublicEscapedMsg = msg("public_escaped", `The |x|umble |x|orde grows stronger.`)

var EpilogueMsg = msg("epilogue", `{{.Player}} escaped{{if .Place}} {{.Place}}{{end}}. {{.Text}} {{.Cry}}`)

var CollectedMsg = msg("collected", `{{if .Pockets}}Acquired for the rebellion:{{range .Pockets}}
- {{fancy .}}{{end}}{{end}}`)

var TimeoutObituaryMsg = msg("timeout_obituary", `{{cap .Pronouns.They}} lingered in the dungeon too long.`)

var AlreadyInsideMsg = msg("already_inside", `You are already in the dungeon! {{.Thread}}`)

var ThreadNameMsg = msg("thread_name", `𝖊𝖘𝖈𝖆𝖕𝖊 𝖗𝖔𝖚𝖙𝖊`)

var ThreadLinkMsg = msg("thread_link", `{ [attempt escape]({{.Link}}) }`)

var ThreadFailedMsg = msg("thread_failed", `The dungeon remains locked! Try again later. ({{.Reason}})`)

var ElsewhereMsg = msg("elsewhere", `You must only flip the humble coin in the dungeon! {{.Thread}}`)

var NotInsideMsg = msg("not_inside", "To the dungeon with you!{{if .Channel}} {{.Channel}}{{else}}\n(`/dungeon crawl` to escape){{end}}")

var GatesShutMsg = msg("gates_shut", `Your cell door remains locked. Try again later`)

var WrongChannelMsg = msg("wrong_channel", `The dungeon's entrance is in {{.Channel}}!`)

var CoinInAirMsg = msg("coin_in_air", `You must only flip the humble coin but once at a time!`)

var NotAdminMsg = msg("not_admin", `Only admins can use this command.`)

var LogChannelSetMsg = msg("log_channel_set", `The dungeon is now in {{.Channel}}. Runs can only start there, and entries and results are posted there.`)

var PronounsSetMsg = msg("pronouns_set", `The dungeon now knows these pronoun roles:
{{.Lines}}`)

var PronounsUnknownMsg = msg("pronouns_unknown", `Nothing was changed. These don't match any role:
{{.Lines}}`)

var PausedMsg = msg("paused", `The dungeon is paused: no new runs can start. {{.Count}} {{if eq .Count 1}}run is{{else}}runs are{{end}} still in progress and will be over within 5 minutes. Run this again to check.`)

var UnpausedMsg = msg("unpaused", `The dungeon is open again.`)

var PronounsClearedMsg = msg("pronouns_cleared", `Pronoun roles forgotten. Everyone is they/them until you set them again.`)

var (
	DescDungeon          = "Commands for Dungeon Bot"
	DescCrawl            = "Descend into the dungeon, if you so dare!"
	DescHistory          = "Behold the humble stockpile"
	DescHeads            = "Flip the humble coin and call heads"
	DescTails            = "Flip the humble coin and call tails"
	DescDungeonAdmin     = "Run the dungeon (admin)"
	DescLogChannel       = "Choose the dungeon's channel, where runs start and results are posted (admin)"
	DescLogChannelOption = "The channel players enter the dungeon from"
	DescPronouns         = "Tell the bot which roles mean which pronouns (admin)"
	DescPronounsShe      = "The role for she/her"
	DescPronounsHe       = "The role for he/him"
	DescPronounsThey     = "The role for they/them"
	DescPronounsHeShe    = "The role for he/she"
	DescPronounsTheyShe  = "The role for they/she"
	DescPronounsTheyHe   = "The role for they/he"
	DescClearPronouns    = "Forget every pronoun role, so everyone is they/them (admin)"
	DescPause            = "Stop new runs from starting; runs in progress can finish (admin)"
	DescUnpause          = "Let new runs start again (admin)"
)
