package bot

import (
	"errors"
	"log"
	"net/url"
	"time"

	"github.com/bwmarrin/discordgo"
)

func withoutURL(err error) error {
	var u *url.Error
	if errors.As(err, &u) {
		return u.Err
	}
	return err
}

func reply(s *discordgo.Session, i *discordgo.InteractionCreate, body string, private bool) error {
	data := &discordgo.InteractionResponseData{
		Content:         body,
		AllowedMentions: noPings(),
	}
	if private {
		data.Flags = discordgo.MessageFlagsEphemeral
	}
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: data,
	})
}

func deferPublic(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
}

func whisper(s *discordgo.Session, i *discordgo.InteractionCreate, body string) error {
	_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content:         body,
		AllowedMentions: noPings(),
		Flags:           discordgo.MessageFlagsEphemeral,
	})
	return err
}

func edit(s *discordgo.Session, i *discordgo.InteractionCreate, body string) error {
	_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content:         &body,
		AllowedMentions: noPings(),
	})
	return err
}

func post(s *discordgo.Session, channelID, body string, pings ...string) {
	if channelID == "" {
		return
	}
	mentions := noPings()
	mentions.Users = pings
	_, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content:         body,
		AllowedMentions: mentions,
	})
	if err != nil {
		log.Printf("discord: could not post to channel %s: %v", channelID, err)
	}
}

func openThread(s *discordgo.Session, channelID, userID, name string) (string, error) {
	if runes := []rune(name); len(runes) > 100 {
		name = string(runes[:100])
	}
	th, err := s.ThreadStartComplex(channelID, &discordgo.ThreadStart{
		Name:                name,
		AutoArchiveDuration: 60,
		Type:                discordgo.ChannelTypeGuildPrivateThread,
	})
	if err != nil {
		return "", err
	}
	return th.ID, s.ThreadMemberAdd(th.ID, userID)
}

func (d *Dungeon) SweepThreads(s *discordgo.Session) {
	channelID := d.logChannel()
	if channelID == "" || s.State.User == nil {
		return
	}
	botID := s.State.User.ID
	var leftover []string

	active, err := s.GuildThreadsActive(d.cfg.GuildID)
	if err != nil {
		log.Printf("discord: could not list active threads: %v", err)
	} else {
		for _, th := range active.Threads {
			if th.ParentID == channelID && th.OwnerID == botID {
				leftover = append(leftover, th.ID)
			}
		}
	}

	var before *time.Time
	for {
		archived, err := s.ThreadsPrivateArchived(channelID, before, 100)
		if err != nil {
			log.Printf("discord: could not list archived threads: %v", err)
			break
		}
		var last *time.Time
		for _, th := range archived.Threads {
			if th.OwnerID == botID {
				leftover = append(leftover, th.ID)
			}
			if th.ThreadMetadata != nil {
				at := th.ThreadMetadata.ArchiveTimestamp
				last = &at
			}
		}
		if !archived.HasMore || last == nil {
			break
		}
		before = last
	}

	for _, id := range leftover {
		deleteThread(s, id)
	}
	if len(leftover) > 0 {
		log.Printf("discord: deleted %d leftover dungeon threads", len(leftover))
	}
}

func deleteThread(s *discordgo.Session, threadID string) {
	if _, err := s.ChannelDelete(threadID); err != nil {
		log.Printf("discord: could not delete thread %s: %v", threadID, err)
	}
}

func noPings() *discordgo.MessageAllowedMentions {
	return &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}
}

func callerID(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

func memberRoles(i *discordgo.InteractionCreate) []string {
	if i.Member == nil {
		return nil
	}
	return i.Member.Roles
}

func displayName(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.DisplayName()
	}
	if i.User != nil {
		return i.User.Username
	}
	return ""
}

func isAdmin(i *discordgo.InteractionCreate) bool {
	if i.Member == nil {
		return false
	}
	const mask = discordgo.PermissionManageServer | discordgo.PermissionAdministrator
	return i.Member.Permissions&mask != 0
}

func mention(userID string) string {
	return "<@" + userID + ">"
}

func channelLink(guildID, channelID string) string {
	return "https://discord.com/channels/" + guildID + "/" + channelID
}

func channelRef(channelID string) string {
	if channelID == "" {
		return ""
	}
	return "<#" + channelID + ">"
}
