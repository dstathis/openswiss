# OpenSwiss - Tournament Management Web Application

A web application for managing Swiss-style tournaments built with Go and the [swisstools](https://pkg.go.dev/github.com/dstathis/swisstools) library.

## Features

### Player Features
- Register for tournaments (requires admin approval)
- View current tournament standings with tiebreakers
- View round pairings
- Browse tournament status and information

### Admin Features
- Accept or reject player registrations
- Start tournaments
- Generate pairings for rounds
- Record match results
- Update standings
- Advance to next round
- Remove players from tournaments
- Full tournament management dashboard

## Installation

1. Clone the repository:
```bash
git clone <repository-url>
cd openswiss
```

2. Install dependencies:
```bash
go mod download
```

3. Build the application:
```bash
go build -o openswiss
```

4. Run the application:
```bash
./openswiss
```

Or specify custom port and admin password:
```bash
./openswiss -port 8080 -admin-password your-secure-password
```

## Usage

### Default Access
- **Player interface**: http://localhost:8080
- **Admin login**: http://localhost:8080/login
- **Default admin password**: `admin123` (change this in production!)

### Tournament Workflow

1. **Setup Phase**:
   - Players register for the tournament
   - Admin accepts players from the pending list
   - Admin starts the tournament when ready

2. **Tournament In Progress**:
   - Admin creates pairings for each round
   - Admin records match results (wins, losses, draws)
   - Admin updates standings after all matches are complete
   - Players can view standings and pairings
   - Admin advances to the next round

3. **Administration**:
   - All tournament data is persisted to `data/tournament.json`
   - Pending players are stored in `data/pending_players.json`
   - Tournament state can be resumed after restart

## Project Structure

```
openswiss/
├── main.go                 # Application entry point
├── go.mod                  # Go module dependencies
├── internal/
│   ├── auth/              # Authentication and authorization
│   ├── handlers/          # HTTP request handlers
│   └── storage/           # Tournament data persistence
├── templates/             # HTML templates
│   ├── base.html          # Base template
│   ├── player/            # Player-facing templates
│   ├── admin/             # Admin templates
│   └── auth/              # Authentication templates
└── data/                  # Data storage (created at runtime)
```

## Extending the Application

### Adding New Features

- **Player result submission**: Could allow players to submit their own match results for admin approval
- **Tournament brackets**: Visual representation of standings and pairings
- **Email notifications**: Notify players of pairings and standings updates
- **Tournament history**: View past tournaments and results
- **Multiple tournaments**: Support running multiple tournaments simultaneously

### Security Considerations

For production use, consider:
- Changing the default admin password
- Implementing password hashing (currently plaintext)
- Adding HTTPS/TLS support
- Implementing CSRF protection
- Adding rate limiting
- Database-backed storage instead of JSON files

## License

This project is licensed under the GNU Affero General Public License (AGPL) version 3. See the [LICENSE.md](./LICENSE.md) file for details.

The swisstools library this application depends on is also licensed under AGPL-3.0.

## Contributing

Contributions are welcome! Please open issues or pull requests for any improvements.



