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
	mux.HandleFunc("/web/", s.handleWebStatic)
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
	mux.HandleFunc("/backup", s.auth(s.handleBackup))
	mux.HandleFunc("/backup/download", s.auth(s.handleBackupDownload))
	mux.HandleFunc("/backup/restore", s.auth(s.handleBackupRestore))
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

const htmxScript = `<script src="https://unpkg.com/htmx.org@1.9.12"></script>`

func (s *Server) render(w http.ResponseWriter, r *http.Request, content string) {
	s.renderWithHead(w, r, content, "")
}

func (s *Server) renderWithHead(w http.ResponseWriter, r *http.Request, content, extraHead string) {
	page := strings.Replace(layout, "{{CONTENT}}", content, 1)
	page = strings.Replace(page, "{{VERSION}}", version.Version, -1)
	page = strings.Replace(page, "{{EXTRA_HEAD}}", extraHead, 1)
	adminNav := ""
	if s.isAdmin(r) {
		adminNav = `<a href="/users">Users</a><a href="/backup">Backup</a>`
	}
	page = strings.Replace(page, "{{ADMIN_NAV}}", adminNav, 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(page))
}

func (s *Server) loadExtensions() (map[string]*config.Extension, error) {
	return config.LoadExtensions(s.extDir, s.cfg.Limits.MaxCallsPerExtension)
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
		errHTML := `<p class="error">Invalid credentials</p>`
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
	admin := s.isAdmin(r)
	body := pageHeader("Dashboard", html.EscapeString(s.pbx.Stats())) +
		panel("", statGrid(
			statCard("Extensions configured", fmt.Sprintf("%d", len(exts)))+
				statCard("Registered", fmt.Sprintf("%d", len(reg)))+
				statCard("SIP listen", fmt.Sprintf("%s:%d", html.EscapeString(s.cfg.Server.BindHost), s.cfg.Server.BindPort))+
				statCard("Version", html.EscapeString(version.Version)),
		)) +
		adminOnly(admin, panel("Reload configuration", formPost("/reload", formActions(`<button type="submit">Reload config</button>`))))
	s.render(w, r, body)
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) || !s.requireAdmin(w, r) {
		return
	}
	cfg, err := config.LoadConfig(s.cfgPath)
	if err == nil {
		s.cfg = cfg
		if s.pbx != nil {
			s.pbx.ReloadConfig(cfg)
			exts, _ := s.loadExtensions()
			s.pbx.ReloadExtensions(exts)
		}
	}
	redirectTo(w, r, "/")
}

