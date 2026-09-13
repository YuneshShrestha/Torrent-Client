package torrent

/*
This parses the torrent metadata and calculates:

info_hash = SHA1(bencoded(info))
*/
import (
	"crypto/sha1"
	"fmt"
	"os"
)

const PieceLengthHash = 20

type MetaInfo struct {
	Announce string

	AnnounceList [][]string

	Info Info

	InfoHash [20]byte
}

type Info struct {
	Name string

	PieceLength int64

	Pieces []byte

	Length int64

	Files []FileInfo
}

type FileInfo struct {
	Path   []string
	Length int64
}

func Load(path string) (*MetaInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	root, err := DecodeBencode(data)
	if err != nil {
		return nil, err
	}

	rootDict, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("torrent root is not dictionary")
	}

	infoRaw, ok := rootDict["info"]
	if !ok {
		return nil, fmt.Errorf("missing info dictionary")
	}

	infoDict, ok := infoRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid info dictionary")
	}

	info, err := parseInfo(infoDict)
	if err != nil {
		return nil, err
	}

	infoBytes, err := encodeBencode(infoDict)
	if err != nil {
		return nil, err
	}

	infoHash := sha1.Sum(infoBytes)

	meta := &MetaInfo{
		Info:     *info,
		InfoHash: infoHash,
	}

	if announce, ok := rootDict["announce"].([]byte); ok {
		meta.Announce = string(announce)
	}

	if announceList, ok := rootDict["announce-list"].([]any); ok {
		for _, tierRaw := range announceList {
			tierList, ok := tierRaw.([]any)
			if !ok {
				continue
			}

			var tier []string

			for _, trackerRaw := range tierList {
				if trackerBytes, ok := trackerRaw.([]byte); ok {
					tier = append(tier, string(trackerBytes))
				}
			}

			if len(tier) > 0 {
				meta.AnnounceList = append(
					meta.AnnounceList,
					tier,
				)
			}
		}
	}

	return meta, nil
}

func parseInfo(dict map[string]any) (*Info, error) {
	result := &Info{}

	nameRaw, ok := dict["name"].([]byte)
	if !ok {
		return nil, fmt.Errorf("missing info.name")
	}

	result.Name = string(nameRaw)

	pieceLengthRaw, ok := dict["piece length"].(int64)
	if !ok {
		return nil, fmt.Errorf("missing piece length")
	}

	result.PieceLength = pieceLengthRaw

	pieces, ok := dict["pieces"].([]byte)
	if !ok {
		return nil, fmt.Errorf("missing pieces")
	}

	if len(pieces)%20 != 0 {
		return nil, fmt.Errorf("invalid pieces length")
	}

	result.Pieces = pieces

	if lengthRaw, ok := dict["length"].(int64); ok {
		result.Length = lengthRaw

		result.Files = []FileInfo{
			{
				Path:   []string{result.Name},
				Length: lengthRaw,
			},
		}

		return result, nil
	}

	filesRaw, ok := dict["files"].([]any)
	if !ok {
		return nil, fmt.Errorf("torrent has neither length nor files")
	}

	for _, fileRaw := range filesRaw {
		fileDict, ok := fileRaw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid file entry")
		}

		length, ok := fileDict["length"].(int64)
		if !ok {
			return nil, fmt.Errorf("invalid file length")
		}

		pathRaw, ok := fileDict["path"].([]any)
		if !ok {
			return nil, fmt.Errorf("invalid file path")
		}

		var path []string

		for _, partRaw := range pathRaw {
			part, ok := partRaw.([]byte)
			if !ok {
				return nil, fmt.Errorf("invalid path component")
			}

			path = append(path, string(part))
		}

		result.Files = append(result.Files, FileInfo{
			Path:   path,
			Length: length,
		})

		result.Length += length
	}

	return result, nil
}

func (m *MetaInfo) PieceCount() int {
	return len(m.Info.Pieces) / 20
}

func (m *MetaInfo) PieceHash(index int) [20]byte {
	var hash [20]byte

	start := index * 20

	copy(
		hash[:],
		m.Info.Pieces[start:start+20],
	)

	return hash
}
