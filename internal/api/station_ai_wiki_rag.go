package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	stationAIWikiRAGRootDir              = "data/wiki-rag"
	stationAIWikiRAGSyncInterval         = time.Hour
	stationAIWikiTopK                    = 6
	stationAIWikiMaxChunkTokens          = 800
	stationAIWikiChunkOverlapTokens      = 120
	stationAIWikiRAGEmbeddingModel       = "text-embedding-3-small"
	stationAIWikiRAGLocalEmbeddingDim    = 384
	stationAIWikiRAGRRFK                 = 60.0
	stationAIWikiLowConfidenceSimilarity = 0.30
)

var (
	stationAIWikiHeadingRe = regexp.MustCompile(`^(#{1,3})\s+(.+?)\s*$`)
	stationAIWikiTokenRe   = regexp.MustCompile(`[\p{L}\p{N}_+\-]+`)
)

type stationAIWikiRAG struct {
	mu       sync.Mutex
	cond     *sync.Cond
	repo     string
	index    *stationAIWikiRAGIndex
	lastSync time.Time
	building bool
}

type stationAIWikiRAGIndex struct {
	Repo           string               `json:"repo"`
	BuiltAt        string               `json:"built_at"`
	FileHashes     map[string]string    `json:"file_hashes"`
	EmbeddingKind  string               `json:"embedding_kind"`
	EmbeddingModel string               `json:"embedding_model"`
	EmbeddingDim   int                  `json:"embedding_dim"`
	Chunks         []stationAIWikiChunk `json:"chunks"`
	Vectors        [][]float32          `json:"vectors"`

	docTF     []map[string]int `json:"-"`
	docLen    []int            `json:"-"`
	docFreq   map[string]int   `json:"-"`
	avgDocLen float64          `json:"-"`
}

type stationAIWikiChunk struct {
	ID         string `json:"id"`
	SourcePath string `json:"source_path"`
	Page       string `json:"page"`
	PageSlug   string `json:"page_slug"`
	Section    string `json:"section"`
	Breadcrumb string `json:"breadcrumb"`
	Locale     string `json:"locale"`
	Content    string `json:"content"`
	TokenCount int    `json:"token_count"`
}

type stationAIWikiSection struct {
	Headings []string
	Content  string
}

type stationAIWikiDocScore struct {
	Index int
	Score float64
}

type stationAIWikiRankedDoc struct {
	Index        int
	VectorScore  float64
	KeywordScore float64
	HybridScore  float64
}

func newStationAIWikiRAG() *stationAIWikiRAG {
	r := &stationAIWikiRAG{}
	r.cond = sync.NewCond(&r.mu)
	return r
}

func (r *stationAIWikiRAG) Start(defaultRepo string) {
	repo := sanitizeWikiRepo(defaultRepo)
	if repo == "" {
		return
	}
	go func() {
		_, _ = r.ensureIndex(context.Background(), repo, true)
		ticker := time.NewTicker(stationAIWikiRAGSyncInterval)
		defer ticker.Stop()
		for range ticker.C {
			_, _ = r.ensureIndex(context.Background(), repo, true)
		}
	}()
}

func (r *stationAIWikiRAG) Retrieve(
	ctx context.Context,
	repo string,
	locale string,
	query string,
	intent stationAIIntentKind,
	topK int,
) ([]aiKnowledgeSnippet, []string, error) {
	repo = sanitizeWikiRepo(repo)
	if strings.TrimSpace(repo) == "" {
		repo = defaultStationAIWikiRepo
	}
	if topK <= 0 {
		topK = stationAIWikiTopK
	}
	idx, err := r.ensureIndex(ctx, repo, false)
	if err != nil && idx == nil {
		return nil, nil, err
	}
	if idx == nil || len(idx.Chunks) == 0 || len(idx.Vectors) == 0 {
		return nil, nil, fmt.Errorf("wiki rag index is empty")
	}

	ranked, bestVec := idx.hybridSearch(ctx, repo, locale, query, intent, topK)
	if len(ranked) == 0 {
		return nil, nil, nil
	}

	out := make([]aiKnowledgeSnippet, 0, len(ranked))
	for _, doc := range ranked {
		ch := idx.Chunks[doc.Index]
		title := ch.Page
		if strings.TrimSpace(ch.Section) != "" && !strings.EqualFold(ch.Section, ch.Page) {
			title = fmt.Sprintf("%s - %s", ch.Page, ch.Section)
		}
		url := fmt.Sprintf("https://github.com/%s/wiki/%s", repo, ch.PageSlug)
		out = append(out, aiKnowledgeSnippet{
			SourceLabel:  "WIKI",
			Title:        title,
			Page:         ch.Page,
			Section:      ch.Section,
			Locale:       ch.Locale,
			URL:          url,
			Content:      aiTrimForPrompt(strings.TrimSpace(ch.Content), 760),
			Score:        int(math.Round(doc.HybridScore * 1000)),
			VectorScore:  doc.VectorScore,
			KeywordScore: doc.KeywordScore,
			HybridScore:  doc.HybridScore,
		})
	}

	warnings := make([]string, 0, 1)
	if bestVec > 0 && bestVec < stationAIWikiLowConfidenceSimilarity {
		warnings = append(
			warnings,
			fmt.Sprintf("wiki context low-confidence (top similarity %.2f)", bestVec),
		)
	}
	return out, warnings, nil
}

