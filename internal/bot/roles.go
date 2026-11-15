package bot

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"golang.org/x/text/unicode/norm"
)

const (
	crawlerRoleName   = "𝔇𝔲𝔫𝔤𝔢𝔬𝔫 ℭ𝔯𝔞𝔴𝔩𝔢𝔯"
	destroyerRoleName = "𝔇𝔲𝔫𝔤𝔢𝔬𝔫 𝔇𝔢𝔰𝔱𝔯𝔬𝔶𝔢𝔯"
	roleLifetime      = 24 * time.Hour
)

type guildRoles interface {
	GuildRoles(guildID string, options ...discordgo.RequestOption) ([]*discordgo.Role, error)
	GuildRoleCreate(guildID string, data *discordgo.RoleParams, options ...discordgo.RequestOption) (*discordgo.Role, error)
	GuildMemberRoleAdd(guildID, userID, roleID string, options ...discordgo.RequestOption) error
	GuildMemberRoleRemove(guildID, userID, roleID string, options ...discordgo.RequestOption) error
}

func plainText(s string) string {
	return strings.ToLower(norm.NFKC.String(s))
}

func (d *Dungeon) EnsureRole(dg guildRoles) {
	if _, err := d.crawlerRole(dg); err != nil {
		log.Printf("discord: %s role unavailable: %v", crawlerRoleName, err)
	}
	if _, err := d.destroyerRole(dg); err != nil {
		log.Printf("discord: %s role unavailable: %v", destroyerRoleName, err)
	}
}

func crawlerSlot(s *State) *string   { return &s.CrawlerRole }
func destroyerSlot(s *State) *string { return &s.DestroyerRole }

func (d *Dungeon) crawlerRole(dg guildRoles) (string, error) {
	return d.ensureRole(dg, crawlerRoleName, crawlerSlot)
}

func (d *Dungeon) destroyerRole(dg guildRoles) (string, error) {
	return d.ensureRole(dg, destroyerRoleName, destroyerSlot)
}

func (d *Dungeon) ensureRole(dg guildRoles, name string, slot func(*State) *string) (string, error) {
	d.roleMu.Lock()
	defer d.roleMu.Unlock()

	roles, err := dg.GuildRoles(d.cfg.GuildID)
	if err != nil {
		return "", err
	}
	if stored := d.storedRole(slot); stored != "" {
		for _, r := range roles {
			if r.ID == stored {
				return stored, nil
			}
		}
	}
	for _, r := range roles {
		if plainText(r.Name) == plainText(name) {
			d.storeRole(slot, r.ID)
			return r.ID, nil
		}
	}

	off := false
	role, err := dg.GuildRoleCreate(d.cfg.GuildID, &discordgo.RoleParams{
		Name:        name,
		Hoist:       &off,
		Mentionable: &off,
	})
	if err != nil {
		return "", err
	}
	d.storeRole(slot, role.ID)
	return role.ID, nil
}

func (d *Dungeon) storedRole(slot func(*State) *string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return *slot(d.state)
}

func (d *Dungeon) storeRole(slot func(*State) *string, roleID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	*slot(d.state) = roleID
	d.save()
}

func (d *Dungeon) grantDestroyer(dg guildRoles, userID string) {
	roleID, err := d.destroyerRole(dg)
	if err != nil {
		log.Printf("discord: %s role unavailable: %v", destroyerRoleName, err)
		return
	}
	if err := dg.GuildMemberRoleAdd(d.cfg.GuildID, userID, roleID); err != nil {
		log.Printf("discord: could not give the %s role to %s: %v", destroyerRoleName, userID, err)
	}
}

func (d *Dungeon) updateCrawlerRole(dg guildRoles, userID string, escaped bool) {
	roleID, err := d.crawlerRole(dg)
	if err != nil {
		log.Printf("discord: %s role unavailable: %v", crawlerRoleName, err)
		return
	}
	if escaped {
		if err := dg.GuildMemberRoleAdd(d.cfg.GuildID, userID, roleID); err != nil {
			log.Printf("discord: could not give the %s role to %s: %v", crawlerRoleName, userID, err)
			return
		}
		d.markCrawler(userID, d.now())
		return
	}
	if err := dg.GuildMemberRoleRemove(d.cfg.GuildID, userID, roleID); err != nil && !isNotFound(err) {
		log.Printf("discord: could not take the %s role from %s: %v", crawlerRoleName, userID, err)
		return
	}
	d.clearCrawler(userID)
}

func (d *Dungeon) ExpireRoles(dg guildRoles, stop <-chan struct{}) {
	tick := time.NewTicker(5 * time.Minute)
	defer tick.Stop()
	d.expireRoles(dg)
	for {
		select {
		case <-tick.C:
			d.expireRoles(dg)
		case <-stop:
			return
		}
	}
}

func (d *Dungeon) expireRoles(dg guildRoles) {
	for _, userID := range d.expiredCrawlers(d.now()) {
		d.updateCrawlerRole(dg, userID, false)
	}
}

func (d *Dungeon) markCrawler(userID string, at time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.state.CrawlerSince == nil {
		d.state.CrawlerSince = map[string]time.Time{}
	}
	d.state.CrawlerSince[userID] = at
	d.save()
}

func (d *Dungeon) clearCrawler(userID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.state.CrawlerSince[userID]; !ok {
		return
	}
	delete(d.state.CrawlerSince, userID)
	d.save()
}

func (d *Dungeon) expiredCrawlers(now time.Time) []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var expired []string
	for userID, since := range d.state.CrawlerSince {
		if now.Sub(since) >= roleLifetime {
			expired = append(expired, userID)
		}
	}
	return expired
}

func isNotFound(err error) bool {
	var rest *discordgo.RESTError
	return errors.As(err, &rest) && rest.Response != nil && rest.Response.StatusCode == http.StatusNotFound
}
