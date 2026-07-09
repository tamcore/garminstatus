// Garmin Status — renders site/data/status.json into an uptime dashboard.
// Pure vanilla JS, no dependencies. The 180-day window is sliced relative to
// the viewer's "now" so the committed data file stays free of wall-clock churn.

"use strict";

const WINDOW_DAYS = 180;
// Day bars are colored by uptime band rather than "any blip = amber", so a
// single brief transient does not paint a whole day amber.
const GREEN_MIN = 0.999; // >= this fraction up -> operational
const AMBER_MIN = 0.95; // >= this -> partial; below -> down

const $ = (id) => document.getElementById(id);

function dayKey(d) {
  return d.toISOString().slice(0, 10); // YYYY-MM-DD (UTC)
}

// Build the ordered list of the last WINDOW_DAYS UTC dates, oldest first.
function windowDates() {
  const out = [];
  const today = new Date();
  const base = Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate());
  for (let i = WINDOW_DAYS - 1; i >= 0; i--) {
    out.push(new Date(base - i * 86400000));
  }
  return out;
}

function classifyDay(bucket) {
  if (!bucket) return "empty";
  if (bucket.upFrac >= GREEN_MIN) return "up";
  if (bucket.upFrac >= AMBER_MIN) return "partial";
  return "down";
}

function pct(frac) {
  return (frac * 100).toFixed(frac >= 0.9995 ? 0 : 2) + "%";
}

function relTime(iso) {
  const then = new Date(iso).getTime();
  if (!Number.isFinite(then)) return "unknown";
  const secs = Math.max(0, (Date.now() - then) / 1000);
  if (secs < 90) return "just now";
  const mins = Math.round(secs / 60);
  if (mins < 90) return `${mins} min ago`;
  const hrs = Math.round(mins / 60);
  if (hrs < 36) return `${hrs} h ago`;
  return `${Math.round(hrs / 24)} d ago`;
}

function fmtDate(iso) {
  const d = new Date(iso);
  if (isNaN(d)) return iso;
  return d.toISOString().slice(0, 16).replace("T", " ") + "Z";
}

// Tooltip -------------------------------------------------------------
const tip = $("tip");
function showTip(el, dateStr, cls, bucket) {
  let label;
  if (cls === "empty") label = "no data";
  else label = pct(bucket.upFrac) + " up";
  tip.innerHTML =
    `<span class="tip__date">${dateStr}</span> · ` +
    `<span class="tip__pct ${cls}">${label}</span>`;
  tip.hidden = false;
  const r = el.getBoundingClientRect();
  tip.style.left = r.left + r.width / 2 + "px";
  tip.style.top = r.top + "px";
}
function hideTip() { tip.hidden = true; }

// Rendering -----------------------------------------------------------
function renderService(svc, dates) {
  const byDate = new Map((svc.days || []).map((d) => [d.date, d]));

  const row = document.createElement("div");
  row.className = "svc";

  const head = document.createElement("div");
  head.className = "svc__head";

  const name = document.createElement("div");
  name.className = "svc__name " + (svc.current === "down" ? "is-down" : "is-up");
  name.innerHTML = `<span class="pip"></span>${escapeHtml(svc.name)}`;

  // Window uptime = mean of covered days' upFrac.
  let sum = 0, n = 0;
  for (const d of dates) {
    const b = byDate.get(dayKey(d));
    if (b) { sum += b.upFrac; n++; }
  }
  const uptime = document.createElement("div");
  uptime.className = "svc__uptime";
  uptime.innerHTML = n
    ? `<b>${pct(sum / n)}</b> uptime · ${n}d`
    : `no data yet`;

  head.append(name, uptime);

  const bars = document.createElement("div");
  bars.className = "bars";
  for (const d of dates) {
    const key = dayKey(d);
    const b = byDate.get(key);
    const cls = classifyDay(b);
    const bar = document.createElement("div");
    bar.className = "bar " + cls;
    bar.addEventListener("mouseenter", () => showTip(bar, key, cls, b));
    bar.addEventListener("mouseleave", hideTip);
    bars.appendChild(bar);
  }

  row.append(head, bars);
  return row;
}

function renderSection(title, services, dates) {
  if (!services || !services.length) return null;
  const sec = document.createElement("section");
  sec.className = "section";
  const h = document.createElement("h2");
  h.className = "section__title";
  h.textContent = title;
  const panel = document.createElement("div");
  panel.className = "panel";
  services
    .slice()
    .sort((a, b) => a.name.localeCompare(b.name))
    .forEach((s) => panel.appendChild(renderService(s, dates)));
  sec.append(h, panel);
  return sec;
}

