// Package update discovers and installs signed-off Ryotunes package releases.
package update

import (
	"archive/tar"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/netx"
	"github.com/ryoku-dev/ryotunes/internal/version"
)

const (
	repositoryURL = "https://github.com/ryoku-dev/ryotunes"
	latestURL     = "https://api.github.com/repos/ryoku-dev/ryotunes/releases/latest"
	maxPackage    = int64(1 << 30)
	maxMetadata   = int64(1 << 20)
	maxChecksum   = int64(4096)
)

var installing atomic.Bool

var httpClient = netx.Client(0)

type manager uint8

const (
	managerNone manager = iota
	managerPacman
	managerXBPS
	managerDNF
)

type installation struct {
	manager manager
	version string
}

type release struct {
	TagName    string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Body       *string `json:"body"`
	Assets     []asset `json:"assets"`
}

type asset struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	URL  string `json:"browser_download_url"`
}

type semver struct {
	major uint64
	minor uint64
	patch uint64
}

// Check queries the latest GitHub release and reports whether this installation can update.
func Check(ctx context.Context) (model.UpdateInfo, error) {
	rel, err := fetchRelease(ctx)
	if err != nil {
		return model.UpdateInfo{}, err
	}
	installed, installErr := detectInstallation(ctx)
	var reason *string
	if installErr != nil {
		text := installErr.Error()
		reason = &text
	}
	info := releaseInfo(rel, version.Version, installed.manager, reason)
	if info.Available && info.CanInstall && installed.version != "" && info.LatestVersion != nil {
		if current, ok := parseVersion(installed.version); ok {
			if latest, valid := parseVersion(*info.LatestVersion); valid && compareVersion(current, latest) >= 0 {
				info.RestartPending = true
			}
		}
	}
	return info, nil
}

// Install downloads, verifies, and installs the selected latest release package.
func Install(ctx context.Context, requested string) (model.UpdateResult, error) {
	if _, ok := parseVersion(requested); !ok {
		return model.UpdateResult{}, errors.New("Select a valid v1 release.")
	}
	if !installing.CompareAndSwap(false, true) {
		return model.UpdateResult{}, errors.New("An update is already in progress.")
	}
	defer installing.Store(false)

	rel, err := fetchRelease(ctx)
	if err != nil {
		return model.UpdateResult{}, err
	}
	installed, installErr := detectInstallation(ctx)
	var reason *string
	if installErr != nil {
		text := installErr.Error()
		reason = &text
	}
	info := releaseInfo(rel, version.Version, installed.manager, reason)
	if info.LatestVersion == nil || *info.LatestVersion != requested {
		return model.UpdateResult{}, errors.New("The latest release changed. Check for new versions again before updating.")
	}
	if !info.Available {
		return model.UpdateResult{}, errors.New("This version is already running, or a newer version is installed.")
	}
	if !info.CanInstall || installErr != nil {
		if info.UnsupportedReason != nil {
			return model.UpdateResult{}, errors.New(*info.UnsupportedReason)
		}
		return model.UpdateResult{}, errors.New("Self update is not available for this installation.")
	}
	if installed.version != "" {
		current, currentOK := parseVersion(installed.version)
		wanted, _ := parseVersion(requested)
		if currentOK && compareVersion(current, wanted) > 0 {
			return model.UpdateResult{}, errors.New("A newer package is already installed. Quit Ryotunes and open it again.")
		}
		if installed.version == requested {
			return model.UpdateResult{Version: requested, RestartRequired: true}, nil
		}
	}

	staging, err := os.MkdirTemp("", "ryotunes-update-")
	if err != nil {
		return model.UpdateResult{}, fmt.Errorf("Could not create private update staging: %v", err)
	}
	defer os.RemoveAll(staging)
	if err := os.Chmod(staging, 0o700); err != nil {
		return model.UpdateResult{}, fmt.Errorf("Could not create private update staging: %v", err)
	}

	pkg, checksumAsset, ok := selectAssets(rel, requested, installed.manager)
	if !ok {
		return model.UpdateResult{}, errors.New("This release has no complete x86_64 package and checksum. View the changelog or check again after the release build finishes.")
	}
	expected, err := fetchChecksum(ctx, checksumAsset, pkg.Name)
	if err != nil {
		return model.UpdateResult{}, err
	}
	packagePath := filepath.Join(staging, pkg.Name)
	if err := downloadPackage(ctx, pkg, packagePath, expected); err != nil {
		return model.UpdateResult{}, err
	}

	switch installed.manager {
	case managerPacman:
		err = installPacman(ctx, packagePath, requested)
	case managerXBPS:
		err = installXBPS(ctx, packagePath, staging, requested)
	case managerDNF:
		err = installDNF(ctx, packagePath, requested)
	default:
		err = errors.New("Self update is not available for this installation.")
	}
	if err != nil {
		return model.UpdateResult{}, err
	}
	return model.UpdateResult{Version: requested, RestartRequired: true}, nil
}

