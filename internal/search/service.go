package search

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"novel/internal/chapter"
	"novel/internal/character"
	"novel/internal/git"
	"novel/internal/location"
	"novel/internal/rag"
	"novel/internal/storage"
	"novel/internal/storyarc"
	"novel/internal/timeline"
)

// Service 协调全局搜索：实体 LIKE + 正文精确 + RAG 语义。
// 缓存章节正文在内存中，避免重复读文件。
type Service struct {
	logger      *slog.Logger
	charStore   *character.Store
	locStore    *location.Store
	tlStore     *timeline.Store
	arcStore    *storyarc.Store
	chapStore   *chapter.Store
	vectorStore *rag.VectorStore

	mu    sync.RWMutex
	cache map[int64]map[int]string // novelID → chapterNum → content
}

// NewService 创建搜索服务。
func NewService(logger *slog.Logger, charStore *character.Store, locStore *location.Store,
	tlStore *timeline.Store, arcStore *storyarc.Store, chapStore *chapter.Store,
	vecStore *rag.VectorStore) *Service {
	return &Service{
		logger:      logger,
		charStore:   charStore,
		locStore:    locStore,
		tlStore:     tlStore,
		arcStore:    arcStore,
		chapStore:   chapStore,
		vectorStore: vecStore,
		cache:       make(map[int64]map[int]string),
	}
}

// SearchAll 执行全局搜索，并发执行三类搜索后合并结果。
func (s *Service) SearchAll(ctx context.Context, novelID int64, query string) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	var (
		wg             sync.WaitGroup
		entityResults  []Result
		contentResults []Result
		ragResults     []Result
	)

	wg.Add(3)
	go func() { defer wg.Done(); entityResults = s.searchEntities(ctx, novelID, query) }()
	go func() { defer wg.Done(); contentResults = s.searchContent(ctx, novelID, query) }()
	go func() { defer wg.Done(); ragResults = s.searchRAG(ctx, novelID, query) }()
	wg.Wait()

	var results []Result
	results = append(results, entityResults...)
	results = append(results, contentResults...)
	results = append(results, ragResults...)

	return results, nil
}

// searchEntities 在各实体 store 上执行 LIKE 搜索。
func (s *Service) searchEntities(ctx context.Context, novelID int64, query string) []Result {
	var results []Result

	// 人物
	chars, err := s.charStore.ListByNovel(ctx, novelID, character.ListByNovelOptions{
		Search:     query,
		PageParams: storage.PageParams{Page: 1, Size: EntityLimit},
	})
	if err != nil {
		s.logger.Warn("character search failed", "err", err)
	} else if chars != nil {
		for _, c := range chars.Items {
			results = append(results, Result{
				Type:    "character",
				ID:      c.ID,
				Title:   c.Name,
				PanelID: "characters",
			})
		}
	}

	// 地点
	locs, err := s.locStore.ListByNovel(ctx, novelID, location.ListByNovelOptions{
		Search:     query,
		PageParams: storage.PageParams{Page: 1, Size: EntityLimit},
	})
	if err != nil {
		s.logger.Warn("location search failed", "err", err)
	} else if locs != nil {
		for _, l := range locs.Items {
			results = append(results, Result{
				Type:     "location",
				ID:       l.ID,
				Title:    l.Name,
				Subtitle: l.LocationType,
				PanelID:  "locations",
			})
		}
	}

	// 时间线
	timelineEntries, err := s.tlStore.SearchByNovel(ctx, novelID, query, EntityLimit)
	if err != nil {
		s.logger.Warn("timeline search failed", "err", err)
	} else {
		for _, e := range timelineEntries {
			subtitle := e.Category
			switch e.Category {
			case "foreshadowing":
				subtitle = "伏笔"
			case "user_directive":
				subtitle = "用户指令"
			}
			results = append(results, Result{
				Type:       "timeline",
				ID:         e.ID,
				Title:      e.Title,
				Subtitle:   subtitle,
				ChapterNum: e.TargetChapter,
				PanelID:    "timeline",
			})
		}
	}

	// 故事弧
	arcs, err := s.arcStore.SearchByNovel(ctx, novelID, query, EntityLimit)
	if err != nil {
		s.logger.Warn("story arc search failed", "err", err)
	} else {
		for _, arc := range arcs {
			results = append(results, Result{
				Type:     "storyarc",
				ID:       arc.ID,
				Title:    arc.Name,
				Subtitle: arc.ArcType,
				PanelID:  "storyarcs",
			})
		}
	}

	// 章节
	chapters, err := s.chapStore.SearchByNovel(ctx, novelID, query, EntityLimit)
	if err != nil {
		s.logger.Warn("chapter title search failed", "err", err)
	} else {
		for _, ch := range chapters {
			results = append(results, Result{
				Type:       "chapter",
				ID:         ch.ID,
				Title:      ch.Title,
				Subtitle:   "标题匹配",
				ChapterNum: ch.ChapterNumber,
				FilePath:   ch.FilePath,
				PanelID:    "chapters",
			})
		}
	}

	return results
}

