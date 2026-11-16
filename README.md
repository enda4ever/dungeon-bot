# Dungeon Bot

This is a discord bot. It's about a dungeon crawl.

-----

#### GAMEPLAY LOOP:

Each run draws 4 lethal and 4 non-lethal rooms with no repeats, shuffled except that the last room is always lethal, so a run can only end in death or in escaping past a lethal room (`lethalPerRun` and `safePerRun` in `internal/bot/dungeon.go`). In each room the player calls heads or tails on the humble coin: a right call passes, a wrong one fails, 50/50. Failing a lethal room ends the run; failing a non-lethal one doesn't.

Runs only start in the dungeon channel an admin picks with `/dungeon-admin channel`; `/dungeon crawl` anywhere else is privately pointed there. It announces the player's entry publicly there, opens a private thread for them in that channel, and privately sends them a link to it. Being added to the thread is the notification that takes them there. The whole run happens in that thread. The final report (escaped or died, and any gold and items) is also posted to the dungeon channel, and the thread is deleted.

Escaping earns the 𝔇𝔲𝔫𝔤𝔢𝔬𝔫 ℭ𝔯𝔞𝔴𝔩𝔢𝔯 role, which the player keeps until a later run of theirs ends in death, or 24 hours after their latest escape, whichever comes first. Escaping again while still holding it also earns 𝔇𝔲𝔫𝔤𝔢𝔬𝔫 𝔇𝔢𝔰𝔱𝔯𝔬𝔶𝔢𝔯, which is kept forever.

A room waits for the player's call, and `/dungeon crawl` mid-run points them back to their thread. A run gets 5 minutes from entering: if it isn't over by then it counts as a death (public report, role removed) and its thread is deleted. Runs live in memory, so restarting the bot mid-run ends them without a report and leaves their threads behind; pause first (below) to avoid that.

Rooms live in `events/lethal.json` and `events/non-lethal.json`. A pass marked `gold` awards 1–25 gold, and the amount is written into the pass text in front of the word "gold", so that text has to say gold. One marked `item` awards that item, which the pass text already names.

```txt
/dungeon crawl ─▶ private thread
  ▼
draw an unvisited room ◀── onwards! ────┐
  ▼                                     │
room text                               │
  ▼                                     │
/heads or /tails                        │
  ▼                                     │
coin flip, right call passes            │
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
| `/dungeon crawl` | @user has been thrown in the dungeon! |
| thread opens | your cell header, then: You must escape the dungeon. A coin has been tossed down to you, which bears the mark of the \|x\|umble \|x\|orde. You discover an opening in the corner of your cell, and enter the secret passage... |
| room text | You flip the humble coin. Call `/heads` or `/tails`... |
| pass | Huzzah! / Helm yeah! / Indeed! / Victorious! |
| fail | Curses! / Misfortune! / Alas! / Oh Dear! |
| next room | Onwards! |
| died | the dungeon claims another |
| escaped | user has escaped the dungeon. the \|x\|umble \|x\|orde grows stronger |
| loot found | You collect: (gold and items) |
| user's spoils | escaped: "@user escaped the dungeon with:" Gold, Items (or just "escaped the dungeon."). died: "Dropped:" Gold, Items, only if anything was gathered. Gold and Items lines are each left out when empty, here and in the public posts |
| public death post | @user perished + the lethal room's `where` (e.g. "in {{.Room}}", filled with the room's header text in the header font) + its third-person `obituary` with pronouns, then "Lost to the guards:" and a bulleted list of gold and items, or "<They> died with dust in <their> pockets." if nothing was gathered. A timeout says just "@user perished." + the timeout line |
| public escape post | \|x\|umble \|x\|orde grows stronger, then @user escaped + the last (lethal) room's pass `where` (e.g. "over {{.Room}}") + its third-person `epilogue` with pronouns + their pass word, then "Acquired for the rebellion:" Gold, Items |
| `/dungeon history` | humble stockpile header, the escape total, a bulleted list of all gold and items carried out by escapees (most common first), then the death total (timeouts count). Loot from deaths isn't counted. Public, any channel |

-----

#### DEV SETUP:

Golang.
Docker.
Make.

Fill out `.env` w/ vars from `.env.example`.

`make test` runs the unit tests, `make lint` formats and vets.

#### ADD TO SERVER:

OAuth2, URL Generator, tick `bot` and `applications.commands`, then tick Manage Roles, View Channels, Send Messages, Create Private Threads, Send Messages in Threads and Manage Threads. Open the link and pick your server.

On startup the bot uses the server's 𝔇𝔲𝔫𝔤𝔢𝔬𝔫 ℭ𝔯𝔞𝔴𝔩𝔢𝔯 and 𝔇𝔲𝔫𝔤𝔢𝔬𝔫 𝔇𝔢𝔰𝔱𝔯𝔬𝔶𝔢𝔯 roles, matched by name in any font, or creates them if they don't exist. It can't position roles: in Server Settings, Roles, keep the bot's own role above both.

Turn on Developer Mode (User Settings, Advanced), right click the server, Copy Server ID, and drop that into `.env` as `GUILD_ID`.

Then `make run`, and once it's up, `/dungeon-admin channel #wherever` picks the dungeon channel. The bot's thread permissions only matter there. Until one is picked, the dungeon stays locked.

On startup the bot deletes any dungeon threads left in that channel, e.g. from a crash mid-run. For leaving it up unattended, `make up`, `make logs` to watch it, `make down` to stop it.

If the server has pronoun roles, `/dungeon-admin pronouns` tells the bot which is which, the same as Goat Bot's. Anyone without one is they/them, and somebody with several, or a combo role like they/she, gets one picked at random each time. `/dungeon-admin clear-pronouns` forgets them all.

The dungeon channel, pronoun roles, the pause, and when each current 𝔇𝔲𝔫𝔤𝔢𝔬𝔫 ℭ𝔯𝔞𝔴𝔩𝔢𝔯 earned the role (their user ID and time), the all-time escape and death totals, and the stockpile of escaped loot are saved in `./data`, so they survive `make up`.

The rooms are built into the bot, so `make up` again after editing them.

To roll out changes without cutting anyone's run short: `/dungeon-admin pause`, run it again until it reports 0 runs in progress (at most 5 minutes), `make up`, then `/dungeon-admin unpause`. The pause is saved, so the restart doesn't reopen the dungeon.

-----

#### COMMANDS:
```txt
/dungeon crawl          enter / find thread
/dungeon history        the humble stockpile
/heads                  call heads
/tails                  call tails
/dungeon-admin channel  set dungeon channel
/dungeon-admin pronouns map pronoun roles
/dungeon-admin clear-pronouns
                        forget pronoun roles
/dungeon-admin pause    stop new runs
/dungeon-admin unpause  allow new runs
```
