package web

const layout = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>VoIP PBX {{VERSION}}</title>
<script src="https://unpkg.com/htmx.org@1.9.12"></script>
<style>body{font-family:system-ui,sans-serif;margin:2rem;max-width:960px}table{border-collapse:collapse;width:100%}td,th{border:1px solid #ccc;padding:.4rem}nav a{margin-right:1rem}.version{color:#666;font-size:.85rem;margin-top:2rem}.nav-version{color:#666;font-size:.85rem;margin-left:.5rem}.online{color:#0a0;font-weight:600}.offline{color:#999}.status-meta{color:#666;font-size:.9rem}</style>
</head><body>
<nav><a href="/">Dashboard</a><a href="/status">Status</a><a href="/extensions">Extensions</a><a href="/hunt">Hunt</a><a href="/conferences">Conferences</a><a href="/paging">Paging</a><a href="/phonebook">Phonebook</a><a href="/trunks">Trunks</a><a href="/users">Users</a><span class="nav-version">{{VERSION}}</span><form style="display:inline" method="post" action="/logout"><button>Logout</button></form></nav>
{{CONTENT}}
<p class="version">VoIP PBX {{VERSION}}</p>
</body></html>`

const loginPage = `<!DOCTYPE html><html><head><meta charset="utf-8"><title>Login</title>
<script src="https://unpkg.com/htmx.org@1.9.12"></script></head>
<body><h1>VoIP PBX</h1><p class="version">{{VERSION}}</p>{{ERR}}
<form method="post" action="/login"><label>User <input name="username"></label>
<label>Password <input type="password" name="password"></label><button>Login</button></form>
<p>Default: admin / admin</p></body></html>`