func (s *Server) handleExtensions(w http.ResponseWriter, r *http.Request) {
	exts, _ := s.loadExtensions()
	admin := s.isAdmin(r)
	editExt := r.URL.Query().Get("edit")
	var editing *config.Extension
	if editExt != "" {
		editing = exts[editExt]
	}

	var rows string
	for _, e := range exts {
		actions := ""
		if admin {
			actions = rowActions(
				editLink("/extensions?edit="+e.Extension),
				deleteForm("/extensions/delete", "extension", e.Extension),
			)
		}
		rows += fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%v</td><td>%v</td><td>%v</td><td>%s</td></tr>`,
			html.EscapeString(e.Extension), html.EscapeString(e.DisplayName), e.Enabled, e.CallWaiting, e.DND, actions)
	}
	headers := th("Ext", "Name", "Enabled", "CW", "DND")
	if admin {
		headers += th("Actions")
	}

	extAttrs := ""
	passRequired := ` required`
	passPlaceholder := "password"
	if editing != nil {
		extAttrs = ` readonly`
		passRequired = ""
		passPlaceholder = "Leave blank to keep current password"
	}
	enabled := editing == nil || editing.Enabled
	cw := editing == nil || editing.CallWaiting
	maxCalls := fmt.Sprintf("%d", config.DefaultMaxCallsPerExtension)
	if editing != nil && editing.MaxSimultaneousCalls > 0 {
		maxCalls = fmt.Sprintf("%d", editing.MaxSimultaneousCalls)
	}
	extVal := editExt
	nameVal := ""
	if editing != nil {
		nameVal = editing.DisplayName
	}

	body := pageHeader("Extensions", "Manage SIP extensions, credentials, and call settings.") +
		panel("Configured extensions", dataTable(headers, rows)) +
		adminOnly(admin, editPanel(editPanelTitle(editing != nil, "Add extension", "Edit extension"), formPost("/extensions/save",
			field("Extension", fmt.Sprintf(`<input name="extension" placeholder="101" required%s%s>`, valAttr(extVal), extAttrs))+
				field("Display name", fmt.Sprintf(`<input name="display_name" placeholder="Name"%s>`, valAttr(nameVal)))+
				field("Password", fmt.Sprintf(`<input name="password" placeholder="%s"%s>`, html.EscapeString(passPlaceholder), passRequired))+
				field("Max simultaneous calls", fmt.Sprintf(`<input name="max_simultaneous_calls" placeholder="%d"%s>`, config.DefaultMaxCallsPerExtension, valAttr(maxCalls)))+
				checkField("Enabled", fmt.Sprintf(`<input type="checkbox" name="enabled" value="1"%s>`, checkedAttr(enabled)))+
				checkField("Call waiting", fmt.Sprintf(`<input type="checkbox" name="call_waiting" value="1"%s>`, checkedAttr(cw)))+
				editFormActions(editing != nil, "/extensions",
					fmt.Sprintf(`<button type="submit">%s</button>`, html.EscapeString(editSubmitLabel(editing != nil)+" extension"))),
		)))
	s.render(w, r, body)
}

func (s *Server) handleExtensionSave(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) || !s.requireAdmin(w, r) {
		return
	}
	max := s.cfg.Limits.MaxCallsPerExtension
	fmt.Sscan(r.FormValue("max_simultaneous_calls"), &max)
	if max <= 0 {
		max = config.DefaultMaxCallsPerExtension
	}
	ext := &config.Extension{
		Extension:            r.FormValue("extension"),
		DisplayName:          r.FormValue("display_name"),
		Password:             r.FormValue("password"),
		Enabled:              r.FormValue("enabled") == "1",
		CallWaiting:          r.FormValue("call_waiting") == "1",
		MaxSimultaneousCalls: max,
	}
	if ext.Extension == "" {
		http.Error(w, "missing extension", 400)
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
			if ext.Password == "" {
				ext.Password = old.Password
			}
		}
	}
	if ext.Password == "" {
		http.Error(w, "password required", 400)
		return
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
	if s.pbx != nil {
		s.pbx.ReloadExtensions(exts)
	}
	redirectTo(w, r, "/extensions")
}

func (s *Server) handleExtensionDelete(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) || !s.requireAdmin(w, r) {
		return
	}
	_ = os.Remove(filepath.Join(s.extDir, r.FormValue("extension")+".toml"))
	exts, _ := s.loadExtensions()
	if s.pbx != nil {
		s.pbx.ReloadExtensions(exts)
	}
	redirectTo(w, r, "/extensions")
}

func (s *Server) handleHunt(w http.ResponseWriter, r *http.Request) {
	groups, _ := s.store.ListHuntGroups()
	admin := s.isAdmin(r)
	editID := queryEditID(r)
	var editing *store.HuntGroup
	if editID > 0 {
		editing, _ = s.store.GetHuntGroup(editID)
	}
	editMembers := ""
	if editing != nil {
		if members, err := s.store.HuntMembers(editing.ID); err == nil {
			editMembers = strings.Join(members, ", ")
		}
	}

	var rows string
	for _, g := range groups {
		members, _ := s.store.HuntMembers(g.ID)
		actions := ""
		if admin {
			actions = rowActions(
				editLink(fmt.Sprintf("/hunt?edit=%d", g.ID)),
				deleteForm("/hunt/delete", "id", fmt.Sprintf("%d", g.ID)),
			)
		}
		rows += fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td><td>%d</td><td>%s</td><td>%s</td></tr>`,
			html.EscapeString(g.Number), html.EscapeString(g.Name), html.EscapeString(g.Strategy),
			g.RingTimeoutSeconds, html.EscapeString(strings.Join(members, ", ")), actions)
	}
	headers := th("Number", "Name", "Strategy", "Timeout", "Members")
	if admin {
		headers += th("Actions")
	}

	numberAttrs := ""
	numberVal := ""
	nameVal := ""
	strategy := "simultaneous"
	timeout := "20"
	if editing != nil {
		numberAttrs = ` readonly`
		numberVal = editing.Number
		nameVal = editing.Name
		strategy = editing.Strategy
		timeout = fmt.Sprintf("%d", editing.RingTimeoutSeconds)
	}
	hiddenID := ""
	if editing != nil {
		hiddenID = hiddenField("id", fmt.Sprintf("%d", editing.ID))
	}

	content := pageHeader("Hunt groups", "Ring groups in the 500–599 range.") +
		panel("Groups", dataTable(headers, rows)) +
		adminOnly(admin, editPanel(editPanelTitle(editing != nil, "Add group", "Edit group"), formPost("/hunt/save",
			hiddenID+
				field("Number", fmt.Sprintf(`<input name="number" placeholder="500" required%s%s>`, valAttr(numberVal), numberAttrs))+
				field("Name", fmt.Sprintf(`<input name="name" placeholder="Sales"%s>`, valAttr(nameVal)))+
				field("Strategy", strategySelect(strategy))+
				field("Ring timeout", fmt.Sprintf(`<input name="ring_timeout" placeholder="20"%s>`, valAttr(timeout)))+
				field("Members", fmt.Sprintf(`<input name="members" placeholder="101,102"%s>`, valAttr(editMembers)))+
				editFormActions(editing != nil, "/hunt",
					fmt.Sprintf(`<button type="submit">%s</button>`, html.EscapeString(editSubmitLabel(editing != nil)+" group"))),
		)))
	s.render(w, r, content)
}