func (r *stationAIWikiRAG) ForceRefresh(ctx context.Context, repo string) (*stationAIWikiRAGIndex, error) {
	repo = sanitizeWikiRepo(repo)
	if strings.TrimSpace(repo) == "" {
		repo = defaultStationAIWikiRepo
	}
	return r.ensureIndex(ctx, repo, true)
}

func (r *stationAIWikiRAG) ensureIndex(ctx context.Context, repo string, force bool) (*stationAIWikiRAGIndex, error) {
	r.mu.Lock()
	for r.building {
		r.cond.Wait()
	}
	needsSync := force ||
		r.index == nil ||
		r.repo != repo ||
		time.Since(r.lastSync) >= stationAIWikiRAGSyncInterval
	if !needsSync {
		idx := r.index
		r.mu.Unlock()
		return idx, nil
	}
	r.building = true
	r.mu.Unlock()

	idx, err := stationAIBuildOrLoadWikiIndex(ctx, repo)

	r.mu.Lock()
	if err == nil && idx != nil {
		r.index = idx
		r.repo = repo
	}
	// Throttle retries after failures too.
	r.lastSync = time.Now()
	current := r.index
	currentRepo := r.repo
	r.building = false
	r.cond.Broadcast()
	r.mu.Unlock()

	// If refresh failed but we still have a usable in-memory index for this repo, keep serving.
	if err != nil && current != nil && currentRepo == repo {
		return current, nil
	}
	return current, err
}

func stationAIBuildOrLoadWikiIndex(ctx context.Context, repo string) (*stationAIWikiRAGIndex, error) {
	repoKey := strings.ReplaceAll(repo, "/", "__")
	rootDir := filepath.Join(stationAIWikiRAGRootDir, repoKey)
	mirrorDir := filepath.Join(rootDir, "mirror")
	indexPath := filepath.Join(rootDir, "index.json")
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, err
	}

	if err := stationAISyncWikiMirror(ctx, repo, mirrorDir); err != nil {
		return nil, err
	}

	fileHashes, mdFiles, err := stationAICollectWikiMarkdownHashes(mirrorDir)
	if err != nil {
		return nil, err
	}

	if cached, err := stationAILoadWikiIndex(indexPath); err == nil && cached != nil {
		if cached.Repo == repo && stationAIEqualHashes(cached.FileHashes, fileHashes) {
			cached.rebuildLexical()
			return cached, nil
		}
	}

	built, err := stationAIBuildWikiIndex(ctx, repo, mirrorDir, mdFiles, fileHashes)
	if err != nil {
		return nil, err
	}
	built.rebuildLexical()
	_ = stationAISaveWikiIndex(indexPath, built)
	return built, nil
}

func stationAISyncWikiMirror(ctx context.Context, repo, mirrorDir string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git is required for wiki mirror: %w", err)
	}
	remoteURL := fmt.Sprintf("https://github.com/%s.wiki.git", repo)
	_, statErr := os.Stat(filepath.Join(mirrorDir, ".git"))
	if statErr == nil {
		log.Printf("[AI][WIKI-RAG] sync start (pull): repo=%s", repo)
		// Existing mirror: fast-forward pull. If it fails, reclone to keep state predictable.
		if err := stationAIRunGit(ctx, "-C", mirrorDir, "remote", "set-url", "origin", remoteURL); err != nil {
			return err
		}
		if err := stationAIRunGit(ctx, "-C", mirrorDir, "pull", "--ff-only", "--quiet"); err == nil {
			return nil
		}
		_ = os.RemoveAll(mirrorDir)
	}
	if err := os.MkdirAll(filepath.Dir(mirrorDir), 0o755); err != nil {
		return err
	}
	log.Printf("[AI][WIKI-RAG] sync start (clone): repo=%s", repo)
	return stationAIRunGit(ctx, "clone", "--depth", "1", remoteURL, mirrorDir)
}

