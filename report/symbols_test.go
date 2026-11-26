package report

import (
	"testing"

	"github.com/marcuoli/go-document-pdf/internal/reportfonts"
)

func TestStructuredReportSymbolFonts(t *testing.T) {
	input := textHTML("a & <b> ⇒ → ✔ ✘ ⚠️")
	if got := markStyles(reportfonts.Faces{}).Replace(input); got != "a &amp; &lt;b&gt; ⇒ → ✔ ✘ ⚠️" {
		t.Fatalf("default markup changed: %q", got)
	}
	if got := markStyles(reportfonts.Faces{Symbol: true}).Replace(input); got !=
		"a &amp; &lt;b&gt; "+symbolFont+"⇒</span> "+symbolFont+"→</span> "+symbolFont+"✔</span> "+symbolFont+"✘</span> ⚠️" {
		t.Fatalf("unexpected arrow, check or cross markup: %q", got)
	}
	if got := markStyles(reportfonts.Faces{Emoji: true}).Replace(input); got !=
		"a &amp; &lt;b&gt; ⇒ → ✔ ✘ "+emojiFont+"⚠</span>" {
		t.Fatalf("unexpected warning markup: %q", got)
	}
}