func (s *Server) handleHuntSave(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) || !s.requireAdmin(w, r) {
		return
	}
	timeout := 20
	fmt.Sscan(r.FormValue("ring_timeout"), &timeout)
	name := r.FormValue("name")
	number := r.FormValue("number")
	strategy := r.FormValue("strategy")
	members := splitCSV(r.FormValue("members"))
	id := formID(r)
	if id > 0 {
		_ = s.store.UpdateHuntGroup(id, name, number, strategy, timeout)
		_ = s.store.SetHuntMembers(id, members)
	} else {
		_ = s.store.CreateHuntGroup(name, number, strategy, timeout)
		g, _ := s.store.GetHuntGroupByNumber(number)
		if g != nil {
			_ = s.store.SetHuntMembers(g.ID, members)
		}
	}
	redirectTo(w, r, "/hunt")
}

func (s *Server) handleHuntDelete(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) || !s.requireAdmin(w, r) {
		return
	}
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
	redirectTo(w, r, "/hunt")
}

func (s *Server) handleConferences(w http.ResponseWriter, r *http.Request) {
	list, _ := s.store.ListConferences()
	admin := s.isAdmin(r)
	editID := queryEditID(r)
	var editing *store.Conference
	if editID > 0 {
		editing, _ = s.store.GetConference(editID)
	}

	var rows string
	for _, c := range list {
		actions := ""
		if admin {
			actions = rowActions(
				editLink(fmt.Sprintf("/conferences?edit=%d", c.ID)),
				deleteForm("/conferences/delete", "id", fmt.Sprintf("%d", c.ID)),
			)
		}
		rows += fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%d</td><td>%s</td></tr>`,
			html.EscapeString(c.Number), html.EscapeString(c.Name), c.MaxParticipants, actions)
	}
	headers := th("Number", "Name", "Max")
	if admin {
		headers += th("Actions")
	}

	numberAttrs := ""
	numberVal := ""
	nameVal := ""
	maxVal := "16"
	pinRequired := ` required`
	pinPlaceholder := "PIN"
	hiddenID := ""
	if editing != nil {
		numberAttrs = ` readonly`
		numberVal = editing.Number
		nameVal = editing.Name
		maxVal = fmt.Sprintf("%d", editing.MaxParticipants)
		pinRequired = ""
		pinPlaceholder = "Leave blank to keep current PIN"
		hiddenID = hiddenField("id", fmt.Sprintf("%d", editing.ID))
	}

	content := pageHeader("Conferences", "Conference rooms in the 600–699 range.") +
		panel("Rooms", dataTable(headers, rows)) +
		adminOnly(admin, editPanel(editPanelTitle(editing != nil, "Add room", "Edit room"), formPost("/conferences/save",
			hiddenID+
				field("Number", fmt.Sprintf(`<input name="number" placeholder="600" required%s%s>`, valAttr(numberVal), numberAttrs))+
				field("Name", fmt.Sprintf(`<input name="name" placeholder="Room"%s>`, valAttr(nameVal)))+
				field("PIN", fmt.Sprintf(`<input name="pin" placeholder="%s"%s>`, html.EscapeString(pinPlaceholder), pinRequired))+
				field("Max participants", fmt.Sprintf(`<input name="max_participants" placeholder="16"%s>`, valAttr(maxVal)))+
				editFormActions(editing != nil, "/conferences",
					fmt.Sprintf(`<button type="submit">%s</button>`, html.EscapeString(editSubmitLabel(editing != nil)+" room"))),
		)))
	s.render(w, r, content)
}

func (s *Server) handleConferenceSave(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) || !s.requireAdmin(w, r) {
		return
	}
	max := 16
	fmt.Sscan(r.FormValue("max_participants"), &max)
	name := r.FormValue("name")
	number := r.FormValue("number")
	pin := r.FormValue("pin")
	id := formID(r)
	if id > 0 {
		_ = s.store.UpdateConference(id, name, number, pin, max)
	} else {
		if pin == "" {
			http.Error(w, "PIN required", 400)
			return
		}
		_ = s.store.CreateConference(name, number, pin, max)
	}
	redirectTo(w, r, "/conferences")
}

func (s *Server) handleConferenceDelete(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) || !s.requireAdmin(w, r) {
		return
	}
	var id int64
	fmt.Sscan(r.FormValue("id"), &id)
	_ = s.store.DeleteConference(id)
	redirectTo(w, r, "/conferences")
}

func (s *Server) handlePaging(w http.ResponseWriter, r *http.Request) {
	list, _ := s.store.ListPagingGroups()
	admin := s.isAdmin(r)
	editID := queryEditID(r)
	var editing *store.PagingGroup
	if editID > 0 {
		editing, _ = s.store.GetPagingGroup(editID)
	}
	editMembers := ""
	if editing != nil {
		if members, err := s.store.PagingMembers(editing.ID); err == nil {
			editMembers = strings.Join(members, ", ")
		}
	}

	var rows string
	for _, p := range list {
		members, _ := s.store.PagingMembers(p.ID)
		actions := ""
		if admin {
			actions = rowActions(
				editLink(fmt.Sprintf("/paging?edit=%d", p.ID)),
				deleteForm("/paging/delete", "id", fmt.Sprintf("%d", p.ID)),
			)
		}
		rows += fmt.Sprintf(`<tr><td>*%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
			html.EscapeString(p.Code), html.EscapeString(p.Name), html.EscapeString(p.Mode),
			html.EscapeString(p.MulticastAddress), html.EscapeString(strings.Join(members, ", ")), actions)
	}
	headers := th("Code", "Name", "Mode", "Multicast", "Members")
	if admin {
		headers += th("Actions")
	}

	codeAttrs := ""
	codeVal := ""
	nameVal := ""
	mode := "unicast"
	mcastVal := ""
	channelVal := "0"
	hiddenID := ""
	if editing != nil {
		codeAttrs = ` readonly`
		codeVal = editing.Code
		nameVal = editing.Name
		mode = editing.Mode
		mcastVal = editing.MulticastAddress
		channelVal = fmt.Sprintf("%d", editing.Channel)
		hiddenID = hiddenField("id", fmt.Sprintf("%d", editing.ID))
	}

	content := pageHeader("Paging", "Paging groups using codes *80–*99.") +
		panel("Groups", dataTable(headers, rows)) +
		adminOnly(admin, editPanel(editPanelTitle(editing != nil, "Add group", "Edit group"), formPost("/paging/save",
			hiddenID+
				field("Code", fmt.Sprintf(`<input name="code" placeholder="80" required%s%s>`, valAttr(codeVal), codeAttrs))+
				field("Name", fmt.Sprintf(`<input name="name" placeholder="All hands"%s>`, valAttr(nameVal)))+
				field("Mode", modeSelect(mode))+
				field("Multicast address", fmt.Sprintf(`<input name="multicast_address" placeholder="224.0.1.100:10000"%s>`, valAttr(mcastVal)))+
				field("Channel", fmt.Sprintf(`<input name="channel" placeholder="0"%s>`, valAttr(channelVal)))+
				field("Members", fmt.Sprintf(`<input name="members" placeholder="101,102"%s>`, valAttr(editMembers)))+
				editFormActions(editing != nil, "/paging",
					fmt.Sprintf(`<button type="submit">%s</button>`, html.EscapeString(editSubmitLabel(editing != nil)+" group"))),
		)))
	s.render(w, r, content)
}

