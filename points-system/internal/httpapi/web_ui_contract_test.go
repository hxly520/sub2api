package httpapi

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestUserDashboardKeepsCoreMetricsAndDelaysEmbeddedReady(t *testing.T) {
	userHTML, err := webFS.ReadFile("web/user.html")
	if err != nil {
		t.Fatal(err)
	}
	userJS, err := webFS.ReadFile("web/assets/user.js")
	if err != nil {
		t.Fatal(err)
	}
	commonJS, err := webFS.ReadFile("web/assets/common.js")
	if err != nil {
		t.Fatal(err)
	}
	userContent := string(userHTML) + string(userJS)
	for _, required := range []string{
		`aria-busy="true"`, `id="total-points"`, `id="yesterday-points"`,
		`id="total-checkin-rewards"`, `id="today-rewards"`, `id="average-points"`,
		`id="dashboard-error"`, `id="retry-dashboard"`, `showDashboardError(error)`,
		`Math.round(totalPoints / chartState.days)`, `.finally(ui.notifyReady)`,
	} {
		if !strings.Contains(userContent, required) {
			t.Fatalf("user dashboard is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		`id="yesterday-spend"`, `id="period-spend"`, `totalPoints / rows.length`, "/api/v1/admin/",
	} {
		if strings.Contains(userContent, forbidden) {
			t.Fatalf("user dashboard exposes out-of-scope content %q", forbidden)
		}
	}
	commonContent := string(commonJS)
	for _, required := range []string{"function notifyReady()", "readySent", "window.parent.postMessage", "notifyReady,", `"needs_review"].includes(value)`,
		"sub2api:points-theme", "event.source !== window.parent", "!parentOrigins.includes(event.origin)",
		"function configuredParentOrigins()", `meta[name="sub2api-parent-origins"]`, "JSON.parse", "Array.isArray(values)",
		"parentOrigins.forEach((origin) => window.parent.postMessage(message, origin))",
		`let embeddedTheme = "";`, "embeddedTheme = event.data.theme;", "effective_date_must_be_tomorrow",
		"embedded && embeddedTheme ? embeddedTheme : sessionTheme"} {
		if !strings.Contains(commonContent, required) {
			t.Fatalf("shared embedded UI is missing delayed ready behavior %q", required)
		}
	}
	for _, forbidden := range []string{`parentOrigin || "*"`, `postMessage(message, "*")`, `meta[name="sub2api-parent-origin"]`} {
		if strings.Contains(commonContent, forbidden) {
			t.Fatalf("shared embedded UI restored an unsafe parent-origin contract %q", forbidden)
		}
	}
}

func TestPointsHTMLCarriesTheExactParentOriginListContract(t *testing.T) {
	for _, name := range []string{"web/user.html", "web/admin.html"} {
		markup, err := webFS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		content := string(markup)
		if !strings.Contains(content, `<meta name="sub2api-parent-origins" content="__SUB2API_EMBED_PARENT_ORIGINS__">`) {
			t.Fatalf("%s is missing the exact parent-origin list metadata", name)
		}
		if strings.Contains(content, "__SUB2API_EMBED_PARENT_ORIGIN__") || strings.Contains(content, `name="sub2api-parent-origin"`) {
			t.Fatalf("%s retained the obsolete single-origin HTML contract", name)
		}
	}
}

func TestAdministratorWaitsForInitialDataBeforeShowingWorkspace(t *testing.T) {
	adminHTML, err := webFS.ReadFile("web/admin.html")
	if err != nil {
		t.Fatal(err)
	}
	adminJS, err := webFS.ReadFile("web/assets/admin.js")
	if err != nil {
		t.Fatal(err)
	}
	content := string(adminJS)
	refresh := strings.Index(content, "await refreshAll();")
	show := strings.Index(content, "app.hidden = false;")
	if refresh < 0 || show < 0 || show < refresh {
		t.Fatal("administrator workspace is shown before its initial data has loaded")
	}
	for _, required := range []string{
		`id="admin-access-message"`, `id="retry-admin-bootstrap"`,
		`setAccessState(error.message, { error: true, retry: true });`,
		`ui.byId("retry-admin-bootstrap").addEventListener("click", bootstrap);`,
	} {
		if !strings.Contains(string(adminHTML)+content, required) {
			t.Fatalf("administrator initial error recovery is missing %q", required)
		}
	}
}

func TestUserLedgerDisplaysAwardedAt(t *testing.T) {
	userHTML, err := webFS.ReadFile("web/user.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(userHTML), `<th scope="col">发放时间</th>`) {
		t.Fatal("user ledger does not label its awarded_at column")
	}

	userJS, err := webFS.ReadFile("web/assets/user.js")
	if err != nil {
		t.Fatal(err)
	}
	content := string(userJS)
	start := strings.Index(content, "if (name === \"ledger\") {")
	end := strings.Index(content[start:], "return;")
	if end >= 0 {
		end += start
	}
	if start < 0 || end <= start {
		t.Fatal("user ledger renderer was not found")
	}
	ledgerRenderer := content[start:end]
	if !strings.Contains(ledgerRenderer, "ui.dateTime(item.awarded_at)") {
		t.Fatal("user ledger does not render awarded_at")
	}
	if strings.Contains(ledgerRenderer, "item.created_at") {
		t.Fatal("user ledger still renders created_at")
	}
}

func TestUserAsyncActionsRestoreButtonsAndConfirmCheckinState(t *testing.T) {
	userJS, err := webFS.ReadFile("web/assets/user.js")
	if err != nil {
		t.Fatal(err)
	}
	content := string(userJS)

	refreshStart := strings.Index(content, `ui.byId("refresh-dashboard").addEventListener`)
	refreshEnd := strings.Index(content, `ui.byId("refresh-ledger").addEventListener`)
	if refreshStart < 0 || refreshEnd <= refreshStart {
		t.Fatal("user dashboard refresh listener is missing")
	}
	refreshBlock := content[refreshStart:refreshEnd]
	captureIndex := strings.Index(refreshBlock, "const button = event.currentTarget;")
	awaitIndex := strings.Index(refreshBlock, "await refreshDashboard();")
	if captureIndex < 0 || awaitIndex < 0 || captureIndex > awaitIndex {
		t.Fatal("dashboard refresh must capture its button before awaiting")
	}
	if strings.Count(refreshBlock, "event.currentTarget") != 1 ||
		!strings.Contains(refreshBlock, `ui.setButtonBusy(button, false);`) {
		t.Fatal("dashboard refresh reads an expired event target or does not restore its button")
	}

	checkinStart := strings.Index(content, `ui.byId("checkin").addEventListener`)
	checkinEndOffset := -1
	if checkinStart >= 0 {
		checkinEndOffset = strings.Index(content[checkinStart:], `document.querySelectorAll(".period-button")`)
	}
	if checkinStart < 0 || checkinEndOffset <= 0 {
		t.Fatal("user check-in listener is missing")
	}
	checkinEnd := checkinStart + checkinEndOffset
	checkinBlock := content[checkinStart:checkinEnd]
	for _, required := range []string{
		"const button = event.currentTarget;",
		"beginPendingCheckin(profile || {});",
		`headers: { "Idempotency-Key": pendingCheckinKey }`,
		"checkinNeedsConfirmation = true;",
		"savePendingCheckin(profile || {});",
		"loadProfile({ confirmCheckin: true })",
		`ui.setButtonBusy(button, false);`,
		"syncCheckin(refreshedProfile || profile || {});",
	} {
		if !strings.Contains(checkinBlock, required) {
			t.Fatalf("user check-in recovery is missing %q", required)
		}
	}
	if strings.Count(checkinBlock, "event.currentTarget") != 1 {
		t.Fatal("user check-in reads event.currentTarget after its async boundary")
	}
	responseReceived := strings.Index(checkinBlock, "responseReceived = true;")
	confirmationRead := strings.Index(checkinBlock, "refreshedProfile = await loadProfile({ confirmCheckin: true });")
	if responseReceived < 0 || confirmationRead <= responseReceived {
		t.Fatal("successful check-in does not enter profile confirmation")
	}
	if strings.Contains(checkinBlock[responseReceived:confirmationRead], "clearPendingCheckin();") {
		t.Fatal("successful check-in clears its idempotency key before profile confirmation")
	}
	for _, required := range []string{
		`let checkinInFlight = false;`, `let checkinNeedsConfirmation = false;`,
		`let pendingCheckinKey = "";`, `let pendingCheckinBaseline = null;`,
		`const pendingCheckinStorageKey = "points.pending-checkin.v1";`,
		`sessionStorage.setItem(pendingCheckinStorageKey`,
		`sessionStorage.getItem(pendingCheckinStorageKey)`,
		`const countAdvanced = ui.number(data?.checkin?.count) >`,
		`if (!sameBusinessDate || countAdvanced)`,
		`if (checkinNeedsConfirmation)`, `button.disabled = !pendingCheckinKey;`,
		`pendingCheckinKey ? "确认签到结果" : "状态待确认"`,
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("user check-in state guard is missing %q", required)
		}
	}
}

func TestUserRecordsUseTenRowPaginationAndCompactCheckinCard(t *testing.T) {
	userHTML, err := webFS.ReadFile("web/user.html")
	if err != nil {
		t.Fatal(err)
	}
	userJS, err := webFS.ReadFile("web/assets/user.js")
	if err != nil {
		t.Fatal(err)
	}
	appCSS, err := webFS.ReadFile("web/assets/app.css")
	if err != nil {
		t.Fatal(err)
	}
	markup := string(userHTML)
	script := string(userJS)
	styles := string(appCSS)

	for _, required := range []string{
		`id="ledger-prev"`, `id="ledger-next"`, `id="ledger-page"`,
		`id="grants-prev"`, `id="grants-next"`, `id="grants-page"`,
		`const recordPageSize = 10;`, `createRecordPage("/api/v1/ledger")`,
		`createRecordPage("/api/v1/balance-grants")`, `next_cursor`,
		`cursor && error?.code === "invalid_cursor"`,
		`loadRecordPage(name, { cursor: "", navigation: "reset" })`,
		`class="table-pagination user-record-pagination"`,
	} {
		if !strings.Contains(markup+script, required) {
			t.Fatalf("user record pagination is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"签到赠送暂未开启", "今日积分数据仍可正常查看", `id="checkin-title"`, `id="checkin-detail"`,
	} {
		if strings.Contains(markup+script, forbidden) {
			t.Fatalf("compact check-in card still contains %q", forbidden)
		}
	}
	for _, required := range []string{
		"min-height: 120px;", ".user-record-pagination", ".pagination-button",
		"grid-template-columns: minmax(0, 1fr) auto;", "display: contents;", "position: static;",
		"--bg: #f9fafb;", "--primary: #0f766e;", "--bg: #020617;",
		"--surface: #1e293b;", "--text: #f1f5f9;", "--primary: #5eead4;",
		"--primary-action: #0f766e;", "background: var(--primary-action);",
		"tbody tr:hover td", "background: var(--primary-soft);",
		"background: var(--surface);", "color: var(--text);",
	} {
		if !strings.Contains(styles, required) {
			t.Fatalf("compact dashboard styles are missing %q", required)
		}
	}
	for _, forbidden := range []string{"background: #f6f8fc;", "background: #f2f6ff;"} {
		if strings.Contains(styles, forbidden) {
			t.Fatalf("theme-aware tables contain a fixed light color %q", forbidden)
		}
	}
}

func TestEmbeddedThemeRemainsAuthoritativeAndCoversInteractiveRecords(t *testing.T) {
	commonJS, err := webFS.ReadFile("web/assets/common.js")
	if err != nil {
		t.Fatal(err)
	}
	userJS, err := webFS.ReadFile("web/assets/user.js")
	if err != nil {
		t.Fatal(err)
	}
	appCSS, err := webFS.ReadFile("web/assets/app.css")
	if err != nil {
		t.Fatal(err)
	}

	common := string(commonJS)
	for _, required := range []string{
		`let embeddedTheme = "";`,
		`embeddedTheme = event.data.theme;`,
		`applyTheme(embedded && embeddedTheme ? embeddedTheme : sessionTheme);`,
		`new CustomEvent("points:themechange"`,
	} {
		if !strings.Contains(common, required) {
			t.Fatalf("embedded theme authority is missing %q", required)
		}
	}
	if !strings.Contains(string(userJS), `window.addEventListener("points:themechange", drawChart);`) {
		t.Fatal("user chart does not redraw when the embedded theme changes")
	}

	styles := string(appCSS)
	for _, required := range []string{
		`:root[data-theme="dark"]`,
		`.pagination-button`,
		`.pagination-button:hover:not(:disabled)`,
		`.status-chip`,
		`tbody tr:hover td`,
		`background: var(--primary-soft);`,
	} {
		if !strings.Contains(styles, required) {
			t.Fatalf("dark record theme coverage is missing %q", required)
		}
	}
}

func TestUserTrendSupportsKeyboardAndEquivalentData(t *testing.T) {
	userHTML, err := webFS.ReadFile("web/user.html")
	if err != nil {
		t.Fatal(err)
	}
	userJS, err := webFS.ReadFile("web/assets/user.js")
	if err != nil {
		t.Fatal(err)
	}
	content := string(userHTML) + string(userJS)
	for _, required := range []string{
		`id="points-chart" role="img" tabindex="0"`,
		`id="chart-data" class="sr-only"`,
		`id="chart-live" class="sr-only" aria-live="polite"`,
		`aria-pressed="true"`,
		`button.setAttribute("aria-pressed", String(active));`,
		`function moveChartPoint(event)`,
		`["ArrowLeft", "ArrowRight", "Home", "End"]`,
		`canvas.addEventListener("keydown", moveChartPoint);`,
		`const width = Math.max(1, Math.round(bounds.width));`,
		`Math.floor(plotWidth / 88) + 1`,
		`ui.renderRows("chart-data-body", rows`,
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("accessible trend is missing %q", required)
		}
	}
}

func TestWebControlsUseLicensedIconSpriteAndPreserveLabels(t *testing.T) {
	userHTML, err := webFS.ReadFile("web/user.html")
	if err != nil {
		t.Fatal(err)
	}
	adminHTML, err := webFS.ReadFile("web/admin.html")
	if err != nil {
		t.Fatal(err)
	}
	commonJS, err := webFS.ReadFile("web/assets/common.js")
	if err != nil {
		t.Fatal(err)
	}
	sprite, err := webFS.ReadFile("web/assets/lucide-sprite.svg")
	if err != nil {
		t.Fatal(err)
	}
	content := string(userHTML) + string(adminHTML) + string(commonJS)
	for _, required := range []string{
		`/assets/lucide-sprite.svg#refresh-cw`,
		`/assets/lucide-sprite.svg#coins`,
		`/assets/lucide-sprite.svg#settings-2`,
		`data-button-label`,
		`function setButtonLabel(button, text)`,
		`button.classList.add("is-busy")`,
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("icon control contract is missing %q", required)
		}
	}
	if !strings.Contains(string(sprite), "@license lucide-static v1.28.0 - ISC") {
		t.Fatal("Lucide sprite does not retain its license attribution")
	}
	for _, path := range []string{"../../THIRD_PARTY_NOTICES.md", "../../Dockerfile"} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("licensed icon distribution is missing %s: %v", path, err)
		}
	}
	notices, err := os.ReadFile("../../THIRD_PARTY_NOTICES.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"lucide-static", "ISC License", "Lucide Icons and Contributors"} {
		if !strings.Contains(string(notices), required) {
			t.Fatalf("third-party notices are missing %q", required)
		}
	}
	dockerfile, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dockerfile), "COPY THIRD_PARTY_NOTICES.md /usr/share/licenses/sub2api-points/THIRD_PARTY_NOTICES.md") {
		t.Fatal("points image does not retain the icon license notice")
	}
	for _, required := range []string{
		`aria-label="上一页"`, `aria-label="下一页"`,
		`text.dataset.buttonLabel = "";`, `button.append(ui.icon(iconName), text);`,
	} {
		if !strings.Contains(content+string(adminJSForContract(t)), required) {
			t.Fatalf("icon button is missing its accessible label contract %q", required)
		}
	}
}

