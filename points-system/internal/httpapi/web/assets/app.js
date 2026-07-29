const state = { csrf: "", role: "user" };

function byId(id) { return document.getElementById(id); }
function money(micro) { return `$${(Number(micro || 0) / 1_000_000).toFixed(2)}`; }
function points(hundredths) {
  return (Number(hundredths || 0) / 100).toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  });
}
function dateTime(value) { return value ? new Date(value).toLocaleString() : "-"; }
function text(value) { return value == null || value === "" ? "-" : String(value); }
function idempotencyKey() { return crypto.randomUUID(); }

function notice(message, error = false) {
  const node = byId("notice");
  node.textContent = message;
  node.classList.toggle("error", error);
  node.classList.remove("hidden");
  window.setTimeout(() => node.classList.add("hidden"), 5000);
}

async function api(path, options = {}) {
  const headers = new Headers(options.headers || {});
  if (options.body) headers.set("Content-Type", "application/json");
  if (options.method && options.method !== "GET") headers.set("X-CSRF-Token", state.csrf);
  const response = await fetch(path, { ...options, headers, credentials: "same-origin" });
  const body = await response.json().catch(() => ({}));
  if (!response.ok || body.error) throw new Error(body.error?.message || `Request failed (${response.status})`);
  return body.data;
}

function setView(name) {
  document.querySelectorAll(".view").forEach((view) => view.classList.add("hidden"));
  document.querySelectorAll(".nav-button").forEach((button) => button.classList.toggle("active", button.dataset.view === name));
  byId(`view-${name}`).classList.remove("hidden");
  if (name === "activity") loadLedger();
  if (name === "admin") loadAdmin();
}

async function loadMe() {
  const data = await api("/api/v1/me");
  state.csrf = data.csrf_token;
  state.role = data.role;
  document.documentElement.dataset.theme = data.theme === "dark" ? "dark" : "light";
  document.documentElement.lang = data.language || "en";
  byId("total-points").textContent = points(data.account.total_points_hundredths);
  byId("total-checkin-rewards").textContent = money(data.account.settled_checkin_reward_microusd);
  byId("checkin-count").textContent = `${data.checkin.count} completed today`;
  const snapshot = data.yesterday_snapshot;
  const snapshotPoints = snapshot?.points_hundredths ?? snapshot?.awarded_points_hundredths ?? 0;
  byId("yesterday-points").textContent = points(snapshotPoints);
  byId("yesterday-spend").textContent = money(snapshot?.actual_cost_microusd);
  byId("today-rewards").textContent = money(data.checkin.awarded_microusd);
  byId("snapshot-date").textContent = snapshot ? new Date(snapshot.business_date).toLocaleDateString() : "-";
  byId("snapshot-status").textContent = snapshot?.status || "-";
  byId("snapshot-revision").textContent = snapshot?.revision || "-";
  byId("snapshot-points").textContent = points(snapshotPoints);
  document.querySelectorAll(".admin-only").forEach((node) => node.classList.toggle("hidden", data.role !== "admin"));
}

function renderRows(target, rows, columns) {
  const body = byId(target);
  body.replaceChildren();
  if (!rows.length) {
    const row = document.createElement("tr");
    const cell = document.createElement("td");
    cell.colSpan = columns.length;
    cell.textContent = "No records";
    row.append(cell);
    body.append(row);
    return;
  }
  rows.forEach((item) => {
    const row = document.createElement("tr");
    columns.forEach((column) => {
      const cell = document.createElement("td");
      const value = typeof column === "function" ? column(item, cell) : item[column];
      if (value !== undefined) cell.textContent = text(value);
      row.append(cell);
    });
    body.append(row);
  });
}

async function loadGrants() {
  const rows = await api("/api/v1/balance-grants?limit=50");
  renderRows("grants-body", rows, [
    (x) => dateTime(x.created_at), (x) => money(x.amount_microusd), "kind", "status", "attempts"
  ]);
}

async function loadLedger() {
  try {
    const rows = await api("/api/v1/ledger?limit=100");
    renderRows("ledger-body", rows, [
      (x) => dateTime(x.created_at), "kind", (x) => signedPoints(x.delta_points_hundredths),
      (x) => points(x.total_after_hundredths),
      (x) => x.business_date ? new Date(x.business_date).toLocaleDateString() : "-", "source"
    ]);
  } catch (error) { notice(error.message, true); }
}

