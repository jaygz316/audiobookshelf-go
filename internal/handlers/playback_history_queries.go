package handlers

const listeningStatsQuery = `
	SELECT ps.id, ps.userId, u.username, ps.mediaItemId, ps.mediaItemType, ps.startTime, 
	       COALESCE(ps.updatedAt, ps.createdAt, '') as updatedAt, COALESCE(ps.extraData, '') as extraData,
	       CASE 
	           WHEN ps.mediaItemType = 'podcastEpisode' THEN COALESCE(pe.title, '')
	           WHEN ps.mediaItemType = 'podcast' THEN COALESCE(p.title, '')
	           ELSE COALESCE(b.title, '')
	       END as title,
	       CASE 
	           WHEN ps.mediaItemType = 'podcastEpisode' THEN COALESCE(p.author, '')
	           WHEN ps.mediaItemType = 'podcast' THEN COALESCE(p.author, '')
	           ELSE COALESCE(li.authorNamesFirstLast, '')
	       END as author,
	       CASE 
	           WHEN ps.mediaItemType = 'podcastEpisode' THEN COALESCE(p.genres, '[]')
	           WHEN ps.mediaItemType = 'podcast' THEN COALESCE(p.genres, '[]')
	           ELSE COALESCE(b.genres, '[]')
	       END as genres
	FROM playbackSessions ps
	LEFT JOIN users u ON u.id = ps.userId
	LEFT JOIN books b ON b.id = ps.mediaItemId AND ps.mediaItemType = 'book'
	LEFT JOIN libraryItems li ON li.mediaId = ps.mediaItemId AND li.mediaType = 'book' AND ps.mediaItemType = 'book'
	LEFT JOIN podcastEpisodes pe ON pe.id = ps.mediaItemId AND ps.mediaItemType = 'podcastEpisode'
	LEFT JOIN podcasts p ON (p.id = pe.podcastId AND ps.mediaItemType = 'podcastEpisode') OR (p.id = ps.mediaItemId AND ps.mediaItemType = 'podcast')
`

const listeningSessionsQuery = `
	SELECT ps.id, ps.userId, u.username, ps.mediaItemId, ps.mediaItemType, ps.startTime, 
	       COALESCE(ps.updatedAt, ps.createdAt, '') as updatedAt, COALESCE(ps.extraData, '') as extraData,
	       CASE 
	           WHEN ps.mediaItemType = 'podcastEpisode' THEN COALESCE(pe.title, '')
	           WHEN ps.mediaItemType = 'podcast' THEN COALESCE(p.title, '')
	           ELSE COALESCE(b.title, '')
	       END as title,
	       CASE 
	           WHEN ps.mediaItemType = 'podcastEpisode' THEN COALESCE(p.author, '')
	           WHEN ps.mediaItemType = 'podcast' THEN COALESCE(p.author, '')
	           ELSE COALESCE(li.authorNamesFirstLast, '')
	       END as author
	FROM playbackSessions ps
	LEFT JOIN users u ON u.id = ps.userId
	LEFT JOIN books b ON b.id = ps.mediaItemId AND ps.mediaItemType = 'book'
	LEFT JOIN libraryItems li ON li.mediaId = ps.mediaItemId AND li.mediaType = 'book' AND ps.mediaItemType = 'book'
	LEFT JOIN podcastEpisodes pe ON pe.id = ps.mediaItemId AND ps.mediaItemType = 'podcastEpisode'
	LEFT JOIN podcasts p ON (p.id = pe.podcastId AND ps.mediaItemType = 'podcastEpisode') OR (p.id = ps.mediaItemId AND ps.mediaItemType = 'podcast')
`
