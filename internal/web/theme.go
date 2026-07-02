package web

// Shared dark theme for the VoIP PBX admin UI.
const themeCSS = `
:root {
  color-scheme: dark;
  --bg: #0f1419;
  --bg-elevated: #141c28;
  --card: #1a2332;
  --card-hover: #1f2a3d;
  --border: #243044;
  --border-light: #30363d;
  --text: #e7ecf1;
  --text-soft: #c5d0de;
  --muted: #8b9cb3;
  --accent: #5b9cff;
  --accent-dim: rgba(91, 156, 255, 0.15);
  --accent-glow: rgba(91, 156, 255, 0.35);
  --ok: #3dd68c;
  --warn: #ffb020;
  --error: #ff5c5c;
  --radius: 14px;
  --radius-sm: 10px;
  --shadow: 0 18px 48px rgba(0, 0, 0, 0.35);
  --page-width: 1180px;
  --space-1: .25rem;
  --space-2: .5rem;
  --space-3: .75rem;
  --space-4: 1rem;
  --space-5: 1.25rem;
  --space-6: 1.5rem;
  --space-8: 2rem;
}
* { box-sizing: border-box; }
body {
  margin: 0;
  min-height: 100vh;
  font-family: ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, sans-serif;
  color: var(--text);
  background-color: var(--bg);
  background-image:
    linear-gradient(180deg, rgba(15, 20, 25, 0.98) 0%, rgba(15, 20, 25, 0.94) 100%),
    radial-gradient(ellipse 90% 60% at 50% -15%, rgba(91, 156, 255, 0.1), transparent 55%);
  background-attachment: fixed;
  line-height: 1.5;
}
a { color: var(--accent); text-decoration: none; }
a:hover { text-decoration: underline; }
.app-shell {
  width: min(100%, var(--page-width));
  margin: 0 auto;
  padding: var(--space-6) var(--space-6) var(--space-8);
}
.topbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-4);
  margin-bottom: var(--space-6);
  padding: var(--space-4) var(--space-5);
  background: rgba(26, 35, 50, 0.72);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  box-shadow: var(--shadow);
  backdrop-filter: blur(12px);
}
.topbar-brand {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  min-width: 0;
}
.brand-icon {
  display: block;
  width: 2.25rem;
  height: 2.25rem;
  flex: 0 0 2.25rem;
}
.topbar-left h1 {
  margin: 0;
  font-size: 1.2rem;
  font-weight: 700;
  letter-spacing: -0.02em;
}
.topbar-left .sub {
  margin: .15rem 0 0;
  color: var(--muted);
  font-size: .82rem;
}
.topbar-right {
  display: flex;
  align-items: center;
  margin-left: auto;
}
.topbar-nav {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
}
.topbar-nav a {
  display: inline-flex;
  align-items: center;
  padding: .45rem .7rem;
  border-radius: 999px;
  color: var(--text-soft);
  font-size: .84rem;
  font-weight: 500;
  text-decoration: none;
  transition: background .15s ease, color .15s ease;
}
.topbar-nav a:hover {
  background: rgba(91, 156, 255, 0.12);
  color: var(--text);
  text-decoration: none;
}
.topbar-logout { margin: 0; padding: 0; display: inline; }
.btn-signout {
  appearance: none;
  border: 1px solid #3d4f66;
  background: #243044;
  color: var(--text);
  border-radius: 999px;
  padding: .45rem .85rem;
  font-size: .82rem;
  font-weight: 600;
  cursor: pointer;
  line-height: 1.2;
  transition: background .15s ease, border-color .15s ease;
}
.btn-signout:hover { background: #2f4058; border-color: var(--accent); }
.page {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}
.page-header {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.page-header h1,
.page > h1 {
  margin: 0;
  font-size: 1.65rem;
  font-weight: 700;
  letter-spacing: -0.03em;
}
.page-lead,
.page > p {
  margin: 0;
  color: var(--muted);
  font-size: .95rem;
  max-width: 70ch;
}
.panel {
  background: rgba(26, 35, 50, 0.88);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  box-shadow: var(--shadow);
  overflow: hidden;
}
.panel-head {
  padding: var(--space-4) var(--space-5);
  border-bottom: 1px solid var(--border);
  background: rgba(20, 28, 40, 0.55);
}
.panel-head h2,
.page h2,
section h2 {
  margin: 0;
  font-size: .95rem;
  font-weight: 600;
  letter-spacing: .01em;
  color: var(--accent);
  text-transform: uppercase;
}
.panel-body {
  padding: var(--space-5);
}
.panel-body > p:first-child { margin-top: 0; }
.panel-body > p:last-child { margin-bottom: 0; }
.table-wrap {
  width: 100%;
  overflow-x: auto;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: #121a26;
}
.data-table,
table {
  width: 100%;
  border-collapse: collapse;
  margin: 0;
}
.data-table th,
.data-table td,
th, td {
  padding: .75rem 1rem;
  text-align: left;
  border-bottom: 1px solid var(--border);
  font-size: .875rem;
  vertical-align: middle;
}
.data-table th,
th {
  color: var(--muted);
  font-weight: 600;
  font-size: .78rem;
  text-transform: uppercase;
  letter-spacing: .04em;
  background: rgba(15, 20, 29, 0.65);
}
.data-table tbody tr:hover,
table tbody tr:hover { background: rgba(91, 156, 255, 0.04); }
.data-table tr:last-child td,
table tr:last-child td { border-bottom: none; }
.subform-row td {
  background: rgba(15, 20, 29, 0.45);
  padding-top: var(--space-3);
  padding-bottom: var(--space-4);
}
.stat-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: var(--space-3);
}
.stat-card {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-4);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: rgba(13, 17, 23, 0.55);
}
.stat-label {
  color: var(--muted);
  font-size: .72rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: .05em;
}
.stat-value {
  font-size: 1.35rem;
  font-weight: 700;
  letter-spacing: -0.02em;
}
.form-stack {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--space-4);
  align-items: end;
}
.form-stack .field {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}
.form-stack .field label,
.form-stack > label {
  color: var(--muted);
  font-size: .8rem;
  font-weight: 500;
}
.form-stack .form-actions,
.form-stack .check-field {
  grid-column: 1 / -1;
}
.form-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: center;
  padding-top: var(--space-1);
}
.inline-form {
  display: inline-flex;
  margin: 0;
  padding: 0;
}
.inline-form-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: center;
}
.inline-form-row input,
.inline-form-row select {
  width: auto;
  min-width: 120px;
  flex: 1 1 140px;
}
.check-field {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  color: var(--text-soft);
  font-size: .875rem;
  cursor: pointer;
}
.check-field input { width: auto; margin: 0; }
input, select, textarea {
  width: 100%;
  background: #0d1117;
  border: 1px solid var(--border-light);
  border-radius: var(--radius-sm);
  color: var(--text);
  padding: .65rem .8rem;
  font-size: .9rem;
  transition: border-color .15s ease, box-shadow .15s ease;
}
input::placeholder { color: #6b7c93; }
input:focus, select:focus, textarea:focus {
  outline: none;
  border-color: var(--accent);
  box-shadow: 0 0 0 3px var(--accent-dim);
}
button, .btn-primary {
  appearance: none;
  border: none;
  background: var(--accent);
  color: #0d1117;
  border-radius: var(--radius-sm);
  padding: .65rem 1.1rem;
  font-size: .875rem;
  font-weight: 600;
  cursor: pointer;
  transition: filter .15s ease, transform .15s ease;
}
button:hover, .btn-primary:hover { filter: brightness(1.08); }
button:active { transform: translateY(1px); }
.btn-sm {
  padding: .45rem .75rem;
  font-size: .8rem;
}
.btn-danger {
  background: #3d1515;
  border: 1px solid #7a2e2e;
  color: #ffb4b4;
}
.btn-danger:hover { background: #542020; filter: none; }
.btn-secondary {
  background: #243044;
  border: 1px solid #3d4f66;
  color: var(--text);
}
.btn-secondary:hover { background: #2f4058; filter: none; }
code {
  background: #0d1117;
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: .15rem .4rem;
  font-size: .85em;
}
.badge {
  display: inline-flex;
  align-items: center;
  padding: .2rem .55rem;
  border-radius: 999px;
  font-size: .72rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: .04em;
}
.badge-online, .online { color: var(--ok); }
.badge-offline, .offline { color: var(--muted); }
.badge-online {
  background: rgba(61, 214, 140, 0.12);
  border: 1px solid rgba(61, 214, 140, 0.25);
}
.badge-offline {
  background: rgba(139, 156, 179, 0.1);
  border: 1px solid rgba(139, 156, 179, 0.2);
}
.filter-bar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: center;
  color: var(--muted);
  font-size: .875rem;
}
.filter-bar a { font-weight: 500; }
.filter-bar strong { color: var(--text); }
.empty-state {
  margin: 0;
  color: var(--muted);
  font-size: .9rem;
}
.error { color: var(--error); margin: 0 0 var(--space-4); font-size: .9rem; }
.status-meta {
  margin: 0 0 var(--space-4);
  color: var(--muted);
  font-size: .85rem;
}
#status-live {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}
#status-live .panel { margin: 0; }
#status-live .status-meta { margin: 0; }
@media (max-width: 900px) {
  .app-shell { padding: var(--space-4); }
  .topbar { padding: var(--space-4); }
  .topbar-right { width: 100%; margin-left: 0; }
}
@media (max-width: 640px) {
  .app-shell {
    padding: var(--space-3);
    padding-left: max(var(--space-3), env(safe-area-inset-left));
    padding-right: max(var(--space-3), env(safe-area-inset-right));
  }
  .page-header h1, .page > h1 { font-size: 1.35rem; }
  .panel-body { padding: var(--space-4); }
  .form-stack { grid-template-columns: 1fr; }
  input, select, textarea, button { font-size: 16px; }
}
`

