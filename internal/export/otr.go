package export

import (
	"encoding/json"
	"fmt"

	"github.com/dstathis/openswiss/internal/models"
	"github.com/dstathis/swisstools"
)

type OTR struct {
	OTRVersion int           `json:"otr_version"`
	Tournament OTRTournament `json:"tournament"`
	Players    []OTRPlayer   `json:"players"`
	Rounds     []OTRRound    `json:"rounds"`
	Playoff    *OTRPlayoff   `json:"playoff,omitempty"`
}

type OTRTournament struct {
	Name        string `json:"name"`
	Date        string `json:"date,omitempty"`
	Location    string `json:"location,omitempty"`
	Format      string `json:"format"`
	SwissRounds int    `json:"swiss_rounds"`
	PlayerCount int    `json:"player_count"`
	PointsWin   int    `json:"points_win"`
	PointsDraw  int    `json:"points_draw"`
	PointsLoss  int    `json:"points_loss"`
	TopCut      int    `json:"top_cut,omitempty"`
}

type OTRPlayer struct {
	ID          int            `json:"id"`
	Name        string         `json:"name"`
	ExternalID  *int           `json:"external_id,omitempty"`
	FinalRank   int            `json:"final_rank"`
	Points      int            `json:"points"`
	Record      OTRRecord      `json:"record"`
	Tiebreakers OTRTiebreakers `json:"tiebreakers"`
	Decklist    *OTRDecklist   `json:"decklist,omitempty"`
}

type OTRRecord struct {
	Wins   int `json:"wins"`
	Losses int `json:"losses"`
	Draws  int `json:"draws"`
}

type OTRTiebreakers struct {
	OpponentMatchWinPct float64 `json:"opponent_match_win_pct"`
	GameWinPct          float64 `json:"game_win_pct"`
	OpponentGameWinPct  float64 `json:"opponent_game_win_pct"`
}

type OTRDecklist struct {
	Main      map[string]int `json:"main"`
	Sideboard map[string]int `json:"sideboard"`
}

type OTRRound struct {
	RoundNumber int          `json:"round_number"`
	Pairings    []OTRPairing `json:"pairings"`
}

type OTRPairing struct {
	PlayerA *OTRPairingPlayer `json:"player_a"`
	PlayerB *OTRPairingPlayer `json:"player_b"`
	Draws   int               `json:"draws"`
}

type OTRPairingPlayer struct {
	ID   int `json:"id"`
	Wins int `json:"wins"`
}

type OTRPlayoff struct {
	Seeds  []int             `json:"seeds"`
	Rounds []OTRPlayoffRound `json:"rounds"`
	Winner *OTRPairingPlayer `json:"winner,omitempty"`
}

type OTRPlayoffRound struct {
	RoundName string       `json:"round_name"`
	Pairings  []OTRPairing `json:"pairings"`
}

func GenerateOTR(t *models.Tournament, eng *swisstools.Tournament) ([]byte, error) {
	standings := eng.GetStandings()
	players := eng.GetPlayers()

	otr := OTR{
		OTRVersion: 1,
		Tournament: OTRTournament{
			Name:        t.Name,
			Format:      "Swiss",
			SwissRounds: eng.GetCurrentRound(),
			PlayerCount: eng.GetPlayerCount(),
			PointsWin:   t.PointsWin,
			PointsDraw:  t.PointsDraw,
			PointsLoss:  t.PointsLoss,
		},
	}

	if t.ScheduledAt != nil {
		otr.Tournament.Date = t.ScheduledAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if t.Location != nil {
		otr.Tournament.Location = *t.Location
	}
	if t.TopCut > 0 {
		otr.Tournament.TopCut = t.TopCut
	}

	// Players
	for _, s := range standings {
		p := OTRPlayer{
			ID:        s.PlayerID,
			Name:      s.Name,
			FinalRank: s.Rank,
			Points:    s.Points,
			Record: OTRRecord{
				Wins:   s.Wins,
				Losses: s.Losses,
				Draws:  s.Draws,
			},
			Tiebreakers: OTRTiebreakers{
				OpponentMatchWinPct: s.Tiebreakers.OpponentMatchWinPct,
				GameWinPct:          s.Tiebreakers.GameWinPercentage,
				OpponentGameWinPct:  s.Tiebreakers.OpponentGameWinPct,
			},
		}

		player, exists := players[s.PlayerID]
		if exists {
			p.ExternalID = player.ExternalID
			if t.DecklistPublic && player.Decklist != nil {
				p.Decklist = &OTRDecklist{
					Main:      player.Decklist.Main,
					Sideboard: player.Decklist.Sideboard,
				}
			}
		}

		otr.Players = append(otr.Players, p)
	}

	// Rounds
	for i := 1; i <= eng.GetCurrentRound(); i++ {
		pairings, err := eng.GetRoundByNumber(i)
		if err != nil {
			continue
		}
		round := OTRRound{RoundNumber: i}
		for _, p := range pairings {
			pairing := OTRPairing{
				PlayerA: &OTRPairingPlayer{ID: p.PlayerA(), Wins: p.PlayerAWins()},
				Draws:   p.Draws(),
			}
			if p.PlayerB() == swisstools.BYE_OPPONENT_ID {
				pairing.PlayerB = nil
			} else {
				pairing.PlayerB = &OTRPairingPlayer{ID: p.PlayerB(), Wins: p.PlayerBWins()}
			}
			round.Pairings = append(round.Pairings, pairing)
		}
		otr.Rounds = append(otr.Rounds, round)
	}

	// Playoff
	playoff := eng.GetPlayoff()
	if playoff != nil {
		otrPlayoff := &OTRPlayoff{
			Seeds: playoff.Seeds,
		}
		for i, round := range playoff.Rounds {
			pr := OTRPlayoffRound{
				RoundName: playoffRoundName(len(playoff.Rounds), i),
			}
			for _, p := range round {
				pairing := OTRPairing{
					PlayerA: &OTRPairingPlayer{ID: p.PlayerA(), Wins: p.PlayerAWins()},
					Draws:   p.Draws(),
				}
				if p.PlayerB() == swisstools.BYE_OPPONENT_ID {
					pairing.PlayerB = nil
				} else {
					pairing.PlayerB = &OTRPairingPlayer{ID: p.PlayerB(), Wins: p.PlayerBWins()}
				}
				pr.Pairings = append(pr.Pairings, pairing)
			}
			otrPlayoff.Rounds = append(otrPlayoff.Rounds, pr)
		}

		// Find winner if playoff is finished
		if playoff.Finished && len(playoff.Rounds) > 0 {
			lastRound := playoff.Rounds[len(playoff.Rounds)-1]
			if len(lastRound) == 1 {
				p := lastRound[0]
				var winnerID int
				if p.PlayerAWins() > p.PlayerBWins() {
					winnerID = p.PlayerA()
				} else {
					winnerID = p.PlayerB()
				}
				otrPlayoff.Winner = &OTRPairingPlayer{ID: winnerID}
			}
		}

		otr.Playoff = otrPlayoff
	}

	return json.MarshalIndent(otr, "", "  ")
}

func playoffRoundName(totalRounds, roundIndex int) string {
	remaining := totalRounds - roundIndex
	switch remaining {
	case 1:
		return "Finals"
	case 2:
		return "Semifinals"
	case 3:
		return "Quarterfinals"
	default:
		return fmt.Sprintf("Round %d", roundIndex+1)
	}
}
