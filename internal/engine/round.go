package engine

import (
	"fmt"

	"github.com/dstathis/openswiss/internal/export"
	"github.com/dstathis/openswiss/internal/models"
	st "github.com/dstathis/swisstools"
)

// LiveRound identifies the round currently being played. Rounds have no table
// of their own -- they live inside the opaque engine_state blob -- so this is
// derived from the engine on every request rather than stored.
type LiveRound struct {
	Number  int // 1-based, matching matches.round
	Playoff bool
	Label   string
}

// CurrentLiveRound reports the round being played right now, or ok=false when
// none is. The false case is load-bearing in three places: the detail page
// emits no timer markup, the JSON endpoint reports state "none", and starting a
// timer is refused rather than guessing at a round number.
//
// Shared by the web handlers and the REST API so the two can never disagree
// about which round a timer belongs to.
func CurrentLiveRound(t *models.Tournament, eng *st.Tournament) (LiveRound, bool) {
	switch t.Status {
	case models.TournamentStatusInProgress:
		n := eng.GetCurrentRound()
		if n < 1 {
			return LiveRound{}, false
		}
		return LiveRound{Number: n, Label: fmt.Sprintf("Round %d", n)}, true

	case models.TournamentStatusPlayoff:
		if eng.GetPlayoffStatus() != "in_progress" {
			return LiveRound{}, false
		}
		pl := eng.GetPlayoff()
		if pl == nil {
			return LiveRound{}, false
		}
		// StartPlayoff pre-allocates every bracket round, so len(Rounds) is the
		// true bracket depth from the first playoff round onward. Reusing
		// PlayoffRoundName keeps a match named identically here, in the stats
		// projection and in the OTR export.
		return LiveRound{
			Number:  pl.CurrentRound + 1,
			Playoff: true,
			Label:   export.PlayoffRoundName(len(pl.Rounds), pl.CurrentRound),
		}, true
	}
	return LiveRound{}, false
}
