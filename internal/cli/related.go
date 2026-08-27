package cli

import (
	"sort"
	"strings"

	"github.com/dmarcocci/atlas/internal/ledger"
)

// relatedCardsFor is a small, deliberate duplication of contextc's
// unexported relatedCards (S5.5): a card is related to workitem w if its
// id appears in w's body or evidence, or if its own evidence shares a
// path with w's evidence. Needed for the `context <id> --json` rendering,
// which contextc does not itself expose a JSON shape for; kept local
// rather than exporting new surface from the F3 package.
func relatedCardsFor(w ledger.Workitem, cards []ledger.Card) []ledger.Card {
	taskPaths := make(map[string]struct{}, len(w.Evidence))
	for _, e := range w.Evidence {
		taskPaths[stripEvidenceLines(e)] = struct{}{}
	}

	var out []ledger.Card
	for _, c := range cards {
		if strings.Contains(w.Body, c.ID) {
			out = append(out, c)
			continue
		}
		mentioned := false
		for _, e := range w.Evidence {
			if e == c.ID {
				mentioned = true
				break
			}
		}
		if mentioned {
			out = append(out, c)
			continue
		}
		shared := false
		for _, ce := range c.Evidence {
			if _, ok := taskPaths[stripEvidenceLines(ce)]; ok {
				shared = true
				break
			}
		}
		if shared {
			out = append(out, c)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func stripEvidenceLines(evidence string) string {
	if i := strings.LastIndex(evidence, ":"); i >= 0 {
		return evidence[:i]
	}
	return evidence
}
