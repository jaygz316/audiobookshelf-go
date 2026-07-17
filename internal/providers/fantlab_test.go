package providers

import (
	"context"
	"testing"
)

func TestFantLabHelpers_tryGetGenres(t *testing.T) {
	t.Run("nil classificatory", func(t *testing.T) {
		res := tryGetGenres(nil)
		if res != nil {
			t.Errorf("expected nil genres, got %v", res)
		}
	})

	t.Run("empty genre group list", func(t *testing.T) {
		res := tryGetGenres(&fantLabClassificatory{
			GenreGroup: []fantLabGenreGroup{},
		})
		if res != nil {
			t.Errorf("expected nil genres, got %v", res)
		}
	})

	t.Run("no genre group with ID 1", func(t *testing.T) {
		res := tryGetGenres(&fantLabClassificatory{
			GenreGroup: []fantLabGenreGroup{
				{GenreGroupID: 2},
			},
		})
		if res != nil {
			t.Errorf("expected nil genres, got %v", res)
		}
	})

	t.Run("genre group 1 is empty", func(t *testing.T) {
		res := tryGetGenres(&fantLabClassificatory{
			GenreGroup: []fantLabGenreGroup{
				{GenreGroupID: 1, Genre: []fantLabGenre{}},
			},
		})
		if res != nil {
			t.Errorf("expected nil genres, got %v", res)
		}
	})

	t.Run("genre group 1 has root genre", func(t *testing.T) {
		res := tryGetGenres(&fantLabClassificatory{
			GenreGroup: []fantLabGenreGroup{
				{
					GenreGroupID: 1,
					Genre: []fantLabGenre{
						{
							Label: "Sci-Fi",
							Genre: []fantLabGenre{
								{Label: "Space Opera"},
								{Label: "Cyberpunk"},
							},
						},
					},
				},
			},
		})
		expected := []string{"Sci-Fi", "Space Opera", "Cyberpunk"}
		if len(res) != len(expected) {
			t.Fatalf("expected %d genres, got %d", len(expected), len(res))
		}
		for i, v := range expected {
			if res[i] != v {
				t.Errorf("expected genres[%d] = %q, got %q", i, v, res[i])
			}
		}
	})
}

func TestFantLabHelpers_tryGetSubGenres(t *testing.T) {
	t.Run("empty genre list", func(t *testing.T) {
		res := tryGetSubGenres(fantLabGenre{
			Label: "Root",
			Genre: []fantLabGenre{},
		})
		if res != nil {
			t.Errorf("expected nil, got %v", res)
		}
	})

	t.Run("omits empty labels", func(t *testing.T) {
		res := tryGetSubGenres(fantLabGenre{
			Label: "Root",
			Genre: []fantLabGenre{
				{Label: "Sub1"},
				{Label: ""},
				{Label: "Sub2"},
			},
		})
		expected := []string{"Sub1", "Sub2"}
		if len(res) != len(expected) {
			t.Fatalf("expected %d, got %d", len(expected), len(res))
		}
		for i, v := range expected {
			if res[i] != v {
				t.Errorf("expected subgenres[%d] = %q, got %q", i, v, res[i])
			}
		}
	})
}

func TestFantLabHelpers_getCoverFromEdition(t *testing.T) {
	t.Run("edition ID 0", func(t *testing.T) {
		provider := &FantLabProvider{}
		res, err := provider.getCoverFromEdition(context.Background(), 0)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if res != "" {
			t.Errorf("expected empty string, got %s", res)
		}
	})
}

func TestFantLabHelpers_tryGetCoverFromEditions(t *testing.T) {
	t.Run("nil editions", func(t *testing.T) {
		provider := &FantLabProvider{}
		cover, isbn := provider.tryGetCoverFromEditions(context.Background(), nil)
		if cover != "" || isbn != "" {
			t.Errorf("expected empty, got cover=%q isbn=%q", cover, isbn)
		}
	})

	t.Run("no matching blocks", func(t *testing.T) {
		provider := &FantLabProvider{}
		editions := map[string]fantLabEditionBlock{
			"20": {
				List: []fantLabEditionItem{
					{EditionID: 123, ISBN: "1234567890"},
				},
			},
		}
		cover, isbn := provider.tryGetCoverFromEditions(context.Background(), editions)
		if cover != "" || isbn != "" {
			t.Errorf("expected empty, got cover=%q isbn=%q", cover, isbn)
		}
	})
}
