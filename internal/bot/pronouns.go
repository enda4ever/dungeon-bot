package bot

import (
	"log"
	"maps"
	"sort"
	"strings"

	"github.com/bwmarrin/discordgo"
)

var pronounOptions = []string{"she", "he", "they", "heshe", "theyshe", "theyhe"}

var pronounRoleSets = map[string][]string{
	"she":     {"she"},
	"he":      {"he"},
	"they":    {"they"},
	"heshe":   {"he", "she"},
	"theyshe": {"they", "she"},
	"theyhe":  {"they", "he"},
}

type Pronouns struct {
	set string
}

func (p Pronouns) forms() pronounForms {
	if f, ok := pronounSets[p.set]; ok {
		return f
	}
	return pronounSets["they"]
}

func (p Pronouns) They() string     { return p.forms().they }
func (p Pronouns) Them() string     { return p.forms().them }
func (p Pronouns) Their() string    { return p.forms().their }
func (p Pronouns) TheyHave() string { return p.forms().theyHave }
func (p Pronouns) Was() string      { return p.forms().was }

func (d *Dungeon) pronounsFor(memberRoles []string) Pronouns {
	d.mu.Lock()
	mapped := maps.Clone(d.state.PronounRoles)
	d.mu.Unlock()

	held := map[string]bool{}
	for _, r := range memberRoles {
		held[r] = true
	}
	var pool []string
	seen := map[string]bool{}
	for _, name := range pronounOptions {
		if roleID := mapped[name]; roleID == "" || !held[roleID] {
			continue
		}
		for _, set := range pronounRoleSets[name] {
			if !seen[set] {
				seen[set] = true
				pool = append(pool, set)
			}
		}
	}
	if len(pool) == 0 {
		return Pronouns{}
	}
	return Pronouns{set: pool[d.dice.IntN(len(pool))]}
}

func (d *Dungeon) setPronounRoles(roles []*discordgo.Role, guildID string, given map[string]string) string {
	chosen := map[string]string{}
	var unknown []string
	for _, name := range pronounOptions {
		value, ok := given[name]
		if !ok {
			continue
		}
		if roleID, found := resolveRoleValue(roles, guildID, value); found {
			chosen[name] = roleID
		} else {
			unknown = append(unknown, "- "+name+": "+value)
		}
	}
	if len(unknown) > 0 {
		return render(PronounsUnknownMsg, MsgData{Lines: strings.Join(unknown, "\n")})
	}

	d.mu.Lock()
	if d.state.PronounRoles == nil {
		d.state.PronounRoles = map[string]string{}
	}
	maps.Copy(d.state.PronounRoles, chosen)
	d.save()
	mapped := maps.Clone(d.state.PronounRoles)
	d.mu.Unlock()

	var lines []string
	for _, name := range pronounOptions {
		if roleID := mapped[name]; roleID != "" {
			lines = append(lines, "- "+name+": <@&"+roleID+">")
		}
	}
	return render(PronounsSetMsg, MsgData{Lines: strings.Join(lines, "\n")})
}

func (d *Dungeon) clearPronouns() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.state.PronounRoles = nil
	d.save()
}

func (d *Dungeon) autocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	if data.Name != "dungeon-admin" || len(data.Options) == 0 || data.Options[0].Name != "pronouns" {
		return
	}
	typed := ""
	for _, o := range data.Options[0].Options {
		if o.Focused {
			typed = o.StringValue()
		}
	}
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{Choices: roleSuggestions(guildRolesOf(s, i.GuildID), i.GuildID, typed)},
	})
	if err != nil {
		log.Printf("discord: could not send role suggestions: %v", err)
	}
}

func guildRolesOf(s *discordgo.Session, guildID string) []*discordgo.Role {
	if g, err := s.State.Guild(guildID); err == nil && len(g.Roles) > 0 {
		return g.Roles
	}
	roles, err := s.GuildRoles(guildID)
	if err != nil {
		log.Printf("discord: could not list roles: %v", err)
	}
	return roles
}

func roleSuggestions(roles []*discordgo.Role, guildID, typed string) []*discordgo.ApplicationCommandOptionChoice {
	want := plainText(typed)
	var slashed, rest []*discordgo.Role
	for _, r := range roles {
		if r.ID == guildID || r.Managed {
			continue
		}
		name := plainText(r.Name)
		if !strings.Contains(name, want) {
			continue
		}
		if strings.Contains(name, "/") {
			slashed = append(slashed, r)
		} else {
			rest = append(rest, r)
		}
	}
	highestFirst := func(rs []*discordgo.Role) {
		sort.SliceStable(rs, func(a, b int) bool { return rs[a].Position > rs[b].Position })
	}
	highestFirst(slashed)
	highestFirst(rest)

	choices := make([]*discordgo.ApplicationCommandOptionChoice, 0, 25)
	for _, r := range append(slashed, rest...) {
		if len(choices) == 25 {
			break
		}
		name := []rune(r.Name)
		if len(name) > 100 {
			name = name[:100]
		}
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{Name: string(name), Value: r.ID})
	}
	return choices
}

func resolveRoleValue(roles []*discordgo.Role, guildID, value string) (string, bool) {
	want := plainText(strings.TrimSpace(value))
	if want == "" {
		return "", false
	}
	var partial []string
	for _, r := range roles {
		if r.ID == guildID || r.Managed {
			continue
		}
		if r.ID == value || plainText(r.Name) == want {
			return r.ID, true
		}
		if strings.Contains(plainText(r.Name), want) {
			partial = append(partial, r.ID)
		}
	}
	if len(partial) == 1 {
		return partial[0], true
	}
	return "", false
}

func pronounOptionDefs() []*discordgo.ApplicationCommandOption {
	descs := map[string]string{
		"she":     DescPronounsShe,
		"he":      DescPronounsHe,
		"they":    DescPronounsThey,
		"heshe":   DescPronounsHeShe,
		"theyshe": DescPronounsTheyShe,
		"theyhe":  DescPronounsTheyHe,
	}
	opts := make([]*discordgo.ApplicationCommandOption, 0, len(pronounOptions))
	for _, name := range pronounOptions {
		opts = append(opts, &discordgo.ApplicationCommandOption{
			Name:         name,
			Description:  descs[name],
			Type:         discordgo.ApplicationCommandOptionString,
			Autocomplete: true,
			Required:     false,
		})
	}
	return opts
}
