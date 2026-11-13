package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	autoshift "shift-coverage-bot"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	http.HandleFunc("/", Handler)
	http.HandleFunc("/api/preview", previewHandler)
	log.Printf("AutoShift admin website listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func previewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	date, err := time.ParseInLocation("2006-01-02", r.FormValue("date"), time.Local)
	if err != nil {
		http.Error(w, "date must use YYYY-MM-DD", http.StatusBadRequest)
		return
	}
	token, err := autoshift.GetMicrosoftToken()
	if err != nil {
		http.Error(w, "Microsoft authentication failed", http.StatusBadGateway)
		return
	}
	text, err := autoshift.BuildDailyScheduleTextForDay(token, date)
	if err != nil {
		log.Printf("generate schedule preview: %v", err)
		http.Error(w, "schedule preview failed", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"date": date.Format("2006-01-02"), "text": text})
}

// Handler serves the public, read-only AutoShift status page on Vercel.
// Posting and claim-management endpoints will require authentication before they
// are added.
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>AutoShift Admin</title><style>
:root{--ink:#152238;--muted:#65738a;--line:#e8edf4;--canvas:#f5f7fb;--panel:#fff;--navy:#193b6a;--blue:#2563eb;--blue-light:#eaf1ff;--green:#087443;--green-light:#eaf8f0;--amber:#a65c00;--amber-light:#fff6df}
*{box-sizing:border-box}body{margin:0;background:var(--canvas);color:var(--ink);font-family:Inter,ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;font-size:15px}
.shell{min-height:100vh;display:grid;grid-template-columns:236px 1fr}.sidebar{background:#122947;color:#e9f0fb;padding:26px 16px;display:flex;flex-direction:column}.brand{display:flex;align-items:center;gap:10px;padding:0 10px 30px;font-weight:750;font-size:20px;letter-spacing:-.4px}.brand-mark{width:28px;height:28px;border-radius:8px;background:#4f8cff;display:grid;place-items:center;color:white;font-size:15px}.workspace{font-size:11px;font-weight:750;text-transform:uppercase;letter-spacing:.08em;color:#8fa7c7;padding:0 10px 9px}.nav-link{display:flex;align-items:center;gap:11px;padding:11px 10px;border-radius:8px;margin-bottom:3px;color:#c9d7ea;text-decoration:none;font-weight:600}.nav-link.active{background:#244873;color:white}.nav-icon{width:19px;text-align:center}.server{margin-top:auto;border:1px solid #315274;border-radius:10px;padding:13px;color:#dbe8f8;background:#183655}.server b{display:block;font-size:13px;margin-bottom:4px}.server span{font-size:12px;color:#a8c7b8}
.content{max-width:1400px;width:100%;margin:0 auto;padding:31px 42px 48px}.topbar{display:flex;justify-content:space-between;align-items:center;margin-bottom:30px}.eyebrow{color:var(--muted);font-size:13px;font-weight:650;margin:0 0 4px}.topbar h1{font-size:28px;letter-spacing:-.7px;margin:0}.admin{display:flex;align-items:center;gap:10px;color:#42516a;font-weight:650}.avatar{width:34px;height:34px;border-radius:50%;background:#dbeafe;color:#1d4ed8;display:grid;place-items:center;font-size:13px;font-weight:800}.health{background:var(--green-light);color:var(--green);border:1px solid #c5e9d4;border-radius:99px;padding:6px 10px;font-size:12px;font-weight:750}
.overview{display:grid;grid-template-columns:1.1fr .9fr .9fr;gap:16px;margin-bottom:24px}.card,.panel{background:var(--panel);border:1px solid var(--line);border-radius:14px;box-shadow:0 1px 2px #15223808}.card{padding:20px}.label{font-size:12px;font-weight:750;color:var(--muted);text-transform:uppercase;letter-spacing:.055em}.metric{font-size:27px;font-weight:780;letter-spacing:-.6px;margin:11px 0 5px}.caption{font-size:13px;color:var(--muted)}.positive{color:var(--green)}
.grid{display:grid;grid-template-columns:minmax(0,1.35fr) minmax(300px,.8fr);gap:20px}.panel{padding:24px}.panel-heading{display:flex;justify-content:space-between;gap:18px;align-items:flex-start;margin-bottom:22px}.panel h2{font-size:18px;letter-spacing:-.25px;margin:0 0 5px}.panel p{margin:0;color:var(--muted);line-height:1.45}.primary{appearance:none;border:0;background:var(--blue);border-radius:8px;padding:10px 13px;color:#fff;font:inherit;font-size:13px;font-weight:750;white-space:nowrap;opacity:.55;cursor:not-allowed}.section-label{font-size:12px;font-weight:750;color:var(--muted);text-transform:uppercase;letter-spacing:.055em;margin:25px 0 9px}.form-row{display:grid;grid-template-columns:1fr 1fr;gap:12px}.field{border:1px solid var(--line);border-radius:9px;padding:12px;background:#fbfcfe}.field span{display:block;color:var(--muted);font-size:12px;margin-bottom:5px}.field strong{font-size:14px}.notice{display:flex;gap:11px;margin-top:19px;padding:13px;border-radius:9px;background:var(--blue-light);color:#24426b;font-size:13px;line-height:1.45}.notice b{color:#1d4ed8}.empty{border:1px dashed #c9d4e3;border-radius:10px;padding:26px 18px;text-align:center;color:var(--muted);font-size:13px}.schedule-item{display:flex;gap:12px;padding:13px 0;border-bottom:1px solid var(--line)}.schedule-item:last-child{border:0}.date-box{width:41px;flex:0 0 41px;border-radius:8px;background:var(--blue-light);color:#1d4ed8;text-align:center;padding:5px 2px;font-weight:800;font-size:12px}.date-box small{display:block;text-transform:uppercase;font-size:9px;margin-bottom:2px}.item-copy{flex:1}.item-copy b{font-size:14px}.item-copy span{display:block;font-size:12px;color:var(--muted);margin-top:3px}.tag{align-self:center;border-radius:99px;padding:5px 8px;background:var(--amber-light);color:var(--amber);font-size:11px;font-weight:750}.footer-note{margin-top:18px;color:var(--muted);font-size:12px}
@media(max-width:850px){.shell{display:block}.sidebar{display:none}.content{padding:24px 18px}.overview,.grid{grid-template-columns:1fr}.topbar{align-items:flex-start}.admin{display:none}}@media(max-width:500px){.overview{gap:10px}.form-row{grid-template-columns:1fr}.panel-heading{display:block}.primary{margin-top:14px;width:100%}}
</style></head><body><div class="shell"><aside class="sidebar"><div class="brand"><span class="brand-mark">↗</span> AutoShift</div><div class="workspace">Service Desk</div><a class="nav-link active" href="/"><span class="nav-icon">▦</span>Overview</a><a class="nav-link" href="#posting"><span class="nav-icon">◷</span>Schedule posting</a><a class="nav-link" href="#claims"><span class="nav-icon">✓</span>Shift claims</a><a class="nav-link" href="#settings"><span class="nav-icon">⚙</span>Settings</a><div class="server"><b>● Internal read-only view</b><span>Posting and claim changes are not enabled.</span></div></aside>
<main class="content"><header class="topbar"><div><p class="eyebrow">ITS Service Desk</p><h1>Schedule operations</h1></div><div class="admin"><span class="health">● System ready</span><span class="avatar">AD</span></div></header>
<section class="overview"><article class="card"><div class="label">Schedule status</div><div class="metric positive">Ready to configure</div><div class="caption">Teams and Slack connection settings are stored securely.</div></article><article class="card"><div class="label">Open coverage</div><div class="metric">—</div><div class="caption">Will appear when schedule data is connected.</div></article><article class="card"><div class="label">Claims today</div><div class="metric">—</div><div class="caption">Will be synchronized from Slack.</div></article></section>
<section class="grid"><article class="panel" id="posting"><div class="panel-heading"><div><h2>Post a daily schedule</h2><p>Generate the current Service Desk and Zoom schedule, then publish it to Slack.</p></div><button class="primary" disabled>Post schedule</button></div><div class="section-label">Schedule details</div><div class="form-row"><div class="field"><span>Schedule date</span><strong>Select a weekday</strong></div><div class="field"><span>Post to</span><strong>Service Desk Slack channel</strong></div></div><div class="notice"><b>ⓘ</b><div><b>Posting is not enabled yet.</b><br>This page is live and read-only. The next step connects this form to AutoShift’s existing read-only Teams schedule generator.</div></div></article>
<aside class="panel" id="claims"><div class="panel-heading"><div><h2>Scheduled posts</h2><p>Upcoming scheduled publishing jobs.</p></div></div><div class="empty">No scheduled posts yet.</div><div class="section-label">Recent activity</div><div class="schedule-item"><div class="date-box"><small>Status</small>✓</div><div class="item-copy"><b>Read-only dashboard deployed</b><span>Internal schedule visibility is active.</span></div><span class="tag">Setup</span></div><p class="footer-note" id="settings">Claims and scheduled posts will be stored in your Supabase database.</p></aside></section></main></div></body></html>`)
}
