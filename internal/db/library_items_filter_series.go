package db

import (
	"database/sql"
	"fmt"
	"strings"
)

func fetchSeriesForBooks(db *sql.DB, bookIDs []string, bookMap map[string]*BookMinifiedJSON) error {
	if len(bookIDs) == 0 {
		return nil
	}

	placeholders := make([]string, len(bookIDs))
	queryArgs := make([]interface{}, len(bookIDs))
	for i, id := range bookIDs {
		placeholders[i] = "?"
		queryArgs[i] = id
	}

	seriesQuery := fmt.Sprintf(`
		SELECT bs.bookId, s.id, s.name, bs.sequence
		FROM bookSeries bs
		JOIN series s ON bs.seriesId = s.id
		WHERE bs.bookId IN (%s)
		ORDER BY CAST(bs.sequence AS FLOAT) ASC NULLS LAST
	`, strings.Join(placeholders, ","))

	sRows, err := db.Query(seriesQuery, queryArgs...)
	if err != nil {
		return err
	}
	defer sRows.Close()

	bookSeriesMap := make(map[string][]*BookSeriesMinifiedJSON)
	for sRows.Next() {
		var bookID, seriesID, seriesName string
		var sequence sql.NullString
		if err := sRows.Scan(&bookID, &seriesID, &seriesName, &sequence); err == nil {
			var seqVal string
			if sequence.Valid {
				seqVal = sequence.String
			}
			bookSeriesMap[bookID] = append(bookSeriesMap[bookID], &BookSeriesMinifiedJSON{
				ID:       seriesID,
				Name:     seriesName,
				Sequence: seqVal,
			})
		}
	}

	for bID, bookMin := range bookMap {
		if sList, ok := bookSeriesMap[bID]; ok {
			bookMin.Metadata.Series = sList

			var nameSeqs []string
			for _, s := range sList {
				if s.Sequence != "" {
					nameSeqs = append(nameSeqs, fmt.Sprintf("%s #%s", s.Name, s.Sequence))
				} else {
					nameSeqs = append(nameSeqs, s.Name)
				}
			}
			bookMin.Metadata.SeriesName = strings.Join(nameSeqs, ", ")

			if len(sList) > 0 && sList[0].Sequence != "" {
				seq := sList[0].Sequence
				bookMin.Metadata.SeriesSequence = &seq
			}
		}
	}
	return nil
}
