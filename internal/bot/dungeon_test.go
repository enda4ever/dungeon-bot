package bot

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/bwmarrin/discordgo"
	"golang.org/x/text/unicode/norm"

	"dungeon-bot/events"
)

const testThread = "thread"

type loadedDice struct {
	rolls []int
}

func (l *loadedDice) Perm(n int) []int {
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	return order
}

func (l *loadedDice) IntN(n int) int {
	if len(l.rolls) == 0 {
		return 0
	}
	roll := l.rolls[0]
	l.rolls = l.rolls[1:]
	return roll % n
}

type memStore struct {
	saved *State
}

func (m *memStore) Load() (*State, error) {
	if m.saved == nil {
		return &State{}, nil
	}
	return m.saved, nil
}

func (m *memStore) Save(s *State) error {
	kept := *s
	m.saved = &kept
	return nil
}

type fakeRoles struct {
	roles   []*discordgo.Role
	held    map[string]bool
	created int
}

func newFakeRoles(roles ...*discordgo.Role) *fakeRoles {
	return &fakeRoles{roles: roles, held: map[string]bool{}}
}

func (f *fakeRoles) GuildRoles(guildID string, options ...discordgo.RequestOption) ([]*discordgo.Role, error) {
	return f.roles, nil
}

func (f *fakeRoles) GuildRoleCreate(guildID string, data *discordgo.RoleParams, options ...discordgo.RequestOption) (*discordgo.Role, error) {
	f.created++
	r := &discordgo.Role{ID: "new-" + strconv.Itoa(f.created), Name: data.Name}
	f.roles = append(f.roles, r)
	return r, nil
}

func (f *fakeRoles) GuildMemberRoleAdd(guildID, userID, roleID string, options ...discordgo.RequestOption) error {
	f.held[userID+"/"+roleID] = true
	return nil
}

func (f *fakeRoles) GuildMemberRoleRemove(guildID, userID, roleID string, options ...discordgo.RequestOption) error {
	delete(f.held, userID+"/"+roleID)
	return nil
}

func testRoom(id string, lethal bool, pass Outcome) Room {
	return Room{
		ID:     id,
		Name:   id,
		Text:   id + " text",
		Pass:   pass,
		Fail:   Outcome{Text: id + " fail"},
		Lethal: lethal,
	}
}

func testDungeon(rooms []Room, rolls ...int) *Dungeon {
	d := NewDungeon(Config{GuildID: "g"}, rooms, &memStore{}, &State{})
	d.dice = &loadedDice{rolls: rolls}
	d.beat = 0
	return d
}

func begin(t *testing.T, d *Dungeon) Room {
	t.Helper()
	room, _, fresh := d.enter("p")
	if !fresh {
		t.Fatal("entered an existing run, want a fresh one")
	}
	d.attach("p", testThread)
	return room
}

func callCoin(t *testing.T, d *Dungeon, side string) turn {
	t.Helper()
	got, err := d.call("p", testThread, side)
	if err != nil {
		t.Fatalf("calling %s: %v", side, err)
	}
	d.settle("p")
	return got
}

func TestShippedRoomsLoad(t *testing.T) {
	rooms, err := LoadRooms(events.Lethal, events.NonLethal)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	lethal := 0
	for _, r := range rooms {
		if r.Lethal {
			lethal++
		}
	}
	if lethal < lethalPerRun || len(rooms)-lethal < safePerRun {
		t.Errorf("%d lethal and %d non-lethal rooms, want at least %d and %d", lethal, len(rooms)-lethal, lethalPerRun, safePerRun)
	}
}

func TestEveryShippedRoomHasATitle(t *testing.T) {
	rooms, err := LoadRooms(events.Lethal, events.NonLethal)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	ids := map[string]bool{}
	for _, r := range rooms {
		ids[r.ID] = true
		if RoomTitles[r.ID] == nil {
			t.Errorf("room %q has no title in RoomTitles", r.ID)
		}
	}
	for id := range RoomTitles {
		if !ids[id] {
			t.Errorf("RoomTitles has %q, which is not a room", id)
		}
	}
}

