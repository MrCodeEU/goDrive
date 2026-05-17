package search

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/v2/analysis/lang/en"
	"github.com/blevesearch/bleve/v2/analysis/token/lowercase"
	"github.com/blevesearch/bleve/v2/analysis/token/porter"
	"github.com/blevesearch/bleve/v2/analysis/tokenizer/letter"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search/query"
)

// BleveEngine implements Engine using an on-disk Bleve index.
type BleveEngine struct {
	index bleve.Index
}

// bleveDoc is the struct Bleve indexes. Field names must match the mapping.
type bleveDoc struct {
	UserID  string
	Path    string
	Name    string
	Content string
}

func docID(userID int64, path string) string {
	return fmt.Sprintf("%d\x00%s", userID, path)
}

const filenameAnalyzerName = "filename"

func buildMapping() *mapping.IndexMappingImpl {
	im := bleve.NewIndexMapping()

	// Register a custom "filename" analyzer that splits on non-letter characters
	// (period, dash, underscore, digits etc.) so "report.txt" → ["report", "txt"].
	// Applies lowercase + Porter stemming for English stemming support.
	if err := im.AddCustomAnalyzer(filenameAnalyzerName, map[string]any{
		"type":          custom.Name,
		"tokenizer":     letter.Name,
		"token_filters": []any{lowercase.Name, en.StopName, porter.Name},
	}); err != nil {
		panic(fmt.Sprintf("bleve: register filename analyzer: %v", err))
	}

	dm := bleve.NewDocumentMapping()

	keyword := bleve.NewTextFieldMapping()
	keyword.Analyzer = "keyword"

	keywordStored := bleve.NewTextFieldMapping()
	keywordStored.Analyzer = "keyword"
	keywordStored.Store = true

	textFilename := bleve.NewTextFieldMapping()
	textFilename.Analyzer = filenameAnalyzerName

	textENStored := bleve.NewTextFieldMapping()
	textENStored.Analyzer = en.AnalyzerName
	textENStored.Store = true

	dm.AddFieldMappingsAt("UserID", keyword)
	dm.AddFieldMappingsAt("Path", keywordStored)
	dm.AddFieldMappingsAt("Name", textFilename)
	dm.AddFieldMappingsAt("Content", textENStored)

	im.DefaultMapping = dm
	return im
}

// OpenBleve opens an existing Bleve index at dir, or creates one if absent.
func OpenBleve(dir string) (*BleveEngine, error) {
	index, err := bleve.Open(dir)
	if err == bleve.ErrorIndexPathDoesNotExist {
		index, err = bleve.New(dir, buildMapping())
	}
	if err != nil {
		return nil, err
	}
	return &BleveEngine{index: index}, nil
}

func (e *BleveEngine) IsEmpty() (bool, error) {
	n, err := e.index.DocCount()
	return n == 0, err
}

func (e *BleveEngine) IndexFile(_ context.Context, userID int64, path, name, content string) error {
	return e.index.Index(docID(userID, path), bleveDoc{
		UserID:  strconv.FormatInt(userID, 10),
		Path:    path,
		Name:    name,
		Content: content,
	})
}

func (e *BleveEngine) Delete(_ context.Context, userID int64, path string) error {
	return e.index.Delete(docID(userID, path))
}

func (e *BleveEngine) DeletePrefix(ctx context.Context, userID int64, pathPrefix string) error {
	userQ := bleve.NewTermQuery(strconv.FormatInt(userID, 10))
	userQ.SetField("UserID")

	exactQ := bleve.NewTermQuery(pathPrefix)
	exactQ.SetField("Path")
	childQ := bleve.NewPrefixQuery(pathPrefix + "/")
	childQ.SetField("Path")
	pathQ := bleve.NewDisjunctionQuery(exactQ, childQ)

	req := bleve.NewSearchRequest(bleve.NewConjunctionQuery(userQ, pathQ))
	req.Size = 10000 // sufficient for personal-drive scale; paginate if needed for large deployments
	req.Fields = []string{}

	result, err := e.index.SearchInContext(ctx, req)
	if err != nil {
		return err
	}
	if len(result.Hits) == 0 {
		return nil
	}
	batch := e.index.NewBatch()
	for _, hit := range result.Hits {
		batch.Delete(hit.ID)
	}
	return e.index.Batch(batch)
}

func (e *BleveEngine) DeleteAllForUser(ctx context.Context, userID int64) error {
	userQ := bleve.NewTermQuery(strconv.FormatInt(userID, 10))
	userQ.SetField("UserID")

	req := bleve.NewSearchRequest(userQ)
	req.Size = 10000 // sufficient for personal-drive scale; paginate if needed for large deployments
	req.Fields = []string{}

	result, err := e.index.SearchInContext(ctx, req)
	if err != nil {
		return err
	}
	if len(result.Hits) == 0 {
		return nil
	}
	batch := e.index.NewBatch()
	for _, hit := range result.Hits {
		batch.Delete(hit.ID)
	}
	return e.index.Batch(batch)
}

func (e *BleveEngine) Search(ctx context.Context, userID int64, queryStr string, limit int) ([]Result, error) {
	tokens := strings.Fields(queryStr)
	if len(tokens) == 0 {
		return nil, nil
	}

	userQ := bleve.NewTermQuery(strconv.FormatInt(userID, 10))
	userQ.SetField("UserID")

	// Each token must match in name OR content (AND across tokens).
	// For content, we combine a stemmed match with a prefix query so that
	// partial words like "plan" also hit "planner".
	tokenQueries := make([]query.Query, len(tokens))
	for i, tok := range tokens {
		nameQ := bleve.NewMatchQuery(tok)
		nameQ.SetField("Name")
		nameQ.Analyzer = filenameAnalyzerName
		nameQ.SetBoost(3.0)

		contentMatchQ := bleve.NewMatchQuery(tok)
		contentMatchQ.SetField("Content")

		contentPrefixQ := bleve.NewPrefixQuery(strings.ToLower(tok))
		contentPrefixQ.SetField("Content")

		tokenQueries[i] = bleve.NewDisjunctionQuery(nameQ, contentMatchQ, contentPrefixQ)
	}

	combined := bleve.NewConjunctionQuery(append([]query.Query{userQ}, tokenQueries...)...)

	req := bleve.NewSearchRequest(combined)
	req.Size = limit
	req.Fields = []string{"Path"}
	req.Highlight = bleve.NewHighlightWithStyle("html")
	req.Highlight.AddField("Content")
	req.Highlight.AddField("Name")

	result, err := e.index.SearchInContext(ctx, req)
	if err != nil {
		return nil, err
	}

	out := make([]Result, 0, len(result.Hits))
	for _, hit := range result.Hits {
		path, _ := hit.Fields["Path"].(string)
		if path == "" {
			continue
		}
		out = append(out, Result{
			UserID:  userID,
			Path:    path,
			Snippet: extractSnippet(hit.Fragments),
		})
	}
	return out, nil
}

func extractSnippet(fragments map[string][]string) string {
	for _, field := range []string{"Content", "Name"} {
		if frags, ok := fragments[field]; ok && len(frags) > 0 {
			s := frags[0]
			s = strings.ReplaceAll(s, "<em>", "<mark>")
			s = strings.ReplaceAll(s, "</em>", "</mark>")
			return s
		}
	}
	return ""
}

func (e *BleveEngine) Close() error {
	return e.index.Close()
}
