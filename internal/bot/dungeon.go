package bot

import (
	"errors"
	"log"
	"math/rand/v2"
	"slices"
	"sync"
	"text/template"
	"time"
)

const (
	lethalPerRun = 4
	safePerRun   = 4
	maxGold      = 25
	runLimit     = 5 * time.Minute
)

var (
	ErrNotInside = errors.New("player is not in the dungeon")
	ErrElsewhere = errors.New("player's run is in another channel")
	ErrCoinInAir = errors.New("player's last coin is still in the air")
)

var sides = [2]string{"heads", "tails"}

type dice interface {
	Perm(n int) []int
	IntN(n int) int
}

type fairDice struct{}

func (fairDice) Perm(n int) []int { return rand.Perm(n) }
func (fairDice) IntN(n int) int   { return rand.IntN(n) }

type run struct {
	order  []int
	at     int
	gold   int
	items  []string
	busy   bool
	thread string
	opened time.Time
}

type turn struct {
	thread  string
	call    string
	landed  string
	passed  bool
	cry     string
	text    string
	gold    int
	item    string
	died    bool
	escaped bool
	next    Room
	rooms   int
	total   int
	items   []string

	obituary *template.Template
	epilogue *template.Template
	place    string
}

type Dungeon struct {
	cfg   Config
	rooms []Room
	dice  dice
	beat  time.Duration
	now   func() time.Time
	store Store

	mu    sync.Mutex
	runs  map[string]*run
	state *State

	roleMu sync.Mutex
}

func NewDungeon(cfg Config, rooms []Room, store Store, state *State) *Dungeon {
	return &Dungeon{
		cfg:   cfg,
		rooms: rooms,
		dice:  fairDice{},
		beat:  2 * time.Second,
		now:   time.Now,
		store: store,
		runs:  map[string]*run{},
		state: state,
	}
}

func (d *Dungeon) enter(userID string) (Room, string, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if r, ok := d.runs[userID]; ok && d.now().Sub(r.opened) < runLimit {
		return d.rooms[r.order[r.at]], r.thread, false
	}
	r := &run{order: d.draw(), opened: d.now()}
	d.runs[userID] = r
	return d.rooms[r.order[0]], "", true
}

func (d *Dungeon) attach(userID, threadID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if r, ok := d.runs[userID]; ok {
		r.thread = threadID
	}
}

func (d *Dungeon) leave(userID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.runs, userID)
}

func (d *Dungeon) draw() []int {
	var lethal, safe []int
	for n, room := range d.rooms {
		if room.Lethal {
			lethal = append(lethal, n)
		} else {
			safe = append(safe, n)
		}
	}
	picked := append(d.pick(safe, safePerRun), d.pick(lethal, lethalPerRun)...)
	var last []int
	if len(picked) > 0 && d.rooms[picked[len(picked)-1]].Lethal {
		picked, last = picked[:len(picked)-1], picked[len(picked)-1:]
	}
	order := make([]int, len(picked))
	for i, p := range d.dice.Perm(len(picked)) {
		order[i] = picked[p]
	}
	return append(order, last...)
}

func (d *Dungeon) pick(pool []int, n int) []int {
	var out []int
	for _, p := range d.dice.Perm(len(pool))[:min(n, len(pool))] {
		out = append(out, pool[p])
	}
	return out
}

func (d *Dungeon) call(userID, channelID, side string) (turn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	r, ok := d.runs[userID]
	switch {
	case !ok:
		return turn{}, ErrNotInside
	case r.thread != channelID:
		return turn{thread: r.thread}, ErrElsewhere
	case r.busy:
		return turn{}, ErrCoinInAir
	}

	room := d.rooms[r.order[r.at]]
	t := turn{thread: r.thread, call: side, landed: sides[d.dice.IntN(2)]}
	t.passed = t.landed == side
	outcome, cries := room.Fail, FailCries
	if t.passed {
		outcome, cries = room.Pass, PassCries
	}
	t.cry = cries[d.dice.IntN(len(cries))]
	t.text = outcome.Text
	if outcome.Gold {
		t.gold = 1 + d.dice.IntN(maxGold)
		r.gold += t.gold
	}
	if outcome.Item != "" {
		t.item = outcome.Item
		r.items = append(r.items, outcome.Item)
	}
	r.at++

	t.died = room.Lethal && !t.passed
	t.escaped = !t.died && r.at == len(r.order)
	if t.died || t.escaped {
		t.rooms, t.total, t.items = r.at, r.gold, slices.Clone(r.items)
		if t.died {
			t.rooms--
			t.obituary = room.obituary
			if room.where != nil {
				t.place = render(room.where, MsgData{Room: fancy(room.title())})
			}
		} else {
			t.epilogue = room.epilogue
			if room.escapeWhere != nil {
				t.place = render(room.escapeWhere, MsgData{Room: fancy(room.title())})
			}
		}
		delete(d.runs, userID)
		return t, nil
	}
	t.next = d.rooms[r.order[r.at]]
	r.busy = true
	return t, nil
}

func (d *Dungeon) expire(userID, threadID string) (turn, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	r, ok := d.runs[userID]
	if !ok || r.thread != threadID {
		return turn{}, false
	}
	delete(d.runs, userID)
	return turn{thread: threadID, died: true, rooms: r.at, total: r.gold, items: slices.Clone(r.items)}, true
}

func (d *Dungeon) settle(userID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if r, ok := d.runs[userID]; ok {
		r.busy = false
	}
}

func (d *Dungeon) logChannel() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state.LogChannel
}

func (d *Dungeon) closedTo(userID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.state.LogChannel == "" {
		return true
	}
	if !d.state.Paused {
		return false
	}
	r, ok := d.runs[userID]
	return !ok || d.now().Sub(r.opened) >= runLimit
}

func (d *Dungeon) setPaused(paused bool) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.state.Paused = paused
	d.save()
	live := 0
	for _, r := range d.runs {
		if d.now().Sub(r.opened) < runLimit {
			live++
		}
	}
	return live
}

func (d *Dungeon) entrance(channelID string) (string, bool) {
	dungeon := d.logChannel()
	return dungeon, dungeon == "" || channelID == dungeon
}

func (d *Dungeon) setLogChannel(channelID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.state.LogChannel = channelID
	d.save()
}

func (d *Dungeon) crawlerRoleID() string {
	return d.storedRole(crawlerSlot)
}

func (d *Dungeon) save() {
	if err := d.store.Save(d.state); err != nil {
		log.Printf("state: save failed: %v", err)
	}
}

func (d *Dungeon) pause() {
	time.Sleep(d.beat)
}
