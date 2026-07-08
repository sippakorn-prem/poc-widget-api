package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Widget struct {
	Key         string   `json:"key"`
	CompanyName string   `json:"companyName"`
	DisplayName string   `json:"displayName"`
	Color       string   `json:"color"`
	Welcome     string   `json:"welcome"`
	Hosts       []string `json:"hosts"`
}

type WidgetStore struct {
	path    string
	mu      sync.RWMutex
	widgets map[string]Widget
}

type adminPageData struct {
	Widgets        []Widget
	WidgetShellURL string
	APIURL         string
}

func main() {
	widgetOrigin := env("WIDGET_ORIGIN", "http://localhost:5173")
	store := mustLoadStore(env("DATA_FILE", "widgets.json"))
	store.seedDemoWidget(splitEnv("ALLOWED_HOSTS", "http://localhost:5174"))

	e := echo.New()
	e.HideBanner = true

	// The iframe document calls the API from the widget origin; the customer
	// origin is checked separately via the bootstrap host parameter.
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOriginFunc: func(origin string) (bool, error) {
			return origin == widgetOrigin || strings.HasPrefix(origin, "http://localhost"), nil
		},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodOptions},
	}))

	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	e.GET("/admin", func(c echo.Context) error {
		data := adminPageData{
			Widgets:        store.list(),
			WidgetShellURL: env("PUBLIC_WIDGET_SHELL_URL", "http://localhost:5173/shell.js"),
			APIURL:         env("PUBLIC_API_URL", requestOrigin(c)),
		}
		c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
		return adminTemplate.Execute(c.Response(), data)
	})

	e.POST("/admin/widgets", func(c echo.Context) error {
		companyName := strings.TrimSpace(c.FormValue("companyName"))
		displayName := defaultString(c.FormValue("displayName"), companyName)
		if displayName == "" {
			displayName = "ZimpleOmni"
		}

		widget := Widget{
			Key:         generateWidgetKey(),
			CompanyName: defaultString(companyName, displayName),
			DisplayName: displayName,
			Color:       defaultString(c.FormValue("color"), "#4f46e5"),
			Welcome:     defaultString(c.FormValue("welcome"), "Served cross-origin and gated by the POC API."),
			Hosts:       parseHostList(c.FormValue("hosts")),
		}

		if err := store.upsert(widget); err != nil {
			return err
		}
		return c.Redirect(http.StatusFound, "/admin")
	})

	e.POST("/admin/widgets/:key/hosts", func(c echo.Context) error {
		host := normalizeHost(c.FormValue("host"))
		if host != "" {
			if err := store.addHost(c.Param("key"), host); err != nil {
				return err
			}
		}
		return c.Redirect(http.StatusFound, "/admin")
	})

	e.POST("/admin/widgets/:key/hosts/delete", func(c echo.Context) error {
		if err := store.removeHost(c.Param("key"), normalizeHost(c.FormValue("host"))); err != nil {
			return err
		}
		return c.Redirect(http.StatusFound, "/admin")
	})

	e.POST("/admin/widgets/:key/delete", func(c echo.Context) error {
		if err := store.delete(c.Param("key")); err != nil {
			return err
		}
		return c.Redirect(http.StatusFound, "/admin")
	})

	e.GET("/widget/v1/bootstrap", func(c echo.Context) error {
		widget, ok := store.get(c.QueryParam("key"))
		if !ok {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "unknown key"})
		}

		host := normalizeHost(c.QueryParam("host"))
		if !contains(widget.Hosts, host) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "origin not allowed"})
		}

		return c.JSON(http.StatusOK, map[string]any{
			"displayName": widget.DisplayName,
			"color":       widget.Color,
			"welcome":     widget.Welcome,
		})
	})

	e.Logger.Fatal(e.Start(":" + env("PORT", "8080")))
}

func mustLoadStore(path string) *WidgetStore {
	store := &WidgetStore{
		path:    path,
		widgets: map[string]Widget{},
	}

	bytes, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store
	}
	if err != nil {
		panic(err)
	}
	if len(bytes) == 0 {
		return store
	}
	if err := json.Unmarshal(bytes, &store.widgets); err != nil {
		panic(err)
	}
	return store
}

func (s *WidgetStore) seedDemoWidget(hosts []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.widgets["wk_demo"]; ok {
		return
	}

	s.widgets["wk_demo"] = Widget{
		Key:         "wk_demo",
		CompanyName: "Demo Company",
		DisplayName: "ZimpleOmni",
		Color:       "#4f46e5",
		Welcome:     "Served cross-origin and gated by the POC API.",
		Hosts:       normalizeHosts(hosts),
	}
	_ = s.saveLocked()
}

