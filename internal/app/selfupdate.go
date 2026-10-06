package app

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"saltrtui/internal/release"
	"saltrtui/internal/ui"
)

// updateCheckSucceededMsg reports a newer stable release discovered by the
// background check; it never triggers a download by itself.
type updateCheckSucceededMsg struct {
	latestVersion string
}

type installationInspectedMsg struct {
	generation   int
	installation release.Installation
	err          error
}

type updateFetchedMsg struct {
	generation int
	artifact   release.Artifact
	err        error
}

type updateDownloadedMsg struct {
	generation int
	binary     []byte
	err        error
}

type updateInstalledMsg struct {
	generation int
	err        error
}

// updateCheckCmd runs the rate-limited, notification-only release check.
// A missing store, network failure, or malformed response is never a
// startup failure: it simply yields no message.
func (m Model) updateCheckCmd() tea.Cmd {
	if m.updateStore == nil {
		return nil
	}
	version, store := m.version, m.updateStore
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result, err := release.CheckLatest(ctx, version, store)
		if err != nil || !result.Available {
			return nil
		}
		return updateCheckSucceededMsg{latestVersion: result.LatestVersion}
	}
}

// openSelfUpdate opens the dedicated review screen. U never downloads
// directly; it only begins inspecting the current installation.
func (m *Model) openSelfUpdate() tea.Cmd {
	if m.availableUpdate == "" {
		return nil
	}
	m.updateGeneration++
	generation := m.updateGeneration
	m.activeView = 8
	m.updatePhase, m.updateText, m.updateIndex = "inspecting", "Checking installation location…", 0
	return func() tea.Msg {
		installation, err := release.InspectInstallation()
		return installationInspectedMsg{generation: generation, installation: installation, err: err}
	}
}

// updateResult routes every self-update background message. It is checked
// early in Model.Update alongside the other feature result handlers.
func (m Model) updateResult(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch value := msg.(type) {
	case updateCheckSucceededMsg:
		if !m.updateInstalled {
			m.availableUpdate = value.latestVersion
		}
		return m, nil, true
	case installationInspectedMsg:
		updated, cmd := m.handleInstallationInspected(value)
		return updated, cmd, true
	case updateFetchedMsg:
		updated, cmd := m.handleUpdateFetched(value)
		return updated, cmd, true
	case updateDownloadedMsg:
		updated, cmd := m.handleUpdateDownloaded(value)
		return updated, cmd, true
	case updateInstalledMsg:
		updated, cmd := m.handleUpdateInstalled(value)
		return updated, cmd, true
	}
	return m, nil, false
}

func (m Model) handleInstallationInspected(msg installationInspectedMsg) (tea.Model, tea.Cmd) {
	if msg.generation != m.updateGeneration || m.updatePhase != "inspecting" {
		return m, nil
	}
	if msg.err != nil || !msg.installation.Eligible || (!msg.installation.Writable && !msg.installation.SudoAllowed) {
		if msg.err == nil && !msg.installation.SudoAllowed {
			msg.err = fmt.Errorf("the protected executable is not a trusted root-owned standalone installation")
		}
		if msg.err == nil {
			msg.err = fmt.Errorf("%s", msg.installation.Reason)
		}
		m.updatePhase, m.updateText = "failed", "Cannot inspect installation: "+msg.err.Error()
		return m, nil
	}
	m.installation = msg.installation
	m.updatePhase, m.updateText = "confirm", ""
	return m, nil
}

// updateSelfUpdate handles keys on the review/progress/completion screen.
// Enter only ever starts a download from the "confirm" phase; every other
// phase ignores it, and Esc/Cancel always leave without installing.
func (m Model) updateSelfUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	if m.updatePhase == "installing" {
		return m, nil
	}
	key := keyMsg.String()
	if key == "esc" || key == "q" || key == "ctrl+c" {
		if m.updateCancel != nil {
			m.updateCancel()
			m.updateCancel = nil
		}
		m.updateGeneration++
		m.activeView = 0
		if key == "q" || key == "ctrl+c" {
			m.stopEvents()
			return m, tea.Quit
		}
		return m, nil
	}
	if m.updatePhase != "confirm" {
		return m, nil
	}
	last := 1
	switch key {
	case "up", "k":
		if m.updateIndex > 0 {
			m.updateIndex--
		}
	case "down", "j":
		if m.updateIndex < last {
			m.updateIndex++
		}
	case "enter":
		if m.width < 60 || m.height < 12 {
			m.updateText = "Resize the terminal to review the full destination and cancellation controls before confirming."
			return m, nil
		}
		if m.updateIndex == last {
			m.activeView = 0
			return m, nil
		}
		m.updateDestination = m.installation.Current
		m.updateSudo = !m.installation.Writable
		m.updatePhase, m.updateText = "downloading", "Downloading release assets…"
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		m.updateCancel = cancel
		version, generation := m.availableUpdate, m.updateGeneration
		return m, func() tea.Msg {
			defer cancel()
			artifact, err := release.Fetch(ctx, version)
			return updateFetchedMsg{generation: generation, artifact: artifact, err: err}
		}
	}
	return m, nil
}