function signedPoints(value) {
  const amount = Number(value || 0);
  return `${amount > 0 ? "+" : ""}${points(amount)}`;
}

async function loadAdmin() {
  if (state.role !== "admin") return;
  await Promise.all([loadPolicies(), loadAdminGrants()]);
}

function refreshTime(minute) {
  const value = Number(minute || 0);
  return `${String(Math.floor(value / 60)).padStart(2, "0")}:${String(value % 60).padStart(2, "0")}`;
}

async function loadPolicies() {
  const rows = await api("/api/v1/admin/policies?limit=50");
  renderRows("policies-body", rows, [
    "version_no",
    (x) => new Date(x.effective_date).toLocaleDateString(),
    (x) => x.enabled ? "On" : "Off",
    (x) => x.checkin_enabled ? `${x.checkin_daily_limit}/day` : "Off",
    (x) => x.mode === "consumer_only" ? "Consumers" : "All users",
    (x) => x.basis === "total" ? "Total" : "Yesterday",
    (x) => points(x.points_per_usd_hundredths),
    (x) => refreshTime(x.refresh_minute),
    (x) => money(x.minimum_checkin_spend_microusd),
    (x) => x.tiers?.length || 0
  ]);
}

async function loadAdminGrants() {
  const rows = await api("/api/v1/admin/balance-grants?limit=100");
  renderRows("admin-grants-body", rows, [
    (x) => dateTime(x.created_at), "user_id", (x) => money(x.amount_microusd), "status", "last_error",
    (item, cell) => {
      cell.className = "actions";
      if (["failed", "reversal_pending"].includes(item.status)) {
        const retry = document.createElement("button");
        retry.className = "secondary";
        retry.textContent = "Retry";
        retry.addEventListener("click", () => grantAction(item.id, "retry"));
        cell.append(retry);
      }
      const canReverse = item.status === "settled" || (item.status === "pending" && item.attempts === 0);
      if (canReverse) {
        const reverse = document.createElement("button");
        reverse.className = "danger";
        reverse.textContent = "Reverse";
        reverse.addEventListener("click", () => grantAction(item.id, "reverse"));
        cell.append(reverse);
      }
    }
  ]);
}

async function grantAction(id, action) {
  try {
    const options = { method: "POST" };
    if (action === "reverse") {
      const reason = window.prompt("Reversal reason");
      if (!reason) return;
      options.body = JSON.stringify({ reason });
    }
    await api(`/api/v1/admin/balance-grants/${encodeURIComponent(id)}/${action}`, options);
    notice(action === "retry" ? "Retry queued" : "Reversal queued");
    await loadAdminGrants();
  } catch (error) { notice(error.message, true); }
}

function tierField(labelText, name, type, value, step, mode) {
  const label = document.createElement("label");
  label.textContent = labelText;
  if (mode) label.dataset.rewardFields = mode;
  let input;
  if (type === "select") {
    input = document.createElement("select");
    [["fixed_range", "Fixed balance range"], ["percentage_range", "Spend percentage range"]].forEach(([optionValue, title]) => {
      const option = document.createElement("option");
      option.value = optionValue;
      option.textContent = title;
      input.append(option);
    });
  } else {
    input = document.createElement("input");
    input.type = type;
    input.min = "0";
    input.step = step;
    input.value = value;
    if (name === "upper") input.placeholder = "\u221e";
  }
  input.dataset.field = name;
  label.append(input);
  return label;
}

function syncTierMode(row) {
  const mode = row.querySelector('[data-field="mode"]').value;
  row.querySelectorAll("[data-reward-fields]").forEach((label) => {
    const active = label.dataset.rewardFields === mode;
    label.hidden = !active;
    label.querySelector("input").disabled = !active;
  });
}

