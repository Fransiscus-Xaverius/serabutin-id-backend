package redact

import "strings"

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

func RedactPhone(phone string) string {
	if len(phone) == 0 {
		return "***"
	}
	if len(phone) <= 8 {
		return "***"
	}
	return phone[:2] + "******" + phone[len(phone)-4:]
}

func RedactName(name string) string {
	if name == "" {
		return ""
	}
	words := strings.Fields(name)
	result := make([]string, len(words))
	for i, word := range words {
		if len(word) == 0 {
			result[i] = ""
			continue
		}
		result[i] = string(word[0]) + "***"
	}
	return strings.Join(result, " ")
}
