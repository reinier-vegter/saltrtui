package app

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"saltrtui/internal/release"
)

func TestUpdateNoticeOpensReviewOnlyWhenAvailable(t *testing.T) {
	m := New(fakeGateway{}, "master-a", true)
	updated := update(m, tea.KeyPressMsg{Code: 'U', Text: "U"})
	if updated.activeView != 0 {
		t.Fatal("U must not open review without an available update")
	}
	m.availableUpdate = "v1.2.4"
	cmd := m.openSelfUpdate()
	if cmd == nil || m.activeView != 8 || m.updatePhase != "inspecting" {
		t.Fatalf("openSelfUpdate did not begin inspection: phase=%s view=%d", m.updatePhase, m.activeView)
	}
}

func TestInstallationInspectedMovesToConfirmOrFailed(t *testing.T) {
	m := New(fakeGateway{}, "master-a", true)
	m.availableUpdate, m.activeView, m.updatePhase, m.updateGeneration = "v1.2.4", 8, "inspecting", 1
	updated := update(m, installationInspectedMsg{generation: 1, installation: release.Installation{Current: "/opt/saltrtui", Writable: false, Eligible: true, SudoAllowed: true}})
	if updated.updatePhase != "confirm" || updated.installation.Current != "/opt/saltrtui" {
		t.Fatalf("expected confirm phase with installation recorded: %#v", updated)
	}
	m.updatePhase = "inspecting"
	updated = update(m, installationInspectedMsg{generation: 1, err: errors.New("boom")})
	if updated.updatePhase != "failed" || !strings.Contains(updated.updateText, "boom") {
		t.Fatalf("expected failed phase on inspection error: %#v", updated)
	}
	// A stale generation must not retroactively affect a newer screen state.
	m.updatePhase, m.updateGeneration = "confirm", 2
	updated = update(m, installationInspectedMsg{generation: 1, installation: release.Installation{Writable: true}})
	if updated.updatePhase != "confirm" {
		t.Fatal("stale inspection result changed a newer phase")
	}
}

func TestConfirmChoicesAndEscCancelWithoutInstalling(t *testing.T) {
	cases := []struct {
		writable    bool
		index       int
		destination string
		sudo        bool
	}{
		{writable: true, index: 0, destination: "/opt/saltrtui", sudo: false},
		{writable: false, index: 0, destination: "/opt/saltrtui", sudo: true},
	}
	for _, tc := range cases {
		m := New(fakeGateway{}, "master-a", true)
		m.width, m.height = 100, 30
		m.availableUpdate, m.activeView, m.updatePhase, m.updateIndex = "v1.2.4", 8, "confirm", tc.index
		m.installation = release.Installation{Current: "/opt/saltrtui", Writable: tc.writable}
		updated := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
		if updated.updatePhase != "downloading" || updated.updateDestination != tc.destination || updated.updateSudo != tc.sudo {
			t.Fatalf("case %#v: phase=%s dest=%s sudo=%v", tc, updated.updatePhase, updated.updateDestination, updated.updateSudo)
		}
		if updated.updateCancel != nil {
			updated.updateCancel()
		}
	}

	m := New(fakeGateway{}, "master-a", true)
	m.width, m.height = 100, 30
	m.availableUpdate, m.activeView, m.updatePhase, m.updateIndex = "v1.2.4", 8, "confirm", 1
	m.installation = release.Installation{Writable: true}
	updated := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if updated.activeView != 0 || updated.updatePhase == "downloading" {
		t.Fatal("selecting Cancel must not start a download")
	}

	m = New(fakeGateway{}, "master-a", true)
	cancelled := false
	m.availableUpdate, m.activeView, m.updatePhase = "v1.2.4", 8, "downloading"
	m.updateCancel = func() { cancelled = true }
	updated = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if !cancelled || updated.activeView != 0 {
		t.Fatal("esc during download must cancel and return to Fleet")
	}
}

func TestStaleDownloadAndVerifyResultsAreIgnoredAfterCancellation(t *testing.T) {
	m := New(fakeGateway{}, "master-a", true)
	m.activeView, m.updatePhase, m.updateGeneration = 8, "downloading", 1
	updated := update(m, updateFetchedMsg{generation: 1, err: nil})
	if updated.updatePhase != "verifying" {
		t.Fatalf("expected verifying phase, got %s", updated.updatePhase)
	}
	// Cancellation bumps the generation; a late result for the old generation must not install.
	updated.updateGeneration++
	stale := update(updated, updateDownloadedMsg{generation: 1, binary: []byte("stale")})
	if stale.updatePhase == "installing" {
		t.Fatal("stale verification result must not begin installation")
	}
}

func TestDownloadAndVerificationFailuresPreserveTheOldExecutable(t *testing.T) {
	m := New(fakeGateway{}, "master-a", true)
	m.activeView, m.updatePhase, m.updateGeneration = 8, "downloading", 1
	updated := update(m, updateFetchedMsg{generation: 1, err: errors.New("network down")})
	if updated.updatePhase != "failed" || !strings.Contains(updated.updateText, "network down") {
		t.Fatalf("download failure not reported: %#v", updated)
	}

	m = New(fakeGateway{}, "master-a", true)
	m.activeView, m.updatePhase, m.updateGeneration = 8, "downloading", 1
	fetched := update(m, updateFetchedMsg{generation: 1})
	if fetched.updatePhase != "verifying" {
		t.Fatal("expected verifying phase")
	}
	failed := update(fetched, updateDownloadedMsg{generation: 1, err: errors.New("checksum mismatch")})
	if failed.updatePhase != "failed" || !strings.Contains(failed.updateText, "checksum mismatch") {
		t.Fatalf("verification failure not reported: %#v", failed)
	}
}

func TestInstallationCompletionClearsNoticeAndKeepsRunningVersion(t *testing.T) {
	m := New(fakeGateway{}, "master-a", true)
	m.version = "v1.2.3"
	m.availableUpdate, m.activeView, m.updatePhase = "v1.2.4", 8, "installing"
	m.updateDestination = "/opt/saltrtui"
	m.installation = release.Installation{Current: "/opt/saltrtui"}
	updated := update(m, updateInstalledMsg{})
	if !updated.updateInstalled || updated.availableUpdate != "" {
		t.Fatal("completion did not clear the notice")
	}
	if updated.version != "v1.2.3" {
		t.Fatal("completion must not change the running process version")
	}
	if !strings.Contains(updated.updateText, "Restart saltrtui") {
		t.Fatalf("completion must explain restart: %s", updated.updateText)
	}
	// A stale check result after completion must not resurrect the old notice.
	resurrected := update(updated, updateCheckSucceededMsg{latestVersion: "v1.2.4"})
	if resurrected.availableUpdate != "" {
		t.Fatal("old update notice returned after a completed install")
	}

	m = New(fakeGateway{}, "master-a", true)
	m.availableUpdate, m.activeView, m.updatePhase = "v1.2.4", 8, "installing"
	m.updateDestination = "/opt/saltrtui"
	failed := update(m, updateInstalledMsg{err: errors.New("sudo cancelled")})
	if failed.updatePhase != "failed" || failed.updateInstalled {
		t.Fatalf("installation failure not reported: %#v", failed)
	}
}

func TestInstallingPhaseBlocksNavigationUntilComplete(t *testing.T) {
	m := New(fakeGateway{}, "master-a", true)
	m.activeView, m.updatePhase = 8, "installing"
	updated := update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if updated.activeView != 8 {
		t.Fatal("installing phase must not be interrupted by Esc")
	}
}