function addTier() {
  const row = document.createElement("div");
  row.className = "tier-row";
  row.append(
    tierField("Lower points", "lower", "number", "0.00", "0.01"),
    tierField("Upper points", "upper", "number", "", "0.01"),
    tierField("Reward mode", "mode", "select"),
    tierField("Fixed min (U)", "fixedMin", "number", "0.01", "0.01", "fixed_range"),
    tierField("Fixed max (U)", "fixedMax", "number", "0.01", "0.01", "fixed_range"),
    tierField("Percentage min", "percentageMin", "number", "0.00", "0.01", "percentage_range"),
    tierField("Percentage max", "percentageMax", "number", "5.00", "0.01", "percentage_range")
  );
  const mode = row.querySelector('[data-field="mode"]');
  mode.addEventListener("change", () => syncTierMode(row));
  const remove = document.createElement("button");
  remove.type = "button";
  remove.className = "remove-tier danger";
  remove.title = "Remove tier";
  remove.setAttribute("aria-label", "Remove tier");
  remove.textContent = "X";
  remove.addEventListener("click", () => row.remove());
  row.append(remove);
  byId("tiers").append(row);
  syncTierMode(row);
}

function scaledInteger(raw, decimals, label) {
  const value = String(raw).trim();
  const match = value.match(new RegExp(`^(\\d+)(?:\\.(\\d{1,${decimals}}))?$`));
  if (!match) throw new Error(`${label} must have at most ${decimals} decimal places`);
  const fraction = (match[2] || "").padEnd(decimals, "0");
  const result = BigInt(match[1]) * (10n ** BigInt(decimals)) + BigInt(fraction || "0");
  if (result > BigInt(Number.MAX_SAFE_INTEGER)) throw new Error(`${label} is too large`);
  return Number(result);
}

function multiplySafe(value, multiplier, label) {
  if (value > Math.floor(Number.MAX_SAFE_INTEGER / multiplier)) throw new Error(`${label} is too large`);
  return value * multiplier;
}
function moneyInput(input, label) { return multiplySafe(scaledInteger(input.value, 2, label), 10_000, label); }
function pointsInput(input, label) { return scaledInteger(input.value, 2, label); }
function percentageInput(input, label) { return multiplySafe(scaledInteger(input.value, 2, label), 100, label); }
function nullableScaled(input, converter, label) { return input.value === "" ? null : converter(input, label); }

function tiersPayload() {
  return [...document.querySelectorAll(".tier-row")].map((row, index) => {
    const field = (name) => row.querySelector(`[data-field="${name}"]`);
    const mode = field("mode").value;
    const label = `Tier ${index + 1}`;
    return {
      lower_points_hundredths: pointsInput(field("lower"), `${label} lower points`),
      upper_points_hundredths: nullableScaled(field("upper"), pointsInput, `${label} upper points`),
      reward_mode: mode,
      fixed_reward_min_microusd: mode === "fixed_range" ? moneyInput(field("fixedMin"), `${label} fixed minimum`) : null,
      fixed_reward_max_microusd: mode === "fixed_range" ? moneyInput(field("fixedMax"), `${label} fixed maximum`) : null,
      reward_percentage_min_ppm: mode === "percentage_range" ? percentageInput(field("percentageMin"), `${label} percentage minimum`) : null,
      reward_percentage_max_ppm: mode === "percentage_range" ? percentageInput(field("percentageMax"), `${label} percentage maximum`) : null
    };
  });
}

function refreshMinuteInput() {
  const match = byId("policy-refresh-time").value.match(/^(\d{2}):(\d{2})$/);
  if (!match) throw new Error("Refresh time is required");
  const hour = Number(match[1]);
  const minute = Number(match[2]);
  if (hour > 23 || minute > 59) throw new Error("Refresh time is invalid");
  return hour * 60 + minute;
}

function integerInput(id, label) {
  const raw = byId(id).value;
  const value = Number(raw);
  if (!/^\d+$/.test(raw) || value < 1 || !Number.isSafeInteger(value)) throw new Error(`${label} must be a positive integer`);
  return value;
}

function syncConsumerOnly() {
  const consumerOnly = byId("policy-consumer-only").checked;
  const basis = byId("policy-basis");
  if (consumerOnly) basis.value = "yesterday";
  basis.disabled = consumerOnly;
}

function syncCheckinControls() {
  const systemEnabled = byId("policy-enabled").checked;
  const toggle = byId("policy-checkin-enabled");
  toggle.disabled = !systemEnabled;
  if (!systemEnabled) toggle.checked = false;
  const enabled = toggle.checked;
  ["policy-checkin-limit", "policy-single-cap", "policy-user-cap", "policy-platform-cap"].forEach((id) => {
    byId(id).required = enabled;
  });
}