func (s *Server) handlePagingSave(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) || !s.requireAdmin(w, r) {
		return
	}
	code := strings.TrimPrefix(r.FormValue("code"), "*")
	ch := 0
	fmt.Sscan(r.FormValue("channel"), &ch)
	name := r.FormValue("name")
	mode := r.FormValue("mode")
	mcast := r.FormValue("multicast_address")
	members := splitCSV(r.FormValue("members"))
	id := formID(r)
	if id > 0 {
		_ = s.store.UpdatePagingGroup(id, name, code, mode, mcast, ch)
		_ = s.store.SetPagingMembers(id, members)
	} else {
		_ = s.store.CreatePagingGroup(name, code, mode, mcast, ch)
		g, _ := s.store.GetPagingByCode(code)
		if g != nil {
			_ = s.store.SetPagingMembers(g.ID, members)
		}
	}
	redirectTo(w, r, "/paging")
}

func (s *Server) handlePagingDelete(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) || !s.requireAdmin(w, r) {
		return
	}
	var id int64
	fmt.Sscan(r.FormValue("id"), &id)
	_ = s.store.DeletePagingGroup(id)
	redirectTo(w, r, "/paging")
}

func (s *Server) handleTrunks(w http.ResponseWriter, r *http.Request) {
	routes, _ := s.store.ListTrunkRoutes()
	routeMap := map[int]store.TrunkRoute{}
	for _, rt := range routes {
		routeMap[rt.TrunkID] = rt
	}
	admin := s.isAdmin(r)

	var rows string
	for _, t := range s.cfg.EnabledTrunks() {
		rt := routeMap[t.ID]
		if rt.RouteType == "" {
			rt.RouteType = "all"
		}
		keepalive, _ := config.NormalizeTrunkKeepalive(t.Keepalive)
		rows += fmt.Sprintf(`<tr><td>%d</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%ds</td><td>%ds</td><td>%s</td><td>%s</td></tr>`,
			t.ID, html.EscapeString(t.Name), html.EscapeString(t.Prefix), html.EscapeString(t.Server),
			html.EscapeString(keepalive), t.KeepaliveSeconds, t.RegisterExpirySeconds,
			html.EscapeString(rt.RouteType), html.EscapeString(rt.RouteTarget))
		if admin {
			rows += fmt.Sprintf(`<tr class="subform-row"><td colspan="9"><form class="inline-form-row" method="post" action="/trunks/route">
<input type="hidden" name="trunk_id" value="%d">
<select name="route_type"><option value="all" %s>All extensions</option>
<option value="extension" %s>Extension</option><option value="group" %s>Hunt group</option></select>
<input name="route_target" value="%s" placeholder="101 or 500"><button type="submit" class="btn-secondary btn-sm">Save route</button></form></td></tr>`,
				t.ID, sel(rt.RouteType, "all"), sel(rt.RouteType, "extension"), sel(rt.RouteType, "group"), html.EscapeString(rt.RouteTarget))
		}
	}
	content := pageHeader("Trunks", "Connection details are in <code>config.toml</code>. Set inbound routing below.") +
		panel("Trunk routes", dataTable(th("ID", "Name", "Prefix", "Server", "Keepalive", "Ping(s)", "Reg(s)", "Route", "Target"), rows))
	s.render(w, r, content)
}

