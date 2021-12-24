package logo

import (
	_ "embed"
)

//go:embed emblem.txt
var emblem string

func Emblem() string {
	return emblem
}

//go:embed wordmark.txt
var wordmark string

func Wordmark() string {
	return wordmark
}
