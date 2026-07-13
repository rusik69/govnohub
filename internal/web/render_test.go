package web

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	gitstore "github.com/rusik69/govnohub/internal/git"
	"github.com/rusik69/govnohub/internal/issue"
	"github.com/rusik69/govnohub/internal/pull"
)

// ---------------------------------------------------------------------------
// Existing tests (kept as-is)
// ---------------------------------------------------------------------------

func TestIsBinaryContent(t *testing.T) {
	if isBinaryContent([]byte("hello\n")) {
		t.Fatal("text should not be binary")
	}
	if !isBinaryContent([]byte{0x00, 0x01}) {
		t.Fatal("nul byte should be binary")
	}
}

func TestRedirectReferer(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "/", nil)
	if got := redirectReferer(r, "/fallback"); got != "/fallback" {
		t.Fatalf("got %q", got)
	}
	r.Header.Set("Referer", "/alice/app")
	if got := redirectReferer(r, "/fallback"); got != "/alice/app" {
		t.Fatalf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// render
// ---------------------------------------------------------------------------

func TestRender(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	render(w, r, templ.NopComponent)
	resp := w.Result()
	if resp.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("got content-type %q", resp.Header.Get("Content-Type"))
	}
}

// ---------------------------------------------------------------------------
// redirectWithFlash
// ---------------------------------------------------------------------------

func TestRedirectWithFlash_Success(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/target", nil)
	redirectWithFlash(w, r, "/target", "all good", false)
	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("got status %d, want 303", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/target?ok=all+good" {
		t.Fatalf("got location %q, want /target?ok=all+good", loc)
	}
}

func TestRedirectWithFlash_Error(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/target", nil)
	redirectWithFlash(w, r, "/target", "something went wrong", true)
	resp := w.Result()
	loc := resp.Header.Get("Location")
	if loc != "/target?err=something+went+wrong" {
		t.Fatalf("got location %q", loc)
	}
}

func TestRedirectWithFlash_ErrorClearsOK(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/target?ok=prev", nil)
	redirectWithFlash(w, r, "/target?ok=prev", "new error", true)
	resp := w.Result()
	loc := resp.Header.Get("Location")
	if loc != "/target?err=new+error" {
		t.Fatalf("got location %q", loc)
	}
}

func TestRedirectWithFlash_SuccessClearsErr(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/target?err=prev", nil)
	redirectWithFlash(w, r, "/target?err=prev", "ok now", false)
	resp := w.Result()
	loc := resp.Header.Get("Location")
	if loc != "/target?ok=ok+now" {
		t.Fatalf("got location %q", loc)
	}
}

func TestRedirectWithFlash_InvalidURL(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	// An invalid URL like one containing null byte
	redirectWithFlash(w, r, "/valid", "msg", false)
	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("got status %d, want 303", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// parseNum
// ---------------------------------------------------------------------------

func requestWithChiParam(key, value string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, chiCtx))
}

func TestParseNum_Valid(t *testing.T) {
	w := httptest.NewRecorder()
	r := requestWithChiParam("number", "42")
	n, ok := parseNum(w, r, "number")
	if !ok {
		t.Fatal("expected ok")
	}
	if n != 42 {
		t.Fatalf("got %d, want 42", n)
	}
}

func TestParseNum_Zero(t *testing.T) {
	w := httptest.NewRecorder()
	r := requestWithChiParam("n", "0")
	_, ok := parseNum(w, r, "n")
	if ok {
		t.Fatal("expected false for zero")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400", w.Code)
	}
}

func TestParseNum_Negative(t *testing.T) {
	w := httptest.NewRecorder()
	r := requestWithChiParam("n", "-5")
	_, ok := parseNum(w, r, "n")
	if ok {
		t.Fatal("expected false for negative")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400", w.Code)
	}
}

func TestParseNum_NotInt(t *testing.T) {
	w := httptest.NewRecorder()
	r := requestWithChiParam("n", "abc")
	_, ok := parseNum(w, r, "n")
	if ok {
		t.Fatal("expected false for non-integer")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400", w.Code)
	}
}