func fetchRelease(ctx context.Context) (*release, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, latestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("Could not contact GitHub: %v", err)
	}
	setHeaders(request)
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Could not contact GitHub: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("GitHub request failed: %s", response.Status)
	}
	body, err := readBounded(response.Body, maxMetadata)
	if err != nil {
		return nil, err
	}
	var rel release
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, fmt.Errorf("Invalid GitHub release metadata: %v", err)
	}
	return &rel, nil
}

func releaseInfo(rel *release, current string, mgr manager, reason *string) model.UpdateInfo {
	info := model.UpdateInfo{
		CurrentVersion:    current,
		ReleaseURL:        repositoryURL + "/releases",
		UnsupportedReason: reason,
	}
	if rel == nil || rel.Draft || rel.Prerelease || !strings.HasPrefix(rel.TagName, "v") {
		return info
	}
	latest := strings.TrimPrefix(rel.TagName, "v")
	next, ok := parseVersion(latest)
	if !ok {
		return info
	}
	info.LatestVersion = &latest
	if installed, valid := parseVersion(current); !valid || compareVersion(next, installed) > 0 {
		info.Available = true
	}
	info.ReleaseURL = repositoryURL + "/releases/tag/v" + latest
	if rel.Body != nil {
		info.Notes = *rel.Body
	}
	_, _, complete := selectAssets(rel, latest, mgr)
	if !complete && info.UnsupportedReason == nil {
		text := "This release has no complete x86_64 package and checksum. View the changelog or check again after the release build finishes."
		if mgr == managerPacman || mgr == managerXBPS || mgr == managerNone {
			text = "This release has no complete x86_64 Arch package and checksum. View the changelog or check again after the release build finishes."
		}
		info.UnsupportedReason = &text
	}
	info.CanInstall = info.UnsupportedReason == nil && complete
	return info
}

func selectAssets(rel *release, version string, mgr manager) (asset, asset, bool) {
	if rel == nil {
		return asset{}, asset{}, false
	}
	name := packageName(version, mgr)
	checksumName := name + ".sha256"
	var pkg, sum asset
	for _, candidate := range rel.Assets {
		switch candidate.Name {
		case name:
			if candidate.Size > 0 && candidate.Size <= maxPackage {
				pkg = candidate
			}
		case checksumName:
			if candidate.Size > 0 && candidate.Size <= maxChecksum {
				sum = candidate
			}
		}
	}
	return pkg, sum, pkg.Name != "" && sum.Name != ""
}

func packageName(version string, mgr manager) string {
	if mgr == managerDNF {
		return "ryotunes-" + version + "-1.x86_64.rpm"
	}
	return "ryotunes-" + version + "-1-x86_64.pkg.tar.zst"
}

func parseVersion(raw string) (semver, bool) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 || parts[0] != "1" {
		return semver{}, false
	}
	if len(parts[1]) > 1 && parts[1][0] == '0' || len(parts[2]) != 1 {
		return semver{}, false
	}
	minor, errMinor := strconv.ParseUint(parts[1], 10, 64)
	patch, errPatch := strconv.ParseUint(parts[2], 10, 64)
	if errMinor != nil || errPatch != nil {
		return semver{}, false
	}
	return semver{major: 1, minor: minor, patch: patch}, true
}

func compareVersion(left, right semver) int {
	switch {
	case left.major != right.major:
		if left.major < right.major {
			return -1
		}
	case left.minor != right.minor:
		if left.minor < right.minor {
			return -1
		}
	case left.patch != right.patch:
		if left.patch < right.patch {
			return -1
		}
	default:
		return 0
	}
	return 1
}

