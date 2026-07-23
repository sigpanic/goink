// Package update 提供 GitHub Release 更新检查功能。
package update

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/sigpanic/goink/internal/version"
)

const (
	repoOwner   = "sigpanic"
	repoName    = "goink"
	apiBaseURL  = "https://api.github.com/repos/" + repoOwner + "/" + repoName
	httpTimeout = 5 * time.Second
)

// ReleaseInfo 表示一个 GitHub Release 的简要信息。
type ReleaseInfo struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`     // markdown 格式的 release notes
	HTMLURL     string `json:"html_url"` // release 页面 URL
	PublishedAt string `json:"published_at"`
}

// CheckResult 是更新检查的结果。
type CheckResult struct {
	CurrentVersion string      `json:"currentVersion"`
	Latest         ReleaseInfo `json:"latest"`
	HasUpdate      bool        `json:"hasUpdate"`
}

// CheckLatest 检查 GitHub 上是否有新版本。
// 返回 CheckResult（无论是否有更新），网络/解析错误时返回 error。
// 前端根据场景决定是否展示错误。
func CheckLatest(logger *slog.Logger) (*CheckResult, error) {
	if logger == nil {
		logger = slog.Default()
	}

	current := strings.TrimPrefix(version.Version, "v")
	if current == "dev" || current == "" {
		current = "0.0.0" // dev 或空版本号用最低版本号，确保 release 能被检测到
	}

	client := &http.Client{Timeout: httpTimeout}
	url := apiBaseURL + "/releases/latest"

	logger.Debug("update: checking", "url", url)
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("update: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// 没有 release，不算错误
		logger.Debug("update: no release found")
		return &CheckResult{CurrentVersion: current, HasUpdate: false}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update: unexpected status %d", resp.StatusCode)
	}

	var release ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("update: decode failed: %w", err)
	}

	latest := strings.TrimPrefix(release.TagName, "v")
	hasUpdate := semverGreaterThan(latest, current)

	return &CheckResult{
		CurrentVersion: current,
		Latest:         release,
		HasUpdate:      hasUpdate,
	}, nil
}

// semverGreaterThan 比较两个语义版本号，如果 a > b 返回 true。
// 支持简单的 major.minor.patch 格式，忽略 pre-release 后缀。
func semverGreaterThan(a, b string) bool {
	a = stripPreRelease(a)
	b = stripPreRelease(b)

	aParts := splitVersion(a)
	bParts := splitVersion(b)

	maxLen := max(len(aParts), len(bParts))

	for i := range maxLen {
		av := versionPart(aParts, i)
		bv := versionPart(bParts, i)
		if av > bv {
			return true
		}
		if av < bv {
			return false
		}
	}
	return false
}

// stripPreRelease 去掉版本号中的 pre-release 后缀（如 "-rc.1"）。
func stripPreRelease(v string) string {
	base, _, ok := strings.Cut(v, "-")
	if ok {
		return base
	}
	return v
}

// splitVersion 将版本号按 "." 分割。
func splitVersion(v string) []string {
	if v == "" {
		return nil
	}
	return strings.Split(v, ".")
}

// versionPart 安全地获取版本号的第 i 段，解析为整数。
// 解析失败返回 -1（小于任何合法版本号段）。
func versionPart(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	var n int
	if _, err := fmt.Sscanf(parts[i], "%d", &n); err != nil {
		return -1
	}
	return n
}
