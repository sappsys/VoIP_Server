package web

import (
	"fmt"
	"html"
	"net/http"
	"strings"
)

const editFormAnchor = "edit-form"

func editLink(path string) string {
	return fmt.Sprintf(`<a class="btn-secondary btn-sm" href="%s#%s">Edit</a>`,
		html.EscapeString(path), editFormAnchor)
}

func rowActions(editLinkHTML, deleteHTML string) string {
	if editLinkHTML == "" && deleteHTML == "" {
		return ""
	}
	if editLinkHTML != "" && deleteHTML != "" {
		return editLinkHTML + " " + deleteHTML
	}
	return editLinkHTML + deleteHTML
}

func hiddenField(name, value string) string {
	return fmt.Sprintf(`<input type="hidden" name="%s" value="%s">`,
		html.EscapeString(name), html.EscapeString(value))
}

func valAttr(v string) string {
	if v == "" {
		return ""
	}
	return ` value="` + html.EscapeString(v) + `"`
}

func checkedAttr(on bool) string {
	if on {
		return " checked"
	}
	return ""
}

func selectedAttr(on bool) string {
	if on {
		return " selected"
	}
	return ""
}

func queryEditID(r *http.Request) int64 {
	var id int64
	fmt.Sscan(r.URL.Query().Get("edit"), &id)
	return id
}

func formID(r *http.Request) int64 {
	var id int64
	fmt.Sscan(r.FormValue("id"), &id)
	return id
}

func editPanelTitle(editing bool, addTitle, editTitle string) string {
	if editing {
		return editTitle
	}
	return addTitle
}

func editSubmitLabel(editing bool) string {
	if editing {
		return "Save changes"
	}
	return "Add"
}

func modeSelect(current string) string {
	if current == "" {
		current = "unicast"
	}
	return fmt.Sprintf(`<select name="mode"><option value="unicast"%s>unicast</option><option value="multicast"%s>multicast</option></select>`,
		selectedAttr(current == "unicast"), selectedAttr(current == "multicast"))
}

func strategySelect(current string) string {
	if current == "" {
		current = "simultaneous"
	}
	return fmt.Sprintf(`<select name="strategy"><option value="simultaneous"%s>simultaneous</option><option value="sequential"%s>sequential</option></select>`,
		selectedAttr(current == "simultaneous"), selectedAttr(current == "sequential"))
}

func editPanel(title, inner string) string {
	var b strings.Builder
	b.WriteString(`<section class="panel" id="`)
	b.WriteString(editFormAnchor)
	b.WriteString(`">`)
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
