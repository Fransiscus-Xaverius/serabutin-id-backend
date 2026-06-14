package whatsapp

import (
	"regexp"
	"strings"
)

var validRegex = regexp.MustCompile(`^(?:\+62|62|0)8[1-9]\d{7,11}$`)

func Normalize(input string) string {
	input = strings.TrimSpace(input)
	input = strings.ReplaceAll(input, "-", "")
	input = strings.ReplaceAll(input, " ", "")

	switch {
	case strings.HasPrefix(input, "+62"):
		return input
	case strings.HasPrefix(input, "62"):
		return "+" + input
	case strings.HasPrefix(input, "0"):
		return "+62" + input[1:]
	default:
		return input
	}
}

func Validate(input string) bool {
	input = strings.TrimSpace(input)
	input = strings.ReplaceAll(input, "-", "")
	input = strings.ReplaceAll(input, " ", "")
	return validRegex.MatchString(input)
}