func stationAIRunGit(ctx context.Context, args ...string) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(timeoutCtx, "git", args...)
	hideConsoleWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return err
		}
		return fmt.Errorf("git %s failed: %s", strings.Join(args, " "), msg)
	}
	return nil
}

func stationAICollectWikiMarkdownHashes(mirrorDir string) (map[string]string, []string, error) {
	hashes := make(map[string]string, 32)
	files := make([]string, 0, 32)
	err := filepath.WalkDir(mirrorDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".md" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(mirrorDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		sum := sha256.Sum256(raw)
		hashes[rel] = hex.EncodeToString(sum[:])
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, nil, fmt.Errorf("wiki mirror has no markdown files")
	}
	return hashes, files, nil
}

func stationAILoadWikiIndex(indexPath string) (*stationAIWikiRAGIndex, error) {
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}
	var idx stationAIWikiRAGIndex
	if err := json.Unmarshal(raw, &idx); err != nil {
		return nil, err
	}
	if len(idx.Chunks) == 0 || len(idx.Chunks) != len(idx.Vectors) {
		return nil, fmt.Errorf("invalid wiki rag index data")
	}
	if idx.EmbeddingDim <= 0 {
		if len(idx.Vectors) > 0 {
			idx.EmbeddingDim = len(idx.Vectors[0])
		} else {
			idx.EmbeddingDim = stationAIWikiRAGLocalEmbeddingDim
		}
	}
	idx.rebuildLexical()
	return &idx, nil
}

func stationAISaveWikiIndex(indexPath string, idx *stationAIWikiRAGIndex) error {
	raw, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	tmp := indexPath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, indexPath)
}

