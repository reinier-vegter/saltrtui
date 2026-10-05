package saltcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"saltrtui/internal/keys"
)

var keyGroups = []struct {
	name  string
	state keys.State
}{
	{"minions_pre", keys.Pending},
	{"minions", keys.Accepted},
	{"minions_rejected", keys.Rejected},
	{"minions_denied", keys.Denied},
}

// Salt key selectors are globs, not literal IDs. Fail closed when a name
// cannot safely target just one key, including for metadata reads.
func literalKeyID(id string) bool {
	return id != "" && !strings.HasPrefix(id, "-") && !strings.ContainsAny(id, "*?[]\\/,\x00") &&
		id != "." && id != ".." && !strings.ContainsFunc(id, unicode.IsControl)
}

func (g *Gateway) ListKeys(ctx context.Context) ([]keys.Key, error) {
	data, err := g.invoke(ctx, "salt-key", "--list", "all", "--out=json", "--no-color")
	if err != nil {
		return nil, fmt.Errorf("key inventory: %w", err)
	}
	var raw map[string][]string
	if err := decode(data, &raw); err != nil || raw == nil {
		return nil, errors.New("key inventory: invalid Salt JSON")
	}
	items := []keys.Key{}
	for _, group := range keyGroups {
		seen := make(map[string]bool)
		ids, ok := raw[group.name]
		if !ok || ids == nil {
			return nil, fmt.Errorf("key inventory: missing %s group", group.name)
		}
		for _, id := range ids {
			if id == "" || strings.ContainsFunc(id, unicode.IsControl) || seen[id] {
				return nil, errors.New("key inventory: invalid or duplicate key ID")
			}
			seen[id] = true
			items = append(items, keys.Key{ID: id, State: group.state})
		}
	}
	keys.Sort(items)
	return items, nil
}

func (g *Gateway) ReadKey(ctx context.Context, key keys.Key) (keys.Detail, error) {
	if !literalKeyID(key.ID) {
		return keys.Detail{}, errors.New("key ID cannot be safely addressed literally")
	}
	detail := keys.Detail{}
	if key.State == keys.Pending {
		// This conventional path is only a best-effort hint. Custom pki_dir
		// installations may have no file here; never claim an announcement time.
		configDir := g.ConfigDir
		if configDir == "" {
			configDir = "/etc/salt"
		}
		path := filepath.Join(configDir, "pki", "master", "minions_pre", key.ID)
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
			detail.FileTime = info.ModTime()
		}
	}
	data, err := g.invoke(ctx, "salt-key", "--finger", key.ID, "--out=json", "--no-color")
	if err != nil {
		return detail, fmt.Errorf("key fingerprint: %w", err)
	}
	var raw map[string]map[string]string
	if err := decode(data, &raw); err != nil || raw == nil {
		return detail, errors.New("key fingerprint: invalid Salt JSON")
	}
	var fingerprint string
	for _, group := range keyGroups {
		for id, value := range raw[group.name] {
			if id != key.ID {
				return detail, errors.New("key fingerprint: unexpected key or state")
			}
			if group.state == key.State {
				fingerprint = value
			}
		}
	}
	if fingerprint == "" || strings.ContainsFunc(fingerprint, unicode.IsControl) {
		return detail, errors.New("key fingerprint: missing or invalid value")
	}
	detail.Fingerprint = fingerprint
	return detail, nil
}

func (g *Gateway) ChangeKey(ctx context.Context, key keys.Key, action keys.Action) error {
	if !literalKeyID(key.ID) || key.State != action.Expected() || (action != keys.Accept && action != keys.Block && action != keys.Revoke) {
		return errors.New("unsafe key action or state")
	}
	// Recheck against a fresh master snapshot, not the UI's possibly old row.
	items, err := g.ListKeys(ctx)
	if err != nil {
		return fmt.Errorf("key action not sent (recheck failed): %w", err)
	}
	found, matches := false, 0
	for _, item := range items {
		if item.ID == key.ID {
			matches++
		}
		if item.ID == key.ID && item.State == key.State {
			found = true
		}
	}
	if !found || matches != 1 {
		return errors.New("key action not sent: key state changed or ID occurs in multiple states; refresh inventory")
	}
	option := map[keys.Action]string{keys.Accept: "--accept", keys.Block: "--reject", keys.Revoke: "--delete"}[action]
	_, err = g.invoke(ctx, "salt-key", option, key.ID, "--yes", "--quiet")
	if err != nil {
		return fmt.Errorf("key action outcome unknown; refresh inventory: %w", err)
	}
	return nil
}

var _ keys.Gateway = (*Gateway)(nil)
