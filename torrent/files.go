package torrent

/*
	Torrent byte stream

	0 ───────────── 999
		file A

	1000 ───────── 2999
		file B
*/

/*
	movie.mp4
	Path   = ["Movie", "movie.mp4"]
	Offset = 0
	Length = 1000

	subtitles.srt
	Path   = ["Movie", "subtitles.srt"]
	Offset = 1000
	Length = 200

	poster.jpg
	Path   = ["Movie", "poster.jpg"]
	Offset = 1200
	Length = 500
*/
type FileSegment struct {
	Path   []string
	Offset int64
	Length int64
}

func (i *Info) FileSegments() []FileSegment {
	var result []FileSegment

	var offset int64

	for _, file := range i.Files {
		result = append(result, FileSegment{
			Path:   file.Path,
			Offset: offset,
			Length: file.Length,
		})

		offset += file.Length
	}

	return result
}
