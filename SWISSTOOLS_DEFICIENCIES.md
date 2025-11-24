# swisstools Library Deficiencies and Workarounds

This document details the deficiencies in the `swisstools` library that this project works around, along with the specific implementation details.

## 1. Player Name Lookup from Pairings (Unexported Fields)

**Deficiency**: The `Player` struct in swisstools has unexported fields, making it impossible to directly access player names from `Pairing` objects returned by `GetRound()`.

**Workaround**: The application builds a player ID-to-name mapping by:
1. Getting all standings via `GetStandings()` (which includes player names)
2. Looking up player IDs using `GetPlayerID(name)` for each standing
3. Also checking pending players that were accepted (in case they're in the tournament but don't have standings yet)
4. Using this map to resolve player IDs from pairings to names

**Location**: 
- `internal/handlers/admin.go:54-73` (Dashboard)
- `internal/handlers/player.go:155-175` (Pairings)

**Code Example**:
```go
// Build player map from standings (which have names) and pending accepted players
standingsForLookup := tournament.GetStandings()
playerMap := make(map[int]string)
for _, s := range standingsForLookup {
    id, ok := tournament.GetPlayerID(s.Name)
    if ok {
        playerMap[id] = s.Name
    }
}

// Also include pending players that were accepted
for _, pp := range pending {
    if pp.Status == "accepted" {
        if id, ok := tournament.GetPlayerID(pp.Name); ok {
            if _, exists := playerMap[id]; !exists {
                playerMap[id] = pp.Name
            }
        }
    }
}
```

**Impact**: This workaround is necessary every time pairings are displayed. It requires multiple API calls and could be inefficient for large tournaments.

## 2. No Player Registration Queue

**Deficiency**: The library has no concept of "pending" players waiting for admin approval. Players must be added directly to the tournament via `AddPlayer()`.

**Workaround**: The application implements a separate registration queue system:
- Custom `PendingPlayer` struct with `Name` and `Status` fields
- Separate JSON file (`data/pending_players.json`) for persistence
- `AddPendingPlayer()`, `AcceptPlayer()`, and `RejectPlayer()` methods
- Players are only added to the tournament when explicitly accepted by an admin

**Location**: `internal/storage/tournament.go:38-262`

**Impact**: This adds significant complexity and requires maintaining separate state that must be kept in sync with the tournament.

## 3. No Result Validation

**Deficiency**: The library doesn't validate that results match between players. For example, if Player A wins 2-1, the library doesn't verify that Player B's result is 1-2. Results are recorded from a single player's perspective via `AddResult(id, wins, losses, draws)`.

**Workaround**: The application relies entirely on admin accuracy when recording results. There's no automatic validation that:
- If Player A in a pairing wins 2-1, Player B loses 1-2
- Results are recorded for both players in a match
- Results are consistent (e.g., wins + losses + draws = total games played)

**Location**: `internal/handlers/admin.go:249-285` (AddResult handler)

**Impact**: This is a significant limitation that could lead to data integrity issues. The admin must manually ensure all results are recorded correctly and consistently.

## 4. No Authentication/Authorization

**Deficiency**: The library has no built-in authentication or authorization system.

**Workaround**: The application implements:
- Session-based authentication (`internal/auth/auth.go`)
- Role-based access control (admin vs player)
- Password-protected admin access
- Middleware for route protection (`RequireAdmin`, `OptionalAuth`)

**Location**: `internal/auth/` package

**Impact**: This is expected for a backend library, but adds significant implementation overhead.

## 5. No Persistence Layer

**Deficiency**: The library provides `DumpTournament()` and `LoadTournament()` for serialization, but no built-in persistence mechanism.

**Workaround**: The application implements:
- File-based persistence using JSON files
- Automatic saving after each tournament modification
- Thread-safe storage with mutex locks
- Separate storage for pending players

**Location**: `internal/storage/tournament.go`

**Impact**: The application must handle all persistence concerns, including error handling, file locking, and data consistency.

## 6. Standings Must Be Manually Updated

**Deficiency**: After recording results, standings are not automatically recalculated. The admin must explicitly call `UpdatePlayerStandings()`.

**Workaround**: The application provides a separate "Update Standings" action in the admin dashboard that calls `UpdatePlayerStandings()`. This is a required step after each round.

**Location**: `internal/handlers/admin.go:287-300` (UpdateStandings handler)

**Impact**: This is easy to forget and could lead to displaying stale standings. The application could benefit from automatically updating standings after all results are recorded.

## 7. No Direct Access to Player Information from Pairings

**Deficiency**: `Pairing` objects only provide player IDs via `PlayerA()` and `PlayerB()`. To get player names, you must:
1. Get the player via `GetPlayerById()` (returns unexported `Player` struct)
2. Or build a mapping from standings as described in deficiency #1

**Workaround**: Same as deficiency #1 - building a player ID-to-name map from standings.

**Impact**: Redundant lookups and potential for inconsistency if standings haven't been updated.

## 8. Tournament State Management (Application Limitation, Not Library)

**Clarification**: The swisstools library itself allows creating multiple `Tournament` objects - there's no library-level restriction. However, this application is architected to manage only a single tournament instance.

**Application Limitation**: The application's design choices that limit it to one tournament:
- `TournamentStorage` stores a single `tournament` field (`st.Tournament`)
- Single persistence file (`data/tournament.json`)
- No tournament selection/management UI
- All handlers operate on the single stored tournament instance

**To Support Multiple Tournaments**: Would require architectural changes:
- Change `TournamentStorage` to store a map of tournaments (e.g., `map[string]st.Tournament`)
- Add tournament ID/name to routes (e.g., `/tournament/{id}/dashboard`)
- Add tournament selection UI
- Per-tournament pending player queues
- Per-tournament persistence files

**Location**: `internal/storage/tournament.go:32-35` (single `tournament` field)

**Impact**: This is an application design choice, not a library deficiency. The library itself doesn't prevent multiple tournaments.

## 9. No Built-in Web Interface

**Deficiency**: The library is backend-only with no web interface.

**Workaround**: The application provides a complete web interface with:
- HTML templates for all views
- HTTP handlers for all operations
- REST-like API endpoints

**Location**: `internal/handlers/` and `templates/`

**Impact**: This is expected for a backend library, but represents significant implementation work.

## Summary

The most significant deficiencies that require active workarounds are:

1. **Unexported Player fields** - Requires building player name maps from standings
2. **No registration queue** - Requires separate pending player system
3. **No result validation** - Relies on admin accuracy
4. **Manual standings updates** - Easy to forget, could show stale data
5. **No persistence** - Application must handle all file I/O and state management

**Note**: Item #8 (Tournament State Management) is not actually a library deficiency - the library allows multiple `Tournament` objects. The single-tournament limitation is an application design choice.

These workarounds add complexity and potential points of failure to the application. Some could be addressed by library improvements (e.g., exported Player fields, automatic standings updates, result validation), while others are architectural decisions (e.g., authentication, web interface, single tournament instance) that are appropriately handled at the application level.