func sel(have, want string) string {
	if have == want {
		return "selected"
	}
	return ""
}

func (s *Server) handleTrunkRouteSave(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) || !s.requireAdmin(w, r) {
		return
	}
	trunkID := 0
	fmt.Sscan(r.FormValue("trunk_id"), &trunkID)
	_ = s.store.SaveTrunkRoute(store.TrunkRoute{
		TrunkID:     trunkID,
		RouteType:   r.FormValue("route_type"),
		RouteTarget: r.FormValue("route_target"),
	})
	redirectTo(w, r, "/trunks")
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		http.Error(w, "admin access required", http.StatusForbidden)
		return
	}
	users, _ := s.store.ListWebUsers()
	editID := queryEditID(r)
	var editing *store.WebUser
	if editID > 0 {
		editing, _ = s.store.GetWebUser(editID)
	}

	var rows string
	for _, u := range users {
		rows += fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td></tr>`,
			html.EscapeString(u.Username), html.EscapeString(NormalizeRole(u.Role)),
			rowActions(
				editLink(fmt.Sprintf("/users?edit=%d", u.ID)),
				deleteForm("/users/delete", "id", fmt.Sprintf("%d", u.ID)),
			))
	}

	usernameAttrs := ""
	usernameVal := ""
	role := RoleUser
	passRequired := ` required`
	passPlaceholder := "password"
	hiddenID := ""
	if editing != nil {
		usernameAttrs = ` readonly`
		usernameVal = editing.Username
		role = NormalizeRole(editing.Role)
		passRequired = ""
		passPlaceholder = "Leave blank to keep current password"
		hiddenID = hiddenField("id", fmt.Sprintf("%d", editing.ID))
	}

	content := pageHeader("Web users", "Users in <code>config.toml</code> are synced on startup.") +
		panel("Accounts", dataTable(th("User", "Role", "Actions"), rows)) +
		editPanel(editPanelTitle(editing != nil, "Add user", "Edit user"), formPost("/users/save",
			hiddenID+
				field("Username", fmt.Sprintf(`<input name="username" required%s%s>`, valAttr(usernameVal), usernameAttrs))+
				field("Password", fmt.Sprintf(`<input name="password" type="password" placeholder="%s"%s>`, html.EscapeString(passPlaceholder), passRequired))+
				roleSelect(role)+
				editFormActions(editing != nil, "/users",
					fmt.Sprintf(`<button type="submit">%s</button>`, html.EscapeString(editSubmitLabel(editing != nil)+" user"))),
		))
	s.render(w, r, content)
}

func (s *Server) handleUserSave(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) || !s.requireAdmin(w, r) {
		return
	}
	username := r.FormValue("username")
	role := NormalizeRole(r.FormValue("role"))
	password := r.FormValue("password")
	id := formID(r)
	if id > 0 {
		var hash string
		if password != "" {
			var err error
			hash, err = store.HashPassword(password)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		}
		_ = s.store.UpdateWebUser(id, username, role, hash)
	} else {
		if password == "" {
			http.Error(w, "password required", 400)
			return
		}
		hash, err := store.HashPassword(password)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		_ = s.store.UpsertWebUser(username, hash, role)
	}
	redirectTo(w, r, "/users")
}

func (s *Server) handleUserDelete(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) || !s.requireAdmin(w, r) {
		return
	}
	var id int64
	fmt.Sscan(r.FormValue("id"), &id)
	_ = s.store.DeleteWebUser(id)
	redirectTo(w, r, "/users")
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
