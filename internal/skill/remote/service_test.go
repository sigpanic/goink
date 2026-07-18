package remote

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"novel/internal/githubapi"
)

// mockFetcher 实现 rawContentFetcher 接口，按 path 返回预设内容或错误。
type mockFetcher struct {
	content map[string][]byte // path -> content
	err     map[string]error  // path -> error
	calls   []string          // 记录调用路径，便于断言
}

func (m *mockFetcher) GetRawContent(ctx context.Context, owner, repo, branch, path string) ([]byte, *githubapi.RateLimit, error) {
	m.calls = append(m.calls, path)
	if e, ok := m.err[path]; ok {
		return nil, nil, e
	}
	if c, ok := m.content[path]; ok {
		return c, nil, nil
	}
	return nil, nil, &githubapi.Error{Kind: githubapi.KindNotFound, Status: 404, Message: "not found"}
}

// mockReloader 实现 skillReloader 接口，记录调用状态以便断言。
type mockReloader struct {
	userReloaded  bool
	novelReloaded map[int64]bool
	userErr       error
	novelErr      error
	userCalls     []string
	novelCalls    []struct {
		id  int64
		dir string
	}
}

func (m *mockReloader) ReloadUser(dir string) error {
	m.userReloaded = true
	m.userCalls = append(m.userCalls, dir)
	return m.userErr
}

func (m *mockReloader) ReloadNovel(id int64, dir string) error {
	if m.novelReloaded == nil {
		m.novelReloaded = map[int64]bool{}
	}
	m.novelReloaded[id] = true
	m.novelCalls = append(m.novelCalls, struct {
		id  int64
		dir string
	}{id: id, dir: dir})
	return m.novelErr
}

// newTestService 构造测试用 Service，注入 mock fetcher、mock reloader 和临时 skill 目录。
// dirResolver 默认指向 t.TempDir()，避免污染真实文件系统。
func newTestService(t *testing.T, fetcher *mockFetcher, reloader *mockReloader) (*Service, *mockFetcher, *mockReloader, string) {
	t.Helper()
	if fetcher == nil {
		fetcher = &mockFetcher{content: map[string][]byte{}, err: map[string]error{}}
	}
	if reloader == nil {
		reloader = &mockReloader{}
	}
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "skills")
	// dirResolver 总是返回同一个临时 skill 目录，target=novel 时也用同一目录避免污染。
	dirResolver := func(target string, novelID int64) (string, error) {
		switch target {
		case "user":
			return skillDir, nil
		case "novel":
			if novelID == 0 {
				return "", errors.New("remote: install to novel layer requires non-zero novelID")
			}
			return skillDir, nil
		default:
			return "", errors.New("remote: invalid target \"invalid\" (want user or novel)")
		}
	}
	svc := newServiceWithClient(fetcher, reloader, nil, dirResolver)
	return svc, fetcher, reloader, skillDir
}

// sampleIndex 构造测试用的 index.json 字节。
func sampleIndex() []byte {
	return []byte(`{
  "updated": "2026-07-18T15:06:02Z",
  "skills": [
    {
      "name": "my-skill",
      "description": "简要描述",
      "category": "分类",
      "mode": "auto",
      "author": "作者",
      "version": 1,
      "file": "my-skill.md"
    }
  ]
}`)
}

// TestListRemoteSkills_FromCache 第一次调用写入内存缓存，第二次调用命中缓存不调 client。
func TestListRemoteSkills_FromCache(t *testing.T) {
	fetcher := &mockFetcher{
		content: map[string][]byte{indexPath: sampleIndex()},
		err:     map[string]error{},
	}
	reloader := &mockReloader{}
	svc := newServiceWithClient(fetcher, reloader, nil, defaultDirResolver)

	// 第一次调用：无缓存，调 client，写入内存缓存。
	_, err := svc.ListRemoteSkills(context.Background(), false)
	require.NoError(t, err)
	require.Len(t, fetcher.calls, 1)

	// 第二次调用：内存缓存命中，不调 client。
	fetcher.calls = nil
	skills, err := svc.ListRemoteSkills(context.Background(), false)
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "my-skill", skills[0].Name)
	assert.Equal(t, "auto", skills[0].Mode)
	assert.Equal(t, 1, skills[0].Version)
	assert.Empty(t, fetcher.calls)
}

// TestListRemoteSkills_ForceRefresh 先写入内存缓存，再 forceRefresh=true 跳过缓存调 client。
func TestListRemoteSkills_ForceRefresh(t *testing.T) {
	// 第一次调用：写入内存缓存（用 sampleIndex）。
	fetcher := &mockFetcher{
		content: map[string][]byte{indexPath: sampleIndex()},
		err:     map[string]error{},
	}
	reloader := &mockReloader{}
	svc := newServiceWithClient(fetcher, reloader, nil, defaultDirResolver)

	_, err := svc.ListRemoteSkills(context.Background(), false)
	require.NoError(t, err)

	// 第二次调用：forceRefresh=true，跳过缓存，调 client 拉新数据。
	fetcher.calls = nil
	fetcher.content[indexPath] = []byte(`{"updated":"2026-07-18T00:00:00Z","skills":[{"name":"fresh","description":"d","category":"c","mode":"auto","author":"a","version":2,"file":"fresh.md"}]}`)

	skills, err := svc.ListRemoteSkills(context.Background(), true)
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "fresh", skills[0].Name)
	assert.Equal(t, 2, skills[0].Version)
	require.Len(t, fetcher.calls, 1)
	assert.Equal(t, indexPath, fetcher.calls[0])
}

