package videoserver

import (
	"strings"
)

type VerboseLevel uint16

const (
	VERBOSE_NONE = VerboseLevel(iota)
	VERBOSE_SIMPLE
	VERBOSE_ADD
	VERBOSE_ALL
)

var verboseMap = map[string]VerboseLevel{
	"v":   VERBOSE_SIMPLE,
	"vv":  VERBOSE_ADD,
	"vvv": VERBOSE_ALL,
}

func NewVerboseLevelFrom(str string) VerboseLevel {
	if verboseLevel, ok := verboseMap[strings.ToLower(str)]; ok {
		return verboseLevel
	}
	return VERBOSE_NONE
}
