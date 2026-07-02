package web

import (
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/sappsys/VoIP_Server/internal/pbx"
	"github.com/sappsys/VoIP_Server/internal/store"
	"github.com/sappsys/VoIP_Server/internal/version"
)

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")
	s.render(w, s.statusPageHTML(filter))
}

func (s *Server) handleStatusFragment(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(s.statusLiveHTML(s.pbx.Status(), s.filteredCallLog(filter), filter)))
}

func (s *Server) statusPageHTML(filter string) string {
	report := s.pbx.Status()
	var b strings.Builder
	b.WriteString(`<h1>Status</h1>`)
	b.WriteString(fmt.Sprintf(`<p>Version <strong>%s</strong> · %d registered · %d active calls</p>`,
		html.EscapeString(version.Version), report.RegisteredCount, report.ActiveCallCount))
	b.WriteString(`<div id="status-live" hx-get="/status/fragment"`)
	if filter != "" {
		b.WriteString(`?filter=` + html.EscapeString(filter))
	}
	b.WriteString(`" hx-trigger="every 5s" hx-swap="innerHTML">`)
	b.WriteString(s.statusLiveHTML(report, s.filteredCallLog(filter), filter))
	b.WriteString(`</div>`)
	return b.String()
}

func (s *Server) filteredCallLog(filter string) []store.CallLogEntry {
	log, _ := s.store.ListCallLog(100)
	if filter == "" {
		return log
	}
	var out []store.CallLogEntry
	for _, e := range log {
		if e.Direction == filter {
			out = append(out, e)
		}
	}
	return out
}

func (s *Server) statusLiveHTML(report pbx.StatusReport, log []store.CallLogEntry, filter string) string {
	hist, _ := s.store.ListExtensionHistory()

	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<p class="status-meta">Updated %s · %d registered · %d active</p>`,
		time.Now().Format("15:04:05"), report.RegisteredCount, report.ActiveCallCount))

	b.WriteString(`<h2>Extensions</h2><table><tr><th>Ext</th><th>Name</th><th>Online</th><th>DND</th><th>Calls</th><th>In call with</th></tr>`)
	for _, e := range report.Extensions {
		online := "offline"
		if e.Registered {
			online = "online"
		}
		with := e.InCallWith
		if with == "" {
			with = "—"
		}
		b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td class="%s">%s</td><td>%v</td><td>%d</td><td>%s</td></tr>`,
			html.EscapeString(e.Extension), html.EscapeString(e.DisplayName),
			online, online, e.DND, e.ActiveCalls, html.EscapeString(with)))
	}
	b.WriteString(`</table>`)

	b.WriteString(`<h2>Active calls</h2>`)
	if len(report.BridgedCalls) == 0 && len(report.Connecting) == 0 {
		b.WriteString(`<p>No active calls.</p>`)
	} else {
		b.WriteString(`<table><tr><th>Caller</th><th>Callee</th><th>State</th></tr>`)
		for _, c := range report.BridgedCalls {
			state := "bridged"
			if c.Parked {
				state = "parked"
			} else if c.TransferReady {
				state = "transfer ready"
			}
			b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td></tr>`,
				html.EscapeString(c.CallerExt), html.EscapeString(c.CalleeExt), state))
		}
		for _, sess := range report.Connecting {
			b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>connecting</td></tr>`,
				html.EscapeString(sess.Caller), html.EscapeString(sess.Callee)))
		}
		b.WriteString(`</table>`)
	}

	if len(report.ParkedSlots) > 0 {
		b.WriteString(`<h2>Parked</h2><p>`)
		for i, slot := range report.ParkedSlots {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(html.EscapeString(slot))
		}
		b.WriteString(`</p>`)
	}

	b.WriteString(`<h2>Extension call history</h2>`)
	if len(hist) == 0 {
		b.WriteString(`<p>No extension history yet.</p>`)
	} else {
		b.WriteString(`<table><tr><th>Extension</th><th>Last dialed</th><th>Last caller</th></tr>`)
		for _, h := range hist {
			b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td></tr>`,
				html.EscapeString(h.Extension), html.EscapeString(h.LastDialed), html.EscapeString(h.LastCaller)))
		}
		b.WriteString(`</table>`)
	}

	b.WriteString(`<h2>Call audit log</h2>`)
	b.WriteString(statusFilterLinks(filter))
	if len(log) == 0 {
		b.WriteString(`<p>No matching call records.</p>`)
	} else {
		b.WriteString(`<table><tr><th>Time</th><th>Direction</th><th>Caller</th><th>Callee</th><th>Trunk</th></tr>`)
		for _, e := range log {
			trunk := e.TrunkName
			if trunk == "" && e.TrunkPrefix != "" {
				trunk = e.TrunkPrefix
			}
			if trunk == "" {
				trunk = "—"
			}
			b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
				html.EscapeString(e.StartedAt.Format("2006-01-02 15:04:05")),
				html.EscapeString(e.Direction),
				html.EscapeString(e.Caller), html.EscapeString(e.Callee),
				html.EscapeString(trunk)))
		}
		b.WriteString(`</table>`)
	}
	return b.String()
}

func statusFilterLinks(active string) string {
	filters := []struct{ label, value string }{
		{"All", ""},
		{"Internal", "internal"},
		{"Inbound trunk", "inbound-trunk"},
		{"Outbound trunk", "outbound-trunk"},
	}
	var parts []string
	for _, f := range filters {
		label := f.label
		if f.value == active || (active == "" && f.value == "") {
			label = "<strong>" + label + "</strong>"
		} else if f.value == "" {
			label = fmt.Sprintf(`<a href="/status">%s</a>`, label)
		} else {
			label = fmt.Sprintf(`<a href="/status?filter=%s">%s</a>`, f.value, label)
		}
		parts = append(parts, label)
	}
	return `<p>Filter: ` + strings.Join(parts, " · ") + `</p>`
}