func detectInstallation(ctx context.Context) (installation, error) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return installation{}, errors.New("Self update supports the x86_64 Arch, Void Linux, and Fedora packages. Download a release for your platform instead.")
	}
	mgr := managerNone
	switch {
	case regularFile("/usr/bin/pacman"):
		mgr = managerPacman
	case regularFile("/usr/bin/xbps-install"):
		mgr = managerXBPS
	case regularFile("/usr/bin/dnf5") || regularFile("/usr/bin/dnf"):
		mgr = managerDNF
	default:
		return installation{}, errors.New("Self update needs pacman, xbps, or dnf.")
	}
	if !regularFile("/usr/bin/pkexec") {
		return installation{}, errors.New("Self update needs polkit (pkexec), with a desktop authentication agent.")
	}
	executable, err := os.Executable()
	if err != nil {
		return installation{}, fmt.Errorf("Cannot determine the running installation: %v", err)
	}
	executable = strings.TrimSuffix(executable, " (deleted)")
	version, err := ownedVersion(ctx, mgr, executable)
	if err != nil {
		return installation{}, errors.New("This copy of Ryotunes was not installed from its package, so it cannot update itself. Install the package from the release page; development builds are never overwritten.")
	}
	return installation{manager: mgr, version: version}, nil
}

func ownedVersion(ctx context.Context, mgr manager, executable string) (string, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	switch mgr {
	case managerPacman:
		owner, err := commandOutput(commandCtx, "/usr/bin/pacman", "-Qoq", executable)
		if err != nil || string(owner) != "ryotunes\n" {
			return "", errors.New("not owned")
		}
		info, err := commandOutput(commandCtx, "/usr/bin/pacman", "-Qi", "ryotunes")
		if err != nil {
			return "", err
		}
		return pacmanInfoVersion(string(info))
	case managerXBPS:
		owner, err := commandOutput(commandCtx, "/usr/bin/xbps-query", "-o", executable)
		if err != nil {
			return "", err
		}
		prefix := strings.SplitN(strings.TrimSpace(string(owner)), ": ", 2)[0]
		return xbpsVersion(prefix)
	case managerDNF:
		owner, err := commandOutput(commandCtx, "/usr/bin/rpm", "-qf", "--qf", "%{NAME}\n", executable)
		if err != nil || string(owner) != "ryotunes\n" {
			return "", errors.New("not owned")
		}
		value, err := commandOutput(commandCtx, "/usr/bin/rpm", "-q", "--qf", "%{VERSION}\n", "ryotunes")
		if err != nil {
			return "", err
		}
		version := strings.TrimSpace(string(value))
		if _, ok := parseVersion(version); !ok {
			return "", errors.New("invalid package version")
		}
		return version, nil
	default:
		return "", errors.New("unsupported manager")
	}
}

func pacmanInfoVersion(info string) (string, error) {
	fields := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(info))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if ok {
			fields[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	if fields["Name"] != "ryotunes" {
		return "", errors.New("unexpected package")
	}
	value := strings.TrimPrefix(fields["Version"], "1:")
	version, _, ok := strings.Cut(value, "-")
	if !ok {
		return "", errors.New("invalid package version")
	}
	if _, valid := parseVersion(version); !valid {
		return "", errors.New("invalid package version")
	}
	// Official release and Ryoku-repository packages share the same URL; this rejects unrelated
	// packages that happen to claim the ryotunes name while accepting either delivery origin.
	url := strings.ToLower(fields["URL"])
	packager := strings.ToLower(fields["Packager"])
	if !strings.Contains(url, "ryoku-dev/ryotunes") && !strings.Contains(packager, "ryoku") {
		return "", errors.New("untrusted package origin")
	}
	return version, nil
}

func xbpsVersion(pkgver string) (string, error) {
	value := strings.TrimPrefix(strings.TrimSpace(pkgver), "ryotunes-")
	version, revision, ok := strings.Cut(value, "_")
	if !ok || version == value || revision == "" {
		return "", errors.New("invalid xbps package")
	}
	if _, err := strconv.ParseUint(revision, 10, 64); err != nil {
		return "", errors.New("invalid xbps package")
	}
	if _, valid := parseVersion(version); !valid {
		return "", errors.New("invalid xbps package")
	}
	return version, nil
}

func fetchChecksum(ctx context.Context, sum asset, packageFile string) (string, error) {
	body, err := fetchAsset(ctx, sum, maxChecksum, 30*time.Second)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(body))
	if len(fields) != 2 || fields[1] != packageFile || len(fields[0]) != 64 {
		return "", errors.New("Invalid package checksum: expected one SHA-256 entry for this release package.")
	}
	if _, err := hex.DecodeString(fields[0]); err != nil {
		return "", errors.New("Invalid package checksum: expected one SHA-256 entry for this release package.")
	}
	return strings.ToLower(fields[0]), nil
}

func fetchAsset(ctx context.Context, item asset, limit int64, timeout time.Duration) ([]byte, error) {
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	url := item.URL
	if url == "" {
		return nil, errors.New("GitHub release asset has no download URL.")
	}
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("Could not contact GitHub: %v", err)
	}
	setHeaders(request)
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Could not contact GitHub: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("GitHub request failed: %s", response.Status)
	}
	return readBounded(response.Body, limit)
}