func (s *WidgetStore) get(key string) (Widget, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	widget, ok := s.widgets[key]
	return widget, ok
}

func (s *WidgetStore) list() []Widget {
	s.mu.RLock()
	defer s.mu.RUnlock()

	widgets := make([]Widget, 0, len(s.widgets))
	for _, widget := range s.widgets {
		widgets = append(widgets, widget)
	}
	sort.Slice(widgets, func(i, j int) bool {
		return widgets[i].CompanyName < widgets[j].CompanyName
	})
	return widgets
}

func (s *WidgetStore) upsert(widget Widget) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.widgets[widget.Key] = widget
	return s.saveLocked()
}

func (s *WidgetStore) addHost(key, host string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	widget, ok := s.widgets[key]
	if !ok {
		return nil
	}
	if !contains(widget.Hosts, host) {
		widget.Hosts = append(widget.Hosts, host)
		sort.Strings(widget.Hosts)
	}
	s.widgets[key] = widget
	return s.saveLocked()
}

func (s *WidgetStore) removeHost(key, host string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	widget, ok := s.widgets[key]
	if !ok {
		return nil
	}
	filtered := widget.Hosts[:0]
	for _, value := range widget.Hosts {
		if value != host {
			filtered = append(filtered, value)
		}
	}
	widget.Hosts = filtered
	s.widgets[key] = widget
	return s.saveLocked()
}

func (s *WidgetStore) delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.widgets, key)
	return s.saveLocked()
}

func (s *WidgetStore) saveLocked() error {
	bytes, err := json.MarshalIndent(s.widgets, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, bytes, 0o644)
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func splitEnv(key, fallback string) []string {
	raw := env(key, fallback)
	return strings.Split(raw, ",")
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return fallback
}

func generateWidgetKey() string {
	bytes := make([]byte, 4)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return "wk_" + hex.EncodeToString(bytes)
}

func parseHostList(raw string) []string {
	return normalizeHosts(strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == ',' || r == ' '
	}))
}

func normalizeHosts(hosts []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(hosts))
	for _, host := range hosts {
		normalized := normalizeHost(host)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	host = strings.TrimRight(host, "/")
	if host == "" {
		return ""
	}

	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		parsed, err := url.Parse(host)
		if err == nil && parsed.Scheme != "" && parsed.Host != "" {
			return parsed.Scheme + "://" + strings.ToLower(parsed.Host)
		}
		return strings.ToLower(host)
	}

	lower := strings.ToLower(host)
	if strings.HasPrefix(lower, "localhost") || strings.HasPrefix(lower, "127.0.0.1") {
		return "http://" + lower
	}
	return "https://" + lower
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func requestOrigin(c echo.Context) string {
	scheme := c.Scheme()
	if forwardedProto := c.Request().Header.Get("X-Forwarded-Proto"); forwardedProto != "" {
		scheme = forwardedProto
	}
	return scheme + "://" + c.Request().Host
}

func originOf(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimRight(rawURL, "/")
	}
	return parsed.Scheme + "://" + parsed.Host
}

