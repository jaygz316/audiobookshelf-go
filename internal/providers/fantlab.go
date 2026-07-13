package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

type FantLabProvider struct{}

func (p *FantLabProvider) Name() string {
	return "fantlab"
}

var filterWorkTypes = map[int]bool{
	7:  true,
	11: true,
	12: true,
	22: true,
	23: true,
	24: true,
	25: true,
	26: true,
	46: true,
	47: true,
	49: true,
	51: true,
	52: true,
	55: true,
	56: true,
	57: true,
}

type fantLabSearchItem struct {
	WorkID     int `json:"work_id"`
	WorkTypeID int `json:"work_type_id"`
}

type fantLabAuthor struct {
	Name string `json:"name"`
}

type fantLabGenre struct {
	Label string         `json:"label"`
	Genre []fantLabGenre `json:"genre"`
}

type fantLabGenreGroup struct {
	GenreGroupID int            `json:"genre_group_id"`
	Genre        []fantLabGenre `json:"genre"`
}

type fantLabClassificatory struct {
	GenreGroup []fantLabGenreGroup `json:"genre_group"`
}

type fantLabEditionItem struct {
	EditionID int    `json:"edition_id"`
	ISBN      string `json:"isbn"`
}

type fantLabEditionBlock struct {
	List []fantLabEditionItem `json:"list"`
}

type fantLabWorkExtended struct {
	WorkID          int                            `json:"work_id"`
	WorkName        string                         `json:"work_name"`
	WorkNameAlts    []string                       `json:"work_name_alts"`
	WorkYear        int                            `json:"work_year"`
	WorkDescription string                         `json:"work_description"`
	Image           string                         `json:"image"`
	Authors         []fantLabAuthor                `json:"authors"`
	Classificatory  *fantLabClassificatory         `json:"classificatory"`
	EditionsBlocks  map[string]fantLabEditionBlock `json:"editions_blocks"`
}

type fantLabEditionResponse struct {
	Image string `json:"image"`
}

func (p *FantLabProvider) getCoverFromEdition(ctx context.Context, editionID int) (string, error) {
	if editionID == 0 {
		return "", nil
	}
	urlStr := fmt.Sprintf("https://api.fantlab.ru/edition/%d", editionID)
	resp, err := getWithRetry(ctx, safeHTTPClient, urlStr)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("edition request returned status %d", resp.StatusCode)
	}

	var edResp fantLabEditionResponse
	if err := json.NewDecoder(resp.Body).Decode(&edResp); err != nil {
		return "", err
	}
	return edResp.Image, nil
}

func (p *FantLabProvider) tryGetCoverFromEditions(ctx context.Context, editions map[string]fantLabEditionBlock) (string, string) {
	if editions == nil {
		return "", ""
	}
	var list []fantLabEditionItem
	if block, ok := editions["30"]; ok && len(block.List) > 0 {
		list = block.List
	} else if block, ok := editions["10"]; ok && len(block.List) > 0 {
		list = block.List
	}

	if len(list) == 0 {
		return "", ""
	}

	lastEdition := list[len(list)-1]
	imageUrl, _ := p.getCoverFromEdition(ctx, lastEdition.EditionID)
	return imageUrl, lastEdition.ISBN
}

func tryGetGenres(classificatory *fantLabClassificatory) []string {
	if classificatory == nil || len(classificatory.GenreGroup) == 0 {
		return nil
	}
	var genresGroup *fantLabGenreGroup
	for i := range classificatory.GenreGroup {
		if classificatory.GenreGroup[i].GenreGroupID == 1 {
			genresGroup = &classificatory.GenreGroup[i]
			break
		}
	}
	if genresGroup == nil || len(genresGroup.Genre) == 0 {
		return nil
	}

	rootGenre := genresGroup.Genre[0]
	genres := []string{rootGenre.Label}
	genres = append(genres, tryGetSubGenres(rootGenre)...)
	return genres
}

func tryGetSubGenres(rootGenre fantLabGenre) []string {
	if len(rootGenre.Genre) == 0 {
		return nil
	}
	var sub []string
	for _, g := range rootGenre.Genre {
		if g.Label != "" {
			sub = append(sub, g.Label)
		}
	}
	return sub
}

func (p *FantLabProvider) SearchBooks(ctx context.Context, query string) ([]*MetadataResult, error) {
	if query == "" {
		return nil, nil
	}

	escapedQuery := url.QueryEscape(query)
	urlStr := fmt.Sprintf("https://api.fantlab.ru/search-works?q=%s&page=1&onlymatches=1", escapedQuery)

	resp, err := getWithRetry(ctx, safeHTTPClient, urlStr)
	if err != nil {
		return nil, fmt.Errorf("search works failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fantlab search returned status %d", resp.StatusCode)
	}

	var searchItems []fantLabSearchItem
	if err := json.NewDecoder(resp.Body).Decode(&searchItems); err != nil {
		return nil, fmt.Errorf("failed to decode search results: %w", err)
	}

	// Limit to first 10 items to prevent flooding
	if len(searchItems) > 10 {
		searchItems = searchItems[:10]
	}

	results := make([]*MetadataResult, len(searchItems))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3) // Concurrency limit

	for i, item := range searchItems {
		wg.Add(1)
		go func(idx int, sItem fantLabSearchItem) {
			defer wg.Done()

			if filterWorkTypes[sItem.WorkTypeID] {
				return
			}

			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			extendedUrl := fmt.Sprintf("https://api.fantlab.ru/work/%d/extended", sItem.WorkID)
			extResp, err := getWithRetry(ctx, safeHTTPClient, extendedUrl)
			if err != nil {
				return
			}
			defer extResp.Body.Close()

			if extResp.StatusCode != http.StatusOK {
				return
			}

			var bookData fantLabWorkExtended
			if err := json.NewDecoder(extResp.Body).Decode(&bookData); err != nil {
				return
			}

			// Clean book data
			var authorNames []string
			for _, au := range bookData.Authors {
				trimmed := strings.TrimSpace(au.Name)
				if trimmed != "" {
					authorNames = append(authorNames, trimmed)
				}
			}

			coverImg, isbn := p.tryGetCoverFromEditions(ctx, bookData.EditionsBlocks)
			if coverImg == "" {
				coverImg = bookData.Image
			}

			coverURL := ""
			if coverImg != "" {
				coverURL = "https://fantlab.ru" + coverImg
			}

			subtitle := ""
			if len(bookData.WorkNameAlts) > 0 {
				subtitle = bookData.WorkNameAlts[0]
			}

			publishedYear := ""
			if bookData.WorkYear > 0 {
				publishedYear = strconv.Itoa(bookData.WorkYear)
			}

			results[idx] = &MetadataResult{
				Title:         bookData.WorkName,
				Subtitle:      subtitle,
				Authors:       authorNames,
				PublishedYear: publishedYear,
				Description:   bookData.WorkDescription,
				CoverURL:      coverURL,
				ISBN:          isbn,
			}
		}(i, item)
	}
	wg.Wait()

	var cleaned []*MetadataResult
	for _, r := range results {
		if r != nil {
			cleaned = append(cleaned, r)
		}
	}
	return cleaned, nil
}

func (p *FantLabProvider) SearchPodcasts(ctx context.Context, query string) ([]*MetadataResult, error) {
	return nil, nil
}
