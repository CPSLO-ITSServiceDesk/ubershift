package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	autoshift "shift-coverage-bot/autoshift"
)

func writeAPIError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func missingPreviewEnvironment() []string {
	required := []string{
		"MS_CLIENT_ID",
		"MS_TENANT_ID",
		"MS_CLIENT_SECRET",
		"MS_TEAM_ID",
		"MS_SERVICE_DESK_GROUP_ID",
	}

	var missing []string
	for _, name := range required {
		if os.Getenv(name) == "" {
			missing = append(missing, name)
		}
	}
	return missing
}

func previewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if missing := missingPreviewEnvironment(); len(missing) > 0 {
		message := "Preview is not configured. Add these Vercel environment variables: " + strings.Join(missing, ", ")
		log.Print(message)
		writeAPIError(w, http.StatusServiceUnavailable, message)
		return
	}

	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Could not load the Pacific time zone.")
		return
	}
	date, err := time.ParseInLocation("2006-01-02", r.FormValue("date"), pacific)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "Date must use YYYY-MM-DD.")
		return
	}
	token, err := autoshift.GetMicrosoftToken()
	if err != nil {
		log.Printf("authenticate Microsoft preview: %v", err)
		writeAPIError(w, http.StatusBadGateway, "Microsoft authentication failed. Check the Vercel environment variables and Microsoft app permissions.")
		return
	}
	text, err := autoshift.BuildDailyScheduleTextForDay(token, date)
	if err != nil {
		log.Printf("generate schedule preview: %v", err)
		writeAPIError(w, http.StatusBadGateway, "Could not generate the schedule. Check the Vercel Runtime Logs for the Microsoft Graph error.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"date": date.Format("2006-01-02"), "text": text})
}

func postHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := autoshift.PostDashboardSchedule(r.FormValue("text"), r.FormValue("channel")); err != nil {
		log.Printf("post dashboard schedule: %v", err)
		writeAPIError(w, http.StatusBadGateway, "Schedule post failed. Check the Slack environment variables and Runtime Logs.")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Schedule posted to Slack."})
}

// Handler serves the public, read-only AutoShift status page on Vercel.
// Posting and claim-management endpoints will require authentication before they
// are added.
func Handler(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Query().Get("action") {
	case "preview":
		previewHandler(w, r)
		return
	case "post":
		postHandler(w, r)
		return
	}
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
.grid{display:grid;grid-template-columns:minmax(0,1.35fr) minmax(300px,.8fr);gap:20px}.panel{padding:24px}.panel-heading{display:flex;justify-content:space-between;gap:18px;align-items:flex-start;margin-bottom:22px}.panel h2{font-size:18px;letter-spacing:-.25px;margin:0 0 5px}.panel p{margin:0;color:var(--muted);line-height:1.45}.primary{appearance:none;border:0;background:var(--blue);border-radius:8px;padding:10px 13px;color:#fff;font:inherit;font-size:13px;font-weight:750;white-space:nowrap;cursor:pointer}.primary:disabled{opacity:.55;cursor:not-allowed}.section-label{font-size:12px;font-weight:750;color:var(--muted);text-transform:uppercase;letter-spacing:.055em;margin:25px 0 9px}.form-row{display:grid;grid-template-columns:1fr 1fr;gap:12px}.field{border:1px solid var(--line);border-radius:9px;padding:12px;background:#fbfcfe}.field span{display:block;color:var(--muted);font-size:12px;margin-bottom:5px}.field input,.field select{width:100%;border:0;background:transparent;color:var(--ink);font:inherit;font-size:14px;font-weight:650;outline:0;padding:0}.field select{cursor:pointer}.notice{display:flex;gap:11px;margin-top:19px;padding:13px;border-radius:9px;background:var(--blue-light);color:#24426b;font-size:13px;line-height:1.45}.notice b{color:#1d4ed8}.preview{margin-top:20px;border-top:1px solid var(--line);padding-top:20px}.preview textarea{width:100%;min-height:330px;resize:vertical;border:1px solid var(--line);border-radius:9px;padding:13px;background:#fbfcfe;color:var(--ink);font:13px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace}.preview-actions{display:flex;justify-content:flex-end;margin-top:12px}.preview-status{margin:9px 0 0;color:var(--muted);font-size:12px}.preview-status.error{color:#b42318}.empty{border:1px dashed #c9d4e3;border-radius:10px;padding:26px 18px;text-align:center;color:var(--muted);font-size:13px}.schedule-item{display:flex;gap:12px;padding:13px 0;border-bottom:1px solid var(--line)}.schedule-item:last-child{border:0}.date-box{width:41px;flex:0 0 41px;border-radius:8px;background:var(--blue-light);color:#1d4ed8;text-align:center;padding:5px 2px;font-weight:800;font-size:12px}.date-box small{display:block;text-transform:uppercase;font-size:9px;margin-bottom:2px}.item-copy{flex:1}.item-copy b{font-size:14px}.item-copy span{display:block;font-size:12px;color:var(--muted);margin-top:3px}.tag{align-self:center;border-radius:99px;padding:5px 8px;background:var(--amber-light);color:var(--amber);font-size:11px;font-weight:750}.footer-note{margin-top:18px;color:var(--muted);font-size:12px}
@media(max-width:850px){.shell{display:block}.sidebar{display:none}.content{padding:24px 18px}.overview,.grid{grid-template-columns:1fr}.topbar{align-items:flex-start}.admin{display:none}}@media(max-width:500px){.overview{gap:10px}.form-row{grid-template-columns:1fr}.panel-heading{display:block}.primary{margin-top:14px;width:100%}}
</style></head><body><div class="shell"><aside class="sidebar"><div class="brand"><span class="brand-mark">↗</span> AutoShift</div><div class="workspace">Service Desk</div><a class="nav-link active" href="/"><span class="nav-icon">▦</span>Overview</a><a class="nav-link" href="#posting"><span class="nav-icon">◷</span>Schedule posting</a><a class="nav-link" href="#claims"><span class="nav-icon">✓</span>Shift claims</a><a class="nav-link" href="#settings"><span class="nav-icon">⚙</span>Settings</a><div class="server"><b>● Internal read-only view</b><span>Posting and claim changes are not enabled.</span></div></aside>
<main class="content"><header class="topbar"><div><p class="eyebrow">ITS Service Desk</p><h1>Schedule operations</h1></div><div class="admin"><span class="health">● System ready</span><span class="avatar">AD</span></div></header>
<section class="overview"><article class="card"><div class="label">Schedule status</div><div class="metric positive">Ready to configure</div><div class="caption">Teams and Slack connection settings are stored securely.</div></article><article class="card"><div class="label">Open coverage</div><div class="metric">—</div><div class="caption">Will appear when schedule data is connected.</div></article><article class="card"><div class="label">Claims today</div><div class="metric">—</div><div class="caption">Will be synchronized from Slack.</div></article></section>
<section class="grid"><article class="panel" id="posting"><div class="panel-heading"><div><h2>Post a daily schedule</h2><p>Choose the date and Slack destination, then generate an editable preview.</p></div><button class="primary" id="preview-button" type="button">Generate preview</button></div><div class="section-label">Schedule details</div><div class="form-row"><label class="field"><span>Schedule date</span><input id="schedule-date" name="date" type="date" required></label><label class="field"><span>Post to</span><select id="schedule-channel" name="channel"><option value="test" selected>Test channel (safe)</option><option value="work">Work Schedule channel</option></select></label></div><div class="notice"><b>ⓘ</b><div><b>Preview before posting.</b><br>Generating a preview reads Teams data. A separate confirmation is required to post your edited draft to Slack.</div></div><div class="preview" id="schedule-preview" hidden><div class="section-label">Review and edit schedule</div><textarea id="schedule-text" aria-label="Editable schedule preview"></textarea><div class="preview-actions"><button class="primary" id="confirm-button" type="button" disabled>Confirm post to Test channel</button></div><p class="preview-status" id="preview-status"></p></div></article>
<aside class="panel" id="claims"><div class="panel-heading"><div><h2>Scheduled posts</h2><p>Upcoming scheduled publishing jobs.</p></div></div><div class="empty">No scheduled posts yet.</div><div class="section-label">Recent activity</div><div class="schedule-item"><div class="date-box"><small>Status</small>✓</div><div class="item-copy"><b>Read-only dashboard deployed</b><span>Internal schedule visibility is active.</span></div><span class="tag">Setup</span></div><p class="footer-note" id="settings">Claims and scheduled posts will be stored in your Supabase database.</p></aside></section></main></div><script>const dateInput=document.getElementById('schedule-date');const channelInput=document.getElementById('schedule-channel');const previewButton=document.getElementById('preview-button');const confirmButton=document.getElementById('confirm-button');const preview=document.getElementById('schedule-preview');const scheduleText=document.getElementById('schedule-text');const previewStatus=document.getElementById('preview-status');const today=new Date();dateInput.value=[today.getFullYear(),String(today.getMonth()+1).padStart(2,'0'),String(today.getDate()).padStart(2,'0')].join('-');function destinationLabel(){return channelInput.options[channelInput.selectedIndex].text}function syncConfirmButton(){confirmButton.textContent='Confirm post to '+destinationLabel()}channelInput.addEventListener('change',syncConfirmButton);previewButton.addEventListener('click',async()=>{if(!dateInput.value){return}previewButton.disabled=true;confirmButton.disabled=true;previewButton.textContent='Generating…';previewStatus.className='preview-status';previewStatus.textContent='Reading the Teams schedule…';preview.hidden=false;try{const response=await fetch('/api?action=preview',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({date:dateInput.value})});const payload=await response.json().catch(()=>({}));if(!response.ok){throw new Error(payload.error||'Could not generate the schedule preview.')}scheduleText.value=payload.text;confirmButton.disabled=false;previewStatus.textContent='Preview generated for '+dateInput.value+' · '+destinationLabel()+'. You can edit the text before posting.'}catch(error){scheduleText.value='';previewStatus.className='preview-status error';previewStatus.textContent=error.message}finally{previewButton.disabled=false;previewButton.textContent='Generate preview'}});confirmButton.addEventListener('click',async()=>{if(!scheduleText.value.trim()){return}if(!window.confirm('Post this edited schedule to '+destinationLabel()+'?')){return}confirmButton.disabled=true;confirmButton.textContent='Posting…';previewStatus.className='preview-status';previewStatus.textContent='Posting to Slack…';try{const response=await fetch('/api?action=post',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({date:dateInput.value,channel:channelInput.value,text:scheduleText.value})});const payload=await response.json().catch(()=>({}));if(!response.ok){throw new Error(payload.error||'Could not post the schedule.')}previewStatus.textContent='Posted to '+destinationLabel()+'.'}catch(error){previewStatus.className='preview-status error';previewStatus.textContent=error.message}finally{confirmButton.disabled=false;syncConfirmButton()}});</script></body></html>`)
}
