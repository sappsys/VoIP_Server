package web

import (
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/sappsys/VoIP_Server/internal/config"
	"github.com/sappsys/VoIP_Server/internal/pbx"
	"github.com/sappsys/VoIP_Server/internal/store"
	"github.com/sappsys/VoIP_Server/internal/version"
	"github.com/sappsys/VoIP_Server/internal/web/auth"
)

type Server struct {
	cfg          *config.Config
	cfgPath      string
	extDir       string
	phonebookDir string
	store        *store.Store
	pbx          *pbx.Server
	sessions     *auth.Sessions
	log          *slog.Logger
}

func New(cfg *config.Config, cfgPath, extDir, phonebookDir string, st *store.Store, pbxSrv *pbx.Server, log *slog.Logger) *Server {
	return &Server{
		cfg:          cfg,
		cfgPath:      cfgPath,
		extDir:       extDir,
		phonebookDir: phonebookDir,
		store:        st,
		pbx:          pbxSrv,
		sessions:     auth.NewSessions(cfg.Web.SessionSecret),
		log:          log,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/logout", s.handleLogout)
	mux.HandleFunc("/", s.auth(s.handleDashboard))
	mux.HandleFunc("/status", s.auth(s.handleStatus))
	mux.HandleFunc("/status/fragment", s.auth(s.handleStatusFragment))
	mux.HandleFunc("/extensions", s.auth(s.handleExtensions))
	mux.HandleFunc("/extensions/save", s.auth(s.handleExtensionSave))
	mux.HandleFunc("/extensions/delete", s.auth(s.handleExtensionDelete))
	mux.HandleFunc("/hunt", s.auth(s.handleHunt))
	mux.HandleFunc("/hunt/save", s.auth(s.handleHuntSave))
	mux.HandleFunc("/hunt/delete", s.auth(s.handleHuntDelete))
	mux.HandleFunc("/conferences", s.auth(s.handleConferences))
	mux.HandleFunc("/conferences/save", s.auth(s.handleConferenceSave))
	mux.HandleFunc("/conferences/delete", s.auth(s.handleConferenceDelete))
	mux.HandleFunc("/paging", s.auth(s.handlePaging))
	mux.HandleFunc("/paging/save", s.auth(s.handlePagingSave))
	mux.HandleFunc("/paging/delete", s.auth(s.handlePagingDelete))
	mux.HandleFunc("/trunks", s.auth(s.handleTrunks))
	mux.HandleFunc("/trunks/route", s.auth(s.handleTrunkRouteSave))
	mux.HandleFunc("/users", s.auth(s.handleUsers))
	mux.HandleFunc("/users/save", s.auth(s.handleUserSave))
	mux.HandleFunc("/users/delete", s.auth(s.handleUserDelete))
	// Remote phonebook: XML endpoints are public (IP phones fetch them without login);
	// the admin management pages require authentication.
	mux.HandleFunc("/phonebook", s.auth(s.handlePhonebook))
	mux.HandleFunc("/phonebook/save", s.auth(s.handlePhonebookSave))
	mux.HandleFunc("/phonebook/delete", s.auth(s.handlePhonebookDelete))
	mux.HandleFunc("/phonebook/", s.handlePhonebookStatic)
	mux.HandleFunc("/reload", s.auth(s.handleReload))
	return mux
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.sessions.Username(r) == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func (s *Server) render(w http.ResponseWriter, content string) {
	page := strings.Replace(layout, "{{CONTENT}}", content, 1)
	page = strings.Replace(page, "{{VERSION}}", version.Version, -1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(page))
}

func (s *Server) loadExtensions() (map[string]*config.Extension, error) {
	return config.LoadExtensions(s.extDir)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html")
		page := strings.Replace(loginPage, "{{ERR}}", "", 1)
		page = strings.Replace(page, "{{VERSION}}", version.Version, -1)
		w.Write([]byte(page))
		return
	}
	user := r.FormValue("username")
	pass := r.FormValue("password")
	u, err := s.store.GetWebUserByUsername(user)
	if err != nil || u == nil || !store.CheckPassword(u.PasswordHash, pass) {
		errHTML := `<p style="color:red">Invalid credentials</p>`
		w.Header().Set("Content-Type", "text/html")
		page := strings.Replace(loginPage, "{{ERR}}", errHTML, 1)
		page = strings.Replace(page, "{{VERSION}}", version.Version, -1)
		w.Write([]byte(page))
		return
	}
	s.sessions.Set(w, user)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.sessions.Clear(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	exts, _ := s.loadExtensions()
	reg := s.pbx.RegisteredExtensions()
	body := fmt.Sprintf(`<h1>Dashboard</h1><p>%s</p><p>Extensions configured: %d | Registered: %d</p><p>SIP: %s:%d</p><p>Version: %s</p>
<form hx-post="/reload"><button>Reload config</button></form>`,
		html.EscapeString(s.pbx.Stats()), len(exts), len(reg),
		html.EscapeString(s.cfg.Server.BindHost), s.cfg.Server.BindPort, html.EscapeString(version.Version))
	s.render(w, body)
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(s.cfgPath)
	if err == nil {
		s.cfg = cfg
		s.pbx.ReloadConfig(cfg)
		exts, _ := s.loadExtensions()
		s.pbx.ReloadExtensions(exts)
	}
	s.handleDashboard(w, r)
}

func (s *Server) handleExtensions(w http.ResponseWriter, r *http.Request) {
	exts, _ := s.loadExtensions()
	var rows string
	for _, e := range exts {
		rows += fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%v</td><td>%v</td><td>%v</td><td>
<form hx-post="/extensions/delete" hx-target="body"><input type="hidden" name="extension" value="%s"><button>Delete</button></form></td></tr>`,
			html.EscapeString(e.Extension), html.EscapeString(e.DisplayName), e.Enabled, e.CallWaiting, e.DND, html.EscapeString(e.Extension))
	}
	form := `<h1>Extensions</h1><table><tr><th>Ext</th><th>Name</th><th>Enabled</th><th>CW</th><th>DND</th><th></th></tr>` + rows + `</table>
<h2>Add / update</h2><form hx-post="/extensions/save" hx-target="body">
<input name="extension" placeholder="101" required>
<input name="display_name" placeholder="Name">
<input name="password" placeholder="password" required>
<label><input type="checkbox" name="enabled" value="1" checked> Enabled</label>
<label><input type="checkbox" name="call_waiting" value="1" checked> Call waiting</label>
<input name="max_simultaneous_calls" placeholder="4" value="4">
<button>Save</button></form>`
	s.render(w, form)
}

func (s *Server) handleExtensionSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	max := 4
	fmt.Sscan(r.FormValue("max_simultaneous_calls"), &max)
	ext := &config.Extension{
		Extension:            r.FormValue("extension"),
		DisplayName:          r.FormValue("display_name"),
		Password:             r.FormValue("password"),
		Enabled:              r.FormValue("enabled") == "1",
		CallWaiting:          r.FormValue("call_waiting") == "1",
		MaxSimultaneousCalls: max,
	}
	if ext.Extension == "" || ext.Password == "" {
		http.Error(w, "missing fields", 400)
		return
	}
	if ext.DisplayName == "" {
		ext.DisplayName = ext.Extension
	}
	if existing, _ := s.loadExtensions(); existing != nil {
		if old, ok := existing[ext.Extension]; ok {
			ext.DND = old.DND
			ext.VideoEnabled = old.VideoEnabled
			ext.Voicemail = old.Voicemail
		}
	}
	_ = os.MkdirAll(s.extDir, 0o755)
	path := filepath.Join(s.extDir, ext.Extension+".toml")
	f, err := os.Create(path)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = toml.NewEncoder(f).Encode(ext)
	f.Close()
	exts, _ := s.loadExtensions()
	s.pbx.ReloadExtensions(exts)
	s.handleExtensions(w, r)
}

func (s *Server) handleExtensionDelete(w http.ResponseWriter, r *http.Request) {
	_ = os.Remove(filepath.Join(s.extDir, r.FormValue("extension")+".toml"))
	exts, _ := s.loadExtensions()
	s.pbx.ReloadExtensions(exts)
	s.handleExtensions(w, r)
}

func (s *Server) handleHunt(w http.ResponseWriter, r *http.Request) {
	groups, _ := s.store.ListHuntGroups()
	var rows string
	for _, g := range groups {
		members, _ := s.store.HuntMembers(g.ID)
		rows += fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td><td>%d</td><td>%s</td><td>
<form hx-post="/hunt/delete"><input type="hidden" name="id" value="%d"><button>Delete</button></form></td></tr>`,
			html.EscapeString(g.Number), html.EscapeString(g.Name), html.EscapeString(g.Strategy),
			g.RingTimeoutSeconds, html.EscapeString(strings.Join(members, ",")), g.ID)
	}
	content := `<h1>Hunt groups (500-599)</h1><table><tr><th>Number</th><th>Name</th><th>Strategy</th><th>Timeout</th><th>Members</th><th></th></tr>` + rows + `</table>
<form hx-post="/hunt/save"><input name="number" placeholder="500" required><input name="name" placeholder="Sales">
<select name="strategy"><option value="simultaneous">simultaneous</option><option value="sequential">sequential</option></select>
<input name="ring_timeout" placeholder="20" value="20"><input name="members" placeholder="101,102"><button>Save</button></form>`
	s.render(w, content)
}

func (s *Server) handleHuntSave(w http.ResponseWriter, r *http.Request) {
	timeout := 20
	fmt.Sscan(r.FormValue("ring_timeout"), &timeout)
	_ = s.store.CreateHuntGroup(r.FormValue("name"), r.FormValue("number"), r.FormValue("strategy"), timeout)
	g, _ := s.store.GetHuntGroupByNumber(r.FormValue("number"))
	if g != nil {
		_ = s.store.SetHuntMembers(g.ID, splitCSV(r.FormValue("members")))
	}
	s.handleHunt(w, r)
}

func (s *Server) handleHuntDelete(w http.ResponseWriter, r *http.Request) {
	var id int64
	fmt.Sscan(r.FormValue("id"), &id)
	g, _ := s.store.GetHuntGroup(id)
	if g != nil {
		if ref, _ := s.store.HuntGroupReferenced(g.Number); ref {
			http.Error(w, "group referenced by trunk route", 400)
			return
		}
	}
	_ = s.store.DeleteHuntGroup(id)
	s.handleHunt(w, r)
}

func (s *Server) handleConferences(w http.ResponseWriter, r *http.Request) {
	list, _ := s.store.ListConferences()
	var rows string
	for _, c := range list {
		rows += fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%d</td><td>
<form hx-post="/conferences/delete"><input type="hidden" name="id" value="%d"><button>Delete</button></form></td></tr>`,
			html.EscapeString(c.Number), html.EscapeString(c.Name), c.MaxParticipants, c.ID)
	}
	content := `<h1>Conferences (600-699)</h1><table><tr><th>Number</th><th>Name</th><th>Max</th><th></th></tr>` + rows + `</table>
<form hx-post="/conferences/save"><input name="number" placeholder="600" required><input name="name" placeholder="Room">
<input name="pin" placeholder="PIN" required><input name="max_participants" placeholder="16" value="16"><button>Save</button></form>`
	s.render(w, content)
}

func (s *Server) handleConferenceSave(w http.ResponseWriter, r *http.Request) {
	max := 16
	fmt.Sscan(r.FormValue("max_participants"), &max)
	_ = s.store.CreateConference(r.FormValue("name"), r.FormValue("number"), r.FormValue("pin"), max)
	s.handleConferences(w, r)
}

func (s *Server) handleConferenceDelete(w http.ResponseWriter, r *http.Request) {
	var id int64
	fmt.Sscan(r.FormValue("id"), &id)
	_ = s.store.DeleteConference(id)
	s.handleConferences(w, r)
}

func (s *Server) handlePaging(w http.ResponseWriter, r *http.Request) {
	list, _ := s.store.ListPagingGroups()
	var rows string
	for _, p := range list {
		members, _ := s.store.PagingMembers(p.ID)
		rows += fmt.Sprintf(`<tr><td>*%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>
<form hx-post="/paging/delete"><input type="hidden" name="id" value="%d"><button>Delete</button></form></td></tr>`,
			html.EscapeString(p.Code), html.EscapeString(p.Name), html.EscapeString(p.Mode),
			html.EscapeString(p.MulticastAddress), html.EscapeString(strings.Join(members, ",")), p.ID)
	}
	content := `<h1>Paging (*80-*99)</h1><table><tr><th>Code</th><th>Name</th><th>Mode</th><th>Multicast</th><th>Members</th><th></th></tr>` + rows + `</table>
<form hx-post="/paging/save"><input name="code" placeholder="80" required><input name="name" placeholder="All hands">
<select name="mode"><option value="unicast">unicast</option><option value="multicast">multicast</option></select>
<input name="multicast_address" placeholder="224.0.1.100:10000"><input name="channel" placeholder="0" value="0">
<input name="members" placeholder="101,102"><button>Save</button></form>`
	s.render(w, content)
}

func (s *Server) handlePagingSave(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.FormValue("code"), "*")
	ch := 0
	fmt.Sscan(r.FormValue("channel"), &ch)
	_ = s.store.CreatePagingGroup(r.FormValue("name"), code, r.FormValue("mode"), r.FormValue("multicast_address"), ch)
	g, _ := s.store.GetPagingByCode(code)
	if g != nil {
		_ = s.store.SetPagingMembers(g.ID, splitCSV(r.FormValue("members")))
	}
	s.handlePaging(w, r)
}

func (s *Server) handlePagingDelete(w http.ResponseWriter, r *http.Request) {
	var id int64
	fmt.Sscan(r.FormValue("id"), &id)
	_ = s.store.DeletePagingGroup(id)
	s.handlePaging(w, r)
}

func (s *Server) handleTrunks(w http.ResponseWriter, r *http.Request) {
	routes, _ := s.store.ListTrunkRoutes()
	routeMap := map[int]store.TrunkRoute{}
	for _, rt := range routes {
		routeMap[rt.TrunkID] = rt
	}

	var rows string
	for _, t := range s.cfg.EnabledTrunks() {
		rt := routeMap[t.ID]
		if rt.RouteType == "" {
			rt.RouteType = "all"
		}
		rows += fmt.Sprintf(`<tr><td>%d</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>
<tr><td colspan="6"><form hx-post="/trunks/route">
<input type="hidden" name="trunk_id" value="%d">
<select name="route_type"><option value="all" %s>All extensions</option>
<option value="extension" %s>Extension</option><option value="group" %s>Hunt group</option></select>
<input name="route_target" value="%s" placeholder="101 or 500"><button>Save route</button></form></td></tr>`,
			t.ID, html.EscapeString(t.Name), html.EscapeString(t.Prefix), html.EscapeString(t.Server),
			html.EscapeString(rt.RouteType), html.EscapeString(rt.RouteTarget),
			t.ID, sel(rt.RouteType, "all"), sel(rt.RouteType, "extension"), sel(rt.RouteType, "group"), html.EscapeString(rt.RouteTarget))
	}
	content := `<h1>Trunks</h1><p>Connection details are in <code>config.toml</code>. Set inbound routing below.</p>
<table><tr><th>ID</th><th>Name</th><th>Prefix</th><th>Server</th><th>Route</th><th>Target</th></tr>` + rows + `</table>`
	s.render(w, content)
}

func sel(have, want string) string {
	if have == want {
		return "selected"
	}
	return ""
}

func (s *Server) handleTrunkRouteSave(w http.ResponseWriter, r *http.Request) {
	trunkID := 0
	fmt.Sscan(r.FormValue("trunk_id"), &trunkID)
	_ = s.store.SaveTrunkRoute(store.TrunkRoute{
		TrunkID:     trunkID,
		RouteType:   r.FormValue("route_type"),
		RouteTarget: r.FormValue("route_target"),
	})
	s.handleTrunks(w, r)
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	users, _ := s.store.ListWebUsers()
	var rows string
	for _, u := range users {
		rows += fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>
<form hx-post="/users/delete"><input type="hidden" name="id" value="%d"><button>Delete</button></form></td></tr>`,
			html.EscapeString(u.Username), html.EscapeString(u.Role), u.ID)
	}
	content := `<h1>Web users</h1><table><tr><th>User</th><th>Role</th><th></th></tr>` + rows + `</table>
<form hx-post="/users/save"><input name="username"><input name="password"><input name="role" value="admin"><button>Save</button></form>
<p>Users in config.toml are synced on startup.</p>`
	s.render(w, content)
}

func (s *Server) handleUserSave(w http.ResponseWriter, r *http.Request) {
	hash, err := store.HashPassword(r.FormValue("password"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = s.store.UpsertWebUser(r.FormValue("username"), hash, r.FormValue("role"))
	s.handleUsers(w, r)
}

func (s *Server) handleUserDelete(w http.ResponseWriter, r *http.Request) {
	var id int64
	fmt.Sscan(r.FormValue("id"), &id)
	_ = s.store.DeleteWebUser(id)
	s.handleUsers(w, r)
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
