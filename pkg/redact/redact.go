package redact

import "strings"

// RedactEmail returns a redacted version of an email address,
// preserving only the first character of the local part and the full domain.
func RedactEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return "***@"
	}
	if len(parts[0]) == 0 {
		return "***@" + parts[1]
	}
	first := string(parts[0][0])
	return first + "***@" + parts[1]
}

// RedactPhone returns a redacted version of a phone number,
// preserving the first 2 and last 4 digits.
func RedactPhone(phone string) string {
	if len(phone) == 0 {
		return "***"
	}
	if len(phone) <= 8 {
		return "***"
	}
	return phone[:2] + "******" + phone[len(phone)-4:]
}

// RedactName returns a redacted version of a name,
// preserving only the first letter of each word.
func RedactName(name string) string {
	if name == "" {
		return ""
	}
	words := strings.Fields(name)
	result := make([]string, len(words))
	for i, word := range words {
		result[i] = string(word[0]) + "***"
	}
	return strings.Join(result, " ")
}