const loginCSS = `
:root {
  color-scheme: dark;
  --bg: #0f1419;
  --text: #e6edf3;
  --muted: #8b9cb3;
  --accent: #5b9cff;
  --error: #ff5c5c;
  --radius: 14px;
}
* { box-sizing: border-box; }
body {
  margin: 0;
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1.5rem;
  font-family: ui-sans-serif, system-ui, sans-serif;
  color: var(--text);
  background: var(--bg);
  background-image:
    linear-gradient(180deg, rgba(8, 12, 18, 0.9) 0%, rgba(15, 20, 29, 0.88) 100%),
    radial-gradient(ellipse 100% 80% at 50% -10%, rgba(91, 156, 255, 0.14), transparent 55%);
  background-attachment: fixed;
}
.card {
  width: min(420px, 100%);
  background: rgba(26, 35, 50, 0.92);
  border: 1px solid rgba(91, 156, 255, 0.18);
  border-radius: var(--radius);
  padding: 2rem;
  box-shadow: 0 24px 64px rgba(0, 0, 0, 0.45), 0 0 0 1px rgba(255, 255, 255, 0.04) inset;
  backdrop-filter: blur(14px);
}
.login-brand {
  display: flex;
  align-items: center;
  gap: 1rem;
  margin-bottom: 1.5rem;
}
.login-brand h1 {
  margin: 0 0 .25rem;
  font-size: 1.4rem;
  font-weight: 700;
  letter-spacing: -0.03em;
}
.login-brand .sub {
  margin: 0;
  color: var(--muted);
  font-size: .88rem;
  line-height: 1.45;
}
.card label {
  display: block;
  font-size: .82rem;
  font-weight: 500;
  color: var(--muted);
  margin-bottom: .4rem;
}
.card input {
  width: 100%;
  padding: .7rem .8rem;
  margin-bottom: 1rem;
  border-radius: 10px;
  border: 1px solid #30363d;
  background: #0d1117;
  color: var(--text);
  font-size: 1rem;
}
.card input:focus {
  outline: none;
  border-color: var(--accent);
  box-shadow: 0 0 0 3px rgba(91, 156, 255, 0.2);
}
.card form > button[type="submit"] {
  width: 100%;
  margin-top: .25rem;
  padding: .75rem;
  border: none;
  border-radius: 10px;
  background: var(--accent);
  color: #0d1117;
  font-weight: 600;
  font-size: 1rem;
  cursor: pointer;
}
.card form > button[type="submit"]:hover { filter: brightness(1.08); }
.error { color: var(--error); margin: 0 0 1rem; font-size: .9rem; }
.hint { color: var(--muted); font-size: .82rem; margin: 1.25rem 0 0; text-align: center; }
`