// searchContent 在章节正文中做精确字符串匹配，使用内存缓存避免重复读文件。
func (s *Service) searchContent(ctx context.Context, novelID int64, query string) []Result {
	s.ensureContentCache(novelID)

	s.mu.RLock()
	srcMap := s.cache[novelID]
	// 拷贝一份防止 UpdateCachedChapter 并发写入
	chapMap := make(map[int]string, len(srcMap))
	for k, v := range srcMap {
		chapMap[k] = v
	}
	s.mu.RUnlock()

	if len(chapMap) == 0 {
		return nil
	}

	type match struct {
		chapNum  int
		position int // 命中字符偏移（按 rune 计）
	}
	var matches []match

	// 按章节号有序遍历
	chapNums := make([]int, 0, len(chapMap))
	for n := range chapMap {
		chapNums = append(chapNums, n)
	}
	sort.Ints(chapNums)

	for _, chapNum := range chapNums {
		content := chapMap[chapNum]
		pos := 0
		for {
			idx := strings.Index(content[pos:], query)
			if idx < 0 {
				break
			}
			absPos := utf8.RuneCountInString(content[:pos+idx])
			matches = append(matches, match{chapNum: chapNum, position: absPos})
			pos = pos + idx + len(query)
		}
		if len(matches) >= ContentLimit {
			break
		}
	}

	if len(matches) > ContentLimit {
		matches = matches[:ContentLimit]
	}

	// 获取章节元数据（标题）
	allChapters, err := s.chapStore.ListAllByNovel(ctx, novelID)
	chapMeta := make(map[int]chapter.Chapter)
	if err != nil {
		s.logger.Warn("chapter list for search content failed", "err", err)
	} else {
		for _, ch := range allChapters {
			chapMeta[ch.ChapterNumber] = ch
		}
	}

	var results []Result
	for _, m := range matches {
		content := chapMap[m.chapNum]
		prefix, hit, suffix := buildContext(content, m.position, query)
		meta := chapMeta[m.chapNum]
		results = append(results, Result{
			Type:          "content",
			ID:            0,
			Title:         meta.Title,
			ChapterNum:    m.chapNum,
			FilePath:      git.ChapterPath(m.chapNum),
			MatchPrefix:   prefix,
			MatchHit:      hit,
			MatchSuffix:   suffix,
			MatchPosition: m.position,
			MatchLen:      utf8.RuneCountInString(query),
			Relevance:     1,
			PanelID:       "chapters",
		})
	}

	return results
}

// buildContext 提取命中位置前后 ContextRadius 个中文字符的上下文，返回前缀、命中词、后缀。
// 前端负责用 JSX 渲染高亮，避免 dangerouslySetInnerHTML。
func buildContext(content string, matchRunePos int, query string) (prefix, hit, suffix string) {
	runes := []rune(content)
	total := len(runes)
	qLen := utf8.RuneCountInString(query)

	start := matchRunePos - ContextRadius
	if start < 0 {
		start = 0
	}
	end := matchRunePos + qLen + ContextRadius
	if end > total {
		end = total
	}

	if start > 0 {
		prefix = "..."
	}
	prefix += string(runes[start:matchRunePos])
	hit = string(runes[matchRunePos : matchRunePos+qLen])
	suffix = string(runes[matchRunePos+qLen : end])
	if end < total {
		suffix += "..."
	}

	return prefix, hit, suffix
}

// ensureContentCache 懒加载指定小说的全部章节内容到内存。
func (s *Service) ensureContentCache(novelID int64) {
	s.mu.RLock()
	_, ok := s.cache[novelID]
	s.mu.RUnlock()
	if ok {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.cache[novelID]; ok {
		return
	}

	chapters, err := s.chapStore.ListAllByNovel(context.Background(), novelID)
	if err != nil {
		s.logger.Warn("搜索缓存: 获取章节列表失败", "novel_id", novelID, "err", err)
		return
	}

	chapMap := make(map[int]string, len(chapters))
	for _, ch := range chapters {
		content, err := git.ReadFile(novelID, ch.FilePath)
		if err != nil {
			continue
		}
		chapMap[ch.ChapterNumber] = content
	}

	s.cache[novelID] = chapMap
	s.logger.Debug("搜索缓存: 已加载章节内容", "novel_id", novelID, "chapters", len(chapMap))
}

// UpdateCachedChapter 在章节保存后更新缓存中的对应章节内容。
func (s *Service) UpdateCachedChapter(novelID int64, chapterNum int, newContent string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if chapMap, ok := s.cache[novelID]; ok {
		chapMap[chapterNum] = newContent
	}
}

// searchRAG 执行向量语义搜索。
func (s *Service) searchRAG(ctx context.Context, novelID int64, query string) []Result {
	if s.vectorStore == nil {
		return nil
	}

	fetchK := RagTopK * 2
	if fetchK > 40 {
		fetchK = 40
	}

	ragResults, err := s.vectorStore.Search(ctx, novelID, query, fetchK, nil)
	if err != nil {
		s.logger.Warn("搜索: RAG 检索失败", "novel_id", novelID, "err", err)
		return nil
	}

	filtered := make([]rag.SearchResult, 0, len(ragResults))
	for _, r := range ragResults {
		if r.Relevance >= 0.3 {
			filtered = append(filtered, r)
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	reranked := rag.MMRRerank(query, filtered, RagTopK, 0.7)

	allChapters, err := s.chapStore.ListAllByNovel(ctx, novelID)
	chapMeta := make(map[int]chapter.Chapter)
	if err != nil {
		s.logger.Warn("chapter list for RAG search failed", "err", err)
	} else {
		for _, ch := range allChapters {
			chapMeta[ch.ChapterNumber] = ch
		}
	}

	var results []Result
	for _, r := range reranked {
		meta := chapMeta[r.ChapterNumber]
		contentPreview := r.Content
		runes := []rune(contentPreview)
		if len(runes) > 200 {
			contentPreview = string(runes[:200]) + "..."
		}

		results = append(results, Result{
			Type:          "rag",
			ID:            0,
			Title:         meta.Title,
			ChapterNum:    r.ChapterNumber,
			FilePath:      git.ChapterPath(r.ChapterNumber),
			MatchPrefix:   contentPreview,
			MatchLen:      utf8.RuneCountInString(r.Content),
			MatchPosition: r.StartRunePos,
			Relevance:     r.Relevance,
			PanelID:       "chapters",
		})
	}

	return results
}
