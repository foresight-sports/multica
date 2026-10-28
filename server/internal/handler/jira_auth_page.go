package handler

import (
	"html/template"
	"net/http"
)

type jiraAuthPageData struct {
	Title, Description, Email, State, Nonce, StyleNonce, ReturnURL string
	Connected                                                      bool
}

// Chromium and Safari enforce form-action on the redirects after a POST too.
// Allow the fixed Atlassian OAuth destinations, never arbitrary external hosts.
const jiraAuthFormAction = "form-action 'self' https://mcp.atlassian.com https://auth.atlassian.com https://id.atlassian.com"

func (h *Handler) jiraAuthPage(w http.ResponseWriter, status int, data jiraAuthPageData) {
	nonce, err := jiraRandom()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not display sign-in")
		return
	}
	data.StyleNonce = nonce
	data.ReturnURL = jiraReturnPath(h.cfg.PublicURL, data.ReturnURL)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'nonce-"+nonce+"'; "+jiraAuthFormAction+"; base-uri 'none'; frame-ancestors 'none'")
	w.WriteHeader(status)
	_ = jiraAuthTemplate.Execute(w, data)
}

func (h *Handler) jiraAuthError(w http.ResponseWriter, status int, message string) {
	h.jiraAuthPage(w, status, jiraAuthPageData{Title: "Let’s reconnect", Description: message})
}

var jiraAuthTemplate = template.Must(template.New("jira-auth").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="dark"><title>{{.Title}} — Multica</title>
<style nonce="{{.StyleNonce}}">
*{box-sizing:border-box}body{margin:0;min-height:100vh;min-height:100svh;background:#101012;color:#f5f5f7;font:15px/1.6 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;display:flex;flex-direction:column}
a{color:inherit}header{padding:28px 40px} .brand{display:inline-flex;align-items:center;gap:10px;text-decoration:none;font:23px Georgia,serif}.star{font:32px/1 sans-serif}
main{flex:1;display:grid;place-items:center;padding:36px 24px 70px;background:radial-gradient(ellipse at 50% 30%,#19233355,transparent 65%)}
.card{width:100%;max-width:500px;padding:40px;background:#19191d;border:1px solid #303037;border-radius:20px;box-shadow:0 24px 90px #0003}
.connection{display:flex;align-items:center;gap:16px;margin-bottom:32px}.app{width:54px;height:54px;display:grid;place-items:center;border:1px solid #393940;border-radius:14px;background:#222227;font-size:34px}.jira{background:#132744;border-color:#24466f}.jira svg{width:28px;height:28px}.bridge{height:1px;width:36px;background:#44444d}.label{margin:0 0 10px;color:#8baeff;font-size:11px;font-weight:650;letter-spacing:.15em;text-transform:uppercase}
h1{font:normal 34px/1.18 Georgia,"Times New Roman",serif;letter-spacing:-.7px;margin:0 0 16px}.description{color:#b9b9c3;margin:0 0 26px}.account{padding:16px 18px;border:1px solid #35353e;border-radius:10px;background:#121215;margin-bottom:22px}.account span{display:block;color:#92929e;font-size:12px;margin-bottom:3px}.account strong{font-weight:550;overflow-wrap:anywhere}.note{font-size:13px;color:#a6a6b1;margin:0 0 26px}
button,.primary{display:flex;justify-content:center;align-items:center;gap:12px;min-height:48px;width:100%;padding:12px 18px;border:1px solid #759dff;border-radius:9px;background:#91afff;color:#101b34;font:600 14px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;text-decoration:none;cursor:pointer}button:hover,.primary:hover{background:#adc3ff}.secondary{display:block;width:fit-content;margin:18px auto 0;text-decoration:none;color:#aaaab5;font-size:13px}.secondary:hover{color:white}a:focus-visible,button:focus-visible{outline:3px solid #c6d5ff;outline-offset:5px}.footnote{margin:24px 0 0;padding-top:20px;border-top:1px solid #303037;color:#92929e;font-size:12px}.success{color:#8bdfb4}footer{text-align:center;color:#737380;font-size:12px;padding:0 24px 24px}
@media(max-width:540px){header{padding:24px}main{padding:16px 16px 40px}.card{padding:28px 24px;border-radius:16px}h1{font-size:30px}}
</style></head><body>
<header><a class="brand" href="{{.ReturnURL}}" aria-label="Multica home"><span class="star" aria-hidden="true">✳</span> Multica</a></header>
<main><section class="card" aria-labelledby="title">
<div class="connection" aria-hidden="true"><div class="app">✳</div><span class="bridge"></span><div class="app jira"><svg viewBox="0 0 24 24" fill="#69a4ff"><path d="M12 2 22 12 12 22 2 12Zm0 6-4 4 4 4 4-4Z" fill-rule="evenodd"/></svg></div></div>
<p class="label{{if .Connected}} success{{end}}">{{if .State}}Your personal connection{{else if .Connected}}Connection complete{{else}}Jira connection{{end}}</p>
<h1 id="title">{{.Title}}</h1><p class="description">{{.Description}}</p>
{{if .State}}
<div class="account"><span>Multica account</span><strong>{{.Email}}</strong></div>
<p class="note">Agents working on your requests will use your Jira account, with your existing permissions. Continue only if this is your Multica account.</p>
<form method="post"><input type="hidden" name="state" value="{{.State}}"><input type="hidden" name="csrf" value="{{.Nonce}}"><button type="submit">Continue to Atlassian <span aria-hidden="true">→</span></button></form>
<a class="secondary" href="{{.ReturnURL}}">Not now</a><p class="footnote">You’ll sign in with Atlassian to authorize access. You can disconnect anytime in your Multica account settings.</p>
{{else}}
{{if not .Connected}}<p class="note">Open Settings → Account → Jira connection in Multica to start a new sign-in.</p>{{end}}
<a class="primary" href="{{.ReturnURL}}">Return to Multica <span aria-hidden="true">→</span></a>
{{if .Connected}}<p class="footnote">You can close this window. If you started from the desktop app, return to it to continue.</p>{{end}}
{{end}}
</section></main><footer>Multica · Your AI teammates, connected.</footer></body></html>`))