// TestListRemoteSkills_NoCacheFetches 无缓存时调 client，解析 JSON，写入内存缓存。
// 再次调用应命中内存缓存，不调 client。
func TestListRemoteSkills_NoCacheFetches(t *testing.T) {
	fetcher := &mockFetcher{
		content: map[string][]byte{indexPath: sampleIndex()},
		err:     map[string]error{},
	}
	reloader := &mockReloader{}
	svc := newServiceWithClient(fetcher, reloader, nil, defaultDirResolver)

	// 第一次调用：无缓存，调 client，写入内存缓存。
	skills, err := svc.ListRemoteSkills(context.Background(), false)
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "my-skill", skills[0].Name)
	require.Len(t, fetcher.calls, 1)

	// 第二次调用：内存缓存命中，不调 client。
	fetcher.calls = nil
	skills2, err := svc.ListRemoteSkills(context.Background(), false)
	require.NoError(t, err)
	require.Len(t, skills2, 1)
	assert.Empty(t, fetcher.calls)
}

// TestListRemoteSkills_NetworkError client 返回 KindNetwork 错误，验证错误透传。
func TestListRemoteSkills_NetworkError(t *testing.T) {
	netErr := &githubapi.Error{Kind: githubapi.KindNetwork, Message: "timeout"}
	fetcher := &mockFetcher{
		content: map[string][]byte{},
		err:     map[string]error{indexPath: netErr},
	}
	reloader := &mockReloader{}
	svc := newServiceWithClient(fetcher, reloader, nil, defaultDirResolver)

	_, err := svc.ListRemoteSkills(context.Background(), false)
	require.Error(t, err)
	var apiErr *githubapi.Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, githubapi.KindNetwork, apiErr.Kind)
}

// TestListRemoteSkills_RateLimited client 返回 KindRateLimited，验证透传。
func TestListRemoteSkills_RateLimited(t *testing.T) {
	rlErr := &githubapi.Error{Kind: githubapi.KindRateLimited, Status: 403, Message: "rate limited"}
	fetcher := &mockFetcher{
		content: map[string][]byte{},
		err:     map[string]error{indexPath: rlErr},
	}
	reloader := &mockReloader{}
	svc := newServiceWithClient(fetcher, reloader, nil, defaultDirResolver)

	_, err := svc.ListRemoteSkills(context.Background(), false)
	require.Error(t, err)
	var apiErr *githubapi.Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, githubapi.KindRateLimited, apiErr.Kind)
}

// TestListRemoteSkills_InvalidJSON client 返回非法 JSON，验证解析错误。
func TestListRemoteSkills_InvalidJSON(t *testing.T) {
	fetcher := &mockFetcher{
		content: map[string][]byte{indexPath: []byte("not json {")},
		err:     map[string]error{},
	}
	reloader := &mockReloader{}
	svc := newServiceWithClient(fetcher, reloader, nil, defaultDirResolver)

	_, err := svc.ListRemoteSkills(context.Background(), false)
	require.Error(t, err)
	// 非法 JSON 不应被识别为 githubapi.Error
	var apiErr *githubapi.Error
	assert.False(t, errors.As(err, &apiErr))
}

// TestGetRemoteSkillContent_OK 正常返回内容。
func TestGetRemoteSkillContent_OK(t *testing.T) {
	fetcher := &mockFetcher{
		content: map[string][]byte{"skills/my-skill.md": []byte("# my skill\n---\nbody")},
		err:     map[string]error{},
	}
	svc, _, _, _ := newTestService(t, fetcher, nil)

	content, err := svc.GetRemoteSkillContent(context.Background(), "my-skill")
	require.NoError(t, err)
	assert.Equal(t, "# my skill\n---\nbody", content)
	require.Len(t, fetcher.calls, 1)
	assert.Equal(t, "skills/my-skill.md", fetcher.calls[0])
}

// TestGetRemoteSkillContent_NotFound client 返回 404，验证透传。
func TestGetRemoteSkillContent_NotFound(t *testing.T) {
	fetcher := &mockFetcher{
		content: map[string][]byte{},
		err:     map[string]error{},
	}
	svc, _, _, _ := newTestService(t, fetcher, nil)

	_, err := svc.GetRemoteSkillContent(context.Background(), "missing")
	require.Error(t, err)
	var apiErr *githubapi.Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, githubapi.KindNotFound, apiErr.Kind)
}