func stationAIEqualHashes(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func stationAIBuildWikiIndex(
	ctx context.Context,
	repo string,
	mirrorDir string,
	mdFiles []string,
	fileHashes map[string]string,
) (*stationAIWikiRAGIndex, error) {
	chunks := make([]stationAIWikiChunk, 0, 256)
	embedInputs := make([]string, 0, 256)

	chunkSeq := 0
	for _, rel := range mdFiles {
		abs := filepath.Join(mirrorDir, filepath.FromSlash(rel))
		raw, err := os.ReadFile(abs)
		if err != nil {
			return nil, err
		}
		pageSlug := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
		page := strings.ReplaceAll(pageSlug, "-", " ")
		sections := stationAIExtractMarkdownSections(string(raw))
		if len(sections) == 0 {
			sections = []stationAIWikiSection{{Headings: nil, Content: strings.TrimSpace(string(raw))}}
		}
		for _, sec := range sections {
			secText := strings.TrimSpace(sec.Content)
			if secText == "" {
				continue
			}
			secChunks := stationAIRecursiveSplitWithOverlap(
				secText,
				stationAIWikiMaxChunkTokens,
				stationAIWikiChunkOverlapTokens,
			)
			sectionLabel := page
			if len(sec.Headings) > 0 {
				sectionLabel = strings.Join(sec.Headings, " > ")
			}
			breadcrumb := stationAIBuildBreadcrumb(page, sec.Headings)
			for _, chunkText := range secChunks {
				chunkText = strings.TrimSpace(chunkText)
				if chunkText == "" {
					continue
				}
				chunkSeq++
				ch := stationAIWikiChunk{
					ID:         fmt.Sprintf("%s:%d", rel, chunkSeq),
					SourcePath: rel,
					Page:       page,
					PageSlug:   pageSlug,
					Section:    sectionLabel,
					Breadcrumb: breadcrumb,
					Locale:     stationAIDetectLocale(chunkText),
					Content:    chunkText,
					TokenCount: stationAITokenCount(chunkText),
				}
				chunks = append(chunks, ch)
				embedInputs = append(embedInputs, breadcrumb+"\n"+chunkText)
			}
		}
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("wiki rag build produced no chunks")
	}

	vectors, kind, model, dim, err := stationAIEmbedTexts(ctx, embedInputs)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(chunks) {
		return nil, fmt.Errorf("wiki rag embeddings mismatch: %d != %d", len(vectors), len(chunks))
	}

	return &stationAIWikiRAGIndex{
		Repo:           repo,
		BuiltAt:        time.Now().UTC().Format(time.RFC3339),
		FileHashes:     fileHashes,
		EmbeddingKind:  kind,
		EmbeddingModel: model,
		EmbeddingDim:   dim,
		Chunks:         chunks,
		Vectors:        vectors,
	}, nil
}

func stationAIExtractMarkdownSections(md string) []stationAIWikiSection {
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	sections := make([]stationAIWikiSection, 0, 24)
	headingStack := make([]string, 0, 3)
	var buf strings.Builder

	flush := func() {
		text := strings.TrimSpace(buf.String())
		if text == "" {
			buf.Reset()
			return
		}
		headings := append([]string(nil), headingStack...)
		sections = append(sections, stationAIWikiSection{
			Headings: headings,
			Content:  text,
		})
		buf.Reset()
	}

	for _, line := range lines {
		if m := stationAIWikiHeadingRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			level := len(m[1])
			title := strings.TrimSpace(m[2])
			if level >= 1 && level <= 3 && title != "" {
				flush()
				for len(headingStack) >= level {
					headingStack = headingStack[:len(headingStack)-1]
				}
				for len(headingStack) < level-1 {
					headingStack = append(headingStack, "Section")
				}
				headingStack = append(headingStack, title)
				continue
			}
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	flush()
	return sections
}

func stationAIRecursiveSplitWithOverlap(text string, maxTokens, overlapTokens int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	base := stationAIRecursiveSplit(text, maxTokens, []string{"\n\n", "\n", ". "})
	if overlapTokens <= 0 || len(base) <= 1 {
		return base
	}
	out := make([]string, 0, len(base))
	prevTokens := make([]string, 0, overlapTokens)
	for i, chunk := range base {
		toks := strings.Fields(chunk)
		if i > 0 && len(prevTokens) > 0 {
			ov := prevTokens
			if len(ov) > overlapTokens {
				ov = ov[len(ov)-overlapTokens:]
			}
			toks = append(append([]string(nil), ov...), toks...)
		}
		out = append(out, strings.TrimSpace(strings.Join(toks, " ")))
		prevTokens = strings.Fields(chunk)
	}
	return out
}

func stationAIRecursiveSplit(text string, maxTokens int, separators []string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if stationAITokenCount(text) <= maxTokens {
		return []string{text}
	}
	if len(separators) == 0 {
		return stationAISlidingTokenSplit(text, maxTokens)
	}
	sep := separators[0]
	parts := strings.Split(text, sep)
	if len(parts) <= 1 {
		return stationAIRecursiveSplit(text, maxTokens, separators[1:])
	}

	grouped := make([]string, 0, len(parts))
	current := ""
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		candidate := part
		if current != "" {
			candidate = current + sep + part
		}
		if current == "" || stationAITokenCount(candidate) <= maxTokens {
			current = candidate
			continue
		}
		grouped = append(grouped, current)
		current = part
	}
	if strings.TrimSpace(current) != "" {
		grouped = append(grouped, current)
	}
	out := make([]string, 0, len(grouped))
	for _, g := range grouped {
		if stationAITokenCount(g) <= maxTokens {
			out = append(out, strings.TrimSpace(g))
			continue
		}
		out = append(out, stationAIRecursiveSplit(g, maxTokens, separators[1:])...)
	}
	return out
}

func stationAISlidingTokenSplit(text string, maxTokens int) []string {
	toks := strings.Fields(text)
	if len(toks) == 0 {
		return nil
	}
	if maxTokens <= 0 {
		maxTokens = stationAIWikiMaxChunkTokens
	}
	out := make([]string, 0, (len(toks)/maxTokens)+1)
	for start := 0; start < len(toks); start += maxTokens {
		end := start + maxTokens
		if end > len(toks) {
			end = len(toks)
		}
		out = append(out, strings.Join(toks[start:end], " "))
	}
	return out
}

func stationAIBuildBreadcrumb(page string, headings []string) string {
	parts := []string{fmt.Sprintf("Page: %s", strings.TrimSpace(page))}
	for _, h := range headings {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		parts = append(parts, h)
	}
	return "[" + strings.Join(parts, " > ") + "]"
}

func stationAITokenCount(text string) int {
	return len(strings.Fields(strings.TrimSpace(text)))
}

func stationAIDetectLocale(text string) string {
	total := 0
	cyr := 0
	for _, r := range text {
		if !unicode.IsLetter(r) {
			continue
		}
		total++
		if unicode.In(r, unicode.Cyrillic) {
			cyr++
		}
	}
	if total == 0 {
		return "en"
	}
	if float64(cyr)/float64(total) >= 0.25 {
		return "ru"
	}
	return "en"
}

func (idx *stationAIWikiRAGIndex) rebuildLexical() {
	n := len(idx.Chunks)
	idx.docTF = make([]map[string]int, n)
	idx.docLen = make([]int, n)
	idx.docFreq = make(map[string]int, 512)
	totalLen := 0
	for i, ch := range idx.Chunks {
		tf := make(map[string]int, 64)
		seen := make(map[string]struct{}, 64)
		docText := ch.Content + "\n" + ch.Section + "\n" + ch.Page
		for _, tok := range stationAITokenize(docText) {
			tf[tok]++
			if _, ok := seen[tok]; !ok {
				seen[tok] = struct{}{}
				idx.docFreq[tok]++
			}
		}
		docLen := 0
		for _, c := range tf {
			docLen += c
		}
		idx.docTF[i] = tf
		idx.docLen[i] = docLen
		totalLen += docLen
	}
	if n > 0 {
		idx.avgDocLen = float64(totalLen) / float64(n)
	}
}

func stationAITokenize(text string) []string {
	raw := stationAIWikiTokenRe.FindAllString(strings.ToLower(text), -1)
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, t := range raw {
		t = strings.TrimSpace(t)
		if len(t) < 2 {
			continue
		}
		out = append(out, t)
	}
	return out
}

func (idx *stationAIWikiRAGIndex) hybridSearch(
	ctx context.Context,
	repo string,
	locale string,
	query string,
	intent stationAIIntentKind,
	topK int,
) ([]stationAIWikiRankedDoc, float64) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, 0
	}
	expandedQuery := stationAIExpandQuery(query, intent)
	pageHints := stationAIQueryHintPageSet(expandedQuery)

	candidates := idx.filteredCandidateIndexes(locale, intent, pageHints)
	if len(candidates) == 0 {
		return nil, 0
	}

	queryTokens := stationAITokenize(expandedQuery)
	vectorScores := make([]stationAIWikiDocScore, 0, len(candidates))
	bestVector := 0.0

	queryVec, qErr := stationAIEmbedQuery(ctx, expandedQuery, idx.EmbeddingKind, idx.EmbeddingModel, idx.EmbeddingDim)
	if qErr == nil && len(queryVec) > 0 {
		for _, docIdx := range candidates {
			score := stationAICosine(queryVec, idx.Vectors[docIdx])
			if score <= 0 {
				continue
			}
			vectorScores = append(vectorScores, stationAIWikiDocScore{Index: docIdx, Score: score})
			if score > bestVector {
				bestVector = score
			}
		}
		sort.SliceStable(vectorScores, func(i, j int) bool {
			if vectorScores[i].Score == vectorScores[j].Score {
				return vectorScores[i].Index < vectorScores[j].Index
			}
			return vectorScores[i].Score > vectorScores[j].Score
		})
	}

	bm25Scores := make([]stationAIWikiDocScore, 0, len(candidates))
	if len(queryTokens) > 0 && idx.avgDocLen > 0 {
		nDocs := len(idx.Chunks)
		for _, docIdx := range candidates {
			score := stationAIBM25(
				queryTokens,
				idx.docTF[docIdx],
				idx.docLen[docIdx],
				idx.avgDocLen,
				idx.docFreq,
				nDocs,
			)
			if score <= 0 {
				continue
			}
			bm25Scores = append(bm25Scores, stationAIWikiDocScore{Index: docIdx, Score: score})
		}
		sort.SliceStable(bm25Scores, func(i, j int) bool {
			if bm25Scores[i].Score == bm25Scores[j].Score {
				return bm25Scores[i].Index < bm25Scores[j].Index
			}
			return bm25Scores[i].Score > bm25Scores[j].Score
		})
	}

	if len(vectorScores) == 0 && len(bm25Scores) == 0 {
		return nil, bestVector
	}

	rankedMap := make(map[int]*stationAIWikiRankedDoc, topK*4)
	pushRRF := func(list []stationAIWikiDocScore, isVector bool) {
		for rank, ds := range list {
			entry := rankedMap[ds.Index]
			if entry == nil {
				entry = &stationAIWikiRankedDoc{Index: ds.Index}
				rankedMap[ds.Index] = entry
			}
			if isVector {
				entry.VectorScore = ds.Score
			} else {
				entry.KeywordScore = ds.Score
			}
			entry.HybridScore += 1.0 / (stationAIWikiRAGRRFK + float64(rank+1))
		}
	}
	pushRRF(vectorScores, true)
	pushRRF(bm25Scores, false)
	if len(pageHints) > 0 {
		for _, entry := range rankedMap {
			if entry == nil || entry.Index < 0 || entry.Index >= len(idx.Chunks) {
				continue
			}
			if _, ok := pageHints[idx.Chunks[entry.Index].PageSlug]; ok {
				// Query->page explicit hint: promote exact wiki page matches.
				entry.HybridScore += 0.012
			}
		}
	}

	ranked := make([]stationAIWikiRankedDoc, 0, len(rankedMap))
	for _, entry := range rankedMap {
		ranked = append(ranked, *entry)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].HybridScore == ranked[j].HybridScore {
			return ranked[i].VectorScore > ranked[j].VectorScore
		}
		return ranked[i].HybridScore > ranked[j].HybridScore
	})
	if len(ranked) > topK {
		ranked = ranked[:topK]
	}
	return ranked, bestVector
}

