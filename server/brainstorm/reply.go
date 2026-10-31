package brainstorm

import (
	"regexp"
	"strings"
)

const (
	replyOpen  = "<reply>"
	replyClose = "</reply>"
)

// ExtractReply returns the text inside the last <reply>...</reply> block of a
// completion. Some agent profiles (CLI-backed ones in particular) print a
// startup banner — loaded skills, extensions, file paths — before the model's
// answer; the tags let us drop that noise instead of showing it in the chat.
// Without tags the whole trimmed completion is returned.
func ExtractReply(text string) string {
	text = stripBanner(text)
	start := strings.LastIndex(text, replyOpen)
	if start < 0 {
		return strings.TrimSpace(text)
	}
	body := text[start+len(replyOpen):]
	if end := strings.Index(body, replyClose); end >= 0 {
		body = body[:end]
	}
	if reply := strings.TrimSpace(body); reply != "" {
		return reply
	}
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, replyOpen, ""), replyClose, ""))
}

var (
	bannerHeading = regexp.MustCompile(`^#{1,3}\s+(Skills|Prompts|Extensions|Themes|Tools|Context|Context files|Models?|MCP servers?)\s*$`)
	bannerItem    = regexp.MustCompile(`^[-*]\s+(/|~/|[A-Za-z]:\\|https?://|\[object Object\])`)
)

// stripBanner drops a CLI agent's startup banner from the start of a
// completion: horizontal rules, known section headings, and list items that
// are paths, URLs or slash commands. It only fires when the text opens with
// such a banner, and stops at the first line that is anything else, so a
// reply that legitimately starts with bullets is left alone.
func stripBanner(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	i := 0
	sawHeading := false
	for ; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		switch {
		case line == "" || line == "---":
		case bannerHeading.MatchString(line):
			sawHeading = true
		case sawHeading && bannerItem.MatchString(line):
		default:
			if !sawHeading {
				return text
			}
			return strings.Join(lines[i:], "\n")
		}
	}
	if !sawHeading {
		return text
	}
	return ""
}