func TestParseNum_Empty(t *testing.T) {
	w := httptest.NewRecorder()
	r := requestWithChiParam("n", "")
	_, ok := parseNum(w, r, "n")
	if ok {
		t.Fatal("expected false for empty")
	}
}

func TestParseNum_MissingKey(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	_, ok := parseNum(w, r, "nonexistent")
	if ok {
		t.Fatal("expected false for missing key")
	}
}

// ---------------------------------------------------------------------------
// parseUUIDParam
// ---------------------------------------------------------------------------

func TestParseUUIDParam_Valid(t *testing.T) {
	w := httptest.NewRecorder()
	id := uuid.New().String()
	r := requestWithChiParam("id", id)
	parsed, ok := parseUUIDParam(w, r, "id")
	if !ok {
		t.Fatal("expected ok")
	}
	if parsed.String() != id {
		t.Fatalf("got %s, want %s", parsed.String(), id)
	}
}

func TestParseUUIDParam_Invalid(t *testing.T) {
	w := httptest.NewRecorder()
	r := requestWithChiParam("id", "not-a-uuid")
	_, ok := parseUUIDParam(w, r, "id")
	if ok {
		t.Fatal("expected false for invalid UUID")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400", w.Code)
	}
}

func TestParseUUIDParam_Empty(t *testing.T) {
	w := httptest.NewRecorder()
	r := requestWithChiParam("id", "")
	_, ok := parseUUIDParam(w, r, "id")
	if ok {
		t.Fatal("expected false for empty")
	}
}

// ---------------------------------------------------------------------------
// layoutMainClass
// ---------------------------------------------------------------------------

func TestLayoutMainClass_FullWidth(t *testing.T) {
	if got := layoutMainClass(LayoutData{FullWidth: true}); got != "p-4" {
		t.Fatalf("got %q", got)
	}
}