var adminTemplate = template.Must(template.New("admin").Funcs(template.FuncMap{
	"join":     strings.Join,
	"originOf": originOf,
}).Parse(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Widget Config POC</title>
    <style>
      :root { color-scheme: light; font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; --ink: #111827; --muted: #667085; --line: #e5e7eb; --accent: #2563eb; --bg: #f7f7fb; --surface: #fff; }
      * { box-sizing: border-box; }
      body { margin: 0; color: var(--ink); background: var(--bg); }
      header { padding: 28px min(5vw, 64px); background: var(--surface); border-bottom: 1px solid var(--line); }
      main { width: min(1120px, calc(100% - 32px)); margin: 28px auto 64px; display: grid; gap: 20px; }
      h1, h2, h3, p { margin-top: 0; }
      h1 { margin-bottom: 8px; font-size: 28px; }
      h2 { margin-bottom: 14px; font-size: 20px; }
      p { color: var(--muted); line-height: 1.55; }
      .panel, .widget { background: var(--surface); border: 1px solid var(--line); border-radius: 8px; padding: 18px; }
      .grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
      label { display: grid; gap: 7px; font-weight: 700; font-size: 13px; }
      input, textarea { width: 100%; border: 1px solid var(--line); border-radius: 8px; padding: 10px 12px; font: inherit; }
      textarea { min-height: 88px; resize: vertical; }
      button { border: 0; border-radius: 8px; background: var(--accent); color: #fff; font: inherit; font-weight: 750; min-height: 40px; padding: 0 14px; cursor: pointer; }
      button.secondary { background: #f3f4f6; color: var(--ink); border: 1px solid var(--line); }
      button.danger { background: #dc2626; }
      .actions { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; margin-top: 14px; }
      .widgets { display: grid; gap: 14px; }
      .widget-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
      code, pre { font-family: "SFMono-Regular", Consolas, "Liberation Mono", monospace; }
      code { background: #eef2ff; color: #3730a3; border-radius: 6px; padding: 2px 6px; }
      pre { overflow-x: auto; white-space: pre-wrap; border: 1px solid var(--line); background: #0f172a; color: #e5e7eb; border-radius: 8px; padding: 12px; }
      ul { margin: 0; padding-left: 18px; }
      li { margin: 6px 0; }
      .host-row { display: flex; align-items: center; gap: 8px; }
      .host-row form { margin: 0; }
      .host-form { display: flex; gap: 8px; margin-top: 12px; }
      .host-form input { min-width: 240px; }
      .empty { color: var(--muted); }
      @media (max-width: 760px) { .grid { grid-template-columns: 1fr; } .widget-head, .host-form { flex-direction: column; align-items: stretch; } }
    </style>
  </head>
  <body>
    <header>
      <h1>Widget Config POC</h1>
      <p>Create a demo widget key and allow one or more host origins to use it. No auth, no production database, just enough for the demo.</p>
    </header>
    <main>
      <section class="panel">
        <h2>Create Widget</h2>
        <form method="post" action="/admin/widgets">
          <div class="grid">
            <label>Company name
              <input name="companyName" placeholder="Company A" required>
            </label>
            <label>Widget display name
              <input name="displayName" placeholder="Company A Support">
            </label>
            <label>Brand color
              <input name="color" value="#4f46e5">
            </label>
            <label>Allowed hosts
              <textarea name="hosts" placeholder="company-a.com&#10;localhost:8080"></textarea>
            </label>
          </div>
          <label style="margin-top: 14px;">Welcome message
            <input name="welcome" value="Served cross-origin and gated by the POC API.">
          </label>
          <div class="actions">
            <button type="submit">Create widget key</button>
          </div>
        </form>
      </section>

      <section class="widgets">
        {{range .Widgets}}
          {{$widget := .}}
          <article class="widget">
            <div class="widget-head">
              <div>
                <h2>{{.CompanyName}}</h2>
                <p>Key: <code>{{.Key}}</code> · Display: {{.DisplayName}}</p>
              </div>
              <form method="post" action="/admin/widgets/{{.Key}}/delete">
                <button class="danger" type="submit">Delete</button>
              </form>
            </div>

            <h3>Allowed hosts</h3>
            {{if .Hosts}}
              <ul>
                {{range .Hosts}}
                  <li class="host-row">
                    <span><code>{{.}}</code></span>
                    <form method="post" action="/admin/widgets/{{$widget.Key}}/hosts/delete">
                      <input type="hidden" name="host" value="{{.}}">
                      <button class="secondary" type="submit">Remove</button>
                    </form>
                  </li>
                {{end}}
              </ul>
            {{else}}
              <p class="empty">No allowed hosts yet.</p>
            {{end}}

            <form class="host-form" method="post" action="/admin/widgets/{{.Key}}/hosts">
              <input name="host" placeholder="https://company-a.com or localhost:8080" required>
              <button type="submit">Add host</button>
            </form>

            <h3 style="margin-top: 18px;">Script snippet</h3>
            <pre>&lt;script
  async
  src="{{$.WidgetShellURL}}"
  data-widget-key="{{.Key}}"
  data-api-url="{{$.APIURL}}"&gt;
&lt;/script&gt;</pre>

            <h3>Iframe snippet</h3>
            <pre>&lt;iframe
  title="OmniChat"
  src="{{originOf $.WidgetShellURL}}/frame/?key={{.Key}}&amp;api={{$.APIURL}}"
  style="width: 400px; height: 640px; border: 0;"&gt;
&lt;/iframe&gt;</pre>

            <h3>Bootstrap test</h3>
            <pre>{{$.APIURL}}/widget/v1/bootstrap?key={{.Key}}&amp;host={{if .Hosts}}{{index .Hosts 0}}{{else}}https://example.com{{end}}</pre>
          </article>
        {{else}}
          <article class="widget">
            <p class="empty">No widgets yet. Create one above.</p>
          </article>
        {{end}}
      </section>
    </main>
  </body>
</html>`))