function localDate(daysAhead) {
  const date = new Date();
  date.setHours(12, 0, 0, 0);
  date.setDate(date.getDate() + daysAhead);
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

document.querySelectorAll(".nav-button").forEach((button) => button.addEventListener("click", () => setView(button.dataset.view)));
byId("refresh-grants").addEventListener("click", () => loadGrants().catch((error) => notice(error.message, true)));
byId("refresh-ledger").addEventListener("click", loadLedger);
byId("refresh-admin-grants").addEventListener("click", () => loadAdminGrants().catch((error) => notice(error.message, true)));
byId("checkin").addEventListener("click", async (event) => {
  event.currentTarget.disabled = true;
  try {
    const result = await api("/api/v1/checkins", { method: "POST", headers: { "Idempotency-Key": idempotencyKey() } });
    notice(`Balance reward ${money(result.reward_microusd)} (${result.delivery_status})`);
    await Promise.all([loadMe(), loadGrants()]);
  } catch (error) { notice(error.message, true); } finally { event.currentTarget.disabled = false; }
});
byId("logout").addEventListener("click", async () => {
  try { await api("/api/v1/logout", { method: "POST" }); window.location.replace("/"); } catch (error) { notice(error.message, true); }
});
byId("grant-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    await api("/api/v1/admin/grants", { method: "POST", headers: { "Idempotency-Key": idempotencyKey() }, body: JSON.stringify({
      user_id: Number(byId("grant-user").value), amount: byId("grant-amount").value, reason: byId("grant-reason").value
    }) });
    notice("Grant applied");
    event.currentTarget.reset();
  } catch (error) { notice(error.message, true); }
});
byId("snapshot-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    const result = await api("/api/v1/admin/snapshots/refresh", { method: "POST", body: JSON.stringify({ business_date: byId("snapshot-business-date").value }) });
    notice(`Refreshed ${result.users} users`);
  } catch (error) { notice(error.message, true); }
});
byId("toggle-policy-form").addEventListener("click", () => byId("policy-form").classList.toggle("hidden"));
byId("add-tier").addEventListener("click", addTier);
byId("policy-consumer-only").addEventListener("change", syncConsumerOnly);
byId("policy-checkin-enabled").addEventListener("change", syncCheckinControls);
byId("policy-enabled").addEventListener("change", syncCheckinControls);
byId("policy-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    const checkinEnabled = byId("policy-checkin-enabled").checked;
    const tiers = tiersPayload();
    if (checkinEnabled && tiers.length === 0) throw new Error("At least one point tier is required when check-in is enabled");
    await api("/api/v1/admin/policies", { method: "POST", body: JSON.stringify({
      effective_date: byId("policy-date").value,
      enabled: byId("policy-enabled").checked,
      mode: byId("policy-consumer-only").checked ? "consumer_only" : "all_users",
      basis: byId("policy-basis").value,
      checkin_enabled: checkinEnabled,
      checkin_daily_limit: integerInput("policy-checkin-limit", "Daily check-in limit"),
      minimum_checkin_spend_microusd: moneyInput(byId("policy-minimum-spend"), "Minimum yesterday spend"),
      checkin_platform_daily_cap_microusd: moneyInput(byId("policy-platform-cap"), "Platform daily cap"),
      checkin_user_daily_cap_microusd: moneyInput(byId("policy-user-cap"), "User daily cap"),
      checkin_single_award_cap_microusd: moneyInput(byId("policy-single-cap"), "Single reward cap"),
      points_per_usd_hundredths: pointsInput(byId("policy-points-rate"), "Points per U"),
      refresh_minute: refreshMinuteInput(),
      tiers
    }) });
    notice("Policy created");
    byId("policy-form").classList.add("hidden");
    await loadPolicies();
  } catch (error) { notice(error.message, true); }
});

const tomorrow = localDate(1);
byId("policy-date").min = tomorrow;
byId("policy-date").value = tomorrow;
byId("snapshot-business-date").max = localDate(-1);
byId("snapshot-business-date").value = localDate(-1);
addTier();
syncConsumerOnly();
syncCheckinControls();
Promise.all([loadMe(), loadGrants()]).catch((error) => notice(error.message, true));
