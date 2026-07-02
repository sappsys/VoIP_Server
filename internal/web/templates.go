package web

const iconHeadHTML = `
<link rel="icon" type="image/svg+xml" href="/web/favicon.svg">
<link rel="shortcut icon" href="/web/favicon.svg">
<link rel="manifest" href="/web/manifest.webmanifest">
<link rel="apple-touch-icon" href="/web/apple-touch-icon.png">`

const layout = `<!DOCTYPE html>
<html lang="en"><head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<meta name="theme-color" content="#0f1419">
<meta name="apple-mobile-web-app-capable" content="yes">
<meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
<meta name="apple-mobile-web-app-title" content="VoIP PBX">
<title>VoIP PBX {{VERSION}}</title>` + iconHeadHTML + `
{{EXTRA_HEAD}}
<style>` + themeCSS + `</style>
</head><body>
<div class="app-shell">
<header class="topbar">
  <div class="topbar-left">
    <div class="topbar-brand">
      <img class="brand-icon" src="/web/logo.svg" width="32" height="32" alt="">
      <div>
        <h1>VoIP PBX</h1>
        <p class="sub">Control panel · {{VERSION}}</p>
      </div>
    </div>
  </div>
  <div class="topbar-right">
    <div class="topbar-nav">
      <a href="/">Dashboard</a>
      <a href="/status">Status</a>
      <a href="/extensions">Extensions</a>
      <a href="/hunt">Hunt</a>
      <a href="/conferences">Conferences</a>
      <a href="/paging">Paging</a>
      <a href="/phonebook">Phonebook</a>
      <a href="/trunks">Trunks</a>
      {{ADMIN_NAV}}
      <form class="topbar-logout" method="post" action="/logout"><button type="submit" class="btn-signout">Sign out</button></form>
    </div>
  </div>
</header>
<main class="page">
{{CONTENT}}
</main>
</div>
</body></html>`

const loginPage = `<!DOCTYPE html>
<html lang="en"><head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<meta name="theme-color" content="#0f1419">
<meta name="apple-mobile-web-app-capable" content="yes">
<meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
<meta name="apple-mobile-web-app-title" content="VoIP PBX">
<title>VoIP PBX — Sign in</title>` + iconHeadHTML + `
<style>` + loginCSS + `</style>
</head>
<body>
<div class="card">
  <div class="login-brand">
    <img src="/web/logo.svg" width="48" height="48" alt="">
    <div>
      <h1>VoIP PBX</h1>
      <p class="sub">Sign in to manage extensions, trunks, hunt groups, and live call status.</p>
    </div>
  </div>
  {{ERR}}
  <form method="post" action="/login">
    <label for="username">Username</label>
    <input id="username" name="username" type="text" autocomplete="username" required>
    <label for="password">Password</label>
    <input id="password" name="password" type="password" autocomplete="current-password" required>
    <button type="submit">Sign in</button>
  </form>
  <p class="hint">Default: admin / admin</p>
</div>
</body></html>`
