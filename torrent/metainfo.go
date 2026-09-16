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

const PieceHashLength = 20

type MetaInfo struct {
	Announce string /// Main tracker

	AnnounceList [][]string /// Multiple trackers grouped into tiers

	Info Info /// This contains information about what is actually being downloaded.

	InfoHash [20]byte /// SHA-1 hash of bencoded info dictionary [The tracker and peers use this hash to identify the torrent.]
}

type Info struct {
	Name string /// Name of the torrent

	PieceLength int64 /// Size of each piece

	Pieces []byte /// Torrent stores the SHA-1 hashes of each piece

	Length int64

	Files []FileInfo
}

type FileInfo struct {
	Path   []string
	Length int64
}

func Load(path string) (*MetaInfo, error) {
	/// Read the .torrent file
	/*
		ubuntu.torrent
			│
			▼
		raw bytes [bytes are bencoded]
	*/
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	/// Decode the torrent to Go struct [so root is decoded torrent]
	root, err := DecodeBencode(data)
	if err != nil {
		return nil, err
	}

	/// Make sure the torrent is a dictionary
	/*
		Conceptually:
			torrent
				│
				▼
				dictionary
				{
					"announce": ...,
					"info": ...
				}

	*/
	rootDict, ok := root.(map[string]any)

	if !ok {
		return nil, fmt.Errorf("torrent root is not dictionary")
	}

	/*
		Ex:
			{
				"announce": "...",
				"info": {
					"name": "file.iso",
					"piece length": 262144,
					"pieces": ...
				}
			}

	*/
	infoRaw, ok := rootDict["info"]
	if !ok {
		return nil, fmt.Errorf("missing info dictionary")
	}

	infoDict, ok := infoRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid info dictionary")
	}

	/// Convert generic map[string]any to our Info struct
	info, err := parseInfo(infoDict)
	if err != nil {
		return nil, err
	}

	infoBytes, err := encodeBencode(infoDict)
	if err != nil {
		return nil, err
	}

	/// Calculates `SHA1(bencoded(info))`
	/*
		                 bencoded
							info
							│
							▼
						┌────────────┐
						│   SHA-1    │
						└────────────┘
							│
							▼
						20 bytes
							│
							▼
						InfoHash

	*/
	infoHash := sha1.Sum(infoBytes)

	meta := &MetaInfo{
		Info:     *info,
		InfoHash: infoHash,
	}

	if announce, ok := rootDict["announce"].([]byte); ok {
		meta.Announce = string(announce)
	}

	/*
		A torrent can have multiple trackers.

		They can be organized into tiers.

		For example:

		announce-list

		Tier 0
		├── tracker1
		└── tracker2

		Tier 1
		├── tracker3
		└── tracker4

		Note:
		rootDict := map[string]any{
			"announce": []byte("http://tracker1.com/announce"),

			"announce-list": []any{
				[]any{
					[]byte("http://tracker1.com/announce"),
					[]byte("http://tracker2.com/announce"),
				},
				[]any{
					[]byte("http://tracker3.com/announce"),
					[]byte("http://tracker4.com/announce"),
				},
			},

			"info": map[string]any{
				// torrent information...
			},
		}
	*/

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

// converts the generic info dictionary into our Info struct
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

	/// Gets all piece SHA-1 hashes.
	pieces, ok := dict["pieces"].([]byte)
	if !ok {
		return nil, fmt.Errorf("missing pieces")
	}

	/// Make sure total number of bytes can be divided into 20-byte hashes.
	if len(pieces)%PieceHashLength != 0 {
		return nil, fmt.Errorf("invalid pieces length")
	}

	result.Pieces = pieces

	/*Single file torrent
	Looks like:
		info
		├── name
		├── piece length
		├── pieces
		└── length

	*/
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

	/// Multi-file torrent doesn't have length instead it has files
	/*
		Conceptually:

		info
		├── name
		├── piece length
		├── pieces
		└── files
			│
			├── file 1
			├── file 2
			└── file 3
		Where each file contains `length` and `path`

		Ex:

		files:
			[
				{
					length: 1000,
					path: ["folder", "a.txt"]
				},
				{
					length: 2000,
					path: ["folder", "b.txt"]
				}
			]
	*/
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
	/*
		Because every piece has exactly one 20-byte SHA-1 hash:
		number of pieces =
			total piece-hash bytes / 20
	*/
	return len(m.Info.Pieces) / PieceHashLength
}

func (m *MetaInfo) PieceHash(index int) [PieceHashLength]byte {
	// This retrieves the SHA-1 hash for one particular piece.
	/*
		Pieces:
		┌────────────────────┬────────────────────┬────────────────────┐
		│      Piece 0       │      Piece 1       │      Piece 2       │
		│      20 bytes      │      20 bytes      │      20 bytes      │
		└────────────────────┴────────────────────┴────────────────────┘
				0-19                20-39               40-59

		`m.Info.Pieces[20:40]`

		gets Piece 1's hash.
	*/
	var hash [PieceHashLength]byte

	start := index * PieceHashLength

	copy(
		hash[:],
		m.Info.Pieces[start:start+PieceHashLength],
	)

	return hash
}