func TestWebIconReferencesExistInLicensedSprite(t *testing.T) {
	userHTML, err := webFS.ReadFile("web/user.html")
	if err != nil {
		t.Fatal(err)
	}
	adminHTML, err := webFS.ReadFile("web/admin.html")
	if err != nil {
		t.Fatal(err)
	}
	sprite, err := webFS.ReadFile("web/assets/lucide-sprite.svg")
	if err != nil {
		t.Fatal(err)
	}

	symbolPattern := regexp.MustCompile(`<symbol id="([^"]+)"`)
	referencePattern := regexp.MustCompile(`/assets/lucide-sprite\.svg#([^"]+)`)
	symbols := make(map[string]struct{})
	for _, match := range symbolPattern.FindAllStringSubmatch(string(sprite), -1) {
		symbols[match[1]] = struct{}{}
	}
	for _, match := range referencePattern.FindAllStringSubmatch(string(userHTML)+string(adminHTML), -1) {
		if _, ok := symbols[match[1]]; !ok {
			t.Fatalf("web page references missing Lucide symbol %q", match[1])
		}
	}
}

func TestAdminDashboardUsesCompleteTabsAndLocksInactiveCheckinSettings(t *testing.T) {
	adminHTML, err := webFS.ReadFile("web/admin.html")
	if err != nil {
		t.Fatal(err)
	}
	adminJS, err := webFS.ReadFile("web/assets/admin.js")
	if err != nil {
		t.Fatal(err)
	}
	content := string(adminHTML) + string(adminJS)
	for _, required := range []string{
		`role="tablist"`, `role="tabpanel"`, `aria-controls="view-users"`,
		`aria-labelledby="tab-users"`, "moveAdminTab", `"ArrowLeft"`, `"ArrowRight"`,
		`id="checkin-settings"`, `id="checkin-tiers"`, `classList.toggle("is-locked"`,
		`<fieldset class="form-block">`, `<fieldset id="checkin-settings" class="form-block">`,
		`<fieldset id="checkin-tiers" class="form-block">`, `<legend class="sr-only">`,
		`id="checkin-lock-reason"`, `id="basis-lock-reason"`,
		`control.disabled = !checkinEnabled`, `setControlDescription(control`,
		`tiers.setAttribute("aria-disabled", "true")`, `finally {`, "ui.notifyReady()",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("administrator dashboard is missing %q", required)
		}
	}
	for _, forbidden := range []string{"手工赠送", "grant-form", "snapshot-form", "/api/v1/admin/snapshots/refresh"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("administrator dashboard restored removed action %q", forbidden)
		}
	}
	markup := string(adminHTML)
	if fieldsets, legends := strings.Count(markup, "<fieldset"), strings.Count(markup, "<legend"); fieldsets != 3 || legends != fieldsets {
		t.Fatalf("policy editor fieldset contract is incomplete: fieldsets=%d legends=%d", fieldsets, legends)
	}
}

