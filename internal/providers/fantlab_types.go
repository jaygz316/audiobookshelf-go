package providers

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