func (m Model) handleUpdateFetched(msg updateFetchedMsg) (tea.Model, tea.Cmd) {
	if msg.generation != m.updateGeneration || m.updatePhase != "downloading" {
		return m, nil
	}
	if msg.err != nil {
		m.updateCancel = nil
		m.updatePhase, m.updateText = "failed", "Download failed: "+msg.err.Error()+"\nCheck connectivity and release availability, then return and try again."
		return m, nil
	}
	m.updatePhase, m.updateText = "verifying", "Verifying SHA-256 and extracting the release…"
	generation := m.updateGeneration
	return m, func() tea.Msg {
		binary, err := release.Verify(msg.artifact)
		return updateDownloadedMsg{generation: generation, binary: binary, err: err}
	}
}

func (m Model) handleUpdateDownloaded(msg updateDownloadedMsg) (tea.Model, tea.Cmd) {
	if msg.generation != m.updateGeneration || m.updatePhase != "verifying" {
		return m, nil
	}
	m.updateCancel = nil
	if msg.err != nil {
		m.updatePhase, m.updateText = "failed", "Verification failed: "+msg.err.Error()+"\nThe executable was not replaced. Return and retry with a fresh download."
		return m, nil
	}
	m.updatePhase, m.updateText = "installing", "Installing verified release at "+m.updateDestination+"…"
	generation, destination := m.updateGeneration, m.updateDestination
	if m.updateSudo {
		command, err := release.SudoCommand(m.installation.Current, destination, m.installation.OriginalDigest, msg.binary)
		if err != nil {
			return m.handleUpdateInstalled(updateInstalledMsg{generation: generation, err: err})
		}
		return m, tea.ExecProcess(command, func(err error) tea.Msg {
			return updateInstalledMsg{generation: generation, err: err}
		})
	}
	return m, func() tea.Msg {
		return updateInstalledMsg{generation: generation, err: release.InstallChecked(destination, m.installation.OriginalDigest, msg.binary)}
	}
}

func (m Model) handleUpdateInstalled(msg updateInstalledMsg) (tea.Model, tea.Cmd) {
	if msg.generation != m.updateGeneration || m.updatePhase != "installing" {
		return m, nil
	}
	if msg.err != nil {
		if release.ReplacementCommitted(msg.err) {
			m.updatePhase = "done"
			m.updateText = fmt.Sprintf("Installed %s at %s, but durability confirmation failed: %v\nRestart saltrtui to use the new version; this process is still %s.", m.availableUpdate, m.updateDestination, msg.err, m.version)
			m.updateInstalled = true
			m.availableUpdate = ""
			return m, nil
		}
		m.updatePhase, m.updateText = "failed", "Installation failed or sudo cancelled: "+msg.err.Error()+"\nCheck destination permissions or retry the chosen installation method."
		return m, nil
	}
	m.updatePhase = "done"
	m.updateText = fmt.Sprintf("Installed %s at %s.\nRestart saltrtui to use the new version; this process is still %s.", m.availableUpdate, m.updateDestination, m.version)
	m.updateInstalled = true
	m.availableUpdate = ""
	return m, nil
}

func (m Model) updateViewData() ui.UpdateViewData {
	return ui.UpdateViewData{
		Width: m.width, Height: m.height, Context: m.context,
		Running: m.version, Available: m.availableUpdate, Index: m.updateIndex,
		Phase: m.updatePhase, Text: m.updateText,
		Current: m.installation.Current, Writable: m.installation.Writable,
	}
}
