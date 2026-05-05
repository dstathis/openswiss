package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"

	"github.com/dstathis/openswiss/internal/api"
	"github.com/dstathis/openswiss/internal/email"
	"github.com/dstathis/openswiss/internal/handlers"
	"github.com/dstathis/openswiss/internal/metrics"
	mw "github.com/dstathis/openswiss/internal/middleware"
)

func main() {
	dsn := mustEnv("DATABASE_URL")
	listen := getenv("LISTEN_ADDR", ":8080")
	secureCookies := getenv("SECURE_COOKIES", "true") != "false"
	baseURL := getenv("BASE_URL", "http://localhost:8080")
	rateLimit, _ := strconv.Atoi(getenv("RATE_LIMIT_PER_MIN", "60"))
	if rateLimit <= 0 {
		rateLimit = 60
	}

	database, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()
	for i := 0; i < 30; i++ {
		if err = database.Ping(); err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		log.Fatalf("ping db: %v", err)
	}

	if err := runMigrations(database); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	tmpl, err := loadTemplates("templates")
	if err != nil {
		log.Fatalf("templates: %v", err)
	}
	renderer := &namedTemplate{root: tmpl}

	emailSender := &email.Sender{Config: email.Config{
		Host:     os.Getenv("SMTP_HOST"),
		Port:     getenv("SMTP_PORT", "587"),
		User:     os.Getenv("SMTP_USER"),
		Password: os.Getenv("SMTP_PASSWORD"),
		From:     os.Getenv("SMTP_FROM"),
	}}

	tournamentH := &handlers.TournamentHandler{DB: database, Tmpl: renderer}
	authH := &handlers.AuthHandler{DB: database, Tmpl: renderer, Email: emailSender, BaseURL: baseURL, SecureCookies: secureCookies}
	playerH := &handlers.PlayerHandler{DB: database, Tmpl: renderer}
	adminH := &handlers.AdminHandler{DB: database, Tmpl: renderer}

	tournamentAPI := &api.TournamentAPI{DB: database}
	playersAPI := &api.PlayersAPI{DB: database}
	roundsAPI := &api.RoundsAPI{DB: database}
	playoffAPI := &api.PlayoffAPI{DB: database}
	usersAPI := &api.UsersAPI{DB: database}
	adminAPI := &api.AdminAPI{DB: database}

	collector := metrics.New()

	r := chi.NewRouter()
	r.Use(mw.SecureHeaders)
	r.Use(collector.Wrap)
	r.Use(mw.MaxBodySize(2 << 20))
	r.Use(mw.SessionAuth(database))
	r.Use(mw.APIKeyAuth(database))

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	r.Get("/metrics", collector.Handler())

	// Public web routes (CSRF-protected for state-changing requests).
	r.Group(func(r chi.Router) {
		r.Use(mw.CSRFProtect(secureCookies))

		r.Get("/", tournamentH.Home)
		r.Get("/tournaments", tournamentH.List)
		r.Get("/tournaments/{id}", tournamentH.Detail)

		r.Get("/login", authH.LoginPage)
		r.Post("/login", authH.Login)
		r.Get("/register", authH.RegisterPage)
		r.Post("/register", authH.Register)
		r.Post("/logout", authH.Logout)
		r.Get("/forgot-password", authH.ForgotPasswordPage)
		r.Post("/forgot-password", authH.ForgotPassword)
		r.Get("/reset-password", authH.ResetPasswordPage)
		r.Post("/reset-password", authH.ResetPassword)

		r.Group(func(r chi.Router) {
			r.Use(mw.RequireAuth)

			r.Get("/dashboard", playerH.Dashboard)
			r.Post("/tournaments/{id}/register", tournamentH.Register)
			r.Post("/tournaments/{id}/unregister", tournamentH.Unregister)
			r.Post("/tournaments/{id}/drop", tournamentH.RequestDrop)
			r.Get("/tournaments/{id}/decklist", tournamentH.DecklistPage)
			r.Post("/tournaments/{id}/decklist", tournamentH.SubmitDecklist)
		})

		r.Group(func(r chi.Router) {
			r.Use(mw.RequireAuth)
			r.Use(mw.RequireRole("organizer"))

			r.Get("/tournaments/new", tournamentH.NewPage)
			r.Post("/tournaments/new", tournamentH.Create)
			r.Get("/tournaments/{id}/manage", tournamentH.ManagePage)
			r.Post("/tournaments/{id}/edit", tournamentH.EditTournament)
			r.Post("/tournaments/{id}/open-registration", tournamentH.OpenRegistration)
			r.Post("/tournaments/{id}/start", tournamentH.Start)
			r.Post("/tournaments/{id}/results", tournamentH.SubmitResults)
			r.Post("/tournaments/{id}/next-round", tournamentH.NextRound)
			r.Post("/tournaments/{id}/re-pair", tournamentH.RepairRound)
			r.Post("/tournaments/{id}/finish", tournamentH.Finish)
			r.Post("/tournaments/{id}/add-player", tournamentH.AddPlayer)
			r.Post("/tournaments/{id}/drop-player", tournamentH.DropPlayer)
			r.Post("/tournaments/{id}/start-playoff", tournamentH.StartPlayoff)
			r.Post("/tournaments/{id}/playoff-results", tournamentH.PlayoffResults)
			r.Post("/tournaments/{id}/next-playoff-round", tournamentH.NextPlayoffRound)
			r.Get("/tournaments/{id}/registrations/{regID}/decklist", tournamentH.OrganizerDecklistPage)
			r.Post("/tournaments/{id}/registrations/{regID}/decklist", tournamentH.OrganizerSubmitDecklist)
		})

		r.Group(func(r chi.Router) {
			r.Use(mw.RequireAuth)
			r.Use(mw.RequireRole("admin"))

			r.Get("/admin/users", adminH.UsersPage)
			r.Post("/admin/users/{id}/role", adminH.UpdateRole)
		})
	})

	// REST API (no CSRF; rate-limited; auth by session or API key).
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(mw.RateLimit(rateLimit))

		// Public
		r.Get("/tournaments", tournamentAPI.List)
		r.Get("/tournaments/{id}", tournamentAPI.Get)
		r.Get("/tournaments/{id}/players", playersAPI.List)
		r.Get("/tournaments/{id}/rounds", roundsAPI.ListRounds)
		r.Get("/tournaments/{id}/rounds/current", roundsAPI.GetCurrentRound)
		r.Get("/tournaments/{id}/rounds/{round}", roundsAPI.GetRound)
		r.Get("/tournaments/{id}/standings", roundsAPI.GetStandings)
		r.Get("/tournaments/{id}/playoff", playoffAPI.Get)
		r.Get("/tournaments/{id}/playoff/rounds/current", playoffAPI.GetCurrentRound)

		// Authenticated (session or API key)
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireAuth)

			r.Get("/users/me", usersAPI.GetMe)
			r.Post("/users/me/api-keys", usersAPI.CreateAPIKey)
			r.Get("/users/me/api-keys", usersAPI.ListAPIKeys)
			r.Delete("/users/me/api-keys/{id}", usersAPI.DeleteAPIKey)

			r.Post("/tournaments/{id}/players", playersAPI.Register)
			r.Delete("/tournaments/{id}/players/me", playersAPI.Unregister)
			r.Get("/tournaments/{id}/players/me/decklist", playersAPI.GetDecklist)
			r.Put("/tournaments/{id}/players/me/decklist", playersAPI.SubmitDecklist)

			// Organizer-only
			r.Group(func(r chi.Router) {
				r.Use(mw.RequireRole("organizer"))

				r.Post("/tournaments", tournamentAPI.Create)
				r.Patch("/tournaments/{id}", tournamentAPI.Update)
				r.Delete("/tournaments/{id}", tournamentAPI.Delete)
				r.Post("/tournaments/{id}/open-registration", tournamentAPI.OpenRegistration)
				r.Post("/tournaments/{id}/start", tournamentAPI.Start)
				r.Post("/tournaments/{id}/finish", tournamentAPI.Finish)

				r.Post("/tournaments/{id}/players/add", playersAPI.AddPlayer)
				r.Post("/tournaments/{id}/players/{pid}/drop", playersAPI.DropPlayer)
				r.Get("/tournaments/{id}/players/{pid}/decklist", playersAPI.GetPlayerDecklist)
				r.Get("/tournaments/{id}/registrations/{regID}/decklist", playersAPI.GetRegistrationDecklist)
				r.Put("/tournaments/{id}/registrations/{regID}/decklist", playersAPI.SetRegistrationDecklist)

				r.Post("/tournaments/{id}/rounds/current/results", roundsAPI.SubmitResults)
				r.Post("/tournaments/{id}/rounds/next", roundsAPI.NextRound)

				r.Post("/tournaments/{id}/playoff/start", playoffAPI.Start)
				r.Post("/tournaments/{id}/playoff/rounds/current/results", playoffAPI.SubmitResults)
				r.Post("/tournaments/{id}/playoff/rounds/next", playoffAPI.NextRound)
			})

			// Admin-only
			r.Group(func(r chi.Router) {
				r.Use(mw.RequireRole("admin"))

				r.Get("/admin/users", adminAPI.ListUsers)
				r.Patch("/admin/users/{id}", adminAPI.UpdateUser)
			})
		})
	})

	log.Printf("openswiss listening on %s", listen)
	if err := http.ListenAndServe(listen, r); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s is required", key)
	}
	return v
}

