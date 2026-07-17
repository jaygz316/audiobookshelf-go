package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

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