func TestRightCallsEscapeWithTheLoot(t *testing.T) {
	d := testDungeon([]Room{
		testRoom("a", false, Outcome{Text: "coins", Gold: true}),
		testRoom("b", true, Outcome{Text: "ruby", Item: "Ruby"}),
		testRoom("c", true, Outcome{Text: "nothing"}),
	}, 0, 0, 9)
	if room := begin(t, d); room.ID != "a" {
		t.Fatalf("entered %s, want a", room.ID)
	}

	first := callCoin(t, d, "heads")
	if !first.passed || first.gold != 10 || first.next.ID != "b" {
		t.Errorf("first call = %+v, want a pass worth 10 gold leading to b", first)
	}
	callCoin(t, d, "heads")
	last := callCoin(t, d, "heads")
	if !last.escaped || last.died || last.rooms != 3 {
		t.Errorf("last call = %+v, want an escape after 3 rooms", last)
	}
	if last.total != 10 || strings.Join(last.items, ",") != "Ruby" {
		t.Errorf("spoils = %d gold and %v, want 10 and [Ruby]", last.total, last.items)
	}
	if _, err := d.call("p", testThread, "heads"); !errors.Is(err, ErrNotInside) {
		t.Errorf("call after escaping = %v, want ErrNotInside", err)
	}
}

func TestWrongCallInALethalRoomKills(t *testing.T) {
	d := testDungeon([]Room{
		testRoom("a", false, Outcome{Text: "pass"}),
		testRoom("b", true, Outcome{Text: "pass"}),
		testRoom("c", true, Outcome{Text: "pass"}),
	}, 1, 0, 1)
	begin(t, d)

	first := callCoin(t, d, "heads")
	if first.passed || first.died || first.next.ID != "b" {
		t.Errorf("first call = %+v, want a survivable fail leading to b", first)
	}
	second := callCoin(t, d, "heads")
	if !second.died || second.escaped || second.rooms != 1 {
		t.Errorf("second call = %+v, want death in the 2nd room with 1 room passed", second)
	}
	if _, err := d.call("p", testThread, "heads"); !errors.Is(err, ErrNotInside) {
		t.Errorf("call after dying = %v, want ErrNotInside", err)
	}
}

func TestARunEndsAfterItsLastRoom(t *testing.T) {
	var rooms []Room
	for n := range 12 {
		rooms = append(rooms, testRoom(strconv.Itoa(n), n%2 == 0, Outcome{Text: "pass"}))
	}
	d := testDungeon(rooms)
	begin(t, d)

	for n := 1; n < lethalPerRun+safePerRun; n++ {
		if got := callCoin(t, d, "heads"); got.escaped {
			t.Fatalf("escaped after %d rooms, want %d", n, lethalPerRun+safePerRun)
		}
	}
	if got := callCoin(t, d, "heads"); !got.escaped {
		t.Errorf("last room = %+v, want an escape", got)
	}
}

func TestEveryRunDrawsTheSameMixWithoutRepeats(t *testing.T) {
	rooms, err := LoadRooms(events.Lethal, events.NonLethal)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	d := NewDungeon(Config{}, rooms, &memStore{}, &State{})

	for n := range 200 {
		player := strconv.Itoa(n)
		d.enter(player)
		order := d.runs[player].order
		lethal := 0
		seen := map[int]bool{}
		for _, r := range order {
			if seen[r] {
				t.Fatalf("room %d drawn twice in %v", r, order)
			}
			seen[r] = true
			if rooms[r].Lethal {
				lethal++
			}
		}
		if len(order) != lethalPerRun+safePerRun || lethal != lethalPerRun {
			t.Fatalf("run has %d rooms, %d lethal, want %d and %d", len(order), lethal, lethalPerRun+safePerRun, lethalPerRun)
		}
		if !rooms[order[len(order)-1]].Lethal {
			t.Fatalf("run %v ends on a non-lethal room, want the last room lethal", order)
		}
	}
}

func TestCallsOnlyCountInTheRunsThread(t *testing.T) {
	d := testDungeon([]Room{
		testRoom("a", false, Outcome{Text: "pass"}),
		testRoom("b", false, Outcome{Text: "pass"}),
	})
	begin(t, d)

	got, err := d.call("p", "elsewhere", "heads")
	if !errors.Is(err, ErrElsewhere) || got.thread != testThread {
		t.Errorf("call outside the thread = %v pointing at %q, want ErrElsewhere pointing at %q", err, got.thread, testThread)
	}
	if next := callCoin(t, d, "heads"); next.next.ID != "b" {
		t.Errorf("call in the thread led to %s, want b, so the stray call cost nothing", next.next.ID)
	}
}

