package bot

import (
	"errors"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

func CommandDefs() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        "dungeon",
			Description: DescDungeon,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "crawl",
					Description: DescCrawl,
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name:        "history",
					Description: DescHistory,
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
			},
		},
		{
			Name:        "heads",
			Description: DescHeads,
		},
		{
			Name:        "tails",
			Description: DescTails,
		},
		{
			Name:                     "dungeon-admin",
			Description:              DescDungeonAdmin,
			DefaultMemberPermissions: ptr(int64(discordgo.PermissionManageServer)),
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "channel",
					Description: DescLogChannel,
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{
							Name:         "channel",
							Description:  DescLogChannelOption,
							Type:         discordgo.ApplicationCommandOptionChannel,
							ChannelTypes: []discordgo.ChannelType{discordgo.ChannelTypeGuildText},
							Required:     true,
						},
					},
				},
				{
					Name:        "pronouns",
					Description: DescPronouns,
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options:     pronounOptionDefs(),
				},
				{
					Name:        "clear-pronouns",
					Description: DescClearPronouns,
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name:        "pause",
					Description: DescPause,
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name:        "unpause",
					Description: DescUnpause,
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
			},
		},
	}
}

func ptr[T any](v T) *T { return &v }

func (d *Dungeon) Route(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.GuildID != d.cfg.GuildID {
		return
	}
	if i.Type == discordgo.InteractionApplicationCommandAutocomplete {
		d.autocomplete(s, i)
		return
	}
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	data := i.ApplicationCommandData()
	sub := ""
	var opts []*discordgo.ApplicationCommandInteractionDataOption
	if len(data.Options) > 0 {
		sub, opts = data.Options[0].Name, data.Options[0].Options
	}

	switch {
	case data.Name == "heads" || data.Name == "tails":
		d.flip(s, i, callerID(i), data.Name)
	case data.Name == "dungeon" && sub == "crawl":
		d.crawl(s, i, callerID(i))
	case data.Name == "dungeon" && sub == "history":
		escapes, deaths, pile := d.stockpile()
		d.play(s, i, false, render(HistoryMsg, MsgData{Escapes: escapes, Fallen: deaths, Pockets: pile}))
	case data.Name == "dungeon-admin":
		d.admin(s, i, sub, opts)
	}
}

func (d *Dungeon) crawl(s *discordgo.Session, i *discordgo.InteractionCreate, userID string) {
	player := mention(userID)
	if dungeon, ok := d.entrance(i.ChannelID); !ok {
		d.play(s, i, true, render(WrongChannelMsg, MsgData{Player: player, Channel: channelRef(dungeon)}))
		return
	}
	if d.closedTo(userID) {
		d.play(s, i, true, render(GatesShutMsg, MsgData{Player: player}))
		return
	}
	room, thread, fresh := d.enter(userID)
	if !fresh {
		d.play(s, i, true, render(AlreadyInsideMsg, MsgData{Player: player, Thread: channelRef(thread)}))
		return
	}
	if err := deferPublic(s, i); err != nil {
		log.Printf("discord: could not acknowledge crawl for %s: %v", userID, err)
		d.leave(userID)
		return
	}

	thread, err := openThread(s, i.ChannelID, userID, render(ThreadNameMsg, MsgData{Name: displayName(i)}))
	if err != nil {
		log.Printf("discord: could not open a thread for %s: %v", userID, err)
		d.leave(userID)
		if err := s.InteractionResponseDelete(i.Interaction); err != nil {
			log.Printf("discord: could not withdraw the entry message: %v", err)
		}
		if err := whisper(s, i, render(ThreadFailedMsg, MsgData{Reason: err.Error()})); err != nil {
			log.Printf("discord: could not send private reply: %v", err)
		}
		return
	}
	d.attach(userID, thread)
	time.AfterFunc(runLimit, func() { d.timeUp(s, userID, thread) })

	entered := render(EnteredMsg, MsgData{Player: player})
	if err := edit(s, i, entered); err != nil {
		log.Printf("discord: could not update reply: %v", err)
	}
	if err := whisper(s, i, render(ThreadLinkMsg, MsgData{Player: player, Link: channelLink(i.GuildID, thread)})); err != nil {
		log.Printf("discord: could not send private reply: %v", err)
	}
	post(s, thread, render(ThreadOpenMsg, MsgData{Player: player}))
	d.pause()
	post(s, thread, roomPrompt(room, player))
}

func (d *Dungeon) flip(s *discordgo.Session, i *discordgo.InteractionCreate, userID, side string) {
	player := mention(userID)
	t, err := d.call(userID, i.ChannelID, side)
	switch {
	case errors.Is(err, ErrNotInside):
		data := MsgData{Player: player}
		if dungeon, ok := d.entrance(i.ChannelID); !ok {
			data.Channel = channelRef(dungeon)
		}
		d.play(s, i, true, render(NotInsideMsg, data))
		return
	case errors.Is(err, ErrElsewhere):
		d.play(s, i, true, render(ElsewhereMsg, MsgData{Player: player, Thread: channelRef(t.thread)}))
		return
	case errors.Is(err, ErrCoinInAir):
		d.play(s, i, true, render(CoinInAirMsg, MsgData{Player: player}))
		return
	}
	defer d.settle(userID)
	d.play(s, i, false, flipSteps(t, player)...)
	if t.died || t.escaped {
		post(s, d.logChannel(), publicReport(t, player, d.pronounsFor(memberRoles(i))))
		deleteThread(s, t.thread)
		wasCrawler := t.escaped && slices.Contains(memberRoles(i), d.crawlerRoleID())
		d.updateCrawlerRole(s, userID, t.escaped)
		if wasCrawler {
			d.grantDestroyer(s, userID)
		}
	}
}

