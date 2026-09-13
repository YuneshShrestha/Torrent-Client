package torrent

/*
	Torrent byte stream

	0 ───────────── 999
		file A

	1000 ───────── 2999
		file B
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
