# swisstools Library Deficiencies and Workarounds

This document details the deficiencies in the `swisstools` library that this project works around, along with the specific implementation details.

## 1. Player Name Lookup from Pairings ✅ FIXED

**Previous Deficiency**: The `Player` struct in swisstools had unexported fields, making it impossible to directly access player names from `Pairing` objects returned by `GetRound()`.

**Status**: ✅ **FIXED** - The `Player.Name` field is now exported in the swisstools library.

**Current Implementation**: The application now directly accesses player names using `GetPlayerById()` and the exported `Name` field:

```go
// Get player names directly from Player objects (Name is now exported)
playerA, _ := tournament.GetPlayerById(playerAID)
playerB, _ := tournament.GetPlayerById(playerBID)
pairing := PairingDisplay{
    PlayerA: playerA.Name,
    PlayerB: playerB.Name,
    // ...
}
```

**Location**: 
- `internal/handlers/admin.go` (Dashboard)
- `internal/handlers/player.go` (Pairings)

**Impact**: This simplifies the code significantly and improves performance by eliminating the need to build player ID-to-name maps from standings.

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

## 7. No Direct Access to Player Information from Pairings ✅ FIXED

**Previous Deficiency**: `Pairing` objects only provide player IDs via `PlayerA()` and `PlayerB()`. Previously, the `Player` struct had unexported fields, requiring workarounds to get player names.

**Status**: ✅ **FIXED** - The `Player.Name` field is now exported, allowing direct access via `GetPlayerById()`.

**Current Implementation**: Player names are now accessed directly:
```go
player, _ := tournament.GetPlayerById(playerID)
playerName := player.Name
```

**Impact**: This eliminates the need for workarounds and provides direct, efficient access to player information.

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

1. ~~**Unexported Player fields**~~ ✅ **FIXED** - Player.Name is now exported
2. **No registration queue** - Requires separate pending player system
3. **No result validation** - Relies on admin accuracy
4. **Manual standings updates** - Easy to forget, could show stale data
5. **No persistence** - Application must handle all file I/O and state management

**Note**: Item #8 (Tournament State Management) is not actually a library deficiency - the library allows multiple `Tournament` objects. The single-tournament limitation is an application design choice.

These workarounds add complexity and potential points of failure to the application. Some could be addressed by library improvements (e.g., exported Player fields, automatic standings updates, result validation), while others are architectural decisions (e.g., authentication, web interface, single tournament instance) that are appropriately handled at the application level.

