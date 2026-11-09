package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/devgianlu/go-librespot/session"
)

const (
	pathfinderEndpoint = "https://api-partner.spotify.com/pathfinder/v2/query"
	hashRegistryURL    = "https://billowing-resonance-da83.johnwatson.workers.dev/hashes"
)

var defaultPathfinderHashes = map[string]string{
	"getAlbum":            "0b6ce3511e1b02e4a40fa95440f992acbe85080eb308af9194322d708fc1914f",
	"queryArtistOverview": "eafb1620d783ece118e72fbfede2b0064483e029d3c1aafafa1c2de0e25a89e7",
	"getTrack":            "e7b1bf4e95cb3d733d48b442f93006e9712fd504e8b88d7a82e6e3bac9306a6b",
	"browseAll":           "dbd8b55e09a58afc52eab438bc228ba28fd72ac2f2148c6c26354980e4579001",
	"searchDesktop":       "eef7cc54888d91bdd6802623477873caa3948ae173a0c34fd86827b267e94c03",
	"searchAlbums":        "202cb3305e31e5a0767ba7925f28bd728cf8f8b0217e6da43909056071cd70e9",
	"searchPlaylists":     "d520014e748f9ea44f7707d8df1819867ac1205e8b7f3e28f22fe5fc858921b1",
	"browsePage":          "f5c4e6d668f5716464a231c1cc8b22c1cbf6ad68b09929fd7de813a30581298b",
}

type hashRegistry struct {
	Fetched    int64             `json:"fetched"`
	Operations map[string]string `json:"operations"`
}

type pathfinderEnvelope struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (p *Spotify) pathfinderQuery(ctx context.Context, sess *session.Session, operation string, variables any, target any) error {
	hash, fresh, err := p.pathfinderHash(ctx, operation, "")
	if err != nil {
		return err
	}
	err = p.sendPathfinder(ctx, sess, operation, hash, variables, target)
	if err == nil || fresh {
		return err
	}
	latest, _, refreshErr := p.pathfinderHash(ctx, operation, hash)
	if refreshErr != nil || latest == hash {
		return err
	}
	return p.sendPathfinder(ctx, sess, operation, latest, variables, target)
}

func (p *Spotify) sendPathfinder(ctx context.Context, sess *session.Session, operation, hash string, variables, target any) error {
	body, err := json.Marshal(map[string]any{
		"operationName": operation, "variables": variables,
		"extensions": map[string]any{"persistedQuery": map[string]any{"version": 1, "sha256Hash": hash}},
	})
	if err != nil {
		return fmt.Errorf("cannot encode %s Pathfinder request: %w", operation, err)
	}
	token, err := sess.Spclient().GetAccessToken(ctx, false)
	if err != nil {
		return fmt.Errorf("cannot obtain Spotify access token: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pathfinderEndpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("app-platform", "WebPlayer")
	req.Header.Set("spotify-app-version", "896000000")
	req.Header.Set("Authorization", "Bearer "+token)
	p.mu.RLock()
	clientToken := p.clientToken
	p.mu.RUnlock()
	if clientToken != "" {
		req.Header.Set("Client-Token", clientToken)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("cannot request %s from Pathfinder: %w", operation, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Spotify rejected %s Pathfinder query: HTTP %d", operation, resp.StatusCode)
	}
	var answer pathfinderEnvelope
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&answer); err != nil {
		return fmt.Errorf("cannot decode %s Pathfinder response: %w", operation, err)
	}
	if len(answer.Errors) > 0 {
		message := ""
		for _, item := range answer.Errors {
			if message != "" {
				message += "; "
			}
			message += item.Message
		}
		return fmt.Errorf("Spotify rejected %s Pathfinder query: %s", operation, message)
	}
	if len(answer.Data) == 0 || bytes.Equal(answer.Data, []byte("null")) {
		return fmt.Errorf("%s Pathfinder response has no data", operation)
	}
	if err := json.Unmarshal(answer.Data, target); err != nil {
		return fmt.Errorf("cannot decode %s Pathfinder response: %w", operation, err)
	}
	return nil
}

func (p *Spotify) pathfinderHash(ctx context.Context, operation, stale string) (string, bool, error) {
	p.hashMu.Lock()
	defer p.hashMu.Unlock()
	if p.hashes == nil {
		p.hashes = readHashRegistry(p.hashPath)
	}
	if stale == "" && p.hashes != nil && time.Since(time.Unix(p.hashes.Fetched, 0)) < 24*time.Hour {
		if hash := p.hashes.Operations[operation]; saneHash(hash) {
			return hash, false, nil
		}
	}
	operations, err := p.fetchHashes(ctx, stale)
	if err == nil {
		p.hashes = &hashRegistry{Fetched: time.Now().Unix(), Operations: operations}
		if body, marshalErr := json.MarshalIndent(p.hashes, "", "  "); marshalErr == nil {
			_ = atomicWrite(p.hashPath, append(body, '\n'), 0o600)
		}
		if hash := operations[operation]; saneHash(hash) {
			return hash, true, nil
		}
		return "", true, fmt.Errorf("the hash registry has no %s query", operation)
	}
	if p.hashes != nil {
		if hash := p.hashes.Operations[operation]; saneHash(hash) {
			return hash, false, nil
		}
	}
	if hash := defaultPathfinderHashes[operation]; hash != "" {
		return hash, false, nil
	}
	return "", true, err
}

func (p *Spotify) fetchHashes(ctx context.Context, stale string) (map[string]string, error) {
	endpoint := hashRegistryURL
	if stale != "" {
		endpoint += "?stale=" + url.QueryEscape(stale)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("Accept", "application/json")
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hash registry HTTP %d", resp.StatusCode)
	}
	var answer struct {
		Operations map[string]string `json:"operations"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&answer); err != nil {
		return nil, err
	}
	if len(answer.Operations) == 0 {
		return nil, fmt.Errorf("the query hash registry is empty")
	}
	for operation, hash := range answer.Operations {
		if !saneHash(hash) {
			return nil, fmt.Errorf("the %s query hash is malformed", operation)
		}
	}
	return answer.Operations, nil
}

func readHashRegistry(path string) *hashRegistry {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var registry hashRegistry
	if json.Unmarshal(body, &registry) != nil || len(registry.Operations) == 0 {
		return nil
	}
	return &registry
}

func saneHash(hash string) bool {
	if len(hash) != 64 {
		return false
	}
	for _, c := range hash {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