func downloadPackage(ctx context.Context, item asset, destination, expected string) error {
	requestCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, item.URL, nil)
	if err != nil {
		return fmt.Errorf("Could not contact GitHub: %v", err)
	}
	setHeaders(request)
	response, err := httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("Could not contact GitHub: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Package download failed: %s", response.Status)
	}
	if response.ContentLength > maxPackage {
		return errors.New("Release package is too large.")
	}
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("Could not stage update: %v", err)
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, maxPackage+1))
	syncErr := file.Sync()
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("Package download interrupted: %v", copyErr)
	}
	if written > maxPackage {
		return errors.New("Release package is too large.")
	}
	if syncErr != nil {
		return fmt.Errorf("Could not save update: %v", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("Could not save update: %v", closeErr)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if written == 0 || actual != expected {
		return errors.New("Package SHA-256 verification failed. Nothing was installed.")
	}
	return nil
}

func installPacman(ctx context.Context, packagePath, requested string) error {
	inspectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	output, err := commandOutput(inspectCtx, "/usr/bin/pacman", "-Qp", packagePath)
	if err != nil || string(output) != "ryotunes 1:"+requested+"-1\n" {
		return errors.New("Downloaded package identity does not match the selected Ryotunes release.")
	}
	return privilegedInstall("pacman", "/usr/bin/pacman", "-U", "--noconfirm", packagePath)
}

func installDNF(ctx context.Context, packagePath, requested string) error {
	inspectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	identity, err := commandOutput(inspectCtx, "/usr/bin/rpm", "-qp", "--qf", "%{NAME} %{VERSION}-%{RELEASE} %{ARCH}\n", packagePath)
	if err != nil || string(identity) != "ryotunes "+requested+"-1 x86_64\n" {
		return errors.New("Downloaded package identity does not match the selected Ryotunes release.")
	}
	dnf := "/usr/bin/dnf"
	if regularFile("/usr/bin/dnf5") {
		dnf = "/usr/bin/dnf5"
	}
	return privilegedInstall("dnf", dnf, "install", "-y", packagePath)
}

func installXBPS(ctx context.Context, packagePath, staging, requested string) error {
	root := filepath.Join(staging, "root")
	repo := filepath.Join(staging, "repo")
	if err := unpackArchPackage(packagePath, root, requested); err != nil {
		return err
	}
	if err := checkGlibc(ctx, filepath.Join(root, "usr", "bin")); err != nil {
		return err
	}
	if err := os.Mkdir(repo, 0o700); err != nil {
		return fmt.Errorf("Could not stage update: %v", err)
	}
	pkgver := "ryotunes-" + requested + "_1"
	if err := runBounded(ctx, 5*time.Minute, "build the xbps package", "/usr/bin/xbps-create", repo,
		"-q", "-A", "x86_64", "-n", pkgver, "-l", "GPL-3.0-or-later", "-H", repositoryURL,
		"-s", "Ryoku music client, playback daemon, and sync server", "-D", voidDependencies(), root); err != nil {
		return err
	}
	packageFile := filepath.Join(repo, pkgver+".x86_64.xbps")
	if err := runBounded(ctx, 5*time.Minute, "index the xbps package", "/usr/bin/xbps-rindex", "", "-a", packageFile); err != nil {
		return err
	}
	return privilegedInstall("xbps", "/usr/bin/xbps-install", "-S", "-y", "-u", "--repository="+repo, "ryotunes")
}

func unpackArchPackage(packagePath, root, requested string) error {
	file, err := os.Open(packagePath)
	if err != nil {
		return fmt.Errorf("Could not unpack the package: %v", err)
	}
	defer file.Close()
	decoder, err := zstd.NewReader(file)
	if err != nil {
		return fmt.Errorf("Could not unpack the package: %v", err)
	}
	defer decoder.Close()
	if err := os.Mkdir(root, 0o700); err != nil {
		return fmt.Errorf("Could not unpack the package: %v", err)
	}
	archive := tar.NewReader(decoder)
	pkginfo := ""
	for {
		header, nextErr := archive.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return fmt.Errorf("Could not unpack the package: %v", nextErr)
		}
		name := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(header.Name)), "./")
		if name == ".PKGINFO" {
			body, readErr := readBounded(archive, 64<<10)
			if readErr != nil {
				return fmt.Errorf("Could not unpack the package: %v", readErr)
			}
			pkginfo = string(body)
			continue
		}
		if name != "usr" && !strings.HasPrefix(name, "usr/") || strings.HasPrefix(name, "usr/lib/systemd") {
			continue
		}
		if strings.Contains(name, "../") || filepath.IsAbs(name) {
			return errors.New("The package contains a path outside its install root.")
		}
		destination := filepath.Join(root, filepath.FromSlash(name))
		if !withinRoot(root, destination) {
			return errors.New("The package contains a path outside its install root.")
		}
		if err := unpackEntry(archive, header, root, destination); err != nil {
			return fmt.Errorf("Could not unpack the package: %v", err)
		}
	}
	if !pkginfoMatches(pkginfo, requested) {
		return errors.New("Downloaded package identity does not match the selected Ryotunes release.")
	}
	return nil
}

