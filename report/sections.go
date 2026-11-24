package report

import (
	"html"
	"net/url"
	"strings"
)

// Section adds an optional heading and ordered paragraphs to a report.
// A paragraph is a sequence of text runs, including any spaces between runs.
type Section struct {
	Heading    string
	Paragraphs [][]TextRun
}

// TextRun supplies content and an optional external HTTP, HTTPS or mail link.
// Newlines render as line breaks. Unusable links are rendered as plain text.
type TextRun struct {
	Text string
	URL  string
}

func sectionHTML(sections []Section) string {
	var out strings.Builder
	for _, section := range sections {
		if section.Heading != "" {
			out.WriteString("<h2>" + textHTML(section.Heading) + "</h2>")
		}
		for _, paragraph := range section.Paragraphs {
			if len(paragraph) == 0 {
				continue
			}
			out.WriteString("<p>")
			for _, run := range paragraph {
				literal := textHTML(run.Text)
				if allowsExternalLink(run.URL) {
					literal = `<a href="` + html.EscapeString(run.URL) + `">` + literal + "</a>"
				}
				out.WriteString(literal)
			}
			out.WriteString("</p>")
		}
	}
	return out.String()
}

func allowsExternalLink(address string) bool {
	if address == "" {
		return false
	}
	parsed, err := url.Parse(address)
	if err != nil {
		return false
	}
	switch parsed.Scheme {
	case "http", "https":
		return parsed.Hostname() != "" && parsed.User == nil
	case "mailto":
		local, domain, found := strings.Cut(parsed.Opaque, "@")
		return found && local != "" && domain != ""
	default:
		return false
	}
}
