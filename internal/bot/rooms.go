package bot

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/template"
)

var goldWord = regexp.MustCompile(`(?i)\bgold\b`)

type Outcome struct {
	Text     string `json:"text"`
	Gold     bool   `json:"gold"`
	Item     string `json:"item"`
	Obituary string `json:"obituary"`
	Epilogue string `json:"epilogue"`
	Where    string `json:"where"`
}

type Room struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Text   string  `json:"text"`
	Pass   Outcome `json:"pass"`
	Fail   Outcome `json:"fail"`
	Lethal bool    `json:"-"`

	obituary    *template.Template
	epilogue    *template.Template
	where       *template.Template
	escapeWhere *template.Template
}

func (r Room) title() string {
	name := strings.ToLower(r.Name)
	if strings.HasPrefix(name, "the ") {
		return name
	}
	return "the " + name
}

func LoadRooms(lethal, nonLethal []byte) ([]Room, error) {
	deadly, err := parseRooms("lethal.json", lethal, true)
	if err != nil {
		return nil, err
	}
	safe, err := parseRooms("non-lethal.json", nonLethal, false)
	if err != nil {
		return nil, err
	}
	rooms := append(deadly, safe...)
	if len(rooms) == 0 {
		return nil, errors.New("there are no rooms")
	}

	seen := map[string]bool{}
	for n, r := range rooms {
		switch {
		case r.ID == "":
			return nil, fmt.Errorf("room %q has no id", r.Name)
		case seen[r.ID]:
			return nil, fmt.Errorf("room id %q is used twice", r.ID)
		case r.Name == "" || r.Text == "" || r.Pass.Text == "" || r.Fail.Text == "":
			return nil, fmt.Errorf("room %q needs a name, text, pass text and fail text", r.ID)
		case r.Pass.Gold && !goldWord.MatchString(r.Pass.Text), r.Fail.Gold && !goldWord.MatchString(r.Fail.Text):
			return nil, fmt.Errorf("room %q awards gold but its text never says gold, so there is nowhere to put the amount", r.ID)
		case !names(r.Pass.Text, r.Pass.Item), !names(r.Fail.Text, r.Fail.Item):
			return nil, fmt.Errorf("room %q awards an item its text never names", r.ID)
		case r.Lethal && (r.Fail.Obituary == "" || r.Fail.Where == ""):
			return nil, fmt.Errorf("room %q is lethal but is missing its obituary or where for the public death post", r.ID)
		case r.Lethal && r.Pass.Where == "":
			return nil, fmt.Errorf("room %q is lethal but its pass has no where for the public escape post", r.ID)
		case r.Lethal && r.Pass.Epilogue == "":
			return nil, fmt.Errorf("room %q is lethal but has no epilogue for the public escape post", r.ID)
		case r.Pass.Epilogue != "" && (r.Pass.Gold && !goldWord.MatchString(r.Pass.Epilogue) || !names(r.Pass.Epilogue, r.Pass.Item)):
			return nil, fmt.Errorf("room %q epilogue must mention its gold or item, like its pass text does", r.ID)
		}
		seen[r.ID] = true
		if r.Fail.Obituary != "" {
			obit, err := template.New(r.ID).Funcs(templateFuncs).Parse(r.Fail.Obituary)
			if err != nil {
				return nil, fmt.Errorf("room %q obituary: %w", r.ID, err)
			}
			rooms[n].obituary = obit
		}
		if r.Fail.Where != "" {
			where, err := template.New(r.ID).Funcs(templateFuncs).Parse(r.Fail.Where)
			if err != nil {
				return nil, fmt.Errorf("room %q where: %w", r.ID, err)
			}
			rooms[n].where = where
		}
		if r.Pass.Where != "" {
			where, err := template.New(r.ID).Funcs(templateFuncs).Parse(r.Pass.Where)
			if err != nil {
				return nil, fmt.Errorf("room %q pass where: %w", r.ID, err)
			}
			rooms[n].escapeWhere = where
		}
		if r.Pass.Epilogue != "" {
			epilogue, err := template.New(r.ID).Funcs(templateFuncs).Parse(r.Pass.Epilogue)
			if err != nil {
				return nil, fmt.Errorf("room %q epilogue: %w", r.ID, err)
			}
			rooms[n].epilogue = epilogue
		}
	}
	return rooms, nil
}

func withGold(text string, gold int) string {
	loc := goldWord.FindStringIndex(text)
	if gold <= 0 || loc == nil {
		return text
	}
	return text[:loc[0]] + fancy(strconv.Itoa(gold)+" "+text[loc[0]:loc[1]]) + text[loc[1]:]
}

func withItem(text, item string) string {
	at := strings.Index(strings.ToLower(text), strings.ToLower(item))
	if item == "" || at < 0 {
		return text
	}
	return text[:at] + fancy(text[at:at+len(item)]) + text[at+len(item):]
}

func names(text, item string) bool {
	return item == "" || strings.Contains(strings.ToLower(text), strings.ToLower(item))
}

func parseRooms(file string, data []byte, lethal bool) ([]Room, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var rooms []Room
	if err := dec.Decode(&rooms); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	for n := range rooms {
		rooms[n].Lethal = lethal
	}
	return rooms, nil
}
