package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"

	"dungeon-bot/events"
	"dungeon-bot/internal/bot"
)

func main() {
	log.SetFlags(log.LstdFlags)

	cfg, err := bot.LoadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	rooms, err := bot.LoadRooms(events.Lethal, events.NonLethal)
	if err != nil {
		log.Fatalf("events: %v", err)
	}

	store := bot.FileStore{Path: cfg.StatePath}
	state, err := store.Load()
	if err != nil {
		log.Fatalf("state: could not read %s: %v", cfg.StatePath, err)
	}

	session, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		log.Fatalf("discord: %v", err)
	}
	session.Identify.Intents = discordgo.IntentsGuilds

	dungeon := bot.NewDungeon(cfg, rooms, store, state)
	session.AddHandler(dungeon.Route)
	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("discord: connected as %s", r.User.String())
	})

	if err := session.Open(); err != nil {
		log.Fatalf("discord: could not connect: %v", err)
	}
	defer session.Close()

	if _, err := session.ApplicationCommandBulkOverwrite(session.State.User.ID, cfg.GuildID, bot.CommandDefs()); err != nil {
		log.Printf("discord: could not register commands: %v", err)
	} else {
		log.Printf("discord: commands registered for guild %s", cfg.GuildID)
	}

	dungeon.EnsureRole(session)
	dungeon.SweepThreads(session)
	stop := make(chan struct{})
	go dungeon.ExpireRoles(session, stop)

	log.Printf("events: %d rooms loaded", len(rooms))
	if state.LogChannel == "" {
		log.Print("state: no dungeon channel set, so the dungeon is locked until /dungeon-admin channel is run")
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	close(stop)
	log.Print("shutting down")
}