func (idx *stationAIWikiRAGIndex) filteredCandidateIndexes(
	locale string,
	intent stationAIIntentKind,
	pageHints map[string]struct{},
) []int {
	locale = strings.TrimSpace(strings.ToLower(locale))
	if locale != "ru" && locale != "en" {
		locale = ""
	}
	allowedPages := stationAIAllowedWikiPages(intent)

	build := func(useLocale bool, useIntent bool, useHints bool) []int {
		out := make([]int, 0, len(idx.Chunks))
		for i, ch := range idx.Chunks {
			if useLocale && locale != "" && ch.Locale != locale {
				continue
			}
			if useIntent && len(allowedPages) > 0 {
				if _, ok := allowedPages[ch.PageSlug]; !ok {
					continue
				}
			}
			if useHints && len(pageHints) > 0 {
				if _, ok := pageHints[ch.PageSlug]; !ok {
					continue
				}
			}
			out = append(out, i)
		}
		return out
	}

	// 1) strict locale + intent + explicit page hints
	if out := build(true, true, true); len(out) > 0 {
		return out
	}
	// 2) any locale + intent + explicit page hints
	if out := build(false, true, true); len(out) > 0 {
		return out
	}
	// 3) any locale + any page + explicit page hints
	if out := build(false, false, true); len(out) > 0 {
		return out
	}
	// 4) strict locale + intent
	if out := build(true, true, false); len(out) > 0 {
		return out
	}
	// 5) any locale + intent
	if out := build(false, true, false); len(out) > 0 {
		return out
	}
	// 6) any locale + any page
	return build(false, false, false)
}

