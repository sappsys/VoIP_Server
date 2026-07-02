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
	s.renderWithHead(w, r, s.statusPageHTML(filter), htmxScript)
}

func (s *Server) handleStatusFragment(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(s.statusLiveHTML(s.pbx.Status(), s.filteredCallLog(filter), filter)))
}

func (s *Server) statusPageHTML(filter string) string {
	report := s.pbx.Status()
	var b strings.Builder
	b.WriteString(pageHeader("Status",
		fmt.Sprintf("Version <strong>%s</strong> · %d registered · %d active calls",
			html.EscapeString(version.Version), report.RegisteredCount, report.ActiveCallCount)))
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

	var extRows strings.Builder
	for _, e := range report.Extensions {
		stateClass := "badge-offline"
		stateLabel := "offline"
		if e.Registered {
			stateClass = "badge-online"
			stateLabel = "online"
		}
		with := e.InCallWith
		if with == "" {
			with = "—"
		}
		extRows.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td><td>%v</td><td>%d</td><td>%s</td></tr>`,
			html.EscapeString(e.Extension), html.EscapeString(e.DisplayName),
			badge(stateClass, stateLabel), e.DND, e.ActiveCalls, html.EscapeString(with)))
	}
	b.WriteString(panel("Extensions", dataTable(th("Ext", "Name", "Online", "DND", "Calls", "In call with"), extRows.String())))

	var callBody strings.Builder
	if len(report.BridgedCalls) == 0 && len(report.Connecting) == 0 {
		callBody.WriteString(emptyState("No active calls."))
	} else {
		var rows strings.Builder
		for _, c := range report.BridgedCalls {
			state := "bridged"
			if c.Parked {
				state = "parked"
			} else if c.TransferReady {
				state = "transfer ready"
			}
			rows.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td></tr>`,
				html.EscapeString(c.CallerExt), html.EscapeString(c.CalleeExt), state))
		}
		for _, sess := range report.Connecting {
			rows.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>connecting</td></tr>`,
				html.EscapeString(sess.Caller), html.EscapeString(sess.Callee)))
		}
		callBody.WriteString(dataTable(th("Caller", "Callee", "State"), rows.String()))
	}
	b.WriteString(panel("Active calls", callBody.String()))

	if len(report.ParkedSlots) > 0 {
		var slots strings.Builder
		for i, slot := range report.ParkedSlots {
			if i > 0 {
				slots.WriteString(", ")
			}
			slots.WriteString(html.EscapeString(slot))
		}
		b.WriteString(panel("Parked", `<p>`+slots.String()+`</p>`))
	}

	var histBody strings.Builder
	if len(hist) == 0 {
		histBody.WriteString(emptyState("No extension history yet."))
	} else {
		var rows strings.Builder
		for _, h := range hist {
			rows.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td></tr>`,
				html.EscapeString(h.Extension), html.EscapeString(h.LastDialed), html.EscapeString(h.LastCaller)))
		}
		histBody.WriteString(dataTable(th("Extension", "Last dialed", "Last caller"), rows.String()))
	}
	b.WriteString(panel("Extension call history", histBody.String()))

	var logBody strings.Builder
	logBody.WriteString(`<div class="filter-bar">Filter: ` + statusFilterLinks(filter) + `</div>`)
	if len(log) == 0 {
		logBody.WriteString(emptyState("No matching call records."))
	} else {
		var rows strings.Builder
		for _, e := range log {
			trunk := e.TrunkName
			if trunk == "" && e.TrunkPrefix != "" {
				trunk = e.TrunkPrefix
			}
			if trunk == "" {
				trunk = "—"
			}
			rows.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
				html.EscapeString(e.StartedAt.Format("2006-01-02 15:04:05")),
				html.EscapeString(e.Direction),
				html.EscapeString(e.Caller), html.EscapeString(e.Callee),
				html.EscapeString(trunk)))
		}
		logBody.WriteString(dataTable(th("Time", "Direction", "Caller", "Callee", "Trunk"), rows.String()))
	}
	b.WriteString(panel("Call audit log", logBody.String()))
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
	return strings.Join(parts, " · ")
}