func TestAdminPolicyEditorUsesSingleNextDayAppendFlow(t *testing.T) {
	adminHTML, err := webFS.ReadFile("web/admin.html")
	if err != nil {
		t.Fatal(err)
	}
	adminJS := adminJSForContract(t)
	content := string(adminHTML) + string(adminJS)
	for _, required := range []string{
		`<form id="policy-form" class="policy-editor">`,
		`id="policy-date" type="hidden"`,
		`id="policy-editor-source"`,
		`id="policy-editor-effective"`,
		`所有策略变更统一于次日生效`,
		`保存后仅影响下一自然日；已经结算的消费积分不会被回溯修改。`,
		`function editorPolicy()`,
		`renderPolicyEditor(editorPolicy());`,
		`const effectiveDate = setPolicyEffectiveDate();`,
		`effective_date: effectiveDate,`,
		`ui.notice(` + "`" + `策略已保存，将于 ${effectiveDate} 生效` + "`" + `);`,
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("single-policy editor is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		`id="toggle-policy-form"`, `id="cancel-policy"`, `<span data-button-label>新建策略</span>`, `创建策略版本`,
		`class="policy-editor hidden"`, `id="policy-date" type="date"`,
		`历史版本审计`, `id="policies-body"`, `保存仅追加审计版本`,
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("single-policy editor restored ambiguous action %q", forbidden)
		}
	}
}

