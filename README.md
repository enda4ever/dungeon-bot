# Dungeon Bot

This is a discord bot. It's about a dungeon crawl.

-----

#### GAMEPLAY LOOP:

Rooms are drawn in random order and never repeat within a run. Every room is a 50/50 pass or fail. Failing a lethal room ends the run; failing a non-lethal one doesn't.

Rooms live in `events/lethal.json` and `events/non-lethal.json`. A pass marked `gold` awards 1–25 gold; one marked `item` awards that item.

```txt
/dungeon crawl
  ▼
draw an unvisited room ◀────────────────┐
  ▼                                     │
room text                               │
  ▼                                     │
coin flip, 50/50                        │
  ├─ pass ─▶ pass text,                 │
  │          gold/item if any ─┐        │
  └─ fail ─▶ fail text         │        │
               ├─ non-lethal ──┤        │
               │ lethal        ▼        │ yes
               │           rooms left? ─┘
               │               │ no
               ▼               ▼
              died          escaped
               │               │
               └───────┬───────┘
                       ▼
                 user's spoils
```

| Step | Message |
|---|---|
| `/dungeon crawl` | user has entered the dungeon |
| coin flip | user flips the humble coin |
| died | the dungeon claims another |
| escaped | user has escaped the dungeon. the humble horde grows in numbers |
| user's spoils | Gold, Found Items, Monsters Slain (eventually) |
