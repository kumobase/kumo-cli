package cli

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

const pkgUtilsNPMJSON = `{"id":1,"organization_id":1,"format":"npm","name":"@acme/utils",` +
	`"latest_version":"1.2.0","version_count":3,` +
	`"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-02-01T00:00:00Z"}`

const pkgUtilsPyPIJSON = `{"id":2,"organization_id":1,"format":"pypi","name":"@acme/utils",` +
	`"latest_version":"0.4.1","version_count":1,` +
	`"created_at":"2024-01-03T00:00:00Z","updated_at":"2024-02-03T00:00:00Z"}`

// pkgOrphanJSON is a package whose versions have all been unpublished, so the
// server reports no latest_version.
const pkgOrphanJSON = `{"id":3,"organization_id":1,"format":"maven","name":"com.acme:lib",` +
	`"version_count":0,"created_at":"2024-01-05T00:00:00Z","updated_at":"2024-02-05T00:00:00Z"}`

const pkgDetailNPMJSON = `{"package":` + pkgUtilsNPMJSON + `,` +
	`"versions":[{"version":"1.2.0","size_bytes":2048,"shasum":"abc123","integrity":"sha512-xyz",` +
	`"published_at":"2024-02-01T00:00:00Z"},` +
	`{"version":"1.1.0","size_bytes":1024,"deprecated":"use 1.2.0",` +
	`"published_at":"2024-01-15T00:00:00Z"}],` +
	`"dist_tags":{"latest":"1.2.0","next":"2.0.0-beta.1"}}`

// pkgDetailPyPIJSON is a non-npm package: dist_tags is an empty map and the
// npm-only shasum/integrity fields are absent.
const pkgDetailPyPIJSON = `{"package":` + pkgUtilsPyPIJSON + `,` +
	`"versions":[{"version":"0.4.1","size_bytes":512,"published_at":"2024-02-03T00:00:00Z"}],` +
	`"dist_tags":{}}`

const pkgVersionNPMJSON = `{"version":"1.2.0","size_bytes":2048,"shasum":"abc123",` +
	`"integrity":"sha512-xyz","published_at":"2024-02-01T00:00:00Z"}`

// pkgSuffix returns the part of the request path after ".../packages/", taken
// from the RAW escaped path. The server routes this segment as a greedy
// wildcard and unescapes only the name, so tests must inspect the escaped form
// to see what the SDK actually put on the wire.
func pkgSuffix(r *http.Request) string {
	const marker = "/packages/"
	p := r.URL.EscapedPath()
	i := strings.LastIndex(p, marker)
	if i < 0 {
		return ""
	}
	return p[i+len(marker):]
}

// packagesMux registers a greedy handler for the org-scoped packages routes,
// mirroring how the server routes them, and records the raw escaped suffix of
// each request so tests can assert on SDK encoding.
func packagesMux(t *testing.T, handler func(w http.ResponseWriter, suffix string)) (*http.ServeMux, *[]string) {
	t.Helper()
	mux := http.NewServeMux()
	var seen []string
	mux.HandleFunc("/api/v1/packages/organizations/acme/packages/", func(w http.ResponseWriter, r *http.Request) {
		suffix := pkgSuffix(r)
		seen = append(seen, suffix)
		handler(w, suffix)
	})
	return mux, &seen
}