func (d *Dungeon) timeUp(s *discordgo.Session, userID, threadID string) {
	t, ok := d.expire(userID, threadID)
	if !ok {
		return
	}
	var roles []string
	if m, err := s.GuildMember(d.cfg.GuildID, userID); err == nil {
		roles = m.Roles
	}
	post(s, d.logChannel(), publicReport(t, mention(userID), d.pronounsFor(roles)))
	deleteThread(s, threadID)
	d.updateCrawlerRole(s, userID, false)
}

func (d *Dungeon) admin(s *discordgo.Session, i *discordgo.InteractionCreate, sub string, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	if !isAdmin(i) {
		d.play(s, i, true, render(NotAdminMsg, MsgData{}))
		return
	}
	switch sub {
	case "channel":
		o := option(opts, "channel")
		if o == nil {
			return
		}
		channelID := o.ChannelValue(nil).ID
		d.setLogChannel(channelID)
		d.play(s, i, true, render(LogChannelSetMsg, MsgData{Channel: channelRef(channelID)}))
	case "pronouns":
		given := map[string]string{}
		for _, name := range pronounOptions {
			if o := option(opts, name); o != nil {
				given[name] = o.StringValue()
			}
		}
		d.play(s, i, true, d.setPronounRoles(guildRolesOf(s, i.GuildID), i.GuildID, given))
	case "clear-pronouns":
		d.clearPronouns()
		d.play(s, i, true, render(PronounsClearedMsg, MsgData{}))
	case "pause":
		d.play(s, i, true, render(PausedMsg, MsgData{Count: d.setPaused(true)}))
	case "unpause":
		d.setPaused(false)
		d.play(s, i, true, render(UnpausedMsg, MsgData{}))
	}
}

func option(opts []*discordgo.ApplicationCommandInteractionDataOption, name string) *discordgo.ApplicationCommandInteractionDataOption {
	for _, o := range opts {
		if o.Name == name {
			return o
		}
	}
	return nil
}

func (d *Dungeon) play(s *discordgo.Session, i *discordgo.InteractionCreate, private bool, steps ...string) {
	for n, body := range steps {
		if n == 0 {
			if err := reply(s, i, body, private); err != nil {
				log.Printf("discord: could not reply: %v", err)
				return
			}
			continue
		}
		d.pause()
		if err := edit(s, i, body); err != nil {
			log.Printf("discord: could not update reply: %v", err)
			return
		}
	}
}

func roomPrompt(room Room, player string) string {
	title, ok := RoomTitles[room.ID]
	if !ok {
		title = UntitledRoomMsg
	}
	data := MsgData{Player: player, Room: room.Name, Text: room.Text}
	return join("\n", render(title, data), render(RoomMsg, data), render(CallMsg, data))
}

func flipSteps(t turn, player string) []string {
	var collect string
	if l := loot(t); l != "" {
		collect = render(CollectMsg, MsgData{Player: player, Loot: l})
	}
	result := join("\n", render(OutcomeMsg, MsgData{Player: player, Cry: t.cry, Text: withItem(withGold(t.text, t.gold), t.item)}), collect)

	if !t.died && !t.escaped {
		onwards := join("\n", result, render(OnwardsMsg, MsgData{Player: player}))
		return []string{result, join("\n\n", onwards, roomPrompt(t.next, player))}
	}
	return []string{result, join("\n\n", result, report(t, player))}
}

func report(t turn, player string) string {
	data := MsgData{Player: player, Gold: t.total, Items: strings.Join(t.items, ", ")}
	if t.escaped {
		return join("\n\n", render(EscapedMsg, data), render(EscapeSpoilsMsg, data))
	}
	return join("\n\n", render(DiedMsg, data), render(DroppedMsg, data))
}

func publicReport(t turn, player string, pronouns Pronouns) string {
	data := MsgData{Player: player, Gold: t.total, Items: strings.Join(t.items, ", ")}
	if t.escaped {
		var story string
		if t.epilogue != nil {
			text := withItem(withGold(render(t.epilogue, MsgData{Pronouns: pronouns}), t.gold), t.item)
			story = render(EpilogueMsg, MsgData{Player: player, Place: t.place, Text: text, Cry: t.cry})
		}
		return join("\n", render(EscapedHeaderMsg, data), join("\n\n", render(PublicEscapedMsg, data), story, render(CollectedMsg, data)))
	}
	obit := t.obituary
	if obit == nil {
		obit = TimeoutObituaryMsg
	}
	death := render(ObituaryMsg, MsgData{Player: player, Place: t.place, Text: render(obit, MsgData{Pronouns: pronouns})})
	pockets := slices.Clone(t.items)
	if t.total > 0 {
		pockets = slices.Insert(pockets, 0, strconv.Itoa(t.total)+" gold")
	}
	dropped := render(PocketsMsg, MsgData{Pronouns: pronouns, Pockets: pockets})
	return join("\n", render(DiedHeaderMsg, data), join("\n\n", render(DiedMsg, data), death, dropped))
}

func loot(t turn) string {
	var found []string
	if t.gold > 0 {
		found = append(found, strconv.Itoa(t.gold)+" gold")
	}
	if t.item != "" {
		found = append(found, t.item)
	}
	return strings.Join(found, ", ")
}
