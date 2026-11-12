package handler

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"os"
)

// Handler serves the private AutoShift administrator landing page on Vercel.
// Schedule posting and claim management are added as separate authenticated API
// endpoints so this page never exposes operational controls by accident.
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !authorized(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="AutoShift Admin", charset="UTF-8"`)
		http.Error(w, "administrator sign-in required", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>AutoShift Admin</title><style>
body{font-family:system-ui,sans-serif;max-width:720px;margin:56px auto;padding:0 24px;color:#172033;background:#f7f9fc}
main{background:#fff;padding:32px;border-radius:14px;box-shadow:0 2px 14px #17203318}h1{margin-top:0}.status{color:#126b39;font-weight:600}
section{border-top:1px solid #dde3ec;margin-top:24px;padding-top:18px}button{padding:10px 14px;border:0;border-radius:7px;background:#dce3ec;color:#4c5665;font-weight:600}
</style></head><body><main><h1>AutoShift Admin</h1><p class="status">Dashboard is protected and online.</p>
<section><h2>Schedule posting</h2><p>The post-schedule form is being connected to Microsoft Teams and Slack.</p><button disabled>Post schedule</button></section>
<section><h2>Shift claims</h2><p>Claims will be stored in Supabase and shown here after the Slack HTTP integration is ready.</p></section>
</main></body></html>`)
}

func authorized(r *http.Request) bool {
	expectedPassword := os.Getenv("AUTOSHIFT_ADMIN_PASSWORD")
	if expectedPassword == "" {
		return false
	}

	username, password, ok := r.BasicAuth()
	return ok && username == "admin" &&
		subtle.ConstantTimeCompare([]byte(password), []byte(expectedPassword)) == 1
}