function renderHero(data) {
  const all = [...(data.services.platforms || []), ...(data.services.features || [])];
  const down = all.filter((s) => s.current === "down");
  const hero = $("hero");
  const dot = $("brandDot");
  let state, headline, sub;
  if (!all.length) {
    state = "";
    headline = "No data yet";
    sub = "The collector has not recorded any status snapshots.";
  } else if (down.length === 0) {
    state = "is-up";
    headline = "All systems operational";
    sub = `${all.length} services monitored · Garmin Connect looks healthy.`;
  } else {
    state = "is-down";
    headline = `${down.length} service${down.length > 1 ? "s" : ""} down`;
    sub = "Affected: " + down.map((s) => s.name).join(", ");
  }
  if (state) hero.classList.add(state);
  $("heroHeadline").textContent = headline;
  $("heroSub").textContent = sub;
  if (state === "is-up") { dot.style.background = "var(--up)"; dot.style.boxShadow = "0 0 10px var(--up-glow)"; }
  else if (state === "is-down") { dot.style.background = "var(--down)"; dot.style.boxShadow = "0 0 10px var(--down-glow)"; }
}

function renderIncidents(data) {
  const list = $("incidentList");
  const incidents = (data.incidents || []).slice().reverse(); // most recent first
  if (!incidents.length) return;
  $("incidentsSection").hidden = false;
  for (const inc of incidents.slice(0, 40)) {
    const li = document.createElement("li");
    const ongoing = !inc.end;
    li.className = "incident" + (ongoing ? " ongoing" : "");
    const when = ongoing
      ? `${fmtDate(inc.start)} → ongoing`
      : `${fmtDate(inc.start)} → ${fmtDate(inc.end)}`;
    li.innerHTML =
      `<div class="incident__top">` +
        `<span class="incident__svc">${escapeHtml(inc.service)}</span>` +
        `<span class="incident__badge">${ongoing ? "ongoing" : "resolved"}</span>` +
      `</div>` +
      `<div class="incident__when">${when}</div>` +
      (inc.reasons && inc.reasons.length
        ? `<ul class="incident__reasons">${inc.reasons.map((r) => `<li>${escapeHtml(r)}</li>`).join("")}</ul>`
        : "");
    list.appendChild(li);
  }
}

function renderAxis(dates) {
  const first = dates[0], last = dates[dates.length - 1];
  const legend = document.createElement("div");
  legend.className = "legend";
  legend.innerHTML =
    `<span><i class="up"></i>operational</span>` +
    `<span><i class="partial"></i>partial</span>` +
    `<span><i class="down"></i>down</span>` +
    `<span><i class="empty"></i>no data</span>`;
  const axis = document.createElement("div");
  axis.className = "bars__axis";
  axis.innerHTML = `<span>${dayKey(first)}</span><span>${WINDOW_DAYS} days</span><span>${dayKey(last)}</span>`;
  const box = document.createElement("div");
  box.className = "section";
  box.append(legend, axis);
  return box;
}

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, (c) =>
    ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

async function main() {
  const dates = windowDates();
  let data;
  try {
    const res = await fetch("data/status.json", { cache: "no-cache" });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    data = await res.json();
  } catch (err) {
    $("heroHeadline").textContent = "Could not load status data";
    $("heroSub").innerHTML = `<span class="error">${escapeHtml(err.message)}</span>`;
    return;
  }
  data.services = data.services || { platforms: [], features: [] };

  renderHero(data);

  // "updated" reflects the last check (generated); fall back to dataThrough.
  const checked = data.generated || data.dataThrough;
  $("lastChecked").textContent = checked ? `updated ${relTime(checked)}` : "";

  const sections = $("sections");
  sections.appendChild(renderAxis(dates));
  const p = renderSection("Platforms", data.services.platforms, dates);
  const f = renderSection("Features", data.services.features, dates);
  if (p) sections.appendChild(p);
  if (f) sections.appendChild(f);

  renderIncidents(data);

  if (data.dataThrough) {
    $("footSpan").textContent = `Data through ${fmtDate(data.dataThrough)} · window ${dayKey(dates[0])} → ${dayKey(dates[dates.length - 1])}`;
  }
}

main();