func stationAIAllowedWikiPages(intent stationAIIntentKind) map[string]struct{} {
	switch intent {
	case stationAIIntentTrading:
		return map[string]struct{}{
			"Station-Trading":          {},
			"Execution-Plan":           {},
			"Radius-Scan":              {},
			"Region-Arbitrage":         {},
			"Route-Trading":            {},
			"Contract-Scanner":         {},
			"Industry-Chain-Optimizer": {},
			"Getting-Started":          {},
		}
	case stationAIIntentDebug:
		return map[string]struct{}{
			"Getting-Started": {},
			"API-Reference":   {},
			"Execution-Plan":  {},
			"Station-Trading": {},
			"Home":            {},
		}
	case stationAIIntentProduct:
		return map[string]struct{}{
			"Home":                     {},
			"Getting-Started":          {},
			"API-Reference":            {},
			"Station-Trading":          {},
			"Execution-Plan":           {},
			"Region-Arbitrage":         {},
			"Route-Trading":            {},
			"Contract-Scanner":         {},
			"Industry-Chain-Optimizer": {},
			"PLEX-Dashboard":           {},
			"War-Tracker":              {},
		}
	default:
		return nil
	}
}

func stationAIExpandQuery(query string, intent stationAIIntentKind) string {
	query = strings.TrimSpace(query)
	lower := strings.ToLower(query)
	expanded := query

	parts := strings.Fields(lower)
	if len(parts) <= 5 {
		switch intent {
		case stationAIIntentTrading:
			expanded += " station trading metrics cts sds pvi filters scan parameters"
		case stationAIIntentDebug:
			expanded += " troubleshooting errors station ai config"
		case stationAIIntentProduct:
			expanded += " product workflow settings documentation"
		}
	}
	if strings.Contains(lower, "sds") {
		expanded += " scam detection score"
	}
	if strings.Contains(lower, "cts") {
		expanded += " composite trade score"
	}
	if strings.Contains(lower, "pvi") {
		expanded += " price volatility index"
	}
	if strings.Contains(lower, "bvs") || strings.Contains(lower, "s2b") || strings.Contains(lower, "bfs") {
		expanded += " buy sell flow ratio market velocity"
	}
	return strings.TrimSpace(expanded)
}