func TestCoinStillInTheAir(t *testing.T) {
	d := testDungeon([]Room{
		testRoom("a", false, Outcome{Text: "pass"}),
		testRoom("b", false, Outcome{Text: "pass"}),
	})
	begin(t, d)

	if _, err := d.call("p", testThread, "heads"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.call("p", testThread, "tails"); !errors.Is(err, ErrCoinInAir) {
		t.Errorf("call mid-flip = %v, want ErrCoinInAir", err)
	}
	d.settle("p")
	if _, err := d.call("p", testThread, "tails"); err != nil {
		t.Errorf("call after the coin landed = %v, want it to go through", err)
	}
}

func TestCrawlingMidRunPointsBackToTheThread(t *testing.T) {
	d := testDungeon([]Room{
		testRoom("a", false, Outcome{Text: "pass"}),
		testRoom("b", false, Outcome{Text: "pass"}),
	})
	begin(t, d)
	callCoin(t, d, "heads")

	if room, thread, fresh := d.enter("p"); fresh || room.ID != "b" || thread != testThread {
		t.Errorf("crawl mid-run = %s in %q (fresh=%v), want b in %q", room.ID, thread, fresh, testThread)
	}
}

func TestRunsOlderThanTheLimitStartOver(t *testing.T) {
	d := testDungeon([]Room{
		testRoom("a", false, Outcome{Text: "pass"}),
		testRoom("b", false, Outcome{Text: "pass"}),
	})
	clock := time.Now()
	d.now = func() time.Time { return clock }
	begin(t, d)
	callCoin(t, d, "heads")

	clock = clock.Add(runLimit - time.Second)
	if _, _, fresh := d.enter("p"); fresh {
		t.Error("a run under the limit started over")
	}
	clock = clock.Add(time.Second)
	if room, _, fresh := d.enter("p"); !fresh || room.ID != "a" {
		t.Errorf("crawl at the limit = %s (fresh=%v), want a fresh run from a", room.ID, fresh)
	}
}

func TestPausingStopsNewRunsButLetsLiveOnesFinish(t *testing.T) {
	store := &memStore{}
	d := NewDungeon(Config{}, []Room{testRoom("a", false, Outcome{Text: "pass"})}, store, &State{LogChannel: "dungeon"})
	d.enter("playing")

	if live := d.setPaused(true); live != 1 {
		t.Errorf("pause reported %d live runs, want 1", live)
	}
	if !d.closedTo("newcomer") {
		t.Error("a new player got in while paused")
	}
	if d.closedTo("playing") {
		t.Error("a player mid-run was locked out by the pause")
	}
	if store.saved == nil || !store.saved.Paused {
		t.Error("the pause was not saved, so a restart would reopen the dungeon")
	}
	d.setPaused(false)
	if d.closedTo("newcomer") {
		t.Error("still closed after unpausing")
	}
}

func TestTheDungeonIsLockedUntilAChannelIsPicked(t *testing.T) {
	d := testDungeon(nil)
	if !d.closedTo("newcomer") {
		t.Error("a player got in before any dungeon channel was picked")
	}
	d.setLogChannel("dungeon")
	if d.closedTo("newcomer") {
		t.Error("still locked after the dungeon channel was picked")
	}
}

func TestTimeUpEndsTheRunAsADeath(t *testing.T) {
	d := testDungeon([]Room{
		testRoom("a", false, Outcome{Text: "coins", Gold: true}),
		testRoom("b", false, Outcome{Text: "pass"}),
		testRoom("c", false, Outcome{Text: "pass"}),
	}, 0, 0, 4)
	begin(t, d)
	callCoin(t, d, "heads")

	if _, ok := d.expire("p", "an older thread"); ok {
		t.Error("a timer from another thread ended this run")
	}
	got, ok := d.expire("p", testThread)
	if !ok || !got.died || got.rooms != 1 || got.total != 5 {
		t.Errorf("expire = %+v (ok=%v), want a death after 1 room with 5 gold", got, ok)
	}
	if _, err := d.call("p", testThread, "heads"); !errors.Is(err, ErrNotInside) {
		t.Errorf("call after time ran out = %v, want ErrNotInside", err)
	}
	if _, ok := d.expire("p", testThread); ok {
		t.Error("expired the same run twice")
	}
}

func TestRunsOnlyStartInTheDungeonChannel(t *testing.T) {
	d := testDungeon(nil)
	if _, ok := d.entrance("anywhere"); !ok {
		t.Error("with no dungeon channel set, a crawl was refused")
	}
	d.setLogChannel("dungeon")
	if _, ok := d.entrance("dungeon"); !ok {
		t.Error("a crawl in the dungeon channel was refused")
	}
	if got, ok := d.entrance("general"); ok || got != "dungeon" {
		t.Errorf("a crawl elsewhere = %q, %v; want refused and pointed at dungeon", got, ok)
	}
}

func TestLogChannelIsSaved(t *testing.T) {
	store := &memStore{}
	d := NewDungeon(Config{}, nil, store, &State{})
	d.setLogChannel("log")

	if d.logChannel() != "log" || store.saved == nil || store.saved.LogChannel != "log" {
		t.Errorf("log channel = %q, saved %+v, want log in both", d.logChannel(), store.saved)
	}
}

func TestGoldRangesFromOneToTwentyFive(t *testing.T) {
	for roll, want := range map[int]int{0: 1, maxGold - 1: maxGold} {
		d := testDungeon([]Room{testRoom("a", false, Outcome{Text: "coins", Gold: true})}, 0, 0, roll)
		begin(t, d)
		if got := callCoin(t, d, "heads"); got.gold != want {
			t.Errorf("roll %d gave %d gold, want %d", roll, got.gold, want)
		}
	}
}

func TestCriesFollowTheCoin(t *testing.T) {
	cases := []struct {
		rolls []int
		want  string
	}{
		{[]int{0, 0}, PassCries[0]},
		{[]int{0, 1}, PassCries[1]},
		{[]int{1, 0}, FailCries[0]},
		{[]int{1, 1}, FailCries[1]},
	}
	for _, c := range cases {
		d := testDungeon([]Room{testRoom("a", false, Outcome{Text: "pass"})}, c.rolls...)
		begin(t, d)
		if got := callCoin(t, d, "heads"); got.cry != c.want {
			t.Errorf("rolls %v cried %q, want %q", c.rolls, got.cry, c.want)
		}
	}
}

func TestFlipRevealsInSteps(t *testing.T) {
	steps := flipSteps(turn{
		call:   "heads",
		landed: "heads",
		passed: true,
		text:   "coins",
		gold:   5,
		next:   testRoom("b", false, Outcome{Text: "pass"}),
	}, "@p")

	if len(steps) != 2 || !strings.HasPrefix(steps[1], steps[0]) {
		t.Fatalf("steps = %q, want two, the second extending the first", steps)
	}
	if !strings.Contains(steps[1], "b text") {
		t.Errorf("last step = %q, want it to end on the next room", steps[1])
	}
}

func TestOnwardsOnlyLeadsToAnotherRoom(t *testing.T) {
	onwards := render(OnwardsMsg, MsgData{})

	steps := flipSteps(turn{next: testRoom("b", false, Outcome{Text: "pass"})}, "@p")
	if !strings.Contains(steps[1], onwards) {
		t.Errorf("last step = %q, want %q before the next room", steps[1], onwards)
	}
	for _, end := range []turn{{died: true}, {escaped: true}} {
		steps := flipSteps(end, "@p")
		if strings.Contains(steps[1], onwards) {
			t.Errorf("last step = %q, want no %q when the run is over", steps[1], onwards)
		}
	}
}

func TestTheFinalReportEndsTheThreadReply(t *testing.T) {
	end := turn{died: true, rooms: 3, total: 7, items: []string{"Ruby"}}
	steps := flipSteps(end, "@p")
	if !strings.HasSuffix(steps[1], report(end, "@p")) {
		t.Errorf("last step = %q, want it to end with the thread's report", steps[1])
	}
}

func TestThePublicEscapePostTellsTheLastRoom(t *testing.T) {
	rooms, err := LoadRooms(events.Lethal, events.NonLethal)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	byID := map[string]Room{}
	for _, r := range rooms {
		byID[r.ID] = r
	}

	door := byID["trap-door"]
	place := render(door.escapeWhere, MsgData{Room: fancy(door.title())})
	escape := turn{escaped: true, cry: "Huzzah!", total: 18, items: []string{"Rusty Knife"}, epilogue: door.epilogue, place: place}
	post := publicReport(escape, "@p", Pronouns{set: "he"})
	want := "@p escaped over " + fancy("the trap door") + ". He noticed a trap door in time and hopped over it before continuing down the hallway. Huzzah!"
	if !strings.Contains(post, want) || !strings.Contains(post, "Acquired for the rebellion:") || strings.Contains(post, "@p has escaped") {
		t.Errorf("public escape post = %q, want %q and the collected loot, without the old escaped line", post, want)
	}

	kitchen := turn{escaped: true, cry: "Indeed!", gold: 9, total: 9, epilogue: byID["kitchen"].epilogue}
	if post := publicReport(kitchen, "@p", Pronouns{}); !strings.Contains(post, fancy("9 gold")) {
		t.Errorf("public escape post = %q, want the last room's gold in the special font", post)
	}
}

func TestThePublicDeathPostTellsTheStoryWithPronouns(t *testing.T) {
	rooms, err := LoadRooms(events.Lethal, events.NonLethal)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	var library Room
	for _, r := range rooms {
		if r.ID == "library" {
			library = r
		}
	}
	place := render(library.where, MsgData{Room: fancy(library.title())})
	death := turn{died: true, total: 4, items: []string{"Ruby"}, obituary: library.obituary, place: place}

	she := publicReport(death, "@p", Pronouns{set: "she"})
	want := "@p perished in " + fancy("the library") + ". She pulled the wrong book"
	pockets := "Lost to the guards:\n- " + fancy("4 gold") + "\n- " + fancy("Ruby")
	if !strings.Contains(she, want) || !strings.Contains(she, "She was crushed") || !strings.HasSuffix(she, pockets) {
		t.Errorf("public death post = %q, want %q, she/her throughout, then %q", she, want, pockets)
	}
	if they := publicReport(death, "@p", Pronouns{}); !strings.Contains(they, "They were crushed") {
		t.Errorf("public death post = %q, want they/were for someone without pronoun roles", they)
	}

	timeout := publicReport(turn{died: true}, "@p", Pronouns{set: "he"})
	if !strings.Contains(timeout, "@p perished. He lingered in the dungeon too long.") || strings.Contains(timeout, "Dropped:") || !strings.Contains(timeout, "He died with dust in his pockets.") {
		t.Errorf("timeout post = %q, want the timeout obituary and dusty pockets instead of an empty dropped list", timeout)
	}
	if strings.Contains(she, "dust in") {
		t.Errorf("public death post = %q, want no dusty pockets when loot was dropped", she)
	}
}

func TestEveryLethalRoomSaysWhereTheyPerishedOrEscaped(t *testing.T) {
	rooms, err := LoadRooms(events.Lethal, events.NonLethal)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	for _, r := range rooms {
		if !r.Lethal {
			continue
		}
		for label, where := range map[string]*template.Template{"death": r.where, "escape": r.escapeWhere} {
			if place := render(where, MsgData{Room: fancy(r.title())}); !strings.Contains(place, fancy(r.title())) {
				t.Errorf("room %q %s place = %q, want it to name %s in the header font", r.ID, label, place, fancy(r.title()))
			}
		}
	}
}

func TestEscapingGrantsTheRoleUntilADeath(t *testing.T) {
	dg := newFakeRoles()
	d := testDungeon(nil)

	d.updateCrawlerRole(dg, "p", true)
	role := d.crawlerRoleID()
	if dg.created != 1 || role == "" || !dg.held["p/"+role] {
		t.Fatalf("after escaping: created %d roles, holds %v, want one new role held by p", dg.created, dg.held)
	}
	d.updateCrawlerRole(dg, "p", true)
	if dg.created != 1 || !dg.held["p/"+role] {
		t.Errorf("after escaping again: created %d roles, holds %v, want the same role kept", dg.created, dg.held)
	}
	d.updateCrawlerRole(dg, "p", false)
	if dg.held["p/"+role] {
		t.Error("still holds the role after dying")
	}
}

func TestTheRoleExpiresADayAfterTheLatestEscape(t *testing.T) {
	dg := newFakeRoles()
	d := testDungeon(nil)
	clock := time.Now()
	d.now = func() time.Time { return clock }

	d.updateCrawlerRole(dg, "p", true)
	role := d.crawlerRoleID()

	clock = clock.Add(20 * time.Hour)
	d.updateCrawlerRole(dg, "p", true)
	clock = clock.Add(23 * time.Hour)
	d.expireRoles(dg)
	if !dg.held["p/"+role] {
		t.Fatal("the role expired 23 hours after a fresh escape, want it kept until 24")
	}

	clock = clock.Add(time.Hour)
	d.expireRoles(dg)
	if dg.held["p/"+role] {
		t.Error("still holds the role 24 hours after the latest escape")
	}
	if _, ok := d.state.CrawlerSince["p"]; ok {
		t.Error("the expired holder is still recorded, so they'd be checked forever")
	}
}

func TestDyingForgetsWhenTheRoleWasEarned(t *testing.T) {
	dg := newFakeRoles()
	d := testDungeon(nil)
	d.updateCrawlerRole(dg, "p", true)
	d.updateCrawlerRole(dg, "p", false)
	if _, ok := d.state.CrawlerSince["p"]; ok {
		t.Error("a holder who died is still recorded")
	}
}

func TestTheDestroyerRoleOutlastsDeath(t *testing.T) {
	dg := newFakeRoles()
	d := testDungeon(nil)

	d.grantDestroyer(dg, "p")
	destroyer := d.storedRole(destroyerSlot)
	if destroyer == "" || !dg.held["p/"+destroyer] {
		t.Fatalf("holds %v, want the destroyer role", dg.held)
	}
	if name := dg.roles[len(dg.roles)-1].Name; name != destroyerRoleName {
		t.Errorf("created role %q, want %q", name, destroyerRoleName)
	}

	d.updateCrawlerRole(dg, "p", false)
	clock := time.Now().Add(48 * time.Hour)
	d.now = func() time.Time { return clock }
	d.expireRoles(dg)
	if !dg.held["p/"+destroyer] {
		t.Error("the destroyer role was lost to a death or the 24 hour expiry, want it kept forever")
	}
}

func TestAnExistingDestroyerRoleIsReused(t *testing.T) {
	dg := newFakeRoles(&discordgo.Role{ID: "existing", Name: "Dungeon Destroyer"})
	d := testDungeon(nil)
	if got, err := d.destroyerRole(dg); err != nil || got != "existing" || dg.created != 0 {
		t.Errorf("got %q (err %v, created %d), want the existing role reused", got, err, dg.created)
	}
}

func TestAnExistingRoleIsFoundWhateverItsFont(t *testing.T) {
	for _, name := range []string{crawlerRoleName, "Dungeon Crawler", "dungeon crawler"} {
		dg := newFakeRoles(&discordgo.Role{ID: "existing", Name: name})
		d := testDungeon(nil)
		if got, err := d.crawlerRole(dg); err != nil || got != "existing" || dg.created != 0 {
			t.Errorf("role named %q: got %q (err %v, created %d), want the existing role reused", name, got, err, dg.created)
		}
	}
}

func TestARenamedRoleIsStillFoundByItsID(t *testing.T) {
	dg := newFakeRoles(&discordgo.Role{ID: "kept", Name: "something else entirely"})
	d := NewDungeon(Config{}, nil, &memStore{}, &State{CrawlerRole: "kept"})
	if got, err := d.crawlerRole(dg); err != nil || got != "kept" || dg.created != 0 {
		t.Errorf("got %q (err %v, created %d), want the stored role reused", got, err, dg.created)
	}
}

func TestPronounsComeFromMappedRoles(t *testing.T) {
	d := testDungeon(nil, 1)
	d.state.PronounRoles = map[string]string{"she": "r-she", "theyhe": "r-theyhe"}

	cases := []struct {
		roles []string
		want  string
	}{
		{nil, "they"},
		{[]string{"r-other"}, "they"},
		{[]string{"r-theyhe"}, "he"},
		{[]string{"r-she"}, "she"},
	}
	for _, c := range cases {
		if got := d.pronounsFor(c.roles).They(); got != c.want {
			t.Errorf("roles %v: they = %q, want %q", c.roles, got, c.want)
		}
	}
}

func TestPronounRolesAreMatchedWhateverTheirFont(t *testing.T) {
	d := testDungeon(nil)
	roles := []*discordgo.Role{
		{ID: "1", Name: "she/her"},
		{ID: "2", Name: "𝔱𝔥𝔢𝔶/𝔱𝔥𝔢𝔪"},
	}
	d.setPronounRoles(roles, "g", map[string]string{"she": "1", "they": "they/them"})
	if got := d.state.PronounRoles; got["she"] != "1" || got["they"] != "2" {
		t.Errorf("pronoun roles = %v, want she on 1 and they on 2", got)
	}
}

func TestUnknownPronounRolesChangeNothing(t *testing.T) {
	d := testDungeon(nil)
	d.setPronounRoles([]*discordgo.Role{{ID: "1", Name: "she/her"}}, "g", map[string]string{"she": "1", "he": "nobody"})
	if len(d.state.PronounRoles) != 0 {
		t.Errorf("pronoun roles = %v, want nothing saved while one is unknown", d.state.PronounRoles)
	}
}

func TestTheGoldAmountGoesInFrontOfTheWordGold(t *testing.T) {
	cases := map[string]string{
		"You find a coin purse with gold under the table.": "You find a coin purse with " + fancy("12 gold") + " under the table.",
		"You find gold beneath one of the plates":          "You find " + fancy("12 gold") + " beneath one of the plates",
		"You find a ruby under the mattress.":              "You find a ruby under the mattress.",
	}
	for text, want := range cases {
		if got := withGold(text, 12); got != want {
			t.Errorf("withGold(%q) = %q, want %q", text, got, want)
		}
	}
	if got := withGold("You find gold.", 0); got != "You find gold." {
		t.Errorf("with no gold rolled: %q, want the text untouched", got)
	}
}

func TestTheReportOnlyListsSpoilsWhenThereAreAny(t *testing.T) {
	if empty := render(DroppedMsg, MsgData{Player: "@p"}); empty != "" {
		t.Errorf("empty-handed death = %q, want no dropped list", empty)
	}
	full := render(DroppedMsg, MsgData{Player: "@p", Gold: 2, Items: "Ruby"})
	if !strings.HasPrefix(full, "Dropped:") || !strings.Contains(full, fancy(2)) || !strings.Contains(full, fancy("Ruby")) {
		t.Errorf("death with spoils = %q, want the dropped gold and items listed", full)
	}
	if escaped := render(EscapeSpoilsMsg, MsgData{Player: "@p"}); escaped != "@p escaped the dungeon." {
		t.Errorf("empty-handed escape = %q, want %q", escaped, "@p escaped the dungeon.")
	}
}

func TestCollectLineOnlyAppearsWithLoot(t *testing.T) {
	next := testRoom("b", false, Outcome{Text: "pass"})
	steps := flipSteps(turn{passed: true, text: "You find gold.", gold: 12, next: next}, "@p")
	if want := render(CollectMsg, MsgData{Loot: "12 gold"}); !strings.Contains(steps[0], want) {
		t.Errorf("reveal = %q, want it to include %q", steps[0], want)
	}
	steps = flipSteps(turn{passed: true, text: "Nothing here.", next: next}, "@p")
	if strings.Contains(steps[0], render(CollectMsg, MsgData{})) {
		t.Errorf("reveal = %q, want no collect line without loot", steps[0])
	}
}

func TestItemsAreStyledWhereTheTextNamesThem(t *testing.T) {
	got := withItem("You find a small silver dagger among the roots.", "Silver Dagger")
	if want := "You find a small " + fancy("silver dagger") + " among the roots."; got != want {
		t.Errorf("withItem = %q, want %q", got, want)
	}
}

func TestFancyIsDoubleStruckAndFoldsBack(t *testing.T) {
	plain := "Ruby Crystal Ball HPQZ 0123456789"
	styled := fancy(plain)
	if styled == plain || norm.NFKC.String(styled) != plain {
		t.Errorf("fancy(%q) = %q, want double-struck letters that fold back to the original", plain, styled)
	}
}

func TestBrokenRoomsAreRejected(t *testing.T) {
	cases := map[string]string{
		"no fail text":   `[{"id": "a", "name": "A", "text": "t", "pass": {"text": "p"}, "fail": {}}]`,
		"misspelt field": `[{"id": "a", "name": "A", "text": "t", "pass": {"text": "p", "itme": "x"}, "fail": {"text": "f"}}]`,
		"repeated id":    `[{"id": "a", "name": "A", "text": "t", "pass": {"text": "p"}, "fail": {"text": "f"}}, {"id": "a", "name": "B", "text": "t", "pass": {"text": "p"}, "fail": {"text": "f"}}]`,
		"gold unsaid":    `[{"id": "a", "name": "A", "text": "t", "pass": {"text": "You find a purse.", "gold": true}, "fail": {"text": "f"}}]`,
		"item unnamed":   `[{"id": "a", "name": "A", "text": "t", "pass": {"text": "You find a gem.", "item": "Ruby"}, "fail": {"text": "f"}}]`,
	}
	for name, lethal := range cases {
		if _, err := LoadRooms([]byte(lethal), []byte(`[]`)); err == nil {
			t.Errorf("%s: loaded without complaint", name)
		}
	}
}
func TestRunsAddToTheLedgerAndEscapesToTheStockpile(t *testing.T) {
	rooms := []Room{
		testRoom("a", false, Outcome{Text: "gold", Gold: true}),
		testRoom("b", true, Outcome{Text: "Rope", Item: "Rope"}),
	}
	d := testDungeon(rooms)

	for range 2 {
		begin(t, d)
		callCoin(t, d, "heads")
		if got := callCoin(t, d, "heads"); !got.escaped {
			t.Fatalf("run = %+v, want an escape", got)
		}
	}
	begin(t, d)
	callCoin(t, d, "heads")
	if got := callCoin(t, d, "tails"); !got.died {
		t.Fatalf("run = %+v, want a death", got)
	}
	begin(t, d)
	if _, ok := d.expire("p", testThread); !ok {
		t.Fatal("run did not time out")
	}

	escapes, deaths, pile := d.stockpile()
	if want := []string{"2 gold", "2 Rope"}; escapes != 2 || deaths != 2 || !slices.Equal(pile, want) {
		t.Errorf("stockpile = %d escapes, %d deaths, %q, want 2, 2 and %q without the dead run's loot", escapes, deaths, pile, want)
	}
	if saved := d.store.(*memStore).saved; saved == nil || saved.Escapes != 2 || saved.Deaths != 2 || saved.Gold != 2 || saved.Items["Rope"] != 2 {
		t.Errorf("saved state = %+v, want the totals saved", saved)
	}
}

func TestTheStockpileListsTheMostCommonItemsFirst(t *testing.T) {
	d := NewDungeon(Config{}, nil, &memStore{}, &State{Escapes: 3, Items: map[string]int{"Shovel": 1, "Rusty Knife": 2, "Ancient Tome": 1, "Gone": 0}})
	_, _, pile := d.stockpile()
	if want := []string{"2 Rusty Knife", "1 Ancient Tome", "1 Shovel"}; !slices.Equal(pile, want) {
		t.Errorf("stockpile = %q, want %q with no gold or zero counts", pile, want)
	}
}

func TestTheHistoryPost(t *testing.T) {
	got := render(HistoryMsg, MsgData{Escapes: 2, Fallen: 17, Pockets: []string{"14 gold", "1 Ancient Tome"}})
	want := "Beyond the dungeon gates, the |x|umble |x|orde grows stronger.\n\n" +
		"2 rebels have successfully escaped their four walls, and together assembled:\n- " + fancy("14 gold") + "\n- " + fancy("1 Ancient Tome") + "\n\n" +
		"17 rebels have crawled into darkness.\nTheir loot was lost, but their courage inspires the horde."
	if !strings.HasSuffix(got, want) || !strings.Contains(got, fancy("humble stockpile")) {
		t.Errorf("history post = %q, want it to end %q", got, want)
	}

	one := render(HistoryMsg, MsgData{Escapes: 1, Fallen: 1})
	if !strings.Contains(one, "1 rebel has successfully escaped their four walls, and together assembled nothing.") || !strings.Contains(one, "\n1 rebel has crawled into darkness.") {
		t.Errorf("history post = %q, want singulars and an empty stockpile", one)
	}
}
