package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/kumobase/kumo-go/client"
	"github.com/kumobase/kumo-go/codes"
	"github.com/kumobase/kumo-go/types"
)

// apiErr builds an *client.APIError carrying the given server code so the
// map*Error switches can be exercised branch by branch.
func apiErr(code string) error { return &client.APIError{StatusCode: 400, Code: code} }

func strptr(s string) *string { return &s }
func uintptr(u uint) *uint    { return &u }

// --- string helpers -------------------------------------------------------

func TestShortSHA(t *testing.T) {
	cases := map[string]string{
		"":                                     "-",
		"abc":                                  "abc",
		"0123456789ab":                         "0123456789ab",
		"0123456789abcdef0123456789abcdef1234": "0123456789ab",
	}
	for in, want := range cases {
		if got := shortSHA(in); got != want {
			t.Errorf("shortSHA(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShortDigest(t *testing.T) {
	cases := map[string]string{
		"":                        "",
		"sha256:abc":              "abc",
		"sha256:0123456789abcdef": "0123456789ab",
		"0123456789abcdef":        "0123456789ab",
	}
	for in, want := range cases {
		if got := shortDigest(in); got != want {
			t.Errorf("shortDigest(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCapitalize(t *testing.T) {
	cases := map[string]string{
		"":        "",
		"running": "Running",
		"Stopped": "Stopped",
		"1abc":    "1abc",
	}
	for in, want := range cases {
		if got := capitalize(in); got != want {
			t.Errorf("capitalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOrDashAndValOr(t *testing.T) {
	if orDash("") != "-" || orDash("x") != "x" {
		t.Error("orDash")
	}
	if valOr("", "fb") != "fb" || valOr("v", "fb") != "v" {
		t.Error("valOr")
	}
}

func TestDerefHelpers(t *testing.T) {
	if deref(nil) != "" || deref(strptr("x")) != "x" {
		t.Error("deref")
	}
	if derefOr(nil, "fb") != "fb" || derefOr(strptr(""), "fb") != "fb" || derefOr(strptr("v"), "fb") != "v" {
		t.Error("derefOr")
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		0:          "0 B",
		512:        "512 B",
		1024:       "1.0 KiB",
		1536:       "1.5 KiB",
		1048576:    "1.0 MiB",
		1073741824: "1.0 GiB",
	}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestExitCodeAndDurationLabel(t *testing.T) {
	if exitCodeLabel(nil) != "-" {
		t.Error("exitCodeLabel nil")
	}
	code := 137
	if exitCodeLabel(&code) != "137" {
		t.Error("exitCodeLabel value")
	}
	if durationLabel(nil) != "-" {
		t.Error("durationLabel nil")
	}
	ms := int64(1500)
	if got := durationLabel(&ms); got != "1.5s" {
		t.Errorf("durationLabel = %q, want 1.5s", got)
	}
}

func TestJobSchedule(t *testing.T) {
	if jobSchedule("") != "manual" || jobSchedule("* * * * *") != "* * * * *" {
		t.Error("jobSchedule")
	}
}

func TestFormatVPSDateTime(t *testing.T) {
	if formatVPSDate("") != "-" || formatVPSTime("") != "-" {
		t.Error("empty should be dash")
	}
	if got := formatVPSDate("2026-07-13T10:20:30Z"); got != "2026-07-13" {
		t.Errorf("formatVPSDate = %q", got)
	}
	if got := formatVPSTime("2026-07-13T10:20:30Z"); got != "2026-07-13T10:20:30Z" {
		t.Errorf("formatVPSTime = %q", got)
	}
	// Unparseable falls through to the raw string.
	if formatVPSDate("not-a-date") != "not-a-date" || formatVPSTime("not-a-date") != "not-a-date" {
		t.Error("unparseable should pass through")
	}
}

func TestVPSDisplayName(t *testing.T) {
	if got := vpsDisplayName(&types.VPSServerResponse{DisplayName: "web"}); got != "web" {
		t.Errorf("named = %q", got)
	}
	if got := vpsDisplayName(&types.VPSServerResponse{ID: 42}); got != "(id 42)" {
		t.Errorf("unnamed = %q", got)
	}
}

func TestVolumeLabels(t *testing.T) {
	if got := volumeAppLabel(&types.VolumeResponse{AppName: strptr("api")}); got != "api" {
		t.Errorf("app name = %q", got)
	}
	if got := volumeAppLabel(&types.VolumeResponse{AppID: uintptr(7)}); got != "#7" {
		t.Errorf("app id = %q", got)
	}
	if got := volumeAppLabel(&types.VolumeResponse{}); got != "-" {
		t.Errorf("no app = %q", got)
	}
	if volumeMountLabel(&types.VolumeResponse{}) != "-" || volumeMountLabel(&types.VolumeResponse{MountPath: "/data"}) != "/data" {
		t.Error("volumeMountLabel")
	}
}

func TestManifestLabels(t *testing.T) {
	if got := manifestArchOSLabel(&types.ManifestResponse{Platform: strptr("linux/arm64")}); got != "linux/arm64" {
		t.Errorf("platform = %q", got)
	}
	if got := manifestArchOSLabel(&types.ManifestResponse{Architecture: strptr("amd64"), OS: strptr("linux")}); got != "linux/amd64" {
		t.Errorf("arch/os = %q", got)
	}
	if got := manifestArchOSLabel(&types.ManifestResponse{Kind: "index"}); got != "multi-arch" {
		t.Errorf("index = %q", got)
	}
	if got := manifestArchOSLabel(&types.ManifestResponse{Kind: "image"}); got != "-" {
		t.Errorf("bare = %q", got)
	}

	if got := manifestSizeLabel(&types.ManifestResponse{ImageSizeBytes: 2048}); got != "2.0 KiB" {
		t.Errorf("image size = %q", got)
	}
	if got := manifestSizeLabel(&types.ManifestResponse{SizeBytes: 1024}); got != "1.0 KiB" {
		t.Errorf("manifest size = %q", got)
	}
	if got := manifestSizeLabel(&types.ManifestResponse{}); got != "-" {
		t.Errorf("no size = %q", got)
	}
}

// --- flag parsers ---------------------------------------------------------

func TestParseJobKind(t *testing.T) {
	for _, in := range []string{"standalone"} {
		if k, err := parseJobKind(in); err != nil || k != types.JobKindStandalone {
			t.Errorf("parseJobKind(%q) = %v, %v", in, k, err)
		}
	}
	for _, in := range []string{"app-attached", "app_attached"} {
		if k, err := parseJobKind(in); err != nil || k != types.JobKindAppAttached {
			t.Errorf("parseJobKind(%q) = %v, %v", in, k, err)
		}
	}
	if _, err := parseJobKind("bogus"); err == nil {
		t.Error("parseJobKind should reject bogus")
	}
}

func TestParseConcurrencyPolicy(t *testing.T) {
	for _, p := range []types.JobConcurrencyPolicy{types.JobConcurrencyForbid, types.JobConcurrencyAllow, types.JobConcurrencyReplace} {
		if got, err := parseConcurrencyPolicy(string(p)); err != nil || got != p {
			t.Errorf("parseConcurrencyPolicy(%q) = %v, %v", p, got, err)
		}
	}
	if _, err := parseConcurrencyPolicy("Nope"); err == nil {
		t.Error("parseConcurrencyPolicy should reject Nope")
	}
}

// --- error mappers --------------------------------------------------------

// mapperCase drives a single map*Error function: each mapped code must yield a
// non-nil, friendly error; a nil input must stay nil; and an unrelated code
// must pass through unchanged.
func assertMaps(t *testing.T, name string, fn func(error) error, mapped []string) {
	t.Helper()
	if fn(nil) != nil {
		t.Errorf("%s(nil) should be nil", name)
	}
	passthrough := apiErr("SOME_UNRELATED_CODE")
	if got := fn(passthrough); !errors.Is(got, passthrough) {
		t.Errorf("%s: unrelated code should pass through, got %v", name, got)
	}
	for _, code := range mapped {
		if got := fn(apiErr(code)); got == nil {
			t.Errorf("%s: code %q returned nil", name, code)
		}
	}
}

func TestMapRegistryError(t *testing.T) {
	assertMaps(t, "mapRegistryError", mapRegistryError,
		[]string{codes.RegistrySuspended, codes.AmbiguousName})
}

func TestMapBuildError(t *testing.T) {
	assertMaps(t, "mapBuildError", func(e error) error { return mapBuildError(e, 1) },
		[]string{
			codes.BuildNotFound, codes.BuildAlreadyRunning, codes.BuildLogNotAvailable,
			codes.BuildConnectionRequired, codes.BuildSourceUnavailable, codes.BuildAppImageImmutable,
			codes.BuildNeedsBranch, codes.BuildInvalidTagPattern, codes.BuildInvalidDockerfilePath,
			codes.BuildNoDockerfile, codes.BuildNoRailpackPlan, codes.BuildProviderError,
		})
}

func TestMapBillingError(t *testing.T) {
	assertMaps(t, "mapBillingError", mapBillingError,
		[]string{
			codes.BillingInvalidFilterCombination, codes.BillingInvalidDateRange,
			codes.BillingInvalidEnumValue, codes.BillingBreakdownFailed, codes.BillingInternalError,
		})
}

func TestMapJobError(t *testing.T) {
	assertMaps(t, "mapJobError", func(e error) error { return mapJobError(e, "job1") },
		[]string{
			codes.JobNotFound, codes.JobExecutionNotFound, codes.JobAppRequired, codes.JobAppNotFound,
			codes.JobImageRequired, codes.JobImageNotFound, codes.JobImageUnauthorized,
			codes.JobImageRegistryUnreachable, codes.JobScheduleInvalid, codes.JobScheduleTooFrequent,
			codes.JobTimezoneInvalid, codes.JobKindInvalid, codes.JobKindUnsupported,
			codes.JobConcurrencyUnsupported, codes.JobInvalidPricingSlug, codes.JobQuotaExceeded,
			codes.JobInsufficientBalance,
		})
	// not-found with empty name uses the generic phrasing branch.
	if got := mapJobError(apiErr(codes.JobNotFound), ""); got == nil {
		t.Error("mapJobError empty name")
	}
}

func TestMapRunnerError(t *testing.T) {
	assertMaps(t, "mapRunnerError", func(e error) error { return mapRunnerError(e, 3) },
		[]string{codes.RunnerJobNotFound, codes.RunnerUnauthorized, codes.RunnerInvalidID})
}

func TestMapSourceError(t *testing.T) {
	assertMaps(t, "mapSourceError", func(e error) error { return mapSourceError(e, 5) },
		[]string{
			codes.SourceConnectionNotFound, codes.BuildConnectionInUse, codes.SourceConnectionForbidden,
			codes.SourceConnectionSuspended, codes.SourceProviderError,
		})
}

func TestMapVolumeErrors(t *testing.T) {
	assertMaps(t, "mapVolumeCreateError", mapVolumeCreateError,
		[]string{
			codes.TargetAppAlreadyHasVolume, codes.AppMustHaveOneReplica, codes.AutoscalingWithVolume,
			codes.SizeBelowMinimum, codes.SizeAboveMaximum,
		})
	assertMaps(t, "mapVolumeAttachError", func(e error) error { return mapVolumeAttachError(e, "ready") },
		[]string{
			codes.TargetAppAlreadyHasVolume, codes.AppMustHaveOneReplica, codes.AutoscalingWithVolume,
			codes.VolumeResizing, codes.VolumeCreating, codes.VolumeDeleting,
		})
	assertMaps(t, "mapVolumeResizeError", func(e error) error { return mapVolumeResizeError(e, "ready", "vol") },
		[]string{
			codes.VolumeNotAttached, codes.AppMustHaveOneReplica, codes.AutoscalingWithVolume,
			codes.CannotShrinkVolume, codes.SizeBelowMinimum, codes.SizeAboveMaximum,
			codes.VolumeResizing, codes.VolumeCreating, codes.VolumeDeleting,
		})
	// mapVolumeBusyError directly.
	if got := mapVolumeBusyError(apiErr(codes.VolumeResizing), "resizing"); got == nil {
		t.Error("mapVolumeBusyError resizing")
	}
	if got := mapVolumeBusyError(apiErr("X"), "ready"); !errors.Is(got, got) {
		t.Error("mapVolumeBusyError passthrough")
	}
}

func TestMapRegistryRepoAndManifestError(t *testing.T) {
	assertMaps(t, "mapRegistryRepoError", func(e error) error { return mapRegistryRepoError(e, "repo") },
		[]string{codes.RegistryRepositoryNotFound, codes.RegistrySuspended, codes.AmbiguousName})
	assertMaps(t, "mapRegistryManifestError", func(e error) error { return mapRegistryManifestError(e, "tag", "repo") },
		[]string{codes.RegistryManifestNotFound, codes.RegistryRepositoryNotFound, codes.RegistrySuspended})
}

func TestMapVPSErrors(t *testing.T) {
	assertMaps(t, "mapVPSCatalogueError", mapVPSCatalogueError,
		[]string{codes.MissingRegion, codes.InvalidRegion})
	assertMaps(t, "mapVPSActionError", func(e error) error { return mapVPSActionError(e, "web", "stopped") },
		[]string{codes.InstanceExpired, codes.InstanceNotRunning, codes.ActionInProgress, codes.SSHNotReady})
	assertMaps(t, "mapVPSRentError", mapVPSRentError,
		[]string{codes.InsufficientBalance, codes.ProviderDisabled, codes.PlanDisabled, codes.MissingRegion, codes.InvalidRegion})
}

func TestMapAPIKeySessionError(t *testing.T) {
	if got := mapAPIKeySessionError(apiErr(codes.APIKeySessionRequired)); got == nil ||
		!strings.Contains(got.Error(), "session login") {
		t.Errorf("mapAPIKeySessionError = %v", got)
	}
	pass := apiErr("X")
	if got := mapAPIKeySessionError(pass); !errors.Is(got, pass) {
		t.Error("mapAPIKeySessionError passthrough")
	}
}