func TestAdminPolicyEditorSupportsPrivateSpendTiersAndUnlimitedCaps(t *testing.T) {
	adminHTML, err := webFS.ReadFile("web/admin.html")
	if err != nil {
		t.Fatal(err)
	}
	adminJS := adminJSForContract(t)
	userHTML, err := webFS.ReadFile("web/user.html")
	if err != nil {
		t.Fatal(err)
	}
	userJS, err := webFS.ReadFile("web/assets/user.js")
	if err != nil {
		t.Fatal(err)
	}

	admin := string(adminHTML) + string(adminJS)
	for _, required := range []string{
		`id="policy-tier-basis"`, `<option value="points">消费积分</option>`,
		`<option value="spend">昨日消费金额</option>`, `阶梯条件（仅管理员可见）`,
		`阶梯条件、比例和金额区间仅管理员可见，不会展示在用户积分中心。`,
		`id="policy-checkin-limit" type="number" min="1" step="1" value="1" required`,
		`id="policy-single-cap" type="number" min="0.01" step="0.01" placeholder="不限"`,
		`id="policy-user-cap" type="number" min="0.01" step="0.01" placeholder="不限"`,
		`id="policy-platform-cap" type="number" min="0.01" step="0.01" placeholder="不限"`,
		`policy.checkin_tier_basis === "spend" ? "spend" : "points"`,
		`checkin_tier_basis: tierBasis`, `lower_spend_microusd: spendBased ? lower : null`,
		`upper_spend_microusd: spendBased ? upper : null`,
		`lower_points_hundredths: spendBased ? null : lower`,
		`upper_points_hundredths: spendBased ? null : upper`,
		`return value == null ? "" : decimalValue(value, divisor, "");`,
		`checkin_single_award_cap_microusd: nullableScaled`,
		`checkin_user_daily_cap_microusd: nullableScaled`,
		`checkin_platform_daily_cap_microusd: nullableScaled`,
		`control.required = checkinEnabled && id === "policy-checkin-limit";`,
		`basis.disabled = !checkinEnabled || consumerOnly || spendBased;`,
		`function changeTierBasis()`,
		`input.value = "";`,
		`阶梯条件已切换，请重新填写每一档的起止范围。`,
	} {
		if !strings.Contains(admin, required) {
			t.Fatalf("administrator spend-tier policy editor is missing %q", required)
		}
	}

	user := string(userHTML) + string(userJS)
	for _, forbidden := range []string{
		"checkin_tier_basis", "lower_spend_microusd", "upper_spend_microusd",
		"lower_points_hundredths", "upper_points_hundredths", "赠送阶梯", "比例最低（%）", "比例最高（%）",
	} {
		if strings.Contains(user, forbidden) {
			t.Fatalf("user dashboard exposes private check-in tier information %q", forbidden)
		}
	}
}

