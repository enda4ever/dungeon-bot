package events

import _ "embed"

//go:embed lethal.json
var Lethal []byte

//go:embed non-lethal.json
var NonLethal []byte