func stationAIQueryHintPageSet(query string) map[string]struct{} {
	query = stationAINormalizeHintQuery(query)
	if query == "" {
		return nil
	}

	out := make(map[string]struct{}, 4)
	add := func(page string) {
		page = strings.TrimSpace(page)
		if page == "" {
			return
		}
		out[page] = struct{}{}
	}
	containsAll := func(parts ...string) bool {
		for _, p := range parts {
			if !strings.Contains(query, p) {
				return false
			}
		}
		return true
	}

	if strings.Contains(query, "radius scan") || strings.Contains(query, "radius tab") ||
		containsAll("радиус", "скан") {
		add("Radius-Scan")
	}
	if strings.Contains(query, "station trading") ||
		containsAll("station", "trade") ||
		containsAll("станц", "трейд") ||
		strings.Contains(query, "cts") || strings.Contains(query, "sds") ||
		strings.Contains(query, "pvi") || strings.Contains(query, "bvs") ||
		strings.Contains(query, "s2b") || strings.Contains(query, "bfs") {
		add("Station-Trading")
	}
	if strings.Contains(query, "execution plan") ||
		containsAll("execution", "plan") ||
		containsAll("команд", "центр") ||
		containsAll("done", "ignored") {
		add("Execution-Plan")
	}
	if strings.Contains(query, "api reference") ||
		strings.Contains(query, "api endpoint") ||
		strings.Contains(query, "/api/") {
		add("API-Reference")
	}
	if strings.Contains(query, "plex dashboard") || containsAll("omega", "plex") {
		add("PLEX-Dashboard")
	}
	if containsAll("war", "tracker") || containsAll("war", "hot") || containsAll("войн", "трек") {
		add("War-Tracker")
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func stationAINormalizeHintQuery(query string) string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		",", " ",
		".", " ",
		":", " ",
		";", " ",
		"!", " ",
		"?", " ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		"{", " ",
		"}", " ",
		"\"", " ",
		"'", " ",
		"\n", " ",
		"\t", " ",
		"/", " ",
		"\\", " ",
		"-", " ",
	)
	return strings.Join(strings.Fields(replacer.Replace(query)), " ")
}

func stationAIBM25(
	queryTokens []string,
	tf map[string]int,
	docLen int,
	avgDocLen float64,
	docFreq map[string]int,
	nDocs int,
) float64 {
	if len(queryTokens) == 0 || len(tf) == 0 || docLen <= 0 || avgDocLen <= 0 || nDocs <= 0 {
		return 0
	}
	const k1 = 1.5
	const b = 0.75
	score := 0.0
	seen := map[string]struct{}{}
	for _, term := range queryTokens {
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		freq := tf[term]
		if freq <= 0 {
			continue
		}
		df := docFreq[term]
		if df <= 0 {
			continue
		}
		idf := math.Log(1 + (float64(nDocs-df)+0.5)/(float64(df)+0.5))
		numer := float64(freq) * (k1 + 1)
		denom := float64(freq) + k1*(1-b+b*(float64(docLen)/avgDocLen))
		score += idf * (numer / denom)
	}
	return score
}

func stationAIEmbedTexts(
	ctx context.Context,
	texts []string,
) (vectors [][]float32, kind string, model string, dim int, err error) {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	model = strings.TrimSpace(os.Getenv("OPENAI_EMBEDDING_MODEL"))
	if model == "" {
		model = stationAIWikiRAGEmbeddingModel
	}
	if apiKey != "" {
		if vecs, err := stationAIEmbedWithOpenAI(ctx, texts, apiKey, model); err == nil && len(vecs) == len(texts) {
			embedDim := 0
			if len(vecs) > 0 {
				embedDim = len(vecs[0])
			}
			return vecs, "openai", model, embedDim, nil
		}
	}
	vecs := stationAIEmbedLocal(texts, stationAIWikiRAGLocalEmbeddingDim)
	return vecs, "local", "local-hash", stationAIWikiRAGLocalEmbeddingDim, nil
}

func stationAIEmbedQuery(ctx context.Context, text, kind, model string, dim int) ([]float32, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	switch strings.TrimSpace(strings.ToLower(kind)) {
	case "openai":
		apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		if apiKey == "" {
			return nil, fmt.Errorf("openai api key is not configured")
		}
		vecs, err := stationAIEmbedWithOpenAI(ctx, []string{text}, apiKey, model)
		if err != nil || len(vecs) != 1 {
			return nil, err
		}
		return vecs[0], nil
	default:
		if dim <= 0 {
			dim = stationAIWikiRAGLocalEmbeddingDim
		}
		vecs := stationAIEmbedLocal([]string{text}, dim)
		if len(vecs) == 0 {
			return nil, nil
		}
		return vecs[0], nil
	}
}

func stationAIEmbedWithOpenAI(ctx context.Context, texts []string, apiKey, model string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	base := strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	endpoint := strings.TrimRight(base, "/") + "/embeddings"
	out := make([][]float32, 0, len(texts))

	type requestPayload struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}
	type responsePayload struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	for start := 0; start < len(texts); start += 64 {
		end := start + 64
		if end > len(texts) {
			end = len(texts)
		}
		payload := requestPayload{
			Model: model,
			Input: texts[start:end],
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		client := &http.Client{Timeout: 25 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("openai embeddings http %d", resp.StatusCode)
		}

		var parsed responsePayload
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, err
		}
		if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
			return nil, fmt.Errorf("openai embeddings error: %s", parsed.Error.Message)
		}
		sort.SliceStable(parsed.Data, func(i, j int) bool {
			return parsed.Data[i].Index < parsed.Data[j].Index
		})
		for _, item := range parsed.Data {
			vec := make([]float32, len(item.Embedding))
			for i, v := range item.Embedding {
				vec[i] = float32(v)
			}
			stationAINormalizeVector(vec)
			out = append(out, vec)
		}
	}
	if len(out) != len(texts) {
		return nil, fmt.Errorf("openai embeddings count mismatch")
	}
	return out, nil
}