func runMigrations(database *sql.DB) error {
	driver, err := postgres.WithInstance(database, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("driver: %w", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://migrations", "postgres", driver)
	if err != nil {
		return fmt.Errorf("migrator: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("up: %w", err)
	}
	return nil
}

// templateFuncs are exposed to all templates.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"deref": func(v interface{}) interface{} {
			switch p := v.(type) {
			case *string:
				if p == nil {
					return ""
				}
				return *p
			case *int:
				if p == nil {
					return 0
				}
				return *p
			case *int64:
				if p == nil {
					return int64(0)
				}
				return *p
			}
			return v
		},
		"derefInt": func(p *int) int {
			if p == nil {
				return 0
			}
			return *p
		},
		"mul100": func(v float64) float64 { return v * 100 },
	}
}

// loadTemplates parses the layout once and one parsed *Template per page,
// each containing its page + the shared layout.
func loadTemplates(root string) (map[string]*template.Template, error) {
	layouts, err := filepath.Glob(filepath.Join(root, "layouts", "*.html"))
	if err != nil {
		return nil, err
	}
	pages, err := filepath.Glob(filepath.Join(root, "pages", "*.html"))
	if err != nil {
		return nil, err
	}
	out := map[string]*template.Template{}
	for _, page := range pages {
		name := filepath.Base(page)
		files := append([]string{}, layouts...)
		files = append(files, page)
		t, err := template.New(name).Funcs(templateFuncs()).ParseFiles(files...)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		out[name] = t
	}
	if len(out) == 0 {
		return nil, fs.ErrNotExist
	}
	return out, nil
}

// namedTemplate satisfies handlers.TemplateRenderer by dispatching ExecuteTemplate
// to the page-specific *template.Template, executing its "layout" block.
type namedTemplate struct {
	root map[string]*template.Template
}

func (n *namedTemplate) ExecuteTemplate(w io.Writer, name string, data interface{}) error {
	t, ok := n.root[name]
	if !ok {
		return fmt.Errorf("template %q not found", name)
	}
	return t.ExecuteTemplate(w, "layout", data)
}
