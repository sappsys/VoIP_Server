package web

import (
	"fmt"
	"html"
	"net/http"
	"strings"
)

func pageHeader(title, lead string) string {
	var b strings.Builder
	b.WriteString(`<header class="page-header"><h1>`)
	b.WriteString(html.EscapeString(title))
	b.WriteString(`</h1>`)
	if lead != "" {
		b.WriteString(`<p class="page-lead">`)
		b.WriteString(lead)
		b.WriteString(`</p>`)
	}
	b.WriteString(`</header>`)
	return b.String()
}

func panel(title, inner string) string {
	var b strings.Builder
	b.WriteString(`<section class="panel">`)
	if title != "" {
		b.WriteString(`<div class="panel-head"><h2>`)
		b.WriteString(html.EscapeString(title))
		b.WriteString(`</h2></div>`)
	}
	b.WriteString(`<div class="panel-body">`)
	b.WriteString(inner)
	b.WriteString(`</div></section>`)
	return b.String()
}

func dataTable(headRow, bodyRows string) string {
	return `<div class="table-wrap"><table class="data-table"><thead><tr>` + headRow +
		`</tr></thead><tbody>` + bodyRows + `</tbody></table></div>`
}

func th(cells ...string) string {
	var b strings.Builder
	for _, c := range cells {
		b.WriteString(`<th>`)
		b.WriteString(c)
		b.WriteString(`</th>`)
	}
	return b.String()
}

func formPost(action, inner string) string {
	return fmt.Sprintf(`<form class="form-stack" method="post" action="%s">%s</form>`,
		html.EscapeString(action), inner)
}

func deleteForm(action, hiddenName, hiddenValue string) string {
	return fmt.Sprintf(
		`<form class="inline-form" method="post" action="%s"><input type="hidden" name="%s" value="%s"><button type="submit" class="btn-danger btn-sm">Delete</button></form>`,
		html.EscapeString(action), html.EscapeString(hiddenName), html.EscapeString(hiddenValue),
	)
}

func statGrid(cards string) string {
	return `<div class="stat-grid">` + cards + `</div>`
}

func statCard(label, value string) string {
	return fmt.Sprintf(`<div class="stat-card"><span class="stat-label">%s</span><span class="stat-value">%s</span></div>`,
		html.EscapeString(label), value)
}

func emptyState(msg string) string {
	return `<p class="empty-state">` + html.EscapeString(msg) + `</p>`
}

func field(label, input string) string {
	return `<div class="field"><label>` + html.EscapeString(label) + `</label>` + input + `</div>`
}

func checkField(label, input string) string {
	return `<label class="check-field">` + input + ` <span>` + html.EscapeString(label) + `</span></label>`
}

func formActions(inner string) string {
	return `<div class="form-actions">` + inner + `</div>`
}

func requirePOST(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

func redirectTo(w http.ResponseWriter, r *http.Request, path string) {
	http.Redirect(w, r, path, http.StatusSeeOther)
}

func badge(class, text string) string {
	return fmt.Sprintf(`<span class="badge %s">%s</span>`, class, html.EscapeString(text))
}

func roleSelect(selected string) string {
	selected = NormalizeRole(selected)
	return field("Role", `<select name="role" required>`+
		`<option value="admin"`+sel(selected, RoleAdmin)+`>Admin (full access)</option>`+
		`<option value="user"`+sel(selected, RoleUser)+`>User (view only)</option>`+
		`</select>`)
}

func adminOnly(admin bool, html string) string {
	if admin {
		return html
	}
	return ""
}