func stationAIEmbedLocal(texts []string, dim int) [][]float32 {
	if dim <= 0 {
		dim = stationAIWikiRAGLocalEmbeddingDim
	}
	out := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vec := make([]float32, dim)
		tokens := stationAITokenize(text)
		if len(tokens) == 0 {
			out = append(out, vec)
			continue
		}
		for _, tok := range tokens {
			h := stationAIHashToken(tok)
			idx := int(h % uint64(dim))
			sign := float32(1.0)
			if (h>>63)&1 == 1 {
				sign = -1.0
			}
			vec[idx] += sign
		}
		stationAINormalizeVector(vec)
		out = append(out, vec)
	}
	return out
}

func stationAIHashToken(token string) uint64 {
	// FNV-1a 64-bit
	const (
		offset uint64 = 1469598103934665603
		prime  uint64 = 1099511628211
	)
	h := offset
	for i := 0; i < len(token); i++ {
		h ^= uint64(token[i])
		h *= prime
	}
	return h
}

func stationAINormalizeVector(vec []float32) {
	sum := 0.0
	for _, v := range vec {
		sum += float64(v * v)
	}
	if sum <= 0 {
		return
	}
	inv := 1.0 / math.Sqrt(sum)
	for i, v := range vec {
		vec[i] = float32(float64(v) * inv)
	}
}

func stationAICosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	dot := 0.0
	for i := 0; i < len(a); i++ {
		dot += float64(a[i] * b[i])
	}
	return dot
}