func adminJSForContract(t *testing.T) []byte {
	t.Helper()
	content, err := webFS.ReadFile("web/assets/admin.js")
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func TestAdminDialogAndAsyncListsKeepInteractionContracts(t *testing.T) {
	adminHTML, err := webFS.ReadFile("web/admin.html")
	if err != nil {
		t.Fatal(err)
	}
	adminJS := adminJSForContract(t)
	userJS, err := webFS.ReadFile("web/assets/user.js")
	if err != nil {
		t.Fatal(err)
	}
	admin := string(adminHTML) + string(adminJS)
	for _, required := range []string{
		`aria-labelledby="reverse-dialog-title"`,
		`aria-describedby="reverse-dialog-description"`,
		`state.reverseReturnFocus = trigger || document.activeElement;`,
		`requestAnimationFrame(() => ui.byId("reverse-reason").focus());`,
		`ui.byId("reverse-dialog").addEventListener("close", finishReverseDialog);`,
		`if (target?.isConnected) requestAnimationFrame(() => target.focus());`,
	} {
		if !strings.Contains(admin, required) {
			t.Fatalf("administrator dialog is missing %q", required)
		}
	}

	for _, test := range []struct {
		name     string
		content  string
		required []string
	}{
		{
			name:    "user",
			content: string(userJS),
			required: []string{
				`requestSequence: 0`,
				`const requestSequence = ++page.requestSequence;`,
				`if (requestSequence !== page.requestSequence) return;`,
				`const requestSequence = ++chartState.requestSequence;`,
				`if (requestSequence !== chartState.requestSequence) return;`,
			},
		},
		{
			name:    "administrator",
			content: string(adminJS),
			required: []string{
				`requestSequence: { policies: 0, users: 0, grants: 0 }`,
				`const requestSequence = ++state.requestSequence.policies;`,
				`const requestSequence = ++state.requestSequence.users;`,
				`const requestSequence = ++state.requestSequence.grants;`,
				`if (requestSequence !== state.requestSequence.users) return;`,
				`if (requestSequence !== state.requestSequence.grants) return;`,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, required := range test.required {
				if !strings.Contains(test.content, required) {
					t.Fatalf("request ordering guard is missing %q", required)
				}
			}
		})
	}
}

func TestAdminUserPaginationCommitsOnlySuccessfulPage(t *testing.T) {
	adminJS := string(adminJSForContract(t))
	for _, required := range []string{
		`usersPage: { limit: 50, offset: 0, total: 0, loading: false }`,
		`function syncAdminUsersPager()`,
		`page.loading = true;`,
		`loadAdminUsers({ offset })`,
		`page.offset = Math.max(0, ui.number(data?.offset ?? requestedOffset));`,
		`if (requestSequence === state.requestSequence.users) {`,
		`page.loading = false;`,
	} {
		if !strings.Contains(adminJS, required) {
			t.Fatalf("administrator user pagination is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		`state.usersPage.offset += state.usersPage.limit;`,
		`state.usersPage.offset = Math.max(0, state.usersPage.offset - state.usersPage.limit);`,
	} {
		if strings.Contains(adminJS, forbidden) {
			t.Fatalf("administrator user pagination mutates page state before success: %q", forbidden)
		}
	}
}

func TestAdminGrantHistoryUsesStableIndependentPagination(t *testing.T) {
	adminHTML, err := webFS.ReadFile("web/admin.html")
	if err != nil {
		t.Fatal(err)
	}
	adminJS := string(adminJSForContract(t))
	combined := string(adminHTML) + adminJS
	for _, required := range []string{
		`id="admin-grants-prev"`, `id="admin-grants-next"`, `id="admin-grants-page"`,
		`grantsPage: { cursor: "", nextCursor: "", backPages: [], forwardPages: [], loading: false }`,
		`/api/v1/admin/balance-grants?cursor=${encodeURIComponent(cursor)}`,
		`nextCursor: typeof data?.next_cursor === "string"`,
		`function previousAdminGrantPage()`, `async function nextAdminGrantPage()`,
		`loadAdminGrants({ cursor: "", navigation: "reset" })`,
	} {
		if !strings.Contains(combined, required) {
			t.Fatalf("administrator grant pagination is missing %q", required)
		}
	}
	if strings.Contains(adminJS, `/api/v1/admin/balance-grants?limit=100`) {
		t.Fatal("administrator grant history still truncates the feed to the latest 100 rows")
	}
}

func TestWebTablesAndStylesKeepAccessibleThemeContracts(t *testing.T) {
	userHTML, err := webFS.ReadFile("web/user.html")
	if err != nil {
		t.Fatal(err)
	}
	adminHTML, err := webFS.ReadFile("web/admin.html")
	if err != nil {
		t.Fatal(err)
	}
	appCSS, err := webFS.ReadFile("web/assets/app.css")
	if err != nil {
		t.Fatal(err)
	}
	markup := string(userHTML) + string(adminHTML)
	styles := string(appCSS)
	if tables, captions := strings.Count(markup, "<table"), strings.Count(markup, "<caption"); tables != captions {
		t.Fatalf("every data table needs a caption: tables=%d captions=%d", tables, captions)
	}
	headers := strings.Count(markup, "<th") - strings.Count(markup, "<thead")
	if scoped := strings.Count(markup, `<th scope="col">`); headers != scoped {
		t.Fatalf("every table header needs column scope: headers=%d scoped=%d", headers, scoped)
	}
	for _, required := range []string{
		".form-lock-reason {",
		".button.compact,",
		"height: 128px;",
		"min-height: 44px;",
		"width: 44px;",
		"background: var(--surface-subtle);",
		"tbody tr:hover td",
		"background: var(--primary-soft);",
	} {
		if !strings.Contains(styles, required) {
			t.Fatalf("theme and touch contract is missing %q", required)
		}
	}
	for selector, want := range map[string]int{
		"@media (prefers-color-scheme: dark)": 1,
		".admin-table-pagination {":           1,
		".user-record-pagination {":           1,
	} {
		if got := strings.Count(styles, selector); got != want {
			t.Fatalf("style selector %q occurs %d times, want %d", selector, got, want)
		}
	}
	for _, forbidden := range []string{
		"background: #f6f8fc;", "background: #f2f6ff;", "—",
	} {
		if strings.Contains(markup+styles, forbidden) {
			t.Fatalf("web UI contains forbidden fixed-theme or copy token %q", forbidden)
		}
	}
}

func TestAdminRefreshRestoresStableButton(t *testing.T) {
	adminJS, err := webFS.ReadFile("web/assets/admin.js")
	if err != nil {
		t.Fatal(err)
	}
	content := string(adminJS)
	start := strings.Index(content, `ui.byId("refresh-admin").addEventListener`)
	end := strings.Index(content, `ui.byId("refresh-admin-grants").addEventListener`)
	if start < 0 || end <= start {
		t.Fatal("administrator refresh listener is missing")
	}
	block := content[start:end]
	captureIndex := strings.Index(block, "const button = event.currentTarget;")
	awaitIndex := strings.Index(block, "await refreshAll();")
	if captureIndex < 0 || awaitIndex < 0 || captureIndex > awaitIndex ||
		strings.Count(block, "event.currentTarget") != 1 ||
		!strings.Contains(block, `ui.setButtonBusy(button, false);`) {
		t.Fatal("administrator refresh does not restore its stable button reference")
	}
}