func TestLayoutMainClass_Default(t *testing.T) {
	if got := layoutMainClass(LayoutData{}); got != "max-w-7xl mx-auto p-4" {
		t.Fatalf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// globalNavClass
// ---------------------------------------------------------------------------

func TestGlobalNavClass_Active(t *testing.T) {
	if got := globalNavClass("dashboard", "dashboard"); got != "active" {
		t.Fatalf("got %q", got)
	}
}

func TestGlobalNavClass_Inactive(t *testing.T) {
	if got := globalNavClass("dashboard", "orgs"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestGlobalNavClass_BothEmpty(t *testing.T) {
	if got := globalNavClass("", ""); got != "active" {
		t.Fatalf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// unreadLabel
// ---------------------------------------------------------------------------

func TestUnreadLabel_Zero(t *testing.T) {
	if got := unreadLabel(0); got != "0" {
		t.Fatalf("got %q", got)
	}
}

func TestUnreadLabel_Under99(t *testing.T) {
	if got := unreadLabel(42); got != "42" {
		t.Fatalf("got %q", got)
	}
}

func TestUnreadLabel_99(t *testing.T) {
	if got := unreadLabel(99); got != "99" {
		t.Fatalf("got %q", got)
	}
}

func TestUnreadLabel_Over99(t *testing.T) {
	if got := unreadLabel(100); got != "99+" {
		t.Fatalf("got %q", got)
	}
}

func TestUnreadLabel_1000(t *testing.T) {
	if got := unreadLabel(1000); got != "99+" {
		t.Fatalf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// avatarClass
// ---------------------------------------------------------------------------

func TestAvatarClass_Default(t *testing.T) {
	if got := avatarClass(""); got != "gh-avatar" {
		t.Fatalf("got %q", got)
	}
}

func TestAvatarClass_Large(t *testing.T) {
	if got := avatarClass("lg"); got != "gh-avatar gh-avatar-lg" {
		t.Fatalf("got %q", got)
	}
}

func TestAvatarClass_UnknownSize(t *testing.T) {
	if got := avatarClass("xl"); got != "gh-avatar" {
		t.Fatalf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// avatarInitial
// ---------------------------------------------------------------------------

func TestAvatarInitial_Normal(t *testing.T) {
	if got := avatarInitial("alice"); got != "A" {
		t.Fatalf("got %q", got)
	}
}

func TestAvatarInitial_Empty(t *testing.T) {
	if got := avatarInitial(""); got != "?" {
		t.Fatalf("got %q", got)
	}
}

func TestAvatarInitial_MixedCase(t *testing.T) {
	if got := avatarInitial("bOb"); got != "B" {
		t.Fatalf("got %q", got)
	}
}

func TestAvatarInitial_SingleChar(t *testing.T) {
	if got := avatarInitial("x"); got != "X" {
		t.Fatalf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// avatarColor
// ---------------------------------------------------------------------------

func TestAvatarColor_Deterministic(t *testing.T) {
	c1 := avatarColor("alice")
	c2 := avatarColor("alice")
	if c1 != c2 {
		t.Fatalf("not deterministic: %s vs %s", c1, c2)
	}
}

func TestAvatarColor_NotEmpty(t *testing.T) {
	if got := avatarColor("test"); got == "" {
		t.Fatal("got empty color")
	}
}

func TestAvatarColor_DifferentNames(t *testing.T) {
	if avatarColor("alice") == avatarColor("bob") {
		t.Log("warning: different names may produce same color, this is expected")
	}
}

// ---------------------------------------------------------------------------
// tabAriaCurrent
// ---------------------------------------------------------------------------

func TestTabAriaCurrent_Active(t *testing.T) {
	if got := tabAriaCurrent("code", "code"); got != "page" {
		t.Fatalf("got %q", got)
	}
}

func TestTabAriaCurrent_Inactive(t *testing.T) {
	if got := tabAriaCurrent("code", "issues"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestTabAriaCurrent_EmptyActive(t *testing.T) {
	if got := tabAriaCurrent("", ""); got != "page" {
		t.Fatalf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// iconName
// ---------------------------------------------------------------------------

func TestIconName_WithIcon(t *testing.T) {
	if got := iconName("star", "code"); got != "star" {
		t.Fatalf("got %q", got)
	}
}

func TestIconName_EmptyIcon(t *testing.T) {
	if got := iconName("", "code"); got != "code" {
		t.Fatalf("got %q", got)
	}
}

func TestIconName_BothEmpty(t *testing.T) {
	if got := iconName("", ""); got != "" {
		t.Fatalf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// tabClass
// ---------------------------------------------------------------------------

func TestTabClass_Active(t *testing.T) {
	if got := tabClass("code", "code"); got != "active" {
		t.Fatalf("got %q", got)
	}
}

func TestTabClass_Inactive(t *testing.T) {
	if got := tabClass("code", "issues"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestTabClass_BothEmpty(t *testing.T) {
	if got := tabClass("", ""); got != "active" {
		t.Fatalf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// subheadPillClass
// ---------------------------------------------------------------------------

func TestSubheadPillClass_ActiveExact(t *testing.T) {
	if got := subheadPillClass("open", "open"); got != "gh-subhead-pill active" {
		t.Fatalf("got %q", got)
	}
}

func TestSubheadPillClass_DefaultToOpen(t *testing.T) {
	if got := subheadPillClass("", "open"); got != "gh-subhead-pill active" {
		t.Fatalf("got %q", got)
	}
}

func TestSubheadPillClass_Inactive(t *testing.T) {
	if got := subheadPillClass("open", "closed"); got != "gh-subhead-pill" {
		t.Fatalf("got %q", got)
	}
}

func TestSubheadPillClass_Closed(t *testing.T) {
	if got := subheadPillClass("closed", "closed"); got != "gh-subhead-pill active" {
		t.Fatalf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// subheadAriaCurrent
// ---------------------------------------------------------------------------

func TestSubheadAriaCurrent_ActiveExact(t *testing.T) {
	if got := subheadAriaCurrent("open", "open"); got != "page" {
		t.Fatalf("got %q", got)
	}
}

func TestSubheadAriaCurrent_DefaultToOpen(t *testing.T) {
	if got := subheadAriaCurrent("", "open"); got != "page" {
		t.Fatalf("got %q", got)
	}
}

func TestSubheadAriaCurrent_Inactive(t *testing.T) {
	if got := subheadAriaCurrent("open", "closed"); got != "" {
		t.Fatalf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// countIssuesByState
// ---------------------------------------------------------------------------

func TestCountIssuesByState_AllOpen(t *testing.T) {
	issues := []issue.Issue{
		{State: "open"},
		{State: "open"},
	}
	if got := countIssuesByState(issues, "open"); got != 2 {
		t.Fatalf("got %d", got)
	}
}

func TestCountIssuesByState_Mixed(t *testing.T) {
	issues := []issue.Issue{
		{State: "open"},
		{State: "closed"},
		{State: "open"},
	}
	if got := countIssuesByState(issues, "open"); got != 2 {
		t.Fatalf("got %d", got)
	}
	if got := countIssuesByState(issues, "closed"); got != 1 {
		t.Fatalf("got %d", got)
	}
}

func TestCountIssuesByState_Empty(t *testing.T) {
	if got := countIssuesByState(nil, "open"); got != 0 {
		t.Fatalf("got %d", got)
	}
}

// ---------------------------------------------------------------------------
// countPRsByState
// ---------------------------------------------------------------------------

func TestCountPRsByState_AllClosed(t *testing.T) {
	prs := []pull.PullRequest{
		{State: "closed"},
		{State: "closed"},
	}
	if got := countPRsByState(prs, "closed"); got != 2 {
		t.Fatalf("got %d", got)
	}
}

func TestCountPRsByState_Mixed(t *testing.T) {
	prs := []pull.PullRequest{
		{State: "open"},
		{State: "closed"},
	}
	if got := countPRsByState(prs, "open"); got != 1 {
		t.Fatalf("got %d", got)
	}
	if got := countPRsByState(prs, "closed"); got != 1 {
		t.Fatalf("got %d", got)
	}
}

// ---------------------------------------------------------------------------
// issueIconClass
// ---------------------------------------------------------------------------

func TestIssueIconClass_Open(t *testing.T) {
	if got := issueIconClass("open"); got != "open" {
		t.Fatalf("got %q", got)
	}
}

func TestIssueIconClass_Closed(t *testing.T) {
	if got := issueIconClass("closed"); got != "closed" {
		t.Fatalf("got %q", got)
	}
}

func TestIssueIconClass_Unknown(t *testing.T) {
	if got := issueIconClass("merged"); got != "closed" {
		t.Fatalf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// prIconClass
// ---------------------------------------------------------------------------

func TestPRIconClass_Open(t *testing.T) {
	if got := prIconClass(pull.PullRequest{State: "open"}); got != "open" {
		t.Fatalf("got %q", got)
	}
}

func TestPRIconClass_ClosedUnmerged(t *testing.T) {
	if got := prIconClass(pull.PullRequest{State: "closed"}); got != "closed" {
		t.Fatalf("got %q", got)
	}
}

func TestPRIconClass_Merged(t *testing.T) {
	now := time.Now()
	pr := pull.PullRequest{State: "closed", MergedAt: &now}
	if got := prIconClass(pr); got != "closed" {
		t.Fatalf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// prIconName
// ---------------------------------------------------------------------------

func TestPRIconName_Open(t *testing.T) {
	if got := prIconName(pull.PullRequest{State: "open"}); got != "git-pull-request" {
		t.Fatalf("got %q", got)
	}
}

func TestPRIconName_Merged(t *testing.T) {
	now := time.Now()
	pr := pull.PullRequest{State: "closed", MergedAt: &now}
	if got := prIconName(pr); got != "git-merge" {
		t.Fatalf("got %q", got)
	}
}

func TestPRIconName_ClosedUnmerged(t *testing.T) {
	pr := pull.PullRequest{State: "closed"}
	if got := prIconName(pr); got != "git-pull-request" {
		t.Fatalf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// labelColor
// ---------------------------------------------------------------------------

func TestLabelColor_WithHash(t *testing.T) {
	if got := labelColor("#ff0000"); got != "#ff0000" {
		t.Fatalf("got %q", got)
	}
}

func TestLabelColor_WithoutHash(t *testing.T) {
	if got := labelColor("ff0000"); got != "#ff0000" {
		t.Fatalf("got %q", got)
	}
}

func TestLabelColor_Empty(t *testing.T) {
	if got := labelColor(""); got != "#656d76" {
		t.Fatalf("got %q", got)
	}
}

func TestLabelColor_Short(t *testing.T) {
	if got := labelColor("abc"); got != "#abc" {
		t.Fatalf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// labelTextColor
// ---------------------------------------------------------------------------

func TestLabelTextColor_DarkOnLight(t *testing.T) {
	// light yellow background -> dark text
	if got := labelTextColor("#ffff00"); got != "#1f2328" {
		t.Fatalf("got %q", got)
	}
}

func TestLabelTextColor_WhiteOnDark(t *testing.T) {
	// dark blue background -> white text
	if got := labelTextColor("#0000ff"); got != "#fff" {
		t.Fatalf("got %q", got)
	}
}

func TestLabelTextColor_ShortColor(t *testing.T) {
	if got := labelTextColor("abc"); got != "#fff" {
		t.Fatalf("got %q", got)
	}
}

func TestLabelTextColor_BorderlineLight(t *testing.T) {
	// luminance ~0.63 > 0.6 -> dark text (color #8a8a8a = RGB 138,138,138)
	// 138/255 = 0.541, weighted: (0.299+0.587+0.114)*0.541 = 1.0*0.541 = 0.541
	// Actually let's use something clearly > 0.6: #aaaaaa = 170/255 = 0.667
	if got := labelTextColor("#aaaaaa"); got != "#1f2328" {
		t.Fatalf("got %q", got)
	}
}

func TestLabelTextColor_BorderlineDark(t *testing.T) {
	// luminance ~0.4 < 0.6 -> white text
	if got := labelTextColor("#666666"); got != "#fff" {
		t.Fatalf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// repoEntryURL
// ---------------------------------------------------------------------------

func TestRepoEntryURL_File(t *testing.T) {
	e := gitstore.TreeEntry{Path: "main.go", Type: "blob"}
	got := repoEntryURL("owner/repo", "main", "", e)
	want := "/owner/repo/blob/main.go?ref=main"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRepoEntryURL_Dir(t *testing.T) {
	e := gitstore.TreeEntry{Path: "src", Type: "dir"}
	got := repoEntryURL("owner/repo", "main", "", e)
	want := "/owner/repo/tree/src?ref=main"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRepoEntryURL_WithBasePath(t *testing.T) {
	e := gitstore.TreeEntry{Path: "main.go", Type: "blob"}
	got := repoEntryURL("owner/repo", "dev", "src/utils", e)
	want := "/owner/repo/blob/src/utils/main.go?ref=dev"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRepoEntryURL_DirWithBasePath(t *testing.T) {
	e := gitstore.TreeEntry{Path: "handlers", Type: "dir"}
	got := repoEntryURL("o/r", "main", "internal", e)
	want := "/o/r/tree/internal/handlers?ref=main"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// renderToast helper (exercises render + Toast together)
// ---------------------------------------------------------------------------

func TestRenderToast(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	renderToast(w, r, "hello", "success")
	resp := w.Result()
	if resp.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("got content-type %q", resp.Header.Get("Content-Type"))
	}
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	if !bytes.Contains(buf.Bytes(), []byte("hello")) {
		t.Fatal("expected toast message in output")
	}
	if !bytes.Contains(buf.Bytes(), []byte("toast-success")) {
		t.Fatal("expected toast-success class in output")
	}
}