func unpackEntry(reader io.Reader, header *tar.Header, root, destination string) error {
	mode := os.FileMode(header.Mode) & 0o777
	switch header.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(destination, mode)
	case tar.TypeReg, tar.TypeRegA:
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(file, reader)
		closeErr := file.Close()
		return errors.Join(copyErr, closeErr)
	case tar.TypeSymlink:
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		return os.Symlink(header.Linkname, destination)
	case tar.TypeLink:
		source := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(filepath.Clean(header.Linkname), "./")))
		if !withinRoot(root, source) {
			return errors.New("hard link escapes install root")
		}
		return os.Link(source, destination)
	default:
		return nil
	}
}

func pkginfoMatches(info, requested string) bool {
	fields := map[string]string{}
	for _, line := range strings.Split(info, "\n") {
		key, value, ok := strings.Cut(line, " = ")
		if ok {
			fields[key] = value
		}
	}
	return fields["pkgname"] == "ryotunes" && fields["pkgver"] == "1:"+requested+"-1" && fields["arch"] == "x86_64"
}

func checkGlibc(ctx context.Context, binDir string) error {
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return fmt.Errorf("Could not check the package: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		command := exec.CommandContext(checkCtx, "/usr/bin/ldd", filepath.Join(binDir, entry.Name()))
		output, commandErr := command.CombinedOutput()
		cancel()
		if checkCtx.Err() == context.DeadlineExceeded {
			return errors.New("Timed out checking the package binaries.")
		}
		if commandErr != nil && len(output) == 0 {
			return fmt.Errorf("Could not check the package binaries: %v", commandErr)
		}
		if strings.Contains(string(output), "version `GLIBC_") && strings.Contains(string(output), "not found") {
			return fmt.Errorf("This release needs a newer glibc than this system has (%s). Nothing was installed.", entry.Name())
		}
	}
	return nil
}

func privilegedInstall(kind, executable string, args ...string) error {
	commandArgs := append([]string{executable}, args...)
	command := exec.Command("/usr/bin/pkexec", commandArgs...)
	command.Stdin = nil
	output, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	if command.ProcessState == nil {
		return fmt.Errorf("Could not start the administrator approval: %v", err)
	}
	return fmt.Errorf("Update was cancelled or %s could not install the package (%s). %s", kind, command.ProcessState.String(), strings.TrimSpace(string(output)))
}

func runBounded(parent context.Context, timeout time.Duration, what, executable, dir string, args ...string) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	command := exec.CommandContext(ctx, executable, args...)
	if dir != "" {
		command.Dir = dir
	}
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("Timed out trying to %s.", what)
	}
	if err != nil {
		return fmt.Errorf("Could not %s: %s", what, strings.TrimSpace(string(output)))
	}
	return nil
}

func commandOutput(ctx context.Context, executable string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, executable, args...)
	command.Env = append(os.Environ(), "LC_ALL=C")
	return command.Output()
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("GitHub response exceeded the allowed size.")
	}
	return body, nil
}

func setHeaders(request *http.Request) {
	request.Header.Set("User-Agent", "Ryotunes/"+version.Version)
	request.Header.Set("Accept", "application/vnd.github+json")
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func withinRoot(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func voidDependencies() string {
	return strings.Join([]string{
		"desktop-file-utils>=0", "ffmpeg6>=0", "gtk+3>=0", "hicolor-icon-theme>=0",
		"libayatana-appindicator>=0", "libwebkit2gtk41>=0", "librsvg>=0", "mpv>=0",
		"openssl>=0", "polkit>=0", "quickshell>=0", "qt6-plugin-tls-openssl>=0",
		"qt6-imageformats>=0", "xdg-utils>=0", "yt-dlp>=0",
	}, " ")
}