// TestInstallRemoteSkill_UserLayer target=user，验证文件写入 + ReloadUser 被调用。
func TestInstallRemoteSkill_UserLayer(t *testing.T) {
	fetcher := &mockFetcher{
		content: map[string][]byte{"skills/my-skill.md": []byte("# my skill content")},
		err:     map[string]error{},
	}
	reloader := &mockReloader{}
	svc, _, _, skillDir := newTestService(t, fetcher, reloader)

	err := svc.InstallRemoteSkill(context.Background(), "my-skill", "user", 0)
	require.NoError(t, err)

	// 验证文件已写入
	written, err := os.ReadFile(filepath.Join(skillDir, "my-skill.md"))
	require.NoError(t, err)
	assert.Equal(t, "# my skill content", string(written))

	// 验证 ReloadUser 被调用
	assert.True(t, reloader.userReloaded)
	require.Len(t, reloader.userCalls, 1)
	assert.Equal(t, skillDir, reloader.userCalls[0])
	// novel reload 不应被调用
	assert.False(t, reloader.novelReloaded[0])
}

// TestInstallRemoteSkill_NovelLayer target=novel，验证文件写入 + ReloadNovel 被调用。
func TestInstallRemoteSkill_NovelLayer(t *testing.T) {
	fetcher := &mockFetcher{
		content: map[string][]byte{"skills/my-skill.md": []byte("# novel skill content")},
		err:     map[string]error{},
	}
	reloader := &mockReloader{}
	svc, _, _, skillDir := newTestService(t, fetcher, reloader)

	err := svc.InstallRemoteSkill(context.Background(), "my-skill", "novel", 42)
	require.NoError(t, err)

	// 验证文件已写入
	written, err := os.ReadFile(filepath.Join(skillDir, "my-skill.md"))
	require.NoError(t, err)
	assert.Equal(t, "# novel skill content", string(written))

	// 验证 ReloadNovel 被调用，且 novelID 正确
	assert.True(t, reloader.novelReloaded[42])
	require.Len(t, reloader.novelCalls, 1)
	assert.Equal(t, int64(42), reloader.novelCalls[0].id)
	assert.Equal(t, skillDir, reloader.novelCalls[0].dir)
	// user reload 不应被调用
	assert.False(t, reloader.userReloaded)
}

// TestInstallRemoteSkill_NovelLayerZeroID target=novel + novelID=0，验证错误。
func TestInstallRemoteSkill_NovelLayerZeroID(t *testing.T) {
	fetcher := &mockFetcher{
		content: map[string][]byte{"skills/my-skill.md": []byte("# content")},
		err:     map[string]error{},
	}
	reloader := &mockReloader{}
	svc, _, _, _ := newTestService(t, fetcher, reloader)

	err := svc.InstallRemoteSkill(context.Background(), "my-skill", "novel", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-zero novelID")
	// 失败时不应触发 reload
	assert.False(t, reloader.userReloaded)
	assert.Empty(t, reloader.novelReloaded)
}

// TestInstallRemoteSkill_InvalidTarget target="invalid"，验证错误。
func TestInstallRemoteSkill_InvalidTarget(t *testing.T) {
	fetcher := &mockFetcher{
		content: map[string][]byte{"skills/my-skill.md": []byte("# content")},
		err:     map[string]error{},
	}
	reloader := &mockReloader{}
	svc, _, _, _ := newTestService(t, fetcher, reloader)

	err := svc.InstallRemoteSkill(context.Background(), "my-skill", "invalid", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid target")
	// 失败时不应触发 reload
	assert.False(t, reloader.userReloaded)
	assert.Empty(t, reloader.novelReloaded)
}

// TestInstallRemoteSkill_ReloadFailureStillSucceeds ReloadUser 返回 error，
// 验证 InstallRemoteSkill 仍返回 nil（只 warn 不 fail）。
func TestInstallRemoteSkill_ReloadFailureStillSucceeds(t *testing.T) {
	fetcher := &mockFetcher{
		content: map[string][]byte{"skills/my-skill.md": []byte("# content")},
		err:     map[string]error{},
	}
	reloader := &mockReloader{
		userErr: errors.New("reload failed"),
	}
	svc, _, _, skillDir := newTestService(t, fetcher, reloader)

	err := svc.InstallRemoteSkill(context.Background(), "my-skill", "user", 0)
	require.NoError(t, err, "reload 失败不应导致 InstallRemoteSkill 失败")

	// 文件应已写入
	written, err := os.ReadFile(filepath.Join(skillDir, "my-skill.md"))
	require.NoError(t, err)
	assert.Equal(t, "# content", string(written))
	// reload 仍被调用，只是返回了 error
	assert.True(t, reloader.userReloaded)
}

// TestDefaultDirResolver_InvalidTarget 验证默认目录解析器对非法 target 的处理。
func TestDefaultDirResolver_InvalidTarget(t *testing.T) {
	_, err := defaultDirResolver("bogus", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid target")
}

// TestDefaultDirResolver_NovelZeroID 验证默认目录解析器对 novel + novelID=0 的处理。
func TestDefaultDirResolver_NovelZeroID(t *testing.T) {
	_, err := defaultDirResolver("novel", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-zero novelID")
}