func TestPackagesList(t *testing.T) {
	mux, _ := packagesMux(t, func(w http.ResponseWriter, _ string) {
		writeEnvelope(w, http.StatusOK, "["+pkgUtilsNPMJSON+","+pkgOrphanJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("packages", "list", "--org", "acme")
	if err != nil {
		t.Fatalf("packages list: %v", err)
	}
	for _, want := range []string{"NAME", "FORMAT", "LATEST", "VERSIONS", "@acme/utils", "npm", "1.2.0", "com.acme:lib", "maven"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %s", want, out)
		}
	}
	// pkgOrphanJSON has no latest_version — it must render as a dash, not blank.
	if !strings.Contains(out, "-") {
		t.Errorf("expected a dash for the missing latest_version: %s", out)
	}
}

func TestPackagesListJSONIsBareArray(t *testing.T) {
	mux, _ := packagesMux(t, func(w http.ResponseWriter, _ string) {
		writeEnvelope(w, http.StatusOK, "["+pkgUtilsNPMJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("packages", "list", "--org", "acme", "-o", "json")
	if err != nil {
		t.Fatalf("packages list -o json: %v", err)
	}
	var items []struct {
		Name   string `json:"name"`
		Format string `json:"format"`
	}
	decodeItems(t, out, &items)
	if len(items) != 1 || items[0].Name != "@acme/utils" || items[0].Format != "npm" {
		t.Fatalf("unexpected json list: %s", out)
	}
}

func TestPackagesListPageFooter(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/packages/organizations/acme/packages/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"code":"OK","message":"ok","data":[%s],`+
			`"meta":{"page":1,"page_size":1,"total_items":3,"total_pages":3}}`, pkgUtilsNPMJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("packages", "list", "--org", "acme", "--page", "1", "--page-size", "1")
	if err != nil {
		t.Fatalf("packages list: %v", err)
	}
	if !strings.Contains(out, "Page 1/3") {
		t.Errorf("expected page footer: %s", out)
	}
}

// TestPackagesGetScopedNameEscaping pins the wire contract for scoped npm
// names: the "/" is escaped to %2F so the name stays one segment, while "@" is
// left alone. url.QueryEscape would mangle the "@" and the server would 404.
func TestPackagesGetScopedNameEscaping(t *testing.T) {
	mux, seen := packagesMux(t, func(w http.ResponseWriter, _ string) {
		writeEnvelope(w, http.StatusOK, pkgDetailNPMJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("packages", "get", "@acme/utils", "--format", "npm", "--org", "acme")
	if err != nil {
		t.Fatalf("packages get: %v", err)
	}
	if len(*seen) != 1 || (*seen)[0] != "npm/@acme%2Futils" {
		t.Fatalf("expected path suffix npm/@acme%%2Futils, got %v", *seen)
	}
	for _, want := range []string{"@acme/utils", "npm", "1.2.0", "DIST TAG", "latest", "next", "VERSION", "use 1.2.0"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q: %s", want, out)
		}
	}
}

// TestPackagesVersionGetRawVersion pins the other half of the encoding
// contract: the version is sent RAW. The server unescapes only the name, so an
// escaped "1.0.0+build" would arrive as "1.0.0%2Bbuild" and never match.
func TestPackagesVersionGetRawVersion(t *testing.T) {
	mux, seen := packagesMux(t, func(w http.ResponseWriter, _ string) {
		writeEnvelope(w, http.StatusOK, pkgVersionNPMJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("packages", "version", "get", "lib", "1.0.0+build",
		"--format", "npm", "--org", "acme"); err != nil {
		t.Fatalf("packages version get: %v", err)
	}
	want := "npm/lib/versions/1.0.0+build"
	if len(*seen) != 1 || (*seen)[0] != want {
		t.Fatalf("expected path suffix %q, got %v", want, *seen)
	}
}

func TestPackagesVersionGetDetail(t *testing.T) {
	mux, _ := packagesMux(t, func(w http.ResponseWriter, _ string) {
		writeEnvelope(w, http.StatusOK, pkgVersionNPMJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("packages", "version", "get", "@acme/utils", "1.2.0",
		"--format", "npm", "--org", "acme")
	if err != nil {
		t.Fatalf("packages version get: %v", err)
	}
	for _, want := range []string{"Version:", "1.2.0", "Shasum:", "abc123", "Integrity:", "Deprecated:"} {
		if !strings.Contains(out, want) {
			t.Errorf("version detail missing %q: %s", want, out)
		}
	}
}

// TestPackagesGetNonNPMOmitsDistTags: dist_tags is npm-only and comes back as
// an empty map elsewhere — rendering an empty section would be noise.
func TestPackagesGetNonNPMOmitsDistTags(t *testing.T) {
	mux, _ := packagesMux(t, func(w http.ResponseWriter, _ string) {
		writeEnvelope(w, http.StatusOK, pkgDetailPyPIJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("packages", "get", "@acme/utils", "--format", "pypi", "--org", "acme")
	if err != nil {
		t.Fatalf("packages get: %v", err)
	}
	if strings.Contains(out, "DIST TAG") {
		t.Errorf("pypi package should render no dist-tags section: %s", out)
	}
	if !strings.Contains(out, "0.4.1") {
		t.Errorf("detail missing version: %s", out)
	}
}

func TestPackagesAutoResolveFormatSingleMatch(t *testing.T) {
	mux, seen := packagesMux(t, func(w http.ResponseWriter, suffix string) {
		if suffix == "" {
			writeEnvelope(w, http.StatusOK, "["+pkgUtilsNPMJSON+","+pkgOrphanJSON+"]")
			return
		}
		writeEnvelope(w, http.StatusOK, pkgDetailNPMJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	// No --format: the name is unique across formats, so it is inferred.
	if _, _, err := runCLI("packages", "get", "@acme/utils", "--org", "acme"); err != nil {
		t.Fatalf("packages get without --format: %v", err)
	}
	if len(*seen) != 2 || (*seen)[1] != "npm/@acme%2Futils" {
		t.Fatalf("expected a list then an npm get, got %v", *seen)
	}
}

func TestPackagesAutoResolveFormatAmbiguous(t *testing.T) {
	mux, _ := packagesMux(t, func(w http.ResponseWriter, _ string) {
		writeEnvelope(w, http.StatusOK, "["+pkgUtilsNPMJSON+","+pkgUtilsPyPIJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("packages", "get", "@acme/utils", "--org", "acme")
	if err == nil {
		t.Fatal("expected an ambiguity error when the name exists in two formats")
	}
	for _, want := range []string{"npm", "pypi", "--format"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("ambiguity error missing %q: %v", want, err)
		}
	}
	if got := exitCodeFor(err); got != 5 {
		t.Errorf("ambiguous format should exit 5 (conflict), got %d", got)
	}
}

func TestPackagesAutoResolveFormatNotFound(t *testing.T) {
	mux, _ := packagesMux(t, func(w http.ResponseWriter, _ string) {
		writeEnvelope(w, http.StatusOK, "["+pkgUtilsNPMJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("packages", "get", "nope", "--org", "acme")
	if err == nil {
		t.Fatal("expected a not-found error")
	}
	if got := exitCodeFor(err); got != 4 {
		t.Errorf("unknown package should exit 4 (not-found), got %d", got)
	}
}

// TestPackagesAutoResolveFormatPaginated: the list route is paginated and
// clamped to 100 server-side, so inference must page or it silently misses
// packages in a large org.
func TestPackagesAutoResolveFormatPaginated(t *testing.T) {
	mux := http.NewServeMux()
	var pages []string
	mux.HandleFunc("/api/v1/packages/organizations/acme/packages/", func(w http.ResponseWriter, r *http.Request) {
		if suffix := pkgSuffix(r); suffix != "" {
			writeEnvelope(w, http.StatusOK, pkgDetailNPMJSON)
			return
		}
		page := r.URL.Query().Get("page")
		pages = append(pages, page)
		w.Header().Set("Content-Type", "application/json")
		// The target lives on page 2; page 1 holds an unrelated package.
		body := pkgOrphanJSON
		if page == "2" {
			body = pkgUtilsNPMJSON
		}
		fmt.Fprintf(w, `{"code":"OK","message":"ok","data":[%s],`+
			`"meta":{"page":%s,"page_size":1,"total_items":2,"total_pages":2}}`, body, page)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("packages", "get", "@acme/utils", "--org", "acme"); err != nil {
		t.Fatalf("packages get should find a package on page 2: %v", err)
	}
	if len(pages) != 2 || pages[0] != "1" || pages[1] != "2" {
		t.Errorf("expected both pages to be walked, got %v", pages)
	}
}

// TestPackagesInvalidFormatIsLocal: an unknown format must fail as a usage
// error before any request, so PACKAGE_INVALID_FORMAT never round-trips.
func TestPackagesInvalidFormatIsLocal(t *testing.T) {
	mux, seen := packagesMux(t, func(w http.ResponseWriter, _ string) {
		writeEnvelope(w, http.StatusOK, pkgDetailNPMJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("packages", "get", "utils", "--format", "bogus", "--org", "acme")
	if err == nil {
		t.Fatal("expected an error for an invalid --format")
	}
	if got := exitCodeFor(err); got != 2 {
		t.Errorf("invalid --format should exit 2 (usage), got %d", got)
	}
	if len(*seen) != 0 {
		t.Errorf("invalid --format must not reach the server, got %v", *seen)
	}
}

func TestPackagesFormatIsCaseInsensitive(t *testing.T) {
	mux, seen := packagesMux(t, func(w http.ResponseWriter, _ string) {
		writeEnvelope(w, http.StatusOK, pkgDetailNPMJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("packages", "get", "utils", "--format", "NPM", "--org", "acme"); err != nil {
		t.Fatalf("--format NPM should be accepted: %v", err)
	}
	if len(*seen) != 1 || !strings.HasPrefix((*seen)[0], "npm/") {
		t.Errorf("expected the format lowercased on the wire, got %v", *seen)
	}
}

// TestPackagesInvalidSortIsLocal: the server honours only name/created_at and
// silently falls back to updated_at, so anything else is rejected up front.
func TestPackagesInvalidSortIsLocal(t *testing.T) {
	mux, seen := packagesMux(t, func(w http.ResponseWriter, _ string) {
		writeEnvelope(w, http.StatusOK, "[]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("packages", "list", "--org", "acme", "--sort", "size")
	if err == nil {
		t.Fatal("expected an error for an unsupported --sort column")
	}
	if got := exitCodeFor(err); got != 2 {
		t.Errorf("invalid --sort should exit 2 (usage), got %d", got)
	}
	if len(*seen) != 0 {
		t.Errorf("invalid --sort must not reach the server, got %v", *seen)
	}
	if _, _, err := runCLI("packages", "list", "--org", "acme", "--sort", "name"); err != nil {
		t.Errorf("--sort name is supported and should pass: %v", err)
	}
}

// TestPackagesDeleteIsScheduled: the server soft-deletes and schedules a GC
// purge, so the result must not claim the deletion is done.
func TestPackagesDeleteIsScheduled(t *testing.T) {
	mux, seen := packagesMux(t, func(w http.ResponseWriter, _ string) {
		writeEnvelope(w, http.StatusOK, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("packages", "delete", "@acme/utils", "--format", "npm",
		"--org", "acme", "-y", "-o", "json")
	if err != nil {
		t.Fatalf("packages delete: %v", err)
	}
	var got struct {
		Resource string `json:"resource"`
		Action   string `json:"action"`
		Status   string `json:"status"`
	}
	decodeData(t, out, &got)
	if got.Resource != "package" || got.Action != "delete" || got.Status != "scheduled" {
		t.Fatalf("unexpected action result: %+v", got)
	}
	if len(*seen) != 1 || (*seen)[0] != "npm/@acme%2Futils" {
		t.Fatalf("unexpected delete path: %v", *seen)
	}
}

func TestPackagesVersionDeleteIsScheduled(t *testing.T) {
	mux, seen := packagesMux(t, func(w http.ResponseWriter, _ string) {
		writeEnvelope(w, http.StatusOK, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("packages", "version", "delete", "@acme/utils", "1.1.0",
		"--format", "npm", "--org", "acme", "-y", "-o", "json")
	if err != nil {
		t.Fatalf("packages version delete: %v", err)
	}
	var got struct {
		Resource string `json:"resource"`
		Status   string `json:"status"`
	}
	decodeData(t, out, &got)
	if got.Resource != "package-version" || got.Status != "scheduled" {
		t.Fatalf("unexpected action result: %+v", got)
	}
	if len(*seen) != 1 || (*seen)[0] != "npm/@acme%2Futils/versions/1.1.0" {
		t.Fatalf("unexpected delete path: %v", *seen)
	}
}

func TestPackagesDeleteAborted(t *testing.T) {
	mux, seen := packagesMux(t, func(w http.ResponseWriter, _ string) {
		writeEnvelope(w, http.StatusOK, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	root := NewRootCmd()
	var out, errBuf strings.Builder
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetIn(strings.NewReader("n\n"))
	root.SetArgs([]string{"packages", "delete", "@acme/utils", "--format", "npm",
		"--org", "acme", "-o", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("a declined confirmation is a user choice, not a failure: %v", err)
	}
	if !strings.Contains(out.String(), `"aborted": true`) {
		t.Errorf("expected an aborted payload, got %q", out.String())
	}
	if len(*seen) != 0 {
		t.Errorf("a declined delete must not reach the server, got %v", *seen)
	}
}

func TestPackagesErrorCodes(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		code     string
		message  string
		wantExit int
		wantText string
	}{
		{"not-found", http.StatusNotFound, "PACKAGE_NOT_FOUND", "package not found", 4, `no package named "@acme/utils"`},
		{"suspended", http.StatusForbidden, "PACKAGE_ORG_SUSPENDED", "org suspended", 1, "suspended"},
		{"invalid-version", http.StatusBadRequest, "PACKAGE_INVALID_VERSION", "bad version", 6, ""},
		{"forbidden", http.StatusForbidden, "PACKAGE_FORBIDDEN", "no access", 3, "do not grant packages access"},
		{"unpublish-forbidden", http.StatusForbidden, "PACKAGE_UNPUBLISH_FORBIDDEN", "window passed", 1, "unpublish window"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mux, _ := packagesMux(t, func(w http.ResponseWriter, _ string) {
				writeError(w, tc.status, tc.code, tc.message)
			})
			srv := newServer(t, mux)
			mockEnv(t, srv.URL)

			_, _, err := runCLI("packages", "get", "@acme/utils", "--format", "npm", "--org", "acme")
			if err == nil {
				t.Fatalf("expected an error for %s", tc.code)
			}
			if got := exitCodeFor(err); got != tc.wantExit {
				t.Errorf("%s: want exit %d, got %d (err=%v)", tc.code, tc.wantExit, got, err)
			}
			if tc.wantText != "" && !strings.Contains(err.Error(), tc.wantText) {
				t.Errorf("%s: message %q missing %q", tc.code, err.Error(), tc.wantText)
			}
		})
	}
}

func TestPackagesVersionNotFound(t *testing.T) {
	mux, _ := packagesMux(t, func(w http.ResponseWriter, _ string) {
		writeError(w, http.StatusNotFound, "PACKAGE_VERSION_NOT_FOUND", "version not found")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("packages", "version", "get", "@acme/utils", "9.9.9",
		"--format", "npm", "--org", "acme")
	if err == nil {
		t.Fatal("expected a not-found error")
	}
	if !strings.Contains(err.Error(), `has no version "9.9.9"`) {
		t.Errorf("unexpected message: %v", err)
	}
	if got := exitCodeFor(err); got != 4 {
		t.Errorf("want exit 4, got %d", got)
	}
}

func TestPackagesPlans(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/packages/plans", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK,
			`{"plans":[{"id":1,"name":"Standard","unit":"GB-month","price_per_unit":"1500.00",`+
				`"currency":"IDR","charge_model":"metered","billing_period":"monthly"}]}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("packages", "plans")
	if err != nil {
		t.Fatalf("packages plans: %v", err)
	}
	for _, want := range []string{"NAME", "PRICE/UNIT", "Standard", "1500.00", "IDR", "GB-month"} {
		if !strings.Contains(out, want) {
			t.Errorf("plans output missing %q: %s", want, out)
		}
	}
}

// TestPackagesResolvesSoleOrg covers the --org fallback: packages share the
// organization entity with the registry, so resolveOrgSlug is reused.
func TestPackagesResolvesSoleOrg(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+orgAcmeJSON+"]")
	})
	mux.HandleFunc("/api/v1/packages/organizations/acme/packages/", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+pkgUtilsNPMJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("packages", "list")
	if err != nil {
		t.Fatalf("packages list without --org: %v", err)
	}
	if !strings.Contains(out, "@acme/utils") {
		t.Errorf("expected the sole org to be resolved: %s", out)
	}
}
