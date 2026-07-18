package app

import "novel/internal/git"

// CommitFileListResult 包含一次 commit 的详细信息和变更文件列表（不含内容）。
type CommitFileListResult struct {
	Commit *git.CommitInfo `json:"commit"`
	Files  []git.FileEntry `json:"files"`
}

// GetCommitLog 获取指定小说的 Git 提交历史。
// afterHash 非空时返回该 commit 之后的更早提交（游标翻页）。
func (a *App) GetCommitLog(novelID int64, n int, afterHash string) ([]git.CommitInfo, error) {
	repo, err := git.New(novelID, a.settings.GitName, a.settings.GitEmail, a.logger)
	if err != nil {
		return nil, err
	}
	return repo.LogDetailed(n, afterHash)
}

// GetCommitFileList 获取指定 commit 的详细信息和变更文件列表（不含文件内容）。
func (a *App) GetCommitFileList(novelID int64, hash string) (*CommitFileListResult, error) {
	repo, err := git.New(novelID, a.settings.GitName, a.settings.GitEmail, a.logger)
	if err != nil {
		return nil, err
	}
	info, entries, err := repo.CommitFileList(hash)
	if err != nil {
		return nil, err
	}
	return &CommitFileListResult{Commit: info, Files: entries}, nil
}

// GetFileDiff 获取指定 commit 中某个文件的前后内容。
func (a *App) GetFileDiff(novelID int64, hash string, filePath string) (*git.FileDiff, error) {
	repo, err := git.New(novelID, a.settings.GitName, a.settings.GitEmail, a.logger)
	if err != nil {
		return nil, err
	}
	return repo.ShowFile(hash, filePath)
}
