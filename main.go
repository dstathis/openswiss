// This file is part of OpenSwiss.
//
// OpenSwiss is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// OpenSwiss is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with OpenSwiss. If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"openswiss/internal/auth"
	"openswiss/internal/handlers"
	"openswiss/internal/storage"
)

func main() {
	port := flag.String("port", "8080", "Port to listen on")
	adminPassword := flag.String("admin-password", "", "Admin password (default: admin123)")
	flag.Parse()

	if *adminPassword == "" {
		*adminPassword = "admin123"
	}

	// Initialize storage
	tournamentStorage, err := storage.NewTournamentStorage()
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	// Initialize auth
	authService := auth.NewAuth(*adminPassword)

	// Initialize handlers
	playerHandlers := handlers.NewPlayerHandlers(tournamentStorage, authService)
	adminHandlers := handlers.NewAdminHandlers(tournamentStorage, authService)
	authHandlers := handlers.NewAuthHandlers(authService)

	// Setup routes
	mux := http.NewServeMux()

	// Public routes
	mux.HandleFunc("/", authService.OptionalAuth(playerHandlers.Home))
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			playerHandlers.RegisterGet(w, r)
		} else if r.Method == http.MethodPost {
			playerHandlers.RegisterPost(w, r)
		}
	})
	mux.HandleFunc("/standings", authService.OptionalAuth(playerHandlers.Standings))
	mux.HandleFunc("/pairings", authService.OptionalAuth(playerHandlers.Pairings))

	// Auth routes
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			authHandlers.LoginGet(w, r)
		} else if r.Method == http.MethodPost {
			authHandlers.LoginPost(w, r)
		}
	})
	mux.HandleFunc("/logout", authHandlers.Logout)

	// Admin routes
	mux.HandleFunc("/admin/dashboard", authService.RequireAdmin(adminHandlers.Dashboard))
	mux.HandleFunc("/admin/accept", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			authService.RequireAdmin(adminHandlers.AcceptPlayer)(w, r)
		}
	})
	mux.HandleFunc("/admin/reject", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			authService.RequireAdmin(adminHandlers.RejectPlayer)(w, r)
		}
	})
	mux.HandleFunc("/admin/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			authService.RequireAdmin(adminHandlers.StartTournament)(w, r)
		}
	})
	mux.HandleFunc("/admin/pair", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			authService.RequireAdmin(adminHandlers.Pair)(w, r)
		}
	})
	mux.HandleFunc("/admin/next-round", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			authService.RequireAdmin(adminHandlers.NextRound)(w, r)
		}
	})
	mux.HandleFunc("/admin/add-result", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			authService.RequireAdmin(adminHandlers.AddResult)(w, r)
		}
	})
	mux.HandleFunc("/admin/update-standings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			authService.RequireAdmin(adminHandlers.UpdateStandings)(w, r)
		}
	})
	mux.HandleFunc("/admin/remove-player", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			authService.RequireAdmin(adminHandlers.RemovePlayer)(w, r)
		}
	})

	fmt.Printf("Starting server on port %s\n", *port)
	fmt.Printf("Admin password: %s\n", *adminPassword)
	fmt.Printf("Open http://localhost:%s in your browser\n", *port)

	log.Fatal(http.ListenAndServe(":"+*port, mux))
}



